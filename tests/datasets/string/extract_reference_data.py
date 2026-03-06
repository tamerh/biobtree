#!/usr/bin/env python3
"""
STRING Reference Data Extraction Script

Fetches STRING protein-protein interaction data from STRING API for each UniProt ID.
Creates reference_data.json with complete interaction information for test validation.

STRING API: https://string-db.org/api
"""

import json
import sys
import time
from pathlib import Path
from typing import Optional, Dict, List
import requests


class StringExtractor:
    """Extracts complete STRING interaction data from official STRING REST API"""

    BASE_URL = "https://string-db.org/api"
    SPECIES_ID = "9606"  # Human

    def __init__(self):
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'Biobtree-Test-Suite/1.0'
        })

    def fetch_entry(self, uniprot_id: str) -> Optional[Dict]:
        """
        Fetch COMPLETE STRING interaction data from REST API

        Uses STRING API to:
        1. Map UniProt ID to STRING ID
        2. Get interaction partners
        3. Get protein information

        Returns the full raw API response to preserve all fields
        for maximum test flexibility.
        """
        result = {
            "uniprot_id": uniprot_id,
            "string_mapping": None,
            "interactions": None,
            "protein_info": None
        }

        # Step 1: Map UniProt ID to STRING ID
        try:
            map_url = f"{self.BASE_URL}/json/get_string_ids"
            params = {
                "identifiers": uniprot_id,
                "species": self.SPECIES_ID,
                "limit": 1
            }
            response = self.session.post(map_url, data=params, timeout=10)

            if response.status_code == 200:
                mapping_data = response.json()
                if mapping_data:
                    result["string_mapping"] = mapping_data[0]
                    string_id = mapping_data[0].get("stringId")

                    # Step 2: Get interaction partners
                    if string_id:
                        partners_url = f"{self.BASE_URL}/json/interaction_partners"
                        params = {
                            "identifiers": string_id,
                            "species": self.SPECIES_ID,
                            "required_score": 400  # Match our threshold
                        }
                        partners_response = self.session.post(partners_url, data=params, timeout=15)

                        if partners_response.status_code == 200:
                            result["interactions"] = partners_response.json()

                        # Step 3: Get protein enrichment/info (optional)
                        # This provides additional context about the protein
                        time.sleep(0.5)  # Rate limiting

                else:
                    print(f"Warning: No STRING mapping found for {uniprot_id}")
                    return None
            else:
                print(f"Warning: HTTP {response.status_code} for {uniprot_id}")
                return None

        except Exception as e:
            print(f"Error fetching {uniprot_id}: {e}")
            return None

        # Only return if we got some interaction data
        if result["interactions"]:
            return result
        else:
            print(f"Warning: No interactions found for {uniprot_id}")
            return None

    def extract_all(self, id_file: Path, output_file: Path, limit: int = None):
        """Extract all protein entries and save to JSON"""

        # Read IDs
        with open(id_file, 'r') as f:
            ids = [line.strip() for line in f if line.strip()]

        if limit and limit < len(ids):
            print(f"Found {len(ids)} UniProt IDs, processing first {limit}")
            ids = ids[:limit]
        else:
            print(f"Found {len(ids)} UniProt IDs to process")
        print("=" * 60)

        results = []
        failed = []

        for i, uniprot_id in enumerate(ids, 1):
            print(f"[{i}/{len(ids)}] Fetching {uniprot_id}...", end=" ", flush=True)

            data = self.fetch_entry(uniprot_id)

            if data:
                results.append(data)
                interaction_count = len(data.get("interactions", []))
                print(f"✓ ({interaction_count} interactions)")
            else:
                failed.append(uniprot_id)
                print("✗ (no data)")

            # Rate limiting - STRING allows up to 1 request/sec for sustained use
            if i < len(ids):
                time.sleep(1.2)

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
        total_interactions = sum(len(entry.get("interactions", [])) for entry in results)
        avg_interactions = total_interactions / len(results) if results else 0

        print(f"  Total interactions: {total_interactions}")
        print(f"  Average interactions per protein: {avg_interactions:.1f}")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "string_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Check prerequisites
    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d 'string,uniprot' --tax 9606 test")
        print("Then: cp test_out/reference/string_ids.txt tests/string/")
        return 1

    # Parse command-line arguments
    limit = int(sys.argv[1]) if len(sys.argv) > 1 else None

    print(f"STRING Reference Data Extraction")
    print(f"Input:  {id_file}")
    print(f"Output: {output_file}")
    if limit:
        print(f"Limit:  {limit} entries")
    print()

    # Extract data
    extractor = StringExtractor()
    extractor.extract_all(id_file, output_file, limit=limit)

    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("\n\nInterrupted by user")
        sys.exit(1)
