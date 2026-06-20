"""Phase 2c: GO annotations -> KGX nodes + edges (aspect-dependent).

GO terms are typed and linked differently per aspect (the property `type` field):

  aspect               node category                  annotation predicate
  -------------------  -----------------------------  -------------------------
  molecular_function   biolink:MolecularActivity      biolink:enables
  biological_process   biolink:BiologicalProcess      biolink:actively_involved_in
  cellular_component   biolink:CellularComponent      biolink:located_in

Pass 1 builds the GO id -> (aspect, name) map from go_sorted properties; pass 2
streams annotation source files (gene/protein -> go) and emits typed edges with
canonical subjects.
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
from .nodes import extract_name, tsv_safe

AGGREGATOR = "infores:biobtree"
GO_PREFIX = "GO"

ASPECT_CATEGORY = {
    "molecular_function": "biolink:MolecularActivity",
    "biological_process": "biolink:BiologicalProcess",
    "cellular_component": "biolink:CellularComponent",
}
ASPECT_PREDICATE = {
    "molecular_function": "biolink:enables",
    "biological_process": "biolink:actively_involved_in",
    "cellular_component": "biolink:located_in",
}

# Subject datasets whose ">go" edges we emit. uniprot (protein) + ensembl (gene)
# cover the bulk; entrez>go (~119M, all-species) and transcript/rnacentral are
# deferred to avoid gene-namespace duplication and size.
DEFAULT_ANNOTATION_SOURCES = ("uniprot", "ensembl")


@dataclass
class GoStats:
    terms: int = 0
    terms_by_aspect: dict = field(default_factory=lambda: defaultdict(int))
    nodes_written: int = 0
    edges_written: int = 0
    edges_with_evidence: int = 0
    edges_missing_aspect: int = 0
    malformed_lines: int = 0
    by_predicate: dict = field(default_factory=lambda: defaultdict(int))
    by_source: dict = field(default_factory=lambda: defaultdict(int))

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        for k in ("terms_by_aspect", "by_predicate", "by_source"):
            d[k] = dict(getattr(self, k))
        return d


def build_go_terms(index_dir, registry, counter=None) -> dict[str, tuple[str, str]]:
    """GO id -> (aspect, name) from go_sorted property lines."""
    terms: dict[str, tuple[str, str]] = {}
    go_id = registry.by_name("go")
    if not go_id:
        return terms
    for path in sorted(glob.glob(str(Path(index_dir) / "go_sorted.*.index.gz"))):
        for raw in iter_index_file(path, counter):
            if not raw.is_property:
                continue
            try:
                d = json.loads(raw.object)
            except (ValueError, TypeError):
                continue
            aspect = d.get("type")
            if aspect in ASPECT_CATEGORY:
                name = d.get("name") or extract_name(raw.object) or ""
                terms[raw.subject] = (aspect, name)
    return terms


def build_go(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    nodes_out: str | Path,
    edges_out: str | Path,
    id_map: dict[str, str] | None = None,
    stats_path: str | Path | None = None,
    annotation_sources: tuple[str, ...] = DEFAULT_ANNOTATION_SOURCES,
) -> GoStats:
    index_dir = Path(index_dir)
    id_map = id_map or {}
    stats = GoStats()
    counter: dict = {}

    terms = build_go_terms(index_dir, registry, counter)
    stats.terms = len(terms)
    for aspect, _ in terms.values():
        stats.terms_by_aspect[aspect] += 1

    # --- GO nodes ---------------------------------------------------------
    nodes_out = Path(nodes_out)
    nodes_out.parent.mkdir(parents=True, exist_ok=True)
    with kgx.xopen(nodes_out, "wt") as out:
        out.write("id\tcategory\tname\tequivalent_identifiers\tprovided_by\n")
        for go_id, (aspect, name) in terms.items():
            curie = to_curie(GO_PREFIX, go_id)
            category = ASPECT_CATEGORY[aspect]
            out.write(f"{curie}\t{category}\t{tsv_safe(name)}\t{curie}\t{AGGREGATOR}\n")
            stats.nodes_written += 1

    # --- GO annotation edges ---------------------------------------------
    def canonical(dataset: str, local_id: str) -> str | None:
        prefix = categories.prefix_for(dataset)
        if not prefix:
            return None
        curie = to_curie(prefix, local_id)
        return id_map.get(curie, curie)

    edges_out = Path(edges_out)
    edges_out.parent.mkdir(parents=True, exist_ok=True)
    with kgx.xopen(edges_out, "wt") as out:
        out.write(kgx.EDGE_HEADER + "\n")
        for src in annotation_sources:
            primary = f"infores:{src}"
            for path in sorted(glob.glob(str(index_dir / f"{src}_sorted.*.index.gz"))):
                for raw in iter_index_file(path, counter):
                    if raw.is_property:
                        continue
                    if registry.name_for_id(raw.object_dataset_id) != "go":
                        continue
                    term = terms.get(raw.object)
                    if term is None:
                        stats.edges_missing_aspect += 1
                        continue
                    aspect, _ = term
                    subj = canonical(src, raw.subject)
                    if subj is None:
                        continue
                    predicate = ASPECT_PREDICATE[aspect]
                    obj = to_curie(GO_PREFIX, raw.object)
                    ev = raw.evidence if raw.evidence and raw.evidence.startswith("ECO:") else ""
                    out.write(
                        kgx.format_edge(
                            subj, predicate, obj, primary,
                            knowledge_level="knowledge_assertion",
                            agent_type="manual_agent",
                            has_evidence=ev,
                        )
                    )
                    if ev:
                        stats.edges_with_evidence += 1
                    stats.edges_written += 1
                    stats.by_predicate[predicate] += 1
                    stats.by_source[src] += 1

    stats.malformed_lines = counter.get("malformed", 0)
    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats
