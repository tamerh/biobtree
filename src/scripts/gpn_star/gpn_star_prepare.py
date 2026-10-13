#!/usr/bin/env python3
"""
Prepare GPN-Star-V (100-vertebrate) scores for biobtree ingestion.

GPN-Star (Benegas et al., Nature 2026; supersedes GPN-MSA) publishes genome-wide
scores on HuggingFace (songlab/gpn-star-scores, Apache-2.0) as per-chromosome
Parquet. We take the **V (100-vertebrate)** model's mutation-rate-CALIBRATED LLR
(`llr_calibrated`) — vertebrate is orthogonal to our existing conservation
(UCSC phyloP470way, mammalian); the M/447-mammal model would double-count it.

Go can't read Parquet, so this script (like gtex_prepare.py) downloads each
`llr_chr{N}.parquet` and converts to the TSV the Go parser reads:

    chrom<TAB>pos<TAB>ref<TAB>alt<TAB>score<TAB>abs_llr   (score = llr_calibrated)

chrom is emitted WITHOUT a 'chr' prefix (biobtree variant keys are 1:pos:ref:alt).
No external sort: the Go side is addProp3 (1 KV/variant) and the generate bucket
step sorts — so order doesn't matter (same as cadd).

Output: raw_data/gpn_star/gpn_star_v.tsv.gz
Requires: pyarrow (conda env biobtree). Run from the repo root.
"""
import os
import sys
import gzip
import subprocess

import pyarrow.parquet as pq

MODEL = "gpn-star-hg38-v100-200m"  # V (100-vertebrate)
BASE = f"https://huggingface.co/datasets/songlab/gpn-star-scores/resolve/main/data/{MODEL}/llr"
OUT_DIR = "raw_data/gpn_star"
OUT_GZ = os.path.join(OUT_DIR, "gpn_star_v.tsv.gz")
CHROMS = [str(i) for i in range(1, 23)] + ["X", "Y"]
WANT = ["chrom", "pos", "ref", "alt", "llr_calibrated", "abs_llr_calibrated"]


def download(chrom):
    name = f"llr_chr{chrom}.parquet"
    dst = os.path.join(OUT_DIR, name)
    if os.path.exists(dst) and os.path.getsize(dst) > 0:
        return dst
    url = f"{BASE}/{name}"
    print(f"  downloading {name} ...", flush=True)
    subprocess.run(["curl", "-fL", "-C", "-", "-o", dst, url], check=True)
    return dst


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    only = sys.argv[1] if len(sys.argv) > 1 else ""
    chroms = [only] if only else CHROMS
    n = 0
    with gzip.open(OUT_GZ + ".part", "wt") as out:
        for chrom in chroms:
            src = download(chrom)
            pf = pq.ParquetFile(src)
            cols = [c for c in WANT if c in pf.schema_arrow.names]
            if "llr_calibrated" not in cols:
                raise SystemExit(f"{src}: no llr_calibrated column (have {pf.schema_arrow.names})")
            rows = 0
            # stream in record batches so a 4.6 GB parquet never loads fully
            for batch in pf.iter_batches(columns=cols, batch_size=1_000_000):
                d = batch.to_pydict()
                ch = d["chrom"]; pos = d["pos"]; ref = d["ref"]; alt = d["alt"]
                llr = d["llr_calibrated"]; absllr = d.get("abs_llr_calibrated", [None] * len(pos))
                for i in range(len(pos)):
                    c = ch[i]
                    if c.startswith("chr") or c.startswith("CHR"):
                        c = c[3:]
                    s = llr[i]
                    if s is None:
                        continue
                    a = absllr[i]
                    out.write(f"{c}\t{pos[i]}\t{ref[i]}\t{alt[i]}\t{s}\t{'' if a is None else a}\n")
                    rows += 1
                    n += 1
            print(f"  chr{chrom}: {rows} rows (total {n})", flush=True)
    os.replace(OUT_GZ + ".part", OUT_GZ)
    print(f"wrote {n} GPN-Star-V scores to {OUT_GZ}", flush=True)


if __name__ == "__main__":
    sys.exit(main())
