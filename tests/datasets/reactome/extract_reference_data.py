#!/usr/bin/env python3
"""
Reactome Reference Data Extraction Script

Fetches Reactome pathway data from Reactome Content Service API for each pathway ID.
Creates reference_data.json with complete pathway information for test validation.

Reactome API: https://reactome.org/ContentService/
"""

import json
import sys
import time
from pathlib import Path
from typing import Optional, Dict
import requests


class ReactomeExtractor:
    """Extracts complete Reactome pathway data from official Reactome REST API"""

    BASE_URL = "https://reactome.org/ContentService"

    def __init__(self):
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'Biobtree-Test-Suite/1.0'
        })

    def fetch_entry(self, pathway_id: str) -> Optional[Dict]:
        """
        Fetch COMPLETE Reactome pathway data from REST API

        Uses Reactome Content Service to get:
        1. Pathway details (name, species, etc.)
        2. Participating molecules (proteins, compounds)
        3. Hierarchy relationships

        Returns the full raw API response to preserve all fields
        for maximum test flexibility.
        """
        result = {
            "pathway_id": pathway_id,
            "pathway_details": None,
            "participants": None,
            "hierarchy": None
        }

        # Step 1: Get pathway details
        try:
            detail_url = f"{self.BASE_URL}/data/query/{pathway_id}"
            response = self.session.get(detail_url, timeout=10)

            if response.status_code == 200:
                result["pathway_details"] = response.json()
            else:
                print(f"Warning: HTTP {response.status_code} for pathway details {pathway_id}")
                return None

        except Exception as e:
            print(f"Error fetching pathway details for {pathway_id}: {e}")
            return None

        # Step 2: Get participating molecules
        try:
            time.sleep(0.5)  # Rate limiting
            participants_url = f"{self.BASE_URL}/data/participants/{pathway_id}"
            response = self.session.get(participants_url, timeout=10)

            if response.status_code == 200:
                result["participants"] = response.json()

        except Exception as e:
            print(f"Warning: Could not fetch participants for {pathway_id}: {e}")

        # Step 3: Get hierarchy (parent/child relationships)
        try:
            time.sleep(0.5)  # Rate limiting
            hierarchy_url = f"{self.BASE_URL}/data/pathways/low/diagram/entity/{pathway_id}/allForms"
            response = self.session.get(hierarchy_url, timeout=10)

            if response.status_code == 200:
                result["hierarchy"] = response.json()

        except Exception as e:
            print(f"Warning: Could not fetch hierarchy for {pathway_id}: {e}")

        return result if result["pathway_details"] else None

    def extract_all(self, id_file: Path, output_file: Path, limit: int = None):
        """Extract all pathway entries and save to JSON"""

        # Read IDs
        with open(id_file, 'r') as f:
            ids = [line.strip() for line in f if line.strip()]

        if limit and limit < len(ids):
            print(f"Found {len(ids)} pathway IDs, processing first {limit}")
            ids = ids[:limit]
        else:
            print(f"Found {len(ids)} pathway IDs to process")
        print("=" * 60)

        results = []
        failed = []

        for i, pathway_id in enumerate(ids, 1):
            print(f"[{i}/{len(ids)}] Fetching {pathway_id}...", end=" ", flush=True)

            data = self.fetch_entry(pathway_id)

            if data:
                results.append(data)
                pathway_name = data.get("pathway_details", {}).get("displayName", "Unknown")
                print(f"✓ {pathway_name}")
            else:
                failed.append(pathway_id)
                print("✗ (no data)")

            # Rate limiting - Reactome API is rate-limited
            if i < len(ids):
                time.sleep(1.0)

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
        pathways_with_participants = sum(1 for entry in results if entry.get("participants"))
        pathways_with_hierarchy = sum(1 for entry in results if entry.get("hierarchy"))

        print(f"  Pathways with participants: {pathways_with_participants}")
        print(f"  Pathways with hierarchy: {pathways_with_hierarchy}")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "reactome_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Check prerequisites
    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d 'reactome' test")
        print("Then: cp test_out/reference/reactome_ids.txt tests/reactome/")
        return 1

    # Parse command-line arguments
    limit = int(sys.argv[1]) if len(sys.argv) > 1 else None

    print(f"Reactome Reference Data Extraction")
    print(f"Input:  {id_file}")
    print(f"Output: {output_file}")
    if limit:
        print(f"Limit:  {limit} entries")
    print()

    # Extract data
    extractor = ReactomeExtractor()
    extractor.extract_all(id_file, output_file, limit=limit)

    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("\n\nInterrupted by user")
        sys.exit(1)
