#!/usr/bin/env python3
"""
Extract reference data from GtoPdb CSV files for testing.

This script reads the GtoPdb CSV files and extracts key data
for test validation. Run after a test build to generate reference_data.json.

Usage:
    python3 extract_reference_data.py [path_to_gtopdb_csv_dir]

If no path is provided, uses the default download location.
"""

import json
import csv
import requests
from pathlib import Path
import sys


def download_csv(url: str, output_path: Path) -> bool:
    """Download CSV file from GtoPdb"""
    try:
        print(f"Downloading {url}...")
        resp = requests.get(url, timeout=60)
        if resp.status_code == 200:
            output_path.write_bytes(resp.content)
            return True
        print(f"Failed to download: HTTP {resp.status_code}")
    except Exception as e:
        print(f"Download error: {e}")
    return False


def extract_reference_data(csv_dir: Path = None, output_path: str = "reference_data.json", limit: int = 100):
    """Extract reference data from GtoPdb CSV files"""

    # Download files if needed
    gtopdb_base = "https://www.guidetopharmacology.org/DATA/"
    files = {
        "targets_and_families.csv": "targets_and_families.csv",
        "GtP_to_UniProt_mapping.csv": "GtP_to_UniProt_mapping.csv",
        "GtP_to_HGNC_mapping.csv": "GtP_to_HGNC_mapping.csv",
    }

    if csv_dir is None:
        csv_dir = Path(__file__).parent / "data"
        csv_dir.mkdir(exist_ok=True)

        for local_name, remote_name in files.items():
            local_path = csv_dir / local_name
            if not local_path.exists():
                download_csv(f"{gtopdb_base}{remote_name}", local_path)

    # Load UniProt mappings
    uniprot_map = {}
    uniprot_file = csv_dir / "GtP_to_UniProt_mapping.csv"
    if uniprot_file.exists():
        with open(uniprot_file, 'r', encoding='utf-8') as f:
            reader = csv.DictReader(f)
            for row in reader:
                target_id = row.get('GtP Target ID', '').strip()
                uniprot_id = row.get('UniProt id', '').strip()
                if target_id and uniprot_id:
                    uniprot_map[target_id] = uniprot_id

    # Load HGNC mappings
    hgnc_map = {}
    hgnc_file = csv_dir / "GtP_to_HGNC_mapping.csv"
    if hgnc_file.exists():
        with open(hgnc_file, 'r', encoding='utf-8') as f:
            reader = csv.DictReader(f)
            for row in reader:
                target_id = row.get('GtP Target ID', '').strip()
                hgnc_id = row.get('HGNC id', '').strip()
                hgnc_symbol = row.get('HGNC symbol', '').strip()
                if target_id and hgnc_id:
                    hgnc_map[target_id] = {
                        'hgnc_id': hgnc_id,
                        'hgnc_symbol': hgnc_symbol
                    }

    # Extract target data
    reference_data = []
    targets_file = csv_dir / "targets_and_families.csv"

    if targets_file.exists():
        with open(targets_file, 'r', encoding='utf-8') as f:
            reader = csv.DictReader(f)
            count = 0

            for row in reader:
                target_id = row.get('Target id', '').strip()
                if not target_id:
                    continue

                entry = {
                    'target_id': target_id,
                    'name': row.get('Target name', '').strip(),
                    'type': row.get('Type', '').strip(),
                    'family_id': row.get('Family id', '').strip(),
                    'family_name': row.get('Family name', '').strip(),
                }

                # Add UniProt mapping
                if target_id in uniprot_map:
                    entry['uniprot_id'] = uniprot_map[target_id]

                # Add HGNC mapping
                if target_id in hgnc_map:
                    entry['hgnc_id'] = hgnc_map[target_id]['hgnc_id']
                    entry['hgnc_symbol'] = hgnc_map[target_id]['hgnc_symbol']

                # Only include entries with name
                if entry['name']:
                    reference_data.append(entry)
                    count += 1

                if count >= limit:
                    break

    # Sort by target_id
    reference_data.sort(key=lambda x: int(x['target_id']))

    # Write output
    with open(output_path, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"Wrote {len(reference_data)} entries to {output_path}")


def main():
    csv_dir = None
    if len(sys.argv) > 1:
        csv_dir = Path(sys.argv[1])
        if not csv_dir.exists():
            print(f"Directory not found: {csv_dir}")
            sys.exit(1)

    extract_reference_data(csv_dir)


if __name__ == "__main__":
    main()
