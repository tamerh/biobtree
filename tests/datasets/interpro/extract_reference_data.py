#!/usr/bin/env python3
"""
InterPro Reference Data Extraction Script

Fetches full InterPro entry data from EBI InterPro API for each InterPro ID.
Creates reference_data.json with complete entry information.
"""

import sys
import json
import time
from pathlib import Path
from typing import Optional, Dict

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class InterProExtractor:
    """Extract InterPro entry data from EBI InterPro API"""

    BASE_URL = "https://www.ebi.ac.uk/interpro/api/entry/interpro"

    def __init__(self, rate_limit: float = 0.2):
        self.rate_limit = rate_limit
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'BiobtreeTestSuite/1.0'
        })

    def fetch_entry(self, interpro_id: str) -> Optional[Dict]:
        """Fetch a single InterPro entry from API - returns complete raw response"""
        url = f"{self.BASE_URL}/{interpro_id}"

        try:
            response = self.session.get(url, timeout=10)

            if response.status_code == 200:
                # Return the complete raw API response
                # This preserves all data for future test enhancements
                data = response.json()

                # Add the ID at the top level for easy access
                data["id"] = interpro_id

                return data

            elif response.status_code == 404:
                print(f"  Warning: {interpro_id} not found (404)")
                return None
            else:
                print(f"  Error: {interpro_id} returned status {response.status_code}")
                return None

        except requests.exceptions.Timeout:
            print(f"  Timeout: {interpro_id}")
            return None
        except requests.exceptions.RequestException as e:
            print(f"  Error fetching {interpro_id}: {e}")
            return None

    def extract_all(self, id_file: Path, output_file: Path):
        """Extract all InterPro entries from ID list"""
        print(f"Reading InterPro IDs from {id_file}")

        with open(id_file, 'r') as f:
            interpro_ids = [line.strip() for line in f if line.strip()]

        print(f"Found {len(interpro_ids)} InterPro IDs")
        print(f"Fetching from EBI InterPro API: {self.BASE_URL}")
        print()

        results = []
        failed = []

        for i, interpro_id in enumerate(interpro_ids, 1):
            print(f"[{i}/{len(interpro_ids)}] Fetching {interpro_id}...", end=" ")
            sys.stdout.flush()

            entry = self.fetch_entry(interpro_id)

            if entry:
                results.append(entry)
                print("✓")
            else:
                failed.append(interpro_id)
                print("✗")

            # Rate limiting
            if i < len(interpro_ids):
                time.sleep(self.rate_limit)

        print()
        print(f"Successfully fetched: {len(results)}/{len(interpro_ids)}")
        if failed:
            print(f"Failed: {len(failed)} entries")
            print(f"Failed IDs: {', '.join(failed[:10])}{'...' if len(failed) > 10 else ''}")

        # Save results with metadata wrapper
        print(f"Saving to {output_file}")
        output_data = {
            "metadata": {
                "total_ids": len(interpro_ids),
                "fetched": len(results),
                "failed": len(failed)
            },
            "entries": results
        }
        with open(output_file, 'w') as f:
            json.dump(output_data, f, indent=2)

        # Print sample
        if results:
            print()
            print("=" * 60)
            print("Sample entry (first ID):")
            first = results[0]
            metadata = first.get("metadata", {})
            print(f"  InterPro ID: {first['id']}")
            print(f"  Name:        {metadata.get('name', 'N/A')}")
            print(f"  Type:        {metadata.get('type', 'N/A')}")
            counters = metadata.get('counters', {})
            if 'proteins' in counters:
                print(f"  Proteins:    {counters['proteins']}")
            if metadata.get('member_databases'):
                db_names = list(metadata['member_databases'].keys())
                print(f"  Member DBs:  {', '.join(db_names[:5])}")
            print(f"  Full fields: {len(first)} top-level keys")
            print(f"  Metadata:    {len(metadata)} fields")
            print("  (Complete raw API response saved)")
            print("=" * 60)

        print("✓ Extraction complete")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "interpro_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d interpro test")
        print("Then copy test_out/reference/interpro_ids.txt here")
        return 1

    extractor = InterProExtractor(rate_limit=0.2)
    extractor.extract_all(id_file, output_file)

    return 0


if __name__ == "__main__":
    sys.exit(main())
