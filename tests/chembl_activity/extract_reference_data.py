#!/usr/bin/env python3
"""
Extract ChEMBL Activity Reference Data

Fetches activity data from ChEMBL REST API for test IDs.
This creates reference data for validating biobtree's activity processing.

Usage:
    python3 extract_reference_data.py
"""

import json
import sys
from pathlib import Path
from time import sleep

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


CHEMBL_API_BASE = "https://www.ebi.ac.uk/chembl/api/data"
IDS_FILE = "chembl_activity_ids.txt"
OUTPUT_FILE = "reference_data.json"


def fetch_activity(activity_id: str) -> dict:
    """Fetch activity data from ChEMBL API"""
    # Remove "CHEMBL_ACT_" prefix if present
    if activity_id.startswith("CHEMBL_ACT_"):
        activity_id = activity_id.replace("CHEMBL_ACT_", "")

    url = f"{CHEMBL_API_BASE}/activity/{activity_id}.json"

    try:
        response = requests.get(url, timeout=10)

        if response.status_code == 200:
            return response.json()
        elif response.status_code == 404:
            print(f"  Warning: Activity {activity_id} not found in API")
            return None
        else:
            print(f"  Warning: API returned status {response.status_code} for {activity_id}")
            return None

    except requests.exceptions.RequestException as e:
        print(f"  Error fetching {activity_id}: {e}")
        return None


def main():
    script_dir = Path(__file__).parent
    ids_file = script_dir / IDS_FILE
    output_file = script_dir / OUTPUT_FILE

    if not ids_file.exists():
        print(f"Error: {IDS_FILE} not found")
        print("Run: ./biobtree -d chembl_activity test")
        return 1

    # Read activity IDs
    with open(ids_file) as f:
        activity_ids = [line.strip() for line in f if line.strip()]

    print(f"Fetching reference data for {len(activity_ids)} activities from ChEMBL API...")
    print(f"API: {CHEMBL_API_BASE}")
    print()

    reference_data = []

    for i, activity_id in enumerate(activity_ids, 1):
        print(f"[{i}/{len(activity_ids)}] Fetching {activity_id}...")

        data = fetch_activity(activity_id)

        if data:
            # Add full ChEMBL activity ID to match database format
            data['activity_chembl_id'] = f"CHEMBL_ACT_{data['activity_id']}"
            reference_data.append(data)

        # Rate limiting: be nice to ChEMBL API
        if i < len(activity_ids):
            sleep(0.5)

    print()
    print(f"Successfully fetched {len(reference_data)}/{len(activity_ids)} activities")

    # Save reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    file_size = output_file.stat().st_size / 1024
    print(f"Reference data saved to: {output_file}")
    print(f"File size: {file_size:.1f} KB")

    return 0


if __name__ == "__main__":
    sys.exit(main())
