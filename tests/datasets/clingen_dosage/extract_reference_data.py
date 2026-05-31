#!/usr/bin/env python3
"""
Extract ClinGen Dosage Sensitivity reference data for tests.

Streams the gene curation TSV and samples genes with a definitive
haploinsufficiency score (3) and a disease id, so the test suite has known-good
Entrez gene ids / scores to validate against a running biobtree server. CC0.
"""

import json
import urllib.request
from pathlib import Path

URL = "https://ftp.clinicalgenome.org/ClinGen_gene_curation_list_GRCh38.tsv"
N = 30


def main():
    out = Path(__file__).parent / "reference_data.json"
    genes = []
    header = None
    with urllib.request.urlopen(URL, timeout=180) as resp:
        for raw in resp:
            line = raw.decode("utf-8", "replace").rstrip("\n")
            if line.startswith("#"):
                if line.startswith("#Gene Symbol"):
                    header = {n.strip(): i for i, n in enumerate(line.lstrip("#").split("\t"))}
                continue
            if header is None or not line:
                continue
            f = line.split("\t")

            def col(name):
                i = header.get(name, -1)
                return f[i].strip() if 0 <= i < len(f) else ""

            gid = col("Gene ID")
            if not gid.isdigit():
                continue
            if col("Haploinsufficiency Score") == "3":
                genes.append({
                    "gene_id": gid,
                    "gene_symbol": col("Gene Symbol"),
                    "haplo_score": "3",
                    "haplo_disease_id": col("Haploinsufficiency Disease ID"),
                })
                if len(genes) >= N:
                    break

    with open(out, "w") as fh:
        json.dump({"genes": genes}, fh, indent=2)
    print(f"Wrote {len(genes)} dosage genes to {out}")


if __name__ == "__main__":
    main()
