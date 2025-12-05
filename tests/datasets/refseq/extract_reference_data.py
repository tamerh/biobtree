#!/usr/bin/env python3
"""
Extract RefSeq reference data from NCBI Entrez for test validation.

Uses NCBI E-utilities API to fetch nucleotide/protein records.
Rate limited to 3 requests/second per NCBI guidelines.

Usage:
    cd tests/datasets/refseq
    python3 extract_reference_data.py
"""

import json
import time
import sys
import os
import re
from urllib.request import urlopen
from urllib.error import HTTPError, URLError
from xml.etree import ElementTree

# NCBI E-utilities base URL
EUTILS_BASE = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"

def load_test_ids(filepath="refseq_ids.txt"):
    """Load RefSeq accessions from file."""
    ids = []
    with open(filepath, 'r') as f:
        for line in f:
            acc = line.strip()
            if acc:
                ids.append(acc)
    return ids

def fetch_refseq_record(accession, retries=3):
    """
    Fetch RefSeq record from NCBI nucleotide/protein database.

    Uses efetch with rettype=gb (GenBank format) for nucleotides
    or rettype=gp (GenPept format) for proteins.
    """
    # Determine database based on accession prefix
    if accession.startswith(('NP_', 'XP_', 'WP_', 'YP_', 'AP_')):
        db = "protein"
        rettype = "gp"
    else:
        db = "nuccore"
        rettype = "gb"

    url = f"{EUTILS_BASE}/efetch.fcgi?db={db}&id={accession}&rettype={rettype}&retmode=text"

    for attempt in range(retries):
        try:
            with urlopen(url, timeout=30) as response:
                content = response.read().decode('utf-8')
                return parse_genbank_record(content, accession)
        except HTTPError as e:
            if e.code == 429:  # Rate limited
                time.sleep(2 ** attempt)
                continue
            elif e.code == 400:  # Bad request - ID not found
                print(f"  Warning: {accession} not found (400)")
                return None
            else:
                print(f"  HTTP error {e.code} for {accession}")
                if attempt < retries - 1:
                    time.sleep(1)
                    continue
                return None
        except URLError as e:
            print(f"  URL error for {accession}: {e}")
            if attempt < retries - 1:
                time.sleep(1)
                continue
            return None
        except Exception as e:
            print(f"  Error fetching {accession}: {e}")
            if attempt < retries - 1:
                time.sleep(1)
                continue
            return None

    return None

def parse_genbank_record(content, accession):
    """
    Parse GenBank/GenPept flat file format to extract key fields.
    """
    record = {
        "accession": accession,
        "raw_accession": "",
        "version": "",
        "definition": "",
        "locus_length": 0,
        "mol_type": "",
        "organism": "",
        "taxid": "",
        "gene_symbol": "",
        "gene_id": "",
        "chromosome": "",
        "status": "",
        "keywords": [],
        "protein_id": "",
        "calculated_mol_wt": 0,
        "exon_count": 0,
        "hgnc_id": "",
        "ccds_id": "",
        "is_mane_select": False,
        "is_mane_plus_clinical": False,
        "ensembl_transcript": "",
        "ensembl_protein": ""
    }

    lines = content.split('\n')
    in_features = False
    in_comment = False
    current_feature = ""
    feature_content = []
    comment_content = []

    for line in lines:
        # Parse LOCUS line
        if line.startswith("LOCUS"):
            parts = line.split()
            if len(parts) >= 3:
                record["locus_length"] = int(parts[2]) if parts[2].isdigit() else 0
                for p in parts:
                    if p in ("aa", "bp"):
                        record["mol_type"] = "protein" if p == "aa" else "nucleotide"

        # Parse VERSION line
        elif line.startswith("VERSION"):
            parts = line.split()
            if len(parts) >= 2:
                record["version"] = parts[1]
                record["raw_accession"] = parts[1]

        # Parse DEFINITION line
        elif line.startswith("DEFINITION"):
            record["definition"] = line[12:].strip()
        elif line.startswith("            ") and record["definition"] and not record["version"]:
            record["definition"] += " " + line.strip()

        # Parse KEYWORDS line
        elif line.startswith("KEYWORDS"):
            kw = line[12:].strip()
            record["keywords"] = [k.strip() for k in kw.rstrip('.').split(';') if k.strip()]
            if "MANE Select" in kw:
                record["is_mane_select"] = True
            if "MANE Plus Clinical" in kw:
                record["is_mane_plus_clinical"] = True

        # Parse FEATURES section
        elif line.startswith("FEATURES"):
            in_features = True
            in_comment = False
            continue

        # Parse COMMENT section
        elif line.startswith("COMMENT"):
            in_comment = True
            in_features = False
            comment_content.append(line[12:].strip())
            continue

        # End of features/comment
        elif line.startswith("ORIGIN") or line.startswith("CONTIG"):
            in_features = False
            in_comment = False
            # Parse accumulated comment
            comment_text = " ".join(comment_content)
            if "VALIDATED REFSEQ" in comment_text:
                record["status"] = "VALIDATED"
            elif "REVIEWED REFSEQ" in comment_text:
                record["status"] = "REVIEWED"
            elif "PROVISIONAL REFSEQ" in comment_text:
                record["status"] = "PROVISIONAL"
            elif "PREDICTED REFSEQ" in comment_text or "MODEL REFSEQ" in comment_text:
                record["status"] = "PREDICTED"

            # Extract MANE Ensembl match from comment
            mane_match = re.search(r'MANE Ensembl match\s*::\s*(\S+)/\s*(\S+)', comment_text)
            if mane_match:
                record["ensembl_transcript"] = mane_match.group(1)
                record["ensembl_protein"] = mane_match.group(2)
                record["is_mane_select"] = True
            continue

        # Accumulate comment
        if in_comment and line.startswith("            "):
            comment_content.append(line.strip())

        # Parse features
        if in_features:
            # New feature starts
            if len(line) > 5 and line[0] == ' ' and line[4] == ' ' and line[5] != ' ':
                # Process previous feature
                if current_feature:
                    parse_feature(current_feature, feature_content, record)
                # Start new feature
                parts = line.split()
                if parts:
                    current_feature = parts[0]
                    feature_content = [line]
            elif line.startswith("                     "):
                feature_content.append(line)

    # Process last feature
    if current_feature:
        parse_feature(current_feature, feature_content, record)

    return record

