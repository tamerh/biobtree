#!/usr/bin/env python3
"""
Taxonomy Reference Data Extraction Script

Fetches full taxonomy data from UniProt REST API for each taxonomy ID.
Creates reference_data.json with complete taxonomy information.

Note: While biobtree processes XML, we use the REST API for test reference data
as it provides equivalent information in an easier-to-parse format.
"""

import sys
import json
import time
from pathlib import Path
from typing import Optional, Dict, List

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class TaxonomyExtractor:
    """Extract taxonomy data from UniProt API"""

    BASE_URL = "https://rest.uniprot.org/taxonomy"

    def __init__(self, rate_limit: float = 0.2):
        self.rate_limit = rate_limit
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'BiobtreeTestSuite/1.0'
        })

    def fetch_entry(self, tax_id: str) -> Optional[Dict]:
        """Fetch a single taxonomy entry from UniProt API"""
        url = f"{self.BASE_URL}/{tax_id}"

        try:
            response = self.session.get(url, timeout=10)

            if response.status_code == 200:
                data = response.json()
                if data.get("taxonId"):
                    return data
                else:
                    print(f"  Warning: No data for {tax_id}")
                    return None
            elif response.status_code == 404:
                print(f"  Warning: {tax_id} not found (404)")
                return None
            else:
                print(f"  Error: {tax_id} returned status {response.status_code}")
                return None

        except requests.exceptions.Timeout:
            print(f"  Timeout: {tax_id}")
            return None
        except requests.exceptions.RequestException as e:
            print(f"  Error fetching {tax_id}: {e}")
            return None

    def extract_all(self, id_file: Path, output_file: Path):
        """Extract all taxonomy entries from ID list"""
        print(f"Reading taxonomy IDs from {id_file}")

        with open(id_file, 'r') as f:
            tax_ids = [line.strip() for line in f if line.strip()]

        print(f"Found {len(tax_ids)} taxonomy IDs")
        print(f"Fetching from UniProt API: {self.BASE_URL}")
        print()

        results = []
        failed = []

        for i, tax_id in enumerate(tax_ids, 1):
            print(f"[{i}/{len(tax_ids)}] Fetching {tax_id}...", end=" ")
            sys.stdout.flush()

            entry = self.fetch_entry(tax_id)

            if entry:
                results.append(entry)
                print("✓")
            else:
                failed.append(tax_id)
                print("✗")

            # Rate limiting
            if i < len(tax_ids):
                time.sleep(self.rate_limit)

        print()
        print(f"Successfully fetched: {len(results)}/{len(tax_ids)}")
        if failed:
            print(f"Failed: {len(failed)} entries")
            print(f"Failed IDs: {', '.join(failed[:10])}{'...' if len(failed) > 10 else ''}")

        # Save results with metadata wrapper
        print(f"Saving to {output_file}")
        output_data = {
            "metadata": {
                "total_ids": len(tax_ids),
                "fetched": len(results),
                "failed": len(failed)
            },
            "entries": results
        }
        with open(output_file, 'w') as f:
            json.dump(output_data, f, indent=2)

        print("✓ Extraction complete")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "taxonomy_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d taxonomy test")
        return 1

    extractor = TaxonomyExtractor(rate_limit=0.2)
    extractor.extract_all(id_file, output_file)

    return 0


if __name__ == "__main__":
    sys.exit(main())
