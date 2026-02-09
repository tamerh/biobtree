#!/usr/bin/env python3
"""
Extract reference data for SIGNOR test IDs from local TSV files.

Usage: python3 extract_reference_data.py

Reads signor_ids.txt and extracts matching rows from the SIGNOR TSV files.
Saves structured reference data to reference_data.json.
"""

import json
import os
import sys

# SIGNOR data files relative to biobtree root
DATA_FILES = [
    "raw_data/signor/all_data_09_02_26.tsv",  # Human
    "raw_data/signor/all_data_M_musculus_09_02_26.tsv",  # Mouse
    "raw_data/signor/all_data_R_norvegicus_09_02_26.tsv",  # Rat
]

def load_test_ids(filename="signor_ids.txt"):
    """Load test IDs from file."""
    if not os.path.exists(filename):
        print(f"Error: {filename} not found")
        sys.exit(1)

    with open(filename, 'r') as f:
        return set(line.strip() for line in f if line.strip())

def parse_signor_files(data_files, test_ids):
    """Parse SIGNOR TSV files and extract matching entries."""
    results = {}

    for data_file in data_files:
        # Check for file relative to script or biobtree root
        if os.path.exists(data_file):
            filepath = data_file
        else:
            # Try relative to biobtree root (3 levels up from this script)
            filepath = os.path.join(os.path.dirname(__file__), "..", "..", "..", data_file)

        if not os.path.exists(filepath):
            print(f"Warning: {data_file} not found")
            continue

        print(f"Processing {data_file}...")

        with open(filepath, 'r', encoding='utf-8') as f:
            # Read header
            header = f.readline().strip().split('\t')
            col_map = {name: idx for idx, name in enumerate(header)}

            for line in f:
                row = line.strip().split('\t')
                if len(row) < len(header):
                    continue

                signor_id = row[col_map.get("SIGNOR_ID", -1)]
                if signor_id not in test_ids:
                    continue

                # Extract all fields
                entry = {
                    "signor_id": signor_id,
                    "entity_a": row[col_map.get("ENTITYA", -1)] if "ENTITYA" in col_map else "",
                    "type_a": row[col_map.get("TYPEA", -1)] if "TYPEA" in col_map else "",
                    "id_a": row[col_map.get("IDA", -1)] if "IDA" in col_map else "",
                    "database_a": row[col_map.get("DATABASEA", -1)] if "DATABASEA" in col_map else "",
                    "entity_b": row[col_map.get("ENTITYB", -1)] if "ENTITYB" in col_map else "",
                    "type_b": row[col_map.get("TYPEB", -1)] if "TYPEB" in col_map else "",
                    "id_b": row[col_map.get("IDB", -1)] if "IDB" in col_map else "",
                    "database_b": row[col_map.get("DATABASEB", -1)] if "DATABASEB" in col_map else "",
                    "effect": row[col_map.get("EFFECT", -1)] if "EFFECT" in col_map else "",
                    "mechanism": row[col_map.get("MECHANISM", -1)] if "MECHANISM" in col_map else "",
                    "residue": row[col_map.get("RESIDUE", -1)] if "RESIDUE" in col_map else "",
                    "sequence": row[col_map.get("SEQUENCE", -1)] if "SEQUENCE" in col_map else "",
                    "tax_id": row[col_map.get("TAX_ID", -1)] if "TAX_ID" in col_map else "",
                    "cell_data": row[col_map.get("CELL_DATA", -1)] if "CELL_DATA" in col_map else "",
                    "tissue_data": row[col_map.get("TISSUE_DATA", -1)] if "TISSUE_DATA" in col_map else "",
                    "pmid": row[col_map.get("PMID", -1)] if "PMID" in col_map else "",
                    "direct": row[col_map.get("DIRECT", -1)] if "DIRECT" in col_map else "",
                    "score": row[col_map.get("SCORE", -1)] if "SCORE" in col_map else "",
                }

                results[signor_id] = entry

    return results

def main():
    print("Extracting SIGNOR reference data...")

    # Load test IDs
    test_ids = load_test_ids()
    print(f"Loaded {len(test_ids)} test IDs")

    # Parse SIGNOR files
    entries = parse_signor_files(DATA_FILES, test_ids)
    print(f"Found {len(entries)} matching entries")

    # Convert to list format for reference_data.json
    reference_data = list(entries.values())

    # Save to file
    output_file = "reference_data.json"
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(reference_data, f, indent=2, ensure_ascii=False)

    print(f"Saved {len(reference_data)} entries to {output_file}")

    # Print some examples
    if reference_data:
        print("\nSample entry:")
        sample = reference_data[0]
        print(f"  ID: {sample['signor_id']}")
        print(f"  {sample['entity_a']} ({sample['type_a']}) → {sample['entity_b']} ({sample['type_b']})")
        print(f"  Effect: {sample['effect']}")
        print(f"  Mechanism: {sample['mechanism']}")

if __name__ == "__main__":
    main()
