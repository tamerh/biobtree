#!/usr/bin/env python3
"""
Clinical Trials Data Preparation Orchestrator for Biobtree

Single entry point for preparing clinical trials data. Called automatically by biobtree
when clinical trials data files don't exist, similar to cellxgene/patents extraction.

Workflow:
1. Download latest AACT snapshot from ClinicalTrials.gov
2. Extract zip file and load tables
3. Process trials into JSON format with all fields needed for biobtree
4. Support incremental updates via content hashing (optional)

Usage:
    # Basic usage (called by biobtree)
    python clinical_trials_prepare.py --output-dir data/clinical_trials

    # Test mode (limited data)
    python clinical_trials_prepare.py --output-dir data/clinical_trials --test-mode

    # With logging to file
    python clinical_trials_prepare.py --output-dir data/clinical_trials --log-file logs/clinical_trials.log

Configuration:
    This script is typically invoked by biobtree's clinical_trials.go when the
    required JSON files don't exist. Configuration comes from:
    - application.param.json: clinicalTrialsDataDir
"""

import os
import sys
import json
import zipfile
import requests
import psutil
import pandas as pd
import argparse
import re
import glob
from datetime import datetime
from pathlib import Path
from bs4 import BeautifulSoup
from typing import List, Dict, Any, Optional
import logging


# Global logger
logger = logging.getLogger('clinical_trials_prepare')


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
    """Log message with memory usage."""
    memory_mb = psutil.Process().memory_info().rss / 1024 / 1024
    logger.info(f"[MEM: {memory_mb:.1f}MB] {message}")


# Update tracking
TRACKING_FILE = "clinical_trials_update.json"
DEFAULT_UPDATE_INTERVAL_DAYS = 30


def get_last_update_info(cache_dir: Path) -> Optional[Dict]:
    """Get information about the last update."""
    tracking_file = cache_dir / TRACKING_FILE
    if tracking_file.exists():
        try:
            with open(tracking_file) as f:
                return json.load(f)
        except Exception:
            pass
    return None


def save_update_info(cache_dir: Path, snapshot_date: str) -> None:
    """Save update tracking information."""
    tracking_file = cache_dir / TRACKING_FILE
    cache_dir.mkdir(parents=True, exist_ok=True)
    with open(tracking_file, 'w') as f:
        json.dump({
            'last_update': datetime.now().isoformat(),
            'snapshot_date': snapshot_date
        }, f, indent=2)


def check_for_update(cache_dir: Path, output_dir: Path, max_age_days: int = DEFAULT_UPDATE_INTERVAL_DAYS) -> Dict[str, Any]:
    """
    Check if clinical trials data needs updating.

    Returns dict with:
      - needs_update: bool
      - reason: str
      - last_update: str or None
      - age_days: float or None
    """
    # Check if output JSON exists
    trials_json = output_dir / "trials.json"
    if not trials_json.exists():
        return {
            'needs_update': True,
            'reason': 'Output file trials.json not found',
            'last_update': None,
            'age_days': None
        }

    # Check tracking file
    update_info = get_last_update_info(cache_dir)
    if not update_info:
        # No tracking file - check file age instead
        import time
        file_age_days = (time.time() - trials_json.stat().st_mtime) / 86400
        if file_age_days > max_age_days:
            return {
                'needs_update': True,
                'reason': f'Data is {file_age_days:.0f} days old (max: {max_age_days})',
                'last_update': None,
                'age_days': file_age_days
            }
        return {
            'needs_update': False,
            'reason': f'Data is {file_age_days:.1f} days old (within {max_age_days} day limit)',
            'last_update': None,
            'age_days': file_age_days
        }

    # Check age from tracking file
    try:
        last_update = datetime.fromisoformat(update_info['last_update'])
        age_days = (datetime.now() - last_update).total_seconds() / 86400

        if age_days > max_age_days:
            return {
                'needs_update': True,
                'reason': f'Data is {age_days:.0f} days old (max: {max_age_days})',
                'last_update': update_info['last_update'],
                'age_days': age_days
            }

        return {
            'needs_update': False,
            'reason': f'Data is {age_days:.1f} days old (within {max_age_days} day limit)',
            'last_update': update_info['last_update'],
            'age_days': age_days
        }
    except Exception as e:
        return {
            'needs_update': True,
            'reason': f'Could not parse tracking file: {e}',
            'last_update': None,
            'age_days': None
        }


