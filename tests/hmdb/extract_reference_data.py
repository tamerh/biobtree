#!/usr/bin/env python3
"""
Extract reference data from HMDB ZIP file for test validation.

This script:
1. Downloads the HMDB metabolites ZIP file (same source as biobtree)
2. Extracts complete XML entries for test IDs
3. Converts XML to structured JSON preserving ALL data
4. Saves as reference_data.json for test validation

IMPORTANT: Extracts COMPLETE raw data (not selective fields) for future-proofing.
"""

import json
import os
import sys
import xml.etree.ElementTree as ET
import zipfile
from io import BytesIO
from pathlib import Path
from typing import Dict, List, Optional
from urllib.request import urlopen

# Add parent directory to path for config access
sys.path.insert(0, str(Path(__file__).parent.parent))


def load_test_ids() -> List[str]:
    """Load HMDB IDs from the test reference file."""
    ids_file = Path("../../test_out/reference/hmdb_ids.txt")
    if not ids_file.exists():
        print(f"Error: {ids_file} not found. Run 'make test' first.")
        sys.exit(1)

    with open(ids_file) as f:
        ids = [line.strip() for line in f if line.strip()]

    print(f"✓ Loaded {len(ids)} test IDs from {ids_file}")
    return ids


def xml_element_to_dict(element: ET.Element) -> Dict:
    """
    Convert XML element to dictionary recursively, preserving complete structure.

    This extracts ALL data from the XML without filtering, ensuring that
    future test enhancements can use any field without re-extraction.
    """
    result = {}

    # Add element text if present
    if element.text and element.text.strip():
        result["_text"] = element.text.strip()

    # Add attributes if present
    if element.attrib:
        result["_attributes"] = element.attrib

    # Process child elements
    children = list(element)
    if children:
        # Group children by tag name
        children_dict = {}
        for child in children:
            child_data = xml_element_to_dict(child)
            tag = child.tag.split('}')[-1]  # Remove namespace if present

            if tag in children_dict:
                # Convert to list if multiple elements with same tag
                if not isinstance(children_dict[tag], list):
                    children_dict[tag] = [children_dict[tag]]
                children_dict[tag].append(child_data)
            else:
                children_dict[tag] = child_data

        result.update(children_dict)

    # If only text, return as string
    if len(result) == 1 and "_text" in result:
        return result["_text"]

    return result


def extract_metabolite_xml(xml_content: str, target_id: str) -> Optional[str]:
    """Extract a single metabolite XML entry from the full XML content."""
    # Find the metabolite entry for the target ID
    start_tag = f"<metabolite>"
    end_tag = "</metabolite>"

    # Parse incrementally to find the right metabolite
    start_idx = 0
    while True:
        start_idx = xml_content.find(start_tag, start_idx)
        if start_idx == -1:
            return None

        end_idx = xml_content.find(end_tag, start_idx)
        if end_idx == -1:
            return None

        # Extract this metabolite entry
        entry_xml = xml_content[start_idx:end_idx + len(end_tag)]

        # Check if this is the target ID
        if f"<accession>{target_id}</accession>" in entry_xml:
            return entry_xml

        start_idx = end_idx + 1


def download_and_extract_reference_data(test_ids: List[str]) -> tuple[List[Dict], List[str]]:
    """
    Download HMDB ZIP and extract complete entries for test IDs.

    Returns:
        - Complete XML data converted to JSON, preserving all fields
        - Raw XML strings for each entry (for creating test XML file)
    """
    # URL from conf/source.dataset.json
    hmdb_url = "http://www.hmdb.ca/system/downloads/current/hmdb_metabolites.zip"

    print(f"\nDownloading HMDB ZIP from {hmdb_url}...")
    print("(This may take a few minutes - the file is large)")

    try:
        # Download ZIP file
        with urlopen(hmdb_url, timeout=600) as response:
            zip_data = BytesIO(response.read())

        print(f"✓ Downloaded {len(zip_data.getvalue()) / 1024 / 1024:.1f} MB")

        # Extract XML from ZIP
        with zipfile.ZipFile(zip_data) as zf:
            # Get the first (and only) file in the ZIP
            xml_filename = zf.namelist()[0]
            print(f"✓ Extracting {xml_filename}...")

            with zf.open(xml_filename) as xml_file:
                xml_content = xml_file.read().decode('utf-8')

        print(f"✓ Extracted {len(xml_content) / 1024 / 1024:.1f} MB of XML data")

        # Extract entries for each test ID
        reference_data = []
        raw_xml_entries = []
        print(f"\nExtracting {len(test_ids)} metabolite entries...")

        for i, hmdb_id in enumerate(test_ids, 1):
            print(f"  [{i}/{len(test_ids)}] Processing {hmdb_id}...")

            # Extract the XML for this metabolite
            entry_xml = extract_metabolite_xml(xml_content, hmdb_id)

            if not entry_xml:
                print(f"    ⚠ Warning: Could not find entry for {hmdb_id}")
                continue

            # Save raw XML for creating test file
            raw_xml_entries.append(entry_xml)

            # Parse the XML entry
            try:
                root = ET.fromstring(entry_xml)
                entry_dict = xml_element_to_dict(root)

                # Add ID at top level for easy access
                entry_dict["id"] = hmdb_id

                reference_data.append(entry_dict)
                print(f"    ✓ Extracted complete entry ({len(json.dumps(entry_dict))} bytes)")

            except ET.ParseError as e:
                print(f"    ✗ Error parsing XML for {hmdb_id}: {e}")
                continue

        print(f"\n✓ Successfully extracted {len(reference_data)} entries")
        return reference_data, raw_xml_entries

    except Exception as e:
        print(f"✗ Error downloading/extracting HMDB data: {e}")
        sys.exit(1)


