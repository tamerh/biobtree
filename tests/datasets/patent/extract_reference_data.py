#!/usr/bin/env python3
"""
Extract Patent reference data from local JSON files.

Reads patent data from test_data/surechembl/ directory and extracts
COMPLETE data for test patent IDs.
"""

import sys
import json
from pathlib import Path
from typing import Dict, List

# Paths
IDS_FILE = "patent_ids.txt"
OUTPUT_FILE = "reference_data.json"
PATENTS_FILE = "../../test_data/surechembl/patents.json"
COMPOUNDS_FILE = "../../test_data/surechembl/compounds.json"
MAPPING_FILE = "../../test_data/surechembl/mapping.json"


def load_patents(target_ids: set) -> Dict[str, dict]:
    """Load patent data from JSON file"""
    patents_path = Path(PATENTS_FILE)
    if not patents_path.exists():
        print(f"Error: {PATENTS_FILE} not found")
        return {}

    with open(patents_path, 'r') as f:
        data = json.load(f)

    patents = {}
    for patent in data.get('patents', []):
        patent_number = patent.get('patent_number')
        if patent_number in target_ids:
            patents[patent_number] = patent

    print(f"Loaded {len(patents)}/{len(target_ids)} patents")
    return patents


def load_compounds() -> Dict[str, dict]:
    """Load compound data from JSON file"""
    compounds_path = Path(COMPOUNDS_FILE)
    if not compounds_path.exists():
        print(f"Warning: {COMPOUNDS_FILE} not found")
        return {}

    with open(compounds_path, 'r') as f:
        data = json.load(f)

    compounds = {}
    for compound in data.get('compounds', []):
        compound_id = str(compound.get('id'))
        if compound_id:
            compounds[compound_id] = compound

    print(f"Loaded {len(compounds)} compounds")
    return compounds


def load_mappings(target_patent_ids: set, patents_by_number: Dict[str, dict]) -> Dict[str, List[str]]:
    """Load patent-compound mappings from JSON file"""
    mapping_path = Path(MAPPING_FILE)
    if not mapping_path.exists():
        print(f"Warning: {MAPPING_FILE} not found")
        return {}

    # Build patent ID → patent_number map
    patent_id_map = {}
    for patent in patents_by_number.values():
        patent_id = str(patent.get('id'))
        patent_number = patent.get('patent_number')
        if patent_id and patent_number:
            patent_id_map[patent_id] = patent_number

    with open(mapping_path, 'r') as f:
        data = json.load(f)

    # Map patent_number → [compound_ids]
    patent_compounds = {}
    for mapping in data.get('mappings', []):
        patent_id = str(mapping.get('patent_id'))
        compound_id = str(mapping.get('compound_id'))

        if patent_id in patent_id_map:
            patent_number = patent_id_map[patent_id]
            if patent_number in target_patent_ids:
                if patent_number not in patent_compounds:
                    patent_compounds[patent_number] = []
                patent_compounds[patent_number].append(compound_id)

    print(f"Loaded mappings for {len(patent_compounds)} patents")
    return patent_compounds


def main():
    """Main extraction process"""
    # Read target IDs
    ids_path = Path(IDS_FILE)
    if not ids_path.exists():
        print(f"Error: {IDS_FILE} not found")
        print("Run: cp ../../test_out/reference/patent_ids.txt .")
        return 1

    with open(ids_path, 'r') as f:
        target_ids = set(line.strip() for line in f if line.strip())

    print(f"Target IDs: {len(target_ids)}")

    # Load all data
    patents = load_patents(target_ids)
    compounds = load_compounds()
    patent_compounds = load_mappings(target_ids, patents)

    # Enhance patent data with compound information
    reference_data = []
    for patent_number, patent in patents.items():
        # Create enhanced entry with ALL original fields
        entry = dict(patent)  # Copy all fields

        # Add compound mappings if available
        if patent_number in patent_compounds:
            entry['compound_ids'] = patent_compounds[patent_number]
            entry['compound_count'] = len(patent_compounds[patent_number])

            # Add compound details for first few compounds
            compound_details = []
            for cid in patent_compounds[patent_number][:5]:  # First 5 compounds
                if cid in compounds:
                    compound_details.append(compounds[cid])
            if compound_details:
                entry['compound_samples'] = compound_details

        reference_data.append(entry)

    print(f"✓ Extracted {len(reference_data)}/{len(target_ids)} patents with complete data")

    # Save reference data
    output_path = Path(OUTPUT_FILE)
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(reference_data, f, indent=2, ensure_ascii=False)

    file_size_kb = output_path.stat().st_size / 1024
    print(f"✓ Saved to {OUTPUT_FILE} ({file_size_kb:.1f} KB)")

    # Show sample
    if reference_data:
        sample = reference_data[0]
        print(f"\nSample entry (showing all extracted fields):")
        print(f"  Patent Number: {sample.get('patent_number')}")
        print(f"  Country: {sample.get('country')}")
        print(f"  Publication Date: {sample.get('publication_date')}")
        print(f"  Family ID: {sample.get('family_id')}")
        print(f"  Title: {sample.get('title', 'N/A')[:60]}...")

        # Show array fields
        cpc = sample.get('cpc', '')
        if cpc and cpc != '[]':
            print(f"  CPC codes: {len(cpc)} chars")
        ipc = sample.get('ipc', '')
        if ipc and ipc != '[]':
            print(f"  IPC codes: {len(ipc)} chars")
        assignees = sample.get('asignee', '')
        if assignees and assignees != '[]':
            print(f"  Assignees: {len(assignees)} chars")

        # Show compound info
        if sample.get('compound_count'):
            print(f"  Compounds: {sample['compound_count']} linked")
            if sample.get('compound_samples'):
                print(f"  Compound samples: {len(sample['compound_samples'])} details included")

    print(f"\n✓ Complete reference data extracted with ALL available fields")

    return 0


if __name__ == "__main__":
    sys.exit(main())
