#!/bin/bash

# Biobtree Build & Management Script (bb.sh)
# ==============================================
# Build datasets, generate databases, manage versions, and run services.
# Each dataset has its own log file for easy debugging.
# Long-running commands run in background - check logs for progress.
#
# Usage:
#   ./bb.sh                            # Update all datasets (default: out/)
#   ./bb.sh <output_dir>               # Update all datasets to specified directory
#   ./bb.sh --status                   # Show dataset status
#   ./bb.sh --check                    # Check for changes only
#   ./bb.sh --from pubchem             # Resume from specific dataset
#   ./bb.sh --only pubchem             # Run single dataset
#   ./bb.sh --generate                 # Run generate phase only (build database)
#   ./bb.sh --db-versions              # Show database versions
#   ./bb.sh --activate                 # Activate latest db version
#   ./bb.sh --activate 2               # Activate specific db version
#   ./bb.sh --cleanup                  # Remove old db versions (keep last 2)
#   ./bb.sh --web                      # Start web server
#   ./bb.sh --test                     # Run integration tests

set -e

# Re-run script in background if BUILD_IN_BG is not set
# This makes the script itself run in background with output to log file
# Check all args for flags that should run in foreground
RUN_FOREGROUND=false
for arg in "$@"; do
    case "$arg" in
        --status|--check|--help|-h|--web|--test|--dry-run|--db-versions|--activate|--cleanup) RUN_FOREGROUND=true ;;
    esac
done
if [[ -z "$BUILD_IN_BG" && "$RUN_FOREGROUND" == "false" ]]; then
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
    pdb
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
    signor
    collectri
    corum
    cellphonedb
    bgee
    cellxgene
    cellxgene_celltype
    scxa
    scxa_expression
    protein_similarity
    esm2_similarity

    # Compounds & drugs
    hmdb
    lipidmaps
    swisslipids
    drugcentral
    bindingdb
    pharmgkb

    # Enzymes & biochemistry
    brenda

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
    orphanet

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
    echo "  --only <datasets> Run specific dataset(s), comma-separated (e.g., uniprot,hgnc,go)"
    echo "  --generate        Run generate phase only (build database)"
    echo "  --federation <name>  With --generate: build specific federation (main, dbsnp)"
    echo "  --force           Force update even if unchanged"
    echo "  --maxcpu <N>      Max CPUs (default: 8)"
    echo "  --dry-run         Show what would be done"
    echo "  --status          Show dataset status from state file"
    echo "  --web             Start web server"
    echo "  --test            Run integration tests (requires server on localhost:9291)"
    echo "  --prod            With --test: test against production MCP server (localhost:8000)"
    echo "  --help            Show this help message"
    echo ""
    echo "Database Version Management:"
    echo "  --db-versions     Show database versions for all federations"
    echo "  --activate [N]    Activate db version N (default: latest) for all federations"
    echo "  --cleanup [N]     Remove old db versions, keeping last N (default: 2)"
    echo ""
    echo "Federations:"
    echo "  main              Default federation (most datasets)"
    echo "  dbsnp             dbSNP variants (separate large database)"
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
WEB_SERVER="false"
RUN_TESTS="false"
TEST_PROD="false"
FEDERATION=""
SHOW_DB_VERSIONS="false"
ACTIVATE_VERSION=""
DO_ACTIVATE="false"
DO_CLEANUP="false"
CLEANUP_KEEP=2

while [[ $# -gt 0 ]]; do
    case "$1" in
        --check)        CHECK_ONLY="true"; shift ;;
        --from)         FROM_DATASET=$2; shift 2 ;;
        --only)         ONLY_DATASET=$2; shift 2 ;;
        --generate)     GENERATE_ONLY="true"; shift ;;
        --federation)   FEDERATION=$2; shift 2 ;;
        --force)        FORCE="true"; shift ;;
        --maxcpu)       MAXCPU=$2; shift 2 ;;
        --dry-run)      DRY_RUN="true"; shift ;;
        --status)       SHOW_STATUS="true"; shift ;;
        --web)          WEB_SERVER="true"; shift ;;
        --test)         RUN_TESTS="true"; shift ;;
        --prod)         TEST_PROD="true"; shift ;;
        --db-versions)  SHOW_DB_VERSIONS="true"; shift ;;
        --activate)
            DO_ACTIVATE="true"
            # Check if next arg is a version number (not another option)
            if [[ -n "$2" && ! "$2" == --* ]]; then
                ACTIVATE_VERSION=$2
                shift 2
            else
                shift
            fi
            ;;
        --cleanup)
            DO_CLEANUP="true"
            # Check if next arg is a number
            if [[ -n "$2" && "$2" =~ ^[0-9]+$ ]]; then
                CLEANUP_KEEP=$2
                shift 2
            else
                shift
            fi
            ;;
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

