#!/usr/bin/env python3
"""
Extract reference data from GWAS Catalog REST API for test validation

This script fetches complete SNP association data from the GWAS Catalog REST API for the test IDs.
The data is used to validate biobtree's GWAS association integration during testing.

GWAS Catalog API: https://www.ebi.ac.uk/gwas/rest/api/singleNucleotidePolymorphisms/{rsId}
"""

import json
import requests
import time
from pathlib import Path


def extract_gwas_reference_data():
    """Extract reference data from GWAS Catalog API for test SNP IDs"""

    script_dir = Path(__file__).parent
    id_file = script_dir / "gwas_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d gwas test")
        print("Then: cp test_out/reference/gwas_ids.txt tests/datasets/gwas/")
        return 1

    # Read test IDs
    with open(id_file, 'r') as f:
        test_ids = [line.strip() for line in f if line.strip()]

    print(f"Extracting reference data for {len(test_ids)} SNPs...")

    reference_data = []

    for i, snp_id in enumerate(test_ids, 1):
        # GWAS Catalog API endpoint for SNPs
        api_url = f"https://www.ebi.ac.uk/gwas/rest/api/singleNucleotidePolymorphisms/{snp_id}"

        try:
            print(f"  [{i}/{len(test_ids)}] Fetching {snp_id}...", end=" ", flush=True)
            response = requests.get(api_url, timeout=10)

            if response.status_code == 200:
                data = response.json()
                # Store complete API response
                data["_test_metadata"] = {
                    "identifier": snp_id,
                    "api_url": api_url,
                    "status_code": response.status_code
                }
                reference_data.append(data)
                print("✓")
            else:
                print(f"Failed (HTTP {response.status_code})")
                # Still add entry to maintain index alignment
                reference_data.append({
                    "_test_metadata": {
                        "identifier": snp_id,
                        "api_url": api_url,
                        "status_code": response.status_code,
                        "error": f"HTTP {response.status_code}"
                    }
                })

            # Rate limiting - be respectful to EBI servers
            time.sleep(0.2)

        except Exception as e:
            print(f"Error: {e}")
            reference_data.append({
                "_test_metadata": {
                    "identifier": snp_id,
                    "api_url": api_url,
                    "error": str(e)
                }
            })

    # Save reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    successful = len([d for d in reference_data if d.get('_test_metadata', {}).get('status_code') == 200])
    print(f"\nSaved reference data to {output_file}")
    print(f"Successfully extracted {successful}/{len(test_ids)} SNPs")

    return 0


if __name__ == "__main__":
    exit(extract_gwas_reference_data())
