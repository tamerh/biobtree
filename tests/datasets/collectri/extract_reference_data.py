#!/usr/bin/env python3
"""
Extract reference data for CollecTRI test entries.

Fetches data from the CollecTRI source TSV file for test IDs.
Source: https://zenodo.org/record/7773985/files/CollecTRI_source.tsv
"""

import json
import requests
from pathlib import Path


def parse_pmids(pmid_str):
    """Parse PMIDs from various formats in the TSV."""
    if not pmid_str or pmid_str == '""':
        return []
    # Remove quotes and split by semicolon or pipe
    pmid_str = pmid_str.strip('"')
    pmids = set()
    for part in pmid_str.replace('|', ';').split(';'):
        part = part.strip()
        if part and part.isdigit():
            pmids.add(part)
    return list(pmids)[:10]  # Limit to 10 PMIDs for reference data


def extract_sources(row, header_map):
    """Extract evidence sources from the row, excluding TRED."""
    sources = []

    source_columns = [
        ("[ExTRI] present", "ExTRI"),
        ("[HTRI] present", "HTRI"),
        ("[TRRUST] present", "TRRUST"),
        ("[TFactS] present", "TFactS"),
        ("[GOA] present", "GOA"),
        ("[IntAct] present", "IntAct"),
        ("[SIGNOR] present", "SIGNOR"),
        ("[CytReg] present", "CytReg"),
        ("[GEREDB] present", "GEREDB"),
        ("[NTNU Curated] present", "NTNU Curated"),
        ("[Pavlidis2021] present", "Pavlidis2021"),
        ("[DoRothEA_A] present", "DoRothEA_A"),
    ]

    for col_name, source_name in source_columns:
        if col_name in header_map:
            idx = header_map[col_name]
            if idx < len(row) and row[idx] and row[idx].lower() not in ('', 'false'):
                # Check for TFactS Source column to exclude TRED
                if source_name == "TFactS" and "[TFactS] Source" in header_map:
                    tfacts_source = row[header_map["[TFactS] Source"]] if header_map["[TFactS] Source"] < len(row) else ""
                    if "TRED" in tfacts_source:
                        # Check if there are other sources
                        other_sources = [s.strip() for s in tfacts_source.split(';') if s.strip() and "TRED" not in s]
                        if not other_sources:
                            continue  # Skip TFactS if only TRED
                sources.append(source_name)

    return sources


def extract_regulation(row, header_map):
    """Extract regulation direction from the row."""
    regulation_columns = [
        "[TRRUST] Regulation",
        "[TFactS] Sign",
        "[GOA] Sign",
        "[SIGNOR] Sign",
        "[CytReg] Activation/Repression",
        "[GEREDB] Effect",
        "[NTNU Curated] Sign",
        "[Pavlidis2021] Mode of action",
        "[DoRothEA_A] Effect",
    ]

    for col_name in regulation_columns:
        if col_name in header_map:
            idx = header_map[col_name]
            if idx < len(row) and row[idx]:
                val = row[idx].lower().strip()
                if 'activ' in val or 'positive' in val or val == '+' or val == 'up':
                    return "Activation"
                elif 'repress' in val or 'negative' in val or 'inhibit' in val or val == '-' or val == 'down':
                    return "Repression"

    return "Unknown"


def extract_confidence(row, header_map):
    """Extract confidence level from the row."""
    confidence_columns = [
        "[ExTRI] Confidence",
        "[HTRI] Confidence",
        "[TFactS] Confidence",
    ]

    for col_name in confidence_columns:
        if col_name in header_map:
            idx = header_map[col_name]
            if idx < len(row) and row[idx]:
                val = row[idx].strip()
                if val:
                    return val

    return ""


def main():
    script_dir = Path(__file__).parent
    ids_file = script_dir / "collectri_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Read test IDs
    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print("Run: ./biobtree -d collectri test")
        return 1

    with open(ids_file) as f:
        test_ids = set(line.strip() for line in f if line.strip())

    print(f"Loading {len(test_ids)} test IDs...")

    # Download CollecTRI TSV
    url = "https://zenodo.org/record/7773985/files/CollecTRI_source.tsv"
    print(f"Downloading from {url}...")

    response = requests.get(url, stream=True)
    response.raise_for_status()

    # Parse TSV
    reference_data = []
    lines = response.text.split('\n')

    # Parse header
    header = lines[0].split('\t')
    header_map = {name.strip(): i for i, name in enumerate(header)}

    print(f"Found {len(header_map)} columns")

    # Process rows
    for line in lines[1:]:
        if not line.strip():
            continue

        row = line.split('\t')
        tf_tg = row[0].strip() if row else ""

        if tf_tg not in test_ids:
            continue

        # Parse TF and target
        parts = tf_tg.split(':', 1)
        if len(parts) != 2:
            continue

        tf_gene = parts[0].strip()
        target_gene = parts[1].strip()

        # Extract evidence
        sources = extract_sources(row, header_map)

        # Skip if only TRED evidence (no sources after filtering)
        if not sources:
            continue

        # Collect PMIDs from various columns
        pmids = set()
        pmid_columns = [
            "[ExTRI] PMID",
            "[HTRI] PMID",
            "[TRRUST] PMID",
            "[TFactS] PMID",
            "[GOA] PMID",
            "[IntAct] PMID",
            "[SIGNOR] PMID",
            "[CytReg] PMID",
            "[GEREDB] PMID",
            "[NTNU Curated] PMID",
            "[Pavlidis2021] PMID",
            "[DoRothEA_A] PMID",
        ]
        for col_name in pmid_columns:
            if col_name in header_map:
                idx = header_map[col_name]
                if idx < len(row):
                    pmids.update(parse_pmids(row[idx]))

        entry = {
            "id": tf_tg,
            "tf_gene": tf_gene,
            "target_gene": target_gene,
            "sources": sources,
            "pmids": list(pmids)[:10],
            "regulation": extract_regulation(row, header_map),
            "confidence": extract_confidence(row, header_map),
        }

        reference_data.append(entry)

    print(f"Extracted {len(reference_data)} entries")

    # Save reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"Saved to {output_file}")
    return 0


if __name__ == "__main__":
    exit(main())