# Get federation for a dataset (dbsnp -> dbsnp, others -> main)
get_federation() {
    local dataset=$1
    case "$dataset" in
        dbsnp) echo "dbsnp" ;;
        *) echo "main" ;;
    esac
}

# ============================================================================
# DATABASE VERSION MANAGEMENT
# ============================================================================

# Get all db versions for a federation (returns space-separated version numbers)
get_db_versions() {
    local federation=$1
    local fed_dir="$OUT_DIR/$federation"

    if [[ ! -d "$fed_dir" ]]; then
        echo ""
        return
    fi

    # Find db_v* directories and extract version numbers
    ls -1 "$fed_dir" 2>/dev/null | grep -E '^db_v[0-9]+$' | sed 's/db_v//' | sort -n | tr '\n' ' '
}

# Get current version (what db symlink points to)
get_current_version() {
    local federation=$1
    local symlink="$OUT_DIR/$federation/db"

    if [[ ! -L "$symlink" ]]; then
        # Not a symlink - might be old format directory
        if [[ -d "$symlink" ]]; then
            echo "legacy"
        else
            echo "none"
        fi
        return
    fi

    local target=$(readlink "$symlink")
    if [[ "$target" =~ ^db_v([0-9]+)$ ]]; then
        echo "${BASH_REMATCH[1]}"
    else
        echo "unknown"
    fi
}

# Get latest (highest) version number
get_latest_version() {
    local federation=$1
    local versions=$(get_db_versions "$federation")

    if [[ -z "$versions" ]]; then
        echo "0"
        return
    fi

    # Get last (highest) version
    echo "$versions" | tr ' ' '\n' | tail -1
}

# Activate a specific db version (update symlink)
activate_db_version() {
    local federation=$1
    local version=$2
    local fed_dir="$OUT_DIR/$federation"
    local symlink="$fed_dir/db"
    local target="db_v$version"
    local target_path="$fed_dir/$target"

    # Verify target exists
    if [[ ! -d "$target_path" ]]; then
        echo "ERROR: Version $version does not exist for federation '$federation'"
        echo "Available versions: $(get_db_versions "$federation")"
        return 1
    fi

    # Handle existing symlink or directory
    if [[ -L "$symlink" ]]; then
        rm "$symlink"
    elif [[ -d "$symlink" ]]; then
        # Legacy directory - rename to db_v0
        echo "Migrating legacy db directory to db_v0..."
        mv "$symlink" "$fed_dir/db_v0"
    fi

    # Create new symlink (relative path)
    ln -s "$target" "$symlink"
    echo "✓ Activated db_v$version for federation '$federation'"
}

