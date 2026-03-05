#!/usr/bin/env python3
"""
SpliceAI Data Processing Script

Downloads and processes SpliceAI precomputed scores from Broad Institute.
Merges gain/loss BED files into a single TSV for biobtree integration.

Input: BED files from Google Cloud Storage (filtered for high-impact variants)
Output: TSV file with variant-level splice predictions

Usage:
    python process_spliceai.py --output /path/to/spliceai_filtered.tsv
    python process_spliceai.py --output /path/to/spliceai_filtered.tsv --threshold 0.5
"""

import argparse
import gzip
import os
import re
import sys
import urllib.request
from collections import defaultdict
from typing import Dict, Iterator, Tuple

# Broad Institute precomputed SpliceAI scores (filtered)
BASE_URL = "https://storage.googleapis.com/tgg-viewer/ref/GRCh38/spliceai"
BED_FILES = {
    "0.2": {
        "splice_loss": f"{BASE_URL}/spliceai_scores.raw.snps_and_indels.hg38.filtered.sorted.score_0.2.splice_loss.bed.gz",
        "splice_gain": f"{BASE_URL}/spliceai_scores.raw.snps_and_indels.hg38.filtered.sorted.score_0.2.splice_gain.bed.gz",
    },
    "0.5": {
        "splice_loss": f"{BASE_URL}/spliceai_scores.raw.snps_and_indels.hg38.filtered.sorted.score_0.5.splice_loss.bed.gz",
        "splice_gain": f"{BASE_URL}/spliceai_scores.raw.snps_and_indels.hg38.filtered.sorted.score_0.5.splice_gain.bed.gz",
    },
}


def parse_annotations(annotation_str: str) -> Dict[str, str]:
    """Parse BED annotation field (key=value;key=value;...)"""
    result = {}
    for item in annotation_str.split(";"):
        if "=" in item:
            key, value = item.split("=", 1)
            result[key.strip()] = value.strip()
    return result


def parse_allele(allele_str: str) -> Tuple[str, int, str, str]:
    """
    Parse allele string from BED annotations.

    Format: chr-pos-ref-alt (e.g., "1-803428-TCCAT-T")
    Returns: (chromosome, position, ref_allele, alt_allele)
    """
    parts = allele_str.split("-")
    if len(parts) >= 4:
        chrom = parts[0]
        pos = int(parts[1])
        ref = parts[2]
        alt = "-".join(parts[3:])  # Handle alt alleles with dashes
        return chrom, pos, ref, alt
    return None, None, None, None


def download_file(url: str, output_path: str, force: bool = False) -> str:
    """Download file from URL if not already present."""
    if os.path.exists(output_path) and not force:
        print(f"  File exists: {output_path}")
        return output_path

    print(f"  Downloading: {url}")
    print(f"  To: {output_path}")

    try:
        urllib.request.urlretrieve(url, output_path)
        print(f"  Downloaded: {os.path.getsize(output_path) / 1024 / 1024:.1f} MB")
    except Exception as e:
        print(f"  Error downloading: {e}")
        raise

    return output_path


def parse_bed_file(filepath: str) -> Iterator[Dict]:
    """
    Parse SpliceAI BED file and yield variant records.

    BED format:
    chr  start  end  annotations  score  strand

    Annotations contain: effect, score, allele, etc.
    """
    open_func = gzip.open if filepath.endswith(".gz") else open

    with open_func(filepath, "rt") as f:
        for line_num, line in enumerate(f, 1):
            line = line.strip()
            if not line or line.startswith("#"):
                continue

            parts = line.split("\t")
            if len(parts) < 4:
                continue

            # Parse annotations from column 4
            annotations = parse_annotations(parts[3])

            # Get effect type
            effect = annotations.get("effect", "")
            if not effect:
                continue

            # Get score
            score_str = annotations.get("score") or annotations.get("max_score", "")
            if not score_str:
                continue

            try:
                score = float(score_str)
            except ValueError:
                continue

            # Parse allele information
            allele_str = annotations.get("allele") or annotations.get("allele_with_max_score", "")
            if not allele_str:
                continue

            chrom, pos, ref, alt = parse_allele(allele_str)
            if chrom is None:
                continue

            yield {
                "chromosome": chrom,
                "position": pos,
                "ref_allele": ref,
                "alt_allele": alt,
                "effect": effect,
                "score": score,
                "allele_info": allele_str,
            }


