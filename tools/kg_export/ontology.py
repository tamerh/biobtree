"""Phase 2e: ontology hierarchy + cross-ontology mappings -> KGX edges.

BioBTree stores every ontology's structure INLINE in the base ontology's
forward file (``<ont>_sorted.*.index.gz``). The object-dataset numeric id on a
line tells you what kind of link it is:

  object dataset == ``<ont>parent``   -> subject is_a object   (subClassOf)
  object dataset == ``<ont>child``    -> reverse of the above  (skipped: dup)
  object dataset == another ONTOLOGY  -> a cross-reference / mapping

So no separate parent/child files exist; they are virtual dataset tags. This
module streams each node ontology's forward file and emits:

  <term>  --biolink:subclass_of-->  <parent term>      (from the *parent tag)
  <term>  --biolink:close_match -->  <other-ont term>   (cross-ontology, same
                                                          biolink category only)

We emit ``close_match`` (not ``exact_match``/``same_as``) because BioBTree does
not retain the source skos predicate, and because the export deliberately does
NOT merge diseases/phenotypes across namespaces — close_match asserts strong
similarity without claiming logical identity. Targets are restricted to
datasets we actually type as nodes of the SAME category, so no dangling/foreign
links are produced.
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

PRIMARY = "infores:biobtree"
GO_PREFIX = "GO"
SUBCLASS = "biolink:subclass_of"
CLOSE_MATCH = "biolink:close_match"

# Categories whose `<ont>parent` tag is a genuine is_a / subClassOf relation.
# Deliberately EXCLUDES Pathway (reactome: sub-pathway is part_of, not is_a) and
# SmallMolecule (chembl_molecule: salt->parent is a chemical, not is_a relation)
# even though those datasets also expose a parent/child hierarchy. GO is handled
# separately (runtime-typed across 3 aspects).
SUBCLASS_CATEGORIES = {
    "biolink:Disease", "biolink:PhenotypicFeature",
    "biolink:GrossAnatomicalStructure", "biolink:Cell", "biolink:ProteinFamily",
}


@dataclass
class OntologyStats:
    ontologies: int = 0
    subclass_edges: int = 0
    close_match_edges: int = 0
    self_loops: int = 0
    by_ontology: dict = field(default_factory=lambda: defaultdict(int))
    by_predicate: dict = field(default_factory=lambda: defaultdict(int))
    malformed_lines: int = 0

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        for k in ("by_ontology", "by_predicate"):
            d[k] = dict(getattr(self, k))
        return d


def _ontology_targets(registry: DatasetRegistry, categories: CategoryMap):
    """Build (ontology, prefix, category, parent_id) for every node ontology
    that has a ``<name>parent`` dataset, plus GO (runtime-typed)."""
    out = []
    names = set(categories.datasets())
    names.add("go")  # runtime-typed, but its hierarchy lives in go_sorted
    for ds in sorted(names):
        parent = registry.by_name(ds + "parent")
        if not parent:
            continue
        if ds == "go":
            prefix, category = GO_PREFIX, None  # 3 aspects -> no close_match
        else:
            prefix = categories.prefix_for(ds)
            category = categories.category_for(ds)
            if not prefix or category not in SUBCLASS_CATEGORIES:
                continue  # not a true is_a ontology (e.g. reactome, chembl)
        out.append((ds, prefix, category, parent.numeric_id))
    return out


def build_ontology(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    edges_out: str | Path,
    stats_path: str | Path | None = None,
) -> OntologyStats:
    index_dir = Path(index_dir)
    stats = OntologyStats()
    counter: dict = {}
    targets = _ontology_targets(registry, categories)

    edges_out = Path(edges_out)
    edges_out.parent.mkdir(parents=True, exist_ok=True)

    def emit(out, s, p, o):
        if s == o:
            stats.self_loops += 1
            return
        out.write(kgx.format_edge(
            s, p, o, PRIMARY,
            knowledge_level="knowledge_assertion", agent_type="manual_agent",
        ))
        stats.by_predicate[p] += 1

    with kgx.xopen(edges_out, "wt") as out:
        out.write(kgx.EDGE_HEADER + "\n")
        for ds, prefix, category, parent_id in targets:
            files = sorted(glob.glob(str(index_dir / f"{ds}_sorted.*.index.gz")))
            if not files:
                continue
            stats.ontologies += 1
            for path in files:
                for raw in iter_index_file(path, counter):
                    if raw.is_property:
                        continue
                    subj = to_curie(prefix, raw.subject)
                    obj_ds_id = raw.object_dataset_id
                    if obj_ds_id == parent_id:
                        emit(out, subj, SUBCLASS, to_curie(prefix, raw.object))
                        stats.subclass_edges += 1
                        stats.by_ontology[ds] += 1
                        continue
                    # cross-ontology mapping: only to a same-category node dataset
                    if category is None:
                        continue
                    obj_ds = registry.name_for_id(obj_ds_id)
                    if not obj_ds or obj_ds == ds:
                        continue
                    if categories.category_for(obj_ds) != category:
                        continue
                    obj = to_curie(categories.prefix_for(obj_ds), raw.object)
                    emit(out, subj, CLOSE_MATCH, obj)
                    stats.close_match_edges += 1
                    stats.by_ontology[ds] += 1

    stats.malformed_lines = counter.get("malformed", 0)
    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats
