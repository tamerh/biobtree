#!/usr/bin/env python3
"""
Extract reference data for GenCC test entries.

Reads GenCC TSV file and extracts data for entries listed in gencc_ids.txt.
"""

import json
import csv
from pathlib import Path


def extract_reference_data():
    """Extract reference data from GenCC TSV file"""
    script_dir = Path(__file__).parent
    ids_file = script_dir / "gencc_ids.txt"
    output_file = script_dir / "reference_data.json"

    # GenCC TSV file location - downloaded during test build
    gencc_file = script_dir.parent.parent.parent / "test_data" / "gencc" / "gencc_submissions.tsv"

    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print("Run: ./biobtree -d gencc test")
        print("Then copy test_out/reference/gencc_ids.txt here")
        return 1

    # Read test IDs
    with open(ids_file) as f:
        test_ids = set(line.strip() for line in f if line.strip())

    print(f"Found {len(test_ids)} test IDs")

    if not gencc_file.exists():
        print(f"Error: GenCC TSV file not found at {gencc_file}")
        print("Attempting to download...")

        import urllib.request
        gencc_file.parent.mkdir(parents=True, exist_ok=True)
        url = "https://search.thegencc.org/download/action/submissions-export-tsv"
        urllib.request.urlretrieve(url, gencc_file)
        print(f"Downloaded to {gencc_file}")

    # Read GenCC TSV and extract matching entries
    reference_data = []

    with open(gencc_file, 'r', encoding='utf-8') as f:
        # Skip potential BOM and read header
        reader = csv.DictReader(f, delimiter='\t')

        for row in reader:
            uuid = row.get('uuid', '').strip().strip('"')
            if uuid in test_ids:
                entry = {
                    'uuid': uuid,
                    'gene_curie': row.get('gene_curie', '').strip().strip('"'),
                    'gene_symbol': row.get('gene_symbol', '').strip().strip('"'),
                    'disease_curie': row.get('disease_curie', '').strip().strip('"'),
                    'disease_title': row.get('disease_title', '').strip().strip('"'),
                    'classification_curie': row.get('classification_curie', '').strip().strip('"'),
                    'classification_title': row.get('classification_title', '').strip().strip('"'),
                    'moi_curie': row.get('moi_curie', '').strip().strip('"'),
                    'moi_title': row.get('moi_title', '').strip().strip('"'),
                    'submitter_curie': row.get('submitter_curie', '').strip().strip('"'),
                    'submitter_title': row.get('submitter_title', '').strip().strip('"'),
                    'submitted_as_date': row.get('submitted_as_date', '').strip().strip('"'),
                    'submitted_as_public_report_url': row.get('submitted_as_public_report_url', '').strip().strip('"'),
                }

                # Parse PMIDs
                pmids_str = row.get('submitted_as_pmids', '').strip().strip('"')
                if pmids_str:
                    pmids_str = pmids_str.replace(';', ',')
                    pmids = [p.strip() for p in pmids_str.split(',') if p.strip()]
                    entry['submitted_as_pmids'] = pmids
                else:
                    entry['submitted_as_pmids'] = []

                reference_data.append(entry)

    print(f"Extracted {len(reference_data)} entries")

    # Write reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"Saved to {output_file}")
    return 0


if __name__ == "__main__":
    exit(extract_reference_data())
