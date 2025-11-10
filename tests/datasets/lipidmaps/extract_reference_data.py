#!/usr/bin/env python3
"""
Extract reference data from LIPID MAPS SDF file

Downloads the full LMSD SDF file and extracts records for test IDs.
Saves the raw SDF data as JSON for test validation.

Usage:
    python3 extract_reference_data.py

Reads lipidmaps_ids.txt and outputs reference_data.json
"""

import json
import sys
import io
import zipfile
from pathlib import Path

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


def parse_sdf_record(sdf_text):
    """Parse a single SDF record into a dictionary."""
    data = {}
    lines = sdf_text.strip().split('\n')

    current_field = None
    current_value = []
    in_mol_block = True

    for line in lines:
        # End of MOL block
        if line.strip() == 'M  END':
            in_mol_block = False
            continue

        # Skip MOL block lines
        if in_mol_block:
            continue

        # Record delimiter - stop parsing
        if line.strip() == '$$$$':
            break

        # Field name line: > <FIELD_NAME>
        if line.startswith('> <') and line.endswith('>'):
            # Save previous field if exists
            if current_field:
                value = '\n'.join(current_value).strip()
                if value:  # Only save non-empty values
                    data[current_field] = value

            # Extract new field name
            current_field = line[3:-1]  # Remove '> <' and '>'
            current_value = []

        elif current_field and not line.startswith('>'):
            # Value line for current field (including empty lines between fields)
            if line.strip():  # Only add non-empty lines to value
                current_value.append(line.strip())

    # Save last field
    if current_field:
        value = '\n'.join(current_value).strip()
        if value:
            data[current_field] = value

    # Parse synonyms into list if present
    if 'SYNONYMS' in data and data['SYNONYMS']:
        # Synonyms are semicolon-delimited
        synonyms = [s.strip() for s in data['SYNONYMS'].split(';') if s.strip()]
        data['SYNONYMS'] = synonyms

    return data


def extract_sdf_records(sdf_content, target_ids):
    """Extract specific records from SDF content.

    Returns:
        tuple: (parsed_records, raw_records, found_ids)
            - parsed_records: list of dictionaries with parsed data
            - raw_records: list of raw SDF text blocks
            - found_ids: set of found LM_IDs
    """
    parsed_records = []
    raw_records = []
    current_record = []
    target_ids_set = set(target_ids)
    found_ids = set()

    for line in sdf_content.split('\n'):
        current_record.append(line)

        # Record delimiter
        if line.strip() == '$$$$':
            # Get raw record text
            record_text = '\n'.join(current_record)

            # Parse this record
            record_data = parse_sdf_record(record_text)

            # Check if this is a target ID
            lm_id = record_data.get('LM_ID')
            if lm_id in target_ids_set:
                parsed_records.append(record_data)
                raw_records.append(record_text)
                found_ids.add(lm_id)
                print(f"  ✓ Found {lm_id} - {record_data.get('NAME', 'Unknown')}")

            # Reset for next record
            current_record = []

            # Early exit if we found all IDs
            if len(found_ids) == len(target_ids):
                break

    return parsed_records, raw_records, found_ids


def download_and_extract_sdf():
    """Download LIPID MAPS SDF ZIP file and extract content."""
    sdf_url = "https://www.lipidmaps.org/files/?file=LMSD&ext=sdf.zip"

    print(f"Downloading LIPID MAPS SDF from: {sdf_url}")
    print("This may take a moment (~20 MB download)...")

    try:
        response = requests.get(sdf_url, timeout=120)
        response.raise_for_status()

        print(f"Downloaded {len(response.content)} bytes")

        # Open ZIP file from memory
        with zipfile.ZipFile(io.BytesIO(response.content)) as zf:
            # Find the SDF file in the ZIP
            sdf_files = [f for f in zf.namelist() if f.endswith('.sdf')]

            if not sdf_files:
                print("Error: No .sdf file found in ZIP archive")
                return None

            sdf_filename = sdf_files[0]
            print(f"Extracting {sdf_filename} from ZIP...")

            # Read SDF content
            with zf.open(sdf_filename) as f:
                sdf_content = f.read().decode('utf-8')

            print(f"SDF file size: {len(sdf_content)} bytes")
            return sdf_content

    except requests.exceptions.RequestException as e:
        print(f"Error downloading SDF file: {e}")
        return None
    except zipfile.BadZipFile as e:
        print(f"Error reading ZIP file: {e}")
        return None


def main():
    """Main extraction function."""
    # Read test IDs
    ids_file = Path(__file__).parent / "lipidmaps_ids.txt"

    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print("\nGenerate test IDs first:")
        print("  cd /path/to/biobtreev2")
        print("  ./biobtree -d lipidmaps test")
        print("  cp test_out/reference/lipidmaps_ids.txt tests/datasets/lipidmaps/")
        sys.exit(1)

    # Read IDs (one per line)
    with open(ids_file) as f:
        ids = [line.strip() for line in f if line.strip() and not line.startswith('#')]

    if not ids:
        print(f"Error: No IDs found in {ids_file}")
        sys.exit(1)

    print(f"Target: Extract reference data for {len(ids)} LIPID MAPS IDs")
    print(f"Source: LIPID MAPS SDF file (full download)")
    print()

    # Download and extract SDF file
    sdf_content = download_and_extract_sdf()

    if not sdf_content:
        print("\nError: Failed to download/extract SDF file")
        sys.exit(1)

    print()
    print("Extracting records for test IDs...")
    print()

    # Extract records for our test IDs
    parsed_records, raw_records, found_ids = extract_sdf_records(sdf_content, ids)

    # Check for missing IDs
    missing_ids = set(ids) - found_ids
    if missing_ids:
        print()
        print(f"Warning: {len(missing_ids)} IDs not found in SDF file:")
        for lm_id in sorted(missing_ids):
            print(f"  - {lm_id}")

    if not parsed_records:
        print("\nError: No reference data extracted")
        sys.exit(1)

    # Save parsed data to JSON
    json_file = Path(__file__).parent / "reference_data.json"
    with open(json_file, 'w') as f:
        json.dump(parsed_records, f, indent=2)

    # Save raw SDF records to file
    sdf_file = Path(__file__).parent / "reference_data_raw.sdf"
    with open(sdf_file, 'w') as f:
        f.write('\n'.join(raw_records))

    print()
    print(f"✓ Saved {len(parsed_records)} entries to {json_file}")
    print(f"✓ Saved raw SDF records to {sdf_file}")
    print()
    print("Summary:")
    print(f"  - Target IDs: {len(ids)}")
    print(f"  - Found: {len(found_ids)}")
    print(f"  - Missing: {len(missing_ids)}")
    print()
    print("Files created:")
    print(f"  - reference_data.json       : Parsed data for tests")
    print(f"  - reference_data_raw.sdf    : Original SDF format (complete reference)")
    print()
    print("Next steps:")
    print("  1. Review reference_data.json and reference_data_raw.sdf")
    print("  2. Update test_cases.json if needed")
    print("  3. Run tests: python3 tests/run_tests.py lipidmaps")


if __name__ == "__main__":
    main()
