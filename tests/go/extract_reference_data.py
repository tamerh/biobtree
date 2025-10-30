#!/usr/bin/env python3
"""
GO Reference Data Extraction Script

Fetches full GO term data from QuickGO REST API for each GO ID.
Creates reference_data.json with complete term information.
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


class GOExtractor:
    """Extract GO term data from QuickGO API"""

    BASE_URL = "https://www.ebi.ac.uk/QuickGO/services/ontology/go/terms"

    def __init__(self, rate_limit: float = 0.2):
        self.rate_limit = rate_limit
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'BiobtreeTestSuite/1.0'
        })

    def fetch_entry(self, go_id: str) -> Optional[Dict]:
        """Fetch a single GO term from QuickGO API"""
        url = f"{self.BASE_URL}/{go_id}"

        try:
            response = self.session.get(url, timeout=10)

            if response.status_code == 200:
                data = response.json()
                # QuickGO returns results in a 'results' array
                if data.get("results") and len(data["results"]) > 0:
                    return data["results"][0]
                else:
                    print(f"  Warning: No data for {go_id}")
                    return None
            elif response.status_code == 404:
                print(f"  Warning: {go_id} not found (404)")
                return None
            else:
                print(f"  Error: {go_id} returned status {response.status_code}")
                return None

        except requests.exceptions.Timeout:
            print(f"  Timeout: {go_id}")
            return None
        except requests.exceptions.RequestException as e:
            print(f"  Error fetching {go_id}: {e}")
            return None

    def extract_all(self, id_file: Path, output_file: Path):
        """Extract all GO terms from ID list"""
        print(f"Reading GO IDs from {id_file}")

        with open(id_file, 'r') as f:
            go_ids = [line.strip() for line in f if line.strip()]

        print(f"Found {len(go_ids)} GO IDs")
        print(f"Fetching from QuickGO API: {self.BASE_URL}")
        print()

        results = []
        failed = []

        for i, go_id in enumerate(go_ids, 1):
            print(f"[{i}/{len(go_ids)}] Fetching {go_id}...", end=" ")
            sys.stdout.flush()

            entry = self.fetch_entry(go_id)

            if entry:
                results.append(entry)
                print("✓")
            else:
                failed.append(go_id)
                print("✗")

            # Rate limiting
            if i < len(go_ids):
                time.sleep(self.rate_limit)

        print()
        print(f"Successfully fetched: {len(results)}/{len(go_ids)}")
        if failed:
            print(f"Failed: {len(failed)} terms")
            print(f"Failed IDs: {', '.join(failed[:10])}{'...' if len(failed) > 10 else ''}")

        # Save results with metadata wrapper
        print(f"Saving to {output_file}")
        output_data = {
            "metadata": {
                "total_ids": len(go_ids),
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
    id_file = script_dir / "go_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d go test")
        return 1

    extractor = GOExtractor(rate_limit=0.2)
    extractor.extract_all(id_file, output_file)

    return 0


if __name__ == "__main__":
    sys.exit(main())
