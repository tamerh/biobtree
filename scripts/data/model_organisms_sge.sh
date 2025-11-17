#!/bin/bash

# This script processes model organisms subset of biobtree using SGE cluster scheduler.
# Based on AlphaFold's 16 model organisms: https://alphafold.ebi.ac.uk/download#proteomes-section
#
# The approach includes:
# - Ensembl genomes for each model organism (via --tax flag)
# - STRING protein interactions for each model organism (via --tax flag)
# - Core datasets (uniprot, go, hgnc, taxonomy, etc.)
#
# Two-phase processing:
# 1. UPDATE phase: CPU-intensive (downloads and processes data)
# 2. GENERATE phase: RAM-intensive (merges all data into final database)
#
# Can run both phases automatically or run them separately for validation.

set -e

source ./scripts/data/common_sge.sh

# Hardcoded queue name
QUEUE="scc"

# Parse arguments
if [[ -z $1 ]]; then
    echo "out dir parameter is required"
    echo "Usage: $0 <output_dir> [OPTIONS]"
    echo "Example: $0 /localscratch/\$USER/biobtree_model_organisms"
    echo "Example: $0 /localscratch/\$USER/biobtree_model_organisms --core1-only"
    echo "Example: $0 /localscratch/\$USER/biobtree_model_organisms --generate-only"
    echo ""
    echo "Options:"
    echo "  (default)         Submit all UPDATE phase jobs (core1 + core2 + ensembl)"
    echo "  --core1-only      Submit only core part 1 job"
    echo "  --core2-only      Submit only core part 2 job"
    echo "  --ensembl-only    Submit only Ensembl job"
    echo "  --generate-only   Run only GENERATE phase (assumes UPDATE already completed)"
    exit 1
fi

OUT_DIR=$1
PHASE_MODE="update"     # Default: run UPDATE phase
RUN_CORE1="true"        # Default: run core1
RUN_CORE2="true"        # Default: run core2
RUN_ENSEMBL="true"      # Default: run ensembl

# Check for mode flag
if [[ ! -z $2 ]]; then
    case "$2" in
        --generate-only)
            PHASE_MODE="generate"
            ;;
        --core1-only)
            RUN_CORE1="true"
            RUN_CORE2="false"
            RUN_ENSEMBL="false"
            ;;
        --core2-only)
            RUN_CORE1="false"
            RUN_CORE2="true"
            RUN_ENSEMBL="false"
            ;;
        --ensembl-only)
            RUN_CORE1="false"
            RUN_CORE2="false"
            RUN_ENSEMBL="true"
            ;;
        *)
            echo "Unknown option: $2"
            echo "Valid options: --core1-only, --core2-only, --ensembl-only, --generate-only"
            exit 1
            ;;
    esac
fi

echo "============================================"
echo "Biobtree Model Organisms Processing"
echo "============================================"
echo "Output directory: $OUT_DIR"
echo "Queue: $QUEUE"
echo "Phase mode: $PHASE_MODE"
echo "Jobs to run: Core1=$RUN_CORE1, Core2=$RUN_CORE2, Ensembl=$RUN_ENSEMBL"
echo ""
echo "Model organisms (16 organisms from AlphaFold):"
echo "  Organism                                    Ensembl ID    STRING ID"
echo "  ──────────────────────────────────────────  ────────────  ──────────"
echo "  Homo sapiens                                9606          9606"
echo "  Mus musculus                                10090         10090"
echo "  Rattus norvegicus                           10116         10116"
echo "  Danio rerio                                 7955          7955"
echo "  Drosophila melanogaster                     7227          7227"
echo "  Caenorhabditis elegans                      6239          6239"
echo "  Saccharomyces cerevisiae S288C              559292        4932 ⚠️"
echo "  Schizosaccharomyces pombe 972h-             284812        284812"
echo "  Escherichia coli K-12 MG1655                511145        511145"
echo "  Arabidopsis thaliana                        3702          3702"
echo "  Oryza sativa                                39947         39947"
echo "  Zea mays                                    4577          4577"
echo "  Glycine max                                 3847          3847"
echo "  Dictyostelium discoideum                    44689         44689"
echo "  Candida albicans                            237561        237561"
echo "  Methanocaldococcus jannaschii               243232        243232"
echo ""
echo "⚠️  Note: STRING uses species-level ID for S. cerevisiae (4932)"
echo "    Ensembl uses strain-specific ID (559292 for S288C)"
echo ""
echo "Datasets to process:"
echo "  - Core part 1: uniprot, go, eco, hgnc, taxonomy, interpro, hmdb, chembl"
echo "  - Core part 2: efo, mondo, hpo, alphafold, rnacentral, reactome,"
echo "                 clinical_trials, patent, string"
echo "  - Ensembl genomes (16 model organisms, strain-specific IDs)"
echo ""
echo "Job structure (3 biobtree commands, or 2 with --core-only):"
echo "  1. Core part 1 (8 datasets, no filtering)"
echo "  2. Core part 2 (9 datasets, STRING filtered by --tax to 16 model organisms)"
echo "  3. Ensembl (filtered by --tax to 16 model organisms) [skipped if --core-only]"
echo "============================================"
echo ""

