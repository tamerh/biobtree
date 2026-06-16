"""Phase 2b: reified edges -> KGX edges.

Some BioBTree relationships are stored through an intermediate entry (a PPI, a
similarity hit, a bioactivity measurement) that links the real entities. Each
such dataset's forward file is sorted by the entry id, so we group consecutive
lines by entry and join the partners in one streaming pass:

  * symmetric (PPI / similarity): all partners share one dataset -> emit
    undirected pairs.
  * bipartite (bioactivity / dependency / expression): entries link a `subject`
    partner and an `object` partner -> emit subject --predicate--> object.

Endpoints are rewritten to canonical CURIEs via the Phase 1 id_map. Numeric
score/affinity qualifiers are a follow-up.
"""

from __future__ import annotations

import glob
import heapq
import json
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

from . import kgx
from .categories import CategoryMap
from .curie import to_curie
from .datasets import DatasetRegistry
from .index import RawXref, iter_index_file
from .predicates import PredicateMap, ReifiedRule

# Similarity/homology datasets are computational predictions, not assertions.
_PREDICTION_DATASETS = {"diamond_similarity", "esm2_similarity"}


@dataclass
class ReifiedStats:
    datasets_processed: int = 0
    lines: int = 0
    groups: int = 0
    edges_written: int = 0
    self_loops: int = 0
    oversized_groups: int = 0
    malformed_lines: int = 0
    by_predicate: dict = field(default_factory=lambda: defaultdict(int))
    by_dataset: dict = field(default_factory=lambda: defaultdict(int))

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_predicate"] = dict(self.by_predicate)
        d["by_dataset"] = dict(self.by_dataset)
        return d


def build_symbol_map(index_dir, registry, dataset="hgnc"):
    """symbol -> id for a gene dataset, from its property-line symbols/names."""
    smap = {}
    for f in sorted(glob.glob(str(Path(index_dir) / f"{dataset}_sorted.*.index.gz"))):
        for raw in iter_index_file(f):
            if not raw.is_property:
                continue
            try:
                d = json.loads(raw.object)
            except (ValueError, TypeError):
                continue
            syms = list(d.get("symbols", [])) + list(d.get("names", []))
            for s in syms:
                if s and s not in smap:
                    smap[s] = raw.subject  # subject is the dataset id (e.g. HGNC:...)
    return smap


def _groups_by_subject(rows):
    """Yield lists of RawXref sharing the same subject from a subject-sorted stream."""
    current_subject = None
    bucket: list[RawXref] = []
    for raw in rows:
        if raw.subject != current_subject:
            if bucket:
                yield bucket
            bucket = []
            current_subject = raw.subject
        bucket.append(raw)
    if bucket:
        yield bucket


def build_reified_edges(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    predicates: PredicateMap,
    out_path: str | Path,
    id_map: dict[str, str] | None = None,
    stats_path: str | Path | None = None,
    datasets: list[str] | None = None,
    max_edges_per_group: int = 5000,
) -> ReifiedStats:
    index_dir = Path(index_dir)
    id_map = id_map or {}
    targets = datasets or predicates.reified_datasets()

    stats = ReifiedStats()
    counter: dict = {}
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)

    def canonical(dataset: str, local_id: str) -> str | None:
        prefix = categories.prefix_for(dataset)
        if not prefix:
            return None
        curie = to_curie(prefix, local_id)
        return id_map.get(curie, curie)

    with kgx.xopen(out_path, "wt") as out:
        out.write(kgx.EDGE_HEADER + "\n")
        for ds in targets:
            rule = predicates.reified_rule(ds)
            if rule is None:
                continue
            files = sorted(glob.glob(str(index_dir / f"{ds}_sorted.*.index.gz")))
            if not files:
                continue
            stats.datasets_processed += 1
            primary = f"infores:{ds}"
            # `via`: resolve an in-entry intermediate id (e.g. a GtoPdb target)
            # to the real node (uniprot) using that dataset's forward index.
            symbol_map = None
            if rule.resolve == "symbol":
                # pairwise resolves both fields via `partner`; bipartite resolves
                # the (symbol-encoded) `object` partner.
                sym_ds = rule.partner if rule.kind == "pairwise" else rule.object
                symbol_map = build_symbol_map(index_dir, registry, sym_ds)
            resolve_map = None
            if rule.kind == "bipartite" and rule.via:
                resolve_map = defaultdict(list)
                for vf in sorted(glob.glob(str(index_dir / f"{rule.via}_sorted.*.index.gz"))):
                    for raw in iter_index_file(vf, counter):
                        if raw.is_property:
                            continue
                        if registry.name_for_id(raw.object_dataset_id) == rule.object:
                            c = canonical(rule.object, raw.object)
                            if c:
                                resolve_map[raw.subject].append(c)
            # heap-merge the dataset's (independently subject-sorted) chunks so a
            # subject split across chunks is still grouped as one entry.
            merged = heapq.merge(
                *(iter_index_file(f, counter) for f in files),
                key=lambda r: r.subject,
            )
            if ds in _PREDICTION_DATASETS:
                kl, at = "prediction", "automated_agent"
            else:
                kl, at = "knowledge_assertion", "manual_agent"
            for group in _groups_by_subject(merged):
                stats.groups += 1
                stats.lines += len(group)
                for row in _emit_group(
                    group, rule, registry, categories, canonical,
                    primary, max_edges_per_group, stats, kl, at,
                    resolve_map, symbol_map,
                ):
                    out.write(row)
                    stats.edges_written += 1
                    stats.by_predicate[rule.predicate] += 1
                    stats.by_dataset[ds] += 1

    stats.malformed_lines = counter.get("malformed", 0)
    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats


