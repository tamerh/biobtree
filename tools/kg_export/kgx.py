"""Phase 3: assemble KGX outputs — merge, serialize (JSONL), validate, manifest.

The node/edge builders (nodes, edges, reified, go) each write a partial KGX TSV.
This module merges them into a single nodes.tsv + edges.tsv, emits KGX JSON-Lines,
runs a lightweight structural validation (dangling-edge check), and writes a
manifest with counts.
"""

from __future__ import annotations

import hashlib
import io
import json
import os
import re
import shutil
import subprocess
from collections import Counter, defaultdict
from pathlib import Path
from typing import Iterable

NODE_HEADER = "id\tcategory\tname\tequivalent_identifiers\tprovided_by"
# KGX edge columns: id + S/P/O + provenance + KL/AT (Translator-expected).
EDGE_HEADER = (
    "id\tsubject\tpredicate\tobject\tprimary_knowledge_source\t"
    "aggregator_knowledge_source\tknowledge_level\tagent_type\t"
    # qualified-edge slots (biolink association). Empty for plain edges.
    # has_evidence: pipe-separated ECO CURIEs. qualifiers: 'slot=v1,v2;slot2=v3'
    # (e.g. assay_type=BAO:..., phenotypic_quality=PATO:...).
    "has_evidence\tqualifiers"
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
    "biolink:CellularComponent", "biolink:ChemicalEntity",
    "biolink:MicroRNA", "biolink:NucleicAcidSequenceMotif", "biolink:Publication",
    "biolink:DiseaseOrPhenotypicFeature", "biolink:RegulatoryRegion",
    "biolink:Exon", "biolink:CodingSequence", "biolink:ProteinDomain",
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
    "cellosaurus", "interpro", "corum", "lipidmaps", "orphanet", "refseq",
    "drugcentral",
    # cross-species phenotype ontologies (bioregistry preferred_prefix, verified)
    "UPHENO", "ZP", "XPO", "WBPhenotype", "FYPO",
    "OBA",  # Ontology of Biological Attributes (GWAS quantitative traits)
    # Alliance model-organism gene namespaces (biolink Gene id_prefixes, verified)
    "MGI", "RGD", "SGD", "ZFIN", "FB", "WB", "Xenbase",
    "MESH",        # biolink prefix map (CTD chemicals + MeSH diseases)
    "civic.vid",   # bioregistry-canonical CIViC variant prefix
    "PMID",        # biolink Publication prefix (GeneRIF)
    "PHARMGKB.PATHWAYS",  # biolink prefix map (PharmGKB pgx pathways)
    "chembl.cell", # bioregistry-registered (ChEMBL cell lines)
    # NON-canonical (flagged, like SWISSLIPID): mirbase.mature (miRDB stores
    # miRBase NAMES not MIMAT accessions); jaspar (no registered CURIE prefix).
    # MolecularActivity id_prefixes (verified in biolink MolecularActivity)
    "EC", "RHEA",
}


# Cap parallelism so the build doesn't monopolize the machine (override via KG_NPROC).
_NPROC = str(max(1, int(os.environ.get("KG_NPROC", min(16, os.cpu_count() or 16)))))


class _ProcFile:
    """Text file-like backed by a pigz subprocess (parallel gzip). Delegates I/O to
    the wrapped text stream; close() drains/awaits the process (and the sink, for
    writers) so the .gz is complete before we move on."""

    def __init__(self, proc, stream, sink=None):
        self._p, self._s, self._sink = proc, stream, sink

    def __getattr__(self, k):
        return getattr(self._s, k)

    def __iter__(self):
        return iter(self._s)

    def __next__(self):
        return next(self._s)

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.close()

    def close(self):
        try:
            self._s.close()      # writer: closes pigz stdin (it then drains); reader: closes pipe
        except Exception:
            pass
        self._p.wait()
        if self._sink is not None:
            self._sink.close()
            if self._p.returncode not in (0, None):
                raise RuntimeError(f"pigz exited {self._p.returncode}")


def xopen(path, mode="rt"):
    """Open plain or gzipped by extension. .gz goes through pigz (parallel gzip)
    when available -- a large speedup writing/reading the billion-row dumps on a
    many-core box -- falling back to single-threaded gzip otherwise. Output is
    byte-identical gzip either way."""
    path = str(path)
    if not path.endswith(".gz"):
        return open(path, mode, encoding="utf-8")
    if shutil.which("pigz"):
        if "r" in mode:
            p = subprocess.Popen(["pigz", "-dc", path], stdout=subprocess.PIPE)
            return _ProcFile(p, io.TextIOWrapper(p.stdout, encoding="utf-8"))
        sink = open(path, "ab" if "a" in mode else "wb")
        p = subprocess.Popen(["pigz", "-p", _NPROC, "-c"],
                             stdin=subprocess.PIPE, stdout=sink)
        return _ProcFile(p, io.TextIOWrapper(p.stdin, encoding="utf-8"), sink=sink)
    import gzip
    return gzip.open(path, mode, encoding="utf-8")


# GO/Reactome GAF evidence codes -> ECO CURIEs, so `has_evidence` is uniformly ECO
# (BioBTree's index evidence field is ECO: for GO annotations but bare GAF codes for
# Reactome — TAS/IEA/IEP). Standard GAF-code -> ECO mapping.
_GAF_EVIDENCE_ECO = {
    "EXP": "ECO:0000269", "IDA": "ECO:0000314", "IPI": "ECO:0000353",
    "IMP": "ECO:0000315", "IGI": "ECO:0000316", "IEP": "ECO:0000270",
    "HTP": "ECO:0006056", "HDA": "ECO:0007005", "HMP": "ECO:0007001",
    "HGI": "ECO:0007003", "HEP": "ECO:0007007", "ISS": "ECO:0000250",
    "ISO": "ECO:0000266", "ISA": "ECO:0000247", "ISM": "ECO:0000255",
    "IGC": "ECO:0000317", "IBA": "ECO:0000318", "IBD": "ECO:0000319",
    "IKR": "ECO:0000320", "IRD": "ECO:0000321", "RCA": "ECO:0000245",
    "TAS": "ECO:0000304", "NAS": "ECO:0000303", "IC": "ECO:0000305",
    "ND": "ECO:0000307", "IEA": "ECO:0000501",
}


def to_evidence_curie(ev: str | None) -> str:
    """Normalize an index evidence field to an ECO CURIE: passthrough for `ECO:`,
    GAF code (Reactome TAS/IEA/…) -> ECO, anything else -> "" (not evidence)."""
    if not ev:
        return ""
    if ev.startswith("ECO:"):
        return ev
    return _GAF_EVIDENCE_ECO.get(ev.strip().upper(), "")


def edge_id(
    subject: str, predicate: str, obj: str, primary: str,
    has_evidence: str = "", qualifiers: str = "",
) -> str:
    """Deterministic edge id (so reified/duplicate edges are identifiable).

    Qualifiers/evidence are folded in ONLY when present, so plain edges keep the
    same id as before and qualified variants of the same S/P/O stay distinct
    (dedup is sort -u on this id, so identical ids would otherwise be dropped).
    """
    key = f"{subject}|{predicate}|{obj}|{primary}"
    if has_evidence or qualifiers:
        key += f"|{has_evidence}|{qualifiers}"
    h = hashlib.md5(key.encode()).hexdigest()
    return f"biobtree:{h[:16]}"


def format_edge(
    subject: str,
    predicate: str,
    obj: str,
    primary: str,
    *,
    knowledge_level: str = "not_provided",
    agent_type: str = "not_provided",
    has_evidence: str = "",
    qualifiers: str = "",
) -> str:
    """One KGX edge TSV row (trailing newline). Single source of column order."""
    eid = edge_id(subject, predicate, obj, primary, has_evidence, qualifiers)
    return (
        f"{eid}\t{subject}\t{predicate}\t{obj}\t{primary}\t{AGGREGATOR}\t"
        f"{knowledge_level}\t{agent_type}\t{has_evidence}\t{qualifiers}\n"
    )


def _read_rows(path: Path):
    with xopen(path, "rt") as fh:
        header = next(fh, "").rstrip("\n")
        for line in fh:
            yield header, line.rstrip("\n")


def _sort_file(src: Path, dst: Path, *key_args: str, uniq: bool = False,
               tmp_dir: Path | None = None) -> None:
    """External (disk-spilling) sort -- memory-flat at any scale. LC_ALL=C for
    deterministic byte order. Used by the billion-scale merge/stub steps."""
    args = ["sort", "-T", str(tmp_dir or src.parent), "-t", "\t",
            "--parallel=" + _NPROC, "-S", "50%", *key_args]
    if shutil.which("pigz"):
        args.append("--compress-program=pigz")  # parallel-compress the spill files
    if uniq:
        args.append("-u")
    args += ["-o", str(dst), str(src)]
    subprocess.run(args, check=True, env={**os.environ, "LC_ALL": "C"})


