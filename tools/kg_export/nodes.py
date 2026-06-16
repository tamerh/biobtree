"""Phase 1: collect + type + normalize nodes -> KGX nodes.tsv.

Streams sorted index files once and:
  * collects every entity in a node-dataset (see categories.yaml) as a node,
  * merges same-entity ids via an identity allowlist (union-find) — Option C,
    "own clusters", gene-first,
  * picks one canonical CURIE per cluster (category priority) and folds the rest
    into biolink ``equivalent_identifiers``,
  * extracts a best-effort human-readable name from node property lines.

Emits KGX TSV (id, category, name, equivalent_identifiers, provided_by) plus a
JSON merge-stats report for spot-checking over-merge.
"""

from __future__ import annotations

import glob
import json
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

from . import kgx
from .categories import CategoryMap
from .curie import to_curie
from .datasets import DatasetRegistry
from .index import iter_index_file

# Internal node-key separator (unit separator; never appears in ids).
SEP = "\x1f"
PROVIDED_BY = "infores:biobtree"

# Best-effort name extraction from a property line's attribute JSON.
_NAME_KEYS_SCALAR = ("symbol", "name", "label", "preferred_name")
_NAME_KEYS_LIST = ("symbols", "names", "labels")


def tsv_safe(text: str) -> str:
    """Strip tab/newline so a free-text field can't break TSV columns."""
    return text.replace("\t", " ").replace("\n", " ").replace("\r", " ")


def extract_name(attr_json: str) -> str | None:
    try:
        d = json.loads(attr_json)
    except (ValueError, TypeError):
        return None
    if not isinstance(d, dict):
        return None
    for k in _NAME_KEYS_SCALAR:
        v = d.get(k)
        if isinstance(v, str) and v.strip():
            return v.strip()
    for k in _NAME_KEYS_LIST:
        v = d.get(k)
        if isinstance(v, list) and v and isinstance(v[0], str) and v[0].strip():
            return v[0].strip()
    return None


class UnionFind:
    """Dict-backed union-find over string node keys."""

    def __init__(self) -> None:
        self.parent: dict[str, str] = {}

    def add(self, x: str) -> None:
        if x not in self.parent:
            self.parent[x] = x

    def find(self, x: str) -> str:
        self.add(x)
        root = x
        while self.parent[root] != root:
            root = self.parent[root]
        while self.parent[x] != root:  # path compression
            self.parent[x], x = root, self.parent[x]
        return root

    def union(self, a: str, b: str) -> bool:
        """Join two sets. Returns True if they were previously distinct."""
        ra, rb = self.find(a), self.find(b)
        if ra == rb:
            return False
        self.parent[ra] = rb
        return True


@dataclass
class NodeStats:
    files_scanned: int = 0
    lines: int = 0
    property_lines: int = 0
    edge_lines: int = 0
    node_candidates: int = 0
    merges: int = 0
    nodes_written: int = 0
    names_found: int = 0
    malformed_lines: int = 0
    multi_member_clusters: int = 0
    mixed_category_clusters: int = 0
    ambiguous_identity_edges: int = 0  # many:1 xrefs left unmerged (over-merge guard)
    suspect_clusters: int = 0  # clusters with >1 id from one namespace (should be 0)
    by_category: dict = field(default_factory=lambda: defaultdict(int))
    largest_clusters: list = field(default_factory=list)

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_category"] = dict(self.by_category)
        return d


def _node_key(dataset: str, local_id: str) -> str:
    return dataset + SEP + local_id


def _split_key(key: str) -> tuple[str, str]:
    ds, local = key.split(SEP, 1)
    return ds, local


