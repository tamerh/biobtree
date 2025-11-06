#!/usr/bin/env python3
"""
RNACentral Reference Data Extraction Script

Fetches RNACentral RNA sequence metadata from RNACentral API for each RNACentral ID.
Creates reference_data.json with complete RNA information for test validation.

RNACentral API: https://rnacentral.org/api/v1/rna/{urs_id}
"""

import json
import sys
import time
from pathlib import Path
from typing import Optional, Dict
import requests


class RnacentralExtractor:
    """Extracts complete RNACentral RNA sequence data from official RNACentral REST API"""

    BASE_URL = "https://rnacentral.org/api/v1/rna"

    def __init__(self):
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'Biobtree-Test-Suite/1.0'
        })

    def fetch_entry(self, rnacentral_id: str) -> Optional[Dict]:
        """
        Fetch COMPLETE RNACentral RNA sequence data from REST API

        Returns the full raw API response including:
        - rnacentral_id
        - md5
        - sequence
        - length
        - rna_type (e.g., rRNA, tRNA, miRNA, lncRNA)
        - description
        - count_distinct_organisms
        - distinct_databases (array of source databases)
        - is_active
        - xrefs (cross-references to external databases)
        - publications
        - secondary_structures

        Preserves all fields for maximum test flexibility.
        """
        try:
            url = f"{self.BASE_URL}/{rnacentral_id}"
            response = self.session.get(url, timeout=10)

            if response.status_code == 200:
                data = response.json()
                return {
                    "rnacentral_id": rnacentral_id,
                    "rnacentral_entry": data
                }
            elif response.status_code == 404:
                print(f"Warning: RNA sequence not found for {rnacentral_id}")
                return None
            else:
                print(f"Warning: HTTP {response.status_code} for {rnacentral_id}")
                return None

        except Exception as e:
            print(f"Error fetching {rnacentral_id}: {e}")
            return None

    def extract_all(self, id_file: Path, output_file: Path, limit: int = None):
        """Extract all RNA entries and save to JSON"""

        # Read IDs
        with open(id_file, 'r') as f:
            ids = [line.strip() for line in f if line.strip()]

        if limit and limit < len(ids):
            print(f"Found {len(ids)} RNACentral IDs, processing first {limit}")
            ids = ids[:limit]
        else:
            print(f"Found {len(ids)} RNACentral IDs to process")
        print("=" * 60)

        results = []
        failed = []

        for i, rnacentral_id in enumerate(ids, 1):
            print(f"[{i}/{len(ids)}] Fetching {rnacentral_id}...", end=" ", flush=True)

            data = self.fetch_entry(rnacentral_id)

            if data:
                results.append(data)
                entry = data.get("rnacentral_entry", {})
                rna_type = entry.get("rna_type", "unknown")
                length = entry.get("length", 0)
                print(f"✓ {rna_type} ({length} nt)")
            else:
                failed.append(rnacentral_id)
                print("✗ (no data)")

            # Rate limiting - be respectful to RNACentral servers
            if i < len(ids):
                time.sleep(0.5)

        print()
        print("=" * 60)
        print(f"Successfully extracted: {len(results)}/{len(ids)} entries")

        if failed:
            print(f"Failed: {len(failed)} entries")
            print(f"Failed IDs: {', '.join(failed[:10])}" +
                  (f" ... and {len(failed) - 10} more" if len(failed) > 10 else ""))

        # Save results
        with open(output_file, 'w') as f:
            json.dump(results, f, indent=2, ensure_ascii=False)

        print(f"\n✓ Saved to {output_file}")
        print(f"  Total entries: {len(results)}")

        # Calculate statistics
        if results:
            rna_types = {}
            total_length = 0
            total_organisms = 0

            for entry in results:
                rna_entry = entry.get("rnacentral_entry", {})
                rna_type = rna_entry.get("rna_type", "unknown")
                rna_types[rna_type] = rna_types.get(rna_type, 0) + 1
                total_length += rna_entry.get("length", 0)
                total_organisms += rna_entry.get("count_distinct_organisms", 0)

            avg_length = total_length / len(results) if results else 0
            avg_organisms = total_organisms / len(results) if results else 0

            print(f"  RNA types:")
            for rna_type, count in sorted(rna_types.items(), key=lambda x: -x[1]):
                print(f"    - {rna_type}: {count}")
            print(f"  Average sequence length: {avg_length:.1f} nt")
            print(f"  Average organisms per RNA: {avg_organisms:.1f}")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "rnacentral_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Check prerequisites
    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d rnacentral test")
        print("Then: cp test_out/reference/rnacentral_ids.txt tests/rnacentral/")
        return 1

    # Parse command-line arguments
    limit = int(sys.argv[1]) if len(sys.argv) > 1 else None

    print(f"RNACentral Reference Data Extraction")
    print(f"Input:  {id_file}")
    print(f"Output: {output_file}")
    if limit:
        print(f"Limit:  {limit} entries")
    print()

    # Extract data
    extractor = RnacentralExtractor()
    extractor.extract_all(id_file, output_file, limit=limit)

    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("\n\nInterrupted by user")
        sys.exit(1)
