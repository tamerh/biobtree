"""Seed-driven published subgraph: an induced, trimmed projection of the full dump.

A *working snapshot* of the full graph -- every node category and edge predicate
present, just smaller: the taxon-dependent spine (genes/proteins/transcripts/...) is
scoped to one organism, and the big relational datasets are capped per node. It runs
as a post-filter over the assembled full `nodes.tsv` + `edges.tsv`; node attributes /
synonyms come back for free by re-running `assemble` on the filtered TSVs with the
same `--node-attributes` tables (they only attach to nodes that survive).

Passes (all streaming; in-memory state is just the kept-node sets + cap counters,
bounded by the *small* output, not the billion-row input):

 1. human set      -- subjects of `in_taxon -> <taxon>` edges (+ HGNC genes).
 2. base spine     -- every FULL-category node, plus SCOPED-category nodes that are
                      human. (Unknown categories are kept, to be safe.)
 3. edges          -- keep an edge anchored on the spine, up to a per-(subject,
                      predicate) cap for big sources (uncapped for `full_sources`/
                      cap 0); each kept edge pulls BOTH endpoints into the node set
                      (so capped compounds / variants / cross-species genes appear).
 4. nodes          -- emit the spine + pulled-in endpoints.

Config: mappings/subgraph.yaml. Completeness is asserted against the full manifest
(every category/predicate that exists upstream must survive the trim).
"""

from __future__ import annotations

import gzip
import json
import subprocess
from collections import defaultdict
from dataclasses import dataclass, field
from multiprocessing import Process, Queue
from pathlib import Path

import yaml

from . import kgx


def load_config(path: str | Path) -> dict:
    return yaml.safe_load(Path(path).read_text()) or {}


@dataclass
class SubgraphStats:
    human_nodes: int = 0
    spine_nodes: int = 0
    nodes_out: int = 0
    edges_in: int = 0
    edges_out: int = 0
    capped_dropped: int = 0
    omitted_dropped: int = 0
    unanchored_dropped: int = 0
    by_predicate: dict = field(default_factory=lambda: defaultdict(int))
    by_category: dict = field(default_factory=lambda: defaultdict(int))

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_predicate"] = dict(self.by_predicate)
        d["by_category"] = dict(self.by_category)
        return d


def _source_name(primary: str) -> str:
    return primary.split(":", 1)[1] if primary.startswith("infores:") else primary


def _parse_caps(config: dict) -> dict:
    """Parse `caps` into {source: (n, by_object)}.

    A cap value is either an int (cap per (subject, predicate)) or a mapping
    {n: int, by: subject|object}. `by: object` caps per (object, predicate) --
    needed when the spine node we want to bound is the edge's OBJECT, e.g.
    clinvar variant--is_sequence_variant_of-->gene (cap variants *per gene* = per
    object) or compound--interacts_with-->target (cap activities *per target*)."""
    out = {}
    for k, v in (config.get("caps") or {}).items():
        if isinstance(v, dict):
            n, by = v.get("n", 0), v.get("by", "subject")
        else:
            n, by = v, "subject"
        if n:
            out[k] = (int(n), by == "object")
    return out


def build_subgraph(
    nodes_tsv: str | Path,
    edges_tsv: str | Path,
    config: dict,
    out_nodes: str | Path,
    out_edges: str | Path,
    stats_path: str | Path | None = None,
    workers: int = 1,
) -> SubgraphStats:
    if workers and workers > 1:
        return _build_parallel(nodes_tsv, edges_tsv, config, out_nodes, out_edges,
                               stats_path, workers)
    nodes_tsv, edges_tsv = Path(nodes_tsv), Path(edges_tsv)
    out_nodes, out_edges = Path(out_nodes), Path(out_edges)
    out_nodes.parent.mkdir(parents=True, exist_ok=True)
    stats = SubgraphStats()

    taxon = config.get("taxon", "NCBITaxon:9606")
    full_cats = set(config.get("full_categories") or [])
    full_prefixes = set(config.get("full_prefixes") or [])
    scoped_cats = set(config.get("scoped_categories") or [])
    full_sources = set(config.get("full_sources") or [])
    omit_sources = set(config.get("omit_sources") or [])
    caps = _parse_caps(config)  # {source: (n, by_object)}
    default_cap = config.get("default_cap", 0)

    def node_cols(row: str):
        p = row.split("\t")
        return p[0], (p[1] if len(p) > 1 else "")

    # --- pass 1: human entity nodes (in_taxon -> taxon; + HGNC genes) -----------
    human: set[str] = set()
    for _, row in kgx._read_rows(edges_tsv):
        if not row:
            continue
        p = row.split("\t")
        if len(p) >= 4 and p[2] == "biolink:in_taxon" and p[3] == taxon:
            human.add(p[1])
    stats.human_nodes = len(human)

    # --- pass 2: base spine -----------------------------------------------------
    spine: set[str] = set()
    for _, row in kgx._read_rows(nodes_tsv):
        if not row:
            continue
        nid, cat = node_cols(row)
        if cat in full_cats or nid.split(":", 1)[0] in full_prefixes or not cat:
            spine.add(nid)
        elif cat in scoped_cats:
            if nid in human or nid.startswith("HGNC:"):
                spine.add(nid)
        else:  # unknown category -> keep (be safe; flagged by completeness check)
            spine.add(nid)
    stats.spine_nodes = len(spine)

    # --- pass 3: edges (anchored on spine, per-(subject,predicate) capped) -------
    keep_nodes: set[str] = set(spine)
    cap_count: dict = defaultdict(int)
    with kgx.xopen(out_edges, "wt") as eout:
        eout.write(kgx.EDGE_HEADER + "\n")
        for _, row in kgx._read_rows(edges_tsv):
            if not row:
                continue
            p = row.split("\t")
            if len(p) < 5:
                continue
            stats.edges_in += 1
            subj, pred, obj, primary = p[1], p[2], p[3], p[4]
            if subj not in spine and obj not in spine:
                stats.unanchored_dropped += 1
                continue
            src = _source_name(primary)
            if src in omit_sources:                    # giant layer dropped entirely
                stats.omitted_dropped += 1
                continue
            if src in full_sources:
                entry = None
            elif src in caps:
                entry = caps[src]                        # (n, by_object)
            else:
                entry = (default_cap, False) if default_cap else None
            if entry:  # bounded; absent -> keep all (representative default)
                cap, by_obj = entry
                key = (obj, pred) if by_obj else (subj, pred)
                if cap_count[key] >= cap:
                    stats.capped_dropped += 1
                    continue
                cap_count[key] += 1
            eout.write(row + "\n")
            stats.edges_out += 1
            stats.by_predicate[pred] += 1
            keep_nodes.add(subj)
            keep_nodes.add(obj)

    # --- pass 4: emit the kept nodes -------------------------------------------
    with kgx.xopen(out_nodes, "wt") as nout:
        nout.write(kgx.NODE_HEADER + "\n")
        for _, row in kgx._read_rows(nodes_tsv):
            if not row:
                continue
            nid, cat = node_cols(row)
            if nid in keep_nodes:
                nout.write(row + "\n")
                stats.nodes_out += 1
                stats.by_category[cat] += 1

    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats


# --- parallel path (zcat | N workers) --------------------------------------------
# The two full edge scans (in_taxon detect + filter) are the cost; we parallelize
# both with the dbsnp-style pattern: a parent process pipes `zcat <edges>` (C zlib)
# and hands newline-aligned byte chunks to worker processes. Caps become per-worker
# APPROXIMATE (a node's edges may split across workers, so the effective cap is up to
# workers*cap) -- fine for a representative graph, where caps only stop the giants
# from exploding. Memory: the spine set is built in the parent and inherited by the
# workers via fork.

def _zcat_chunks(path: Path, q: Queue, n_workers: int, chunk_mb: int = 16):
    """Pipe `zcat path` and put newline-aligned byte chunks on q; then n sentinels."""
    proc = subprocess.Popen(["zcat", str(path)], stdout=subprocess.PIPE)
    carry = b""
    csize = chunk_mb << 20
    while True:
        buf = proc.stdout.read(csize)
        if not buf:
            break
        buf = carry + buf
        nl = buf.rfind(b"\n")
        if nl < 0:
            carry = buf
            continue
        q.put(buf[:nl + 1])
        carry = buf[nl + 1:]
    if carry:
        q.put(carry)
    proc.stdout.close()
    proc.wait()
    for _ in range(n_workers):
        q.put(None)


def _intaxon_worker(q: Queue, taxon: bytes, rq: Queue):
    found: set = set()
    while True:
        chunk = q.get()
        if chunk is None:
            break
        for line in chunk.split(b"\n"):
            if not line:
                continue
            f = line.split(b"\t", 4)
            if len(f) >= 4 and f[2] == b"biolink:in_taxon" and f[3] == taxon:
                found.add(f[1])
    rq.put(found)


def _filter_worker(wid: int, q: Queue, spine: set, omit: set, caps: dict,
                   default_cap: int, out_dir: Path, rq: Queue):
    eout = gzip.open(out_dir / f"edges.{wid}.tsv.gz", "wb", compresslevel=1)
    epout = gzip.open(out_dir / f"endpoints.{wid}.txt.gz", "wb", compresslevel=1)
    cap_count: dict = defaultdict(int)
    edges_out = capped = omitted = unanchored = 0
    by_pred: dict = defaultdict(int)
    while True:
        chunk = q.get()
        if chunk is None:
            break
        for line in chunk.split(b"\n"):
            if not line:
                continue
            f = line.split(b"\t", 5)
            if len(f) < 5:
                continue
            subj, pred, obj, primary = f[1], f[2], f[3], f[4]
            if subj not in spine and obj not in spine:
                unanchored += 1
                continue
            src = primary[8:] if primary.startswith(b"infores:") else primary
            if src in omit:
                omitted += 1
                continue
            entry = caps.get(src)
            if entry is None and default_cap:
                entry = (default_cap, False)
            if entry:
                cap, by_obj = entry
                key = (obj, pred) if by_obj else (subj, pred)
                if cap_count[key] >= cap:
                    capped += 1
                    continue
                cap_count[key] += 1
            eout.write(line + b"\n")
            epout.write(subj + b"\n")
            epout.write(obj + b"\n")
            edges_out += 1
            by_pred[pred.decode()] += 1
    eout.close()
    epout.close()
    rq.put((edges_out, capped, omitted, unanchored, dict(by_pred)))


