#!/usr/bin/env python3
"""
Extract reference data from IntAct test database build.
Reads protein IDs from test_out/reference/intact_ids.txt
"""

import json
from pathlib import Path

def main():
    script_dir = Path(__file__).parent
    project_root = script_dir.parent.parent.parent
    ids_file = project_root / "test_out" / "reference" / "intact_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print("Build the test database first: ./biobtree -d intact test")
        return 1

    # Read protein IDs
    protein_ids = []
    with open(ids_file, 'r') as f:
        for line in f:
            protein_id = line.strip()
            if protein_id:
                protein_ids.append(protein_id)

    if not protein_ids:
        print("Error: No protein IDs found in intact_ids.txt")
        return 1

    print(f"Found {len(protein_ids)} proteins in test database")

    # Create reference data (first 10 proteins for faster tests)
    reference_data = []
    for protein_id in protein_ids[:10]:
        reference_data.append({
            "protein_id": protein_id
        })

    # Write reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"✓ Created {output_file} with {len(reference_data)} proteins")
    print(f"  Sample proteins: {', '.join([p['protein_id'] for p in reference_data[:3]])}")

    return 0


if __name__ == "__main__":
    import sys
    sys.exit(main())