def save_reference_data(reference_data: List[Dict], output_file: Path):
    """Save reference data to JSON file."""
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    file_size = output_file.stat().st_size
    print(f"\n✓ Saved {len(reference_data)} entries to {output_file}")
    print(f"  File size: {file_size / 1024:.1f} KB")
    print(f"  Complete raw data preserved for future test enhancements")


def save_test_xml(raw_xml_entries: List[str], output_dir: Path):
    """
    Create test XML file with extracted entries and zip it.

    This creates a small HMDB XML file for fast testing without
    downloading the entire 220K+ entry file.
    """
    # Create complete XML file with HMDB structure
    xml_header = '<?xml version="1.0" encoding="UTF-8"?>\n'
    xml_header += '<hmdb xmlns="http://www.hmdb.ca">\n'
    xml_footer = '</hmdb>\n'

    # Combine all entries
    xml_content = xml_header
    for entry_xml in raw_xml_entries:
        # Add proper indentation
        xml_content += "  " + entry_xml.replace("\n", "\n  ") + "\n"
    xml_content += xml_footer

    # Save XML file
    xml_file = output_dir / "hmdb_test.xml"
    with open(xml_file, 'w', encoding='utf-8') as f:
        f.write(xml_content)

    xml_size = xml_file.stat().st_size
    print(f"\n✓ Created test XML: {xml_file}")
    print(f"  Size: {xml_size / 1024:.1f} KB ({len(raw_xml_entries)} entries)")

    # Create ZIP file (same structure as HMDB source)
    zip_file = output_dir / "hmdb_test.zip"
    with zipfile.ZipFile(zip_file, 'w', zipfile.ZIP_DEFLATED) as zf:
        zf.write(xml_file, "hmdb_metabolites.xml")

    zip_size = zip_file.stat().st_size
    print(f"\n✓ Created test ZIP: {zip_file}")
    print(f"  Size: {zip_size / 1024:.1f} KB")
    print(f"  Compression ratio: {xml_size / zip_size:.1f}x")

    # Remove uncompressed XML to save space
    xml_file.unlink()

    return zip_file


def main():
    """Main extraction workflow."""
    print("=" * 60)
    print("HMDB Reference Data Extraction")
    print("=" * 60)

    # Load test IDs
    test_ids = load_test_ids()

    # Download and extract reference data
    reference_data, raw_xml_entries = download_and_extract_reference_data(test_ids)

    if not reference_data:
        print("\n✗ No reference data extracted")
        sys.exit(1)

    # Save JSON reference data for test validation
    output_file = Path(__file__).parent / "reference_data.json"
    save_reference_data(reference_data, output_file)

    # Save test XML file for biobtree testing
    output_dir = Path(__file__).parent
    zip_file = save_test_xml(raw_xml_entries, output_dir)

    print("\n" + "=" * 60)
    print("✓ Reference data extraction complete")
    print("=" * 60)
    print(f"\nNext steps:")
    print(f"  1. JSON reference data: reference_data.json")
    print(f"  2. Test XML ZIP: {zip_file.name}")
    print(f"\nTo use the test ZIP in biobtree, update conf/source.dataset.json:")
    print(f'  "path2": "tests/hmdb/{zip_file.name}"')


if __name__ == "__main__":
    main()
