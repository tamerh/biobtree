#!/usr/bin/env python3
"""
Clinical Trials Tracking Database for Biobtree

Tracks which trials have been processed and their content hashes
to enable incremental updates by detecting new and modified trials.

Usage:
    from tracking_db import TrialsTracker, compute_trial_hash

    tracker = TrialsTracker("data/clinical_trials/state/tracking.db")

    # Add or update a trial
    tracker.add_or_update_trial("NCT12345678", "2025-01-15", "abc123hash")

    # Get trial info
    trial = tracker.get_trial("NCT12345678")

    # Get statistics
    stats = tracker.get_stats()
"""

import sqlite3
import hashlib
import json
from pathlib import Path
from datetime import datetime
from typing import Optional, Dict, List, Any


class TrialsTracker:
    """Manages SQLite database for tracking clinical trials processing state."""

    def __init__(self, db_path: str):
        """
        Initialize the trials tracker.

        Args:
            db_path: Path to SQLite database file
        """
        self.db_path = Path(db_path)

        # Create parent directory if it doesn't exist
        self.db_path.parent.mkdir(parents=True, exist_ok=True)

        # Initialize database
        self.init_database()

    def init_database(self):
        """Create tracking table and indices if they don't exist."""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                CREATE TABLE IF NOT EXISTS trials (
                    nct_id TEXT PRIMARY KEY,
                    last_update_date TEXT,
                    content_hash TEXT,
                    last_processed_date TEXT
                )
            """)

            # Create index for faster lookups
            conn.execute("CREATE INDEX IF NOT EXISTS idx_nct ON trials(nct_id)")
            conn.execute("CREATE INDEX IF NOT EXISTS idx_hash ON trials(content_hash)")

            conn.commit()

    def get_trial(self, nct_id: str) -> Optional[Dict[str, str]]:
        """
        Get trial information from database.

        Args:
            nct_id: NCT ID to look up

        Returns:
            Dictionary with trial info or None if not found
        """
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            cursor = conn.execute(
                "SELECT * FROM trials WHERE nct_id = ?",
                (nct_id,)
            )
            row = cursor.fetchone()

            if row:
                return dict(row)
            return None

    def add_or_update_trial(self, nct_id: str, last_update_date: str, content_hash: str):
        """
        Add or update trial record.

        Args:
            nct_id: NCT ID
            last_update_date: Last update date from AACT data
            content_hash: Hash of trial content for change detection
        """
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                INSERT OR REPLACE INTO trials
                (nct_id, last_update_date, content_hash, last_processed_date)
                VALUES (?, ?, ?, datetime('now'))
            """, (nct_id, last_update_date, content_hash))

            conn.commit()

    def add_or_update_batch(self, trials: List[Dict[str, str]], batch_size: int = 1000):
        """
        Add or update multiple trials in batches for better performance.

        Args:
            trials: List of dicts with keys: nct_id, last_update_date, content_hash
            batch_size: Number of records to insert per batch
        """
        with sqlite3.connect(self.db_path) as conn:
            for i in range(0, len(trials), batch_size):
                batch = trials[i:i + batch_size]
                conn.executemany("""
                    INSERT OR REPLACE INTO trials
                    (nct_id, last_update_date, content_hash, last_processed_date)
                    VALUES (?, ?, ?, datetime('now'))
                """, [(t['nct_id'], t['last_update_date'], t['content_hash']) for t in batch])

            conn.commit()

    def get_all_nct_ids(self) -> List[str]:
        """
        Get all tracked NCT IDs.

        Returns:
            List of NCT IDs
        """
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.execute("SELECT nct_id FROM trials")
            return [row[0] for row in cursor.fetchall()]

    def get_all_trials(self) -> List[Dict[str, str]]:
        """
        Get all tracked trials with full info.

        Returns:
            List of trial dictionaries
        """
        with sqlite3.connect(self.db_path) as conn:
            conn.row_factory = sqlite3.Row
            cursor = conn.execute("SELECT * FROM trials")
            return [dict(row) for row in cursor.fetchall()]

    def trial_exists(self, nct_id: str) -> bool:
        """
        Check if trial exists in tracking database.

        Args:
            nct_id: NCT ID to check

        Returns:
            True if trial exists, False otherwise
        """
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.execute(
                "SELECT 1 FROM trials WHERE nct_id = ? LIMIT 1",
                (nct_id,)
            )
            return cursor.fetchone() is not None

    def get_stats(self) -> Dict[str, Any]:
        """
        Get statistics about the tracking database.

        Returns:
            Dictionary with stats
        """
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.execute("SELECT COUNT(*) FROM trials")
            total_trials = cursor.fetchone()[0]

            cursor = conn.execute("SELECT MIN(last_processed_date) FROM trials")
            oldest_processed = cursor.fetchone()[0]

            cursor = conn.execute("SELECT MAX(last_processed_date) FROM trials")
            newest_processed = cursor.fetchone()[0]

            # Get database file size
            db_size_bytes = self.db_path.stat().st_size if self.db_path.exists() else 0
            db_size_mb = db_size_bytes / (1024 * 1024)

            return {
                "total_trials": total_trials,
                "oldest_processed": oldest_processed,
                "newest_processed": newest_processed,
                "db_size_mb": round(db_size_mb, 2),
                "db_path": str(self.db_path)
            }

    def reset(self):
        """Delete all records from the tracking database."""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM trials")
            conn.commit()

    def delete_trial(self, nct_id: str):
        """
        Delete a specific trial from tracking database.

        Args:
            nct_id: NCT ID to delete
        """
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM trials WHERE nct_id = ?", (nct_id,))
            conn.commit()


