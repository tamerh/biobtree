#!/usr/bin/env python3
"""
Extract reference data for MSigDB test entries.

Reads the MSigDB IDs generated during test build and extracts reference
data from the index files.
"""

import json
import gzip
from pathlib import Path


def extract_reference_data():
    """Extract reference data from MSigDB index files"""
    script_dir = Path(__file__).parent
    ids_file = script_dir / "msigdb_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Default location after test build
    default_ids_location = script_dir.parent.parent.parent / "test_out" / "reference" / "msigdb_ids.txt"

    # Check if IDs file exists locally, if not, try to copy from test output
    if not ids_file.exists():
        if default_ids_location.exists():
            print(f"Copying IDs from {default_ids_location}")
            with open(default_ids_location) as src, open(ids_file, 'w') as dst:
                dst.write(src.read())
        else:
            print(f"Error: {ids_file} not found")
            print("Run: ./biobtree -d msigdb test")
            print("Then copy test_out/reference/msigdb_ids.txt here")
            return 1

    # Read test IDs
    with open(ids_file) as f:
        test_ids = set(line.strip() for line in f if line.strip())

    print(f"Found {len(test_ids)} test IDs")

    # Find MSigDB index files
    index_dir = script_dir.parent.parent.parent / "test_out" / "index"
    index_files = list(index_dir.glob("msigdb_sorted.*.index.gz"))

    if not index_files:
        print(f"Error: No MSigDB index files found in {index_dir}")
        print("Run: ./biobtree -d msigdb test")
        return 1

    print(f"Found {len(index_files)} index files")

    # Extract entries from index files
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
        print(f"  systematic_name: {sample.get('systematic_name', 'N/A')}")
        print(f"  standard_name: {sample.get('standard_name', 'N/A')}")
        print(f"  collection: {sample.get('collection', 'N/A')}")
        print(f"  gene_count: {sample.get('gene_count', 'N/A')}")
        print(f"  gene_symbols: {len(sample.get('gene_symbols', []))} genes")

    return 0


if __name__ == "__main__":
    exit(extract_reference_data())