# Create logs directory
mkdir -p logs

################################################ UPDATE phase ################################################

if [[ "$PHASE_MODE" == "generate" ]]; then
    echo "Skipping UPDATE phase (--generate-only mode)"
    echo ""
else
    echo "Starting UPDATE phase..."
    echo ""

    # Resource requirements for UPDATE phase (CPU-intensive)
    JOB_CPU=8
    JOB_MEMORY=32000
    JOB_RUNTIME=604800  # 7 days in seconds
    BB_DEFAULT_PARAM="--include-optionals"

    # Model organism taxonomy IDs (16 organisms from AlphaFold)
    # Format: Human,Mouse,Rat,Zebrafish,Fly,Worm,Yeast(budding),Yeast(fission),E.coli,
    #         Arabidopsis,Rice,Maize,Soybean,Dictyostelium,Candida,Methanocaldococcus
    #
    # IMPORTANT: Ensembl and STRING use DIFFERENT taxonomy ID for S. cerevisiae!
    # - Ensembl: Uses strain-specific ID (559292 for S. cerevisiae S288C)
    # - STRING: Uses species-level ID (4932 for S. cerevisiae species)
    # - S. pombe: Both use 284812 (972h- strain)
    #
    # Ensembl taxonomy IDs (strain-specific for S. cerevisiae):
    ENSEMBL_TAXIDS="9606,10090,10116,7955,7227,6239,559292,284812,511145,3702,39947,4577,3847,44689,237561,243232"

    # STRING taxonomy IDs (species-level for S. cerevisiae only):
    STRING_TAXIDS="9606,10090,10116,7955,7227,6239,4932,284812,511145,3702,39947,4577,3847,44689,237561,243232"

    # Dataset groups
    declare -a SUBMITTED_JOBS=()

    # Core datasets split into 2 parts to reduce concurrent downloads
    CORE_PART1="uniprot,go,eco,hgnc,taxonomy,interpro,hmdb,chembl,clinvar,lipidmaps,swisslipids,uberon,bgee,cl,rhea,gwas_study,gwas,dbsnp,intact"
    CORE_PART2="chebi,efo,mondo,hpo,alphafold,rnacentral,reactome,clinical_trials,patent,string"

    # Calculate total jobs to submit
    TOTAL_JOBS=0
    [[ "$RUN_CORE1" == "true" ]] && ((TOTAL_JOBS++)) || true
    [[ "$RUN_CORE2" == "true" ]] && ((TOTAL_JOBS++)) || true
    [[ "$RUN_ENSEMBL" == "true" ]] && ((TOTAL_JOBS++)) || true

    JOB_NUM=0

    # 1. Submit core part 1 job (if enabled)
    if [[ "$RUN_CORE1" == "true" ]]; then
        ((++JOB_NUM))
        echo "[${JOB_NUM}/${TOTAL_JOBS}] Submitting core_part1 job..."
        rm -rf ${OUT_DIR}/core_part1
        mkdir -p ${OUT_DIR}/core_part1
        cat > run_core_part1.sh <<EOF
#!/bin/bash
cd ${PWD}
./biobtree $BB_DEFAULT_PARAM -d "${CORE_PART1}" --out-dir "${OUT_DIR}/core_part1" -idx core_part1 update
EOF
        chmod +x run_core_part1.sh
        qsub -cwd -V -q "$QUEUE" -N "core_part1" -pe smp $JOB_CPU -l h_vmem=${JOB_MEMORY}M -l h_rt=${JOB_RUNTIME} -o logs/core_part1.log -j y ./run_core_part1.sh
        SUBMITTED_JOBS+=("core_part1")
        echo "  ✓ Submitted: core part 1 (uniprot, go, eco, hgnc, taxonomy, interpro, hmdb, chembl)"
        echo ""
    fi

    # 2. Submit core part 2 job (if enabled, includes STRING with --tax filter)
    if [[ "$RUN_CORE2" == "true" ]]; then
        ((++JOB_NUM))
        echo "[${JOB_NUM}/${TOTAL_JOBS}] Submitting core_part2 job..."
        rm -rf ${OUT_DIR}/core_part2
        mkdir -p ${OUT_DIR}/core_part2
        cat > run_core_part2.sh <<EOF
#!/bin/bash
cd ${PWD}
./biobtree $BB_DEFAULT_PARAM -d "${CORE_PART2}" --tax ${STRING_TAXIDS} --out-dir "${OUT_DIR}/core_part2" -idx core_part2 update
EOF
        chmod +x run_core_part2.sh
        qsub -cwd -V -q "$QUEUE" -N "core_part2" -pe smp $JOB_CPU -l h_vmem=${JOB_MEMORY}M -l h_rt=${JOB_RUNTIME} -o logs/core_part2.log -j y ./run_core_part2.sh
        SUBMITTED_JOBS+=("core_part2")
        echo "  ✓ Submitted: core part 2 (efo, mondo, hpo, alphafold, rnacentral, reactome, clinical_trials, patent, string)"
        echo ""
    fi

    # 3. Submit Ensembl job (if enabled) when building ref db add chembl,mondo,hgnc manually 
    if [[ "$RUN_ENSEMBL" == "true" ]]; then
        ((++JOB_NUM))
        echo "[${JOB_NUM}/${TOTAL_JOBS}] Submitting Ensembl job..."
        rm -rf ${OUT_DIR}/ensembl_model
        mkdir -p ${OUT_DIR}/ensembl_model
        cat > run_ensembl.sh <<EOF
#!/bin/bash
cd ${PWD}
./biobtree $BB_DEFAULT_PARAM --eoa --tax ${ENSEMBL_TAXIDS} -d "ensembl" --out-dir "${OUT_DIR}/ensembl_model" -idx ensembl_model update
EOF
        chmod +x run_ensembl.sh
        qsub -cwd -V -q "$QUEUE" -N "ensembl_model" -pe smp $JOB_CPU -l h_vmem=${JOB_MEMORY}M -l h_rt=${JOB_RUNTIME} -o logs/ensembl_model.log -j y ./run_ensembl.sh
        SUBMITTED_JOBS+=("ensembl_model")
        echo "  ✓ Submitted: Ensembl (filtered to 16 model organisms)"
        echo ""
    fi

    echo "============================================"
    echo "UPDATE phase jobs submitted: ${SUBMITTED_JOBS[@]}"
    echo "============================================"
    echo ""

    # Exit after submitting UPDATE jobs
    echo "Monitor jobs with:"
    echo "  qstat -u \$(whoami)"
    [[ "$RUN_CORE1" == "true" ]] && echo "  tail -f logs/core_part1.log"
    [[ "$RUN_CORE2" == "true" ]] && echo "  tail -f logs/core_part2.log"
    [[ "$RUN_ENSEMBL" == "true" ]] && echo "  tail -f logs/ensembl_model.log"
    echo ""
    echo "After all jobs complete, run GENERATE phase with:"
    echo "  $0 ${OUT_DIR} --generate-only"
    echo ""
    echo "============================================"
    exit 0
fi

################################################ GENERATE phase ################################################

echo "Starting validation before GENERATE phase..."
echo ""

