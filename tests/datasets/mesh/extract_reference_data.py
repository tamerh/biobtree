#!/usr/bin/env python3
"""
Extract MeSH reference data from ASCII files.

Downloads MeSH ASCII files and extracts COMPLETE term data for test IDs.
Preserves ALL fields from ASCII format for comprehensive testing.
"""

import sys
import json
import requests
from pathlib import Path
from typing import Dict, List, Optional, Any

# MeSH ASCII file URLs (same as in config)
MESH_DESC_URL = "https://nlmpubs.nlm.nih.gov/projects/mesh/MESH_FILES/asciimesh/d2025.bin"
MESH_SUPP_URL = "https://nlmpubs.nlm.nih.gov/projects/mesh/MESH_FILES/asciimesh/c2025.bin"
IDS_FILE = "mesh_ids.txt"
OUTPUT_FILE = "reference_data.json"
DESC_CACHE_FILE = "d2025.bin"
SUPP_CACHE_FILE = "c2025.bin"


def download_mesh_file(url: str, cache_file: str) -> str:
    """Download MeSH file or use cached version"""
    cache_path = Path(cache_file)

    if cache_path.exists():
        print(f"Using cached file: {cache_file}")
        with open(cache_path, 'r', encoding='utf-8') as f:
            return f.read()

    print(f"Downloading from {url}...")
    response = requests.get(url, timeout=300)
    response.raise_for_status()

    content = response.text

    # Cache for future use
    with open(cache_path, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"✓ Downloaded and cached ({len(content)} bytes)")

    return content


def parse_mesh_records(content: str) -> Dict[str, Dict[str, Any]]:
    """
    Parse MeSH ASCII format into dictionary of records keyed by UI.
    Returns dict: {UI: {field: [values]}}
    """
    records = {}
    current_record = None
    current_ui = None

    for line in content.splitlines():
        line = line.strip()

        if line == "*NEWRECORD":
            # Save previous record
            if current_ui and current_record:
                records[current_ui] = current_record
            # Start new record
            current_record = {}
            current_ui = None
            continue

        # Parse field = value
        if " = " in line:
            field, value = line.split(" = ", 1)
            field = field.strip()
            value = value.strip()

            if field == "UI":
                current_ui = value

            # Store all values for each field (some fields can repeat)
            if field not in current_record:
                current_record[field] = []
            current_record[field].append(value)

    # Save last record
    if current_ui and current_record:
        records[current_ui] = current_record

    return records


def extract_entry_terms(entries: List[str]) -> List[str]:
    """Extract entry terms from ENTRY/PRINT ENTRY fields"""
    terms = []
    for entry in entries:
        # Format: "term|semtype|semtype|flags..."
        if "|" in entry:
            term = entry.split("|")[0]
            terms.append(term)
        else:
            terms.append(entry)
    return terms


def extract_pharmacological_actions(actions: List[str]) -> List[str]:
    """Extract pharmacological action IDs"""
    action_ids = []
    for action in actions:
        # Descriptors: plain ID like "D000893"
        # Supplementary: format like "D000894-Anti-Inflammatory Agents, Non-Steroidal"
        if "-" in action:
            action_id = action.split("-")[0]
        else:
            action_id = action
        action_ids.append(action_id)
    return action_ids


def create_reference_entry(ui: str, record: Dict[str, List[str]], is_supplementary: bool) -> Dict[str, Any]:
    """Convert raw record to normalized reference entry"""
    entry = {
        "id": ui,
        "is_supplementary": is_supplementary
    }

    # Name field
    if "MH" in record:  # Descriptor: Main Heading
        entry["name"] = record["MH"][0]
    elif "NM" in record:  # Supplementary: Name
        entry["name"] = record["NM"][0]

    # Descriptor class
    if "DC" in record:
        entry["descriptor_class"] = record["DC"][0]

    # Entry terms (synonyms)
    entry_terms = []
    if "ENTRY" in record:
        entry_terms.extend(extract_entry_terms(record["ENTRY"]))
    if "PRINT ENTRY" in record:
        entry_terms.extend(extract_entry_terms(record["PRINT ENTRY"]))
    if "SY" in record:  # Supplementary synonyms
        entry_terms.extend(extract_entry_terms(record["SY"]))
    if entry_terms:
        entry["entry_terms"] = entry_terms

    # Tree numbers
    if "MN" in record:
        entry["tree_numbers"] = record["MN"]

    # Scope note
    if "MS" in record:
        entry["scope_note"] = record["MS"][0]
    elif "NO" in record:  # Supplementary note
        entry["scope_note"] = record["NO"][0]

    # Annotation
    if "AN" in record:
        entry["annotation"] = record["AN"][0]

    # History note
    if "HN" in record:
        entry["history_note"] = record["HN"][0]

    # Allowable qualifiers
    if "AQ" in record:
        # Format: "AA AD AE AG..."
        qualifiers = record["AQ"][0].split()
        entry["allowable_qualifiers"] = qualifiers

    # Pharmacological actions
    if "PA" in record:
        entry["pharmacological_actions"] = extract_pharmacological_actions(record["PA"])

    # Registry number (supplementary)
    if "RN" in record:
        entry["registry_number"] = record["RN"][0]

    # Heading mapped to (supplementary)
    if "HM" in record:
        # Format: "*D000894-Anti-Inflammatory Agents"
        hm_value = record["HM"][0]
        if "-" in hm_value:
            entry["heading_mapped_to"] = hm_value.split("-")[0].lstrip("*")

    # Date fields
    if "DA" in record:
        entry["date_created"] = record["DA"][0]
    if "DX" in record:
        entry["date_established"] = record["DX"][0]
    if "DR" in record:
        entry["date_revised"] = record["DR"][0]

    return entry


def main():
    """Main extraction function"""
    # Read test IDs
    ids_file = Path(IDS_FILE)
    if not ids_file.exists():
        print(f"Error: {IDS_FILE} not found")
        print("Run: ./biobtree -d mesh test")
        return 1

    with open(ids_file) as f:
        test_ids = [line.strip() for line in f if line.strip()]

    print(f"Found {len(test_ids)} test IDs")
    print(f"ID range: {test_ids[0]} to {test_ids[-1]}")

    # Download and parse descriptor file
    print("\nProcessing descriptors...")
    desc_content = download_mesh_file(MESH_DESC_URL, DESC_CACHE_FILE)
    desc_records = parse_mesh_records(desc_content)
    print(f"✓ Parsed {len(desc_records)} descriptors")

    # Download and parse supplementary file
    print("\nProcessing supplementary concepts...")
    supp_content = download_mesh_file(MESH_SUPP_URL, SUPP_CACHE_FILE)
    supp_records = parse_mesh_records(supp_content)
    print(f"✓ Parsed {len(supp_records)} supplementary concepts")

    # Extract reference data for test IDs
    print("\nExtracting reference data...")
    reference_data = []
    desc_count = 0
    supp_count = 0

    for mesh_id in test_ids:
        if mesh_id in desc_records:
            entry = create_reference_entry(mesh_id, desc_records[mesh_id], False)
            reference_data.append(entry)
            desc_count += 1
        elif mesh_id in supp_records:
            entry = create_reference_entry(mesh_id, supp_records[mesh_id], True)
            reference_data.append(entry)
            supp_count += 1
        else:
            print(f"Warning: {mesh_id} not found in MeSH files")

    print(f"✓ Extracted {len(reference_data)} entries ({desc_count} descriptors, {supp_count} supplementary)")

    # Save reference data
    output_path = Path(OUTPUT_FILE)
    with open(output_path, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"\n✓ Reference data saved to {OUTPUT_FILE}")
    print(f"  Total entries: {len(reference_data)}")
    print(f"  Descriptors: {desc_count}")
    print(f"  Supplementary: {supp_count}")

    # Show sample
    if reference_data:
        sample = reference_data[0]
        print(f"\nSample entry: {sample['id']}")
        print(f"  Name: {sample.get('name', 'N/A')}")
        print(f"  Entry terms: {len(sample.get('entry_terms', []))}")
        print(f"  Tree numbers: {len(sample.get('tree_numbers', []))}")
        print(f"  Pharmacological actions: {len(sample.get('pharmacological_actions', []))}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
