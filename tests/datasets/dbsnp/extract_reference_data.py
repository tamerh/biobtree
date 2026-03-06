#!/usr/bin/env python3
"""
Extract reference data from dbSNP VCF for test validation

This script parses the local VCF file to create reference data for test validation.
Unlike GWAS which has a REST API, dbSNP data comes from VCF files,
so we extract the reference data directly from the same source.

VCF format: https://www.ncbi.nlm.nih.gov/variation/docs/human_variation_vcf/
"""

import json
import gzip
from pathlib import Path


def parse_vcf_info(info_str):
    """Parse VCF INFO field into dictionary"""
    info = {}
    for field in info_str.split(';'):
        if '=' in field:
            key, value = field.split('=', 1)
            info[key] = value
        else:
            info[field] = True
    return info


def extract_dbsnp_reference_data():
    """Extract reference data from VCF file for test SNP IDs"""

    script_dir = Path(__file__).parent
    vcf_file = script_dir / "GCF_000001405.40.gz"
    # Resolve path relative to biobtreev2 root
    id_file = script_dir.parent.parent.parent / "test_out/reference/dbsnp_ids.txt"
    output_file = script_dir / "reference_data.json"

    if not vcf_file.exists():
        print(f"Error: {vcf_file} not found")
        return 1

    if not id_file.exists():
        print(f"Error: {id_file} not found")
        print("Run: ./biobtree -d dbsnp test")
        return 1

    # Read test IDs
    with open(id_file, 'r') as f:
        test_ids = set(line.strip() for line in f if line.strip())

    print(f"Extracting reference data for {len(test_ids)} SNPs from VCF...")

    reference_data = []
    found_ids = set()

    # Parse VCF file
    with gzip.open(vcf_file, 'rt') as f:
        for line in f:
            # Skip headers
            if line.startswith('#'):
                continue

            # Parse VCF line
            fields = line.strip().split('\t')
            if len(fields) < 8:
                continue

            chrom, pos, rs_id, ref, alt, qual, filt, info = fields[:8]

            # Check if this is one of our test SNPs
            if rs_id not in test_ids:
                continue

            found_ids.add(rs_id)

            # Parse INFO field
            info_dict = parse_vcf_info(info)

            # Extract relevant fields
            snp_data = {
                "rsId": rs_id,
                "chromosome": chrom,
                "position": int(pos),
                "refAllele": ref,
                "altAllele": alt,
                "_test_metadata": {
                    "identifier": rs_id,
                    "source": "VCF file"
                }
            }

            # Add INFO fields if present
            if 'dbSNPBuildID' in info_dict:
                snp_data["buildId"] = info_dict['dbSNPBuildID']

            if 'AF' in info_dict:
                try:
                    snp_data["alleleFrequency"] = float(info_dict['AF'])
                except ValueError:
                    pass

            if 'GENEINFO' in info_dict:
                # GENEINFO format: "GENE:GENEID"
                parts = info_dict['GENEINFO'].split(':')
                if len(parts) >= 2:
                    snp_data["geneName"] = parts[0]
                    snp_data["geneId"] = parts[1]

            if 'VC' in info_dict:
                snp_data["variantClass"] = info_dict['VC']

            if 'CLNSIG' in info_dict:
                snp_data["clinicalSignificance"] = info_dict['CLNSIG']

            reference_data.append(snp_data)

            print(f"  ✓ {rs_id}")

    # Check for missing IDs
    missing = test_ids - found_ids
    if missing:
        print(f"\nWarning: {len(missing)} SNPs not found in VCF:")
        for rs_id in sorted(missing):
            print(f"  - {rs_id}")

    # Save reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"\nSaved reference data to {output_file}")
    print(f"Successfully extracted {len(reference_data)}/{len(test_ids)} SNPs")

    return 0


if __name__ == "__main__":
    exit(extract_dbsnp_reference_data())
