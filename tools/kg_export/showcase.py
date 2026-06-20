"""Showcase: demonstrate the GIANT layers (dbSNP variants, PubChem/ChEMBL bioactivity)
on a small curated set of famous genes + compounds, since those layers are excluded
wholesale from the subgraph. Proof-by-recognizable-example, not the billions.

A targeted extraction from the index (filtered to mappings/showcase.yaml), merged into
the subgraph at assemble -- independent of the billion-scale full dump:

  1. resolve genes (symbol -> human entrez + canonical HGNC) and their UniProt proteins
  2. write the entrez gene-id list -> `tools/dbsnp_py/extract.py --genes` produces the
     dbSNP variant nodes/edges (one federation scan; run separately)
  3. resolve compounds (name -> ChEMBL / PubChem id, via name/synonyms)
  4. bioactivity: ChEMBL/PubChem activity groups whose compound OR target protein is in
     the showcase set -> `compound -interacts_with-> protein` edges (+ the compound /
     variant nodes those introduce; genes/proteins already live in the subgraph)
"""

from __future__ import annotations

import glob
import json
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

import yaml

from . import kgx
from .categories import CategoryMap
from .curie import to_curie
from .datasets import DatasetRegistry
from .index import iter_index_file

HUMAN_TAXID = "9606"
PRED_BIOACTIVITY = "biolink:interacts_with"


@dataclass
class ShowcaseStats:
    genes_resolved: int = 0
    proteins: int = 0
    compounds_resolved: int = 0
    bioactivity_edges: int = 0
    compound_nodes: int = 0
    by_source: dict = field(default_factory=lambda: defaultdict(int))

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_source"] = dict(self.by_source)
        return d


def load_config(path: str | Path) -> dict:
    return yaml.safe_load(Path(path).read_text()) or {}


def _files(index_dir: Path, stem: str):
    return sorted(glob.glob(str(index_dir / f"{stem}_sorted.*.index.gz")))


def _prop(raw):
    try:
        d = json.loads(raw.object)
    except (ValueError, TypeError):
        return None
    return d if isinstance(d, dict) else None


# --- resolution -----------------------------------------------------------------

def resolve_genes(index_dir: Path, symbols: set[str], id_map: dict):
    """gene SYMBOL -> human entrez ids + canonical (HGNC) node curies."""
    want = {s.upper() for s in symbols}
    entrez_ids: set[str] = set()
    canon: set[str] = set()
    for path in _files(index_dir, "entrez"):
        for raw in iter_index_file(path):
            if not raw.is_property:
                continue
            d = _prop(raw)
            if not d or str(d.get("tax_id")) != HUMAN_TAXID:
                continue
            sym = (d.get("symbol") or "").upper()
            if sym in want:
                entrez_ids.add(raw.subject)
                c = id_map.get(f"NCBIGene:{raw.subject}", f"NCBIGene:{raw.subject}")
                canon.add(c)
    return entrez_ids, canon


def resolve_proteins(index_dir: Path, hgnc_canon: set[str], uniprot_prefix: str):
    """showcase HGNC genes -> their UniProt proteins (via uniprot_from_hgnc)."""
    hgnc = {c for c in hgnc_canon if c.startswith("HGNC:")}
    proteins: set[str] = set()
    for path in _files(index_dir, "uniprot_from_hgnc"):
        for raw in iter_index_file(path):
            if raw.is_property:
                continue
            if raw.object in hgnc:                 # object = HGNC:id
                proteins.add(to_curie(uniprot_prefix, raw.subject))
    return proteins


def resolve_compounds(index_dir: Path, names: set[str], categories: CategoryMap):
    """compound NAME -> ChEMBL / PubChem node curies (via name + synonyms)."""
    want = {n.lower() for n in names}
    out: set[str] = set()
    chembl_pref = categories.prefix_for("chembl_molecule") or "CHEMBL.COMPOUND"
    pubchem_pref = categories.prefix_for("pubchem") or "PUBCHEM.COMPOUND"

    for path in _files(index_dir, "chembl_molecule"):
        for raw in iter_index_file(path):
            if not raw.is_property:
                continue
            d = _prop(raw)
            mol = (d or {}).get("molecule") or {}
            cand = [mol.get("name", "")] + list(mol.get("altNames") or [])
            if any(isinstance(x, str) and x.lower() in want for x in cand):
                out.add(to_curie(chembl_pref, raw.subject))

    for path in _files(index_dir, "pubchem"):
        for raw in iter_index_file(path):
            if not raw.is_property:
                continue
            d = _prop(raw)
            cand = [(d or {}).get("title", "")] + list((d or {}).get("synonyms") or [])
            if any(isinstance(x, str) and x.lower() in want for x in cand):
                out.add(to_curie(pubchem_pref, raw.subject))
    return out


