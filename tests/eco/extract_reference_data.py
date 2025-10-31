#!/usr/bin/env python3
"""
ECO Reference Data Extraction Script

Fetches full ECO term data from EBI OLS (Ontology Lookup Service) API for each ECO ID.
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


class ECOExtractor:
    """Extract ECO term data from EBI OLS API"""

    BASE_URL = "https://www.ebi.ac.uk/ols/api/ontologies/eco/terms"

    def __init__(self, rate_limit: float = 0.2):
        self.rate_limit = rate_limit
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'BiobtreeTestSuite/1.0'
        })

    def fetch_entry(self, eco_id: str) -> Optional[Dict]:
        """Fetch a single ECO term from OLS API"""
        # OLS expects double-encoded IRI
        iri = f"http://purl.obolibrary.org/obo/{eco_id.replace(':', '_')}"
        encoded_iri = quote(quote(iri, safe=''), safe='')
        url = f"{self.BASE_URL}/{encoded_iri}"

        try:
            response = self.session.get(url, timeout=10)

            if response.status_code == 200:
                data = response.json()

                # Extract relevant fields
                entry = {
                    "id": eco_id,
                    "name": data.get("label", ""),
                    "description": data.get("description", [None])[0] if data.get("description") else None,
                    "type": "eco"
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
                print(f"  Warning: {eco_id} not found (404)")
                return None
            else:
                print(f"  Error: {eco_id} returned status {response.status_code}")
                return None

        except requests.exceptions.Timeout:
            print(f"  Timeout: {eco_id}")
            return None
        except requests.exceptions.RequestException as e:
            print(f"  Error fetching {eco_id}: {e}")
            return None

    def extract_all(self, id_file: Path, output_file: Path):
        """Extract all ECO terms from ID list"""
        print(f"Reading ECO IDs from {id_file}")

        with open(id_file, 'r') as f:
            eco_ids = [line.strip() for line in f if line.strip()]

        print(f"Found {len(eco_ids)} ECO IDs")
        print(f"Fetching from EBI OLS API: {self.BASE_URL}")
        print()

        results = []
        failed = []

        for i, eco_id in enumerate(eco_ids, 1):
            print(f"[{i}/{len(eco_ids)}] Fetching {eco_id}...", end=" ")
            sys.stdout.flush()

            entry = self.fetch_entry(eco_id)

            if entry:
                results.append(entry)
                print("✓")
            else:
                failed.append(eco_id)
                print("✗")

            # Rate limiting
            if i < len(eco_ids):
                time.sleep(self.rate_limit)

        print()
        print(f"Successfully fetched: {len(results)}/{len(eco_ids)}")
        if failed:
            print(f"Failed: {len(failed)} terms")
            print(f"Failed IDs: {', '.join(failed[:10])}{'...' if len(failed) > 10 else ''}")

        # Save results with metadata wrapper
        print(f"Saving to {output_file}")
        output_data = {
            "metadata": {
                "total_ids": len(eco_ids),
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
            print(f"  ECO ID:      {first['id']}")
            print(f"  Name:        {first['name']}")
            if first.get('description'):
                desc = first['description'][:70]
                print(f"  Description: {desc}...")
            if first.get('synonyms'):
                print(f"  Synonyms:    {len(first['synonyms'])}")
            print("=" * 60)

        print("✓ Extraction complete")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "eco_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d eco test")
        print("Then copy test_out/reference/eco_ids.txt here")
        return 1

    extractor = ECOExtractor(rate_limit=0.2)
    extractor.extract_all(id_file, output_file)

    return 0


if __name__ == "__main__":
    sys.exit(main())
