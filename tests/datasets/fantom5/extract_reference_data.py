#!/usr/bin/env python3
"""
Extract FANTOM5 Reference Data

FANTOM5 doesn't have a REST API like HGNC/UniProt.
Instead, we extract reference data from the biobtree API after a test build.

Usage:
1. Build test database: ./biobtree -d "fantom5_promoter" test
2. Start web server: ./biobtree web
3. Run this script: python3 extract_reference_data.py

This creates reference_data.json with sample entries from each FANTOM5 dataset.
"""

import json
import sys
from pathlib import Path

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


BASE_URL = "http://localhost:9292"


def get_entry(identifier: str, dataset: str) -> dict:
    """Fetch entry from biobtree API"""
    try:
        url = f"{BASE_URL}/ws/entry/?i={identifier}&s={dataset}"
        response = requests.get(url, timeout=10)
        if response.status_code == 200:
            data = response.json()
            if data and len(data) > 0:
                return data[0]
    except Exception as e:
        print(f"Error fetching {dataset}/{identifier}: {e}")
    return None


def load_ids(filename: str) -> list:
    """Load IDs from file"""
    path = Path(__file__).parent / filename
    if not path.exists():
        return []
    with open(path) as f:
        return [line.strip() for line in f if line.strip()]


def main():
    print("Extracting FANTOM5 reference data from biobtree API...")

    reference_data = {
        "fantom5_promoter": [],
        "fantom5_enhancer": [],
        "fantom5_gene": []
    }

    # Load IDs from test build
    promoter_ids = load_ids("fantom5_promoter_ids.txt")[:20]
    enhancer_ids = load_ids("fantom5_enhancer_ids.txt")[:20]
    gene_ids = load_ids("fantom5_gene_ids.txt")[:20]

    # If no ID files, use sequential IDs
    if not promoter_ids:
        promoter_ids = [str(i) for i in range(1, 21)]
    if not enhancer_ids:
        enhancer_ids = [str(i) for i in range(1, 21)]
    if not gene_ids:
        gene_ids = [str(i) for i in range(1, 21)]

    # Extract promoter data
    print(f"\nExtracting {len(promoter_ids)} promoter entries...")
    for id in promoter_ids:
        entry = get_entry(id, "fantom5_promoter")
        if entry:
            reference_data["fantom5_promoter"].append(entry)
            attrs = entry.get("Attributes", {}).get("Fantom5Promoter", {})
            gene = attrs.get("gene_symbol", "N/A")
            print(f"  {id}: {gene}")

    # Extract enhancer data
    print(f"\nExtracting {len(enhancer_ids)} enhancer entries...")
    for id in enhancer_ids:
        entry = get_entry(id, "fantom5_enhancer")
        if entry:
            reference_data["fantom5_enhancer"].append(entry)
            attrs = entry.get("Attributes", {}).get("Fantom5Enhancer", {})
            genes = attrs.get("associated_genes", [])
            gene_count = len(genes)
            print(f"  {id}: {gene_count} associated genes")

    # Extract gene data
    print(f"\nExtracting {len(gene_ids)} gene entries...")
    for id in gene_ids:
        entry = get_entry(id, "fantom5_gene")
        if entry:
            reference_data["fantom5_gene"].append(entry)
            attrs = entry.get("Attributes", {}).get("Fantom5Gene", {})
            symbol = attrs.get("gene_symbol", "N/A")
            print(f"  {id}: {symbol}")

    # Summary
    print(f"\n=== Summary ===")
    print(f"Promoters: {len(reference_data['fantom5_promoter'])}")
    print(f"Enhancers: {len(reference_data['fantom5_enhancer'])}")
    print(f"Genes: {len(reference_data['fantom5_gene'])}")

    # Save reference data
    output_file = Path(__file__).parent / "reference_data.json"
    with open(output_file, "w") as f:
        json.dump(reference_data, f, indent=2)

    print(f"\nSaved to: {output_file}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
