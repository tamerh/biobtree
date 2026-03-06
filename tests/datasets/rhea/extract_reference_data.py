#!/usr/bin/env python3
"""
Extract reference data from Rhea API for test validation

This script fetches complete reaction data from the Rhea REST API for the test IDs.
The data is used to validate biobtree's Rhea integration during testing.

Rhea API: https://www.rhea-db.org/rest/1.0/ws/reaction/{id}
"""

import json
import requests
import time
from pathlib import Path


def extract_rhea_reference_data():
    """Extract reference data from Rhea API for test IDs"""

    script_dir = Path(__file__).parent
    id_file = script_dir / "rhea_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d rhea test")
        print("Then: cp test_out/reference/rhea_ids.txt tests/datasets/rhea/")
        return 1

    # Read test IDs
    with open(id_file, 'r') as f:
        test_ids = [line.strip() for line in f if line.strip()]

    print(f"Extracting reference data for {len(test_ids)} Rhea reactions...")

    reference_data = []

    for i, rhea_id in enumerate(test_ids, 1):
        # Remove RHEA: prefix for API call (API expects numeric ID)
        numeric_id = rhea_id.replace("RHEA:", "")
        api_url = f"https://www.rhea-db.org/rest/1.0/ws/reaction/{numeric_id}"

        try:
            print(f"  [{i}/{len(test_ids)}] Fetching {rhea_id}...", end=" ", flush=True)
            response = requests.get(api_url, timeout=10)

            if response.status_code == 200:
                # Parse XML response and convert to JSON structure
                # For now, store raw response - we'll parse in tests if needed
                data = {
                    "identifier": rhea_id,
                    "api_url": api_url,
                    "status_code": response.status_code,
                    "response": response.text[:500] if response.text else None  # Truncate for storage
                }
                reference_data.append(data)
                print("✓")
            else:
                print(f"Failed (HTTP {response.status_code})")
                # Still add entry to maintain index alignment
                reference_data.append({
                    "identifier": rhea_id,
                    "api_url": api_url,
                    "status_code": response.status_code,
                    "error": f"HTTP {response.status_code}"
                })

            # Rate limiting
            time.sleep(0.1)

        except Exception as e:
            print(f"Error: {e}")
            reference_data.append({
                "identifier": rhea_id,
                "api_url": api_url,
                "error": str(e)
            })

    # Save reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"\nSaved reference data to {output_file}")
    print(f"Successfully extracted {len([d for d in reference_data if d.get('status_code') == 200])}/{len(test_ids)} reactions")

    return 0


if __name__ == "__main__":
    exit(extract_rhea_reference_data())
