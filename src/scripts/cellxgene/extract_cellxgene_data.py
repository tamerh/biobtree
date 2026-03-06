#!/usr/bin/env python3
"""
CELLxGENE Census Data Extraction Script for Biobtree

This script extracts aggregated data from the CZ CELLxGENE Census API
and generates JSON files for biobtree to consume.

CONSOLIDATED VERSION: Produces 2 datasets instead of 5
  1. cellxgene_datasets.json - Dataset metadata
  2. cellxgene_celltype.json - Comprehensive cell type data (merged)

Requirements:
    pip install cellxgene-census pandas tiledbsoma

Usage:
    python extract_cellxgene_data.py [--output-dir ./data] [--test-mode]

Author: Biobtree Project
Date: 2026-01-28
"""

import argparse
import json
import logging
import os
import sys
import time
import urllib.request
from datetime import datetime
from typing import Dict, List, Any, Optional
from collections import defaultdict

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger(__name__)


def check_dependencies():
    """Check if required packages are installed."""
    missing = []
    try:
        import cellxgene_census
    except ImportError:
        missing.append('cellxgene-census')
    try:
        import pandas
    except ImportError:
        missing.append('pandas')
    try:
        import tiledbsoma
    except ImportError:
        missing.append('tiledbsoma')

    if missing:
        logger.error(f"Missing required packages: {', '.join(missing)}")
        logger.error("Install with: pip install " + " ".join(missing))
        sys.exit(1)


def write_json_lines(filepath: str, records: List[Dict], description: str):
    """Write records as JSON lines (one JSON object per line)."""
    logger.info(f"Writing {len(records)} {description} to {filepath}")
    with open(filepath, 'w', encoding='utf-8') as f:
        for record in records:
            f.write(json.dumps(record, ensure_ascii=False) + '\n')
    logger.info(f"Successfully wrote {filepath}")


def fetch_curation_api_metadata() -> Dict[str, Dict]:
    """
    Fetch dataset metadata from CELLxGENE Curation API.

    This API provides organism, assay, tissue, disease for ALL datasets,
    including spatial transcriptomics and other modalities not in Census obs.

    Returns:
        Dict mapping dataset_id to metadata dict with organism, assay, tissue, disease
    """
    logger.info("Fetching metadata from CELLxGENE Curation API...")

    api_url = "https://api.cellxgene.cziscience.com/curation/v1/collections"

    try:
        req = urllib.request.Request(api_url, headers={"Accept": "application/json"})
        with urllib.request.urlopen(req, timeout=120) as response:
            collections = json.loads(response.read().decode('utf-8'))

        logger.info(f"Fetched {len(collections)} collections from Curation API")

        # Build dataset lookup
        dataset_lookup = {}
        for coll in collections:
            for ds in coll.get('datasets', []):
                dataset_id = ds.get('dataset_id', '')
                if not dataset_id:
                    continue

                # Extract organism info
                organisms = ds.get('organism', [])
                organism_labels = [o.get('label', '') for o in organisms]
                organism_ids = [o.get('ontology_term_id', '') for o in organisms]

                # Extract assay info
                assays = ds.get('assay', [])
                assay_labels = [a.get('label', '') for a in assays]
                assay_ids = [a.get('ontology_term_id', '') for a in assays]

                # Extract tissue info
                tissues = ds.get('tissue', [])
                tissue_labels = [t.get('label', '') for t in tissues]
                tissue_ids = [t.get('ontology_term_id', '') for t in tissues]

                # Extract disease info
                diseases = ds.get('disease', [])
                disease_labels = [d.get('label', '') for d in diseases]
                disease_ids = [d.get('ontology_term_id', '') for d in diseases]

                dataset_lookup[dataset_id] = {
                    'organism': organism_labels[0] if organism_labels else '',
                    'organism_taxid': organism_ids[0] if organism_ids else '',
                    'assay_types': assay_labels,
                    'assay_efo_ids': assay_ids,
                    'tissues': tissue_labels,
                    'tissue_uberon_ids': tissue_ids,
                    'diseases': disease_labels,
                    'disease_mondo_ids': disease_ids,
                }

        logger.info(f"Built metadata lookup for {len(dataset_lookup)} datasets")
        return dataset_lookup

    except Exception as e:
        logger.error(f"Failed to fetch Curation API data: {e}")
        return {}


def extract_datasets_with_metadata(census, output_dir: str, test_mode: bool = False) -> List[Dict]:
    """
    Extract dataset metadata using:
    - Curation API: Primary source for organism, assay, tissue, disease (available for ALL datasets)
    - Census obs: Cell type information only (available for scRNA-seq datasets in Census)

    This ensures all datasets have complete metadata, including spatial transcriptomics
    and other modalities not represented in Census obs tables.
    """
    logger.info("Extracting dataset metadata...")
    start_time = time.time()

    import pandas as pd

    # ========================================================================
    # 1. Fetch metadata from CELLxGENE Curation API (primary source)
    # ========================================================================
    curation_metadata = fetch_curation_api_metadata()
    logger.info(f"Got metadata from Curation API for {len(curation_metadata)} datasets")

    # ========================================================================
    # 2. Get datasets table from Census
    # ========================================================================
    datasets_df = census["census_info"]["datasets"].read().concat().to_pandas()
    logger.info(f"Found {len(datasets_df)} datasets in Census")

    if test_mode:
        datasets_df = datasets_df.head(20)
        logger.info(f"[TEST MODE] Limited to {len(datasets_df)} datasets")

    # ========================================================================
    # 3. Get cell type info from Census obs (only available for some datasets)
    # ========================================================================
    logger.info("Querying Census obs for cell type information...")

    # Query obs for human cells - only need cell type and dataset_id
    human_obs = census["census_data"]["homo_sapiens"].obs.read(
        column_names=["dataset_id", "cell_type_ontology_term_id", "cell_type"]
    ).concat().to_pandas()
    logger.info(f"Loaded {len(human_obs)} human cell records")

    # Also get mouse data
    mouse_obs = census["census_data"]["mus_musculus"].obs.read(
        column_names=["dataset_id", "cell_type_ontology_term_id", "cell_type"]
    ).concat().to_pandas()
    logger.info(f"Loaded {len(mouse_obs)} mouse cell records")

    # Combine
    all_obs = pd.concat([human_obs, mouse_obs], ignore_index=True)
    logger.info(f"Total cell records: {len(all_obs)}")

    # Aggregate cell types per dataset
    logger.info("Aggregating cell types per dataset...")
    celltype_metadata = {}

    for dataset_id, group in all_obs.groupby("dataset_id", observed=True):
        celltype_metadata[dataset_id] = {
            "cell_types": sorted(group["cell_type"].dropna().unique().tolist()),
            "cell_type_cl_ids": sorted(group["cell_type_ontology_term_id"].dropna().unique().tolist()),
        }

    logger.info(f"Got cell type data for {len(celltype_metadata)} datasets from Census obs")

    # ========================================================================
    # 4. Build final records (merge Census table + Curation API + Census obs)
    # ========================================================================
    records = []
    datasets_with_curation = 0
    datasets_with_celltypes = 0

    for idx, row in datasets_df.iterrows():
        dataset_id = str(row.get("dataset_id", ""))

        # Get metadata from Curation API (organism, assay, tissue, disease)
        curation = curation_metadata.get(dataset_id, {})
        if curation:
            datasets_with_curation += 1

        # Get cell types from Census obs
        celltypes = celltype_metadata.get(dataset_id, {})
        if celltypes.get("cell_types"):
            datasets_with_celltypes += 1

        record = {
            "dataset_id": dataset_id,
            "title": str(row.get("dataset_title", "")),
            "collection_name": str(row.get("collection_name", "")),
            "collection_id": str(row.get("collection_id", "")),
            "collection_doi": str(row.get("collection_doi", "")),
            "cell_count": int(row.get("dataset_total_cell_count", 0)),
            "citation": str(row.get("citation", "")),

            # From Curation API (always available)
            "organism": curation.get("organism", ""),
            "organism_taxid": curation.get("organism_taxid", ""),
            "assay_types": curation.get("assay_types", []),
            "assay_efo_ids": curation.get("assay_efo_ids", []),
            "tissues": curation.get("tissues", []),
            "tissue_uberon_ids": curation.get("tissue_uberon_ids", []),
            "diseases": curation.get("diseases", []),
            "disease_mondo_ids": curation.get("disease_mondo_ids", []),

            # From Census obs (only for scRNA-seq datasets in Census)
            "cell_types": celltypes.get("cell_types", []),
            "cell_type_cl_ids": celltypes.get("cell_type_cl_ids", []),
        }
        records.append(record)

    elapsed = time.time() - start_time
    logger.info(f"Extracted {len(records)} datasets in {elapsed:.2f}s")
    logger.info(f"  - {datasets_with_curation} have Curation API metadata (organism, assay, tissue, disease)")
    logger.info(f"  - {datasets_with_celltypes} have cell type information from Census obs")

    # Write output
    output_path = os.path.join(output_dir, "cellxgene_datasets.json")
    write_json_lines(output_path, records, "datasets")

    return records