def compute_trial_hash(trial_data: Dict[str, Any]) -> str:
    """
    Compute a hash of trial content for change detection.

    Args:
        trial_data: Dictionary containing trial information

    Returns:
        SHA256 hash of the trial content
    """
    # Extract key fields that matter for change detection
    # Use sorted keys to ensure consistent ordering
    relevant_fields = [
        'nct_id',
        'brief_title',
        'brief_summary',
        'detailed_description',
        'overall_status',
        'phase',
        'study_type',
        'interventions',
        'conditions',
        'sponsors',
        'publications'
    ]

    # Build a canonical representation
    content_parts = []
    for field in relevant_fields:
        value = trial_data.get(field, '')

        # Convert lists/dicts to sorted JSON for consistent hashing
        if isinstance(value, (list, dict)):
            value = json.dumps(value, sort_keys=True)

        content_parts.append(f"{field}:{value}")

    # Compute hash
    content_str = '|'.join(content_parts)
    return hashlib.sha256(content_str.encode('utf-8')).hexdigest()


def main():
    """CLI interface for tracking database operations."""
    import argparse

    parser = argparse.ArgumentParser(
        description="Clinical Trials Tracking Database Management"
    )
    parser.add_argument(
        '--tracking-db',
        type=str,
        default='data/clinical_trials/state/tracking.db',
        help='Path to tracking database'
    )
    parser.add_argument(
        'command',
        choices=['stats', 'list', 'get', 'reset', 'delete'],
        help='Command to execute'
    )
    parser.add_argument(
        '--nct-id',
        type=str,
        help='NCT ID (for get/delete commands)'
    )
    parser.add_argument(
        '--limit',
        type=int,
        default=10,
        help='Limit for list command'
    )

    args = parser.parse_args()

    # Initialize tracker
    tracker = TrialsTracker(args.tracking_db)

    if args.command == 'stats':
        stats = tracker.get_stats()
        print("\n=== Tracking Database Statistics ===")
        print(f"Database: {stats['db_path']}")
        print(f"Total trials tracked: {stats['total_trials']:,}")
        print(f"Oldest processed: {stats['oldest_processed']}")
        print(f"Newest processed: {stats['newest_processed']}")
        print(f"Database size: {stats['db_size_mb']} MB")
        print()

    elif args.command == 'list':
        trials = tracker.get_all_trials()
        print(f"\n=== Showing first {args.limit} trials ===")
        for trial in trials[:args.limit]:
            print(f"\nNCT ID: {trial['nct_id']}")
            print(f"  Last Update: {trial['last_update_date']}")
            print(f"  Processed: {trial['last_processed_date']}")
            print(f"  Hash: {trial['content_hash'][:16]}...")

    elif args.command == 'get':
        if not args.nct_id:
            print("Error: --nct-id required for 'get' command")
            return

        trial = tracker.get_trial(args.nct_id)
        if trial:
            print(f"\n=== Trial {args.nct_id} ===")
            for key, value in trial.items():
                print(f"{key}: {value}")
        else:
            print(f"Trial {args.nct_id} not found in tracking database")

    elif args.command == 'reset':
        response = input("Are you sure you want to delete all tracking records? (yes/no): ")
        if response.lower() == 'yes':
            tracker.reset()
            print("Tracking database reset complete")
        else:
            print("Reset cancelled")

    elif args.command == 'delete':
        if not args.nct_id:
            print("Error: --nct-id required for 'delete' command")
            return

        tracker.delete_trial(args.nct_id)
        print(f"Deleted trial {args.nct_id} from tracking database")


if __name__ == '__main__':
    main()
