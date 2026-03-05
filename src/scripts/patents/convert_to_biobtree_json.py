#!/usr/bin/env python3
"""
Convert SureChEMBL parquet files to JSON format for biobtree ingestion.

This script reads patents, compounds, and mapping parquet files and converts them
to JSON format with the wrapped structure that biobtree's jsparser expects:
- {"patents": [...]}
- {"compounds": [...]}
- {"mappings": [...]}

Optionally merges USPTO-Chem abstracts with SureChEMBL patent data. SureChEMBL
only provides titles, while USPTO-Chem has full abstracts for US patents.

Optimized for low memory usage via streaming (reads/writes in chunks).

Usage:
    python convert_to_biobtree_json.py --input data/patents/surechembl/2025-10-01 \
        --output data/patents/biobtree --verbose

    # With USPTO abstract enrichment:
    python convert_to_biobtree_json.py --input data/patents/surechembl/2025-10-01 \
        --output data/patents/biobtree \
        --uspto-parquet data/patents/uspto_historical.parquet --verbose
"""

import argparse
import json
import sys
from pathlib import Path
from datetime import datetime
from typing import Optional, Dict
import pyarrow.parquet as pq
import pandas as pd

# Chunk size for streaming (number of rows per batch)
BATCH_SIZE = 50000


def load_uspto_abstracts(uspto_parquet: Path) -> Dict[str, str]:
    """
    Load USPTO-Chem abstracts into a lookup dictionary.

    Args:
        uspto_parquet: Path to USPTO parquet file with columns: patent_number, abstract

    Returns:
        Dictionary mapping patent_number → abstract
    """
    print(f"Loading USPTO abstracts from {uspto_parquet}...")
    df = pd.read_parquet(uspto_parquet, columns=['patent_number', 'abstract'])

    # Filter out rows without abstracts
    df = df[df['abstract'].notna() & (df['abstract'] != '')]

    abstracts = dict(zip(df['patent_number'], df['abstract']))
    print(f"Loaded {len(abstracts):,} USPTO abstracts")
    return abstracts


def convert_value(val):
    """Convert pyarrow/numpy types to JSON-serializable Python types."""
    if val is None:
        return None
    if hasattr(val, 'as_py'):  # pyarrow scalar
        return val.as_py()
    if hasattr(val, 'item'):  # numpy scalar
        return val.item()
    if isinstance(val, (datetime,)):
        return val.isoformat()
    return val


def stream_parquet_to_json(
    input_file: Path,
    output_file: Path,
    wrapper_key: str,
    verbose: bool = False,
    uspto_abstracts: Optional[Dict[str, str]] = None
):
    """
    Stream parquet file to JSON without loading everything into memory.

    Args:
        input_file: Input parquet file
        output_file: Output JSON file
        wrapper_key: JSON wrapper key (e.g., "patents")
        verbose: Print detailed progress
        uspto_abstracts: Optional dictionary of patent_number → abstract for enrichment

    Returns:
        Tuple of (records written, abstracts merged count)
    """
    parquet_file = pq.ParquetFile(input_file)
    total_rows = parquet_file.metadata.num_rows
    columns = [col.name for col in parquet_file.schema_arrow]

    if verbose:
        print(f"  Records: {total_rows:,}")
        print(f"  Columns: {', '.join(columns)}")
        print(f"  Writing: {output_file}")
        if uspto_abstracts:
            print(f"  USPTO abstracts available: {len(uspto_abstracts):,}")

    record_count = 0
    abstracts_merged = 0

    with open(output_file, 'w') as f:
        # Write opening structure
        f.write('{"' + wrapper_key + '":[')

        first_record = True
        batches_processed = 0

        # Stream through parquet file in batches
        for batch in parquet_file.iter_batches(batch_size=BATCH_SIZE):
            # Convert batch to Python dicts
            batch_dict = batch.to_pydict()
            batch_len = len(batch_dict[columns[0]])

            for i in range(batch_len):
                record = {col: convert_value(batch_dict[col][i]) for col in columns}

                # Merge USPTO abstract if available
                if uspto_abstracts and 'patent_number' in record:
                    patent_num = record.get('patent_number')
                    if patent_num and patent_num in uspto_abstracts:
                        record['abstract'] = uspto_abstracts[patent_num]
                        abstracts_merged += 1

                if not first_record:
                    f.write(',')
                first_record = False

                # Write record as compact JSON (no indent for speed/size)
                json.dump(record, f, separators=(',', ':'), default=str)
                record_count += 1

            batches_processed += 1
            if verbose and batches_processed % 20 == 0:
                progress = (record_count / total_rows) * 100
                merged_pct = (abstracts_merged / record_count * 100) if record_count > 0 else 0
                print(f"    Progress: {record_count:,}/{total_rows:,} ({progress:.1f}%), abstracts: {abstracts_merged:,} ({merged_pct:.1f}%)")

        # Write closing structure
        f.write(']}')

    return record_count, abstracts_merged


def convert_parquet_to_json(
    input_dir: Path,
    output_dir: Path,
    verbose: bool = False,
    uspto_parquet: Optional[Path] = None
):
    """
    Convert parquet files to JSON format for biobtree.

    Args:
        input_dir: Directory containing parquet files
        output_dir: Directory where JSON files will be written
        verbose: Print detailed progress
        uspto_parquet: Optional path to USPTO parquet file for abstract enrichment
    """

    if verbose:
        print(f"\n{'='*80}")
        print(f"Converting SureChEMBL Parquet to Biobtree JSON (Streaming)")
        print(f"{'='*80}")
        print(f"Input directory:  {input_dir}")
        print(f"Output directory: {output_dir}")
        print(f"Batch size: {BATCH_SIZE:,} rows")
        if uspto_parquet:
            print(f"USPTO abstracts:  {uspto_parquet}")
        print(f"Started at: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")

    # Load USPTO abstracts if provided
    uspto_abstracts = None
    if uspto_parquet and uspto_parquet.exists():
        uspto_abstracts = load_uspto_abstracts(uspto_parquet)

    # Create output directory
    output_dir.mkdir(parents=True, exist_ok=True)

    # Define file mappings
    conversions = [
        {
            'input': 'patents.parquet',
            'output': 'patents.json',
            'wrapper_key': 'patents',
            'description': 'Patents metadata',
            'merge_abstracts': True  # Enable abstract merging for patents
        },
        {
            'input': 'compounds.parquet',
            'output': 'compounds.json',
            'wrapper_key': 'compounds',
            'description': 'Chemical compounds',
            'merge_abstracts': False
        },
        {
            'input': 'patent_compound_map.parquet',
            'output': 'mapping.json',
            'wrapper_key': 'mappings',
            'description': 'Patent-compound mappings',
            'merge_abstracts': False
        }
    ]

    stats = {}

    for conversion in conversions:
        input_file = input_dir / conversion['input']
        output_file = output_dir / conversion['output']

        if not input_file.exists():
            print(f"  WARNING: {conversion['input']} not found, skipping...")
            continue

        if verbose:
            print(f"\n{conversion['description']}:")
            print(f"  Reading: {input_file}")

        # Stream convert parquet to JSON
        # Pass USPTO abstracts only for patents file
        abstracts_for_conversion = uspto_abstracts if conversion.get('merge_abstracts') else None
        record_count, abstracts_merged = stream_parquet_to_json(
            input_file, output_file, conversion['wrapper_key'], verbose, abstracts_for_conversion
        )

        # Collect statistics
        stats[conversion['wrapper_key']] = {
            'records': record_count,
            'file_size_mb': output_file.stat().st_size / (1024 * 1024)
        }
        if abstracts_merged > 0:
            stats[conversion['wrapper_key']]['abstracts_merged'] = abstracts_merged

        if verbose:
            msg = f"  Completed ({stats[conversion['wrapper_key']]['file_size_mb']:.2f} MB)"
            if abstracts_merged > 0:
                msg += f", {abstracts_merged:,} abstracts merged"
            print(msg)

    # Write summary
    summary_file = output_dir / 'conversion_summary.json'
    summary = {
        'conversion_date': datetime.now().isoformat(),
        'input_directory': str(input_dir),
        'output_directory': str(output_dir),
        'statistics': stats
    }

    with open(summary_file, 'w') as f:
        json.dump(summary, f, indent=2)

    if verbose:
        print(f"\n{'='*80}")
        print(f"Conversion Summary:")
        print(f"{'='*80}")
        for key, stat in stats.items():
            print(f"  {key:15s}: {stat['records']:8,} records ({stat['file_size_mb']:8.2f} MB)")
        print(f"\nSummary written to: {summary_file}")
        print(f"Completed at: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        print(f"{'='*80}\n")

    return stats


def main():
    parser = argparse.ArgumentParser(
        description='Convert SureChEMBL parquet files to JSON for biobtree',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Convert parquet files in a release directory
  python convert_to_biobtree_json.py \\
      --input data/patents/surechembl/2025-10-01 \\
      --output data/patents/biobtree

  # Convert with USPTO abstract enrichment
  python convert_to_biobtree_json.py \\
      --input data/patents/surechembl/2025-10-01 \\
      --output data/patents/biobtree \\
      --uspto-parquet data/patents/uspto_historical.parquet \\
      --verbose
        """
    )

    parser.add_argument(
        '--input',
        '-i',
        type=Path,
        required=True,
        help='Input directory containing parquet files'
    )

    parser.add_argument(
        '--output',
        '-o',
        type=Path,
        required=True,
        help='Output directory for JSON files'
    )

    parser.add_argument(
        '--uspto-parquet',
        type=Path,
        default=None,
        help='Optional USPTO parquet file for abstract enrichment (from process_uspto_json.py)'
    )

    parser.add_argument(
        '--verbose',
        '-v',
        action='store_true',
        help='Print detailed progress information'
    )

    args = parser.parse_args()

    # Validate input directory
    if not args.input.exists():
        print(f"ERROR: Input directory does not exist: {args.input}", file=sys.stderr)
        sys.exit(1)

    if not args.input.is_dir():
        print(f"ERROR: Input path is not a directory: {args.input}", file=sys.stderr)
        sys.exit(1)

    # Validate USPTO parquet if provided
    if args.uspto_parquet and not args.uspto_parquet.exists():
        print(f"WARNING: USPTO parquet file not found: {args.uspto_parquet}", file=sys.stderr)
        print(f"         Continuing without USPTO abstract enrichment", file=sys.stderr)
        args.uspto_parquet = None

    try:
        stats = convert_parquet_to_json(
            args.input, args.output, args.verbose, args.uspto_parquet
        )

        if not stats:
            print("WARNING: No files were converted", file=sys.stderr)
            sys.exit(1)

        sys.exit(0)

    except Exception as e:
        print(f"ERROR: Conversion failed: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == '__main__':
    main()
