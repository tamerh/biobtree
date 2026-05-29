#!/usr/bin/env python3
"""
Extract CIViC reference data for tests.

Samples genes (features) and evidence items from the CIViC nightly TSV
releases so the test suite has known-good IDs/fields to validate against a
running biobtree server. No API key required.
"""

import csv
import io
import json
import urllib.request
from pathlib import Path

BASE = "https://civicdb.org/downloads/nightly/"
FEATURES = BASE + "nightly-FeatureSummaries.tsv"
EVIDENCE = BASE + "nightly-AcceptedAndSubmittedClinicalEvidenceSummaries.tsv"

N_GENES = 40
N_EVIDENCE = 40


def fetch_tsv(url):
    with urllib.request.urlopen(url, timeout=120) as resp:
        text = resp.read().decode("utf-8", errors="replace")
    return list(csv.DictReader(io.StringIO(text), delimiter="\t"))


def main():
    out = Path(__file__).parent / "reference_data.json"

    genes = []
    for row in fetch_tsv(FEATURES):
        if row.get("feature_type") == "Gene" and row.get("entrez_id"):
            genes.append({
                "feature_id": row["feature_id"],
                "name": row["name"],
                "gene_symbol": row["name"],
                "entrez_id": row["entrez_id"],
                "feature_type": row["feature_type"],
            })
        if len(genes) >= N_GENES:
            break

    evidence = []
    for row in fetch_tsv(EVIDENCE):
        if row.get("evidence_id") and row.get("doid"):
            evidence.append({
                "evidence_id": row["evidence_id"],
                "disease": row.get("disease", ""),
                "doid": row.get("doid", ""),
                "therapies": [t for t in row.get("therapies", "").split(",") if t],
                "evidence_type": row.get("evidence_type", ""),
                "evidence_level": row.get("evidence_level", ""),
            })
        if len(evidence) >= N_EVIDENCE:
            break

    with open(out, "w") as f:
        json.dump({"genes": genes, "evidence": evidence}, f, indent=2)
    print(f"Wrote {len(genes)} genes + {len(evidence)} evidence items to {out}")


if __name__ == "__main__":
    main()
