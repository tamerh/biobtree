#!/bin/bash
#
# Clinical Trials Data Preparation Script for Biobtree
#
# Downloads AACT clinical trials data and converts to JSON format for biobtree ingestion.
#
# Usage:
#   ./prepare.sh                           # Default: data/clinical_trials/
#   ./prepare.sh --output-dir /path/to/out # Custom output directory
#   ./prepare.sh --full                    # Force full rebuild
#   ./prepare.sh --limit 1000              # Test mode with limit
#
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIOBTREE_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Default output directory (relative to biobtree root)
OUTPUT_DIR="${BIOBTREE_ROOT}/data/clinical_trials"
LIMIT=""
FULL=""
VERBOSE=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --limit)
            LIMIT="--limit $2"
            shift 2
            ;;
        --full)
            FULL="--full"
            shift
            ;;
        --verbose|-v)
            VERBOSE="--verbose"
            shift
            ;;
        --help|-h)
            echo "Clinical Trials Data Preparation Script"
            echo ""
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --output-dir DIR   Output directory (default: data/clinical_trials/)"
            echo "  --limit N          Limit number of trials (for testing)"
            echo "  --full             Force full rebuild (ignore tracking)"
            echo "  --verbose, -v      Verbose output"
            echo "  --help, -h         Show this help"
            echo ""
            echo "Output structure:"
            echo "  DIR/downloads/     AACT snapshot ZIP files"
            echo "  DIR/extracted/     Extracted AACT tables"
            echo "  DIR/biobtree/      JSON files for biobtree"
            echo "  DIR/state/         Tracking database"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Create output directories
DOWNLOAD_DIR="${OUTPUT_DIR}/downloads"
EXTRACT_DIR="${OUTPUT_DIR}/extracted"
BIOBTREE_DIR="${OUTPUT_DIR}/biobtree"
STATE_DIR="${OUTPUT_DIR}/state"

mkdir -p "$DOWNLOAD_DIR"
mkdir -p "$EXTRACT_DIR"
mkdir -p "$BIOBTREE_DIR"
mkdir -p "$STATE_DIR"

echo "========================================================================"
echo "Clinical Trials Data Preparation for Biobtree"
echo "========================================================================"
echo "Output directory: $OUTPUT_DIR"
echo "Downloads:        $DOWNLOAD_DIR"
echo "Extracted:        $EXTRACT_DIR"
echo "Biobtree JSON:    $BIOBTREE_DIR"
echo "State tracking:   $STATE_DIR"
echo "========================================================================"
echo ""

# Run the download and extract script
python3 "$SCRIPT_DIR/download_and_extract.py" \
    --download-dir "$DOWNLOAD_DIR" \
    --extract-dir "$EXTRACT_DIR" \
    --output-dir "$BIOBTREE_DIR" \
    --tracking-db "$STATE_DIR/tracking.db" \
    $LIMIT \
    $FULL \
    $VERBOSE

echo ""
echo "========================================================================"
echo "Clinical trials data preparation complete!"
echo "========================================================================"
echo "Biobtree JSON: $BIOBTREE_DIR/trials.json"
echo ""
echo "To build biobtree with clinical trials:"
echo "  biobtree -d clinical_trials build"
echo "========================================================================"
