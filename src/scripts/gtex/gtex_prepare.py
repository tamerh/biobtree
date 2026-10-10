#!/usr/bin/env python3
"""
Prepare GTEx v10 cis-QTL significant pairs for biobtree ingestion.

GTEx v10 ships the significant variant-gene pairs as per-tissue PARQUET files
inside two tarballs (eQTL, sQTL). Go can't read parquet conveniently, and the
tissue is encoded in the filename, so this script:

  1. downloads the eQTL + sQTL tarballs (once) from the open-access GTEx bucket,
  2. reads every *.signif_pairs.parquet member (tissue = filename prefix),
  3. writes one combined TSV, then sorts it by variant key so the Go parser can
     aggregate all associations for a variant in a single streaming pass.

Output: raw_data/gtex/gtex_associations.sorted.tsv.gz
Columns: variant_key  qtl_type  gene_id  tissue  slope  pval  phenotype_id
  variant_key = chr:pos:ref:alt (no 'chr' prefix), derived from the GTEx
  variant_id {chr}_{pos}_{ref}_{alt}_b38.

Requires: pyarrow (conda env biobtree). Run from the repo root.
"""
import os
import sys
import gzip
import tarfile
import subprocess
import urllib.request

import pyarrow.parquet as pq

BUCKET = "https://storage.googleapis.com/adult-gtex/bulk-qtl/v10/single-tissue-cis-qtl"
TARS = {"eqtl": "GTEx_Analysis_v10_eQTL.tar", "sqtl": "GTEx_Analysis_v10_sQTL.tar"}
OUT_DIR = "raw_data/gtex"
RAW_TSV = os.path.join(OUT_DIR, "gtex_associations.tsv")
SORTED_GZ = os.path.join(OUT_DIR, "gtex_associations.sorted.tsv.gz")


def variant_key(vid):
    # chr1_13550_G_A_b38 -> 1:13550:G:A
    p = vid.split("_")
    if len(p) < 4:
        return None
    chrom = p[0][3:] if p[0].lower().startswith("chr") else p[0]
    return f"{chrom}:{p[1]}:{p[2]}:{p[3]}"


def download(name):
    dst = os.path.join(OUT_DIR, name)
    if os.path.exists(dst):
        return dst
    url = f"{BUCKET}/{name}"
    print(f"downloading {url} ...", flush=True)
    urllib.request.urlretrieve(url, dst + ".part")
    os.rename(dst + ".part", dst)
    return dst


def process_tar(path, qtl_type, out):
    n = 0
    with tarfile.open(path, "r") as tar:
        for m in tar.getmembers():
            if not m.name.endswith(".signif_pairs.parquet"):
                continue
            tissue = os.path.basename(m.name).split(".")[0]
            f = tar.extractfile(m)
            if f is None:
                continue
            table = pq.read_table(f, columns=["variant_id", "gene_id", "slope",
                                              "pval_nominal", "phenotype_id"])
            d = table.to_pydict()
            vids = d["variant_id"]
            genes = d["gene_id"]
            slopes = d["slope"]
            pvals = d["pval_nominal"]
            phenos = d.get("phenotype_id", [""] * len(vids))
            for i in range(len(vids)):
                vk = variant_key(vids[i])
                if vk is None:
                    continue
                gene = (genes[i] or "").split(".")[0]  # strip ENSG version
                pheno = phenos[i] if qtl_type == "sqtl" and phenos[i] else ""
                out.write(f"{vk}\t{qtl_type}\t{gene}\t{tissue}\t{slopes[i]}\t{pvals[i]}\t{pheno}\n")
                n += 1
            print(f"  {qtl_type} {tissue}: {len(vids)} pairs", flush=True)
    return n


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    total = 0
    with open(RAW_TSV, "w") as out:
        for qtl_type, name in TARS.items():
            tar = download(name)
            total += process_tar(tar, qtl_type, out)
    print(f"wrote {total} associations to {RAW_TSV}; sorting by variant key ...", flush=True)
    # external sort by variant_key (col 1) so the Go parser can stream-aggregate
    with gzip.open(SORTED_GZ, "wb") as gz:
        p = subprocess.Popen(["sort", "-t", "\t", "-k1,1", "-S", "4G", RAW_TSV],
                             stdout=subprocess.PIPE)
        for line in p.stdout:
            gz.write(line)
        p.wait()
    os.remove(RAW_TSV)
    print(f"done -> {SORTED_GZ}", flush=True)


if __name__ == "__main__":
    sys.exit(main())
