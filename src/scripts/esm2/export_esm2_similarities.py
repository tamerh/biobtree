#!/usr/bin/env python3
"""
ESM2 Protein Similarity Export Script for Biobtree

Exports top-K similar proteins from Qdrant ESM2 collection to TSV format
for biobtree integration.

Features:
    - Batch scrolling through Qdrant collection
    - Parallel similarity searches using thread pool
    - Streaming writes (memory efficient)
    - Checkpoint/resume capability
    - Progress tracking

Output TSV format:
    query_id    target_id    cosine_similarity    rank

Requirements:
    pip install qdrant-client tqdm

Usage:
    python export_esm2_similarities.py \
        --qdrant-url http://localhost:6333 \
        --output /path/to/esm2_similarities_top50.tsv \
        --top-k 50 \
        --workers 8

Author: Biobtree Project
Date: 2026-02
"""

import argparse
import json
import logging
import os
import sys
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from threading import Lock
from typing import Dict, List, Optional, Tuple, Iterator

try:
    from qdrant_client import QdrantClient
    from qdrant_client.models import PointStruct, ScrollRequest
except ImportError:
    print("ERROR: qdrant-client not installed. Run: pip install qdrant-client")
    sys.exit(1)

try:
    from tqdm import tqdm
except ImportError:
    print("ERROR: tqdm not installed. Run: pip install tqdm")
    sys.exit(1)

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger(__name__)


@dataclass
class ExportConfig:
    """Configuration for ESM2 similarity export."""
    qdrant_url: str
    collection: str
    output_file: str
    top_k: int
    scroll_batch_size: int
    search_workers: int
    search_batch_size: int  # Number of proteins per batch query
    checkpoint_file: Optional[str]
    checkpoint_interval: int
    timeout: int


class ProgressTracker:
    """Thread-safe progress tracking with checkpoint support."""

    def __init__(self, total: int, checkpoint_file: Optional[str] = None):
        self.total = total
        self.processed = 0
        self.checkpoint_file = checkpoint_file
        self.processed_ids: set = set()
        self.lock = Lock()
        self.start_time = time.time()

        # Load checkpoint if exists
        if checkpoint_file and Path(checkpoint_file).exists():
            self._load_checkpoint()

    def _load_checkpoint(self):
        """Load processed IDs from checkpoint file."""
        try:
            with open(self.checkpoint_file, 'r') as f:
                self.processed_ids = set(line.strip() for line in f if line.strip())
            self.processed = len(self.processed_ids)
            logger.info(f"Resumed from checkpoint: {self.processed:,} proteins already processed")
        except Exception as e:
            logger.warning(f"Failed to load checkpoint: {e}")

    def is_processed(self, protein_id: str) -> bool:
        """Check if protein was already processed."""
        return protein_id in self.processed_ids

    def mark_processed(self, protein_id: str):
        """Mark protein as processed."""
        with self.lock:
            self.processed_ids.add(protein_id)
            self.processed += 1

    def save_checkpoint(self, force: bool = False):
        """Save checkpoint to file."""
        if not self.checkpoint_file:
            return

        with self.lock:
            if force or self.processed % 10000 == 0:
                try:
                    with open(self.checkpoint_file, 'w') as f:
                        for pid in self.processed_ids:
                            f.write(f"{pid}\n")
                except Exception as e:
                    logger.warning(f"Failed to save checkpoint: {e}")

    def get_stats(self) -> Dict:
        """Get current progress statistics."""
        elapsed = time.time() - self.start_time
        rate = self.processed / elapsed if elapsed > 0 else 0
        remaining = (self.total - self.processed) / rate if rate > 0 else 0

        return {
            'processed': self.processed,
            'total': self.total,
            'percent': (self.processed / self.total * 100) if self.total > 0 else 0,
            'rate': rate,
            'elapsed_sec': elapsed,
            'remaining_sec': remaining
        }


class TSVWriter:
    """Thread-safe streaming TSV writer."""

    HEADER = "query_id\ttarget_id\tcosine_similarity\trank\n"

    def __init__(self, output_file: str, append: bool = False):
        self.output_file = output_file
        self.lock = Lock()
        self.rows_written = 0

        # Open file
        mode = 'a' if append else 'w'
        self.file = open(output_file, mode, buffering=1)  # Line buffered

        # Write header if new file
        if not append:
            self.file.write(self.HEADER)

    def write_similarities(self, query_id: str, similarities: List[Tuple[str, float, int]]):
        """
        Write similarity results for a protein.

        Args:
            query_id: Query protein ID
            similarities: List of (target_id, cosine_similarity, rank) tuples
        """
        lines = []
        for target_id, similarity, rank in similarities:
            lines.append(f"{query_id}\t{target_id}\t{similarity:.6f}\t{rank}\n")

        with self.lock:
            self.file.writelines(lines)
            self.rows_written += len(lines)

    def flush(self):
        """Flush buffer to disk."""
        with self.lock:
            self.file.flush()

    def close(self):
        """Close file."""
        self.file.close()

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()