class AACTDownloader:
    """Downloads and manages AACT flat file exports."""

    def __init__(self, base_url: str = "https://aact.ctti-clinicaltrials.org/downloads"):
        """Initialize the AACT downloader."""
        self.base_url = base_url
        self.session = requests.Session()
        self.session.headers.update({
            'User-Agent': 'Biobtree Clinical Trials Pipeline (https://github.com/tamerh/biobtree)'
        })

    def get_latest_snapshot_info(self) -> dict:
        """Get information about the latest AACT flat file snapshot."""
        log(f"Fetching snapshot information from {self.base_url}")

        try:
            response = self.session.get(self.base_url, timeout=30)
            response.raise_for_status()

            soup = BeautifulSoup(response.content, 'html.parser')

            # Find all DigitalOcean Spaces links for flat files
            all_links = []
            for link in soup.find_all('a', href=True):
                href = link['href']
                if 'digitaloceanspaces.com' in href and 'Download File' in link.get_text():
                    all_links.append(href)

            if len(all_links) >= 2:
                flat_file_url = all_links[1]  # Second link is flat files
                snapshot_info = {
                    'filename': f"aact_flat_files_{datetime.now().strftime('%Y%m%d')}.zip",
                    'url': flat_file_url,
                    'type': 'flat_files'
                }
                log(f"Found flat file snapshot: {snapshot_info['filename']}")
                return snapshot_info
            else:
                raise ValueError(f"Could not find flat file link. Found {len(all_links)} links.")

        except requests.RequestException as e:
            log(f"ERROR: Failed to fetch snapshot information: {e}")
            raise
        except Exception as e:
            log(f"ERROR: Failed to parse snapshot page: {e}")
            raise

    def check_existing_snapshot(self, download_dir: str) -> Optional[str]:
        """Check if we have today's snapshot. Returns path if exists, None otherwise."""
        today_str = datetime.now().strftime('%Y%m%d')
        existing_snapshots = glob.glob(os.path.join(download_dir, "aact_flat_files_*.zip"))

        if not existing_snapshots:
            return None

        for snapshot_path in existing_snapshots:
            snapshot_name = os.path.basename(snapshot_path)
            match = re.search(r'aact_flat_files_(\d{8})\.zip', snapshot_name)
            if match:
                file_date = match.group(1)
                if file_date == today_str:
                    return snapshot_path
                else:
                    # Found old snapshot - clean it up
                    log(f"Removing old snapshot from {file_date}: {snapshot_name}")
                    try:
                        os.remove(snapshot_path)
                    except Exception as e:
                        log(f"WARNING: Could not remove old snapshot: {e}")

        return None

    def download_snapshot(self, snapshot_info: dict, download_dir: str) -> str:
        """Download the AACT flat file snapshot if not already present."""
        filename = snapshot_info['filename']
        url = snapshot_info['url']
        local_path = os.path.join(download_dir, filename)

        # Check for existing snapshot
        existing_snapshot = self.check_existing_snapshot(download_dir)

        if existing_snapshot:
            file_size = os.path.getsize(existing_snapshot)
            log(f"Today's snapshot already exists: {os.path.basename(existing_snapshot)} ({file_size/1024/1024:.1f}MB)")
            return existing_snapshot

        log(f"Downloading flat file snapshot from {url}")
        os.makedirs(download_dir, exist_ok=True)

        try:
            response = self.session.get(url, stream=True, timeout=60)
            response.raise_for_status()

            total_size = int(response.headers.get('content-length', 0))
            downloaded = 0
            last_log_mb = 0

            with open(local_path, 'wb') as f:
                for chunk in response.iter_content(chunk_size=8192):
                    if chunk:
                        f.write(chunk)
                        downloaded += len(chunk)

                        current_mb = downloaded / (1024 * 1024)
                        if current_mb - last_log_mb >= 100:
                            if total_size > 0:
                                progress = (downloaded / total_size) * 100
                                log(f"Downloaded {current_mb:.1f}MB ({progress:.1f}%)")
                            else:
                                log(f"Downloaded {current_mb:.1f}MB")
                            last_log_mb = current_mb

            final_size = os.path.getsize(local_path)
            log(f"Download complete: {final_size/1024/1024:.1f}MB")
            return local_path

        except Exception as e:
            log(f"ERROR: Failed to download: {e}")
            if os.path.exists(local_path):
                os.remove(local_path)
            raise

    def extract_snapshot(self, zip_path: str, extract_dir: str) -> dict:
        """Extract the AACT flat file snapshot and return info about extracted files."""
        log(f"Extracting snapshot to {extract_dir}")
        os.makedirs(extract_dir, exist_ok=True)

        try:
            extracted_files = {}
            with zipfile.ZipFile(zip_path, 'r') as zip_ref:
                file_list = zip_ref.namelist()
                log(f"Archive contains {len(file_list)} files")

                for file_info in zip_ref.infolist():
                    zip_ref.extract(file_info, extract_dir)

                    filename = file_info.filename
                    if filename.endswith('.txt'):
                        table_name = os.path.splitext(os.path.basename(filename))[0]
                        file_path = os.path.join(extract_dir, filename)
                        file_size = os.path.getsize(file_path)
                        extracted_files[table_name] = {
                            'path': file_path,
                            'size_mb': file_size / 1024 / 1024
                        }

            log(f"Extracted {len(extracted_files)} table files")

            # Save extraction_info.json
            info_path = os.path.join(extract_dir, 'extraction_info.json')
            info = {
                'extraction_date': datetime.now().isoformat(),
                'tables': extracted_files,
                'total_files': len(extracted_files),
                'total_size_mb': sum(f['size_mb'] for f in extracted_files.values())
            }
            with open(info_path, 'w') as f:
                json.dump(info, f, indent=2)

            return extracted_files

        except Exception as e:
            log(f"ERROR: Failed to extract snapshot: {e}")
            raise


