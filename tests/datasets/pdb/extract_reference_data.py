#!/usr/bin/env python3
"""
Extract reference data from RCSB PDB API for testing.

This script fetches metadata for PDB entries listed in pdb_ids.txt
and stores them in reference_data.json for use by the test framework.

Usage:
    python3 extract_reference_data.py
"""

import json
import time
from pathlib import Path

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    exit(1)

# RCSB PDB API base URL
RCSB_API_BASE = "https://data.rcsb.org/rest/v1/core/entry"

def load_pdb_ids(filepath: Path) -> list:
    """Load PDB IDs from text file, ignoring comments and empty lines."""
    ids = []
    with open(filepath, 'r') as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith('#'):
                continue
            # Extract just the ID (first word before any comment)
            pdb_id = line.split()[0].upper()
            if len(pdb_id) == 4:  # Valid PDB ID length
                ids.append(pdb_id)
    return ids

def fetch_pdb_entry(pdb_id: str) -> dict:
    """Fetch a single PDB entry from RCSB API."""
    url = f"{RCSB_API_BASE}/{pdb_id}"
    try:
        response = requests.get(url, timeout=30)
        if response.status_code == 200:
            return response.json()
        else:
            print(f"  Warning: {pdb_id} returned status {response.status_code}")
            return None
    except requests.RequestException as e:
        print(f"  Error fetching {pdb_id}: {e}")
        return None

def extract_reference_data(entry: dict) -> dict:
    """Extract relevant fields from RCSB PDB API response."""
    if not entry:
        return None

    pdb_id = entry.get("rcsb_id", "")

    # Extract struct info
    struct = entry.get("struct", {})
    title = struct.get("title", "")

    # Extract classification from struct_keywords
    struct_keywords = entry.get("struct_keywords", {})
    header = struct_keywords.get("pdbx_keywords", "")

    # Extract experimental method
    exptl = entry.get("exptl", [{}])
    method = exptl[0].get("method", "") if exptl else ""

    # Extract resolution (if applicable)
    reflns = entry.get("reflns", [{}])
    resolution = None
    if reflns:
        resolution = reflns[0].get("d_resolution_high")
    if not resolution:
        # Try rcsb_entry_info for computed resolution
        entry_info = entry.get("rcsb_entry_info", {})
        resolution = entry_info.get("resolution_combined", [None])[0]

    # Extract deposition date
    rcsb_accession = entry.get("rcsb_accession_info", {})
    deposit_date = rcsb_accession.get("deposit_date", "")

    # Extract organism
    source = entry.get("rcsb_entry_container_identifiers", {})
    entity_ids = source.get("entity_ids", [])

    # Get polymer entities for organism info
    polymer_entities = entry.get("polymer_entities", [])
    organism = ""
    if polymer_entities:
        src_org = polymer_entities[0].get("rcsb_entity_source_organism", [{}])
        if src_org:
            organism = src_org[0].get("scientific_name", "")

    # Extract molecule type from entry_info
    entry_info = entry.get("rcsb_entry_info", {})
    mol_type = entry_info.get("selected_polymer_entity_types", "")

    # Get polymer entity count
    polymer_count = entry_info.get("polymer_entity_count", 0)

    # Extract authors
    audit_authors = entry.get("audit_author", [])
    authors = [a.get("name", "") for a in audit_authors if a.get("name")]

    # Get cross-reference counts
    xrefs = {}

    # UniProt xrefs from polymer entities
    uniprot_ids = set()
    for pe in polymer_entities:
        uniprot_kb = pe.get("rcsb_polymer_entity_container_identifiers", {}).get("uniprot_ids", [])
        uniprot_ids.update(uniprot_kb)
    if uniprot_ids:
        xrefs["uniprot"] = list(uniprot_ids)

    # GO terms
    go_terms = set()
    for pe in polymer_entities:
        go_annot = pe.get("rcsb_polymer_entity_annotation", [])
        for annot in go_annot:
            if annot.get("type") == "GO":
                go_terms.add(annot.get("annotation_id", ""))
    if go_terms:
        xrefs["go"] = [g for g in go_terms if g]

    return {
        "pdb_id": pdb_id,
        "title": title,
        "header": header,
        "method": method,
        "resolution": resolution,
        "deposit_date": deposit_date,
        "organism": organism,
        "molecule_type": mol_type,
        "polymer_count": polymer_count,
        "authors": authors[:5],  # Limit to first 5 authors
        "xrefs": xrefs
    }

def main():
    script_dir = Path(__file__).parent
    ids_file = script_dir / "pdb_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Load PDB IDs
    print(f"Loading PDB IDs from {ids_file}...")
    pdb_ids = load_pdb_ids(ids_file)
    print(f"Found {len(pdb_ids)} PDB IDs")

    # Fetch data from RCSB API
    reference_data = []
    for i, pdb_id in enumerate(pdb_ids):
        print(f"  [{i+1}/{len(pdb_ids)}] Fetching {pdb_id}...")
        entry = fetch_pdb_entry(pdb_id)
        if entry:
            ref_data = extract_reference_data(entry)
            if ref_data:
                reference_data.append(ref_data)
        # Be nice to the API
        time.sleep(0.2)

    # Save reference data
    print(f"\nSaving {len(reference_data)} entries to {output_file}...")
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print("Done!")
    return 0

if __name__ == "__main__":
    exit(main())
