#!/usr/bin/env python3
"""
ESM1b variant-effect preparation for biobtree.

Melts the published human-proteome ESM1b LLR matrices (Brandes et al., Nat Genet
2023) into the per-variant TSV that the Go parser (src/update/esm1b.go) streams:

    uniprot<TAB>protein_variant<TAB>position<TAB>llr<TAB>gene_symbol

Input: ALL_hum_isoforms_ESM1b_LLR.zip — one CSV per UniProt id
(<uniprot>_LLR.csv, incl. isoforms like P01116-2). Each CSV is a 20xL matrix:
  - header: first cell empty, then "WT_AA position" per column (e.g. "M 1","A 2")
  - 20 data rows: mutant AA (K,R,H,E,D,N,Q,T,S,C,G,A,V,L,I,M,P,Y,F,W) + LLR per col
  - cell = ESM1b log-likelihood ratio (<=0; more negative = more damaging;
    WT==mut diagonal is 0.000 and is skipped)

variant notation = WT_AA + position + mut_AA (e.g. "M1K") — the same single-letter
format AlphaMissense's protein_variant uses, so Atlas joins ESM to a variant via
(uniprot_id, protein_variant).

gene_symbol is joined from isoform_list.csv (id,txt where txt = "GENE (SYM) | uid").

biobtree can't melt a 42k-CSV matrix archive, so this src/scripts step does it; the
Go side then streams the resulting gz TSV (same pattern as conservation_prepare.py).

Usage:
    python esm1b_prepare.py --zip raw_data/esm1b/ALL_hum_isoforms_ESM1b_LLR.zip \
        --isoform-list raw_data/esm1b/isoform_list.csv \
        --output raw_data/esm1b/esm1b_llr.tsv.gz
"""
import argparse
import csv
import gzip
import io
import logging
import os
import sys
import time
import zipfile

logger = logging.getLogger("esm1b_prepare")


def setup_logging(log_file=None):
    fmt = logging.Formatter("[%(asctime)s] [%(levelname)s] %(message)s", "%Y-%m-%d %H:%M:%S")
    ch = logging.StreamHandler(sys.stdout); ch.setFormatter(fmt); logger.addHandler(ch)
    if log_file:
        os.makedirs(os.path.dirname(log_file) or ".", exist_ok=True)
        fh = logging.FileHandler(log_file, mode="w"); fh.setFormatter(fmt); logger.addHandler(fh)
    logger.setLevel(logging.INFO)


def log(m):
    logger.info(m)


def load_gene_symbols(path):
    """isoform_list.csv: id,txt  where txt = 'GENE (SYM) | uniprot'. Return uid->GENE."""
    out = {}
    if not path or not os.path.exists(path):
        log(f"isoform list not found ({path}); gene_symbol will be empty")
        return out
    with open(path, newline="") as f:
        r = csv.reader(f)
        next(r, None)  # header id,txt
        for row in r:
            if len(row) < 2:
                continue
            uid = row[0].strip()
            gene = row[1].split("(")[0].strip()  # leading "GENE " before "(SYM)"
            out[uid] = gene
    log(f"loaded {len(out):,} gene symbols from isoform list")
    return out


def melt_csv(text, uniprot, gene, out):
    """Melt one <uniprot>_LLR.csv (20xL matrix) → per-variant TSV rows into out.
    Returns the number of variant rows written."""
    r = csv.reader(io.StringIO(text))
    header = next(r, None)
    if not header:
        return 0
    # header[1:] are "WT_AA position" columns
    cols = []
    for c in header[1:]:
        c = c.strip()
        if not c:
            cols.append(None); continue
        wt = c[0]
        pos = c[1:].strip()
        cols.append((wt, pos))
    n = 0
    for row in r:
        if not row:
            continue
        mut = row[0].strip()
        if not mut:
            continue
        for i, cell in enumerate(row[1:]):
            col = cols[i] if i < len(cols) else None
            if col is None:
                continue
            wt, pos = col
            if wt == mut:          # WT (diagonal, LLR 0) — not a variant
                continue
            cell = cell.strip()
            if cell == "":
                continue
            variant = f"{wt}{pos}{mut}"
            out.write(f"{uniprot}\t{variant}\t{pos}\t{cell}\t{gene}\n")
            n += 1
    return n


def main():
    ap = argparse.ArgumentParser(description="Melt ESM1b LLR matrices into a per-variant TSV")
    ap.add_argument("--zip", required=True, help="ALL_hum_isoforms_ESM1b_LLR.zip")
    ap.add_argument("--isoform-list", default=None, help="isoform_list.csv (for gene symbols)")
    ap.add_argument("--output", required=True, help="output gz TSV path")
    ap.add_argument("--log-file", default=None)
    ap.add_argument("--limit", type=int, default=0, help="stop after N isoforms (test)")
    args = ap.parse_args()

    setup_logging(args.log_file)
    genes = load_gene_symbols(args.isoform_list)

    t0 = time.time()
    total_vars = 0
    n_iso = 0
    os.makedirs(os.path.dirname(args.output) or ".", exist_ok=True)
    log(f"melting {args.zip} -> {args.output}")
    with zipfile.ZipFile(args.zip) as z, gzip.open(args.output, "wt") as out:
        names = [n for n in z.namelist() if n.endswith("_LLR.csv")]
        log(f"{len(names):,} isoform CSVs in archive")
        for name in names:
            base = os.path.basename(name)          # <uniprot>_LLR.csv
            uniprot = base[:-len("_LLR.csv")]
            gene = genes.get(uniprot, genes.get(uniprot.split("-")[0], ""))
            with z.open(name) as f:
                text = io.TextIOWrapper(f, encoding="utf-8").read()
            total_vars += melt_csv(text, uniprot, gene, out)
            n_iso += 1
            if n_iso % 2000 == 0:
                log(f"  {n_iso:,} isoforms, {total_vars:,} variants ({time.time()-t0:.0f}s)")
            if args.limit and n_iso >= args.limit:
                log(f"[LIMIT] stopped after {n_iso} isoforms"); break

    log("=" * 70)
    log(f"ESM1b prep complete: {n_iso:,} isoforms -> {total_vars:,} variants -> "
        f"{args.output} ({os.path.getsize(args.output)//1_000_000} MB) in {time.time()-t0:.0f}s")
    log("=" * 70)


if __name__ == "__main__":
    main()