def merge_nodes(inputs: Iterable[str | Path], out_path: str | Path) -> int:
    """Concatenate node TSVs, de-duplicating by node id.

    Dedup is an external ``sort -u`` on the id column (disk-spilling), so it scales
    past RAM -- a billion-node graph (e.g. with the dbSNP layer) won't fit an
    in-memory set. One line per id survives (content-identical dups collapse
    cleanly; for the rare same-id/different-content case one is kept arbitrarily)."""
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    tmp_dir = out_path.parent
    body = Path(str(out_path) + ".nbody.tmp")

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
    if total and shutil.which("sort"):
        srt = Path(str(out_path) + ".nsorted.tmp")
        _sort_file(body, srt, "-k1,1", uniq=True, tmp_dir=tmp_dir)
        body.unlink()
        final = srt

    n = 0
    with xopen(out_path, "wt") as out:
        out.write(NODE_HEADER + "\n")
        with final.open(encoding="utf-8") as fb:
            for line in fb:
                out.write(line)
                n += 1
    final.unlink()
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
    # temp files + sort spill go next to the output (on /data), NOT /tmp (small root)
    tmp_dir = out_path.parent
    total = 0

    if dedup and shutil.which("sort"):
        # Stream rows STRAIGHT into `sort -u` -- no uncompressed .body.tmp (that was
        # ~194GB for a billion edges and exhausted the disk); sort spills compressed.
        srt = Path(str(out_path) + ".sorted.tmp")
        args = ["sort", "-u", "-t", "\t", "-k1,1", "-T", str(tmp_dir),
                "--parallel=" + _NPROC, "-S", "50%"]
        if shutil.which("pigz"):
            args.append("--compress-program=pigz")
        args += ["-o", str(srt)]
        sp = subprocess.Popen(args, stdin=subprocess.PIPE,
                              env={**os.environ, "LC_ALL": "C"}, text=True)
        for inp in inputs:
            p = Path(inp)
            if not p.exists():
                continue
            for _, row in _read_rows(p):
                if row:
                    sp.stdin.write(row + "\n")
                    total += 1
        sp.stdin.close()
        if sp.wait() != 0:
            raise RuntimeError(f"merge_edges sort failed (exit {sp.returncode})")
        kept = 0
        with xopen(out_path, "wt") as out:
            out.write(EDGE_HEADER + "\n")
            with open(srt, encoding="utf-8") as fb:
                for line in fb:
                    out.write(line)
                    kept += 1
        srt.unlink()
        return {"input": total, "written": kept, "removed": total - kept}

    # no-dedup fallback: stream straight to the gz output
    with xopen(out_path, "wt") as out:
        out.write(EDGE_HEADER + "\n")
        for inp in inputs:
            p = Path(inp)
            if not p.exists():
                continue
            for _, row in _read_rows(p):
                if row:
                    out.write(row + "\n")
                    total += 1
    return {"input": total, "written": total, "removed": 0}


def nodes_to_jsonl(nodes_tsv: str | Path, out_path: str | Path,
                   attributes: dict | None = None, attr_path: str | Path | None = None,
                   merge_fn=None, tmp_dir: Path | None = None) -> int:
    """Convert nodes.tsv -> nodes.jsonl, optionally merging node attributes as props.

    Two attribute sources (mutually exclusive):
      - ``attributes``: an in-memory ``{node_id: {prop: val}}`` dict (small graphs).
      - ``attr_path``:  a node-attribute table (``id<TAB>json`` lines) joined by a
        memory-flat **sorted merge-join** -- the node body is sorted by id and walked
        in lock-step with the (pre-sorted) attr file. This is what keeps assemble
        memory-flat at billion-scale (a 138M-row attr table would OOM as a dict).
    """
    cols = NODE_HEADER.split("\t")
    attributes = attributes or {}

    def _shape(d: dict, extra: dict | None):
        # KGX category should be a list; include the universal root so consumers that
        # query biolink:NamedThing match.
        if d.get("category"):
            cats = [d["category"]]
            if d["category"] != "biolink:NamedThing":
                cats.append("biolink:NamedThing")
            d["category"] = cats
        else:
            d["category"] = []
        d["equivalent_identifiers"] = (
            d["equivalent_identifiers"].split("|") if d.get("equivalent_identifiers") else [])
        if extra:
            d.update(extra)
        return d

    n = 0
    if attr_path is not None:
        tmp = Path(tmp_dir or Path(out_path).parent)
        # node body (no header), sorted by id to match the LC_ALL=C attr sort
        nb = Path(str(out_path) + ".nbody.tmp")
        with xopen(nodes_tsv, "rt") as fh, open(nb, "w") as o:
            next(fh, "")
            for line in fh:
                if line.strip():
                    o.write(line if line.endswith("\n") else line + "\n")
        nbs = Path(str(out_path) + ".nbody.sorted.tmp")
        _sort_file(nb, nbs, "-k1,1", tmp_dir=tmp)
        nb.unlink(missing_ok=True)
        af = xopen(attr_path, "rt")
        a_line = af.readline()
        with xopen(out_path, "wt") as out, open(nbs) as nf:
            for line in nf:
                row = line.rstrip("\n")
                if not row:
                    continue
                d = dict(zip(cols, row.split("\t")))
                nid = d.get("id") or ""
                extra: dict = {}
                while a_line:
                    aid, _, ajs = a_line.rstrip("\n").partition("\t")
                    if aid < nid:
                        a_line = af.readline()
                        continue
                    if aid > nid:
                        break
                    try:
                        props = json.loads(ajs)
                    except (ValueError, TypeError):
                        props = None
                    if props:
                        if merge_fn:
                            merge_fn(extra, props)
                        else:
                            extra.update(props)
                    a_line = af.readline()
                out.write(json.dumps(_shape(d, extra)) + "\n")
                n += 1
        af.close()
        nbs.unlink(missing_ok=True)
        return n

    with xopen(out_path, "wt") as out:
        for _, row in _read_rows(Path(nodes_tsv)):
            if not row:
                continue
            d = dict(zip(cols, row.split("\t")))
            out.write(json.dumps(_shape(d, attributes.get(d.get("id")))) + "\n")
            n += 1
    return n


def edges_to_jsonl(edges_tsv: str | Path, out_path: str | Path) -> int:
    cols = EDGE_HEADER.split("\t")
    n = 0
    with xopen(out_path, "wt") as out:
        for _, row in _read_rows(Path(edges_tsv)):
            if not row:
                continue
            d = dict(zip(cols, row.split("\t")))
            # has_evidence -> list of CURIEs; qualifiers 'slot=v1,v2;..' -> dict
            ev = d.get("has_evidence") or ""
            d["has_evidence"] = ev.split("|") if ev else []
            q = d.get("qualifiers") or ""
            d["qualifiers"] = (
                {kv.split("=", 1)[0]: kv.split("=", 1)[1].split(",")
                 for kv in q.split(";") if "=" in kv}
                if q else {}
            )
            out.write(json.dumps(d) + "\n")
            n += 1
    return n


# prefix -> category for entities referenced by edges but not built as nodes
# (taxid-scoped genes/proteins from broader sources). Known aliases for prefixes
# whose dataset prefix differs from how xref values arrive.
_STUB_ALIAS = {"SLM": "biolink:SmallMolecule"}


def _prefix_category(categories) -> dict[str, str]:
    """Build prefix -> biolink category from the category map (drops ambiguous)."""
    m: dict[str, str] = {}
    ambiguous: set[str] = set()
    for ds in categories.datasets():
        e = categories.entry_for(ds)
        if e.prefix in m and m[e.prefix] != e.category:
            ambiguous.add(e.prefix)
        m[e.prefix] = e.category
    for p in ambiguous:  # e.g. ENSEMBL (Gene vs Transcript) -> pattern below
        m.pop(p, None)
    m.update(_STUB_ALIAS)
    return m


def _stub_category(curie: str, pmap: dict[str, str]) -> str | None:
    prefix, _, local = curie.partition(":")
    if prefix in pmap:
        return pmap[prefix]
    if prefix == "ENSEMBL":  # ambiguous: disambiguate by id pattern
        return "biolink:Transcript" if re.search(r"T\d", local) else "biolink:Gene"
    return None


