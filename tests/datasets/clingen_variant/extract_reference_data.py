#!/usr/bin/env python3
"""
Extract ClinGen Variant Pathogenicity reference data for tests.

Downloads the Evidence Repository classifications TSV and samples variants that
carry both an Allele Registry CA id and a ClinVar Variation Id, so the test
suite can validate the ClinVar bridge against a running biobtree server. CC0.
"""

import json
import urllib.request
from pathlib import Path

URL = "https://erepo.clinicalgenome.org/evrepo/api/summary/classifications/download"
N = 30


def main():
    out = Path(__file__).parent / "reference_data.json"
    variants = []
    header = None
    with urllib.request.urlopen(URL, timeout=240) as resp:
        for raw in resp:
            line = raw.decode("utf-8", "replace").rstrip("\n")
            if header is None:
                header = {n.strip(): i for i, n in enumerate(line.split("\t"))}
                continue
            if not line:
                continue
            f = line.split("\t")

            def col(name):
                i = header.get(name, -1)
                return f[i].strip() if 0 <= i < len(f) else ""

            ca = col("Allele Registry Id")
            clinvar = col("ClinVar Variation Id")
            if ca and clinvar.isdigit():
                variants.append({
                    "ca_id": ca,
                    "clinvar_id": clinvar,
                    "gene_symbol": col("HGNC Gene Symbol"),
                    "mondo_id": col("Mondo Id"),
                    "assertion": col("Assertion"),
                })
                if len(variants) >= N:
                    break

    with open(out, "w") as fh:
        json.dump({"variants": variants}, fh, indent=2)
    print(f"Wrote {len(variants)} variant classifications to {out}")


if __name__ == "__main__":
    main()