def build_nodes(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    out_path: str | Path,
    stats_path: str | Path | None = None,
    id_map_path: str | Path | None = None,
    datasets: list[str] | None = None,
    max_lines: int | None = None,
) -> NodeStats:
    """Build KGX nodes.tsv from sorted index files. Returns merge stats."""
    index_dir = Path(index_dir)
    if datasets:
        files: list[str] = []
        for ds in datasets:
            files += glob.glob(str(index_dir / f"{ds}_sorted.*.index.gz"))
        files = sorted(set(files))
    else:
        files = sorted(glob.glob(str(index_dir / "*_sorted.*.index.gz")))

    uf = UnionFind()
    names: dict[str, str] = {}  # node_key -> best-effort name
    stats = NodeStats()
    counter: dict = {}

    def register(dataset: str, local_id: str) -> str:
        # dataset is recoverable from the key (_split_key), so uf.parent is the
        # single source of truth for node candidates — no parallel dict needed.
        key = _node_key(dataset, local_id)
        uf.add(key)
        return key

    # identity edges are collected, not unioned immediately, so we can enforce
    # 1:1 cardinality afterwards (avoids many:1-xref over-merge).
    identity_edges: list[tuple[str, str, str, str]] = []

    stop = False
    for path in files:
        if stop:
            break
        stats.files_scanned += 1
        for raw in iter_index_file(path, counter):
            stats.lines += 1
            if max_lines and stats.lines > max_lines:
                stop = True
                break
            src_ds = registry.name_for_id(raw.source_dataset_id)
            src_is_node = bool(src_ds and categories.is_node_dataset(src_ds))

            if raw.is_property:
                stats.property_lines += 1
                if src_is_node:
                    key = register(src_ds, raw.subject)
                    if key not in names:
                        nm = extract_name(raw.object)
                        if nm:
                            names[key] = nm
                continue

            stats.edge_lines += 1
            obj_ds = registry.name_for_id(raw.object_dataset_id)
            obj_is_node = bool(obj_ds and categories.is_node_dataset(obj_ds))

            src_key = register(src_ds, raw.subject) if src_is_node else None
            obj_key = register(obj_ds, raw.object) if obj_is_node else None

            if (
                src_key
                and obj_key
                and categories.is_identity_pair(src_ds, obj_ds)
                and categories.category_for(src_ds) == categories.category_for(obj_ds)
            ):
                identity_edges.append((src_key, src_ds, obj_key, obj_ds))

    stats.malformed_lines = counter.get("malformed", 0)
    stats.node_candidates = len(uf.parent)

    # Cardinality-aware merge: only union 1:1 identity mappings. A node that maps
    # to >1 node in the other namespace (many:1 xref, e.g. two HGNC genes sharing
    # one Ensembl id) is NOT merged — better unmerged than wrongly collapsed.
    nbr: dict[tuple[str, str], set] = defaultdict(set)
    for a, da, b, db in identity_edges:
        nbr[(a, db)].add(b)
        nbr[(b, da)].add(a)
    for a, da, b, db in identity_edges:
        if len(nbr[(a, db)]) == 1 and len(nbr[(b, da)]) == 1:
            if uf.union(a, b):
                stats.merges += 1
        else:
            stats.ambiguous_identity_edges += 1

    # Group node keys into clusters by union-find root.
    clusters: dict[str, list[str]] = defaultdict(list)
    for key in uf.parent:
        clusters[uf.find(key)].append(key)

    cluster_sizes: list[tuple[int, str]] = []
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    id_map_fh = None
    if id_map_path:
        Path(id_map_path).parent.mkdir(parents=True, exist_ok=True)
        id_map_fh = kgx.xopen(id_map_path, "wt")
        id_map_fh.write("member\tcanonical\n")
    try:
        with kgx.xopen(out_path, "wt") as out:
            out.write("id\tcategory\tname\tequivalent_identifiers\tprovided_by\n")
            for members in clusters.values():
                if len(members) > 1:
                    dsl = [_split_key(k)[0] for k in members]
                    if len(dsl) != len(set(dsl)):  # >1 id from one namespace
                        stats.suspect_clusters += 1
                row = _emit_cluster(members, names, categories, stats, id_map_fh)
                if row is None:
                    continue
                out.write(row)
                stats.nodes_written += 1
                if len(members) > 1:
                    stats.multi_member_clusters += 1
                    cluster_sizes.append((len(members), row.split("\t", 1)[0]))
    finally:
        if id_map_fh:
            id_map_fh.close()

    cluster_sizes.sort(reverse=True)
    stats.largest_clusters = [
        {"size": n, "canonical": cid} for n, cid in cluster_sizes[:25]
    ]

    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats


def _emit_cluster(
    members: list[str],
    names: dict[str, str],
    categories: CategoryMap,
    stats: NodeStats,
    id_map_fh=None,
) -> str | None:
    parsed = [_split_key(k) for k in members]  # [(dataset, local_id), ...]
    cats = {categories.category_for(ds) for ds, _ in parsed}
    cats.discard(None)
    if not cats:
        return None
    if len(cats) > 1:
        stats.mixed_category_clusters += 1
    category = sorted(cats)[0]

    priority = categories.priority_for(category)

    def rank(item: tuple[str, str]) -> tuple[int, str, str]:
        ds, local = item
        try:
            p = priority.index(ds)
        except ValueError:
            p = len(priority)
        return (p, ds, local)

    parsed.sort(key=rank)
    canonical_ds, canonical_local = parsed[0]
    canonical = to_curie(categories.prefix_for(canonical_ds), canonical_local)

    equivalent = []
    for ds, local in parsed:
        equivalent.append(to_curie(categories.prefix_for(ds), local))
    # de-dup, keep canonical first
    seen = set()
    eq_ordered = []
    for c in [canonical] + equivalent:
        if c not in seen:
            seen.add(c)
            eq_ordered.append(c)

    # id_map: every non-canonical member CURIE -> canonical (for edge rewriting).
    if id_map_fh is not None and len(eq_ordered) > 1:
        for c in eq_ordered:
            if c != canonical:
                id_map_fh.write(f"{c}\t{canonical}\n")

    # prefer the canonical member's name, else any member's name
    name = ""
    canonical_key = _node_key(canonical_ds, canonical_local)
    if canonical_key in names:
        name = names[canonical_key]
    else:
        for k in members:
            if k in names:
                name = names[k]
                break
    if name:
        stats.names_found += 1

    stats.by_category[category] += 1
    return (
        f"{canonical}\t{category}\t{tsv_safe(name)}\t"
        f"{'|'.join(eq_ordered)}\t{PROVIDED_BY}\n"
    )
