#!/usr/bin/env python3
"""
AlphaFold Reference Data Extraction Script

Fetches AlphaFold protein structure predictions from AlphaFold DB API for each UniProt ID.
Creates reference_data.json with complete structure information for test validation.

AlphaFold API: https://alphafold.ebi.ac.uk/api/prediction/{uniprot_id}
"""

import json
import sys
import time
from pathlib import Path
from typing import Optional, Dict
import requests


class AlphaFoldExtractor:
    """Extracts complete AlphaFold structure prediction data from official AlphaFold DB REST API"""

    BASE_URL = "https://alphafold.ebi.ac.uk/api/prediction"

    def __init__(self):
        self.session = requests.Session()
        self.session.headers.update({
            'Accept': 'application/json',
            'User-Agent': 'Biobtree-Test-Suite/1.0'
        })

    def fetch_entry(self, uniprot_id: str) -> Optional[Dict]:
        """
        Fetch COMPLETE AlphaFold structure prediction data from REST API

        Returns the full raw API response including:
        - entryId (AlphaFold model ID)
        - gene
        - uniprotAccession
        - uniprotId
        - uniprotDescription
        - taxId, organismScientificName
        - sequenceVersionDate, latestVersion
        - allVersions[]
        - isReviewed (SwissProt vs TrEMBL)
        - pdbUrl, cifUrl, bcifUrl, mmcifUrl
        - paeImageUrl, paeDocUrl
        - modelCreatedDate
        - amAnnotationsUrl
        - uniprotStart, uniprotEnd, uniprotSequence
        - modelCategory, modelIdentifier

        Preserves all fields for maximum test flexibility.
        """
        try:
            url = f"{self.BASE_URL}/{uniprot_id}"
            response = self.session.get(url, timeout=10)

            if response.status_code == 200:
                data = response.json()
                # AlphaFold API returns array of predictions
                # For Swiss-Prot entries, usually one prediction per protein
                if isinstance(data, list) and len(data) > 0:
                    # Get the first (and typically only) prediction
                    entry = data[0]
                    return {
                        "uniprot_id": uniprot_id,
                        "alphafold_entry": entry
                    }
                else:
                    print(f"Warning: Empty response for {uniprot_id}")
                    return None
            elif response.status_code == 404:
                print(f"Warning: No AlphaFold prediction found for {uniprot_id}")
                return None
            else:
                print(f"Warning: HTTP {response.status_code} for {uniprot_id}")
                return None

        except Exception as e:
            print(f"Error fetching {uniprot_id}: {e}")
            return None

    def extract_all(self, id_file: Path, output_file: Path, limit: int = None):
        """Extract all protein structure predictions and save to JSON"""

        # Read IDs
        with open(id_file, 'r') as f:
            ids = [line.strip() for line in f if line.strip()]

        if limit and limit < len(ids):
            print(f"Found {len(ids)} UniProt IDs, processing first {limit}")
            ids = ids[:limit]
        else:
            print(f"Found {len(ids)} UniProt IDs to process")
        print("=" * 60)

        results = []
        failed = []

        for i, uniprot_id in enumerate(ids, 1):
            print(f"[{i}/{len(ids)}] Fetching {uniprot_id}...", end=" ", flush=True)

            data = self.fetch_entry(uniprot_id)

            if data:
                results.append(data)
                entry = data.get("alphafold_entry", {})
                model_id = entry.get("entryId", "unknown")
                gene = entry.get("gene", "")
                gene_str = f" ({gene})" if gene else ""
                print(f"✓ {model_id}{gene_str}")
            else:
                failed.append(uniprot_id)
                print("✗ (no data)")

            # Rate limiting - be respectful to EBI servers
            if i < len(ids):
                time.sleep(0.5)

        print()
        print("=" * 60)
        print(f"Successfully extracted: {len(results)}/{len(ids)} entries")

        if failed:
            print(f"Failed: {len(failed)} entries")
            print(f"Failed IDs: {', '.join(failed[:10])}" +
                  (f" ... and {len(failed) - 10} more" if len(failed) > 10 else ""))

        # Save results
        with open(output_file, 'w') as f:
            json.dump(results, f, indent=2, ensure_ascii=False)

        print(f"\n✓ Saved to {output_file}")
        print(f"  Total entries: {len(results)}")

        # Calculate statistics
        if results:
            reviewed_count = sum(1 for entry in results
                               if entry.get("alphafold_entry", {}).get("isReviewed", False))
            with_gene = sum(1 for entry in results
                          if entry.get("alphafold_entry", {}).get("gene"))

            print(f"  SwissProt (reviewed): {reviewed_count}")
            print(f"  Entries with gene annotation: {with_gene}")


def main():
    script_dir = Path(__file__).parent
    id_file = script_dir / "alphafold_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Check prerequisites
    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d alphafold test")
        print("Then: cp test_out/reference/alphafold_ids.txt tests/alphafold/")
        return 1

    # Parse command-line arguments
    limit = int(sys.argv[1]) if len(sys.argv) > 1 else None

    print(f"AlphaFold Reference Data Extraction")
    print(f"Input:  {id_file}")
    print(f"Output: {output_file}")
    if limit:
        print(f"Limit:  {limit} entries")
    print()

    # Extract data
    extractor = AlphaFoldExtractor()
    extractor.extract_all(id_file, output_file, limit=limit)

    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("\n\nInterrupted by user")
        sys.exit(1)
