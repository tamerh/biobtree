#!/usr/bin/env python3
"""
Extract reference data for Clinical Trials tests.

Reads trials from local test_data/clinical_trials/trials.json file
and creates reference_data.json with complete trial information.
"""

import json
from pathlib import Path

def main():
    # Paths
    script_dir = Path(__file__).parent
    ids_file = script_dir / "clinical_trials_ids.txt"
    source_file = script_dir.parent.parent / "test_data" / "clinical_trials" / "trials.json"
    output_file = script_dir / "reference_data.json"

    # Read trial IDs
    print(f"Reading trial IDs from {ids_file}")
    with open(ids_file) as f:
        trial_ids = set(line.strip() for line in f if line.strip())

    print(f"Looking for {len(trial_ids)} trial IDs: {sorted(trial_ids)}")

    # Read trials from source file
    print(f"\nReading trials from {source_file}")
    with open(source_file) as f:
        all_trials = json.load(f)

    print(f"Found {len(all_trials)} total trials in source file")

    # Extract matching trials
    reference_data = []
    found_ids = set()

    for trial in all_trials:
        nct_id = trial.get("nct_id", "")
        if nct_id in trial_ids:
            reference_data.append(trial)
            found_ids.add(nct_id)
            print(f"  ✓ Found {nct_id}: {trial.get('brief_title', '')[:60]}...")

    # Check for missing IDs
    missing_ids = trial_ids - found_ids
    if missing_ids:
        print(f"\n⚠ Warning: {len(missing_ids)} trial IDs not found: {sorted(missing_ids)}")

    # Write reference data
    print(f"\nWriting {len(reference_data)} trials to {output_file}")
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"✓ Reference data extracted successfully")
    print(f"  Total trials: {len(reference_data)}")
    print(f"  Output: {output_file}")

if __name__ == "__main__":
    main()
