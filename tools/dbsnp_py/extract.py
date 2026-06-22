"""dbSNP federation -> KGX, pure-Python multiprocessing (parity check vs the Go POC).

Same pattern as the Go POC: let `zcat` decompress (C zlib, ~328 MB/s ceiling on
one stream) and pipe plaintext in; parallelize the per-variant CPU (json + format +
md5) across workers. The catch vs Go: Python has no shared-memory workers (GIL ->
processes), so data must be COPIED across the process boundary. We therefore ship
large *byte chunks* (not per-variant objects) to workers; each worker parses its
chunk and writes its own KGX shard files.

Chunks are split only at line boundaries, so a variant straddling a chunk edge is
seen partially by two workers -- harmless: node/edge dedup at assemble unions them
(every gene/transcript line is independent; the single property line lands whole in
one chunk). Run:

    zcat dbsnp_sorted.*.index.gz | python extract.py --workers 8 --max-bytes 3.7e9 --out out/dbsnp_py
"""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import sys
import time
from multiprocessing import Process, Queue

CATEGORY = "biolink:SequenceVariant"
PRED = "biolink:is_sequence_variant_of"
PRIMARY = "infores:dbsnp"
AGG = "infores:biobtree"
KL, AT = "knowledge_assertion", "manual_agent"
NODE_HEADER = "id\tcategory\tname\tequivalent_identifiers\tprovided_by"
EDGE_HEADER = ("id\tsubject\tpredicate\tobject\tprimary_knowledge_source\t"
               "aggregator_knowledge_source\tknowledge_level\tagent_type\thas_evidence\tqualifiers")

B_PROP, B_ENTREZ, B_REFSEQ = b"-1", b"4", b"8"


def edge_id(subj: str, pred: str, obj: str) -> str:
    h = hashlib.md5(f"{subj}|{pred}|{obj}|{PRIMARY}".encode()).hexdigest()
    return "biobtree:" + h[:16]


def load_id_map(path: str | None) -> dict:
    """member CURIE -> canonical CURIE (Phase-1 map). Small (~219K rows), so each
    worker can hold the full dict. Empty if no path."""
    if not path:
        return {}
    m: dict = {}
    with gzip.open(path, "rt") as fh:
        next(fh, "")  # header
        for line in fh:
            member, _, canon = line.rstrip("\n").partition("\t")
            if member and canon:
                m[member] = canon
    return m


def _attrs_json(p: dict) -> str | None:
    out = {}
    for k in ("variant_type", "variant_class", "chromosome"):
        if p.get(k):
            out[k] = p[k]
    mane = p.get("hgvs_mane") or {}
    if mane.get("consequence"):
        out["consequence"] = mane["consequence"]
    if p.get("is_common"):
        out["is_common"] = True
    if mane.get("is_mane_select"):
        out["mane_select"] = True
    return json.dumps(out) if out else None


def worker(wid: int, q: Queue, out: str, with_attrs: bool, idm: dict, rq: Queue,
           genes_filter: set | None = None) -> None:
    nodes = gzip.open(f"{out}/dbsnp_nodes.{wid}.tsv.gz", "wt", compresslevel=1)
    edges = gzip.open(f"{out}/dbsnp_edges.{wid}.tsv.gz", "wt", compresslevel=1)
    attrs = gzip.open(f"{out}/dbsnp_attrs.{wid}.tsv.gz", "wt", compresslevel=1) if with_attrs else None
    nodes.write(NODE_HEADER + "\n")
    edges.write(EDGE_HEADER + "\n")
    nv = ge = te = ar = 0

    def flush(cur, genes, txs, prop):
        nonlocal nv, ge, te, ar
        if cur is None:
            return
        # showcase: keep only variants of the requested genes (entrez ids, bytes)
        if genes_filter is not None and not any(g in genes_filter for g in genes):
            return
        nv += 1
        rs = cur.decode().lower()  # canonical dbSNP id is lowercase: DBSNP:rs10
        subj = "DBSNP:" + rs
        name = rs
        if with_attrs and prop is not None:
            try:
                p = json.loads(prop)
            except (ValueError, TypeError):
                p = None
            if isinstance(p, dict):
                if p.get("rs_id"):
                    name = p["rs_id"]
                aj = _attrs_json(p)
                if aj:
                    attrs.write(f"{subj}\t{aj}\n")
                    ar += 1
        nodes.write(f"{subj}\t{CATEGORY}\t{name}\t{subj}\t{AGG}\n")
        for g in genes:
            gc = "NCBIGene:" + g.decode()
            obj = idm.get(gc, gc)  # canonicalize to the merged gene node (HGNC)
            edges.write(f"{edge_id(subj, PRED, obj)}\t{subj}\t{PRED}\t{obj}\t{PRIMARY}\t{AGG}\t{KL}\t{AT}\t\t\n")
            ge += 1
        for t in txs:
            obj = "refseq:" + t.decode()
            edges.write(f"{edge_id(subj, PRED, obj)}\t{subj}\t{PRED}\t{obj}\t{PRIMARY}\t{AGG}\t{KL}\t{AT}\t\t\n")
            te += 1

    while True:
        chunk = q.get()
        if chunk is None:
            break
        cur = None
        genes: list = []
        txs: list = []
        prop = None
        for line in chunk.split(b"\n"):
            if not line:
                continue
            f = line.split(b"\t", 3)
            if len(f) < 4:
                continue
            if f[0] != cur:
                flush(cur, genes, txs, prop)
                cur, genes, txs, prop = f[0], [], [], None
            ds = f[3]
            if ds == B_PROP:
                prop = f[2]
            elif ds == B_ENTREZ:
                genes.append(f[2])
            elif ds == B_REFSEQ:
                txs.append(f[2])
        flush(cur, genes, txs, prop)

    nodes.close()
    edges.close()
    if attrs:
        attrs.close()
    rq.put((nv, ge, te, ar))


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="out/dbsnp_py")
    ap.add_argument("--workers", type=int, default=8)
    ap.add_argument("--max-bytes", type=float, default=0, help="stop after N plaintext bytes (0=all)")
    ap.add_argument("--no-attrs", action="store_true")
    ap.add_argument("--chunk-mb", type=int, default=16)
    ap.add_argument("--id-map", default=None, help="Phase-1 member->canonical map (gene canonicalization)")
    ap.add_argument("--genes", default=None, help="showcase: file of entrez gene ids (one per line); "
                                                  "keep only variants of these genes")
    a = ap.parse_args()

    import os
    os.makedirs(a.out, exist_ok=True)
    with_attrs = not a.no_attrs
    maxb = int(a.max_bytes)
    idm = load_id_map(a.id_map)
    if a.id_map:
        print(f"id_map loaded: {len(idm)} entries", file=sys.stderr)
    gf = None
    if a.genes:
        gf = {ln.strip().encode() for ln in open(a.genes) if ln.strip()}
        print(f"gene filter: {len(gf)} entrez ids (showcase mode)", file=sys.stderr)

    q: Queue = Queue(maxsize=a.workers * 4)
    rq: Queue = Queue()
    procs = [Process(target=worker, args=(w, q, a.out, with_attrs, idm, rq, gf)) for w in range(a.workers)]
    for p in procs:
        p.start()

    t0 = time.time()
    rd = sys.stdin.buffer
    csize = a.chunk_mb << 20
    carry = b""
    total = 0
    while True:
        buf = rd.read(csize)
        if not buf:
            break
        buf = carry + buf
        nl = buf.rfind(b"\n")
        if nl < 0:
            carry = buf
            continue
        chunk, carry = buf[:nl + 1], buf[nl + 1:]
        total += len(chunk)
        q.put(chunk)
        if maxb and total >= maxb:
            break
    if carry:
        q.put(carry)
        total += len(carry)
    for _ in procs:
        q.put(None)

    nv = ge = te = ar = 0
    for _ in procs:
        a4 = rq.get()
        nv += a4[0]; ge += a4[1]; te += a4[2]; ar += a4[3]
    for p in procs:
        p.join()

    dt = time.time() - t0
    mb = total / 1e6
    print(f"dbsnp_py done in {dt:.1f}s (workers={a.workers})", file=sys.stderr)
    print(f"  variants={nv} gene_edges={ge} transcript_edges={te} attr_rows={ar}", file=sys.stderr)
    print(f"  parsed {mb/1000:.1f} GB plaintext @ {mb/dt:.0f} MB/s  {nv/dt:.0f} variants/s", file=sys.stderr)
    if nv:
        print(f"  extrapolate -> ~{dt*(1.1e9/nv)/60:.0f} min full (~1.1B)", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