class QdrantExporter:
    """Exports ESM2 similarities from Qdrant to TSV."""

    def __init__(self, config: ExportConfig):
        self.config = config
        self.client = QdrantClient(
            url=config.qdrant_url,
            timeout=config.timeout
        )

        # Verify collection exists
        try:
            info = self.client.get_collection(config.collection)
            self.total_proteins = info.points_count
            self.vector_size = info.config.params.vectors.size
            logger.info(f"Collection '{config.collection}': {self.total_proteins:,} proteins, {self.vector_size}-dim vectors")
        except Exception as e:
            logger.error(f"Failed to connect to Qdrant: {e}")
            raise

    def scroll_proteins(self) -> Iterator[List[PointStruct]]:
        """
        Scroll through all proteins in collection.

        Yields batches of points with vectors and payloads.
        """
        offset = None

        while True:
            result = self.client.scroll(
                collection_name=self.config.collection,
                limit=self.config.scroll_batch_size,
                offset=offset,
                with_vectors=True,
                with_payload=True
            )

            points, next_offset = result

            if not points:
                break

            yield points

            offset = next_offset
            if offset is None:
                break

    def search_similar_batch(self, proteins: List[Tuple[str, List[float]]]) -> Dict[str, List[Tuple[str, float, int]]]:
        """
        Batch search for similar proteins.

        Args:
            proteins: List of (protein_id, vector) tuples

        Returns:
            Dict mapping protein_id to list of (target_id, cosine_similarity, rank) tuples
        """
        from qdrant_client.models import QueryRequest

        try:
            # Build batch query requests
            requests = [
                QueryRequest(
                    query=vector,
                    limit=self.config.top_k + 1,
                    with_payload=True
                )
                for protein_id, vector in proteins
            ]

            # Execute batch query
            results = self.client.query_batch_points(
                collection_name=self.config.collection,
                requests=requests
            )

            # Process results
            batch_results = {}
            for i, (protein_id, _) in enumerate(proteins):
                similarities = []
                rank = 0

                for hit in results[i].points:
                    target_id = hit.payload.get('protein_id')

                    # Skip self-match
                    if target_id == protein_id:
                        continue

                    rank += 1
                    if rank > self.config.top_k:
                        break

                    # Qdrant with Cosine distance: score is already similarity (0-1)
                    similarity = hit.score
                    similarities.append((target_id, similarity, rank))

                batch_results[protein_id] = similarities

            return batch_results

        except Exception as e:
            logger.warning(f"Batch search failed: {e}")
            # Return empty results for all proteins in batch
            return {protein_id: [] for protein_id, _ in proteins}

    def search_similar(self, protein_id: str, vector: List[float]) -> List[Tuple[str, float, int]]:
        """
        Search for similar proteins (single query fallback).

        Args:
            protein_id: Query protein ID (to exclude from results)
            vector: Query embedding vector

        Returns:
            List of (target_id, cosine_similarity, rank) tuples
        """
        try:
            # Search for top_k + 1 to account for self-match
            # Using query_points (qdrant-client >= 1.7)
            result = self.client.query_points(
                collection_name=self.config.collection,
                query=vector,
                limit=self.config.top_k + 1,
                with_payload=True
            )

            similarities = []
            rank = 0

            for hit in result.points:
                target_id = hit.payload.get('protein_id')

                # Skip self-match
                if target_id == protein_id:
                    continue

                rank += 1
                if rank > self.config.top_k:
                    break

                # Qdrant with Cosine distance: score is already similarity (0-1)
                # Higher score = more similar
                similarity = hit.score

                similarities.append((target_id, similarity, rank))

            return similarities

        except Exception as e:
            logger.warning(f"Search failed for {protein_id}: {e}")
            return []

    def export(self):
        """Run the export process using batch queries."""
        logger.info("=" * 60)
        logger.info("ESM2 Similarity Export (Batch Mode)")
        logger.info("=" * 60)
        logger.info(f"Qdrant URL: {self.config.qdrant_url}")
        logger.info(f"Collection: {self.config.collection}")
        logger.info(f"Output: {self.config.output_file}")
        logger.info(f"Top-K: {self.config.top_k}")
        logger.info(f"Scroll batch: {self.config.scroll_batch_size}")
        logger.info(f"Search batch: {self.config.search_batch_size}")
        logger.info(f"Workers: {self.config.search_workers}")
        logger.info("=" * 60)

        # Initialize tracking
        tracker = ProgressTracker(
            total=self.total_proteins,
            checkpoint_file=self.config.checkpoint_file
        )

        # Check if resuming
        append_mode = tracker.processed > 0
        if append_mode:
            logger.info(f"Resuming export from {tracker.processed:,} proteins")

        # Initialize writer
        with TSVWriter(self.config.output_file, append=append_mode) as writer:

            # Progress bar
            pbar = tqdm(
                total=self.total_proteins,
                initial=tracker.processed,
                desc="Exporting",
                unit="proteins",
                smoothing=0.1
            )

            # Process using batch queries
            for scroll_batch in self.scroll_proteins():
                # Filter out already processed proteins
                to_process = [
                    p for p in scroll_batch
                    if not tracker.is_processed(p.payload.get('protein_id'))
                ]

                if not to_process:
                    continue

                # Process in search batches
                for i in range(0, len(to_process), self.config.search_batch_size):
                    search_batch = to_process[i:i + self.config.search_batch_size]

                    # Prepare batch: list of (protein_id, vector)
                    proteins = [
                        (p.payload.get('protein_id'), p.vector)
                        for p in search_batch
                        if p.payload.get('protein_id')
                    ]

                    if not proteins:
                        continue

                    # Execute batch query
                    batch_results = self.search_similar_batch(proteins)

                    # Write results
                    for protein_id, similarities in batch_results.items():
                        if similarities:
                            writer.write_similarities(protein_id, similarities)
                        tracker.mark_processed(protein_id)
                        pbar.update(1)

                # Periodic checkpoint and flush
                if tracker.processed % self.config.checkpoint_interval == 0:
                    tracker.save_checkpoint()
                    writer.flush()

                    # Log progress
                    stats = tracker.get_stats()
                    logger.info(
                        f"Progress: {stats['processed']:,}/{stats['total']:,} "
                        f"({stats['percent']:.1f}%) | "
                        f"Rate: {stats['rate']:.1f}/s | "
                        f"ETA: {stats['remaining_sec']/3600:.1f}h"
                    )

            pbar.close()

        # Final checkpoint
        tracker.save_checkpoint(force=True)

        # Summary
        stats = tracker.get_stats()
        logger.info("=" * 60)
        logger.info("Export Complete!")
        logger.info(f"Proteins processed: {stats['processed']:,}")
        logger.info(f"Total time: {stats['elapsed_sec']/3600:.2f} hours")
        logger.info(f"Average rate: {stats['rate']:.1f} proteins/sec")
        logger.info(f"Output file: {self.config.output_file}")
        logger.info("=" * 60)


