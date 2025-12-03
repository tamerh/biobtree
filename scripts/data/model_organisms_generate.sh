#!/bin/bash

# This script runs the GENERATE phase for model organisms subset of biobtree.
# Run this AFTER the UPDATE phase completes successfully.
#
# GENERATE phase: RAM-intensive (merges all index files into final database)
# This phase is single-threaded but requires high RAM (64GB+ recommended)

set -e

# Parse arguments
if [[ -z $1 ]]; then
    echo "out dir parameter is required"
    echo "Usage: $0 <output_dir>"
    echo "Example: $0 /localscratch/\$USER/biobtree_model_organisms"
    exit 1
fi

OUT_DIR=$1

echo "============================================"
echo "Biobtree Model Organisms - GENERATE Phase"
echo "============================================"
echo "Output directory: $OUT_DIR"
echo ""

# Create logs directory
mkdir -p logs

echo "Starting validation before GENERATE phase..."
echo ""

# Check for index files in subdirectories (all 6 required for complete DB)
MISSING_FILES=()

if [[ ! -f ${OUT_DIR}/core_part1/index/core_part1.meta.json ]]; then
    MISSING_FILES+=("core_part1")
fi

if [[ ! -f ${OUT_DIR}/core_part2/index/core_part2.meta.json ]]; then
    MISSING_FILES+=("core_part2")
fi

if [[ ! -f ${OUT_DIR}/core_part3/index/core_part3.meta.json ]]; then
    MISSING_FILES+=("core_part3")
fi

if [[ ! -f ${OUT_DIR}/core_part4/index/core_part4.meta.json ]]; then
    MISSING_FILES+=("core_part4")
fi

if [[ ! -f ${OUT_DIR}/core_part5/index/core_part5.meta.json ]]; then
    MISSING_FILES+=("core_part5")
fi

if [[ ! -f ${OUT_DIR}/ensembl_model/index/ensembl_model.meta.json ]]; then
    MISSING_FILES+=("ensembl_model")
fi

if [[ ${#MISSING_FILES[@]} -gt 0 ]]; then
    echo "  ✗ ERROR: Not all index files found"
    echo ""
    echo "Missing index files for:"
    for job in "${MISSING_FILES[@]}"; do
        echo "  - ${OUT_DIR}/${job}/index/${job}.meta.json"
    done
    echo ""
    echo "Required files for complete database:"
    echo "  - ${OUT_DIR}/core_part1/index/core_part1.meta.json"
    echo "  - ${OUT_DIR}/core_part2/index/core_part2.meta.json"
    echo "  - ${OUT_DIR}/core_part3/index/core_part3.meta.json"
    echo "  - ${OUT_DIR}/core_part4/index/core_part4.meta.json"
    echo "  - ${OUT_DIR}/core_part5/index/core_part5.meta.json"
    echo "  - ${OUT_DIR}/ensembl_model/index/ensembl_model.meta.json"
    echo ""
    echo "Make sure UPDATE phase completed successfully for all 6 jobs."
    echo "Run UPDATE jobs with:"
    echo "  ./scripts/data/model_organisms_update.sh ${OUT_DIR}"
    echo "Or individually:"
    echo "  ./scripts/data/model_organisms_update.sh ${OUT_DIR} --core1-only"
    echo "  ./scripts/data/model_organisms_update.sh ${OUT_DIR} --core2-only"
    echo "  ./scripts/data/model_organisms_update.sh ${OUT_DIR} --core3-only"
    echo "  ./scripts/data/model_organisms_update.sh ${OUT_DIR} --core4-only"
    echo "  ./scripts/data/model_organisms_update.sh ${OUT_DIR} --core5-only"
    echo "  ./scripts/data/model_organisms_update.sh ${OUT_DIR} --ensembl-only"
    exit 1
fi

echo "  ✓ All index files found (core_part1, core_part2, core_part3, core_part4, core_part5, ensembl_model)"
echo ""

# Consolidate index files from subdirectories for generate phase
echo "Consolidating index files..."
mkdir -p ${OUT_DIR}/index

# Move all six index directories
# NOTE: Each part has bucket directories (0/, 1/, 2/, ...) under index/ that we don't need
#       for generate phase. We only need the *.gz files. Delete bucket dirs first to avoid conflicts.

echo "  - Moving core_part1 index..."
# Delete bucket directories (numeric dirs), keep only .gz files
find ${OUT_DIR}/core_part1/index -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} +
mv ${OUT_DIR}/core_part1/index/* ${OUT_DIR}/index/
rm -rf ${OUT_DIR}/core_part1

echo "  - Moving core_part2 index..."
find ${OUT_DIR}/core_part2/index -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} +
mv ${OUT_DIR}/core_part2/index/* ${OUT_DIR}/index/
rm -rf ${OUT_DIR}/core_part2

echo "  - Moving core_part3 index..."
find ${OUT_DIR}/core_part3/index -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} +
mv ${OUT_DIR}/core_part3/index/* ${OUT_DIR}/index/
rm -rf ${OUT_DIR}/core_part3

echo "  - Moving core_part4 index..."
find ${OUT_DIR}/core_part4/index -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} +
mv ${OUT_DIR}/core_part4/index/* ${OUT_DIR}/index/
rm -rf ${OUT_DIR}/core_part4

echo "  - Moving core_part5 index..."
find ${OUT_DIR}/core_part5/index -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} +
mv ${OUT_DIR}/core_part5/index/* ${OUT_DIR}/index/
rm -rf ${OUT_DIR}/core_part5

echo "  - Moving ensembl_model index..."
find ${OUT_DIR}/ensembl_model/index -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} +
mv ${OUT_DIR}/ensembl_model/index/* ${OUT_DIR}/index/
rm -rf ${OUT_DIR}/ensembl_model

echo "  ✓ Index files consolidated to ${OUT_DIR}/index/"
echo ""

# GENERATE phase: RAM-intensive, less CPU-intensive
# This phase merges all the index files into the final biobtree database
echo "Starting GENERATE phase..."
echo ""
echo "Resource requirements:"
echo "  - CPU: Not intensive (single-threaded)"
echo "  - RAM: High (64GB+ recommended)"
echo "  - Runtime: Several hours"
echo ""

# Run GENERATE phase locally with nohup (not CPU-intensive, runs on local machine)
echo "Running GENERATE phase locally (in background with nohup)..."
nohup ./biobtree --keep --out-dir ${OUT_DIR} generate > logs/generate_model.log 2>&1 &
GENERATE_PID=$!

echo "  ✓ Started: GENERATE phase (PID: ${GENERATE_PID})"
echo ""
echo "============================================"
echo "GENERATE phase started locally!"
echo "============================================"
echo ""
echo "Monitor the generate process with:"
echo "  tail -f logs/generate_model.log"
echo "  ps aux | grep ${GENERATE_PID}"
echo ""
echo "Wait for GENERATE to complete:"
echo "  wait ${GENERATE_PID}"
echo "  # Or check log file for completion message"
echo ""
echo "After GENERATE completes, you can:"
echo "  1. Create a backup: tar -czf biobtree_model_organisms_db.tar.gz ${OUT_DIR}/db"
echo "  2. Delete index files to save space: rm -rf ${OUT_DIR}/index"
echo "  3. Start web services: nohup ./biobtree --out-dir ${OUT_DIR} web > logs/web_model.log 2>&1 &"
echo ""
echo "============================================"
echo "GENERATE phase initiated successfully!"
echo "============================================"
