#!/usr/bin/env python3
"""
Extract reference data for CTD test entries.

Reads the CTD IDs generated during test build and extracts reference
data from the index files or by querying the biobtree API.
"""

import json
import gzip
import re
from pathlib import Path


def extract_reference_data():
    """Extract reference data from CTD index files"""
    script_dir = Path(__file__).parent
    ids_file = script_dir / "ctd_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Default location after test build
    default_ids_location = script_dir.parent.parent.parent / "test_out" / "reference" / "ctd_ids.txt"

    # Check if IDs file exists locally, if not, try to copy from test output
    if not ids_file.exists():
        if default_ids_location.exists():
            print(f"Copying IDs from {default_ids_location}")
            with open(default_ids_location) as src, open(ids_file, 'w') as dst:
                dst.write(src.read())
        else:
            print(f"Error: {ids_file} not found")
            print("Run: ./biobtree -d ctd test")
            print("Then copy test_out/reference/ctd_ids.txt here")
            return 1

    # Read test IDs
    with open(ids_file) as f:
        test_ids = set(line.strip() for line in f if line.strip())

    print(f"Found {len(test_ids)} test IDs")

    # Find CTD index files
    index_dir = script_dir.parent.parent.parent / "test_out" / "index"
    index_files = list(index_dir.glob("ctd_sorted.*.index.gz"))

    if not index_files:
        print(f"Error: No CTD index files found in {index_dir}")
        print("Run: ./biobtree -d ctd test")
        return 1

    print(f"Found {len(index_files)} index files")

    # Extract entries from index files
    reference_data = []
    entries_by_id = {}

    for index_file in index_files:
        print(f"Reading {index_file.name}...")
        with gzip.open(index_file, 'rt', encoding='utf-8') as f:
            for line in f:
                parts = line.strip().split('\t')
                if len(parts) < 3:
                    continue

                entry_id = parts[0]
                if entry_id not in test_ids:
                    continue

                # Check if this is an attribute line (contains JSON)
                value = parts[2]
                if value.startswith('{') and value.endswith('}'):
                    try:
                        attrs = json.loads(value)
                        if entry_id not in entries_by_id:
                            entries_by_id[entry_id] = attrs
                        else:
                            # Merge attributes
                            entries_by_id[entry_id].update(attrs)
                    except json.JSONDecodeError:
                        pass

    # Convert to list
    reference_data = list(entries_by_id.values())
    print(f"Extracted {len(reference_data)} entries with attributes")

    # Write reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"Saved to {output_file}")

    # Print sample
    if reference_data:
        sample = reference_data[0]
        print("\nSample entry:")
        print(f"  chemical_id: {sample.get('chemical_id', 'N/A')}")
        print(f"  chemical_name: {sample.get('chemical_name', 'N/A')[:60]}...")
        print(f"  gene_interactions: {len(sample.get('gene_interactions', []))} entries")
        print(f"  disease_associations: {len(sample.get('disease_associations', []))} entries")

    return 0


if __name__ == "__main__":
    exit(extract_reference_data())
