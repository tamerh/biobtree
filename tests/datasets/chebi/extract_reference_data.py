#!/usr/bin/env python3
"""
Extract reference data from biobtree API for ChEBI test validation.

This script:
1. Reads test ChEBI IDs from chebi_ids.txt
2. Queries biobtree API for each ID to get complete compound data
3. Saves structured JSON for test validation

IMPORTANT: Uses biobtree API as source of truth since ChEBI now provides
complete compound data including names, structures, formulas, and classifications.
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


def load_test_ids(id_file: Path) -> list:
    """Load ChEBI IDs from the test reference file."""
    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Generate test IDs first by running: ./biobtree -d chebi test")
        print("Then copy test_out/reference/chebi_ids.txt to tests/datasets/chebi/")
        sys.exit(1)

    with open(id_file) as f:
        ids = [line.strip() for line in f if line.strip()]

    print(f"✓ Loaded {len(ids)} test IDs from {id_file}")
    return ids


def fetch_chebi_entry(api_url: str, chebi_id: str) -> dict:
    """
    Fetch complete ChEBI entry from biobtree API.

    Returns None if entry not found or API error.
    """
    try:
        response = requests.get(
            f"{api_url}/ws/entry/",
            params={"i": chebi_id, "s": "chebi"},
            timeout=10
        )

        if response.status_code == 200:
            data = response.json()

            if isinstance(data, list) and len(data) > 0:
                entry = data[0]

                # Check for error response
                if 'Err' in entry:
                    print(f"  ✗ {chebi_id}: {entry['Err']}")
                    return None

                # Verify we got ChEBI attributes
                if 'Attributes' not in entry or 'Chebi' not in entry['Attributes']:
                    print(f"  ✗ {chebi_id}: No ChEBI attributes in response")
                    return None

                print(f"  ✓ {chebi_id}: {entry['Attributes']['Chebi'].get('name', 'N/A')}")
                return entry

        return None

    except Exception as e:
        print(f"  ✗ {chebi_id}: {type(e).__name__}: {e}")
        return None


def extract_reference_data(api_url: str, test_ids: list, max_entries: int = 20) -> dict:
    """
    Extract reference data for test IDs from biobtree API.

    Args:
        api_url: Base URL for biobtree API
        test_ids: List of ChEBI IDs to fetch
        max_entries: Maximum number of entries to include (default: 20)

    Returns:
        Dictionary with metadata and entry list
    """
    print(f"\nFetching up to {max_entries} ChEBI entries from biobtree API...")
    print(f"API: {api_url}")
    print()

    entries = []
    failed = 0

    for i, chebi_id in enumerate(test_ids[:max_entries], 1):
        print(f"[{i}/{min(len(test_ids), max_entries)}] Fetching {chebi_id}...")

        entry = fetch_chebi_entry(api_url, chebi_id)
        if entry:
            entries.append(entry)
        else:
            failed += 1

        # Rate limiting
        if i < min(len(test_ids), max_entries):
            time.sleep(0.1)

    reference_data = {
        "metadata": {
            "source": "biobtree_api",
            "api_base": api_url,
            "total_ids": len(test_ids),
            "requested": max_entries,
            "fetched": len(entries),
            "failed": failed,
            "note": "Reference data extracted from biobtree test database API with full compound attributes"
        },
        "entries": entries
    }

    return reference_data


def main():
    """Main entry point."""
    print("=" * 70)
    print("ChEBI Reference Data Extractor")
    print("=" * 70)

    script_dir = Path(__file__).parent
    id_file = script_dir / "chebi_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Get API URL from command line or use default
    api_url = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:9292"

    # Check if biobtree API is running
    try:
        response = requests.get(f"{api_url}/ws/meta/", timeout=5)
        response.raise_for_status()
        print(f"✓ biobtree API is running at {api_url}\n")
    except Exception as e:
        print(f"✗ biobtree API not accessible at {api_url}")
        print(f"  Error: {e}")
        print("\nPlease ensure biobtree is running:")
        print("  ./biobtree --out-dir test_out web")
        sys.exit(1)

    # Load test IDs
    test_ids = load_test_ids(id_file)

    # Extract reference data (limit to 20 for test suite)
    reference_data = extract_reference_data(api_url, test_ids, max_entries=20)

    # Save to file
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    file_size = output_file.stat().st_size / 1024

    print()
    print("=" * 70)
    print(f"✓ Reference data saved to {output_file.absolute()} ({file_size:.1f} KB)")
    print(f"  Total entries: {reference_data['metadata']['fetched']}")
    print(f"  Failed: {reference_data['metadata']['failed']}")
    print("=" * 70)
    print("✓ Extraction complete")

    return 0


if __name__ == "__main__":
    sys.exit(main())
