"""Phase 2: map BioBTree xrefs to biolink edges -> KGX edges.tsv.

Reads forward ``<ds>_sorted.*.index.gz`` files only (reverse `*_from_*` files are
the mirror image and are skipped). Each xref is mapped via predicates.yaml to a
biolink predicate, endpoints are rewritten to canonical node CURIEs (using the
Phase 1 id_map), bidirectional storage is collapsed by authoring one canonical
direction per pair, and unmapped/skip pairs are counted (no `related_to`
catch-all).
"""

from __future__ import annotations

import glob
import json
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

from .categories import CategoryMap
from .curie import to_curie
from .datasets import DatasetRegistry
from .index import iter_index_file
from .predicates import PredicateMap

AGGREGATOR = "infores:biobtree"


@dataclass
class EdgeStats:
    files_scanned: int = 0
    lines: int = 0
    property_lines: int = 0
    edges_written: int = 0
    dropped_not_node: int = 0  # an endpoint dataset isn't a typed node
    skipped: int = 0  # recognized pair, intentionally not emitted
    unmapped: int = 0  # pair has no predicate rule
    by_predicate: dict = field(default_factory=lambda: defaultdict(int))
    unmapped_pairs: dict = field(default_factory=lambda: defaultdict(int))

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_predicate"] = dict(self.by_predicate)
        # only the top unmapped pairs, to keep the report readable
        top = sorted(self.unmapped_pairs.items(), key=lambda kv: -kv[1])[:40]
        d["unmapped_pairs"] = dict(top)
        return d


def load_id_map(path: str | Path | None) -> dict[str, str]:
    """member CURIE -> canonical CURIE (from Phase 1). Empty if no path."""
    if not path:
        return {}
    id_map: dict[str, str] = {}
    p = Path(path)
    if not p.exists():
        return id_map
    with p.open(encoding="utf-8") as fh:
        header = next(fh, "")  # skip header
        for line in fh:
            member, _, canonical = line.rstrip("\n").partition("\t")
            if member and canonical:
                id_map[member] = canonical
    return id_map


def build_edges(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    predicates: PredicateMap,
    out_path: str | Path,
    id_map: dict[str, str] | None = None,
    stats_path: str | Path | None = None,
    datasets: list[str] | None = None,
    max_lines: int | None = None,
) -> EdgeStats:
    index_dir = Path(index_dir)
    id_map = id_map or {}
    if datasets:
        files: list[str] = []
        for ds in datasets:
            files += glob.glob(str(index_dir / f"{ds}_sorted.*.index.gz"))
        files = sorted(set(files))
    else:
        # forward files only: exclude reverse "*_from_*" mirrors
        files = sorted(
            f
            for f in glob.glob(str(index_dir / "*_sorted.*.index.gz"))
            if "_from_" not in Path(f).name
        )

    stats = EdgeStats()
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)

    def canonical(dataset: str, local_id: str) -> str:
        curie = to_curie(categories.prefix_for(dataset), local_id)
        return id_map.get(curie, curie)

    stop = False
    with out_path.open("w", encoding="utf-8") as out:
        out.write(
            "subject\tpredicate\tobject\tprimary_knowledge_source\t"
            "aggregator_knowledge_source\n"
        )
        for path in files:
            if stop:
                break
            stats.files_scanned += 1
            for raw in iter_index_file(path):
                stats.lines += 1
                if max_lines and stats.lines > max_lines:
                    stop = True
                    break
                if raw.is_property:
                    stats.property_lines += 1
                    continue
                src_ds = registry.name_for_id(raw.source_dataset_id)
                obj_ds = registry.name_for_id(raw.object_dataset_id)
                if not (src_ds and categories.is_node_dataset(src_ds)
                        and obj_ds and categories.is_node_dataset(obj_ds)):
                    stats.dropped_not_node += 1
                    continue
                rule = predicates.rule_for(src_ds, obj_ds)
                if rule is None:
                    stats.unmapped += 1
                    stats.unmapped_pairs[predicates.key(src_ds, obj_ds)] += 1
                    continue
                if rule.is_skip:
                    stats.skipped += 1
                    continue

                if rule.flip:
                    subj = canonical(obj_ds, raw.object)
                    obj = canonical(src_ds, raw.subject)
                else:
                    subj = canonical(src_ds, raw.subject)
                    obj = canonical(obj_ds, raw.object)

                out.write(
                    f"{subj}\t{rule.predicate}\t{obj}\t"
                    f"infores:{src_ds}\t{AGGREGATOR}\n"
                )
                stats.edges_written += 1
                stats.by_predicate[rule.predicate] += 1

    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats
