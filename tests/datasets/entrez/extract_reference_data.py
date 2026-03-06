#!/usr/bin/env python3
"""
Extract Entrez Gene reference data from NCBI

This fetches gene information for the IDs in entrez_ids.txt using NCBI E-utilities.
Used for reference when writing test cases.

Note: Uses NCBI E-utilities API (efetch) to get gene records.
Rate limiting is applied to be respectful to NCBI servers.
"""

import json
import sys
import time
import gzip
import io
from pathlib import Path
from typing import List, Dict, Optional, Any
from urllib.request import urlopen, Request
from urllib.error import URLError, HTTPError
from xml.etree import ElementTree as ET


class EntrezExtractor:
    """Extract Entrez Gene reference data from NCBI FTP gene_info file"""

    # Using NCBI FTP for gene_info (tab-delimited, simpler to parse)
    GENE_INFO_URL = "https://ftp.ncbi.nlm.nih.gov/gene/DATA/gene_info.gz"

    def __init__(self, ids_file: Path, output_file: Path):
        self.ids_file = ids_file
        self.output_file = output_file
        self.stats = {
            "total": 0,
            "success": 0,
            "failed": 0,
            "no_data": 0
        }

    def load_ids(self) -> List[str]:
        """Load Entrez Gene IDs from file"""
        if not self.ids_file.exists():
            raise FileNotFoundError(f"{self.ids_file} not found")

        with open(self.ids_file) as f:
            ids = [line.strip() for line in f if line.strip()]

        self.stats["total"] = len(ids)
        return ids

    def fetch_from_gene_info(self, gene_ids: List[str]) -> List[Dict[str, Any]]:
        """
        Fetch gene info by downloading and parsing gene_info.gz from NCBI FTP
        More reliable than E-utilities API for bulk data
        """
        print(f"Downloading gene_info.gz from NCBI FTP...")
        print(f"URL: {self.GENE_INFO_URL}")
        print("(This may take a moment for large file)")
        print()

        target_ids = set(gene_ids)
        entries = []

        try:
            req = Request(self.GENE_INFO_URL)
            req.add_header('User-Agent', 'biobtree-test/1.0')

            with urlopen(req, timeout=60) as response:
                # Read and decompress
                compressed_data = response.read()
                with gzip.GzipFile(fileobj=io.BytesIO(compressed_data)) as f:
                    # Parse header
                    header_line = f.readline().decode('utf-8').strip()
                    if header_line.startswith('#'):
                        header_line = header_line[1:]  # Remove leading #
                    headers = header_line.split('\t')

                    # Find column indices
                    col_map = {h: i for i, h in enumerate(headers)}

                    line_count = 0
                    for line in f:
                        line_count += 1
                        if line_count % 1000000 == 0:
                            print(f"  Processed {line_count:,} lines, found {len(entries)} matches...")

                        line = line.decode('utf-8').strip()
                        if not line:
                            continue

                        fields = line.split('\t')
                        gene_id = fields[col_map.get('GeneID', 1)]

                        if gene_id in target_ids:
                            entry = {}
                            for header, idx in col_map.items():
                                if idx < len(fields):
                                    entry[header] = fields[idx]
                            entries.append(entry)
                            self.stats["success"] += 1
                            print(f"  Found gene {gene_id}: {entry.get('Symbol', 'N/A')}")

                            # Early exit if we found all
                            if len(entries) == len(target_ids):
                                break

            # Check for missing IDs
            found_ids = {e.get('GeneID') for e in entries}
            missing_ids = target_ids - found_ids
            if missing_ids:
                print(f"\n  Warning: {len(missing_ids)} IDs not found: {missing_ids}")
                self.stats["no_data"] = len(missing_ids)

        except (URLError, HTTPError) as e:
            print(f"Error downloading gene_info: {e}")
            self.stats["failed"] = len(gene_ids)
            return []
        except Exception as e:
            print(f"Error processing gene_info: {e}")
            self.stats["failed"] = len(gene_ids)
            return []

        return entries

    def extract_all(self) -> List[Dict]:
        """Extract all entries"""
        print("=" * 60)
        print("  Entrez Gene Reference Data Extraction")
        print("=" * 60)
        print()

        # Load IDs
        ids = self.load_ids()
        print(f"Found {len(ids)} Entrez Gene IDs to fetch")
        print()

        # Fetch data from NCBI
        entries = self.fetch_from_gene_info(ids)

        return entries

    def save_results(self, entries: List[Dict]):
        """Save results to JSON file"""
        output = {
            "metadata": {
                "total_ids": self.stats["total"],
                "fetched": self.stats["success"],
                "failed": self.stats["failed"],
                "no_data": self.stats["no_data"],
                "date": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "source": "NCBI FTP gene_info.gz"
            },
            "entries": entries
        }

        with open(self.output_file, "w") as f:
            json.dump(output, f, indent=2, ensure_ascii=False)

    def print_summary(self):
        """Print extraction summary"""
        print()
        print("=" * 60)
        print("Fetch Summary:")
        print(f"  Total:     {self.stats['total']}")
        print(f"  Success:   {self.stats['success']}")
        print(f"  No data:   {self.stats['no_data']}")
        print(f"  Failed:    {self.stats['failed']}")
        print("=" * 60)
        print()

        if self.stats["success"] > 0:
            size = self.output_file.stat().st_size / 1024
            print(f"Saved to: {self.output_file} ({size:.1f} KB)")
            print()

            # Show sample
            with open(self.output_file) as f:
                data = json.load(f)
                if data["entries"]:
                    first = data["entries"][0]
                    print("Sample entry (first ID):")
                    print(f"  Gene ID:     {first.get('GeneID')}")
                    print(f"  Symbol:      {first.get('Symbol')}")
                    print(f"  Description: {first.get('description', 'N/A')[:60]}...")
                    print(f"  Type:        {first.get('type_of_gene')}")
                    print(f"  Chromosome:  {first.get('chromosome')}")
                    print(f"  Tax ID:      {first.get('tax_id')}")
                    print()

        print("=" * 60)
        print("Reference data extraction complete")
        print()
        print("Files created:")
        print("  - entrez_ids.txt        (Gene IDs)")
        print("  - reference_data.json   (Full gene entries)")
        print()
        print("Use these files to write test cases in test_cases.json")
        print("=" * 60)


def main():
    # Paths
    script_dir = Path(__file__).parent
    ids_file = script_dir / "entrez_ids.txt"
    output_file = script_dir / "reference_data.json"

    try:
        extractor = EntrezExtractor(ids_file, output_file)
        entries = extractor.extract_all()

        if entries:
            extractor.save_results(entries)
            extractor.print_summary()
            sys.exit(0)
        else:
            print("\nError: No data fetched successfully")
            sys.exit(1)

    except KeyboardInterrupt:
        print("\n\nInterrupted by user")
        sys.exit(1)
    except Exception as e:
        print(f"\nError: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
