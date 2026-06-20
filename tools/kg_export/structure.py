"""Sub-gene / protein structure layer (#1b).

BioBTree stores the granular structural sub-entities of a transcript / protein as
their own datasets, typed as nodes via categories.yaml and emitted by the standard
`nodes` builder. Their `has_part` containment splits across two builders by which
direction the forward index carries the link:

  transcript --has_part--> exon / cds   ALREADY emitted by the `edges` builder:
      the forward `transcript_sorted` carries transcript->exon/->cds, mapped by the
      `transcript>exon` / `transcript>cds` rules in predicates.yaml.

This module emits the rest -- the links whose forward direction the `edges` builder
can't reach (protein->feature lives in `uniprot_sorted` as `uniprot>ufeature`, which
has no forward rule) or that need data only on the property line (ECO evidence):

  protein --has_part--> protein-feature   parent accession recovered from the feature
      id; the feature's inline `evidences` ECO codes attach as `has_evidence` -- the
      first place ECO codes land on a KG edge (~5M of ~5.8M features carry one).
  cds     --translates_to--> protein      the Ensembl-translation -> UniProt coding
      link (cds is typed CodingSequence, a distinct node from the transcript, so it
      needs its own edge to the protein).

Feature `type`/`description`/`location` and exon genomic coordinates are written to
a node-attribute table (merged at assemble like attributes.py).

Emitted always-on (the full-picture decision) -- the seed-driven published subgraph
filters it down to the structure of seed genes/proteins.
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

PRIMARY_ENSEMBL = "infores:ensembl"
PRIMARY_UNIPROT = "infores:uniprot"


@dataclass
class StructureStats:
    cds_translates_to: int = 0
    feature_haspart: int = 0
    feature_with_evidence: int = 0
    attr_rows: int = 0
    malformed_lines: int = 0
    by_predicate: dict = field(default_factory=lambda: defaultdict(int))

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_predicate"] = dict(self.by_predicate)
        return d


# UniProt feature type -> Sequence Ontology CURIE. Applies biolink's recommendation
# (biolink-model#1210: a general category + a `type` property holding a specific
# ontology term, rather than a class per feature). The node keeps `category:
# biolink:ProteinDomain` (biolink's only sub-protein class) as the general bucket and
# the raw human-readable `feature_type`; `type` is added only for feature types whose
# SO id is unambiguous (so we never ship a guessed ontology id). Extend as needed.
_FEATURE_SO = {
    "domain": "SO:0000417",                         # polypeptide_domain
    "signal peptide": "SO:0000418",                 # signal_peptide
    "transit peptide": "SO:0000725",                # transit_peptide
    "propeptide": "SO:0001062",                     # propeptide
    "chain": "SO:0000419",                          # mature_protein_region
    "binding site": "SO:0000409",                   # binding_site
    "active site": "SO:0001104",                    # catalytic_residue
    "transmembrane region": "SO:0001077",           # transmembrane_polypeptide_region
    "disulfide bond": "SO:0001088",                 # disulfide_bond
    "coiled-coil region": "SO:0001080",             # coiled_coil
    "repeat": "SO:0001068",                         # polypeptide_repeat
    "short sequence motif": "SO:0001067",           # polypeptide_motif
    "region of interest": "SO:0000839",             # polypeptide_region
    "compositionally biased region": "SO:0001066",  # compositionally_biased_region_of_peptide
}
_SO_EXON = "SO:0000147"  # exon


def _files(index_dir: Path, stem: str) -> list[str]:
    return sorted(glob.glob(str(index_dir / f"{stem}_sorted.*.index.gz")))


def _feature_parent(feature_id: str) -> str | None:
    """A UniProt feature id is `<accession>_F<n>`; recover the accession."""
    acc, sep, suffix = feature_id.rpartition("_F")
    if not sep or not suffix.isdigit() or not acc or "_" in acc:
        return None
    return acc


def _eco_codes(evidences) -> list[str]:
    if not isinstance(evidences, list):
        return []
    out, seen = [], set()
    for ev in evidences:
        t = ev.get("type") if isinstance(ev, dict) else None
        if isinstance(t, str) and t.startswith("ECO:") and t not in seen:
            seen.add(t)
            out.append(t)
    return out


def build_structure(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    edges_out: str | Path,
    attrs_out: str | Path,
    id_map: dict[str, str] | None = None,
    stats_path: str | Path | None = None,
) -> StructureStats:
    index_dir = Path(index_dir)
    id_map = id_map or {}
    stats = StructureStats()
    counter: dict = {}

    ens = categories.prefix_for("transcript") or "ENSEMBL"   # ENSEMBL (exon/cds/transcript)
    up = categories.prefix_for("uniprot") or "UniProtKB"
    feat_prefix = categories.prefix_for("ufeature") or "uniprot.feature"

    def canon(curie: str) -> str:
        return id_map.get(curie, curie)

    edges_out, attrs_out = Path(edges_out), Path(attrs_out)
    edges_out.parent.mkdir(parents=True, exist_ok=True)

    with kgx.xopen(edges_out, "wt") as eout, kgx.xopen(attrs_out, "wt") as aout:
        eout.write(kgx.EDGE_HEADER + "\n")

        def emit(subj, pred, obj, primary, has_evidence=""):
            eout.write(kgx.format_edge(
                subj, pred, obj, primary,
                knowledge_level="knowledge_assertion", agent_type="manual_agent",
                has_evidence=has_evidence,
            ))
            stats.by_predicate[pred] += 1

        # 1. cds --translates_to--> protein  (cds_sorted non-property: cds -> uniprot)
        for path in _files(index_dir, "cds"):
            for raw in iter_index_file(path, counter):
                if raw.is_property:
                    continue
                if registry.name_for_id(raw.object_dataset_id) != "uniprot":
                    continue
                emit(to_curie(ens, raw.subject), "biolink:translates_to",
                     to_curie(up, raw.object), PRIMARY_ENSEMBL)
                stats.cds_translates_to += 1

        # 2. protein --has_part--> feature (+ ECO evidence + node attrs).
        #    Parent accession recovered from the feature id; one property line each.
        for path in _files(index_dir, "ufeature"):
            for raw in iter_index_file(path, counter):
                if not raw.is_property:
                    continue
                acc = _feature_parent(raw.subject)
                if acc is None:
                    continue
                try:
                    d = json.loads(raw.object)
                except (ValueError, TypeError):
                    continue
                if not isinstance(d, dict):
                    continue
                feat = to_curie(feat_prefix, raw.subject)
                eco = _eco_codes(d.get("evidences"))
                emit(canon(to_curie(up, acc)), "biolink:has_part", feat,
                     PRIMARY_UNIPROT, has_evidence="|".join(eco))
                stats.feature_haspart += 1
                if eco:
                    stats.feature_with_evidence += 1
                loc = d.get("location") or {}
                attrs = {}
                if d.get("type"):
                    attrs["feature_type"] = d["type"]          # raw UniProt type (always)
                    so = _FEATURE_SO.get(d["type"])
                    if so:
                        attrs["type"] = so                     # specific SO term (biolink #1210)
                if d.get("description"):
                    attrs["description"] = str(d["description"])[:200]
                if isinstance(loc, dict):
                    if loc.get("begin") not in (None, ""):
                        attrs["begin"] = loc["begin"]
                    if loc.get("end") not in (None, ""):
                        attrs["end"] = loc["end"]
                if attrs:
                    aout.write(f"{feat}\t{json.dumps(attrs)}\n")
                    stats.attr_rows += 1

        # 3. exon node attributes (genomic coordinates; one property line each).
        for path in _files(index_dir, "exon"):
            for raw in iter_index_file(path, counter):
                if not raw.is_property:
                    continue
                try:
                    d = json.loads(raw.object)
                except (ValueError, TypeError):
                    continue
                if not isinstance(d, dict):
                    continue
                attrs = {k: d[k] for k in ("start", "end", "strand", "seq_region")
                         if d.get(k) not in (None, "")}
                if attrs:
                    attrs["type"] = _SO_EXON  # all exons -> SO:exon (biolink #1210)
                    aout.write(f"{to_curie(ens, raw.subject)}\t{json.dumps(attrs)}\n")
                    stats.attr_rows += 1

    stats.malformed_lines = counter.get("malformed", 0)
    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats
