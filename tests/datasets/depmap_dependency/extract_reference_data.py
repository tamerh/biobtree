#!/usr/bin/env python3
"""
Reference data for DepMap dependency tests.

Reuses the same curated canonical genes as the DepMap aggregate suite; the tests
validate that these genes have per-cell-line dependency edges bridging to
Cellosaurus. CC BY 4.0.
"""

import json
from pathlib import Path

GENES = [
    {"gene_id": "3845", "gene_symbol": "KRAS"},
    {"gene_id": "673", "gene_symbol": "BRAF"},
    {"gene_id": "7157", "gene_symbol": "TP53"},
    {"gene_id": "5728", "gene_symbol": "PTEN"},
    {"gene_id": "4609", "gene_symbol": "MYC"},
    {"gene_id": "6122", "gene_symbol": "RPL3"},
]


def main():
    out = Path(__file__).parent / "reference_data.json"
    with open(out, "w") as fh:
        json.dump({"genes": GENES}, fh, indent=2)
    print(f"Wrote {len(GENES)} curated genes to {out}")


if __name__ == "__main__":
    main()
