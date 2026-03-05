#!/usr/bin/env python3
"""
Process USPTO-Chem Historical JSON Data for Biobtree

Reads downloaded USPTO-Chem JSON files and converts them to a single parquet file
with patent text data (title, abstract) for enriching SureChEMBL patents.

Output: Single parquet file with columns:
    - patent_number (e.g., US-20040019981-A1)
    - title
    - abstract
    - publication_date

Usage:
    python process_uspto_json.py \\
        --input-dir data/patents/uspto_historical \\
        --output data/patents/uspto_historical.parquet
"""

import os
import sys
import json
import argparse
from pathlib import Path
from typing import Set, List, Dict, Optional
from datetime import datetime
import pandas as pd
from tqdm import tqdm
import psutil


def log(message: str) -> None:
    """Print message with timestamp."""
    timestamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
    print(f"[{timestamp}] {message}", flush=True)


def log_memory_usage() -> None:
    """Log current memory usage."""
    process = psutil.Process()
    mem_info = process.memory_info()
    mem_mb = mem_info.rss / 1024 / 1024
    log(f"Memory usage: {mem_mb:.1f} MB")


def extract_patent_number(patent_json: Dict) -> Optional[str]:
    """
    Extract patent number in SureChEMBL format from USPTO-Chem JSON.

    Format: US-{doc_number}-{kind}
    Example: US-20040019981-A1
    """
    try:
        pub = patent_json.get('publication', {})
        doc_number = pub.get('doc_number', '')
        kind = pub.get('kind', '')

        if doc_number and kind:
            return f"US-{doc_number}-{kind}"

        return None
    except Exception:
        return None


def process_json_file(json_path: Path) -> List[Dict]:
    """Process a single USPTO-Chem JSON file."""
    patents = []

    try:
        with open(json_path) as f:
            data = json.load(f)

        for patent_json in data:
            patent_number = extract_patent_number(patent_json)
            if not patent_number:
                continue

            title = patent_json.get('title', '')
            abstract = patent_json.get('abstract', '')
            pub_date = patent_json.get('publication', {}).get('date', '')

            if title or abstract:
                patents.append({
                    'patent_number': patent_number,
                    'title': title or None,
                    'abstract': abstract or None,
                    'publication_date': pub_date or None
                })

    except Exception as e:
        log(f"ERROR processing {json_path.name}: {e}")

    return patents


def find_all_json_files(input_dir: Path) -> List[Path]:
    """Find all JSON files in input directory."""
    json_files = []

    for json_file in input_dir.rglob('*.json'):
        json_files.append(json_file)

    json_files.sort()
    return json_files


def process_all_files(
    input_dir: Path,
    output_file: Path,
    limit_files: Optional[int] = None,
    batch_size: int = 500
) -> int:
    """Process all USPTO-Chem JSON files in batches."""
    log(f"Finding JSON files in {input_dir}")
    log_memory_usage()

    json_files = find_all_json_files(input_dir)

    if not json_files:
        log("ERROR: No JSON files found!")
        return 0

    log(f"Found {len(json_files):,} JSON files")

    if limit_files and len(json_files) > limit_files:
        log(f"Limiting to {limit_files} files (test mode)")
        json_files = json_files[:limit_files]

    batch_files = []
    all_batches = []
    total_processed = 0
    total_matched = 0
    batch_num = 0

    for json_file in tqdm(json_files, desc="Processing JSON files"):
        patents = process_json_file(json_file)

        total_processed += 1
        total_matched += len(patents)

        batch_files.extend(patents)

        if len(batch_files) >= batch_size or total_processed == len(json_files):
            if batch_files:
                batch_num += 1
                batch_df = pd.DataFrame(batch_files)
                batch_df = batch_df.drop_duplicates(subset='patent_number', keep='first')
                all_batches.append(batch_df)

                log(f"Batch {batch_num}: {len(batch_df):,} patents, Total so far: {total_matched:,}")
                batch_files = []

        if total_processed % 500 == 0:
            log_memory_usage()

    log("\n" + "="*60)
    log("Combining batches...")
    log("="*60)

    if all_batches:
        log(f"Concatenating {len(all_batches)} batches...")
        df = pd.concat(all_batches, ignore_index=True)

        original_len = len(df)
        df = df.drop_duplicates(subset='patent_number', keep='first')

        if len(df) < original_len:
            log(f"Removed {original_len - len(df):,} duplicate patents")

        log(f"Final dataset: {len(df):,} unique patents")
        log_memory_usage()

        output_file.parent.mkdir(parents=True, exist_ok=True)
        df.to_parquet(output_file, index=False)

        log(f"\nSaved {len(df):,} patents to {output_file}")
        log(f"File size: {output_file.stat().st_size / 1024 / 1024:.1f} MB")

        return len(df)
    else:
        log("WARNING: No patents processed!")
        return 0


def main():
    parser = argparse.ArgumentParser(description='Process USPTO-Chem JSON files to parquet')
    parser.add_argument('--input-dir', type=str, required=True,
                       help='Directory containing USPTO-Chem JSON files')
    parser.add_argument('--output', type=str, required=True,
                       help='Output parquet file')
    parser.add_argument('--limit-files', type=int,
                       help='Limit number of files to process (test mode)')

    args = parser.parse_args()

    log("="*60)
    log("USPTO-Chem JSON Processing (Biobtree)")
    log("="*60)
    log(f"Input directory: {args.input_dir}")
    log(f"Output: {args.output}")

    total_patents = process_all_files(
        input_dir=Path(args.input_dir),
        output_file=Path(args.output),
        limit_files=args.limit_files
    )

    if total_patents == 0:
        log("WARNING: No patents processed")
        sys.exit(1)

    log(f"\nTotal patents: {total_patents:,}")
    log("\nProcessing complete!")


if __name__ == '__main__':
    main()