# Validate based on phase mode
if [[ "$PHASE_MODE" == "generate" ]]; then
    # In generate-only mode, check for index files in subdirectories (all 3 required for complete DB)
    if [[ ! -f ${OUT_DIR}/core_part1/index/core_part1.meta.json ]] || \
       [[ ! -f ${OUT_DIR}/core_part2/index/core_part2.meta.json ]] || \
       [[ ! -f ${OUT_DIR}/ensembl_model/index/ensembl_model.meta.json ]]; then
        echo "  ✗ ERROR: Not all index files found"
        echo ""
        echo "Required files for complete database:"
        echo "  - ${OUT_DIR}/core_part1/index/core_part1.meta.json"
        echo "  - ${OUT_DIR}/core_part2/index/core_part2.meta.json"
        echo "  - ${OUT_DIR}/ensembl_model/index/ensembl_model.meta.json"
        echo ""
        echo "Make sure UPDATE phase completed successfully for all 3 jobs."
        echo "Run UPDATE jobs with:"
        echo "  $0 ${OUT_DIR}  # (runs all 3 jobs)"
        echo "Or individually:"
        echo "  $0 ${OUT_DIR} --core1-only"
        echo "  $0 ${OUT_DIR} --core2-only"
        echo "  $0 ${OUT_DIR} --ensembl-only"
        exit 1
    fi
    echo "  ✓ All index files found"
    echo ""

    # Consolidate index files from subdirectories for generate phase
    echo "Consolidating index files..."
    mkdir -p ${OUT_DIR}/index

    # Move all three index directories (always present in generate-only mode)
    mv ${OUT_DIR}/core_part1/index/* ${OUT_DIR}/index/
    rm -rf ${OUT_DIR}/core_part1

    mv ${OUT_DIR}/core_part2/index/* ${OUT_DIR}/index/
    rm -rf ${OUT_DIR}/core_part2

    mv ${OUT_DIR}/ensembl_model/index/* ${OUT_DIR}/index/
    rm -rf ${OUT_DIR}/ensembl_model

    echo "  ✓ Index files consolidated"
else
    # In update mode, check for individual dataset meta files based on what was run
    declare -a FAILED_JOBS=()

    if [[ "$RUN_CORE1" == "true" ]] && [[ ! -f ${OUT_DIR}/core_part1/index/core_part1.meta.json ]]; then
        echo "  ✗ ERROR: meta json file not found for core_part1"
        FAILED_JOBS+=("core_part1")
    fi

    if [[ "$RUN_CORE2" == "true" ]] && [[ ! -f ${OUT_DIR}/core_part2/index/core_part2.meta.json ]]; then
        echo "  ✗ ERROR: meta json file not found for core_part2"
        FAILED_JOBS+=("core_part2")
    fi

    # Check for ensembl_model if it was run
    if [[ "$RUN_ENSEMBL" == "true" ]] && [[ ! -f ${OUT_DIR}/ensembl_model/index/ensembl_model.meta.json ]]; then
        echo "  ✗ ERROR: meta json file not found for ensembl_model"
        FAILED_JOBS+=("ensembl_model")
    fi

    if [[ ${#FAILED_JOBS[@]} -gt 0 ]]; then
        echo ""
        echo "============================================"
        echo "ERROR: Some jobs failed!"
        echo "Failed jobs: ${FAILED_JOBS[@]}"
        echo "============================================"
        echo ""
        echo "Please check the log files in logs/ directory:"
        for job in "${FAILED_JOBS[@]}"; do
            echo "  - logs/${job}.log"
        done
        echo ""
        exit 1
    fi

    echo "  ✓ All validation checks passed"
    echo ""

    # Move all indexes to root directory for generate phase
    echo "Consolidating index files..."
    mkdir -p ${OUT_DIR}/index

    # Move core_part1 index if it was run
    if [[ "$RUN_CORE1" == "true" ]]; then
        mv ${OUT_DIR}/core_part1/index/* ${OUT_DIR}/index/
        rm -rf ${OUT_DIR}/core_part1
    fi

    # Move core_part2 index if it was run
    if [[ "$RUN_CORE2" == "true" ]]; then
        mv ${OUT_DIR}/core_part2/index/* ${OUT_DIR}/index/
        rm -rf ${OUT_DIR}/core_part2
    fi

    # Move ensembl_model index if it was run
    if [[ "$RUN_ENSEMBL" == "true" ]]; then
        mv ${OUT_DIR}/ensembl_model/index/* ${OUT_DIR}/index/
        rm -rf ${OUT_DIR}/ensembl_model
    fi

    echo "  ✓ Index files consolidated"
fi
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
echo "Processing pipeline initiated successfully!"
echo "============================================"
