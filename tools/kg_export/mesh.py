"""Optional: MeSH disease subset -> KGX Disease nodes + cross-ontology close_match.

MeSH is heterogeneous (chemicals + diseases + anatomy + ...) and 91% of its
records are supplementary concepts with NO tree-number type field, so MeSH is NOT
a categories.yaml node -- typing all of it would overclaim (the mesh coverage
audit). But the Atlas uses the DISEASE subset (mondo<->mesh, mesh->clinical_trials),
so this builder emits ONLY that subset:

  * MeSH descriptors whose tree_numbers fall in the disease trees (C* = diseases,
    F03* = mental disorders) -> biolink:Disease nodes (MESH:Dxxxxxx).
  * the curated MONDO->MeSH disease cross-references (in mondo_sorted) -> emitted
    as biolink:close_match (the same conservative under-claim used for the other
    cross-ontology disease mappings; we don't merge across namespaces).

Chemical MeSH records (the CTD `ctd` ChemicalEntity nodes, MESH:Cxxxxx) are a
disjoint id space, so the two MESH-prefixed node sets don't collide.
"""

from __future__ import annotations

import glob
import json
from dataclasses import dataclass
from pathlib import Path

from . import kgx
from .categories import CategoryMap
from .curie import to_curie
from .datasets import DatasetRegistry
from .index import iter_index_file
from .nodes import tsv_safe

AGGREGATOR = "infores:biobtree"
PRIMARY = "infores:mesh"
PREFIX = "MESH"
CATEGORY = "biolink:Disease"


@dataclass
class MeshStats:
    descriptors: int = 0
    disease_nodes: int = 0
    close_match_edges: int = 0
    malformed_lines: int = 0

    def to_json(self) -> dict:
        return self.__dict__.copy()


def _is_disease_tree(trees) -> bool:
    """True if any MeSH tree number is in the disease trees (C* or F03*)."""
    if not isinstance(trees, list):
        return False
    return any(isinstance(t, str) and (t.startswith("C") or t.startswith("F03"))
               for t in trees)


def build_mesh(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    nodes_out: str | Path,
    edges_out: str | Path,
    stats_path: str | Path | None = None,
) -> MeshStats:
    index_dir = Path(index_dir)
    stats = MeshStats()
    counter: dict = {}

    # pass 1: collect disease-tree MeSH descriptors (id -> name) + emit nodes
    disease: dict[str, str] = {}
    for path in sorted(glob.glob(str(index_dir / "mesh_sorted.*.index.gz"))):
        for raw in iter_index_file(path, counter):
            if not raw.is_property:
                continue
            stats.descriptors += 1
            try:
                d = json.loads(raw.object)
            except (ValueError, TypeError):
                continue
            if isinstance(d, dict) and _is_disease_tree(d.get("tree_numbers")):
                disease[raw.subject] = (d.get("descriptor_name") or "").strip()

    nodes_out, edges_out = Path(nodes_out), Path(edges_out)
    nodes_out.parent.mkdir(parents=True, exist_ok=True)
    edges_out.parent.mkdir(parents=True, exist_ok=True)
    with kgx.xopen(nodes_out, "wt") as nout:
        nout.write(kgx.NODE_HEADER + "\n")
        for mid, name in disease.items():
            curie = to_curie(PREFIX, mid)
            nout.write(f"{curie}\t{CATEGORY}\t{tsv_safe(name)}\t{curie}\t{AGGREGATOR}\n")
            stats.disease_nodes += 1

    # pass 2: MONDO->MeSH disease cross-references -> close_match
    mondo_prefix = categories.prefix_for("mondo") or "MONDO"
    with kgx.xopen(edges_out, "wt") as eout:
        eout.write(kgx.EDGE_HEADER + "\n")
        for path in sorted(glob.glob(str(index_dir / "mondo_sorted.*.index.gz"))):
            for raw in iter_index_file(path, counter):
                if raw.is_property:
                    continue
                if registry.name_for_id(raw.object_dataset_id) != "mesh":
                    continue
                if raw.object not in disease:
                    continue
                eout.write(kgx.format_edge(
                    to_curie(mondo_prefix, raw.subject),
                    "biolink:close_match",
                    to_curie(PREFIX, raw.object),
                    PRIMARY,
                    knowledge_level="knowledge_assertion", agent_type="manual_agent",
                ))
                stats.close_match_edges += 1

    stats.malformed_lines = counter.get("malformed", 0)
    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats
