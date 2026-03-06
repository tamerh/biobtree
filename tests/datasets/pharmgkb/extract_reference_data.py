#!/usr/bin/env python3
"""
Extract reference data from PharmGKB chemicals.zip for testing.

This script reads the chemicals.zip file and extracts key data
for test validation. Run after a test build to generate reference_data.json.
"""

import json
import zipfile
import csv
from pathlib import Path
import sys


def extract_reference_data(zip_path: str, output_path: str, limit: int = 100):
    """Extract reference data from PharmGKB chemicals.zip"""

    reference_data = []

    with zipfile.ZipFile(zip_path, 'r') as zf:
        # Find TSV file inside zip
        tsv_files = [f for f in zf.namelist() if f.endswith('.tsv')]
        if not tsv_files:
            print("No TSV file found in zip")
            return

        tsv_file = tsv_files[0]
        print(f"Reading {tsv_file}...")

        with zf.open(tsv_file) as f:
            # Read as text
            content = f.read().decode('utf-8', errors='replace')
            lines = content.split('\n')

            # Parse header
            header = lines[0].split('\t')
            col_map = {name.strip(): i for i, name in enumerate(header)}

            count = 0
            for line in lines[1:]:
                if not line.strip():
                    continue

                fields = line.split('\t')

                pharmgkb_id = get_field(fields, col_map, 'PharmGKB Accession Id')
                if not pharmgkb_id:
                    continue

                entry = {
                    'pharmgkb_id': pharmgkb_id,
                    'name': get_field(fields, col_map, 'Name'),
                    'type': get_field(fields, col_map, 'Type'),
                    'generic_names': parse_list(get_field(fields, col_map, 'Generic Names')),
                    'trade_names': parse_list(get_field(fields, col_map, 'Trade Names')),
                    'rxnorm_ids': parse_list(get_field(fields, col_map, 'RxNorm Identifiers')),
                    'atc_codes': parse_list(get_field(fields, col_map, 'ATC Identifiers')),
                    'pubchem_cids': parse_list(get_field(fields, col_map, 'PubChem Compound Identifiers')),
                    'clinical_annotation_count': parse_int(get_field(fields, col_map, 'Clinical Annotation Count')),
                    'variant_annotation_count': parse_int(get_field(fields, col_map, 'Variant Annotation Count')),
                    'pathway_count': parse_int(get_field(fields, col_map, 'Pathway Count')),
                    'top_clinical_level': get_field(fields, col_map, 'Top Clinical Annotation Level'),
                    'top_fda_label_level': get_field(fields, col_map, 'Top FDA Label Testing Level'),
                    'has_dosing_guideline': get_field(fields, col_map, 'Dosing Guideline') == 'Yes',
                    'dosing_guideline_sources': parse_list(get_field(fields, col_map, 'Dosing Guideline Sources')),
                }

                # Only include entries with meaningful data
                if entry['name'] or entry['clinical_annotation_count'] > 0:
                    reference_data.append(entry)
                    count += 1

                if count >= limit:
                    break

    # Sort by clinical annotation count (most annotated first)
    reference_data.sort(key=lambda x: x.get('clinical_annotation_count', 0), reverse=True)

    # Write output
    with open(output_path, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"Wrote {len(reference_data)} entries to {output_path}")


def get_field(fields, col_map, col_name):
    """Safely get field value by column name"""
    if col_name in col_map and col_map[col_name] < len(fields):
        return fields[col_map[col_name]].strip()
    return ''


def parse_list(s):
    """Parse comma-separated or quoted list"""
    if not s:
        return []
    # Handle quoted values like "value1", "value2"
    parts = s.split(',')
    result = []
    for part in parts:
        part = part.strip().strip('"')
        if part:
            result.append(part)
    return result


def parse_int(s):
    """Parse integer with fallback to 0"""
    if not s or s == 'n/a':
        return 0
    try:
        return int(s)
    except ValueError:
        return 0


def main():
    # Default paths
    default_zip = '/data/bioyoda/snapshots/raw_data/clinpgx/chemicals.zip'
    script_dir = Path(__file__).parent
    default_output = script_dir / 'reference_data.json'

    zip_path = sys.argv[1] if len(sys.argv) > 1 else default_zip
    output_path = sys.argv[2] if len(sys.argv) > 2 else str(default_output)
    limit = int(sys.argv[3]) if len(sys.argv) > 3 else 100

    if not Path(zip_path).exists():
        print(f"Error: {zip_path} not found")
        print(f"Usage: {sys.argv[0]} [zip_path] [output_path] [limit]")
        sys.exit(1)

    extract_reference_data(zip_path, output_path, limit)


if __name__ == '__main__':
    main()
