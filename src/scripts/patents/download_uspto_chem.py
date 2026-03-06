#!/usr/bin/env python3
"""
Download USPTO-Chem Historical Data for Biobtree

Downloads pre-processed USPTO chemical/pharmaceutical patent data from:
https://eloyfelix.github.io/uspto-chem/

This provides full historical coverage (2001-present) of biomedical patents
in JSON format, already filtered by IPC/CPC codes for chemistry/pharma.

These abstracts are merged with SureChEMBL data to provide richer patent information.

Usage:
    python download_uspto_chem.py \\
        --output-dir data/patents/uspto_historical \\
        --tracking-file data/patents/state/uspto_download.json
"""

import os
import sys
import json
import argparse
import requests
from pathlib import Path
from typing import Dict, List, Set, Optional
from datetime import datetime
from tqdm import tqdm
import time


def log(message: str) -> None:
    """Print message with timestamp."""
    timestamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
    print(f"[{timestamp}] {message}", flush=True)


class USPTOChemDownloader:
    """Downloads USPTO-Chem historical JSON data."""

    def __init__(
        self,
        output_dir: Path,
        tracking_file: Path,
        start_year: int = 2001,
        end_year: int = 2025,
        datasets: List[str] = None,
        limit_files: Optional[int] = None,
        debug: bool = False
    ):
        self.output_dir = Path(output_dir)
        self.tracking_file = Path(tracking_file)
        self.start_year = start_year
        self.end_year = end_year
        self.datasets = datasets or ['applications', 'grants']
        self.limit_files = limit_files
        self.debug = debug

        # Create directories
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.tracking_file.parent.mkdir(parents=True, exist_ok=True)

        # Load tracking data
        self.downloaded_files = self._load_tracking()

        # Stats
        self.stats = {
            'total_files': 0,
            'downloaded': 0,
            'skipped': 0,
            'errors': 0,
            'total_bytes': 0
        }

    def _load_tracking(self) -> Set[str]:
        """Load set of already downloaded files."""
        if self.tracking_file.exists():
            with open(self.tracking_file) as f:
                data = json.load(f)
                return set(data.get('downloaded_files', []))
        return set()

    def _save_tracking(self):
        """Save tracking data."""
        with open(self.tracking_file, 'w') as f:
            json.dump({
                'downloaded_files': sorted(list(self.downloaded_files)),
                'last_updated': datetime.now().isoformat(),
                'stats': self.stats
            }, f, indent=2)

    def fetch_contents_index(self) -> Dict:
        """Fetch the contents.json index file."""
        log("Fetching contents index from uspto-chem...")

        url = "https://eloyfelix.github.io/uspto-chem/contents.json"

        try:
            response = requests.get(url, timeout=30)
            response.raise_for_status()
            contents = response.json()

            log(f"Successfully fetched contents index")
            return contents

        except Exception as e:
            log(f"ERROR: Failed to fetch contents index: {e}")
            sys.exit(1)

    def extract_file_list(self, contents: Dict) -> List[Dict]:
        """Extract list of files to download from contents index."""
        files = []

        for dataset in self.datasets:
            if dataset not in contents:
                log(f"WARNING: Dataset '{dataset}' not found in contents")
                continue

            dataset_data = contents[dataset]

            for year_str, year_data in dataset_data.items():
                year = int(year_str)

                # Filter by year range
                if year < self.start_year or year > self.end_year:
                    continue

                for month_str, url_list in year_data.items():
                    month = int(month_str)

                    for url in url_list:
                        filename = Path(url).name
                        date_str = filename.replace('.json', '').replace('-SUPP', '').replace('_r1', '')

                        # Handle grants with 'I' prefix
                        if date_str.startswith('I'):
                            date_str = date_str[1:]

                        files.append({
                            'url': url,
                            'dataset': dataset,
                            'year': year,
                            'month': month,
                            'date': date_str,
                            'filename': filename
                        })

        log(f"Found {len(files)} files to download")

        if self.limit_files and len(files) > self.limit_files:
            log(f"Limiting to {self.limit_files} files (test mode)")
            files = files[:self.limit_files]

        return files

    def download_file(self, file_info: Dict) -> bool:
        """Download a single JSON file."""
        url = file_info['url']
        dataset = file_info['dataset']
        year = file_info['year']
        month = f"{file_info['month']:02d}"
        filename = file_info['filename']

        # Check if already downloaded
        file_key = f"{dataset}/{year}/{month}/{filename}"
        if file_key in self.downloaded_files:
            self.stats['skipped'] += 1
            return True

        # Create output directory
        output_dir = self.output_dir / dataset / str(year) / month
        output_dir.mkdir(parents=True, exist_ok=True)

        output_file = output_dir / filename

        # Download with retry logic
        max_retries = 3
        retry_delay = 2

        for attempt in range(max_retries):
            try:
                response = requests.get(url, timeout=60, stream=True)
                response.raise_for_status()

                content_type = response.headers.get('Content-Type', '')
                if 'json' not in content_type and 'application/octet-stream' not in content_type:
                    log(f"WARNING: URL returned non-JSON content: {url}")
                    self.stats['errors'] += 1
                    return False

                with open(output_file, 'wb') as f:
                    for chunk in response.iter_content(chunk_size=8192):
                        f.write(chunk)

                # Verify file is valid JSON
                try:
                    with open(output_file) as f:
                        json.load(f)
                except json.JSONDecodeError as e:
                    log(f"ERROR: Downloaded file is not valid JSON: {output_file}")
                    output_file.unlink()
                    self.stats['errors'] += 1
                    return False

                file_size = output_file.stat().st_size
                self.stats['total_bytes'] += file_size
                self.stats['downloaded'] += 1
                self.downloaded_files.add(file_key)

                if self.debug:
                    log(f"  Downloaded: {filename} ({file_size / 1024:.1f} KB)")

                return True

            except requests.exceptions.RequestException as e:
                if attempt < max_retries - 1:
                    if self.debug:
                        log(f"  Retry {attempt + 1}/{max_retries} for {filename}: {e}")
                    time.sleep(retry_delay)
                    retry_delay *= 2
                else:
                    log(f"ERROR: Failed to download {filename} after {max_retries} attempts: {e}")
                    self.stats['errors'] += 1
                    return False

        return False

    def download_all(self):
        """Download all files from the contents index."""
        log("="*60)
        log("USPTO-Chem Historical Data Download (Biobtree)")
        log("="*60)
        log(f"Output directory: {self.output_dir}")
        log(f"Year range: {self.start_year}-{self.end_year}")
        log(f"Datasets: {', '.join(self.datasets)}")

        contents = self.fetch_contents_index()
        file_list = self.extract_file_list(contents)

        if not file_list:
            log("No files to download!")
            return

        self.stats['total_files'] = len(file_list)

        log(f"\nDownloading {len(file_list)} files...")
        log(f"Already downloaded: {len(self.downloaded_files)} files")

        success_count = 0
        for file_info in tqdm(file_list, desc="Downloading files"):
            if self.download_file(file_info):
                success_count += 1

            if success_count % 100 == 0:
                self._save_tracking()

        self._save_tracking()

        log("\n" + "="*60)
        log("Download Statistics")
        log("="*60)
        log(f"Total files: {self.stats['total_files']}")
        log(f"Downloaded: {self.stats['downloaded']}")
        log(f"Skipped (already downloaded): {self.stats['skipped']}")
        log(f"Errors: {self.stats['errors']}")
        log(f"Total data: {self.stats['total_bytes'] / 1024 / 1024 / 1024:.2f} GB")


def main():
    parser = argparse.ArgumentParser(description='Download USPTO-Chem historical JSON data')
    parser.add_argument('--output-dir', type=str, required=True,
                       help='Output directory for JSON files')
    parser.add_argument('--tracking-file', type=str, required=True,
                       help='Tracking file for resumable downloads')
    parser.add_argument('--datasets', nargs='+', default=['applications', 'grants'],
                       choices=['applications', 'grants'],
                       help='Datasets to download (default: both)')
    parser.add_argument('--start-year', type=int, default=2001,
                       help='Start year (default: 2001)')
    parser.add_argument('--end-year', type=int, default=2025,
                       help='End year (default: 2025)')
    parser.add_argument('--limit-files', type=int,
                       help='Limit number of files to download (test mode)')
    parser.add_argument('--debug', action='store_true',
                       help='Enable debug logging')

    args = parser.parse_args()

    downloader = USPTOChemDownloader(
        output_dir=Path(args.output_dir),
        tracking_file=Path(args.tracking_file),
        start_year=args.start_year,
        end_year=args.end_year,
        datasets=args.datasets,
        limit_files=args.limit_files,
        debug=args.debug
    )

    downloader.download_all()
    log("\nDownload complete!")


if __name__ == '__main__':
    main()
