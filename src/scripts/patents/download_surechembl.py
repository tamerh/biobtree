#!/usr/bin/env python3
"""
SureChEMBL Data Downloader for Biobtree

Downloads SureChEMBL Parquet files from EMBL-EBI FTP server.
Automatically fetches latest release from remote FTP and skips download if files already exist.

Data Source: https://ftp.ebi.ac.uk/pub/databases/chembl/SureChEMBL/bulk_data/

Files downloaded per release:
- compounds.parquet          (~20M compounds)
- patents.parquet           (~17M patents)
- patent_compound_map.parquet
- metadata.parquet

Usage:
    # Download latest release
    python download_surechembl.py --raw-dir data/patents/surechembl

    # Update mode (skip download if release already exists on disk)
    python download_surechembl.py --raw-dir data/patents/surechembl --update-mode

    # Test mode with debug
    python download_surechembl.py --raw-dir data/patents/surechembl --debug --limit-files 2
"""

import os
import sys
import requests
import time
import argparse
from datetime import datetime
from pathlib import Path
from typing import List, Dict, Optional
from urllib.parse import urljoin
from bs4 import BeautifulSoup
from tqdm import tqdm


def log(message: str) -> None:
    """Print message with timestamp."""
    timestamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
    print(f"[{timestamp}] {message}", flush=True)


class SureChEMBLDownloader:
    """Downloads SureChEMBL bulk data from EMBL-EBI FTP server."""

    BASE_URL = "https://ftp.ebi.ac.uk/pub/databases/chembl/SureChEMBL/bulk_data/"

    # Core Parquet files in each release (required)
    EXPECTED_FILES = [
        'compounds.parquet',
        'patents.parquet',
        'patent_compound_map.parquet',
        'fields.parquet'
    ]

    def __init__(self, raw_dir: str, session: Optional[requests.Session] = None):
        """
        Initialize downloader.

        Args:
            raw_dir: Base directory for raw data storage
            session: Optional requests session (will create if None)
        """
        self.raw_dir = Path(raw_dir)
        self.raw_dir.mkdir(parents=True, exist_ok=True)

        self.session = session or requests.Session()
        self.session.headers.update({
            'User-Agent': 'Biobtree Patents Pipeline (https://github.com/tamerh/biobtree)'
        })

    def get_available_releases(self) -> List[str]:
        """
        Get list of available release directories from FTP server.

        Returns:
            List of release directory names (sorted by date, newest first)
        """
        log(f"Fetching available releases from {self.BASE_URL}")

        try:
            response = self.session.get(self.BASE_URL, timeout=30)
            response.raise_for_status()

            soup = BeautifulSoup(response.content, 'html.parser')

            # Find directory links (look for YYYY-MM-DD pattern)
            releases = []
            for link in soup.find_all('a', href=True):
                href = link['href']
                # Match directory pattern: YYYY-MM-DD/
                if href.endswith('/') and len(href.split('-')) == 3:
                    release_name = href.rstrip('/')
                    try:
                        # Validate it's a valid date
                        datetime.strptime(release_name, '%Y-%m-%d')
                        releases.append(release_name)
                    except ValueError:
                        continue

            # Sort by date (newest first)
            releases.sort(reverse=True)

            log(f"Found {len(releases)} releases")
            if releases:
                log(f"Latest release: {releases[0]}")

            return releases

        except requests.RequestException as e:
            log(f"ERROR: Failed to fetch release list: {e}")
            raise
        except Exception as e:
            log(f"ERROR: Failed to parse FTP directory: {e}")
            raise

    def get_latest_release(self) -> Optional[str]:
        """
        Get the latest release version.

        Returns:
            Latest release name (e.g., "2025-10-15") or None if not found
        """
        releases = self.get_available_releases()
        return releases[0] if releases else None

    def check_existing_release(self, release_version: str) -> bool:
        """
        Check if a release has been fully downloaded locally.

        Args:
            release_version: Release version to check

        Returns:
            True if all expected files exist
        """
        release_dir = self.raw_dir / release_version

        if not release_dir.exists():
            return False

        # Check if all expected files are present and non-empty
        for filename in self.EXPECTED_FILES:
            file_path = release_dir / filename
            if not file_path.exists() or file_path.stat().st_size == 0:
                return False

        return True

    def download_file(self, url: str, local_path: Path, retries: int = 3) -> bool:
        """
        Download a single file with progress bar and retry logic.

        Args:
            url: URL to download from
            local_path: Local path to save to
            retries: Number of retry attempts

        Returns:
            True if successful, False otherwise
        """
        for attempt in range(1, retries + 1):
            try:
                log(f"Downloading {url}")

                # Stream download with progress bar
                response = self.session.get(url, stream=True, timeout=60)
                response.raise_for_status()

                total_size = int(response.headers.get('content-length', 0))

                # Ensure parent directory exists
                local_path.parent.mkdir(parents=True, exist_ok=True)

                # Download with progress bar
                with open(local_path, 'wb') as f:
                    if total_size == 0:
                        # Size unknown, download without progress bar
                        f.write(response.content)
                        log(f"Downloaded {local_path.name}")
                    else:
                        # Download with tqdm progress bar
                        with tqdm(
                            desc=local_path.name,
                            total=total_size,
                            unit='B',
                            unit_scale=True,
                            unit_divisor=1024,
                        ) as bar:
                            for chunk in response.iter_content(chunk_size=8192):
                                if chunk:
                                    f.write(chunk)
                                    bar.update(len(chunk))

                # Verify file was created and has size
                if local_path.exists() and local_path.stat().st_size > 0:
                    file_size_mb = local_path.stat().st_size / (1024 * 1024)
                    log(f"Successfully downloaded {local_path.name} ({file_size_mb:.1f}MB)")
                    return True
                else:
                    raise RuntimeError("Downloaded file is empty or missing")

            except Exception as e:
                error_msg = str(e) if str(e) else f"{type(e).__name__}"
                if attempt < retries:
                    log(f"Attempt {attempt}/{retries} failed: {error_msg}")
                    log(f"Retrying in {2 ** attempt} seconds...")
                    time.sleep(2 ** attempt)
                else:
                    log(f"FAILED after {retries} attempts: {error_msg}")

                    # Clean up partial download
                    if local_path.exists():
                        local_path.unlink()

                    return False

        return False

    def download_release(self, release_version: str, limit_files: Optional[int] = None,
                        skip_existing: bool = True) -> bool:
        """
        Download all files for a specific release.

        Args:
            release_version: Release version to download (e.g., "2025-10-15")
            limit_files: Optional limit on number of files to download (for testing)
            skip_existing: Skip files that already exist locally

        Returns:
            True if all files downloaded successfully
        """
        log(f"\n{'='*70}")
        log(f"Downloading SureChEMBL Release: {release_version}")
        log(f"{'='*70}\n")

        release_dir = self.raw_dir / release_version
        release_url = urljoin(self.BASE_URL, f"{release_version}/")

        files_to_download = self.EXPECTED_FILES[:limit_files] if limit_files else self.EXPECTED_FILES

        success_count = 0
        total_files = len(files_to_download)

        for i, filename in enumerate(files_to_download, 1):
            log(f"\n[{i}/{total_files}] Processing {filename}")

            local_path = release_dir / filename
            file_url = urljoin(release_url, filename)

            # Check if file already exists
            if skip_existing and local_path.exists() and local_path.stat().st_size > 0:
                file_size_mb = local_path.stat().st_size / (1024 * 1024)
                log(f"File already exists ({file_size_mb:.1f}MB), skipping")
                success_count += 1
                continue

            # Download file
            if self.download_file(file_url, local_path):
                success_count += 1
            else:
                log(f"ERROR: Failed to download {filename}")

        log(f"\n{'='*70}")
        log(f"Download Summary: {success_count}/{total_files} files successful")
        log(f"{'='*70}\n")

        return success_count == total_files

    def cleanup_old_releases(self, keep_latest: int = 2):
        """
        Remove old release directories, keeping only the latest N.

        Args:
            keep_latest: Number of latest releases to keep
        """
        if not self.raw_dir.exists():
            return

        # Get all release directories
        release_dirs = []
        for item in self.raw_dir.iterdir():
            if item.is_dir():
                try:
                    # Validate directory name is a date
                    datetime.strptime(item.name, '%Y-%m-%d')
                    release_dirs.append(item)
                except ValueError:
                    continue

        # Sort by date (newest first)
        release_dirs.sort(key=lambda x: x.name, reverse=True)

        # Remove old releases
        if len(release_dirs) > keep_latest:
            log(f"\nCleaning up old releases (keeping latest {keep_latest})")

            for old_dir in release_dirs[keep_latest:]:
                log(f"Removing old release: {old_dir.name}")
                try:
                    import shutil
                    shutil.rmtree(old_dir)
                except Exception as e:
                    log(f"WARNING: Could not remove {old_dir.name}: {e}")

    def get_latest_local_release(self) -> Optional[str]:
        """
        Get the latest release version available locally.

        Returns:
            Latest local release name or None if none found
        """
        if not self.raw_dir.exists():
            return None

        release_dirs = []
        for item in self.raw_dir.iterdir():
            if item.is_dir():
                try:
                    datetime.strptime(item.name, '%Y-%m-%d')
                    if self.check_existing_release(item.name):
                        release_dirs.append(item.name)
                except ValueError:
                    continue

        release_dirs.sort(reverse=True)
        return release_dirs[0] if release_dirs else None


def main():
    """Main execution function."""
    parser = argparse.ArgumentParser(
        description='Download SureChEMBL patent data for biobtree',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Download latest release
  %(prog)s --raw-dir data/patents/surechembl

  # Update mode (download only if new)
  %(prog)s --raw-dir data/patents/surechembl --update-mode

  # Debug mode (limit files)
  %(prog)s --raw-dir data/patents/surechembl --debug --limit-files 2
        """
    )
    parser.add_argument('--raw-dir', required=True,
                       help='Base directory for raw data storage')
    parser.add_argument('--release', default=None,
                       help='Specific release to download (e.g., "2025-10-15"). Default: latest')
    parser.add_argument('--update-mode', action='store_true',
                       help='Update mode: skip download if release already exists on disk')
    parser.add_argument('--debug', action='store_true',
                       help='Debug mode: enable verbose logging')
    parser.add_argument('--limit-files', type=int, default=None,
                       help='Limit number of files to download (for testing)')
    parser.add_argument('--cleanup-old', type=int, default=None,
                       help='Keep only N latest releases (cleanup old ones)')

    args = parser.parse_args()

    log("=" * 70)
    log("SureChEMBL Data Downloader (Biobtree)")
    log("=" * 70)

    try:
        downloader = SureChEMBLDownloader(args.raw_dir)

        # Determine which release to download
        if args.release:
            release_version = args.release
            log(f"Using specified release: {release_version}")
        else:
            release_version = downloader.get_latest_release()
            if not release_version:
                log("ERROR: No releases found on FTP server")
                return 1
            log(f"Latest release: {release_version}")

        # Update mode: check if we already have this release on disk
        if args.update_mode:
            if downloader.check_existing_release(release_version):
                log(f"Release {release_version} already exists on disk")
                log("UPDATE MODE: Skipping download")
                return 0
            else:
                log(f"Release {release_version} not found locally, proceeding with download...")

        # Download the release
        success = downloader.download_release(
            release_version,
            limit_files=args.limit_files,
            skip_existing=True
        )

        if not success:
            log("ERROR: Download failed")
            return 1

        log(f"\nSuccessfully downloaded release: {release_version}")

        # Cleanup old releases if requested
        if args.cleanup_old:
            downloader.cleanup_old_releases(keep_latest=args.cleanup_old)

        log("\n" + "=" * 70)
        log("Download complete!")
        log(f"Release: {release_version}")
        log(f"Location: {downloader.raw_dir / release_version}")
        log("=" * 70)

        return 0

    except KeyboardInterrupt:
        log("\nDownload interrupted by user")
        return 1
    except Exception as e:
        log(f"FATAL ERROR: {e}")
        if args.debug:
            import traceback
            traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.exit(main())
