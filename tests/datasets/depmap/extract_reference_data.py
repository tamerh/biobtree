#!/usr/bin/env python3
"""
Reference data for DepMap tests.

Uses a small curated set of canonical genes that are always present in DepMap
CRISPR screens (well-known oncogenes / tumor suppressors / essential genes), so
the suite can validate the gene-essentiality aggregate without re-downloading
the 440 MB matrix. CC BY 4.0.

Run this only to refresh the curated list; it writes a static reference set.
"""

import json
from pathlib import Path

GENES = [
    {"gene_id": "3845", "gene_symbol": "KRAS"},
    {"gene_id": "673", "gene_symbol": "BRAF"},
    {"gene_id": "7157", "gene_symbol": "TP53"},
    {"gene_id": "5728", "gene_symbol": "PTEN"},
    {"gene_id": "4609", "gene_symbol": "MYC"},
    {"gene_id": "6122", "gene_symbol": "RPL3"},  # pan-essential (ribosomal)
]


def main():
    out = Path(__file__).parent / "reference_data.json"
    with open(out, "w") as fh:
        json.dump({"genes": GENES}, fh, indent=2)
    print(f"Wrote {len(GENES)} curated DepMap genes to {out}")


if __name__ == "__main__":
    main()
