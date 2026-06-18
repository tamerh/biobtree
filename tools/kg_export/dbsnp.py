"""Optional: dbSNP federation -> KGX variant nodes + edges (streaming).

dbSNP is a SEPARATE BioBTree federation with its own index dir
(``<out>/dbsnp/index/dbsnp_sorted.*``, ~110 GB) — NOT under ``main/index``. It
is huge (~1B variants), so it is an OPT-IN step: a full run with dbSNP yields
true total variant stats; the default/representative export leaves it out and
lets the annotated sources (ClinVar/PharmGKB/GWAS) contribute the meaningful
variants instead.

Streaming, single pass (the ``refseq.py`` pattern): each variant's lines are
contiguous (subject-sorted), so we buffer a block, read its property for the
name, and emit:

  variant  --biolink:is_sequence_variant_of-->  gene   (entrez, id_map-canonical)

Variants are NOT gene-normalized (no identity merges), so streaming keeps memory
flat regardless of scale. Rich attributes (gnomAD frequency, variant_type,
consequence) are present in the property JSON and can be attached later; v1 emits
the node + the gene link. variant->disease is left to ClinVar (already in the KG).
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
from .nodes import tsv_safe

AGGREGATOR = "infores:biobtree"
PRIMARY = "infores:dbsnp"
CATEGORY = "biolink:SequenceVariant"


@dataclass
class DbsnpStats:
    variants: int = 0
    nodes_written: int = 0
    edges_written: int = 0
    by_predicate: dict = field(default_factory=lambda: defaultdict(int))
    malformed_lines: int = 0

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_predicate"] = dict(self.by_predicate)
        return d


def _name(prop_json: str) -> str:
    try:
        d = json.loads(prop_json)
    except (ValueError, TypeError):
        return ""
    return (d.get("rs_id") or "").strip() if isinstance(d, dict) else ""


def _blocks(index_dir: Path, counter: dict, max_variants: int | None):
    for path in sorted(glob.glob(str(index_dir / "dbsnp_sorted.*.index.gz"))):
        cur, buf, seen = None, [], 0
        for raw in iter_index_file(path, counter):
            if raw.subject != cur:
                if buf:
                    yield cur, buf
                    seen += 1
                    if max_variants and seen >= max_variants:
                        return
                cur, buf = raw.subject, []
            buf.append(raw)
        if buf and not (max_variants and seen >= max_variants):
            yield cur, buf


def build_dbsnp(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    nodes_out: str | Path,
    edges_out: str | Path,
    id_map: dict[str, str] | None = None,
    stats_path: str | Path | None = None,
    max_variants: int | None = None,
) -> DbsnpStats:
    index_dir = Path(index_dir)
    id_map = id_map or {}
    stats = DbsnpStats()
    counter: dict = {}
    var_prefix = categories.prefix_for("dbsnp") or "DBSNP"
    gene_prefix = categories.prefix_for("entrez") or "NCBIGene"

    def canonical_gene(local_id: str) -> str:
        curie = to_curie(gene_prefix, local_id)
        return id_map.get(curie, curie)

    nodes_out, edges_out = Path(nodes_out), Path(edges_out)
    nodes_out.parent.mkdir(parents=True, exist_ok=True)
    edges_out.parent.mkdir(parents=True, exist_ok=True)

    with kgx.xopen(nodes_out, "wt") as nout, kgx.xopen(edges_out, "wt") as eout:
        nout.write(kgx.NODE_HEADER + "\n")
        eout.write(kgx.EDGE_HEADER + "\n")
        for accession, block in _blocks(index_dir, counter, max_variants):
            stats.variants += 1
            subj = to_curie(var_prefix, accession)
            prop = next((r.object for r in block if r.is_property), None)
            name = _name(prop) if prop else ""
            nout.write(f"{subj}\t{CATEGORY}\t{tsv_safe(name)}\t{subj}\t{AGGREGATOR}\n")
            stats.nodes_written += 1
            seen = set()
            for r in block:
                if r.is_property:
                    continue
                if registry.name_for_id(r.object_dataset_id) != "entrez":
                    continue  # variant -> gene only (transcript consequence: later)
                gene = canonical_gene(r.object)
                if gene in seen:
                    continue
                seen.add(gene)
                eout.write(kgx.format_edge(
                    subj, "biolink:is_sequence_variant_of", gene, PRIMARY,
                    knowledge_level="knowledge_assertion", agent_type="manual_agent",
                ))
                stats.edges_written += 1
                stats.by_predicate["biolink:is_sequence_variant_of"] += 1

    stats.malformed_lines = counter.get("malformed", 0)
    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats
