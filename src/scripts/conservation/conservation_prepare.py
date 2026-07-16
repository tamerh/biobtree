#!/usr/bin/env python3
"""
Conservation data preparation for biobtree.

Produces the per-position conservation TSV that the Go parser (src/update/
conservation.go) streams:

    chrom<TAB>pos<TAB>phylop<TAB>gerp<TAB>phastcons     (pos is 1-based)

from the UCSC hg38 whole-genome bigWig tracks:
    phyloP470way   https://hgdownload.soe.ucsc.edu/goldenPath/hg38/phyloP470way/hg38.phyloP470way.bw
    phastCons470way https://hgdownload.soe.ucsc.edu/goldenPath/hg38/phastCons470way/hg38.phastCons470way.bw

biobtree does not parse bigWig, so this src/scripts step converts it; the Go side
then streams the TSV. GERP++ RS is sourced from the Ensembl Compara ~90-mammal
bigWig (GRCh38-native; --no-gerp reverts to an empty column). Only positions
carrying a phyloP value are emitted (bigWig stores intervals — unaligned/repetitive
bases are absent); GERP/phastCons come from different alignments than phyloP, so a
row may carry phyloP with an empty GERP or phastCons.

Chromosomes are extracted in parallel (one worker per chromosome) and the
per-chromosome gz outputs are concatenated into one multi-member gzip
(conservation_hg38.tsv.gz) — the Go gzip reader reads it as a single stream, and
the parser buckets/sorts internally so global order is not required.

Usage:
    python conservation_prepare.py --output-dir raw_data/conservation
    python conservation_prepare.py --output-dir raw_data/conservation --chroms 21,Y --workers 4
"""
import argparse
import gzip
import logging
import multiprocessing as mp
import os
import shutil
import sys
import time
import urllib.request

logger = logging.getLogger("conservation_prepare")

PHYLOP_URL = "https://hgdownload.soe.ucsc.edu/goldenPath/hg38/phyloP470way/hg38.phyloP470way.bw"
PHASTCONS_URL = "https://hgdownload.soe.ucsc.edu/goldenPath/hg38/phastCons470way/hg38.phastCons470way.bw"
# GERP++ RS from Ensembl Compara — GRCh38-native (no liftover), Ensembl "no
# restrictions" data policy (ingest + KG-export clean). Pin the release path: the
# "NN_mammals" prefix changes per release. Ensembl chrom naming has NO "chr" prefix.
GERP_URL = "https://ftp.ensembl.org/pub/release-115/compara/conservation_scores/92_mammals.gerp_conservation_score/gerp_conservation_scores.homo_sapiens.GRCh38.bw"
PHYLOP_BW = "hg38.phyloP470way.bw"
PHASTCONS_BW = "hg38.phastCons470way.bw"
GERP_BW = "gerp_conservation_scores.homo_sapiens.GRCh38.bw"
MAIN_CHROMS = [str(i) for i in range(1, 23)] + ["X", "Y"]
WINDOW = 5_000_000  # bp per read window (bounds phastCons lookup memory)


def setup_logging(log_file=None):
    fmt = logging.Formatter("[%(asctime)s] [%(levelname)s] %(message)s", "%Y-%m-%d %H:%M:%S")
    ch = logging.StreamHandler(sys.stdout); ch.setFormatter(fmt)
    logger.addHandler(ch)
    if log_file:
        os.makedirs(os.path.dirname(log_file) or ".", exist_ok=True)
        fh = logging.FileHandler(log_file, mode="w"); fh.setFormatter(fmt); logger.addHandler(fh)
    logger.setLevel(logging.INFO)


def log(msg):
    logger.info(msg)


def _download(url, dest):
    if os.path.exists(dest) and os.path.getsize(dest) > 0:
        log(f"exists, skip download: {dest} ({os.path.getsize(dest)//1_000_000} MB)")
        return
    log(f"downloading {url} -> {dest}")
    tmp = dest + ".part"
    with urllib.request.urlopen(url) as r, open(tmp, "wb") as f:
        shutil.copyfileobj(r, f, length=1 << 20)
    os.rename(tmp, dest)
    log(f"downloaded {dest} ({os.path.getsize(dest)//1_000_000} MB)")


def _resolve_key(bw, chrom):
    """Resolve a chromosome name against a bigWig's naming (UCSC uses 'chrN',
    Ensembl uses 'N'). Returns the matching contig name or None."""
    if chrom in bw.chroms():
        return chrom
    alt = "chr" + chrom
    if alt in bw.chroms():
        return alt
    return None


def extract_chrom(args):
    """Worker: extract one chromosome to its own gz TSV. Returns (chrom, positions).

    Output row: chrom<TAB>pos<TAB>phylop<TAB>gerp<TAB>phastcons (1-based).
    The loop is PHYLOP-ANCHORED — only positions with a phyloP interval are
    emitted. GERP (Ensembl ~90-mammal EPO alignment) and phastCons (470-way multiz)
    have different coverage than phyloP, so a position may carry phyloP but an empty
    GERP/phastCons, and GERP-only positions are dropped. This is the established
    phyloP-anchored design; the three columns are not from one alignment (normal
    for a multi-source conservation table)."""
    import pyBigWig
    chrom, phylop_bw, phastcons_bw, gerp_bw, out_path = args
    pp = pyBigWig.open(phylop_bw); pc = pyBigWig.open(phastcons_bw)
    gp = pyBigWig.open(gerp_bw) if gerp_bw else None
    key = _resolve_key(pp, chrom)                       # phyloP anchors output
    if key is None:
        pp.close(); pc.close()
        if gp: gp.close()
        return chrom, 0
    ckey = _resolve_key(pc, chrom)                      # phastCons (UCSC 'chrN')
    gkey = _resolve_key(gp, chrom) if gp else None      # GERP (Ensembl 'N')
    length = pp.chroms()[key]
    outc = chrom[3:] if chrom.startswith("chr") else chrom
    n = 0
    with gzip.open(out_path, "wt") as out:
        for start in range(0, length, WINDOW):
            end = min(start + WINDOW, length)
            pcv = {}
            if ckey:
                for s, e, v in (pc.intervals(ckey, start, end) or []):
                    for p in range(s, e):
                        pcv[p] = v
            gpv = {}
            if gkey:
                for s, e, v in (gp.intervals(gkey, start, end) or []):
                    for p in range(s, e):
                        gpv[p] = v
            for s, e, v in (pp.intervals(key, start, end) or []):
                for p in range(s, e):
                    cval = pcv.get(p)
                    gval = gpv.get(p)
                    out.write("%s\t%d\t%.3f\t%s\t%s\n" % (
                        outc, p + 1, v,
                        "" if gval is None else "%.3f" % gval,
                        "" if cval is None else "%.3f" % cval))
                    n += 1
    pp.close(); pc.close()
    if gp: gp.close()
    return chrom, n


def main():
    ap = argparse.ArgumentParser(description="Prepare per-position conservation TSV from UCSC bigWigs")
    ap.add_argument("--output-dir", required=True, help="dir for the merged TSV (parser's dataset path dir)")
    ap.add_argument("--bigwig-dir", default=None, help="dir holding the bigWigs (default: output-dir)")
    ap.add_argument("--chroms", default=",".join(MAIN_CHROMS), help="comma-separated chromosomes (default 1-22,X,Y)")
    ap.add_argument("--workers", type=int, default=max(1, (os.cpu_count() or 2) - 2))
    ap.add_argument("--out-name", default="conservation_hg38.tsv.gz")
    ap.add_argument("--log-file", default=None)
    ap.add_argument("--keep-per-chrom", action="store_true", help="keep the per-chromosome gz parts")
    ap.add_argument("--no-gerp", action="store_true", help="skip GERP (leave column 4 empty, legacy behavior)")
    args = ap.parse_args()

    setup_logging(args.log_file)
    bw_dir = args.bigwig_dir or args.output_dir
    os.makedirs(args.output_dir, exist_ok=True)
    os.makedirs(bw_dir, exist_ok=True)
    phylop = os.path.join(bw_dir, PHYLOP_BW)
    phastcons = os.path.join(bw_dir, PHASTCONS_BW)
    gerp = None if args.no_gerp else os.path.join(bw_dir, GERP_BW)

    log("=" * 70)
    log("Conservation preparation (phyloP470way + phastCons470way%s, hg38)" %
        ("" if args.no_gerp else " + GERP"))
    log(f"Workers: {args.workers} | Chroms: {args.chroms}")
    log("=" * 70)

    _download(PHYLOP_URL, phylop)
    _download(PHASTCONS_URL, phastcons)
    if gerp:
        _download(GERP_URL, gerp)

    chroms = [c.strip() for c in args.chroms.split(",") if c.strip()]
    part_dir = os.path.join(args.output_dir, "_parts")
    os.makedirs(part_dir, exist_ok=True)
    tasks = [(c, phylop, phastcons, gerp, os.path.join(part_dir, f"cons_{c}.tsv.gz")) for c in chroms]

    t0 = time.time()
    total = 0
    with mp.Pool(args.workers) as pool:
        for chrom, n in pool.imap_unordered(extract_chrom, tasks):
            total += n
            log(f"  chr{chrom}: {n:,} positions")

    # Concatenate per-chromosome gz parts into one multi-member gzip.
    out_path = os.path.join(args.output_dir, args.out_name)
    log(f"merging {len(chroms)} parts -> {out_path}")
    with open(out_path, "wb") as w:
        for c in chroms:
            part = os.path.join(part_dir, f"cons_{c}.tsv.gz")
            if os.path.exists(part):
                with open(part, "rb") as r:
                    shutil.copyfileobj(r, w, length=1 << 20)
    if not args.keep_per_chrom:
        shutil.rmtree(part_dir, ignore_errors=True)

    log("=" * 70)
    log(f"Conservation prep complete: {total:,} positions -> {out_path} "
        f"({os.path.getsize(out_path)//1_000_000} MB) in {time.time()-t0:.0f}s")
    log("=" * 70)


if __name__ == "__main__":
    main()