def _emit_group(group, rule, registry, categories, canonical, primary,
                max_edges_per_group, stats, knowledge_level, agent_type,
                resolve_map=None, symbol_map=None):
    """Yield KGX edge rows for one reified entry group (pairwise/star/bipartite)."""
    def edge(a, b):
        if not a or not b:
            return None
        if a == b:
            stats.self_loops += 1
            return None
        return kgx.format_edge(
            a, rule.predicate, b, primary,
            knowledge_level=knowledge_level, agent_type=agent_type,
        )

    def partners(role_dataset, exclude=None, symbols=None):
        seen, uniq = set(), []
        for raw in group:
            if raw.is_property:
                continue
            if registry.name_for_id(raw.object_dataset_id) != role_dataset:
                continue
            if exclude is not None and raw.object == exclude:
                continue
            val = raw.object
            if symbols is not None:  # object is a gene symbol -> resolve to id
                val = symbols.get(raw.object)
                if not val:
                    continue
            c = canonical(role_dataset, val)
            if c and c not in seen:
                seen.add(c)
                uniq.append(c)
        return uniq

    if rule.kind == "pairwise":
        # the real binary pairs are named in each property line's JSON; the
        # partner SET is NOT pairwise-complete, so never clique it.
        for raw in group:
            if not raw.is_property:
                continue
            try:
                d = json.loads(raw.object)
            except (ValueError, TypeError):
                continue
            if rule.require and any(str(d.get(k)) != str(v)
                                    for k, v in rule.require.items()):
                continue  # e.g. keep only protein-protein (database_a==UNIPROT)
            a_raw, b_raw = d.get(rule.subject_field), d.get(rule.object_field)
            if not a_raw or not b_raw:
                continue
            if symbol_map is not None:  # field values are gene symbols
                a_id, b_id = symbol_map.get(str(a_raw)), symbol_map.get(str(b_raw))
                if not a_id or not b_id:
                    continue  # unresolved symbol -> skip
                a, b = canonical(rule.partner, a_id), canonical(rule.partner, b_id)
            else:
                a, b = canonical(rule.partner, str(a_raw)), canonical(rule.partner, str(b_raw))
            row = edge(a, b)
            if row:
                yield row

    elif rule.kind == "star":
        # group key is the query entity; emit it -> each hit (never hit<->hit)
        subj_raw = group[0].subject
        subj = canonical(rule.partner, subj_raw)
        if not subj:
            return
        objs = partners(rule.partner, exclude=subj_raw)
        if len(objs) > max_edges_per_group:
            stats.oversized_groups += 1
            return
        for o in objs:
            row = edge(subj, o)
            if row:
                yield row

    else:  # bipartite
        subs = partners(rule.subject)
        if rule.via and resolve_map is not None:
            # object partners are the resolved nodes of the in-entry `via` ids
            seen, objs = set(), []
            for raw in group:
                if raw.is_property:
                    continue
                if registry.name_for_id(raw.object_dataset_id) != rule.via:
                    continue
                for c in resolve_map.get(raw.object, []):
                    if c not in seen:
                        seen.add(c)
                        objs.append(c)
        elif rule.resolve == "symbol":  # object partner is symbol-encoded
            objs = partners(rule.object, symbols=symbol_map)
        else:
            objs = partners(rule.object)
        if len(subs) * len(objs) > max_edges_per_group:
            stats.oversized_groups += 1
            return
        for a in subs:
            for b in objs:
                row = edge(a, b)
                if row:
                    yield row
