#!/usr/bin/env python3
"""
Extract reference data from Protein Similarity test database build.
Reads DIAMOND protein IDs from test_out/reference/protein_similarity_ids.txt
"""

import json
from pathlib import Path

def main():
    script_dir = Path(__file__).parent
    project_root = script_dir.parent.parent.parent
    ids_file = project_root / "test_out" / "reference" / "protein_similarity_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print("Build the test database first: ./biobtree -d protein_similarity test")
        return 1

    # Read DIAMOND protein IDs
    protein_ids = []
    with open(ids_file, 'r') as f:
        for line in f:
            protein_id = line.strip()
            if protein_id:
                protein_ids.append(protein_id)

    if not protein_ids:
        print("Error: No protein IDs found in protein_similarity_ids.txt")
        return 1

    print(f"Found {len(protein_ids)} proteins in test database")

    # Create reference data (first 10 DIAMOND IDs for faster tests)
    reference_data = []
    for protein_id in protein_ids[:10]:
        reference_data.append({
            "diamond_id": protein_id,
            "uniprot_id": protein_id[1:] if protein_id.startswith('d') else protein_id
        })

    # Write reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"✓ Created {output_file} with {len(reference_data)} proteins")
    print(f"  Sample DIAMOND IDs: {', '.join([p['diamond_id'] for p in reference_data[:3]])}")

    return 0


if __name__ == "__main__":
    import sys
    sys.exit(main())
