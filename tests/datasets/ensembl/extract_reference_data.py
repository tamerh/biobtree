#!/usr/bin/env python3
"""
Extract reference data for Ensembl genes from Ensembl REST API.

Reads ensembl_ids.txt (from test_out/reference/) and fetches complete
gene data from the Ensembl REST API. Saves to reference_data.json.

Note: This works for main Ensembl division only. Ensembl Genomes API
has SSL certificate issues.
"""

import json
import time
import sys
from pathlib import Path

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)

ENSEMBL_API_BASE = "https://rest.ensembl.org"
IDS_FILE = "../../test_out/reference/ensembl_ids.txt"
OUTPUT_FILE = "reference_data.json"

def fetch_gene(gene_id: str) -> dict:
    """Fetch gene data from Ensembl API"""
    url = f"{ENSEMBL_API_BASE}/lookup/id/{gene_id}"
    headers = {"Content-Type": "application/json"}

    try:
        response = requests.get(url, headers=headers, timeout=10)

        if response.status_code == 200:
            gene_data = response.json()

            # Also fetch cross-references
            xref_url = f"{ENSEMBL_API_BASE}/xrefs/id/{gene_id}"
            xref_response = requests.get(xref_url, headers=headers, timeout=10)

            if xref_response.status_code == 200:
                gene_data["xrefs"] = xref_response.json()

            return gene_data
        else:
            print(f"  Error {response.status_code} for {gene_id}")
            return None

    except Exception as e:
        print(f"  Exception for {gene_id}: {e}")
        return None

def main():
    script_dir = Path(__file__).parent
    ids_file = script_dir / IDS_FILE
    output_file = script_dir / OUTPUT_FILE

    # Read gene IDs
    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print(f"Run: ./biobtree --genome-taxids 9606,7227 -d ensembl test")
        return 1

    with open(ids_file) as f:
        # IDs are in format "genome:gene_id"
        gene_ids = [line.strip().split(':')[1] if ':' in line else line.strip()
                    for line in f if line.strip()]

    print(f"Found {len(gene_ids)} gene IDs")
    print(f"Fetching from {ENSEMBL_API_BASE}...")

    # Fetch data for each gene
    genes = []
    for i, gene_id in enumerate(gene_ids, 1):
        print(f"Fetching {i}/{len(gene_ids)}: {gene_id}...", end=" ")

        gene_data = fetch_gene(gene_id)
        if gene_data:
            genes.append(gene_data)
            print("✓")
        else:
            print("✗")

        # Rate limiting
        time.sleep(0.1)

    # Save results
    with open(output_file, 'w') as f:
        json.dump(genes, f, indent=2)

    file_size = output_file.stat().st_size / 1024
    print(f"\nExtracted {len(genes)}/{len(gene_ids)} genes ({file_size:.1f} KB)")
    print(f"Saved to: {output_file}")

    return 0 if len(genes) > 0 else 1

if __name__ == "__main__":
    sys.exit(main())