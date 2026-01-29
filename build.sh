#!/bin/bash

# Biobtree Build Script - Sequential One-by-One
# ==============================================
# Runs each dataset individually for maximum reliability.
# Each dataset has its own log file for easy debugging.
# All commands run in background - check logs for progress.
#
# Usage:
#   ./build.sh                         # Update all datasets (default: out/)
#   ./build.sh <output_dir>            # Update all datasets to specified directory
#   ./build.sh --status                # Show dataset status
#   ./build.sh --check                 # Check for changes only
#   ./build.sh --from pubchem          # Resume from specific dataset
#   ./build.sh --only pubchem          # Run single dataset
#   ./build.sh --generate              # Run generate phase only (build database)

set -e

# Re-run script in background if BUILD_IN_BG is not set
# This makes the script itself run in background with output to log file
if [[ -z "$BUILD_IN_BG" && "$1" != "--status" && "$1" != "--check" && "$1" != "--help" && "$1" != "-h" ]]; then
    mkdir -p logs
    LOG_FILE="logs/build_$(date +%Y%m%d_%H%M%S).log"
    echo "Running in background. Log: $LOG_FILE"
    echo "Monitor: tail -f $LOG_FILE"
    BUILD_IN_BG=1 nohup "$0" "$@" > "$LOG_FILE" 2>&1 &
    echo "PID: $!"
    exit 0
fi

# ============================================================================
# CONFIGURATION - Edit these settings as needed
# ============================================================================

# Taxonomy IDs for model organisms (AlphaFold's 16 model organisms)
TAXIDS="9606,10090,10116,7955,7227,6239,559292,284812,511145,3702,39947,4577,3847,44689,237561,243232"

# Default settings
MAXCPU=8
LOG_DIR="logs"

# Global options applied to ALL datasets
GLOBAL_OPTS="--include-optionals --lookupdb"

# ----------------------------------------------------------------------------
# DATASET-SPECIFIC OPTIONS
# Format: OPTS_<dataset>="extra options"
# These are added to GLOBAL_OPTS for specific datasets
# ----------------------------------------------------------------------------
OPTS_ensembl="--eo --tax ${TAXIDS}"
OPTS_string="--tax ${TAXIDS}"
OPTS_entrez="--genome-taxids ${TAXIDS}"
OPTS_refseq="--genome-taxids ${TAXIDS}"
OPTS_pubchem="--pubchem-sdf-workers 1"
OPTS_patent="--bucket-sort-workers 8"
OPTS_bgee="--bucket-sort-workers 8"

# ----------------------------------------------------------------------------
# DATASETS LIST - Order matters (foundations first)
# Comment out datasets you don't want to run
# ----------------------------------------------------------------------------
DATASETS=(
    # Foundation
    taxonomy
    hgnc
    ensembl

    # Core biological
    uniprot
    interpro
    chebi

    # ChEMBL (expanded - was group "chembl")
    chembl_document
    chembl_assay
    chembl_activity
    chembl_molecule
    chembl_target
    chembl_target_component
    chembl_cell_line

    # Structure & function
    alphafold
    rnacentral
    reactome
    rhea

    # Variants & disease
    clinvar
    gwas_study
    gwas
    dbsnp
    alphamissense
    alphamissense_transcript

    # Interactions & pathways
    intact
    biogrid
    string
    bgee
    cellxgene
    cellxgene_celltype
    scxa
    scxa_expression
    protein_similarity

    # Compounds & drugs
    hmdb
    lipidmaps
    swisslipids
    drugcentral
    bindingdb
    pharmgkb

    # PubChem (large - run one at a time)
    pubchem
    pubchem_activity
    pubchem_assay

    # Clinical & medical
    clinical_trials
    antibody
    gencc
    ctd
    msigdb

    # Patents
    patent

    # Ontologies (expanded - was group "ontology")
    go
    eco
    efo
    uberon
    cl
    mondo
    hpo
    oba
    pato
    obi
    xco
    bao

    # Vocabularies
    mesh

    # NCBI (run last - depends on others for xrefs)
    entrez
    refseq
)

# ============================================================================
# ARGUMENT PARSING
# ============================================================================

