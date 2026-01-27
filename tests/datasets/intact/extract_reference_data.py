#!/usr/bin/env python3
"""
Extract reference data from IntAct test database build.
Reads interaction IDs from test_out/reference/intact_ids.txt

New interaction-centric model:
- Entries are keyed by interaction ID (EBI-xxx)
- Each interaction contains data for both proteins
- Cross-references link proteins to their interactions
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

    # Read interaction IDs
    interaction_ids = []
    with open(ids_file, 'r') as f:
        for line in f:
            interaction_id = line.strip()
            if interaction_id:
                interaction_ids.append(interaction_id)

    if not interaction_ids:
        print("Error: No interaction IDs found in intact_ids.txt")
        return 1

    print(f"Found {len(interaction_ids)} interactions in test database")

    # Create reference data (first 20 interactions for comprehensive tests)
    reference_data = []
    for interaction_id in interaction_ids[:20]:
        reference_data.append({
            "interaction_id": interaction_id
        })

    # Write reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"Created {output_file} with {len(reference_data)} interactions")
    print(f"  Sample interactions: {', '.join([i['interaction_id'] for i in reference_data[:3]])}")

    return 0


if __name__ == "__main__":
    import sys
    sys.exit(main())
