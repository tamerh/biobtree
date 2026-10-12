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
import io
import gzip
import tarfile
import subprocess
import urllib.request

import pyarrow.parquet as pq

# eQTL signif_pairs has no phenotype_id (phenotype == gene); sQTL has both. Read
# only the columns a given file actually has, and derive a missing gene_id from
# the sQTL phenotype_id (…:clu_N:ENSG…).
WANT = ["variant_id", "gene_id", "slope", "pval_nominal", "phenotype_id"]

BUCKET = "https://storage.googleapis.com/adult-gtex/bulk-qtl/v10/single-tissue-cis-qtl"
TARS = {"eqtl": "GTEx_Analysis_v10_eQTL.tar", "sqtl": "GTEx_Analysis_v10_sQTL.tar"}
OUT_DIR = "raw_data/gtex"
RAW_TSV = os.path.join(OUT_DIR, "gtex_associations.tsv")
SORTED_GZ = os.path.join(OUT_DIR, "gtex_associations.sorted.tsv.gz")

# Gene-level eGene (eQTL) / sGene (sQTL) summary. These ship as per-tissue
# .txt.gz members (*.v10.eGenes.txt.gz / *.v10.sGenes.txt.gz) alongside the
# signif_pairs, one row per gene with its top association and q-value. Parsed by
# HEADER NAME (eGenes and sGenes differ in column order).
RAW_EGENE_TSV = os.path.join(OUT_DIR, "gtex_egenes.tsv")
SORTED_EGENE_GZ = os.path.join(OUT_DIR, "gtex_egenes.sorted.tsv.gz")
EGENE_SUFFIX = {"eqtl": ".v10.eGenes.txt.gz", "sqtl": ".v10.sGenes.txt.gz"}
EGENE_QVAL_SIG = 0.05  # genome-wide significant eGene/sGene threshold


def variant_key(vid):
    # chr1_13550_G_A_b38 -> 1:13550:G:A
    p = vid.split("_")
    if len(p) < 4:
        return None
    chrom = p[0][3:] if p[0].lower().startswith("chr") else p[0]
    return f"{chrom}:{p[1]}:{p[2]}:{p[3]}"


def process_egene_tar(path, qtl_type, out):
    """Read every per-tissue eGenes/sGenes .txt.gz member, emit one row per gene:
    gene_id  qtl_type  tissue  gene_name  is_egene  qval  lead_variant  slope"""
    suffix = EGENE_SUFFIX[qtl_type]
    n = 0
    with tarfile.open(path, "r") as tar:
        for m in tar.getmembers():
            if not m.name.endswith(suffix):
                continue
            tissue = os.path.basename(m.name).split(".")[0]
            f = tar.extractfile(m)
            if f is None:
                continue
            with gzip.open(f, "rt") as gz:
                header = gz.readline().rstrip("\n").split("\t")
                col = {name: i for i, name in enumerate(header)}
                need = ("gene_id", "gene_name", "variant_id", "qval", "slope")
                if not all(k in col for k in need):
                    print(f"  WARN {qtl_type} {tissue}: missing cols {[k for k in need if k not in col]}", flush=True)
                    continue
                rows = 0
                for line in gz:
                    p = line.rstrip("\n").split("\t")
                    if len(p) <= col["qval"]:
                        continue
                    gene = p[col["gene_id"]].split(".")[0]  # strip ENSG version
                    if not gene:
                        continue
                    qval = p[col["qval"]]
                    try:
                        is_egene = "true" if float(qval) <= EGENE_QVAL_SIG else "false"
                    except ValueError:
                        is_egene = "false"
                    lead = variant_key(p[col["variant_id"]]) or ""
                    out.write(f"{gene}\t{qtl_type}\t{tissue}\t{p[col['gene_name']]}\t{is_egene}\t{qval}\t{lead}\t{p[col['slope']]}\n")
                    rows += 1
                    n += 1
            print(f"  {qtl_type} {tissue}: {rows} genes", flush=True)
    return n


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
            bio = io.BytesIO(f.read())  # seekable: lets us inspect schema then read
            names = set(pq.ParquetFile(bio).schema_arrow.names)
            bio.seek(0)
            cols = [c for c in WANT if c in names]
            d = pq.read_table(bio, columns=cols).to_pydict()
            vids = d["variant_id"]
            genes = d.get("gene_id", [""] * len(vids))
            slopes = d["slope"]
            pvals = d["pval_nominal"]
            phenos = d.get("phenotype_id", [""] * len(vids))
            for i in range(len(vids)):
                vk = variant_key(vids[i])
                if vk is None:
                    continue
                gene = (genes[i] or "").split(".")[0]  # strip ENSG version
                pheno = phenos[i] or ""
                # sQTL: derive gene from phenotype_id (…:clu_N:ENSG…) if no gene col
                if not gene and pheno:
                    tail = pheno.split(":")[-1]
                    if tail.startswith("ENSG"):
                        gene = tail.split(".")[0]
                if qtl_type != "sqtl":
                    pheno = ""
                out.write(f"{vk}\t{qtl_type}\t{gene}\t{tissue}\t{slopes[i]}\t{pvals[i]}\t{pheno}\n")
                n += 1
            print(f"  {qtl_type} {tissue}: {len(vids)} pairs", flush=True)
    return n


def sort_gz(raw_tsv, sorted_gz):
    """External sort by key column 1 so the Go parser can stream-aggregate."""
    with gzip.open(sorted_gz, "wb") as gz:
        p = subprocess.Popen(["sort", "-t", "\t", "-k1,1", "-S", "4G", raw_tsv],
                             stdout=subprocess.PIPE)
        for line in p.stdout:
            gz.write(line)
        p.wait()
    os.remove(raw_tsv)


def build_associations():
    total = 0
    with open(RAW_TSV, "w") as out:
        for qtl_type, name in TARS.items():
            tar = download(name)
            total += process_tar(tar, qtl_type, out)
    print(f"wrote {total} associations to {RAW_TSV}; sorting by variant key ...", flush=True)
    sort_gz(RAW_TSV, SORTED_GZ)
    print(f"done -> {SORTED_GZ}", flush=True)


def build_egenes():
    total = 0
    with open(RAW_EGENE_TSV, "w") as out:
        for qtl_type, name in TARS.items():
            tar = download(name)  # already present from the associations run
            total += process_egene_tar(tar, qtl_type, out)
    print(f"wrote {total} gene rows to {RAW_EGENE_TSV}; sorting by gene id ...", flush=True)
    sort_gz(RAW_EGENE_TSV, SORTED_EGENE_GZ)
    print(f"done -> {SORTED_EGENE_GZ}", flush=True)


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    mode = sys.argv[1] if len(sys.argv) > 1 else ""
    if mode == "--egenes-only":
        build_egenes()
    elif mode == "--variants-only":
        build_associations()
    else:
        build_associations()
        build_egenes()


if __name__ == "__main__":
    sys.exit(main())