# Default output directory
OUT_DIR="out"

show_help() {
    echo "Usage: $0 [output_dir] [OPTIONS]"
    echo ""
    echo "Arguments:"
    echo "  output_dir        Output directory (default: out/)"
    echo ""
    echo "Options:"
    echo "  --check           Check for source changes without updating"
    echo "  --from <dataset>  Resume from specific dataset"
    echo "  --only <dataset>  Run only specific dataset"
    echo "  --generate        Run generate phase only (build database)"
    echo "  --force           Force update even if unchanged"
    echo "  --maxcpu <N>      Max CPUs (default: 8)"
    echo "  --dry-run         Show what would be done"
    echo "  --status          Show dataset status from state file"
    echo "  --help            Show this help message"
    echo ""
    echo "Available datasets:"
    echo "  ${DATASETS[*]}" | fold -s -w 70
    exit 0
}

# Check if first argument is a directory (not an option)
if [[ -n $1 && ! $1 == --* ]]; then
    OUT_DIR=$1
    shift
fi

# Options
CHECK_ONLY="false"
FROM_DATASET=""
ONLY_DATASET=""
GENERATE_ONLY="false"
FORCE="false"
DRY_RUN="false"
SHOW_STATUS="false"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --check)        CHECK_ONLY="true"; shift ;;
        --from)         FROM_DATASET=$2; shift 2 ;;
        --only)         ONLY_DATASET=$2; shift 2 ;;
        --generate)     GENERATE_ONLY="true"; shift ;;
        --force)        FORCE="true"; shift ;;
        --maxcpu)       MAXCPU=$2; shift 2 ;;
        --dry-run)      DRY_RUN="true"; shift ;;
        --status)       SHOW_STATUS="true"; shift ;;
        --help|-h)      show_help ;;
        *)              echo "Unknown option: $1"; show_help ;;
    esac
done

# ============================================================================
# HELPER FUNCTIONS
# ============================================================================

timestamp() {
    date "+%Y-%m-%d %H:%M:%S"
}

