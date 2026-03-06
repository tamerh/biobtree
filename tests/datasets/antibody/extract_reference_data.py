#!/usr/bin/env python3
"""
Extract Reference Data for Antibody Tests

Note: The antibody dataset integrates 4 different sources (TheraSAbDab, SAbDab,
IMGT/GENE-DB, IMGT/LIGM-DB), each with different data formats and no unified API.

Reference data for testing is extracted directly from the biobtree test database
rather than from external APIs, since:
- TheraSAbDab: CSV file, no REST API
- SAbDab: TSV file, no official REST API
- IMGT/GENE-DB: FASTA file from FTP
- IMGT/LIGM-DB: Compressed FASTA file from FTP

The test suite validates the integrated data structure in biobtree rather than
comparing against external API responses.
"""

import json
from pathlib import Path

def main():
    """Create placeholder reference data file"""

    reference_data = {
        "note": "Antibody dataset integrates 4 sources without unified REST APIs",
        "sources": {
            "therasabdab": "CSV file - no REST API",
            "sabdab": "TSV file - no official REST API",
            "imgt_gene": "FASTA from FTP",
            "imgt_ligm": "Compressed FASTA from FTP"
        },
        "test_approach": "Validate integrated biobtree data structure",
        "test_ids_file": "antibody_ids.txt"
    }

    output_file = Path(__file__).parent / "reference_data.json"

    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"Created placeholder reference data: {output_file}")
    print("\nNote: Antibody tests validate biobtree integration directly")
    print("rather than comparing against external API responses.")

if __name__ == "__main__":
    main()
