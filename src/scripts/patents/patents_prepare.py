#!/usr/bin/env python3
"""
Patent Data Preparation Orchestrator for Biobtree

Single entry point for preparing patent data. Called automatically by biobtree
when patent data files don't exist, similar to cellxgene extraction.

Workflow:
1. Download SureChEMBL parquet files (if not present)
2. Optionally download USPTO-Chem JSON files (for abstracts)
3. Optionally process USPTO JSON to parquet
4. Convert all to biobtree JSON format

Usage:
    # Basic usage (called by biobtree)
    python patents_prepare.py --output-dir data/patents

    # With USPTO abstract enrichment
    python patents_prepare.py --output-dir data/patents --include-uspto

    # Test mode (limited data)
    python patents_prepare.py --output-dir data/patents --test-mode

    # With logging to file
    python patents_prepare.py --output-dir data/patents --log-file logs/patents.log

Configuration:
    This script is typically invoked by biobtree's patents.go when the
    required JSON files don't exist. Configuration comes from:
    - application.param.json: patentsDataDir, patentsIncludeUSPTO
"""

import argparse
import json
import os
import sys
import time
import logging
from datetime import datetime
from pathlib import Path
from typing import Optional, Dict, List, Set
import requests
from bs4 import BeautifulSoup
import pyarrow.parquet as pq
import pandas as pd
from tqdm import tqdm
import psutil


# Global logger
logger = logging.getLogger('patents_prepare')