def add_stub_nodes(nodes_tsv: str | Path, edges_tsv: str | Path, categories) -> dict:
    """Append a minimal node for every edge endpoint that lacks one.

    Taxid-scoped sources mean some edges reference genes/proteins biobtree
    doesn't store as nodes; a materialized KG needs a node per endpoint. Category
    is inferred from the CURIE prefix. Endpoints whose prefix can't be typed are
    left (and counted) — they'll still show as dangling in validate().

    Memory-flat (scales to a billion-edge graph): the "endpoints with no node" set
    is computed by ``comm`` over the sorted-unique node ids and sorted-unique edge
    endpoints, not an in-memory id set.
    """
    pmap = _prefix_category(categories)
    nodes_tsv, edges_tsv = Path(nodes_tsv), Path(edges_tsv)
    tmp = nodes_tsv.parent

    # sorted-unique node ids
    nids = Path(str(nodes_tsv) + ".nids.tmp")
    with nids.open("w", encoding="utf-8") as fh:
        for _, row in _read_rows(nodes_tsv):
            if row:
                fh.write(row.split("\t", 1)[0] + "\n")
    nids_u = Path(str(nodes_tsv) + ".nids_u.tmp")
    _sort_file(nids, nids_u, "-k1,1", uniq=True, tmp_dir=tmp)
    nids.unlink()

    # sorted-unique edge endpoints (subjects + objects)
    eps = Path(str(nodes_tsv) + ".eps.tmp")
    with eps.open("w", encoding="utf-8") as fh:
        for _, row in _read_rows(edges_tsv):
            if not row:
                continue
            parts = row.split("\t")
            if len(parts) < 4:
                continue
            fh.write(parts[1] + "\n")
            fh.write(parts[3] + "\n")
    eps_u = Path(str(nodes_tsv) + ".eps_u.tmp")
    _sort_file(eps, eps_u, "-k1,1", uniq=True, tmp_dir=tmp)
    eps.unlink()

    # comm -13: endpoints present in edges (file2) but absent from nodes (file1)
    by_cat: Counter = Counter()
    untyped = 0
    env = {**os.environ, "LC_ALL": "C"}
    proc = subprocess.Popen(
        ["comm", "-13", str(nids_u), str(eps_u)],
        stdout=subprocess.PIPE, env=env, text=True,
    )
    with xopen(nodes_tsv, "at") as out:  # gz append = new member, readers concat fine
        for line in proc.stdout:
            ep = line.rstrip("\n")
            if not ep:
                continue
            cat = _stub_category(ep, pmap)
            if cat:
                out.write(f"{ep}\t{cat}\t\t{ep}\t{AGGREGATOR}\n")
                by_cat[cat] += 1
            else:
                untyped += 1
    proc.wait()
    nids_u.unlink()
    eps_u.unlink()
    return {
        "stubs_added": sum(by_cat.values()),
        "untyped_endpoints": untyped,
        "by_category": dict(by_cat),
    }


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


def validate_streaming(
    nodes_tsv: str | Path, edges_tsv: str | Path, *,
    removed_edges: int = 0, stub_untyped: int = 0,
) -> dict:
    """Billion-scale validation: cheap streaming shape checks, with dedup/dangling
    taken from the construction steps instead of giant in-memory sets.

    The full graph (with the dbSNP layer) has ~1.1B nodes / ~3B edges -- the
    ``node_ids`` / ``seen_edge_ids`` sets in ``validate()`` won't fit RAM. But the
    properties they check are already *guaranteed by construction*: ``merge_nodes``
    / ``merge_edges`` sort-dedup (so duplicates are 0; ``removed_edges`` records how
    many edge dups collapsed), and ``add_stub_nodes`` materializes a node for every
    typed endpoint (so the only dangling left is ``stub_untyped`` -- endpoints whose
    prefix can't be typed). So here we only stream once for the per-row shape checks
    (bad CURIE / category / predicate / non-canonical prefix) and fold in those
    counts. Use ``validate()`` (exact, in-memory) for the small published subgraph.
    """
    bad_node_curie = bad_category = node_count = 0
    non_biolink_prefixes: dict[str, int] = defaultdict(int)
    for _, row in _read_rows(Path(nodes_tsv)):
        if not row:
            continue
        parts = row.split("\t")
        nid = parts[0]
        node_count += 1
        if ":" not in nid:
            bad_node_curie += 1
        elif nid.split(":", 1)[0] not in CANONICAL_PREFIXES:
            non_biolink_prefixes[nid.split(":", 1)[0]] += 1
        if len(parts) > 1 and parts[1] and parts[1] not in KNOWN_CATEGORIES:
            bad_category += 1

    edges = bad_predicate = 0
    for _, row in _read_rows(Path(edges_tsv)):
        if not row:
            continue
        parts = row.split("\t")
        if len(parts) < 4:
            continue
        edges += 1
        if not parts[2].startswith("biolink:"):
            bad_predicate += 1

    return {
        "mode": "streaming",
        "nodes": node_count,
        "edges": edges,
        "bad_node_curie": bad_node_curie,
        "bad_category": bad_category,
        "bad_predicate": bad_predicate,
        # dangling/dup are guaranteed by the sort-based merge + stub steps; recorded
        # from their stats rather than recomputed with billion-entry sets.
        "untyped_dangling_endpoints": stub_untyped,
        "duplicate_edges_removed_at_merge": removed_edges,
        "non_biolink_prefixes": dict(sorted(
            non_biolink_prefixes.items(), key=lambda kv: -kv[1])),
        "ok": all(v == 0 for v in (
            bad_node_curie, bad_category, bad_predicate, stub_untyped)),
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
