#!/usr/bin/env python3
"""
OBA Reference Data Extraction Script

Fetches full OBA term data from EBI OLS (Ontology Lookup Service) API for each OBA ID.
Creates reference_data.json with complete term information.
"""

import sys
import json
import time
from pathlib import Path
from typing import Optional, Dict
from urllib.parse import quote

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class OBAExtractor:
    """Extract OBA term data from EBI OLS API"""

    BASE_URL = "https://www.ebi.ac.uk/ols/api/ontologies/oba/terms"

    def __init__(self, rate_limit: float = 0.2):
        self.rate_limit = rate_limit
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'BiobtreeTestSuite/1.0'
        })

    def fetch_entry(self, oba_id: str) -> Optional[Dict]:
        """Fetch a single OBA term from OLS API"""
        # OLS expects double-encoded IRI
        iri = f"http://purl.obolibrary.org/obo/{oba_id.replace(':', '_')}"
        encoded_iri = quote(quote(iri, safe=''), safe='')
        url = f"{self.BASE_URL}/{encoded_iri}"

        try:
            response = self.session.get(url, timeout=10)

            if response.status_code == 200:
                data = response.json()

                # Extract relevant fields
                entry = {
                    "id": oba_id,
                    "name": data.get("label", ""),
                    "description": data.get("description", [None])[0] if data.get("description") else None,
                    "type": "biological_attribute"
                }

                # Add synonyms if present
                synonyms = data.get("synonyms", [])
                if synonyms:
                    entry["synonyms"] = synonyms

                # Add is_obsolete flag
                if data.get("is_obsolete"):
                    entry["isObsolete"] = True

                return entry

            elif response.status_code == 404:
                print(f"  Warning: {oba_id} not found (404)")
                return None
            else:
                print(f"  Error: {oba_id} returned status {response.status_code}")
                return None

        except requests.exceptions.Timeout:
            print(f"  Timeout: {oba_id}")
            return None
        except requests.exceptions.RequestException as e:
            print(f"  Error fetching {oba_id}: {e}")
            return None

    def extract_all(self, id_file: Path, output_file: Path):
        """Extract all OBA terms from ID list"""
        print(f"Reading OBA IDs from {id_file}")

        with open(id_file, 'r') as f:
            oba_ids = [line.strip() for line in f if line.strip()]

        print(f"Found {len(oba_ids)} OBA IDs")
        print(f"Fetching from EBI OLS API: {self.BASE_URL}")
        print()

        results = []
        failed = []

        for i, oba_id in enumerate(oba_ids, 1):
            print(f"[{i}/{len(oba_ids)}] Fetching {oba_id}...", end=" ")
            sys.stdout.flush()

            entry = self.fetch_entry(oba_id)

            if entry:
                results.append(entry)
                print("OK")
            else:
                failed.append(oba_id)
                print("FAILED")

            # Rate limiting
            if i < len(oba_ids):
                time.sleep(self.rate_limit)

        print()
        print(f"Successfully fetched: {len(results)}/{len(oba_ids)}")
        if failed:
            print(f"Failed: {len(failed)} terms")
            print(f"Failed IDs: {', '.join(failed[:10])}{'...' if len(failed) > 10 else ''}")

        # Save results with metadata wrapper
        print(f"Saving to {output_file}")
        output_data = {
            "metadata": {
                "total_ids": len(oba_ids),
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
            print(f"  OBA ID:      {first['id']}")
            print(f"  Name:        {first['name']}")
            if first.get('description'):
                desc = first['description'][:70]
                print(f"  Description: {desc}...")
            if first.get('synonyms'):
                print(f"  Synonyms:    {len(first['synonyms'])}")
            print("=" * 60)

        print("Extraction complete")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "oba_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d oba test")
        print("Then copy test_out/reference/oba_ids.txt here")
        return 1

    extractor = OBAExtractor(rate_limit=0.2)
    extractor.extract_all(id_file, output_file)

    return 0


if __name__ == "__main__":
    sys.exit(main())