class AACTExtractor:
    """Extracts clinical trial data from AACT flat files."""

    def __init__(self, extract_dir: str):
        self.extract_dir = extract_dir
        self.extraction_info = None

    def load_extraction_info(self) -> bool:
        """Load extraction info JSON."""
        info_path = os.path.join(self.extract_dir, 'extraction_info.json')

        if not os.path.exists(info_path):
            log(f"ERROR: extraction_info.json not found in {self.extract_dir}")
            return False

        try:
            with open(info_path, 'r') as f:
                self.extraction_info = json.load(f)
            log(f"Loaded extraction info: {self.extraction_info['total_files']} tables")
            return True
        except Exception as e:
            log(f"ERROR: Failed to load extraction info: {e}")
            return False

    def load_table(self, table_name: str) -> Optional[pd.DataFrame]:
        """Load a table from the extracted flat files."""
        if not self.extraction_info:
            return None

        if table_name not in self.extraction_info['tables']:
            return None

        table_info = self.extraction_info['tables'][table_name]
        table_path = table_info['path']

        if not os.path.exists(table_path):
            return None

        try:
            log(f"Loading table: {table_name} ({table_info['size_mb']:.1f}MB)")
            df = pd.read_csv(table_path, sep='|', low_memory=False)
            log(f"  Loaded {len(df):,} rows")
            return df
        except Exception as e:
            log(f"ERROR: Failed to load table {table_name}: {e}")
            return None

    def extract_trials(self, limit: Optional[int] = None,
                      exclude_withdrawn: bool = True) -> List[Dict[str, Any]]:
        """Extract trials from AACT tables into biobtree format."""
        log("Starting trial extraction...")

        # Load main studies table
        studies = self.load_table('studies')
        if studies is None:
            return []

        log(f"Initial studies count: {len(studies):,}")

        # Filter withdrawn studies
        if exclude_withdrawn and 'overall_status' in studies.columns:
            original_count = len(studies)
            studies = studies[studies['overall_status'] != 'Withdrawn'].copy()
            log(f"Filtered out {original_count - len(studies):,} withdrawn studies")

        # Load and merge brief summaries
        brief_summaries = self.load_table('brief_summaries')
        if brief_summaries is not None:
            studies = studies.merge(
                brief_summaries[['nct_id', 'description']],
                on='nct_id',
                how='left'
            )
            studies.rename(columns={'description': 'brief_summary'}, inplace=True)

        # Load detailed descriptions
        detailed_descriptions = self.load_table('detailed_descriptions')
        if detailed_descriptions is not None:
            studies = studies.merge(
                detailed_descriptions[['nct_id', 'description']],
                on='nct_id',
                how='left',
                suffixes=('', '_detailed')
            )
            studies.rename(columns={'description': 'detailed_description'}, inplace=True)

        # Apply limit
        if limit:
            studies = studies.head(limit)

        kept_nct_ids = set(studies['nct_id'].values)
        log(f"Processing {len(kept_nct_ids):,} studies")

        # Process interventions
        interventions_dict = {}
        interventions = self.load_table('interventions')
        if interventions is not None:
            interventions = interventions[interventions['nct_id'].isin(kept_nct_ids)]
            for nct_id, group in interventions.groupby('nct_id'):
                interventions_dict[nct_id] = []
                for _, row in group.iterrows():
                    interventions_dict[nct_id].append({
                        'type': str(row.get('intervention_type', '')),
                        'name': str(row.get('name', '')),
                        'description': str(row.get('description', ''))
                    })

        # Process conditions
        conditions_dict = {}
        conditions = self.load_table('conditions')
        if conditions is not None:
            conditions = conditions[conditions['nct_id'].isin(kept_nct_ids)]
            for nct_id, group in conditions.groupby('nct_id'):
                conditions_dict[nct_id] = []
                for _, row in group.iterrows():
                    condition_name = str(row.get('name', ''))
                    if condition_name and condition_name != 'nan':
                        conditions_dict[nct_id].append(condition_name)

        # Process sponsors
        sponsors_dict = {}
        sponsors = self.load_table('sponsors')
        if sponsors is not None:
            sponsors = sponsors[sponsors['nct_id'].isin(kept_nct_ids)]
            for nct_id, group in sponsors.groupby('nct_id'):
                sponsors_dict[nct_id] = []
                for _, row in group.iterrows():
                    sponsor_name = str(row.get('name', ''))
                    if sponsor_name and sponsor_name != 'nan':
                        sponsors_dict[nct_id].append({
                            'name': sponsor_name,
                            'agency_class': str(row.get('agency_class', '')),
                            'role': str(row.get('lead_or_collaborator', ''))
                        })

        # Process publications
        publications_dict = {}
        study_references = self.load_table('study_references')
        if study_references is not None:
            study_references = study_references[study_references['nct_id'].isin(kept_nct_ids)]
            for nct_id, group in study_references.groupby('nct_id'):
                publications_dict[nct_id] = []
                for _, row in group.iterrows():
                    pmid_raw = row.get('pmid', '')
                    pmid = str(pmid_raw).replace('.0', '') if pd.notna(pmid_raw) else ''
                    if pmid and pmid != 'nan' and pmid != '':
                        publications_dict[nct_id].append({
                            'pmid': pmid,
                            'reference_type': str(row.get('reference_type', '')),
                            'citation': str(row.get('citation', ''))
                        })

        # Convert to list of dicts
        log("Converting to final format...")
        extracted_trials = []

        for idx, row in studies.iterrows():
            nct_id = row['nct_id']

            trial_data = {
                'nct_id': nct_id,
                'brief_title': str(row.get('brief_title', '')),
                'official_title': str(row.get('official_title', '')),
                'brief_summary': str(row.get('brief_summary', '')),
                'detailed_description': str(row.get('detailed_description', '')) if 'detailed_description' in row else '',
                'overall_status': str(row.get('overall_status', '')),
                'phase': str(row.get('phase', '')),
                'study_type': str(row.get('study_type', '')),
                'enrollment': int(row.get('enrollment', 0)) if pd.notna(row.get('enrollment')) else 0,
                'start_date': str(row.get('start_date', '')),
                'completion_date': str(row.get('completion_date', '')),
                'interventions': interventions_dict.get(nct_id, []),
                'conditions': conditions_dict.get(nct_id, []),
                'sponsors': sponsors_dict.get(nct_id, []),
                'publications': publications_dict.get(nct_id, [])
            }

            extracted_trials.append(trial_data)

        log(f"Extraction complete: {len(extracted_trials):,} trials")
        return extracted_trials


