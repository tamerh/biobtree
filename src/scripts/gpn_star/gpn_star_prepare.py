#!/usr/bin/env python3
"""
Prepare GPN-Star-V (100-vertebrate) scores for biobtree ingestion.

GPN-Star (Benegas et al., Nature 2026; supersedes GPN-MSA) publishes genome-wide
scores on HuggingFace (songlab/gpn-star-scores, Apache-2.0) as per-chromosome
Parquet, in two sets for the V (100-vertebrate) model:
  - llr/llr_chr{N}.parquet     — per (pos,alt): chrom,pos,ref,alt,llr_calibrated,abs_llr_calibrated
  - entropy/entropy_chr{N}.parquet — per POSITION: chrom,pos,ref,entropy_calibrated

We ship the mutation-rate-CALIBRATED scores (llr + abs_llr + entropy). V is the
product choice (leads ClinVar/coding AUPRC); M (447) is a planned second.

Go can't read Parquet, so this (like gtex_prepare.py) downloads both sets per
chromosome and streams a merge by position (both sorted by pos; entropy is 1 row
per position, applied to all alts there) into the TSV the Go parser reads:

    chrom<TAB>pos<TAB>ref<TAB>alt<TAB>score<TAB>abs_llr<TAB>entropy   (score = llr_calibrated)

chrom WITHOUT 'chr' prefix. No external sort (Go addProp3 + generate buckets sort).

Output: raw_data/gpn_star/gpn_star_v.tsv.gz
Requires: pyarrow (conda env biobtree). Run from the repo root.
"""
import os
import sys
import gzip
import subprocess

import pyarrow.parquet as pq

MODEL = "gpn-star-hg38-v100-200m"  # V (100-vertebrate)
BASE = f"https://huggingface.co/datasets/songlab/gpn-star-scores/resolve/main/data/{MODEL}"
OUT_DIR = "raw_data/gpn_star"
OUT_GZ = os.path.join(OUT_DIR, "gpn_star_v.tsv.gz")
CHROMS = [str(i) for i in range(1, 23)] + ["X", "Y"]  # no chrM


def download(kind, chrom):
    name = f"{kind}_chr{chrom}.parquet"
    dst = os.path.join(OUT_DIR, name)
    if os.path.exists(dst) and os.path.getsize(dst) > 0:
        return dst
    url = f"{BASE}/{kind}/{name}"
    print(f"  downloading {name} ...", flush=True)
    subprocess.run(["curl", "-fL", "-C", "-", "-o", dst, url], check=True)
    return dst


def llr_rows(path):
    """yield (pos, ref, alt, chrom, llr, abs_llr) from the per-(pos,alt) llr parquet."""
    pf = pq.ParquetFile(path)
    cols = ["chrom", "pos", "ref", "alt", "llr_calibrated", "abs_llr_calibrated"]
    for b in pf.iter_batches(columns=cols, batch_size=1_000_000):
        d = b.to_pydict()
        ch, pos, ref, alt = d["chrom"], d["pos"], d["ref"], d["alt"]
        llr, absllr = d["llr_calibrated"], d.get("abs_llr_calibrated", [None] * len(pos))
        for i in range(len(pos)):
            yield pos[i], ref[i], alt[i], ch[i], llr[i], absllr[i]


def ent_rows(path):
    """yield (pos, entropy) from the per-position entropy parquet."""
    pf = pq.ParquetFile(path)
    cols = ["pos", "entropy_calibrated"]
    for b in pf.iter_batches(columns=cols, batch_size=1_000_000):
        d = b.to_pydict()
        pos, ent = d["pos"], d["entropy_calibrated"]
        for i in range(len(pos)):
            yield pos[i], ent[i]


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    only = sys.argv[1] if len(sys.argv) > 1 else ""
    chroms = [only] if only else CHROMS
    n = 0
    with gzip.open(OUT_GZ + ".part", "wt") as out:
        for chrom in chroms:
            llr_path = download("llr", chrom)
            ent_path = download("entropy", chrom)
            eit = ent_rows(ent_path)
            ep, ev = next(eit, (None, None))
            rows = 0
            for pos, ref, alt, ch, llr, absllr in llr_rows(llr_path):
                if llr is None:
                    continue
                # advance the per-position entropy cursor to this pos (both asc by pos)
                while ep is not None and ep < pos:
                    ep, ev = next(eit, (None, None))
                ent = ev if ep == pos and ev is not None else ""
                c = ch[3:] if (ch.startswith("chr") or ch.startswith("CHR")) else ch
                a = "" if absllr is None else absllr
                out.write(f"{c}\t{pos}\t{ref}\t{alt}\t{llr}\t{a}\t{ent}\n")
                rows += 1
                n += 1
            print(f"  chr{chrom}: {rows} rows (total {n})", flush=True)
    os.replace(OUT_GZ + ".part", OUT_GZ)
    print(f"wrote {n} GPN-Star-V scores to {OUT_GZ}", flush=True)


if __name__ == "__main__":
    sys.exit(main())