def extract_celltype_consolidated(census, output_dir: str, test_mode: bool = False) -> List[Dict]:
    """
    Extract CONSOLIDATED cell type data.

    Merges what was previously 4 separate datasets:
    - cellguide (cell type definitions)
    - markers (marker genes)
    - expression (expression by tissue)
    - counts (cell counts from summary_cell_counts)

    Primary key: Cell Ontology ID (CL:XXXXXXX)
    """
    logger.info("Extracting consolidated cell type data...")
    start_time = time.time()

    import pandas as pd

    # ========================================================================
    # 1. Get cell type information from obs (both human and mouse)
    # ========================================================================
    logger.info("Getting cell type information from obs...")

    human_obs = census["census_data"]["homo_sapiens"].obs.read(
        column_names=["cell_type_ontology_term_id", "cell_type",
                      "tissue_ontology_term_id", "tissue",
                      "disease_ontology_term_id", "disease"]
    ).concat().to_pandas()

    mouse_obs = census["census_data"]["mus_musculus"].obs.read(
        column_names=["cell_type_ontology_term_id", "cell_type",
                      "tissue_ontology_term_id", "tissue",
                      "disease_ontology_term_id", "disease"]
    ).concat().to_pandas()

    all_obs = pd.concat([human_obs, mouse_obs], ignore_index=True)
    logger.info(f"Total cell metadata: {len(all_obs)} records")

    # ========================================================================
    # 2. Aggregate per cell type
    # ========================================================================
    logger.info("Aggregating data per cell type...")

    cell_type_data = {}

    for cl_id, group in all_obs.groupby("cell_type_ontology_term_id", observed=True):
        # Get first cell type name (should be consistent)
        cell_type_name = group["cell_type"].iloc[0] if len(group) > 0 else ""

        # Get unique tissues with counts
        tissue_counts = group.groupby(
            ["tissue_ontology_term_id", "tissue"],
            observed=True
        ).size().reset_index(name='cell_count')

        expression_by_tissue = []
        for _, row in tissue_counts.iterrows():
            expression_by_tissue.append({
                "tissue_uberon": row["tissue_ontology_term_id"],
                "tissue_name": row["tissue"],
                "cell_count": int(row["cell_count"])
            })

        # Sort by cell count descending
        expression_by_tissue.sort(key=lambda x: x["cell_count"], reverse=True)

        cell_type_data[cl_id] = {
            "name": cell_type_name,
            "total_cells": len(group),
            "tissues": sorted(group["tissue"].dropna().unique().tolist()),
            "tissue_ids": sorted(group["tissue_ontology_term_id"].dropna().unique().tolist()),
            "diseases": sorted(group["disease"].dropna().unique().tolist()),
            "disease_ids": sorted(group["disease_ontology_term_id"].dropna().unique().tolist()),
            "expression_by_tissue": expression_by_tissue
        }

    logger.info(f"Found {len(cell_type_data)} unique cell types")

    # ========================================================================
    # 3. Get additional counts from summary_cell_counts
    # ========================================================================
    logger.info("Getting summary cell counts...")

    counts_df = census["census_info"]["summary_cell_counts"].read().concat().to_pandas()

    # Filter to cell_type category and build lookup
    cell_type_counts = counts_df[counts_df["category"] == "cell_type"]
    counts_lookup = {}
    for _, row in cell_type_counts.iterrows():
        ont_id = str(row.get("ontology_term_id", ""))
        if ont_id and ont_id != "na":
            counts_lookup[ont_id] = {
                "total_cell_count": int(row.get("total_cell_count", 0)),
                "unique_cell_count": int(row.get("unique_cell_count", 0))
            }

    logger.info(f"Found {len(counts_lookup)} cell type counts from summary")

    # ========================================================================
    # 4. Build final records
    # ========================================================================
    if test_mode:
        # Limit to first 100 cell types in test mode
        cell_type_data = dict(list(cell_type_data.items())[:100])
        logger.info(f"[TEST MODE] Limited to {len(cell_type_data)} cell types")

    records = []
    for cl_id, info in cell_type_data.items():
        # Get counts from summary if available
        counts = counts_lookup.get(cl_id, {})

        record = {
            "id": cl_id.replace(":", "_"),
            "cell_type_cl": cl_id,
            "name": info["name"],
            "definition": "",  # Would come from Cell Ontology OWL file
            "synonyms": [],

            # Marker genes (would need computation)
            "canonical_markers": [],
            "canonical_marker_ids": [],
            "markers": [],  # Computed marker genes

            # Cell type hierarchy (would come from CL ontology)
            "develops_from": [],
            "develops_from_ids": [],

            # Tissue distribution
            "found_in_tissues": info["tissues"],
            "found_in_tissue_ids": info["tissue_ids"],

            # Disease associations
            "associated_diseases": info["diseases"],
            "associated_disease_ids": info["disease_ids"],

            # Cell counts
            "total_cells": info["total_cells"],
            "total_cell_count": counts.get("total_cell_count", info["total_cells"]),
            "unique_cell_count": counts.get("unique_cell_count", 0),

            # Expression by tissue (nested array)
            "expression_by_tissue": info["expression_by_tissue"]
        }
        records.append(record)

    # Sort by total cells descending
    records.sort(key=lambda x: x["total_cells"], reverse=True)

    elapsed = time.time() - start_time
    logger.info(f"Extracted {len(records)} consolidated cell type records in {elapsed:.2f}s")

    output_path = os.path.join(output_dir, "cellxgene_celltype.json")
    write_json_lines(output_path, records, "cell type records")

    return records


