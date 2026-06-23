"""Phase 2d: RefSeq -> KGX nodes + edges (type-dependent, like GO).

A single ``refseq`` BioBTree dataset conflates three biolink categories, keyed
by each accession's property ``type`` field (exactly as GO is split by aspect):

  type                                          node category
  --------------------------------------------  ---------------------------
  mRNA                                          biolink:Transcript
  ncRNA                                         biolink:NoncodingRNAProduct
  protein / predicted_protein /                 biolink:Protein
    protein_nonredundant / protein_organelle

So RefSeq can't be a single ``categories.yaml`` entry. Instead this module
streams the subject-sorted ``refseq_sorted.*.index.gz`` file: every line for one
accession is contiguous, so we buffer a block, read its property line to type
the node, and emit the block's cross-references as typed edges:

  Transcript / ncRNA  --transcribed_from-->  Gene        (entrez, canonicalized)
  Transcript          --translates_to    ->  Protein     (NP_ via refseq, UniProt)
  Gene                --has_gene_product ->  Protein      (NP_/XP_ -> entrez gene)
  any                 --in_taxon         ->  OrganismTaxon

RefSeq protein/transcript nodes are NEW namespaces (prefix ``refseq``); they are
not merged with Ensembl/UniProt (no identity pair), only linked. This mirrors
BioBTree, which keeps RefSeq as its own cross-referenced dataset.
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
PRIMARY = "infores:refseq"
REFSEQ_PREFIX = "refseq"

TYPE_CATEGORY = {
    "mRNA": "biolink:Transcript",
    "ncRNA": "biolink:NoncodingRNAProduct",
    "protein": "biolink:Protein",
    "predicted_protein": "biolink:Protein",
    "protein_nonredundant": "biolink:Protein",
    "protein_organelle": "biolink:Protein",
}
# Fallback typing by accession prefix when `type` is missing/unknown.
_PREFIX_CATEGORY = {
    "NM_": "biolink:Transcript", "XM_": "biolink:Transcript",
    "NR_": "biolink:NoncodingRNAProduct", "XR_": "biolink:NoncodingRNAProduct",
    "NP_": "biolink:Protein", "XP_": "biolink:Protein",
    "WP_": "biolink:Protein", "YP_": "biolink:Protein",
}
_TRANSCRIPT_CATS = {"biolink:Transcript", "biolink:NoncodingRNAProduct"}
_PROTEIN_ACC = ("NP_", "XP_", "WP_", "YP_")


@dataclass
class RefseqStats:
    accessions: int = 0
    nodes_written: int = 0
    edges_written: int = 0
    untyped: int = 0
    by_category: dict = field(default_factory=lambda: defaultdict(int))
    by_predicate: dict = field(default_factory=lambda: defaultdict(int))
    malformed_lines: int = 0

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        for k in ("by_category", "by_predicate"):
            d[k] = dict(getattr(self, k))
        return d


def _category_for(accession: str, type_field: str | None) -> str | None:
    if type_field and type_field in TYPE_CATEGORY:
        return TYPE_CATEGORY[type_field]
    return _PREFIX_CATEGORY.get(accession[:3])


def _node_name(prop_json: str) -> str:
    try:
        d = json.loads(prop_json)
    except (ValueError, TypeError):
        return ""
    if not isinstance(d, dict):
        return ""
    return (d.get("description") or d.get("symbol") or "").strip()


def _node_type(prop_json: str) -> str | None:
    try:
        d = json.loads(prop_json)
    except (ValueError, TypeError):
        return None
    return d.get("type") if isinstance(d, dict) else None


def _blocks(index_dir: Path, counter: dict):
    """Yield (subject, [RawXref,...]) blocks from the subject-sorted file."""
    for path in sorted(glob.glob(str(index_dir / "refseq_sorted.*.index.gz"))):
        cur = None
        buf: list = []
        for raw in iter_index_file(path, counter):
            if raw.subject != cur:
                if buf:
                    yield cur, buf
                cur, buf = raw.subject, []
            buf.append(raw)
        if buf:
            yield cur, buf


def build_refseq(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    nodes_out: str | Path,
    edges_out: str | Path,
    id_map: dict[str, str] | None = None,
    stats_path: str | Path | None = None,
) -> RefseqStats:
    index_dir = Path(index_dir)
    id_map = id_map or {}
    stats = RefseqStats()
    counter: dict = {}

    gene_prefix = categories.prefix_for("entrez") or "NCBIGene"
    taxon_prefix = categories.prefix_for("taxonomy") or "NCBITaxon"
    uniprot_prefix = categories.prefix_for("uniprot") or "UniProtKB"

    def canonical_gene(local_id: str) -> str:
        curie = to_curie(gene_prefix, local_id)
        return id_map.get(curie, curie)

    nodes_out, edges_out = Path(nodes_out), Path(edges_out)
    nodes_out.parent.mkdir(parents=True, exist_ok=True)
    edges_out.parent.mkdir(parents=True, exist_ok=True)

    def emit(out, s, p, o):
        out.write(kgx.format_edge(
            s, p, o, PRIMARY,
            knowledge_level="knowledge_assertion", agent_type="manual_agent",
        ))
        stats.edges_written += 1
        stats.by_predicate[p] += 1

    with kgx.xopen(nodes_out, "wt") as nout, kgx.xopen(edges_out, "wt") as eout:
        nout.write(kgx.NODE_HEADER + "\n")
        eout.write(kgx.EDGE_HEADER + "\n")
        for accession, block in _blocks(index_dir, counter):
            stats.accessions += 1
            prop = next((r.object for r in block if r.is_property), None)
            category = _category_for(accession, _node_type(prop) if prop else None)
            if category is None:
                stats.untyped += 1
                continue
            subj = to_curie(REFSEQ_PREFIX, accession)
            name = _node_name(prop) if prop else ""
            nout.write(f"{subj}\t{category}\t{tsv_safe(name)}\t{subj}\t{AGGREGATOR}\n")
            stats.nodes_written += 1
            stats.by_category[category] += 1

            is_rna = category in _TRANSCRIPT_CATS
            for r in block:
                if r.is_property:
                    continue
                obj_ds = registry.name_for_id(r.object_dataset_id)
                if obj_ds == "entrez":
                    gene = canonical_gene(r.object)
                    if is_rna:
                        emit(eout, subj, "biolink:transcribed_from", gene)
                    else:  # protein -> gene: Gene has_gene_product Protein
                        emit(eout, gene, "biolink:has_gene_product", subj)
                elif obj_ds == "taxonomy":
                    emit(eout, subj, "biolink:in_taxon", to_curie(taxon_prefix, r.object))
                elif obj_ds == "refseq":
                    # transcript -> its protein product (NP_/XP_/...); emit once
                    # from the RNA side so the NP_ block doesn't duplicate it.
                    if is_rna and r.object[:3] in _PROTEIN_ACC:
                        emit(eout, subj, "biolink:translates_to",
                             to_curie(REFSEQ_PREFIX, r.object))
                elif obj_ds == "uniprot":
                    if is_rna:
                        emit(eout, subj, "biolink:translates_to",
                             to_curie(uniprot_prefix, r.object))
                    else:  # RefSeq protein == UniProt protein
                        emit(eout, subj, "biolink:same_as",
                             to_curie(uniprot_prefix, r.object))

    stats.malformed_lines = counter.get("malformed", 0)
    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats
