#!/usr/bin/env python3
"""
Extract ChEMBL Assay Reference Data

Fetches assay data from ChEMBL REST API for test IDs.
This creates reference data for validating biobtree's assay processing.

Usage:
    python3 extract_reference_data.py
"""

import json
import sys
from pathlib import Path
from time import sleep

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


CHEMBL_API_BASE = "https://www.ebi.ac.uk/chembl/api/data"
IDS_FILE = "chembl_assay_ids.txt"
OUTPUT_FILE = "reference_data.json"


def fetch_assay(assay_id: str) -> dict:
    """Fetch assay data from ChEMBL API"""
    url = f"{CHEMBL_API_BASE}/assay/{assay_id}.json"

    try:
        response = requests.get(url, timeout=10)

        if response.status_code == 200:
            return response.json()
        elif response.status_code == 404:
            print(f"  Warning: Assay {assay_id} not found in API")
            return None
        else:
            print(f"  Warning: API returned status {response.status_code} for {assay_id}")
            return None

    except requests.exceptions.RequestException as e:
        print(f"  Error fetching {assay_id}: {e}")
        return None


def main():
    script_dir = Path(__file__).parent
    ids_file = script_dir / IDS_FILE
    output_file = script_dir / OUTPUT_FILE

    if not ids_file.exists():
        print(f"Error: {IDS_FILE} not found")
        print("Run: ./biobtree -d chembl_assay test")
        return 1

    # Read assay IDs
    with open(ids_file) as f:
        assay_ids = [line.strip() for line in f if line.strip()]

    print(f"Fetching reference data for {len(assay_ids)} assays from ChEMBL API...")
    print(f"API: {CHEMBL_API_BASE}")
    print()

    reference_data = []

    for i, assay_id in enumerate(assay_ids, 1):
        print(f"[{i}/{len(assay_ids)}] Fetching {assay_id}...")

        data = fetch_assay(assay_id)

        if data:
            reference_data.append(data)

        # Rate limiting: be nice to ChEMBL API
        if i < len(assay_ids):
            sleep(0.5)

    print()
    print(f"Successfully fetched {len(reference_data)}/{len(assay_ids)} assays")

    # Save reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    file_size = output_file.stat().st_size / 1024
    print(f"Reference data saved to: {output_file}")
    print(f"File size: {file_size:.1f} KB")

    return 0


if __name__ == "__main__":
    sys.exit(main())
