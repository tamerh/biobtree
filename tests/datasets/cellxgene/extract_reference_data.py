#!/usr/bin/env python3
"""
Extract reference data for CELLxGENE test validation.

Reference data comes from the locally extracted JSON files (not an external API)
since CELLxGENE data is extracted from Census via our Python script.

Data files:
- /data/bioyoda/snapshots/raw_data/cellxgene/cellxgene_datasets.json
- /data/bioyoda/snapshots/raw_data/cellxgene/cellxgene_celltype.json
"""

import json
import os
import sys

# Data file locations
DATASETS_FILE = "/data/bioyoda/snapshots/raw_data/cellxgene/cellxgene_datasets.json"
CELLTYPE_FILE = "/data/bioyoda/snapshots/raw_data/cellxgene/cellxgene_celltype.json"

def load_json_lines(filepath):
    """Load JSON lines file into a list of records."""
    records = []
    with open(filepath, 'r', encoding='utf-8') as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    return records

def extract_datasets_reference():
    """Extract reference data for cellxgene datasets."""
    # Load test IDs
    with open('cellxgene_ids.txt', 'r') as f:
        test_ids = [line.strip() for line in f if line.strip()]

    print(f"Loading datasets from {DATASETS_FILE}")
    all_datasets = load_json_lines(DATASETS_FILE)

    # Build lookup by dataset_id
    dataset_lookup = {d['dataset_id']: d for d in all_datasets}

    # Extract reference data for test IDs
    reference = []
    for dataset_id in test_ids:
        if dataset_id in dataset_lookup:
            reference.append(dataset_lookup[dataset_id])
        else:
            print(f"Warning: Dataset {dataset_id} not found")

    print(f"Extracted {len(reference)} dataset records")
    return reference

def extract_celltype_reference():
    """Extract reference data for cellxgene_celltype."""
    # Load test IDs
    with open('cellxgene_celltype_ids.txt', 'r') as f:
        test_ids = [line.strip() for line in f if line.strip()]

    print(f"Loading cell types from {CELLTYPE_FILE}")
    all_celltypes = load_json_lines(CELLTYPE_FILE)

    # Build lookup by id (CL_XXXXXXX format)
    celltype_lookup = {ct['id']: ct for ct in all_celltypes}

    # Extract reference data for test IDs
    reference = []
    for ct_id in test_ids:
        if ct_id in celltype_lookup:
            reference.append(celltype_lookup[ct_id])
        else:
            print(f"Warning: Cell type {ct_id} not found")

    print(f"Extracted {len(reference)} cell type records")
    return reference

def main():
    # Extract both datasets
    datasets_ref = extract_datasets_reference()
    celltype_ref = extract_celltype_reference()

    # Combine into single reference file
    reference_data = {
        "cellxgene": datasets_ref,
        "cellxgene_celltype": celltype_ref
    }

    # Write reference data
    with open('reference_data.json', 'w', encoding='utf-8') as f:
        json.dump(reference_data, f, indent=2, ensure_ascii=False)

    print(f"\nWrote reference_data.json")
    print(f"  - {len(datasets_ref)} cellxgene datasets")
    print(f"  - {len(celltype_ref)} cellxgene_celltype entries")

if __name__ == '__main__':
    main()