# Cleanup old versions, keeping the last N
cleanup_old_versions() {
    local federation=$1
    local keep=${2:-2}
    local fed_dir="$OUT_DIR/$federation"
    local current=$(get_current_version "$federation")

    local versions=($(get_db_versions "$federation"))
    local count=${#versions[@]}

    if [[ $count -le $keep ]]; then
        echo "Only $count version(s) exist, nothing to cleanup"
        return
    fi

    local to_delete=$((count - keep))
    echo "Cleaning up $to_delete old version(s), keeping last $keep..."

    for ((i=0; i<to_delete; i++)); do
        local ver=${versions[$i]}
        if [[ "$ver" == "$current" ]]; then
            echo "  Skipping db_v$ver (currently active)"
            continue
        fi
        echo "  Removing db_v$ver..."
        rm -rf "$fed_dir/db_v$ver"
    done
}

# Show db versions for all federations
show_db_versions() {
    echo ""
    echo "============================================"
    echo "Database Versions"
    echo "============================================"
    echo "Output directory: $OUT_DIR"
    echo ""

    for federation in main dbsnp; do
        local fed_dir="$OUT_DIR/$federation"
        if [[ ! -d "$fed_dir" ]]; then
            continue
        fi

        local current=$(get_current_version "$federation")
        local latest=$(get_latest_version "$federation")
        local versions=$(get_db_versions "$federation")

        echo "Federation: $federation"
        echo "  Current: ${current:-none}"
        echo "  Latest:  ${latest:-none}"
        echo -n "  Available: "

        if [[ -z "$versions" ]]; then
            echo "none"
        else
            for v in $versions; do
                if [[ "$v" == "$current" ]]; then
                    echo -n "v$v* "
                else
                    echo -n "v$v "
                fi
            done
            echo ""
        fi

        # Show sizes
        for v in $versions; do
            local size=$(du -sh "$fed_dir/db_v$v" 2>/dev/null | cut -f1)
            echo "    db_v$v: ${size:-unknown}"
        done
        echo ""
    done
}

# Calculate actual KV size from index files for a dataset
# Counts: {dataset}_sorted.*.index.gz + {dataset}_from_*.index.gz
# Uses federation-aware paths: {OUT_DIR}/{federation}/index/
calc_kv_size() {
    local dataset=$1
    local federation=$(get_federation "$dataset")
    local index_dir="$OUT_DIR/$federation/index"

    if [[ ! -d "$index_dir" ]]; then
        echo "0"
        return
    fi

    # Count main file + from_* files (files that add data TO this dataset)
    local total=$(ls -la "$index_dir" 2>/dev/null | grep -E "^-.* ${dataset}_sorted\..*\.index\.gz$|^-.* ${dataset}_from_.*\.index\.gz$" | awk '{total += $5} END {print total+0}')
    echo "${total:-0}"
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
    # Sum KV size across all federation index directories
    local total_kv=0
    for fed_index in "$OUT_DIR"/*/index; do
        if [[ -d "$fed_index" ]]; then
            local fed_size=$(ls -la "$fed_index" 2>/dev/null | grep -E "\.index\.gz$" | awk '{total += $5} END {print total+0}')
            total_kv=$((total_kv + fed_size))
        fi
    done

    echo "Last Build: $build_time"
    echo "Version: $build_version"
    echo "Total KV Size: $(format_size $total_kv)"
    echo ""

    # Header
    printf "%-25s %-8s %-12s %-20s %-12s %-12s\n" "DATASET" "FED" "STATUS" "LAST BUILD" "KV SIZE" "DURATION"
    printf "%-25s %-8s %-12s %-20s %-12s %-12s\n" "-------------------------" "--------" "------------" "--------------------" "------------" "------------"

    # Collect all dataset info for sorting
    local temp_file=$(mktemp)
    for dataset in "${DATASETS[@]}"; do
        local status=$(jq -r ".datasets[\"$dataset\"].status // \"-\"" "$state_file")
        local last_build=$(jq -r ".datasets[\"$dataset\"].last_build_time // \"\"" "$state_file")
        local kv_size=$(calc_kv_size "$dataset")
        local duration=$(jq -r ".datasets[\"$dataset\"].build_duration_sec // 0" "$state_file")
        local federation=$(get_federation "$dataset")

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

        # Store with raw kv_size for sorting (tab-separated: raw_size, formatted_line)
        printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\n" "$kv_size" "$dataset" "$federation" "$status_display" "$last_build" "$kv_display" "$duration_display" >> "$temp_file"
    done

    # Sort by kv_size (first field) descending and print formatted output
    sort -t$'\t' -k1 -n -r "$temp_file" | while IFS=$'\t' read -r raw_size dataset federation status_display last_build kv_display duration_display; do
        printf "%-25s %-8s %-12s %-20s %-12s %-12s\n" "$dataset" "$federation" "$status_display" "$last_build" "$kv_display" "$duration_display"
    done
    rm -f "$temp_file"

    echo ""
    echo "============================================"

    # Summary counts - count from DATASETS array, not state file
    local merged_count=0
    local processing_count=0
    local not_built_count=0
    for dataset in "${DATASETS[@]}"; do
        local status=$(jq -r ".datasets[\"$dataset\"].status // \"\"" "$state_file")
        if [[ "$status" == "merged" ]]; then
            ((merged_count++)) || true
        elif [[ "$status" == "processing" ]]; then
            ((processing_count++)) || true
        else
            ((not_built_count++)) || true
        fi
    done

    echo "Summary: $merged_count merged, $processing_count processing, $not_built_count not built"
    echo ""
}

run_tests() {
    local server_url="${1:-http://localhost:9291}"
    local use_mcp="${2:-false}"
    local test_dir="tests/xintegration"
    local test_script="${test_dir}/run_integration_tests.py"

    if [[ ! -f "$test_script" ]]; then
        echo "ERROR: Test script not found: $test_script"
        exit 1
    fi

    # Check if python3 is available
    if ! command -v python3 &> /dev/null; then
        echo "ERROR: python3 is required for integration tests"
        exit 1
    fi

    echo ""
    echo "============================================"
    echo "Running Integration Tests"
    echo "============================================"
    echo "Server: $server_url"
    echo "Mode: $([ "$use_mcp" == "true" ] && echo "MCP API" || echo "Biobtree direct")"
    echo "Test script: $test_script"
    echo ""

    # Run the integration tests
    local extra_args=""
    if [[ "$use_mcp" == "true" ]]; then
        extra_args="--mcp"
    fi

    python3 "$test_script" --server "$server_url" --no-report $extra_args
    local exit_code=$?

    if [[ $exit_code -eq 0 ]]; then
        echo ""
        echo "✓ All integration tests passed"
    else
        echo ""
        echo "✗ Some integration tests failed"
    fi

    return $exit_code
}

# ============================================================================
# MAIN
# ============================================================================

echo "============================================"
echo "BioBTree Build & Management Script"
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

# DB versions mode
if [[ "$SHOW_DB_VERSIONS" == "true" ]]; then
    show_db_versions
    exit 0
fi

# Activate version mode
if [[ "$DO_ACTIVATE" == "true" ]]; then
    echo ""
    echo "============================================"
    echo "Activating Database Version"
    echo "============================================"

    for federation in main dbsnp; do
        fed_dir="$OUT_DIR/$federation"
        if [[ ! -d "$fed_dir" ]]; then
            continue
        fi

        version="$ACTIVATE_VERSION"
        if [[ -z "$version" ]]; then
            version=$(get_latest_version "$federation")
        fi

        if [[ "$version" == "0" || -z "$version" ]]; then
            echo "No versions found for federation '$federation'"
            continue
        fi

        current=$(get_current_version "$federation")
        if [[ "$current" == "$version" ]]; then
            echo "Federation '$federation': already on db_v$version"
        else
            activate_db_version "$federation" "$version"
        fi
    done

    echo ""
    echo "Note: Restart the web service to use the new version"
    exit 0
fi

# Cleanup mode
if [[ "$DO_CLEANUP" == "true" ]]; then
    echo ""
    echo "============================================"
    echo "Cleaning Up Old Database Versions"
    echo "============================================"
    echo "Keeping last $CLEANUP_KEEP version(s)"
    echo ""

    for federation in main dbsnp; do
        fed_dir="$OUT_DIR/$federation"
        if [[ ! -d "$fed_dir" ]]; then
            continue
        fi

        echo "Federation: $federation"
        cleanup_old_versions "$federation" "$CLEANUP_KEEP"
        echo ""
    done
    exit 0
fi

# Test mode
if [[ "$RUN_TESTS" == "true" ]]; then
    if [[ "$TEST_PROD" == "true" ]]; then
        run_tests "http://localhost:8000" "true"
    else
        run_tests "http://localhost:9291" "false"
    fi
    exit $?
fi

# Web server mode
if [[ "$WEB_SERVER" == "true" ]]; then
    if [[ "$OUT_DIR" == "out" ]]; then
        # Default directory - run in foreground
        echo "Starting web server (foreground)..."
        exec ./biobtree web
    else
        # Custom directory - run in background with --prod
        echo "Starting web server in background..."
        echo "Log: ${LOG_DIR}/web.log"
        nohup ./biobtree --out-dir "$OUT_DIR" --prod web > "${LOG_DIR}/web.log" 2>&1 &
        echo "PID: $!"
        echo "Monitor: tail -f ${LOG_DIR}/web.log"
    fi
    exit 0
fi

# Selected datasets mode (supports comma-separated list)
if [[ -n "$ONLY_DATASET" ]]; then
    # Split by comma into array
    IFS=',' read -ra SELECTED_DATASETS <<< "$ONLY_DATASET"

    FAILED_SELECTED=()
    COMPLETED_SELECTED=0
    TOTAL_SELECTED=${#SELECTED_DATASETS[@]}

    for dataset in "${SELECTED_DATASETS[@]}"; do
        # Trim whitespace
        dataset=$(echo "$dataset" | xargs)
        ((COMPLETED_SELECTED++)) || true

        echo ""
        echo "[$COMPLETED_SELECTED/$TOTAL_SELECTED] Processing: $dataset"

        if ! run_dataset "$dataset"; then
            FAILED_SELECTED+=("$dataset")
            echo "WARNING: $dataset failed, continuing..."
        fi
    done

    echo ""
    echo "============================================"
    if [[ ${#FAILED_SELECTED[@]} -eq 0 ]]; then
        echo "✓ All $TOTAL_SELECTED dataset(s) completed successfully"
    else
        echo "Completed: $((TOTAL_SELECTED - ${#FAILED_SELECTED[@]}))/$TOTAL_SELECTED"
        echo "Failed: ${FAILED_SELECTED[*]}"
        exit 1
    fi
    exit 0
fi

# Generate only mode
if [[ "$GENERATE_ONLY" == "true" ]]; then
    if [[ -n "$FEDERATION" ]]; then
        log_header "Generate ($FEDERATION federation)"
        echo "Building $FEDERATION federation database..."
        echo "Log: ${LOG_DIR}/generate_${FEDERATION}.log"

        if [[ "$DRY_RUN" == "true" ]]; then
            echo "[DRY RUN] ./biobtree --out-dir \"$OUT_DIR\" --federation \"$FEDERATION\" --lmdb-safety-factor 4.5 generate"
        elif ./biobtree --out-dir "$OUT_DIR" --federation "$FEDERATION" --lmdb-safety-factor 4.5 generate > "${LOG_DIR}/generate_${FEDERATION}.log" 2>&1; then
            echo "✓ Generate complete ($FEDERATION federation)"

            # Show version info
            latest=$(get_latest_version "$FEDERATION")
            current=$(get_current_version "$FEDERATION")
            echo ""
            echo "New version created: db_v$latest"
            echo "Current active:      db_v$current"
            if [[ "$latest" != "$current" ]]; then
                echo ""
                echo "To activate the new version:"
                echo "  $0 $OUT_DIR --activate"
            fi
        else
            echo "✗ Generate FAILED ($FEDERATION) - see ${LOG_DIR}/generate_${FEDERATION}.log"
            exit 1
        fi
    else
        log_header "Generate (all federations)"
        echo "Building all federation databases..."
        echo "Log: ${LOG_DIR}/generate.log"

        if [[ "$DRY_RUN" == "true" ]]; then
            echo "[DRY RUN] ./biobtree --out-dir \"$OUT_DIR\" --lmdb-safety-factor 4.5 generate"
        elif ./biobtree --out-dir "$OUT_DIR" --lmdb-safety-factor 4.5 generate > "${LOG_DIR}/generate.log" 2>&1; then
            echo "✓ Generate complete (all federations)"

            # Show version info for each federation
            echo ""
            echo "Version Summary:"
            for federation in main dbsnp; do
                fed_dir="$OUT_DIR/$federation"
                if [[ ! -d "$fed_dir" ]]; then
                    continue
                fi
                latest=$(get_latest_version "$federation")
                current=$(get_current_version "$federation")
                if [[ "$latest" != "0" ]]; then
                    echo "  $federation: created db_v$latest (active: db_v$current)"
                fi
            done

            echo ""
            echo "To activate the new version(s):"
            echo "  $0 $OUT_DIR --activate"
        else
            echo "✗ Generate FAILED - see ${LOG_DIR}/generate.log"
            exit 1
        fi
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