def process_spliceai_data(
    output_path: str,
    cache_dir: str,
    threshold: str = "0.2",
    force_download: bool = False,
    test_limit: int = 0,
) -> int:
    """
    Download and process SpliceAI BED files into TSV format.

    Args:
        output_path: Path for output TSV file
        cache_dir: Directory for downloaded BED files
        threshold: Score threshold ("0.2" or "0.5")
        force_download: Force re-download of files
        test_limit: Limit number of variants (0 = no limit)

    Returns:
        Number of variants written
    """
    os.makedirs(cache_dir, exist_ok=True)

    # Select files based on threshold
    if threshold not in BED_FILES:
        raise ValueError(f"Invalid threshold: {threshold}. Must be '0.2' or '0.5'")

    files = BED_FILES[threshold]

    # Download files
    print(f"\nDownloading SpliceAI files (threshold={threshold})...")
    local_files = {}
    for file_type, url in files.items():
        filename = os.path.basename(url)
        local_path = os.path.join(cache_dir, filename)
        local_files[file_type] = download_file(url, local_path, force_download)

    # Process files and merge variants
    # Use dict to deduplicate (same variant may appear in multiple files)
    # Key: (chr, pos, ref, alt, effect)
    # Value: best score for that variant/effect combination
    variants = {}

    print(f"\nProcessing BED files...")
    for file_type, filepath in local_files.items():
        print(f"  Processing: {os.path.basename(filepath)}")
        count = 0

        for record in parse_bed_file(filepath):
            key = (
                record["chromosome"],
                record["position"],
                record["ref_allele"],
                record["alt_allele"],
                record["effect"],
            )

            # Keep the highest score for each variant/effect
            if key not in variants or record["score"] > variants[key]["score"]:
                variants[key] = record

            count += 1
            if test_limit > 0 and count >= test_limit:
                break

        print(f"    Parsed {count:,} records")

    print(f"\nTotal unique variants: {len(variants):,}")

    # Write output TSV
    print(f"\nWriting output: {output_path}")

    with open(output_path, "w") as f:
        # Header
        f.write("chromosome\tposition\tref_allele\talt_allele\teffect\tscore\tallele_info\n")

        # Sort by chromosome and position for consistent output
        sorted_keys = sorted(variants.keys(), key=lambda k: (
            int(k[0]) if k[0].isdigit() else ord(k[0][0]) + 100,  # Sort X, Y, M after numbers
            k[1],  # position
            k[2],  # ref
            k[3],  # alt
            k[4],  # effect
        ))

        for key in sorted_keys:
            v = variants[key]
            f.write(f"{v['chromosome']}\t{v['position']}\t{v['ref_allele']}\t{v['alt_allele']}\t{v['effect']}\t{v['score']}\t{v['allele_info']}\n")

    print(f"Written {len(variants):,} variants")
    return len(variants)


def main():
    parser = argparse.ArgumentParser(
        description="Process SpliceAI precomputed scores from Broad Institute"
    )
    parser.add_argument(
        "--output", "-o",
        default="spliceai_filtered.tsv",
        help="Output TSV file path (default: spliceai_filtered.tsv)"
    )
    parser.add_argument(
        "--cache-dir", "-c",
        default="cache",
        help="Directory for downloaded BED files (default: cache)"
    )
    parser.add_argument(
        "--threshold", "-t",
        choices=["0.2", "0.5"],
        default="0.2",
        help="Score threshold for filtering (default: 0.2)"
    )
    parser.add_argument(
        "--force-download", "-f",
        action="store_true",
        help="Force re-download of files"
    )
    parser.add_argument(
        "--test-limit",
        type=int,
        default=0,
        help="Limit number of variants per file (for testing, 0 = no limit)"
    )

    args = parser.parse_args()

    print("=" * 60)
    print("SpliceAI Data Processor")
    print("=" * 60)
    print(f"Output: {args.output}")
    print(f"Cache: {args.cache_dir}")
    print(f"Threshold: {args.threshold}")

    try:
        count = process_spliceai_data(
            output_path=args.output,
            cache_dir=args.cache_dir,
            threshold=args.threshold,
            force_download=args.force_download,
            test_limit=args.test_limit,
        )
        print("\n" + "=" * 60)
        print(f"SUCCESS: Processed {count:,} variants")
        print("=" * 60)
        return 0
    except Exception as e:
        print(f"\nERROR: {e}")
        import traceback
        traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.exit(main())