log_header() {
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  $(timestamp) | $1"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

get_dataset_options() {
    local dataset=$1
    local opts="${GLOBAL_OPTS} --maxcpu ${MAXCPU}"

    # Get dataset-specific options from OPTS_<dataset> variable
    local var_name="OPTS_${dataset}"
    local extra_opts="${!var_name}"
    if [[ -n "$extra_opts" ]]; then
        opts="$opts $extra_opts"
    fi

    if [[ "$FORCE" == "true" ]]; then
        opts="$opts --force"
    fi

    echo "$opts"
}

run_dataset() {
    local dataset=$1
    local log_file="${LOG_DIR}/${dataset}.log"
    local opts=$(get_dataset_options "$dataset")

    log_header "$dataset"
    echo "Log: $log_file"

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "[DRY RUN] ./biobtree --out-dir \"$OUT_DIR\" $opts -d \"$dataset\" update"
        return 0
    fi

    # Run update (script itself is already in background)
    if ./biobtree --out-dir "$OUT_DIR" $opts -d "$dataset" update > "$log_file" 2>&1; then
        echo "✓ $dataset completed successfully"
        return 0
    else
        echo "✗ $dataset FAILED - see $log_file"
        echo ""
        echo "Last 20 lines of log:"
        tail -20 "$log_file"
        return 1
    fi
}

check_dataset() {
    local dataset=$1
    local opts=$(get_dataset_options "$dataset")

    echo -n "  $dataset: "

    if ./biobtree --out-dir "$OUT_DIR" $opts -d "$dataset" check 2>/dev/null | grep -q "changed"; then
        echo "CHANGED"
    else
        echo "unchanged"
    fi
}

format_duration() {
    local secs=$1
    # Handle decimal seconds
    local int_secs=$(printf "%.0f" "$secs")
    local hours=$((int_secs / 3600))
    local mins=$(((int_secs % 3600) / 60))
    local remaining=$((int_secs % 60))

    if [[ $hours -gt 0 ]]; then
        printf "%dh %dm %ds" $hours $mins $remaining
    elif [[ $mins -gt 0 ]]; then
        printf "%dm %ds" $mins $remaining
    else
        printf "%ds" $remaining
    fi
}

format_size() {
    local bytes=$1
    if [[ -z "$bytes" || "$bytes" == "null" || "$bytes" == "0" ]]; then
        echo "-"
        return
    fi

    local gb=$(echo "scale=2; $bytes / 1073741824" | bc)
    local mb=$(echo "scale=2; $bytes / 1048576" | bc)
    local kb=$(echo "scale=2; $bytes / 1024" | bc)

    if (( $(echo "$gb >= 1" | bc -l) )); then
        printf "%.1f GB" "$gb"
    elif (( $(echo "$mb >= 1" | bc -l) )); then
        printf "%.1f MB" "$mb"
    else
        printf "%.1f KB" "$kb"
    fi
}

show_status() {
    local state_file="$OUT_DIR/dataset_state.json"

    if [[ ! -f "$state_file" ]]; then
        echo "ERROR: State file not found: $state_file"
        echo "Run an update first to generate the state file."
        exit 1
    fi

    # Check if jq is available
    if ! command -v jq &> /dev/null; then
        echo "ERROR: jq is required for --status option"
        echo "Install with: apt install jq"
        exit 1
    fi

    echo ""
    echo "============================================"
    echo "Dataset Status Report"
    echo "============================================"
    echo "State file: $state_file"
    echo ""

    # Global info
    local build_time=$(jq -r '.last_build_time // "N/A"' "$state_file" | cut -d'T' -f1,2 | tr 'T' ' ' | cut -d'.' -f1)
    local build_version=$(jq -r '.build_version // "N/A"' "$state_file")
    local total_kv=$(jq -r '.total_kv_size // 0' "$state_file")

    echo "Last Build: $build_time"
    echo "Version: $build_version"
    echo "Total KV Size: $(format_size $total_kv)"
    echo ""

    # Header
    printf "%-25s %-12s %-20s %-12s %-12s\n" "DATASET" "STATUS" "LAST BUILD" "KV SIZE" "DURATION"
    printf "%-25s %-12s %-20s %-12s %-12s\n" "-------------------------" "------------" "--------------------" "------------" "------------"

    # Loop through datasets in the DATASETS array
    for dataset in "${DATASETS[@]}"; do
        local status=$(jq -r ".datasets[\"$dataset\"].status // \"-\"" "$state_file")
        local last_build=$(jq -r ".datasets[\"$dataset\"].last_build_time // \"\"" "$state_file")
        local kv_size=$(jq -r ".datasets[\"$dataset\"].kv_size // 0" "$state_file")
        local duration=$(jq -r ".datasets[\"$dataset\"].build_duration_sec // 0" "$state_file")

        # Format build time (extract date and time)
        if [[ "$last_build" == "" || "$last_build" == "0001-01-01T00:00:00Z" ]]; then
            last_build="-"
        else
            last_build=$(echo "$last_build" | cut -d'T' -f1,2 | tr 'T' ' ' | cut -d'.' -f1 | cut -d'+' -f1)
        fi

        # Format status with color codes
        local status_display="$status"
        if [[ "$status" == "merged" ]]; then
            status_display="✓ merged"
        elif [[ "$status" == "processing" ]]; then
            status_display="⏳ processing"
        elif [[ "$status" == "-" || "$status" == "" ]]; then
            status_display="- not built"
        fi

        # Format sizes and duration
        local kv_display=$(format_size "$kv_size")
        local duration_display="-"
        if [[ "$duration" != "0" && "$duration" != "null" ]]; then
            duration_display=$(format_duration "$duration")
        fi

        printf "%-25s %-12s %-20s %-12s %-12s\n" "$dataset" "$status_display" "$last_build" "$kv_display" "$duration_display"
    done

    echo ""
    echo "============================================"

    # Summary counts
    local merged_count=$(jq '[.datasets[] | select(.status == "merged")] | length' "$state_file")
    local processing_count=$(jq '[.datasets[] | select(.status == "processing")] | length' "$state_file")
    local not_built_count=$(jq '[.datasets[] | select(.status == "" or .status == null)] | length' "$state_file")

    echo "Summary: $merged_count merged, $processing_count processing, $not_built_count not built"
    echo ""
}

# ============================================================================
# MAIN
# ============================================================================

echo "============================================"
echo "BiobTree Build Script - Sequential One-by-One"
echo "============================================"
echo "Output: $OUT_DIR"
echo "CPUs: $MAXCPU"
echo "Mode: $([ "$CHECK_ONLY" == "true" ] && echo "CHECK ONLY" || echo "UPDATE")"
echo ""

# Create directories
mkdir -p "$OUT_DIR" "$LOG_DIR"

# Check mode
if [[ "$CHECK_ONLY" == "true" ]]; then
    echo "Checking for source changes..."
    echo ""
    for dataset in "${DATASETS[@]}"; do
        check_dataset "$dataset"
    done
    exit 0
fi

# Status mode
if [[ "$SHOW_STATUS" == "true" ]]; then
    show_status
    exit 0
fi

# Single dataset mode
if [[ -n "$ONLY_DATASET" ]]; then
    if run_dataset "$ONLY_DATASET"; then
        echo ""
        echo "✓ Single dataset update complete: $ONLY_DATASET"
    else
        echo ""
        echo "✗ Failed: $ONLY_DATASET"
        exit 1
    fi

    exit 0
fi

# Generate only mode
if [[ "$GENERATE_ONLY" == "true" ]]; then
    log_header "Generate"
    echo "Merging all data into final database..."
    echo "Log: ${LOG_DIR}/generate.log"

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "[DRY RUN] ./biobtree --out-dir \"$OUT_DIR\" --lmdb-safety-factor 4.5 generate"
    elif ./biobtree --out-dir "$OUT_DIR" --lmdb-safety-factor 4.5 generate > "${LOG_DIR}/generate.log" 2>&1; then
        echo "✓ Generate complete"
    else
        echo "✗ Generate FAILED - see ${LOG_DIR}/generate.log"
        exit 1
    fi
    exit 0
fi

# Full update mode
START_PROCESSING="false"
if [[ -z "$FROM_DATASET" ]]; then
    START_PROCESSING="true"
fi

FAILED_DATASETS=()
COMPLETED=0
TOTAL=${#DATASETS[@]}

for dataset in "${DATASETS[@]}"; do
    # Handle --from option
    if [[ "$START_PROCESSING" != "true" ]]; then
        if [[ "$dataset" == "$FROM_DATASET" ]]; then
            START_PROCESSING="true"
        else
            echo "Skipping $dataset (before --from $FROM_DATASET)"
            continue
        fi
    fi

    ((COMPLETED++)) || true
    echo ""
    echo "[$COMPLETED/$TOTAL] Processing: $dataset"

    if ! run_dataset "$dataset"; then
        FAILED_DATASETS+=("$dataset")
        echo ""
        echo "WARNING: $dataset failed, continuing with next dataset..."
        echo "         Re-run with: $0 $OUT_DIR --only $dataset"
        echo ""
    fi
done

# Summary
echo ""
echo "============================================"
echo "Update Summary"
echo "============================================"
echo "Total datasets: $TOTAL"
echo "Completed: $((TOTAL - ${#FAILED_DATASETS[@]}))"
echo "Failed: ${#FAILED_DATASETS[@]}"

if [[ ${#FAILED_DATASETS[@]} -gt 0 ]]; then
    echo ""
    echo "Failed datasets:"
    for d in "${FAILED_DATASETS[@]}"; do
        echo "  - $d (log: ${LOG_DIR}/${d}.log)"
    done
    echo ""
    echo "Re-run failed datasets individually:"
    for d in "${FAILED_DATASETS[@]}"; do
        echo "  $0 $OUT_DIR --only $d"
    done
fi

# Reminder about generate
if [[ ${#FAILED_DATASETS[@]} -eq 0 ]]; then
    echo ""
    echo "All datasets updated. Run generate to build database:"
    echo "  $0 $OUT_DIR --generate"
fi

echo ""
echo "============================================"
echo "Complete!"
echo "============================================"
echo "Output: $OUT_DIR"
echo "Logs: $LOG_DIR/"
echo ""
echo "Start web service:"
echo "  ./biobtree --out-dir \"$OUT_DIR\" web"
echo ""