def _build_parallel(nodes_tsv, edges_tsv, config, out_nodes, out_edges, stats_path, workers):
    nodes_tsv, edges_tsv = Path(nodes_tsv), Path(edges_tsv)
    out_nodes, out_edges = Path(out_nodes), Path(out_edges)
    out_nodes.parent.mkdir(parents=True, exist_ok=True)
    tmp = out_edges.parent / "_sub_shards"
    tmp.mkdir(parents=True, exist_ok=True)
    stats = SubgraphStats()

    taxon = config.get("taxon", "NCBITaxon:9606").encode()
    full_cats = set(config.get("full_categories") or [])
    full_prefixes = set(config.get("full_prefixes") or [])
    scoped_cats = set(config.get("scoped_categories") or [])
    omit = {s.encode() for s in (config.get("omit_sources") or [])}
    # per-worker caps are independent, so divide the configured (total) cap by the
    # worker count -> the summed effective cap ~= the configured cap (approximate).
    # value is (n, by_object); axis is preserved per source.
    caps = {k.encode(): (max(1, n // workers), by_obj)
            for k, (n, by_obj) in _parse_caps(config).items()}
    default_cap = config.get("default_cap", 0)
    if default_cap:
        default_cap = max(1, default_cap // workers)

    # phase A: human anchor (subjects of in_taxon -> taxon), parallel
    q: Queue = Queue(maxsize=workers * 4)
    rq: Queue = Queue()
    procs = [Process(target=_intaxon_worker, args=(q, taxon, rq)) for _ in range(workers)]
    for p in procs:
        p.start()
    _zcat_chunks(edges_tsv, q, workers)
    human: set = set()
    for _ in procs:
        human |= rq.get()
    for p in procs:
        p.join()
    stats.human_nodes = len(human)

    # phase B prep: spine (node ids as bytes), built in parent -> inherited by workers
    spine: set = set()
    with kgx.xopen(nodes_tsv, "rt") as fh:
        next(fh, "")
        for line in fh:
            if not line.strip():
                continue
            parts = line.rstrip("\n").split("\t")
            nid, cat = parts[0], (parts[1] if len(parts) > 1 else "")
            nb = nid.encode()
            if cat in full_cats or nid.split(":", 1)[0] in full_prefixes or not cat:
                spine.add(nb)
            elif cat in scoped_cats:
                if nb in human or nid.startswith("HGNC:"):
                    spine.add(nb)
            else:
                spine.add(nb)
    stats.spine_nodes = len(spine)
    del human

    # phase B: filter edges, parallel -> edge + endpoint shards
    q = Queue(maxsize=workers * 4)
    rq = Queue()
    procs = [Process(target=_filter_worker,
                     args=(w, q, spine, omit, caps, default_cap, tmp, rq))
             for w in range(workers)]
    for p in procs:
        p.start()
    _zcat_chunks(edges_tsv, q, workers)
    for _ in procs:
        eo, cap, om, un, bp = rq.get()
        stats.edges_out += eo
        stats.capped_dropped += cap
        stats.omitted_dropped += om
        stats.unanchored_dropped += un
        for k, v in bp.items():
            stats.by_predicate[k] += v
    for p in procs:
        p.join()

    # keep_nodes = spine + endpoints of kept edges (from the endpoint shards)
    keep: set = set(spine)
    for w in range(workers):
        with gzip.open(tmp / f"endpoints.{w}.txt.gz", "rb") as fh:
            for line in fh:
                keep.add(line.rstrip(b"\n"))
    del spine

    # concat edge shards into out_edges (with header)
    with kgx.xopen(out_edges, "wt") as eout:
        eout.write(kgx.EDGE_HEADER + "\n")
    with open(out_edges, "ab") as eout:
        for w in range(workers):
            with open(tmp / f"edges.{w}.tsv.gz", "rb") as sh:
                while True:
                    b = sh.read(1 << 20)
                    if not b:
                        break
                    eout.write(b)

    # phase C: emit kept nodes (single pass; nodes file is small)
    with kgx.xopen(out_nodes, "wt") as nout:
        nout.write(kgx.NODE_HEADER + "\n")
        with kgx.xopen(nodes_tsv, "rt") as fh:
            next(fh, "")
            for line in fh:
                if not line.strip():
                    continue
                row = line.rstrip("\n")
                parts = row.split("\t")
                if parts[0].encode() in keep:
                    nout.write(row + "\n")
                    stats.nodes_out += 1
                    stats.by_category[parts[1] if len(parts) > 1 else ""] += 1

    # cleanup shards
    for w in range(workers):
        (tmp / f"edges.{w}.tsv.gz").unlink(missing_ok=True)
        (tmp / f"endpoints.{w}.txt.gz").unlink(missing_ok=True)
    try:
        tmp.rmdir()
    except OSError:
        pass

    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats


def check_completeness(stats: SubgraphStats, full_manifest: dict,
                       expected_missing: dict | None = None) -> dict:
    """Every category/predicate in the full graph must survive the trim, else the
    snapshot isn't faithful (a small dataset was accidentally dropped). Predicates/
    categories deliberately dropped (omitted giant layers, e.g. similar_to from the
    omitted similarity sources) are declared in `expected_missing` and don't fail."""
    expected_missing = expected_missing or {}
    exp_preds = set(expected_missing.get("predicates") or [])
    exp_cats = set(expected_missing.get("categories") or [])
    full_cats = set((full_manifest.get("node_categories") or {}).keys())
    full_preds = set((full_manifest.get("edge_predicates") or {}).keys())
    missing_cats = sorted(full_cats - set(stats.by_category) - exp_cats)
    missing_preds = sorted(full_preds - set(stats.by_predicate) - exp_preds)
    return {
        "ok": not missing_cats and not missing_preds,
        "missing_categories": missing_cats,
        "missing_predicates": missing_preds,
        "expected_missing": sorted(exp_preds | exp_cats),
    }
