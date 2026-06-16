"""Phase 3: assemble KGX outputs — merge, serialize (JSONL), validate, manifest.

The node/edge builders (nodes, edges, reified, go) each write a partial KGX TSV.
This module merges them into a single nodes.tsv + edges.tsv, emits KGX JSON-Lines,
runs a lightweight structural validation (dangling-edge check), and writes a
manifest with counts.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
from collections import defaultdict
from pathlib import Path
from typing import Iterable

NODE_HEADER = "id\tcategory\tname\tequivalent_identifiers\tprovided_by"
# KGX edge columns: id + S/P/O + provenance + KL/AT (Translator-expected).
EDGE_HEADER = (
    "id\tsubject\tpredicate\tobject\tprimary_knowledge_source\t"
    "aggregator_knowledge_source\tknowledge_level\tagent_type"
)
AGGREGATOR = "infores:biobtree"
BIOLINK_VERSION = "4.2.1"  # target Monarch release line; pin as needed

# Leaf categories the exporter emits. Used by validate() to reject unknown
# (e.g. invalid) categories. Keep in sync with categories.yaml + go.py.
KNOWN_CATEGORIES = {
    "biolink:Gene", "biolink:Protein", "biolink:Transcript", "biolink:Disease",
    "biolink:PhenotypicFeature", "biolink:SmallMolecule", "biolink:Drug",
    "biolink:SequenceVariant", "biolink:Pathway", "biolink:Cell",
    "biolink:CellLine", "biolink:GrossAnatomicalStructure",
    "biolink:OrganismTaxon", "biolink:ProteinFamily",
    "biolink:MacromolecularComplex", "biolink:NoncodingRNAProduct",
    "biolink:MolecularActivity", "biolink:BiologicalProcess",
    "biolink:CellularComponent",
}


# Canonical prefixes: biolink prefix-map entries (verified against
# biolink_model_prefix_map.json) PLUS bioregistry-verified prefixes for resources
# absent from biolink's curated subset. Node CURIEs whose prefix is outside this
# set are flagged by validate() as non-canonical (won't node-normalize cleanly).
# Still non-canonical (documented in categories.yaml): SWISSLIPID (needs SLM +
# zero-padded ids — an id transform, deferred).
CANONICAL_PREFIXES = {
    # biolink prefix map
    "UniProtKB", "ENSEMBL", "NCBIGene", "HGNC", "CHEBI", "CHEMBL.COMPOUND",
    "PUBCHEM.COMPOUND", "REACT", "GO", "MONDO", "DOID", "EFO", "OMIM", "HP",
    "MP", "UBERON", "CL", "NCBITaxon", "CLINVAR", "DBSNP", "DRUGBANK",
    "RNACENTRAL", "MSigDB", "HMDB", "GTOPDB",
    # bioregistry-verified (absent from biolink's curated map)
    "cellosaurus", "interpro", "corum", "lipidmaps", "orphanet",
}


def edge_id(subject: str, predicate: str, obj: str, primary: str) -> str:
    """Deterministic edge id (so reified/duplicate edges are identifiable)."""
    h = hashlib.md5(f"{subject}|{predicate}|{obj}|{primary}".encode()).hexdigest()
    return f"biobtree:{h[:16]}"


def format_edge(
    subject: str,
    predicate: str,
    obj: str,
    primary: str,
    *,
    knowledge_level: str = "not_provided",
    agent_type: str = "not_provided",
) -> str:
    """One KGX edge TSV row (trailing newline). Single source of column order."""
    eid = edge_id(subject, predicate, obj, primary)
    return (
        f"{eid}\t{subject}\t{predicate}\t{obj}\t{primary}\t{AGGREGATOR}\t"
        f"{knowledge_level}\t{agent_type}\n"
    )


def _read_rows(path: Path):
    with path.open(encoding="utf-8") as fh:
        header = next(fh, "").rstrip("\n")
        for line in fh:
            yield header, line.rstrip("\n")


def merge_nodes(inputs: Iterable[str | Path], out_path: str | Path) -> int:
    """Concatenate node TSVs, de-duplicating by node id (first wins)."""
    seen: set[str] = set()
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    n = 0
    with out_path.open("w", encoding="utf-8") as out:
        out.write(NODE_HEADER + "\n")
        for inp in inputs:
            p = Path(inp)
            if not p.exists():
                continue
            for _, row in _read_rows(p):
                if not row:
                    continue
                node_id = row.split("\t", 1)[0]
                if node_id in seen:
                    continue
                seen.add(node_id)
                out.write(row + "\n")
                n += 1
    return n


def merge_edges(
    inputs: Iterable[str | Path], out_path: str | Path, dedup: bool = True
) -> dict:
    """Concatenate edge TSVs and (by default) dedup by edge id.

    The generate/merge step that builds BioBTree's LMDB only dedups xrefs
    *per source key*; the same logical edge arriving via different keys (e.g. one
    protein pair across two intact entries, or a gene-protein edge via two gene
    namespaces) is collapsed by the query SERVICE at read time, not in storage.
    A materialized KG has no read-time layer, so we dedup here — one edge per
    deterministic id (subject|predicate|object|primary).

    Dedup is an external ``sort -u`` on the id column (disk-spilling, so it scales
    past RAM). Returns {input, written, removed}.
    """
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)

    # 1. concatenate bodies (no headers) to a temp file
    body = out_path.with_suffix(".body.tmp")
    total = 0
    with body.open("w", encoding="utf-8") as fh:
        for inp in inputs:
            p = Path(inp)
            if not p.exists():
                continue
            for _, row in _read_rows(p):
                if row:
                    fh.write(row + "\n")
                    total += 1

    final = body
    if dedup and total and shutil.which("sort"):
        srt = out_path.with_suffix(".sorted.tmp")
        env = {**os.environ, "LC_ALL": "C"}  # byte order, deterministic + fast
        subprocess.run(
            ["sort", "-t", "\t", "-k1,1", "-u", "-o", str(srt), str(body)],
            check=True, env=env,
        )
        body.unlink()
        final = srt

    # 2. write header + (deduped) body to the output, counting kept rows
    kept = 0
    with out_path.open("w", encoding="utf-8") as out:
        out.write(EDGE_HEADER + "\n")
        with final.open(encoding="utf-8") as fb:
            for line in fb:
                out.write(line)
                kept += 1
    final.unlink()
    return {"input": total, "written": kept, "removed": total - kept}


def nodes_to_jsonl(nodes_tsv: str | Path, out_path: str | Path) -> int:
    cols = NODE_HEADER.split("\t")
    n = 0
    with Path(out_path).open("w", encoding="utf-8") as out:
        for _, row in _read_rows(Path(nodes_tsv)):
            if not row:
                continue
            vals = row.split("\t")
            d = dict(zip(cols, vals))
            # KGX category should be a list; include the universal root so
            # consumers that query biolink:NamedThing match. (Full ancestor-chain
            # expansion via biolink-model-toolkit is a follow-up.)
            if d.get("category"):
                cats = [d["category"]]
                if d["category"] != "biolink:NamedThing":
                    cats.append("biolink:NamedThing")
                d["category"] = cats
            else:
                d["category"] = []
            d["equivalent_identifiers"] = (
                d["equivalent_identifiers"].split("|")
                if d.get("equivalent_identifiers")
                else []
            )
            out.write(json.dumps(d) + "\n")
            n += 1
    return n


def edges_to_jsonl(edges_tsv: str | Path, out_path: str | Path) -> int:
    cols = EDGE_HEADER.split("\t")
    n = 0
    with Path(out_path).open("w", encoding="utf-8") as out:
        for _, row in _read_rows(Path(edges_tsv)):
            if not row:
                continue
            out.write(json.dumps(dict(zip(cols, row.split("\t")))) + "\n")
            n += 1
    return n


def validate(nodes_tsv: str | Path, edges_tsv: str | Path) -> dict:
    """Lightweight structural validation: dangling edges + shape + dup checks."""
    node_ids: set[str] = set()
    bad_node_curie = 0
    bad_category = 0
    duplicate_node_ids = 0
    non_biolink_prefixes: dict[str, int] = defaultdict(int)
    for _, row in _read_rows(Path(nodes_tsv)):
        if not row:
            continue
        parts = row.split("\t")
        nid = parts[0]
        if nid in node_ids:
            duplicate_node_ids += 1
        node_ids.add(nid)
        if ":" not in nid:
            bad_node_curie += 1
        else:
            prefix = nid.split(":", 1)[0]
            if prefix not in CANONICAL_PREFIXES:
                non_biolink_prefixes[prefix] += 1
        if len(parts) > 1 and parts[1] and parts[1] not in KNOWN_CATEGORIES:
            bad_category += 1

    # edge columns: id, subject, predicate, object, primary, agg, kl, at
    edges = 0
    dangling_subject = 0
    dangling_object = 0
    bad_predicate = 0
    seen_edge_ids: set[str] = set()
    duplicate_edges = 0
    for _, row in _read_rows(Path(edges_tsv)):
        if not row:
            continue
        parts = row.split("\t")
        if len(parts) < 4:
            continue
        edges += 1
        eid, subj, pred, obj = parts[0], parts[1], parts[2], parts[3]
        if eid in seen_edge_ids:
            duplicate_edges += 1
        else:
            seen_edge_ids.add(eid)
        if subj not in node_ids:
            dangling_subject += 1
        if obj not in node_ids:
            dangling_object += 1
        if not pred.startswith("biolink:"):
            bad_predicate += 1

    return {
        "nodes": len(node_ids),
        "edges": edges,
        "dangling_subject_edges": dangling_subject,
        "dangling_object_edges": dangling_object,
        "bad_node_curie": bad_node_curie,
        "bad_category": bad_category,
        "bad_predicate": bad_predicate,
        "duplicate_node_ids": duplicate_node_ids,
        "duplicate_edges": duplicate_edges,
        # not gating (valid bioregistry prefixes can be outside biolink's curated
        # map); surfaced for the bioregistry-alignment pass.
        "non_biolink_prefixes": dict(sorted(
            non_biolink_prefixes.items(), key=lambda kv: -kv[1])),
        "ok": all(
            v == 0
            for v in (
                dangling_subject, dangling_object, bad_node_curie,
                bad_category, bad_predicate, duplicate_node_ids,
            )
        ),
    }


def manifest(
    nodes_tsv: str | Path,
    edges_tsv: str | Path,
    data_version: str | None = None,
    validation: dict | None = None,
    license: str = "https://www.gnu.org/licenses/agpl-3.0",
) -> dict:
    by_category: dict[str, int] = defaultdict(int)
    node_count = 0
    for _, row in _read_rows(Path(nodes_tsv)):
        if not row:
            continue
        node_count += 1
        cat = row.split("\t")[1] if "\t" in row else ""
        by_category[cat] += 1

    by_predicate: dict[str, int] = defaultdict(int)
    by_source: dict[str, int] = defaultdict(int)
    edge_count = 0
    for _, row in _read_rows(Path(edges_tsv)):
        if not row:
            continue
        parts = row.split("\t")
        if len(parts) < 6:  # id, subject, predicate, object, primary, agg, ...
            continue
        edge_count += 1
        by_predicate[parts[2]] += 1  # predicate
        by_source[parts[4]] += 1  # primary_knowledge_source

    return {
        "name": "biobtree-kg",
        "data_version": data_version,
        "biolink_model_version": BIOLINK_VERSION,
        "license": license,
        "format": "kgx-tsv+jsonl",
        "generated_by": "tools.kg_export",
        "knowledge_source": AGGREGATOR,
        "node_count": node_count,
        "edge_count": edge_count,
        "node_categories": dict(sorted(by_category.items(), key=lambda kv: -kv[1])),
        "edge_predicates": dict(sorted(by_predicate.items(), key=lambda kv: -kv[1])),
        "edge_sources": dict(sorted(by_source.items(), key=lambda kv: -kv[1])),
        "validation": validation,
    }
