#!/usr/bin/env python3
"""
Extract full UniProt data for reference IDs

This fetches complete UniProt entries for the IDs in uniprot_ids.txt
Used for reference when writing test cases.
"""

import json
import sys
import time
from pathlib import Path
from typing import List, Dict, Optional

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class UniProtExtractor:
    """Extract UniProt reference data from REST API"""

    BASE_URL = "https://rest.uniprot.org/uniprotkb"

    def __init__(self, ids_file: Path, output_file: Path):
        self.ids_file = ids_file
        self.output_file = output_file
        self.stats = {
            "total": 0,
            "success": 0,
            "failed": 0,
            "no_data": 0
        }

    def load_ids(self) -> List[str]:
        """Load UniProt IDs from file"""
        if not self.ids_file.exists():
            raise FileNotFoundError(f"{self.ids_file} not found")

        with open(self.ids_file) as f:
            ids = [line.strip() for line in f if line.strip()]

        self.stats["total"] = len(ids)
        return ids

    def fetch_entry(self, uniprot_id: str) -> Optional[Dict]:
        """Fetch single UniProt entry from API"""
        url = f"{self.BASE_URL}/{uniprot_id}"
        headers = {"Accept": "application/json"}

        try:
            response = requests.get(url, headers=headers, timeout=10)
            response.raise_for_status()

            data = response.json()

            # Check if entry exists
            if data.get("primaryAccession"):
                return data
            else:
                self.stats["no_data"] += 1
                return None

        except requests.RequestException as e:
            print(f"  ✗ Error fetching {uniprot_id}: {e}", file=sys.stderr)
            self.stats["failed"] += 1
            return None

    def extract_all(self) -> List[Dict]:
        """Extract all entries"""
        print("═" * 60)
        print("  UniProt Reference Data Extraction")
        print("═" * 60)
        print()

        # Load IDs
        ids = self.load_ids()
        print(f"Found {len(ids)} UniProt IDs to fetch")
        print()

        # Fetch each entry
        print("Fetching data from UniProt REST API...")
        print()

        entries = []
        for i, uniprot_id in enumerate(ids, 1):
            print(f"[{i:4d}/{len(ids)}] Fetching {uniprot_id} ... ", end="", flush=True)

            entry = self.fetch_entry(uniprot_id)

            if entry:
                print("✓")
                entries.append(entry)
                self.stats["success"] += 1
            elif entry is None and self.stats["no_data"] > self.stats["failed"]:
                print("⚠ No data")
            else:
                print("✗")

            # Rate limiting (be nice to the API)
            time.sleep(0.2)

        return entries

    def save_results(self, entries: List[Dict]):
        """Save results to JSON file"""
        output = {
            "metadata": {
                "total_ids": self.stats["total"],
                "fetched": self.stats["success"],
                "failed": self.stats["failed"],
                "no_data": self.stats["no_data"],
                "date": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
            },
            "entries": entries
        }

        with open(self.output_file, "w") as f:
            json.dump(output, f, indent=2, ensure_ascii=False)

    def print_summary(self):
        """Print extraction summary"""
        print()
        print("═" * 60)
        print("Fetch Summary:")
        print(f"  Total:     {self.stats['total']}")
        print(f"  Success:   {self.stats['success']}")
        print(f"  No data:   {self.stats['no_data']}")
        print(f"  Failed:    {self.stats['failed']}")
        print("═" * 60)
        print()

        if self.stats["success"] > 0:
            size = self.output_file.stat().st_size / 1024
            print(f"✓ Saved to: {self.output_file} ({size:.1f} KB)")
            print()

            # Show sample
            with open(self.output_file) as f:
                data = json.load(f)
                if data["entries"]:
                    first = data["entries"][0]
                    print("Sample entry (first ID):")
                    print(f"  Primary Accession: {first.get('primaryAccession')}")
                    print(f"  Organism:          {first.get('organism', {}).get('scientificName')}")
                    print(f"  Gene Names:        {', '.join([g.get('geneName', {}).get('value', '') for g in first.get('genes', [])])}")
                    print(f"  Protein Names:     {first.get('proteinDescription', {}).get('recommendedName', {}).get('fullName', {}).get('value')}")
                    print(f"  Reviewed:          {first.get('entryType') == 'UniProtKB reviewed (Swiss-Prot)'}")
                    print()

        print("═" * 60)
        print("✓ Reference data extraction complete")
        print()
        print("Files created:")
        print("  - uniprot_ids.txt      (1000 UniProt IDs)")
        print("  - reference_data.json  (Full UniProt entries)")
        print()
        print("Use these files to write test cases in test_cases.json")
        print("═" * 60)


def main():
    # Paths
    script_dir = Path(__file__).parent
    ids_file = script_dir / "uniprot_ids.txt"
    output_file = script_dir / "reference_data.json"

    try:
        extractor = UniProtExtractor(ids_file, output_file)
        entries = extractor.extract_all()

        if entries:
            extractor.save_results(entries)
            extractor.print_summary()
            sys.exit(0)
        else:
            print("\nError: No data fetched successfully")
            sys.exit(1)

    except KeyboardInterrupt:
        print("\n\nInterrupted by user")
        sys.exit(1)
    except Exception as e:
        print(f"\nError: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
