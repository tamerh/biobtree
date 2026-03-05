#!/bin/bash
#
# Patent Data Preparation Script for Biobtree
#
# Downloads SureChEMBL patent data and converts to JSON format for biobtree ingestion.
# Optionally enriches patent data with USPTO abstracts.
#
# Usage:
#   ./prepare.sh                           # Default: data/patents/
#   ./prepare.sh --output-dir /path/to/out # Custom output directory
#   ./prepare.sh --with-uspto              # Include USPTO abstract enrichment
#   ./prepare.sh --update-mode             # Skip if data already exists
#   ./prepare.sh --verbose                 # Detailed progress output
#
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIOBTREE_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Default output directory (relative to biobtree root)
OUTPUT_DIR="${BIOBTREE_ROOT}/data/patents"
UPDATE_MODE=""
VERBOSE=""
CLEANUP_OLD=""
WITH_USPTO=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --update-mode)
            UPDATE_MODE="--update-mode"
            shift
            ;;
        --verbose|-v)
            VERBOSE="--verbose"
            shift
            ;;
        --cleanup-old)
            CLEANUP_OLD="--cleanup-old $2"
            shift 2
            ;;
        --with-uspto)
            WITH_USPTO="yes"
            shift
            ;;
        --help|-h)
            echo "Patent Data Preparation Script"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --output-dir DIR   Output directory (default: data/patents/)"
            echo "  --update-mode      Skip download if release already exists"
            echo "  --verbose, -v      Print detailed progress"
            echo "  --cleanup-old N    Keep only N latest releases"
            echo "  --with-uspto       Include USPTO abstract enrichment"
            echo "  --help, -h         Show this help"
            echo ""
            echo "Output structure:"
            echo "  DIR/surechembl/YYYY-MM-DD/  Raw parquet files"
            echo "  DIR/uspto_historical/       USPTO JSON files (if --with-uspto)"
            echo "  DIR/biobtree/               JSON files for biobtree"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Create output directories
SURECHEMBL_DIR="${OUTPUT_DIR}/surechembl"
BIOBTREE_DIR="${OUTPUT_DIR}/biobtree"
USPTO_DIR="${OUTPUT_DIR}/uspto_historical"
USPTO_STATE_DIR="${OUTPUT_DIR}/state"

mkdir -p "$SURECHEMBL_DIR"
mkdir -p "$BIOBTREE_DIR"

echo "========================================================================"
echo "Patent Data Preparation for Biobtree"
echo "========================================================================"
echo "Output directory: $OUTPUT_DIR"
echo "SureChEMBL data:  $SURECHEMBL_DIR"
echo "Biobtree JSON:    $BIOBTREE_DIR"
if [ -n "$WITH_USPTO" ]; then
    echo "USPTO data:       $USPTO_DIR"
fi
echo "========================================================================"
echo ""

# Step 1: Download SureChEMBL data
echo "[Step 1] Downloading SureChEMBL data..."
python3 "$SCRIPT_DIR/download_surechembl.py" \
    --raw-dir "$SURECHEMBL_DIR" \
    $UPDATE_MODE \
    $CLEANUP_OLD

# Find the latest release directory
LATEST_RELEASE=$(ls -1 "$SURECHEMBL_DIR" | grep -E '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' | sort -r | head -1)

if [ -z "$LATEST_RELEASE" ]; then
    echo "ERROR: No release found in $SURECHEMBL_DIR"
    exit 1
fi

RELEASE_DIR="${SURECHEMBL_DIR}/${LATEST_RELEASE}"
echo "Latest release: $LATEST_RELEASE"
echo "Release directory: $RELEASE_DIR"
echo ""

# Step 2: Download USPTO data (if requested)
USPTO_PARQUET=""
if [ -n "$WITH_USPTO" ]; then
    echo "[Step 2] Downloading USPTO-Chem data..."
    mkdir -p "$USPTO_DIR"
    mkdir -p "$USPTO_STATE_DIR"

    python3 "$SCRIPT_DIR/download_uspto_chem.py" \
        --output-dir "$USPTO_DIR" \
        --tracking-file "$USPTO_STATE_DIR/uspto_download.json"

    # Step 3: Process USPTO JSON to parquet
    echo ""
    echo "[Step 3] Processing USPTO JSON to parquet..."
    USPTO_PARQUET="${OUTPUT_DIR}/uspto_historical.parquet"

    python3 "$SCRIPT_DIR/process_uspto_json.py" \
        --input-dir "$USPTO_DIR" \
        --output "$USPTO_PARQUET"

    echo "USPTO parquet: $USPTO_PARQUET"
    echo ""
fi

# Step 4: Convert parquet to JSON
if [ -n "$WITH_USPTO" ]; then
    echo "[Step 4] Converting parquet to JSON (with USPTO abstracts)..."
    python3 "$SCRIPT_DIR/convert_to_biobtree_json.py" \
        --input "$RELEASE_DIR" \
        --output "$BIOBTREE_DIR" \
        --uspto-parquet "$USPTO_PARQUET" \
        $VERBOSE
else
    echo "[Step 2] Converting parquet to JSON..."
    python3 "$SCRIPT_DIR/convert_to_biobtree_json.py" \
        --input "$RELEASE_DIR" \
        --output "$BIOBTREE_DIR" \
        $VERBOSE
fi

echo ""
echo "========================================================================"
echo "Patent data preparation complete!"
echo "========================================================================"
echo "Release: $LATEST_RELEASE"
echo "Biobtree JSON files: $BIOBTREE_DIR"
if [ -n "$WITH_USPTO" ]; then
    echo "USPTO abstracts: Merged from $USPTO_PARQUET"
fi
echo ""
echo "To build biobtree with patents:"
echo "  biobtree -d patent build"
echo "========================================================================"
