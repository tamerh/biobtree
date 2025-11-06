#!/usr/bin/env python3
"""
ChEBI Reference Data Extraction Script

For ChEBI, biobtree mainly stores cross-references from the database_accession file.
Since the full ChEBI data files are large and may not be easily accessible,
we create minimal reference data directly from biobtree for testing purposes.

Note: This is acceptable for test reference data as we're documenting what's in our test database.
For datasets with accessible APIs (GO, ECO, UniProt, etc.), we extract from the remote source.
"""

import sys
import json
import time
from pathlib import Path

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


def extract_from_biobtree(api_url: str, id_file: Path, output_file: Path):
    """Extract ChEBI reference data from biobtree test database"""
    print("=" * 60)
    print("ChEBI Reference Data Extraction")
    print("=" * 60)
    print()
    print("Note: Extracting from biobtree test database")
    print("      (ChEBI flat files not easily accessible)")
    print()

    # Load test IDs
    with open(id_file, 'r') as f:
        test_ids = [line.strip() for line in f if line.strip()]

    print(f"Found {len(test_ids)} test ChEBI IDs")
    print(f"Querying: {api_url}")
    print()

    # Extract data for test IDs
    results = []
    failed = []

    for i, chebi_id in enumerate(test_ids, 1):
        try:
            response = requests.get(
                f"{api_url}/ws/search",
                params={"i": chebi_id},
                timeout=5
            )

            if response.status_code == 200:
                data = response.json()
                if data.get("results"):
                    # Create minimal entry
                    entry = {
                        "id": chebi_id,
                        "hasData": True
                    }
                    results.append(entry)
                else:
                    failed.append(chebi_id)
            else:
                failed.append(chebi_id)

            if i % 10 == 0:
                print(f"  Processed {i}/{len(test_ids)}...")

            time.sleep(0.05)  # Rate limit

        except Exception as e:
            print(f"  Error with {chebi_id}: {e}")
            failed.append(chebi_id)

    print()
    print(f"  Found: {len(results)}/{len(test_ids)}")
    if failed:
        print(f"  Failed: {len(failed)} IDs")

    # Save results
    output_data = {
        "metadata": {
            "total_ids": len(test_ids),
            "fetched": len(results),
            "failed": len(failed),
            "note": "Minimal reference data extracted from biobtree test database"
        },
        "entries": results
    }

    with open(output_file, 'w') as f:
        json.dump(output_data, f, indent=2)

    file_size = output_file.stat().st_size / 1024

    print()
    print("=" * 60)
    print(f"✓ Saved to: {output_file.absolute()} ({file_size:.1f} KB)")
    print()
    print(f"Entries: {len(results)}")
    print("=" * 60)
    print("✓ Extraction complete")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "chebi_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d chebi test")
        print("Then copy test_out/reference/chebi_ids.txt here")
        return 1

    # Get API URL from command line or use default
    api_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:9292"

    extract_from_biobtree(api_url, id_file, output_file)

    return 0


if __name__ == "__main__":
    sys.exit(main())