def parse_feature(feature_type, lines, record):
    """Parse a single feature block."""
    content = " ".join(l.strip() for l in lines)

    if feature_type == "source":
        # Extract organism
        match = re.search(r'/organism="([^"]+)"', content)
        if match:
            record["organism"] = match.group(1)

        # Extract taxon ID
        match = re.search(r'/db_xref="taxon:(\d+)"', content)
        if match:
            record["taxid"] = match.group(1)

        # Extract chromosome
        match = re.search(r'/chromosome="([^"]+)"', content)
        if match:
            record["chromosome"] = match.group(1)

    elif feature_type == "gene":
        # Extract gene symbol
        match = re.search(r'/gene="([^"]+)"', content)
        if match:
            record["gene_symbol"] = match.group(1)

        # Extract GeneID
        match = re.search(r'/db_xref="GeneID:(\d+)"', content)
        if match:
            record["gene_id"] = match.group(1)

        # Extract HGNC
        match = re.search(r'/db_xref="HGNC:([^"]+)"', content)
        if match:
            record["hgnc_id"] = "HGNC:" + match.group(1)

    elif feature_type == "CDS":
        # Extract protein ID
        match = re.search(r'/protein_id="([^"]+)"', content)
        if match:
            record["protein_id"] = match.group(1)

        # Extract CCDS
        match = re.search(r'/db_xref="CCDS:([^"]+)"', content)
        if match:
            record["ccds_id"] = match.group(1)

        # Extract GeneID if not already set
        if not record["gene_id"]:
            match = re.search(r'/db_xref="GeneID:(\d+)"', content)
            if match:
                record["gene_id"] = match.group(1)

        # Extract HGNC if not already set
        if not record["hgnc_id"]:
            match = re.search(r'/db_xref="HGNC:([^"]+)"', content)
            if match:
                record["hgnc_id"] = "HGNC:" + match.group(1)

    elif feature_type == "Protein":
        # Extract molecular weight
        match = re.search(r'/calculated_mol_wt=(\d+)', content)
        if match:
            record["calculated_mol_wt"] = int(match.group(1))

    elif feature_type == "exon":
        record["exon_count"] += 1

def main():
    """Main entry point."""
    print("RefSeq Reference Data Extractor")
    print("=" * 50)

    # Load test IDs
    ids_file = "refseq_ids.txt"
    if not os.path.exists(ids_file):
        print(f"Error: {ids_file} not found")
        print("Run: cp ../../../test_out/reference/refseq_ids.txt .")
        sys.exit(1)

    test_ids = load_test_ids(ids_file)
    print(f"Loaded {len(test_ids)} RefSeq accessions")

    # Fetch reference data
    reference_data = []
    success = 0
    failed = 0

    for i, acc in enumerate(test_ids):
        print(f"[{i+1}/{len(test_ids)}] Fetching {acc}...", end=" ", flush=True)

        record = fetch_refseq_record(acc)
        if record:
            reference_data.append(record)
            success += 1
            print("OK")
        else:
            failed += 1
            print("FAILED")

        # Rate limiting - NCBI requires < 3 requests/second
        time.sleep(0.4)

    # Save reference data
    output_file = "reference_data.json"
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print()
    print("=" * 50)
    print(f"Results: {success} succeeded, {failed} failed")
    print(f"Reference data saved to {output_file}")

if __name__ == "__main__":
    main()
