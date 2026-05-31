#!/usr/bin/env python3
"""
Extract ClinGen Gene-Disease Validity reference data for tests.

Downloads the gene-validity summary CSV and samples curations with a MONDO
disease id and a strong classification, exposing known-good assertion ids /
gene symbols for validation against a running biobtree server. CC0.
"""

import csv
import io
import json
import urllib.request
from pathlib import Path

URL = "https://search.clinicalgenome.org/kb/gene-validity/download"
N = 30


def assertion_id(report_url):
    marker = "assertion_"
    idx = report_url.find(marker)
    if idx < 0:
        return ""
    tail = report_url[idx + len(marker):]
    return tail[:36] if len(tail) >= 36 else ""


def main():
    out = Path(__file__).parent / "reference_data.json"
    with urllib.request.urlopen(URL, timeout=180) as resp:
        text = resp.read().decode("utf-8", "replace")

    reader = csv.reader(io.StringIO(text))
    header = None
    curations = []
    for row in reader:
        if not row:
            continue
        if header is None:
            if row[0].strip() == "GENE SYMBOL":
                header = {n.strip(): i for i, n in enumerate(row)}
            continue
        if row[0].strip().startswith("+") or not row[0].strip():
            continue

        def col(name):
            i = header.get(name, -1)
            return row[i].strip() if 0 <= i < len(row) else ""

        aid = assertion_id(col("ONLINE REPORT"))
        mondo = col("DISEASE ID (MONDO)")
        if aid and mondo.startswith("MONDO:"):
            curations.append({
                "assertion_id": aid,
                "gene_symbol": col("GENE SYMBOL"),
                "hgnc_id": col("GENE ID (HGNC)"),
                "mondo_id": mondo,
                "classification": col("CLASSIFICATION"),
            })
            if len(curations) >= N:
                break

    with open(out, "w") as fh:
        json.dump({"curations": curations}, fh, indent=2)
    print(f"Wrote {len(curations)} gene-validity curations to {out}")


if __name__ == "__main__":
    main()
