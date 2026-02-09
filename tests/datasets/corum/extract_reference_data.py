#!/usr/bin/env python3
"""
Extract reference data from CORUM source files for testing.

This script reads the CORUM source JSON file and extracts sample entries
for use as reference data in tests.
"""

import json
import os
import random
from pathlib import Path


def extract_reference_data():
    """Extract sample entries from CORUM source file."""

    # Source file path
    source_file = Path("/data/bioyoda/biobtreev2/raw_data/CORUM/corum_allComplexes.json")

    if not source_file.exists():
        print(f"Error: Source file not found: {source_file}")
        return None

    print(f"Reading source file: {source_file}")

    with open(source_file, 'r') as f:
        data = json.load(f)

    print(f"Total entries: {len(data)}")

    # Select diverse sample entries
    # Include first few entries for consistent testing
    sample_entries = data[:10]

    # Add some random entries for diversity
    random.seed(42)  # For reproducibility
    if len(data) > 20:
        random_indices = random.sample(range(10, len(data)), min(90, len(data) - 10))
        for idx in random_indices:
            sample_entries.append(data[idx])

    print(f"Selected {len(sample_entries)} sample entries")

    # Show sample of what we're extracting
    if sample_entries:
        first = sample_entries[0]
        print(f"\nFirst entry preview:")
        print(f"  complex_id: {first.get('complex_id')}")
        print(f"  complex_name: {first.get('complex_name')}")
        print(f"  organism: {first.get('organism')}")
        print(f"  subunits: {len(first.get('subunits', []))}")
        print(f"  functions: {len(first.get('functions', []))}")

    return sample_entries


def main():
    """Main entry point."""
    script_dir = Path(__file__).parent
    output_file = script_dir / "reference_data.json"

    entries = extract_reference_data()

    if entries:
        with open(output_file, 'w') as f:
            json.dump(entries, f, indent=2)

        print(f"\nReference data written to: {output_file}")
        print(f"Entries saved: {len(entries)}")
    else:
        print("No entries extracted")
        return 1

    return 0


if __name__ == "__main__":
    exit(main())