def write_biobtree_json(trials: List[Dict[str, Any]], output_dir: str) -> str:
    """Write trials to JSON format for biobtree."""
    output_path = os.path.join(output_dir, 'trials.json')

    log(f"Writing {len(trials):,} trials to {output_path}")

    with open(output_path, 'w') as f:
        f.write('{"trials":[')
        first = True
        for trial in trials:
            if not first:
                f.write(',')
            first = False
            json.dump(trial, f, separators=(',', ':'), default=str)
        f.write(']}')

    file_size_mb = os.path.getsize(output_path) / 1024 / 1024
    log(f"Written {file_size_mb:.2f}MB")

    return output_path


def main():
    """Main execution function."""
    parser = argparse.ArgumentParser(
        description="Clinical trials data preparation orchestrator for biobtree",
        formatter_class=argparse.RawDescriptionHelpFormatter
    )

    parser.add_argument("--output-dir", type=Path, required=True,
                       help="Output directory for all clinical trials data")
    parser.add_argument("--test-mode", action="store_true",
                       help="Test mode: process limited data (1000 trials)")
    parser.add_argument("--log-file", type=str, default=None,
                       help="Path to log file (in addition to console)")
    parser.add_argument("--check-update", action="store_true",
                       help="Only check if update is needed, exit with code 0 if yes, 10 if no")
    parser.add_argument("--force", action="store_true",
                       help="Force re-download and regeneration even if up to date")
    parser.add_argument("--max-age-days", type=int, default=DEFAULT_UPDATE_INTERVAL_DAYS,
                       help=f"Maximum age in days before update is needed (default: {DEFAULT_UPDATE_INTERVAL_DAYS})")

    args = parser.parse_args()

    # Setup logging
    setup_logging(args.log_file)

    # Derived directories - cache inside output_dir
    cache_dir = args.output_dir / "cache"
    download_dir = cache_dir / "downloads"
    extract_dir = cache_dir / "extracted"

    # Ensure cache_dir exists for update check
    cache_dir.mkdir(parents=True, exist_ok=True)

    # Handle --check-update mode: just check if update needed and exit
    if args.check_update:
        update_info = check_for_update(cache_dir, args.output_dir, args.max_age_days)
        log(f"Update check: {update_info['reason']}")
        if update_info['needs_update']:
            log("Update IS needed")
            sys.exit(0)  # Exit 0 = update needed
        else:
            log("Update NOT needed")
            sys.exit(10)  # Exit 10 = no update needed

    # Limit for test mode
    limit = 1000 if args.test_mode else None

    log("=" * 70)
    log("Clinical Trials Data Preparation for Biobtree")
    log("=" * 70)
    log(f"Output directory: {args.output_dir}")
    log(f"Cache directory:  {cache_dir}")
    log(f"Test mode:        {args.test_mode}")
    log(f"Force update:     {args.force}")
    log(f"Max age (days):   {args.max_age_days}")
    if args.log_file:
        log(f"Log file:         {args.log_file}")
    log("=" * 70)

    # Check if update is needed (unless --force)
    if not args.force:
        update_info = check_for_update(cache_dir, args.output_dir, args.max_age_days)
        log(f"\nUpdate check: {update_info['reason']}")

        if not update_info['needs_update']:
            log("=" * 70)
            log("Data is already up to date - nothing to do!")
            if update_info.get('last_update'):
                log(f"Last update: {update_info['last_update']}")
            log("=" * 70)
            sys.exit(0)

    # System info
    memory = psutil.virtual_memory()
    log(f"System RAM: {memory.total / 1024**3:.1f}GB total, {memory.available / 1024**3:.1f}GB available")

    try:
        # Create output directories
        args.output_dir.mkdir(parents=True, exist_ok=True)
        download_dir.mkdir(parents=True, exist_ok=True)
        extract_dir.mkdir(parents=True, exist_ok=True)

        # Step 1: Download snapshot
        log("")
        log("[Step 1] Downloading AACT snapshot...")
        downloader = AACTDownloader()
        snapshot_info = downloader.get_latest_snapshot_info()
        snapshot_path = downloader.download_snapshot(snapshot_info, str(download_dir))

        # Step 2: Extract snapshot
        log("")
        log("[Step 2] Extracting AACT tables...")
        downloader.extract_snapshot(snapshot_path, str(extract_dir))

        # Step 3: Extract and convert trials
        log("")
        log("[Step 3] Extracting and converting trials...")
        extractor = AACTExtractor(str(extract_dir))
        if not extractor.load_extraction_info():
            log("ERROR: Failed to load extraction info")
            sys.exit(1)

        trials = extractor.extract_trials(limit=limit, exclude_withdrawn=True)

        if not trials:
            log("ERROR: No trials extracted")
            sys.exit(1)

        # Step 4: Write output directly to output_dir (path from source1.dataset.json)
        log("")
        log("[Step 4] Writing biobtree JSON...")
        output_path = write_biobtree_json(trials, str(args.output_dir))

        # Save update tracking info
        snapshot_date = datetime.now().strftime('%Y-%m-%d')
        save_update_info(cache_dir, snapshot_date)
        log(f"Update tracking saved: {snapshot_date}")

        # Summary
        log("")
        log("=" * 70)
        log("Clinical trials data preparation complete!")
        log("=" * 70)
        log(f"Trials extracted: {len(trials):,}")
        log(f"Output file:      {output_path}")
        log("=" * 70)

        return 0

    except Exception as e:
        log(f"ERROR: {e}")
        import traceback
        traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.exit(main())