def main():
    parser = argparse.ArgumentParser(
        description="Export ESM2 protein similarities from Qdrant to TSV for biobtree",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Basic export
    python export_esm2_similarities.py \\
        --qdrant-url http://localhost:6333 \\
        --output esm2_similarities_top50.tsv

    # With resume capability
    python export_esm2_similarities.py \\
        --qdrant-url http://localhost:6333 \\
        --output esm2_similarities_top50.tsv \\
        --checkpoint esm2_checkpoint.txt

    # Custom settings
    python export_esm2_similarities.py \\
        --qdrant-url http://localhost:6333 \\
        --output esm2_similarities_top50.tsv \\
        --top-k 50 \\
        --workers 16 \\
        --scroll-batch 200
        """
    )

    parser.add_argument(
        "--qdrant-url",
        required=True,
        help="Qdrant server URL (e.g., http://localhost:6333)"
    )
    parser.add_argument(
        "--collection",
        default="esm2",
        help="Qdrant collection name (default: esm2)"
    )
    parser.add_argument(
        "--output",
        required=True,
        help="Output TSV file path"
    )
    parser.add_argument(
        "--top-k",
        type=int,
        default=50,
        help="Number of similar proteins per query (default: 50)"
    )
    parser.add_argument(
        "--scroll-batch",
        type=int,
        default=100,
        help="Batch size for scrolling through collection (default: 100)"
    )
    parser.add_argument(
        "--workers",
        type=int,
        default=4,
        help="Number of parallel workers for batch processing (default: 4)"
    )
    parser.add_argument(
        "--search-batch",
        type=int,
        default=20,
        help="Number of proteins per batch query (default: 20)"
    )
    parser.add_argument(
        "--checkpoint",
        help="Checkpoint file for resume capability"
    )
    parser.add_argument(
        "--checkpoint-interval",
        type=int,
        default=5000,
        help="Save checkpoint every N proteins (default: 5000)"
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=300,
        help="Qdrant request timeout in seconds (default: 300)"
    )

    args = parser.parse_args()

    # Create output directory if needed
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    # Create config
    config = ExportConfig(
        qdrant_url=args.qdrant_url,
        collection=args.collection,
        output_file=args.output,
        top_k=args.top_k,
        scroll_batch_size=args.scroll_batch,
        search_workers=args.workers,
        search_batch_size=args.search_batch,
        checkpoint_file=args.checkpoint,
        checkpoint_interval=args.checkpoint_interval,
        timeout=args.timeout
    )

    # Run export
    try:
        exporter = QdrantExporter(config)
        exporter.export()
    except KeyboardInterrupt:
        logger.warning("Export interrupted by user. Use --checkpoint to resume.")
        sys.exit(1)
    except Exception as e:
        logger.error(f"Export failed: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