def main():
    parser = argparse.ArgumentParser(
        description="Extract CELLxGENE Census data for Biobtree (Consolidated 2-dataset version)"
    )
    parser.add_argument(
        "--output-dir", "-o",
        default="./cellxgene_data",
        help="Output directory for JSON files (default: ./cellxgene_data)"
    )
    parser.add_argument(
        "--test-mode", "-t",
        action="store_true",
        help="Run in test mode with limited records"
    )
    parser.add_argument(
        "--census-version",
        default="stable",
        help="Census version to use (default: stable)"
    )

    args = parser.parse_args()

    check_dependencies()

    import cellxgene_census

    os.makedirs(args.output_dir, exist_ok=True)
    logger.info(f"Output directory: {args.output_dir}")

    total_start = time.time()

    logger.info(f"Opening Census connection (version: {args.census_version})...")
    try:
        census = cellxgene_census.open_soma(census_version=args.census_version)
        logger.info("Census connection established")
    except Exception as e:
        logger.error(f"Failed to open Census: {e}")
        sys.exit(1)

    try:
        stats = {}

        # 1. Dataset metadata (with enriched cell types, tissues, etc.)
        datasets = extract_datasets_with_metadata(census, args.output_dir, args.test_mode)
        stats["datasets"] = len(datasets)

        # 2. Consolidated cell type data (replaces counts, markers, expression, cellguide)
        celltypes = extract_celltype_consolidated(census, args.output_dir, args.test_mode)
        stats["celltypes"] = len(celltypes)

        # Write metadata file
        metadata = {
            "extraction_date": datetime.now().isoformat(),
            "census_version": args.census_version,
            "test_mode": args.test_mode,
            "statistics": stats,
            "note": "Consolidated 2-dataset version (datasets + celltype)"
        }
        metadata_path = os.path.join(args.output_dir, "extraction_metadata.json")
        with open(metadata_path, 'w') as f:
            json.dump(metadata, f, indent=2)

        total_elapsed = time.time() - total_start
        logger.info("=" * 60)
        logger.info("EXTRACTION COMPLETE (CONSOLIDATED VERSION)")
        logger.info(f"Total time: {total_elapsed:.2f}s ({total_elapsed/60:.1f} minutes)")
        logger.info(f"Output directory: {args.output_dir}")
        logger.info("Statistics:")
        for key, value in stats.items():
            logger.info(f"  - {key}: {value:,} records")
        logger.info("=" * 60)

    finally:
        census.close()
        logger.info("Census connection closed")


if __name__ == "__main__":
    main()
