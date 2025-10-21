#!/usr/bin/env python3
"""
FAST sample creation - read only first row group, not entire file!
"""

import pyarrow.parquet as pq
import pandas as pd
from pathlib import Path

base_dir = Path("test_out/raw_data/patents/surechembl/2025-10-01")

print("="*80)
print("FAST SAMPLE CREATION - Reading only first rows!")
print("="*80)

# Read ONLY first row group (much faster!)
print("\nReading patents (first row group only)...")
patents_file = pq.ParquetFile(str(base_dir / "patents.parquet"))
patents_df = patents_file.read_row_group(0).to_pandas().head(10)
print(f"  Got {len(patents_df)} patents")
print(f"  Columns: {list(patents_df.columns)}")

print("\nReading compounds (first row group only)...")
compounds_file = pq.ParquetFile(str(base_dir / "compounds.parquet"))
compounds_df = compounds_file.read_row_group(0).to_pandas().head(50)
print(f"  Got {len(compounds_df)} compounds")

print("\nReading mapping (first row group only)...")
mapping_file = pq.ParquetFile(str(base_dir / "patent_compound_map.parquet"))
mapping_df = mapping_file.read_row_group(0).to_pandas().head(100)
print(f"  Got {len(mapping_df)} mappings")

# Filter to get related data
print("\n" + "="*80)
print("FILTERING...")
print("="*80)

test_patent_ids = set(patents_df['id'])
print(f"Patent IDs: {sorted(test_patent_ids)}")

test_mapping = mapping_df[mapping_df['patent_id'].isin(test_patent_ids)]
print(f"Mappings for these patents: {len(test_mapping)}")

test_compound_ids = set(test_mapping['compound_id'])
print(f"Unique compounds: {len(test_compound_ids)}")

test_compounds = compounds_df[compounds_df['id'].isin(test_compound_ids)]
print(f"Compound records: {len(test_compounds)}")

# Save
output_dir = Path("biobtreev2/test_data/surechembl")
output_dir.mkdir(parents=True, exist_ok=True)

print("\n" + "="*80)
print(f"SAVING to {output_dir.absolute()}")
print("="*80)

patents_df.to_parquet(output_dir / 'patents_sample.parquet', index=False)
print(f"  ✓ patents_sample.parquet ({len(patents_df)} rows)")

test_compounds.to_parquet(output_dir / 'compounds_sample.parquet', index=False)
print(f"  ✓ compounds_sample.parquet ({len(test_compounds)} rows)")

test_mapping.to_parquet(output_dir / 'mapping_sample.parquet', index=False)
print(f"  ✓ mapping_sample.parquet ({len(test_mapping)} rows)")

print("\n" + "="*80)
print("SUCCESS!")
print("="*80)
print(f"\nTest data location: {output_dir.absolute()}")
print(f"  - {len(patents_df)} patents")
print(f"  - {len(test_compounds)} compounds")
print(f"  - {len(test_mapping)} patent-compound mappings")
