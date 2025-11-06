#!/usr/bin/env python3
"""
ChEMBL Molecule Reference Data Extraction Script

Fetches full ChEMBL molecule data from ChEMBL REST API for each molecule ID.
Creates reference_data.json with complete molecule information for test validation.

ChEMBL API: https://www.ebi.ac.uk/chembl/api/data/molecule/<CHEMBL_ID>
"""

import json
import sys
import time
from pathlib import Path
from typing import Optional, Dict
import requests

class ChEMBLMoleculeExtractor:
    """Extracts complete ChEMBL molecule data from official ChEMBL REST API"""

    BASE_URL = "https://www.ebi.ac.uk/chembl/api/data/molecule"

    def __init__(self):
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'Biobtree-Test-Suite/1.0'
        })

    def fetch_entry(self, chembl_id: str) -> Optional[Dict]:
        """
        Fetch COMPLETE ChEMBL molecule data from REST API

        Returns the full raw API response to preserve all fields
        for maximum test flexibility.
        """
        url = f"{self.BASE_URL}/{chembl_id}.json"

        try:
            response = self.session.get(url, timeout=10)

            if response.status_code == 200:
                data = response.json()

                # Add ID at top level for easy access in tests
                data["id"] = chembl_id

                return data
            elif response.status_code == 404:
                print(f"Warning: {chembl_id} not found (404)")
                return None
            else:
                print(f"Warning: HTTP {response.status_code} for {chembl_id}")
                return None

        except Exception as e:
            print(f"Error fetching {chembl_id}: {e}")
            return None

    def extract_all(self, id_file: Path, output_file: Path):
        """Extract all molecule entries and save to JSON"""

        # Read IDs
        with open(id_file, 'r') as f:
            ids = [line.strip() for line in f if line.strip()]

        print(f"Extracting {len(ids)} ChEMBL molecule entries from API...")

        entries = []
        for idx, chembl_id in enumerate(ids, 1):
            print(f"  [{idx}/{len(ids)}] Fetching {chembl_id}...", end=' ')

            entry = self.fetch_entry(chembl_id)

            if entry:
                entries.append(entry)
                print("✓")
            else:
                print("✗")

            # Rate limiting - be nice to ChEMBL API
            time.sleep(0.2)

        # Save to JSON
        with open(output_file, 'w') as f:
            json.dump(entries, f, indent=2)

        print(f"\n✓ Saved {len(entries)} entries to {output_file}")
        print(f"  File size: {output_file.stat().st_size / 1024:.1f} KB")

        if len(entries) < len(ids):
            print(f"\nWarning: Could not fetch {len(ids) - len(entries)} entries")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "chembl_molecule_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("\nRun these steps first:")
        print("  1. cd /data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/biobtreev2")
        print("  2. ./biobtree -d 'chembl_molecule' test")
        print("  3. cp test_out/reference/chembl_molecule_ids.txt tests/chembl_molecule/")
        return 1

    extractor = ChEMBLMoleculeExtractor()
    extractor.extract_all(id_file, output_file)

    return 0


if __name__ == "__main__":
    sys.exit(main())
