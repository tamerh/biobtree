#!/usr/bin/env python3
"""
SwissLipids Reference Data Extraction Script

Fetches complete SwissLipids data from API for each test ID, including ALL columns
from all 6 TSV files to serve as reference data for testing.

Creates reference_data.json with complete TSV data (not just parsed fields).
"""

import sys
import json
import time
import gzip
from pathlib import Path
from typing import Optional, Dict, List
from io import BytesIO

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class SwissLipidsExtractor:
    """Extract complete SwissLipids data from API"""

    BASE_URL = "https://www.swisslipids.org/api/file.php"

    # All 6 TSV files available from SwissLipids API
    TSV_FILES = {
        'lipids': 'lipids.tsv',
        'lipids2uniprot': 'lipids2uniprot.tsv',
        'go': 'go.tsv',
        'tissues': 'tissues.tsv',
        'enzymes': 'enzymes.tsv',
        'evidences': 'evidences.tsv'
    }

    def __init__(self, rate_limit: float = 0.5):
        self.rate_limit = rate_limit
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': '*/*',
            'User-Agent': 'BiobtreeTestSuite/1.0'
        })
        # Cache TSV data to avoid re-downloading for each ID
        self.tsv_cache = {}

    def download_tsv(self, filename: str) -> Optional[List[List[str]]]:
        """Download and parse a TSV file from SwissLipids API"""

        if filename in self.tsv_cache:
            print(f"  Using cached data for {filename}")
            return self.tsv_cache[filename]

        print(f"  Downloading {filename} from API...", end=" ")
        sys.stdout.flush()

        url = f"{self.BASE_URL}?cas=download_files&file={filename}"

        try:
            response = self.session.get(url, timeout=60, stream=True)

            if response.status_code != 200:
                print(f"✗ (HTTP {response.status_code})")
                return None

            # Read first 2 bytes to check if gzipped
            peek_bytes = response.raw.read(2)
            response.raw._fp.fp.raw._sock.close()  # Close connection

            # Re-download to get full content
            response = self.session.get(url, timeout=60)
            content = response.content

            # Check for gzip magic number (0x1f 0x8b)
            if len(peek_bytes) >= 2 and peek_bytes[0] == 0x1f and peek_bytes[1] == 0x8b:
                # Decompress gzipped content
                content = gzip.decompress(content)

            # Parse TSV (handle encoding errors gracefully)
            lines = content.decode('utf-8', errors='replace').strip().split('\n')
            data = []

            for line in lines:
                if line.strip():
                    cols = line.split('\t')
                    data.append(cols)

            print(f"✓ ({len(data)} rows)")
            self.tsv_cache[filename] = data
            return data

        except requests.exceptions.RequestException as e:
            print(f"✗ ({e})")
            return None

    def extract_lipid_data(self, slm_id: str, all_tsv_data: Dict[str, List[List[str]]]) -> Optional[Dict]:
        """Extract complete data for a single SwissLipids ID from all TSV files"""

        result = {
            "id": slm_id,
            "lipids_data": None,
            "lipids2uniprot": [],
            "go": [],
            "tissues": [],
            "enzymes": [],
            "evidences": []
        }

        # Extract from main lipids.tsv (column 0 is Lipid ID)
        if 'lipids' in all_tsv_data:
            for row in all_tsv_data['lipids']:
                if row and row[0] == slm_id:
                    # Store all 29 columns as-is
                    result['lipids_data'] = {
                        'columns': row,
                        'headers': [
                            "Lipid ID", "Level", "Name", "Abbreviation*", "Synonyms*",
                            "Lipid class*", "Parent", "Components*", "SMILES (pH7.3)",
                            "InChI (pH7.3)", "InChI key (pH7.3)", "Formula (pH7.3)",
                            "Charge (pH7.3)", "Mass (pH7.3)", "Exact Mass (neutral form)",
                            "Exact m/z of [M.]+", "Exact m/z of [M+H]+", "Exact m/z of [M+K]+",
                            "Exact m/z of [M+Na]+", "Exact m/z of [M+Li]+", "Exact m/z of [M+NH4]+",
                            "Exact m/z of [M-H]-", "Exact m/z of [M+Cl]-", "Exact m/z of [M+OAc]-",
                            "CHEBI", "LIPID MAPS", "HMDB", "MetaNetX", "PMID"
                        ]
                    }
                    break

        # Extract from lipids2uniprot.tsv (column 0 is Lipid ID)
        if 'lipids2uniprot' in all_tsv_data:
            for row in all_tsv_data['lipids2uniprot']:
                if row and len(row) >= 2 and row[0] == slm_id:
                    result['lipids2uniprot'].append(row)

        # Extract from go.tsv (column 0 is Lipid ID)
        if 'go' in all_tsv_data:
            for row in all_tsv_data['go']:
                if row and len(row) >= 2 and row[0] == slm_id:
                    result['go'].append(row)

        # Extract from tissues.tsv (column 0 is Lipid ID)
        if 'tissues' in all_tsv_data:
            for row in all_tsv_data['tissues']:
                if row and len(row) >= 2 and row[0] == slm_id:
                    result['tissues'].append(row)

        # Extract from enzymes.tsv (column 0 is Lipid ID)
        if 'enzymes' in all_tsv_data:
            for row in all_tsv_data['enzymes']:
                if row and len(row) >= 2 and row[0] == slm_id:
                    result['enzymes'].append(row)

        # Extract from evidences.tsv (column 0 is Lipid ID)
        if 'evidences' in all_tsv_data:
            for row in all_tsv_data['evidences']:
                if row and len(row) >= 2 and row[0] == slm_id:
                    result['evidences'].append(row)

        return result if result['lipids_data'] else None

    def extract_all(self, id_file: Path, output_file: Path, limit: Optional[int] = None):
        """Extract all SwissLipids data from ID list"""
        print(f"Reading SwissLipids IDs from {id_file}")

        with open(id_file, 'r') as f:
            slm_ids = [line.strip() for line in f if line.strip()]

        if limit:
            slm_ids = slm_ids[:limit]
            print(f"Limiting to first {limit} IDs")

        print(f"Found {len(slm_ids)} SwissLipids IDs")
        print(f"Fetching from SwissLipids API: {self.BASE_URL}")
        print()

        # Download all TSV files once
        print("=" * 60)
        print("Step 1: Downloading all TSV files from API")
        print("=" * 60)
        all_tsv_data = {}
        for key, filename in self.TSV_FILES.items():
            data = self.download_tsv(filename)
            if data:
                all_tsv_data[key] = data
            time.sleep(self.rate_limit)

        print()
        print("=" * 60)
        print("Step 2: Extracting data for each test ID")
        print("=" * 60)

        results = []
        failed = []

        for i, slm_id in enumerate(slm_ids, 1):
            print(f"[{i}/{len(slm_ids)}] Processing {slm_id}...", end=" ")
            sys.stdout.flush()

            entry = self.extract_lipid_data(slm_id, all_tsv_data)

            if entry:
                results.append(entry)
                # Show summary of what was found
                counts = []
                if entry['lipids2uniprot']:
                    counts.append(f"{len(entry['lipids2uniprot'])} UniProt")
                if entry['go']:
                    counts.append(f"{len(entry['go'])} GO")
                if entry['tissues']:
                    counts.append(f"{len(entry['tissues'])} tissues")
                if entry['enzymes']:
                    counts.append(f"{len(entry['enzymes'])} enzymes")
                if entry['evidences']:
                    counts.append(f"{len(entry['evidences'])} evidences")

                summary = ", ".join(counts) if counts else "no xrefs"
                print(f"✓ ({summary})")
            else:
                failed.append(slm_id)
                print("✗ (not found)")

        print()
        print("=" * 60)
        print(f"Successfully extracted: {len(results)}/{len(slm_ids)}")
        if failed:
            print(f"Failed: {len(failed)} IDs")
            print(f"Failed IDs: {', '.join(failed[:10])}{'...' if len(failed) > 10 else ''}")

        # Save results with metadata wrapper
        print(f"Saving to {output_file}")
        output_data = {
            "metadata": {
                "total_ids": len(slm_ids),
                "fetched": len(results),
                "failed": len(failed),
                "note": "Complete TSV data with all columns preserved for reference"
            },
            "entries": results
        }
        with open(output_file, 'w') as f:
            json.dump(output_data, f, indent=2)

        # Print sample
        if results:
            print()
            print("=" * 60)
            print("Sample entry (first ID):")
            first = results[0]
            print(f"  SwissLipids ID: {first['id']}")
            if first['lipids_data']:
                cols = first['lipids_data']['columns']
                print(f"  Name:           {cols[2] if len(cols) > 2 else 'N/A'}")
                print(f"  Abbreviation:   {cols[3] if len(cols) > 3 else 'N/A'}")
                print(f"  Level:          {cols[1] if len(cols) > 1 else 'N/A'}")
                print(f"  Total columns:  {len(cols)}")
            print(f"  UniProt xrefs:  {len(first['lipids2uniprot'])}")
            print(f"  GO xrefs:       {len(first['go'])}")
            print(f"  Tissue xrefs:   {len(first['tissues'])}")
            print(f"  Enzyme xrefs:   {len(first['enzymes'])}")
            print(f"  Evidence xrefs: {len(first['evidences'])}")
            print("=" * 60)

        print("✓ Extraction complete")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "swisslipids_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d swisslipids test")
        print("Then: cp test_out/reference/swisslipids_ids.txt tests/datasets/swisslipids/")
        return 1

    # Allow limiting number of IDs to extract (for quick testing)
    limit = None
    if len(sys.argv) > 1:
        try:
            limit = int(sys.argv[1])
            print(f"Will extract first {limit} IDs only")
        except ValueError:
            print(f"Invalid limit argument: {sys.argv[1]}")
            return 1

    extractor = SwissLipidsExtractor(rate_limit=0.5)
    extractor.extract_all(id_file, output_file, limit=limit)

    return 0


if __name__ == "__main__":
    sys.exit(main())