def setup_logging(log_file: Optional[str] = None) -> None:
    """Setup logging to both console and optionally a file."""
    # Create formatter
    formatter = logging.Formatter(
        '[%(asctime)s] [%(levelname)s] %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )

    # Console handler
    console_handler = logging.StreamHandler(sys.stdout)
    console_handler.setFormatter(formatter)
    logger.addHandler(console_handler)

    # File handler (if specified) - overwrite each run for clean logs
    if log_file:
        log_path = Path(log_file)
        log_path.parent.mkdir(parents=True, exist_ok=True)
        file_handler = logging.FileHandler(log_file, mode='w')
        file_handler.setFormatter(formatter)
        logger.addHandler(file_handler)

    logger.setLevel(logging.INFO)


def log(message: str) -> None:
    """Log message with timestamp."""
    logger.info(message)


def log_memory_usage() -> None:
    """Log current memory usage."""
    process = psutil.Process()
    mem_info = process.memory_info()
    mem_mb = mem_info.rss / 1024 / 1024
    log(f"Memory usage: {mem_mb:.1f} MB")


# =============================================================================
# SureChEMBL Download
# =============================================================================

SURECHEMBL_BASE_URL = "https://ftp.ebi.ac.uk/pub/databases/chembl/SureChEMBL/bulk_data/"
REQUIRED_FILES = ["patents.parquet", "compounds.parquet", "patent_compound_map.parquet"]
RELEASE_TRACKING_FILE = "surechembl_release.txt"


def get_latest_release() -> Optional[str]:
    """Fetch the latest release from SureChEMBL FTP (just the newest one)."""
    releases = get_available_releases()
    return releases[0] if releases else None


def get_cached_release(cache_dir: Path) -> Optional[str]:
    """Get the release version we have cached."""
    tracking_file = cache_dir / RELEASE_TRACKING_FILE
    if tracking_file.exists():
        return tracking_file.read_text().strip()
    return None


def save_cached_release(cache_dir: Path, release: str) -> None:
    """Save the cached release version."""
    tracking_file = cache_dir / RELEASE_TRACKING_FILE
    tracking_file.write_text(release)


def check_uspto_for_updates(cache_dir: Path) -> Dict[str, any]:
    """
    Check if USPTO data needs updating.

    Simple check: if uspto_historical.parquet exists, consider it up-to-date.
    The parquet file is regenerated from all downloaded JSON files, so if it
    exists, all the data has been processed.

    Returns dict with:
      - has_new_files: bool
      - new_file_count: int
      - reason: str
    """
    uspto_parquet = cache_dir / "uspto_historical.parquet"

    if uspto_parquet.exists():
        # Check file age - if less than 30 days old, consider up-to-date
        import time
        file_age_days = (time.time() - uspto_parquet.stat().st_mtime) / 86400
        if file_age_days < 30:
            return {
                'has_new_files': False,
                'new_file_count': 0,
                'reason': f'USPTO parquet exists (age: {file_age_days:.1f} days)'
            }
        else:
            return {
                'has_new_files': True,
                'new_file_count': 0,
                'reason': f'USPTO parquet is {file_age_days:.0f} days old, may need refresh'
            }

    # No parquet file - need to download USPTO data
    return {
        'has_new_files': True,
        'new_file_count': 0,
        'reason': 'USPTO parquet not found'
    }


def check_for_update(cache_dir: Path, include_uspto: bool = False) -> Dict[str, any]:
    """
    Check if there's a newer SureChEMBL release or USPTO files available.

    Returns dict with:
      - needs_update: bool
      - cached_release: str or None
      - latest_release: str or None
      - reason: str
      - uspto_update: dict (if include_uspto=True)
    """
    cached_release = get_cached_release(cache_dir)
    latest_release = get_latest_release()

    result = {
        'needs_update': False,
        'cached_release': cached_release,
        'latest_release': latest_release,
        'reason': ''
    }

    if not latest_release:
        result['reason'] = 'Could not fetch latest release from SureChEMBL'
        return result

    if not cached_release:
        result['needs_update'] = True
        result['reason'] = 'No cached release found'
        return result

    if cached_release != latest_release:
        result['needs_update'] = True
        result['reason'] = f'Newer SureChEMBL release: {cached_release} -> {latest_release}'
        return result

    # SureChEMBL is up to date, check USPTO if enabled
    if include_uspto:
        uspto_check = check_uspto_for_updates(cache_dir)
        result['uspto_update'] = uspto_check

        if uspto_check['has_new_files']:
            result['needs_update'] = True
            result['reason'] = f"SureChEMBL up-to-date ({latest_release}), but {uspto_check['reason']}"
            return result

    result['reason'] = f'Already on latest release: {latest_release}'
    if include_uspto:
        result['reason'] += ' (USPTO also up-to-date)'

    return result


def get_available_releases() -> List[str]:
    """Fetch list of available releases from SureChEMBL FTP."""
    log("Fetching available SureChEMBL releases...")
    try:
        response = requests.get(SURECHEMBL_BASE_URL, timeout=30)
        response.raise_for_status()
        soup = BeautifulSoup(response.text, 'html.parser')
        releases = []
        for link in soup.find_all('a'):
            href = link.get('href', '')
            if href.endswith('/') and href[:-1].replace('-', '').isdigit():
                releases.append(href[:-1])
        releases.sort(reverse=True)
        return releases
    except Exception as e:
        log(f"ERROR: Failed to fetch releases: {e}")
        return []


def download_file(url: str, dest: Path, desc: str = "") -> bool:
    """Download a file with progress bar."""
    try:
        response = requests.get(url, stream=True, timeout=300)
        response.raise_for_status()
        total_size = int(response.headers.get('content-length', 0))

        dest.parent.mkdir(parents=True, exist_ok=True)

        with open(dest, 'wb') as f:
            with tqdm(total=total_size, unit='B', unit_scale=True, desc=desc) as pbar:
                for chunk in response.iter_content(chunk_size=8192):
                    f.write(chunk)
                    pbar.update(len(chunk))
        return True
    except Exception as e:
        log(f"ERROR downloading {url}: {e}")
        if dest.exists():
            dest.unlink()
        return False


def download_surechembl(cache_dir: Path, test_mode: bool = False) -> Optional[Path]:
    """Download SureChEMBL data. Returns path to release directory."""
    releases = get_available_releases()
    if not releases:
        log("ERROR: No SureChEMBL releases found")
        return None

    latest_release = releases[0]
    log(f"Latest SureChEMBL release: {latest_release}")

    release_dir = cache_dir / "surechembl" / latest_release
    release_dir.mkdir(parents=True, exist_ok=True)

    # Check if already downloaded
    all_exist = all((release_dir / f).exists() for f in REQUIRED_FILES)
    if all_exist:
        log(f"SureChEMBL data already exists at {release_dir}")
        # Ensure release is tracked
        save_cached_release(cache_dir, latest_release)
        return release_dir

    # Download files
    log(f"Downloading SureChEMBL release {latest_release}...")
    for filename in REQUIRED_FILES:
        dest = release_dir / filename
        if dest.exists():
            log(f"  {filename} already exists, skipping")
            continue

        url = f"{SURECHEMBL_BASE_URL}{latest_release}/{filename}"
        log(f"  Downloading {filename}...")
        if not download_file(url, dest, filename):
            log(f"ERROR: Failed to download {filename}")
            return None

    # Save the release version we just downloaded
    save_cached_release(cache_dir, latest_release)
    log(f"SureChEMBL download complete: {release_dir}")
    return release_dir


# =============================================================================
# USPTO-Chem Download and Processing
# =============================================================================

def download_uspto(output_dir: Path, test_mode: bool = False) -> Optional[Path]:
    """Download USPTO-Chem data and process to parquet."""
    uspto_dir = output_dir / "uspto_historical"
    state_dir = output_dir / "state"
    tracking_file = state_dir / "uspto_download.json"

    uspto_dir.mkdir(parents=True, exist_ok=True)
    state_dir.mkdir(parents=True, exist_ok=True)

    # Load tracking data
    downloaded_files: Set[str] = set()
    if tracking_file.exists():
        with open(tracking_file) as f:
            data = json.load(f)
            downloaded_files = set(data.get('downloaded_files', []))

    # Fetch contents index
    log("Fetching USPTO-Chem contents index...")
    try:
        response = requests.get("https://eloyfelix.github.io/uspto-chem/contents.json", timeout=30)
        response.raise_for_status()
        contents = response.json()
    except Exception as e:
        log(f"ERROR: Failed to fetch USPTO contents: {e}")
        return None

    # Extract file list
    files_to_download = []
    for dataset in ['applications', 'grants']:
        if dataset not in contents:
            continue
        for year_str, year_data in contents[dataset].items():
            year = int(year_str)
            if year < 2001 or year > 2025:
                continue
            for month_str, url_list in year_data.items():
                month = int(month_str)
                for url in url_list:
                    filename = Path(url).name
                    file_key = f"{dataset}/{year}/{month:02d}/{filename}"
                    if file_key not in downloaded_files:
                        files_to_download.append({
                            'url': url,
                            'dataset': dataset,
                            'year': year,
                            'month': month,
                            'filename': filename,
                            'file_key': file_key
                        })

    # In test mode, limit downloads
    if test_mode and len(files_to_download) > 10:
        log(f"[TEST MODE] Limiting USPTO download to 10 files")
        files_to_download = files_to_download[:10]

    log(f"USPTO files to download: {len(files_to_download)}")

    # Download files
    stats = {'downloaded': 0, 'errors': 0}
    for file_info in tqdm(files_to_download, desc="Downloading USPTO"):
        url = file_info['url']
        out_path = uspto_dir / file_info['dataset'] / str(file_info['year']) / f"{file_info['month']:02d}" / file_info['filename']
        out_path.parent.mkdir(parents=True, exist_ok=True)

        try:
            response = requests.get(url, timeout=60)
            response.raise_for_status()
            with open(out_path, 'wb') as f:
                f.write(response.content)

            # Verify JSON
            with open(out_path) as f:
                json.load(f)

            downloaded_files.add(file_info['file_key'])
            stats['downloaded'] += 1
        except Exception as e:
            log(f"ERROR downloading {file_info['filename']}: {e}")
            stats['errors'] += 1
            if out_path.exists():
                out_path.unlink()

    # Save tracking
    with open(tracking_file, 'w') as f:
        json.dump({
            'downloaded_files': sorted(list(downloaded_files)),
            'last_updated': datetime.now().isoformat(),
            'stats': stats
        }, f, indent=2)

    log(f"USPTO download complete: {stats['downloaded']} files, {stats['errors']} errors")

    # Process to parquet
    return process_uspto_to_parquet(uspto_dir, output_dir, test_mode)


def process_uspto_to_parquet(uspto_dir: Path, output_dir: Path, test_mode: bool = False) -> Optional[Path]:
    """Process USPTO JSON files to a single parquet file."""
    output_file = output_dir / "uspto_historical.parquet"

    # Check if already processed
    if output_file.exists():
        log(f"USPTO parquet already exists: {output_file}")
        return output_file

    log("Processing USPTO JSON files to parquet...")

    # Find all JSON files
    json_files = list(uspto_dir.rglob('*.json'))
    if not json_files:
        log("WARNING: No USPTO JSON files found")
        return None

    log(f"Found {len(json_files)} USPTO JSON files")

    if test_mode and len(json_files) > 20:
        json_files = json_files[:20]
        log(f"[TEST MODE] Limiting to {len(json_files)} files")

    # Process files
    all_patents = []
    for json_path in tqdm(json_files, desc="Processing USPTO JSON"):
        try:
            with open(json_path) as f:
                data = json.load(f)

            for patent in data:
                pub = patent.get('publication', {})
                doc_number = pub.get('doc_number', '')
                kind = pub.get('kind', '')

                if doc_number and kind:
                    patent_number = f"US-{doc_number}-{kind}"
                    title = patent.get('title', '')
                    abstract = patent.get('abstract', '')
                    pub_date = pub.get('date', '')

                    if title or abstract:
                        all_patents.append({
                            'patent_number': patent_number,
                            'title': title or None,
                            'abstract': abstract or None,
                            'publication_date': pub_date or None
                        })
        except Exception as e:
            log(f"ERROR processing {json_path.name}: {e}")

    if not all_patents:
        log("WARNING: No patents extracted from USPTO files")
        return None

    # Create dataframe and deduplicate
    df = pd.DataFrame(all_patents)
    df = df.drop_duplicates(subset='patent_number', keep='first')

    # Save to parquet
    output_file.parent.mkdir(parents=True, exist_ok=True)
    df.to_parquet(output_file, index=False)

    log(f"USPTO parquet created: {len(df):,} patents, {output_file.stat().st_size / 1024 / 1024:.1f} MB")
    return output_file


# =============================================================================
# Convert to Biobtree JSON
# =============================================================================

BATCH_SIZE = 50000


def convert_value(val):
    """Convert pyarrow/numpy types to JSON-serializable Python types."""
    if val is None:
        return None
    if hasattr(val, 'as_py'):
        return val.as_py()
    if hasattr(val, 'item'):
        return val.item()
    if isinstance(val, datetime):
        return val.isoformat()
    return val


def load_uspto_abstracts(uspto_parquet: Path) -> Dict[str, str]:
    """Load USPTO abstracts into lookup dictionary."""
    log(f"Loading USPTO abstracts from {uspto_parquet}...")
    df = pd.read_parquet(uspto_parquet, columns=['patent_number', 'abstract'])
    df = df[df['abstract'].notna() & (df['abstract'] != '')]
    abstracts = dict(zip(df['patent_number'], df['abstract']))
    log(f"Loaded {len(abstracts):,} USPTO abstracts")
    return abstracts


def convert_to_biobtree_json(
    surechembl_dir: Path,
    output_dir: Path,
    uspto_parquet: Optional[Path] = None,
    test_mode: bool = False
) -> bool:
    """Convert SureChEMBL parquet to biobtree JSON format."""
    log("Converting SureChEMBL data to biobtree JSON...")

    output_dir.mkdir(parents=True, exist_ok=True)

    # Load USPTO abstracts if available
    uspto_abstracts = None
    if uspto_parquet and uspto_parquet.exists():
        uspto_abstracts = load_uspto_abstracts(uspto_parquet)

    conversions = [
        ('patents.parquet', 'patents.json', 'patents', True),
        ('compounds.parquet', 'compounds.json', 'compounds', False),
        ('patent_compound_map.parquet', 'mapping.json', 'mappings', False),
    ]

    stats = {}

    for input_name, output_name, wrapper_key, merge_abstracts in conversions:
        input_file = surechembl_dir / input_name
        output_file = output_dir / output_name

        if not input_file.exists():
            log(f"WARNING: {input_name} not found, skipping")
            continue

        log(f"Converting {input_name} -> {output_name}...")

        parquet_file = pq.ParquetFile(input_file)
        total_rows = parquet_file.metadata.num_rows
        columns = [col.name for col in parquet_file.schema_arrow]

        # Test mode limit
        max_rows = 10000 if test_mode else None

        record_count = 0
        abstracts_merged = 0

        with open(output_file, 'w') as f:
            f.write('{"' + wrapper_key + '":[')
            first_record = True

            for batch in parquet_file.iter_batches(batch_size=BATCH_SIZE):
                batch_dict = batch.to_pydict()
                batch_len = len(batch_dict[columns[0]])

                for i in range(batch_len):
                    record = {col: convert_value(batch_dict[col][i]) for col in columns}

                    # Merge USPTO abstract if available
                    if merge_abstracts and uspto_abstracts:
                        patent_num = record.get('patent_number')
                        if patent_num and patent_num in uspto_abstracts:
                            record['abstract'] = uspto_abstracts[patent_num]
                            abstracts_merged += 1

                    if not first_record:
                        f.write(',')
                    first_record = False

                    json.dump(record, f, separators=(',', ':'), default=str)
                    record_count += 1

                    if max_rows and record_count >= max_rows:
                        break

                if max_rows and record_count >= max_rows:
                    break

            f.write(']}')

        stats[wrapper_key] = {
            'records': record_count,
            'file_size_mb': output_file.stat().st_size / (1024 * 1024),
            'abstracts_merged': abstracts_merged
        }

        log(f"  {wrapper_key}: {record_count:,} records, {stats[wrapper_key]['file_size_mb']:.1f} MB" +
            (f", {abstracts_merged:,} abstracts" if abstracts_merged else ""))

    # Write summary
    summary_file = output_dir / 'conversion_summary.json'
    with open(summary_file, 'w') as f:
        json.dump({
            'conversion_date': datetime.now().isoformat(),
            'surechembl_dir': str(surechembl_dir),
            'output_dir': str(output_dir),
            'uspto_enrichment': uspto_parquet is not None,
            'statistics': stats
        }, f, indent=2)

    return True


# =============================================================================
# Main Orchestrator
# =============================================================================

def main():
    parser = argparse.ArgumentParser(
        description='Patent data preparation orchestrator for biobtree',
        formatter_class=argparse.RawDescriptionHelpFormatter
    )

    parser.add_argument(
        '--output-dir',
        type=Path,
        required=True,
        help='Output directory for all patent data'
    )

    parser.add_argument(
        '--include-uspto',
        action='store_true',
        help='Include USPTO abstract enrichment'
    )

    parser.add_argument(
        '--test-mode',
        action='store_true',
        help='Test mode: download and process limited data'
    )

    parser.add_argument(
        '--log-file',
        type=str,
        default=None,
        help='Path to log file (in addition to console)'
    )

    parser.add_argument(
        '--check-update',
        action='store_true',
        help='Only check if update is needed, exit with code 0 if yes, 10 if no'
    )

    parser.add_argument(
        '--force',
        action='store_true',
        help='Force re-download and regeneration even if up to date'
    )

    args = parser.parse_args()

    # Setup logging
    setup_logging(args.log_file)

    # Cache directory for intermediate files (inside output_dir)
    cache_dir = args.output_dir / "cache"
    cache_dir.mkdir(parents=True, exist_ok=True)

    # Handle --check-update mode: just check if update needed and exit
    if args.check_update:
        update_info = check_for_update(cache_dir, include_uspto=args.include_uspto)
        log(f"Update check: {update_info['reason']}")
        if 'uspto_update' in update_info:
            log(f"USPTO check: {update_info['uspto_update']['reason']}")
        if update_info['needs_update']:
            log("Update IS needed")
            sys.exit(0)  # Exit 0 = update needed
        else:
            log("Update NOT needed")
            sys.exit(10)  # Exit 10 = no update needed

    log("=" * 70)
    log("Patent Data Preparation for Biobtree")
    log("=" * 70)
    log(f"Output directory: {args.output_dir}")
    log(f"Cache directory:  {cache_dir}")
    log(f"Include USPTO:    {args.include_uspto}")
    log(f"Test mode:        {args.test_mode}")
    log(f"Force update:     {args.force}")
    if args.log_file:
        log(f"Log file:         {args.log_file}")
    log("=" * 70)

    # Check if update is needed (unless --force)
    if not args.force:
        update_info = check_for_update(cache_dir, include_uspto=args.include_uspto)
        log(f"\nUpdate check: {update_info['reason']}")
        if 'uspto_update' in update_info:
            log(f"USPTO check: {update_info['uspto_update']['reason']}")

        # Check if output JSON files exist
        patents_json = args.output_dir / "patents.json"
        compounds_json = args.output_dir / "compounds.json"
        mapping_json = args.output_dir / "mapping.json"
        json_exists = patents_json.exists() and compounds_json.exists() and mapping_json.exists()

        if not update_info['needs_update'] and json_exists:
            log("=" * 70)
            log("Data is already up to date - nothing to do!")
            log(f"Cached release: {update_info['cached_release']}")
            log("=" * 70)
            sys.exit(0)

        if update_info['needs_update']:
            log(f"Update needed: {update_info['reason']}")
        elif not json_exists:
            log("JSON files missing - will regenerate from cached parquet")

    # Create directories
    args.output_dir.mkdir(parents=True, exist_ok=True)

    # Step 1: Download SureChEMBL to cache directory
    log("\n[Step 1] Downloading SureChEMBL data...")
    surechembl_dir = download_surechembl(cache_dir, args.test_mode)
    if not surechembl_dir:
        log("ERROR: Failed to download SureChEMBL data")
        sys.exit(1)

    # Step 2: Optionally download and process USPTO to cache directory
    uspto_parquet = None
    if args.include_uspto:
        log("\n[Step 2] Downloading and processing USPTO data...")
        uspto_parquet = download_uspto(cache_dir, args.test_mode)
        if not uspto_parquet:
            log("WARNING: USPTO processing failed, continuing without abstracts")

    # Step 3: Convert to biobtree JSON
    log("\n[Step 3] Converting to biobtree JSON format...")
    # Output directly to output_dir (path from source1.dataset.json)
    success = convert_to_biobtree_json(
        surechembl_dir,
        args.output_dir,
        uspto_parquet,
        args.test_mode
    )

    if not success:
        log("ERROR: Conversion failed")
        sys.exit(1)

    log("\n" + "=" * 70)
    log("Patent data preparation complete!")
    log("=" * 70)
    log(f"Output directory: {args.output_dir}")
    log_memory_usage()
    log("=" * 70)


if __name__ == '__main__':
    main()
