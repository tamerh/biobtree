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
from itertools import combinations
from pathlib import Path

from .categories import CategoryMap
from .curie import to_curie
from .datasets import DatasetRegistry
from .index import RawXref, iter_index_file
from .predicates import PredicateMap, ReifiedRule

AGGREGATOR = "infores:biobtree"


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

    with out_path.open("w", encoding="utf-8") as out:
        out.write(
            "subject\tpredicate\tobject\tprimary_knowledge_source\t"
            "aggregator_knowledge_source\n"
        )
        for ds in targets:
            rule = predicates.reified_rule(ds)
            if rule is None:
                continue
            files = sorted(glob.glob(str(index_dir / f"{ds}_sorted.*.index.gz")))
            if not files:
                continue
            stats.datasets_processed += 1
            primary = f"infores:{ds}"
            # heap-merge the dataset's (independently subject-sorted) chunks so a
            # subject split across chunks is still grouped as one entry.
            merged = heapq.merge(
                *(iter_index_file(f, counter) for f in files),
                key=lambda r: r.subject,
            )
            for group in _groups_by_subject(merged):
                stats.groups += 1
                stats.lines += len(group)
                for row in _emit_group(
                    group, rule, registry, categories, canonical,
                    primary, max_edges_per_group, stats,
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
                max_edges_per_group, stats):
    """Yield KGX edge rows for one reified entry group."""
    def partners(role_dataset: str) -> list[str]:
        out = []
        for raw in group:
            if raw.is_property:
                continue
            ds = registry.name_for_id(raw.object_dataset_id)
            if ds == role_dataset:
                c = canonical(ds, raw.object)
                if c:
                    out.append(c)
        # de-dup, stable
        seen = set()
        uniq = []
        for c in out:
            if c not in seen:
                seen.add(c)
                uniq.append(c)
        return uniq

    if rule.kind == "symmetric":
        members = partners(rule.partner)
        n = len(members)
        if n * (n - 1) // 2 > max_edges_per_group:  # prospective undirected pairs
            stats.oversized_groups += 1
            return
        for a, b in combinations(sorted(members), 2):
            if a == b:
                stats.self_loops += 1
                continue
            yield f"{a}\t{rule.predicate}\t{b}\t{primary}\t{AGGREGATOR}\n"
    else:  # bipartite
        subs = partners(rule.subject)
        objs = partners(rule.object)
        if len(subs) * len(objs) > max_edges_per_group:  # prospective product
            stats.oversized_groups += 1
            return
        for a in subs:
            for b in objs:
                if a == b:
                    stats.self_loops += 1
                    continue
                yield f"{a}\t{rule.predicate}\t{b}\t{primary}\t{AGGREGATOR}\n"