# --- bioactivity ----------------------------------------------------------------

def _blocks(path, counter):
    """Yield (subject, [raw,...]) groups from a subject-sorted activity file."""
    cur, buf = None, []
    for raw in iter_index_file(path, counter):
        if raw.subject != cur:
            if buf:
                yield cur, buf
            cur, buf = raw.subject, []
        buf.append(raw)
    if buf:
        yield cur, buf


def build_bioactivity(index_dir: Path, registry: DatasetRegistry, categories: CategoryMap,
                      compounds: set[str], proteins: set[str], id_map: dict,
                      eout, stats: ShowcaseStats) -> set[str]:
    """ChEMBL/PubChem activity groups whose compound OR target protein is in the
    showcase set -> compound -interacts_with-> protein. Returns the compound curies
    seen (so non-subgraph PubChem compounds can be emitted as nodes)."""
    up = categories.prefix_for("uniprot") or "UniProtKB"
    seen_compounds: set[str] = set()
    for ds, cmp_ds, cmp_prefix in (
        ("chembl_activity", "chembl_molecule", categories.prefix_for("chembl_molecule") or "CHEMBL.COMPOUND"),
        ("pubchem_activity", "pubchem", categories.prefix_for("pubchem") or "PUBCHEM.COMPOUND"),
    ):
        counter: dict = {}
        for path in _files(index_dir, ds):
            for _subj, group in _blocks(path, counter):
                compound = protein = None
                for raw in group:
                    if raw.is_property:
                        continue
                    member_ds = registry.name_for_id(raw.object_dataset_id)
                    if member_ds == cmp_ds:
                        compound = to_curie(cmp_prefix, raw.object)
                    elif member_ds == "uniprot":
                        protein = to_curie(up, raw.object)
                if not (compound and protein):
                    continue
                if compound in compounds or protein in proteins:
                    eout.write(kgx.format_edge(
                        compound, PRED_BIOACTIVITY, protein, f"infores:{ds}",
                        knowledge_level="knowledge_assertion", agent_type="manual_agent"))
                    stats.bioactivity_edges += 1
                    stats.by_source[ds] += 1
                    seen_compounds.add(compound)
    return seen_compounds


def build_showcase(index_dir, registry, categories, config, id_map,
                   out_nodes, out_edges, gene_filter_out, stats_path=None):
    index_dir = Path(index_dir)
    id_map = id_map or {}
    stats = ShowcaseStats()
    symbols = set(config.get("genes") or [])
    names = set(config.get("compounds") or [])

    entrez_ids, hgnc_canon = resolve_genes(index_dir, symbols, id_map)
    stats.genes_resolved = len(hgnc_canon)
    Path(gene_filter_out).write_text("".join(e + "\n" for e in sorted(entrez_ids)))

    proteins = resolve_proteins(index_dir, hgnc_canon, categories.prefix_for("uniprot") or "UniProtKB")
    stats.proteins = len(proteins)
    compounds = resolve_compounds(index_dir, names, categories)
    stats.compounds_resolved = len(compounds)

    out_edges = Path(out_edges)
    out_edges.parent.mkdir(parents=True, exist_ok=True)
    with kgx.xopen(out_edges, "wt") as eout:
        eout.write(kgx.EDGE_HEADER + "\n")
        seen = build_bioactivity(index_dir, registry, categories, compounds, proteins,
                                 id_map, eout, stats)

    # emit nodes for the showcase compounds (PubChem ones aren't in the subgraph).
    pubchem_pref = categories.prefix_for("pubchem") or "PUBCHEM.COMPOUND"
    with kgx.xopen(out_nodes, "wt") as nout:
        nout.write(kgx.NODE_HEADER + "\n")
        for c in sorted(compounds | seen):
            cat = categories.category_for("pubchem" if c.startswith(pubchem_pref + ":") else "chembl_molecule")
            nout.write(f"{c}\t{cat or 'biolink:SmallMolecule'}\t\t{c}\tinfores:biobtree\n")
            stats.compound_nodes += 1

    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats
