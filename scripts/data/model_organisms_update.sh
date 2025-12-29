#!/bin/bash

# This script runs the UPDATE phase for model organisms subset of biobtree.
# Based on AlphaFold's 16 model organisms: https://alphafold.ebi.ac.uk/download#proteomes-section
#
# The approach includes:
# - Ensembl genomes for each model organism (via --tax flag)
# - STRING protein interactions for each model organism (via --tax flag)
# - Core datasets split into 3 parts (core1, core2, core3-dbsnp)
#
# UPDATE phase: CPU-intensive (downloads and processes data)
# After completion, run the separate GENERATE script: model_organisms_generate.sh
#
# Jobs run SEQUENTIALLY with retry logic (max 2 attempts per job)

set -e

# Parse arguments
if [[ -z $1 ]]; then
    echo "out dir parameter is required"
    echo "Usage: $0 <output_dir> [OPTIONS]"
    echo "Example: $0 /localscratch/\$USER/biobtree_model_organisms"
    echo "Example: $0 /localscratch/\$USER/biobtree_model_organisms --core1-only"
    echo "Example: $0 /localscratch/\$USER/biobtree_model_organisms --maxcpu 8"
    echo ""
    echo "Options:"
    echo "  (default)         Run all UPDATE phase jobs (core1 + core2 + core3 + core4 + ensembl)"
    echo "  --core1-only      Run only core part 1 job"
    echo "  --core2-only      Run only core part 2 job"
    echo "  --core3-only      Run only core part 3 job (dbsnp - large dataset)"
    echo "  --core4-only      Run only core part 4 job (pubchem datasets)"
    echo "  --core5-only      Run only core part 5 job (entrez gene)"
    echo "  --ensembl-only    Run only Ensembl job"
    echo "  --maxcpu <N>      Maximum number of CPUs for biobtree (default: 4)"
    exit 1
fi

OUT_DIR=$1
RUN_CORE1="true"        # Default: run core1
RUN_CORE2="true"        # Default: run core2
RUN_CORE3="true"        # Default: run core3 (dbsnp)
RUN_CORE4="true"        # Default: run core4 (pubchem)
RUN_CORE5="true"        # Default: run core5 (entrez)
RUN_ENSEMBL="true"      # Default: run ensembl
MAXCPU=8                # Default: 8 CPUs

# Parse additional arguments
shift
while [[ $# -gt 0 ]]; do
    case "$1" in
        --core1-only)
            RUN_CORE1="true"
            RUN_CORE2="false"
            RUN_CORE3="false"
            RUN_CORE4="false"
            RUN_CORE5="false"
            RUN_ENSEMBL="false"
            shift
            ;;
        --core2-only)
            RUN_CORE1="false"
            RUN_CORE2="true"
            RUN_CORE3="false"
            RUN_CORE4="false"
            RUN_CORE5="false"
            RUN_ENSEMBL="false"
            shift
            ;;
        --core3-only)
            RUN_CORE1="false"
            RUN_CORE2="false"
            RUN_CORE3="true"
            RUN_CORE4="false"
            RUN_CORE5="false"
            RUN_ENSEMBL="false"
            shift
            ;;
        --core4-only)
            RUN_CORE1="false"
            RUN_CORE2="false"
            RUN_CORE3="false"
            RUN_CORE4="true"
            RUN_CORE5="false"
            RUN_ENSEMBL="false"
            shift
            ;;
        --core5-only)
            RUN_CORE1="false"
            RUN_CORE2="false"
            RUN_CORE3="false"
            RUN_CORE4="false"
            RUN_CORE5="true"
            RUN_ENSEMBL="false"
            shift
            ;;
        --ensembl-only)
            RUN_CORE1="false"
            RUN_CORE2="false"
            RUN_CORE3="false"
            RUN_CORE4="false"
            RUN_CORE5="false"
            RUN_ENSEMBL="true"
            shift
            ;;
        --maxcpu)
            if [[ -z $2 || $2 =~ ^- ]]; then
                echo "Error: --maxcpu requires a numeric argument"
                exit 1
            fi
            MAXCPU=$2
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Valid options: --core1-only, --core2-only, --core3-only, --core4-only, --core5-only, --ensembl-only, --maxcpu <N>"
            exit 1
            ;;
    esac
done

echo "============================================"
echo "Biobtree Model Organisms - UPDATE Phase"
echo "============================================"
echo "Output directory: $OUT_DIR"
echo "Jobs to run: Core1=$RUN_CORE1, Core2=$RUN_CORE2, Core3=$RUN_CORE3, Core4=$RUN_CORE4, Core5=$RUN_CORE5, Ensembl=$RUN_ENSEMBL"
echo "Max CPUs: $MAXCPU"
echo "Execution: SEQUENTIAL (one job at a time with retry)"
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
echo "  - Core part 1: uniprot, taxonomy, interpro, hmdb, chembl,"
echo "                 clinvar, lipidmaps, swisslipids, gwas_study,"
echo "                 gwas, intact, antibody"
echo "  - Core part 2: chebi, alphafold, rnacentral, reactome,"
echo "                 clinical_trials, patent, string, bgee, ontology"
echo "  - Core part 3: dbsnp (large dataset, prone to FTP issues)"
echo "  - Core part 4: pubchem, pubchem_activity, pubchem_assay"
echo "  - Core part 5: entrez gene + refseq (large datasets, model organism genomes)"
echo "  - Ensembl genomes (16 model organisms, strain-specific IDs)"
echo ""
echo "Retry policy: Each job will be retried once if it fails (max 2 attempts)"
echo "Wait time between retries: 5 minutes"
echo "============================================"
echo ""

# Create logs directory
mkdir -p logs

BB_DEFAULT_PARAM="--include-optionals -c --lookupdb --maxcpu ${MAXCPU}"

# Model organism taxonomy IDs (16 organisms from AlphaFold)
# IMPORTANT: Ensembl and STRING use DIFFERENT taxonomy ID for S. cerevisiae!
# - Ensembl: Uses strain-specific ID (559292 for S. cerevisiae S288C)
# - STRING: Uses species-level ID (4932 for S. cerevisiae species)
ENSEMBL_TAXIDS="9606,10090,10116,7955,7227,6239,559292,284812,511145,3702,39947,4577,3847,44689,237561,243232"
STRING_TAXIDS="9606,10090,10116,7955,7227,6239,4932,284812,511145,3702,39947,4577,3847,44689,237561,243232"

# Core datasets split into 4 parts: TODO add back diamond later 
CORE_PART1="uniprot,taxonomy,interpro,hmdb,chembl,clinvar,lipidmaps,swisslipids,gwas_study,gwas,intact,antibody,protein_similarity,rhea"
CORE_PART2="chebi,alphafold,rnacentral,reactome,clinical_trials,patent,string,bgee,ontology,mesh,gencc"

# Part 3: Large/unstable dataset (dbsnp - prone to FTP issues, needs retry)
CORE_PART3="dbsnp"
# Part 4: PubChem datasets
CORE_PART4="pubchem,pubchem_activity,pubchem_assay"
# Part 5: Entrez Gene + RefSeq (large datasets, uses genome-taxids for RefSeq)
CORE_PART5="entrez,refseq"

# Calculate total jobs to run
TOTAL_JOBS=0
[[ "$RUN_CORE1" == "true" ]] && ((TOTAL_JOBS++)) || true
[[ "$RUN_CORE2" == "true" ]] && ((TOTAL_JOBS++)) || true
[[ "$RUN_CORE3" == "true" ]] && ((TOTAL_JOBS++)) || true
[[ "$RUN_CORE4" == "true" ]] && ((TOTAL_JOBS++)) || true
[[ "$RUN_CORE5" == "true" ]] && ((TOTAL_JOBS++)) || true
[[ "$RUN_ENSEMBL" == "true" ]] && ((TOTAL_JOBS++)) || true

JOB_NUM=0

# Retry configuration
MAX_RETRIES=2  # Total 3 attempts (initial + 2 retries)
WAIT_MINUTES=5

# Helper function to run a job with retry logic
run_job_with_retry() {
    local job_name=$1
    local job_command=$2
    local log_file=$3
    local out_dir=$4

    echo "============================================"
    echo "Starting: ${job_name}"
    echo "============================================"

    attempt=1
    while [ $attempt -le $((MAX_RETRIES + 1)) ]; do
        echo "Attempt ${attempt} of $((MAX_RETRIES + 1)) for ${job_name}"

        # Clean output directory before retry (except on first attempt)
        if [ $attempt -gt 1 ]; then
            echo "Cleaning ${out_dir} before retry..."
            rm -rf ${out_dir}/*
            sleep 2
        fi

        # Run the job
        echo "Running: ${job_command}"
        eval ${job_command} > ${log_file} 2>&1

        # Check if successful
        if [ $? -eq 0 ]; then
            echo "✓ ${job_name} completed successfully on attempt ${attempt}"
            echo ""
            return 0
        else
            echo "✗ ${job_name} failed on attempt ${attempt}"

            if [ $attempt -le $MAX_RETRIES ]; then
                echo "Waiting ${WAIT_MINUTES} minutes before retry..."
                sleep $((WAIT_MINUTES * 60))
                ((attempt++))
            else
                echo "✗ Maximum retries exceeded for ${job_name}. Giving up."
                echo ""
                return 1
            fi
        fi
    done
}

# 1. Run core part 1 job (if enabled)
if [[ "$RUN_CORE1" == "true" ]]; then
    ((++JOB_NUM))
    echo ""
    echo "[${JOB_NUM}/${TOTAL_JOBS}] Core Part 1"
    rm -rf ${OUT_DIR}/core_part1
    mkdir -p ${OUT_DIR}/core_part1

    CMD="./biobtree $BB_DEFAULT_PARAM -d \"${CORE_PART1}\" --out-dir \"${OUT_DIR}/core_part1\" -idx core_part1 update"

    if ! run_job_with_retry "Core Part 1" "$CMD" "logs/core_part1.log" "${OUT_DIR}/core_part1"; then
        echo "ERROR: Core Part 1 failed after retries. Exiting."
        exit 1
    fi
fi

# 2. Run core part 2 job (if enabled, includes STRING with --tax filter)
if [[ "$RUN_CORE2" == "true" ]]; then
    ((++JOB_NUM))
    echo ""
    echo "[${JOB_NUM}/${TOTAL_JOBS}] Core Part 2"
    rm -rf ${OUT_DIR}/core_part2
    mkdir -p ${OUT_DIR}/core_part2

    CMD="./biobtree $BB_DEFAULT_PARAM -d \"${CORE_PART2}\" --tax ${STRING_TAXIDS} --out-dir \"${OUT_DIR}/core_part2\" -idx core_part2 update"

    if ! run_job_with_retry "Core Part 2" "$CMD" "logs/core_part2.log" "${OUT_DIR}/core_part2"; then
        echo "ERROR: Core Part 2 failed after retries. Exiting."
        exit 1
    fi
fi

# 3. Run core part 3 job (dbsnp) (if enabled)
if [[ "$RUN_CORE3" == "true" ]]; then
    ((++JOB_NUM))
    echo ""
    echo "[${JOB_NUM}/${TOTAL_JOBS}] Core Part 3 (dbsnp)"
    rm -rf ${OUT_DIR}/core_part3
    mkdir -p ${OUT_DIR}/core_part3

    # Use --bucket-sort-workers 1 for dbsnp to reduce memory usage during sorting
    # dbsnp bucket files can be 30GB+ each, causing OOM with multiple workers
    CMD="./biobtree $BB_DEFAULT_PARAM --bucket-sort-workers 1 -d \"${CORE_PART3}\" --out-dir \"${OUT_DIR}/core_part3\" -idx core_part3 update"

    if ! run_job_with_retry "Core Part 3 (dbsnp)" "$CMD" "logs/core_part3_dbsnp.log" "${OUT_DIR}/core_part3"; then
        echo "ERROR: Core Part 3 (dbsnp) failed after retries. Exiting."
        exit 1
    fi
fi

# 4. Run core part 4 job (pubchem) (if enabled)
if [[ "$RUN_CORE4" == "true" ]]; then
    ((++JOB_NUM))
    echo ""
    echo "[${JOB_NUM}/${TOTAL_JOBS}] Core Part 4 (pubchem)"
    rm -rf ${OUT_DIR}/core_part4
    mkdir -p ${OUT_DIR}/core_part4

    # Use --pubchem-sdf-workers 1 to reduce memory usage during SDF parsing
    # Each SDF file can be 100-500MB compressed, 1-5GB uncompressed - multiple workers cause OOM
    CMD="./biobtree $BB_DEFAULT_PARAM --pubchem-sdf-workers 1 -d \"${CORE_PART4}\" --out-dir \"${OUT_DIR}/core_part4\" -idx core_part4 update"

    if ! run_job_with_retry "Core Part 4 (pubchem)" "$CMD" "logs/core_part4_pubchem.log" "${OUT_DIR}/core_part4"; then
        echo "ERROR: Core Part 4 (pubchem) failed after retries. Exiting."
        exit 1
    fi
fi

# 5. Run core part 5 job (entrez + refseq) (if enabled)
if [[ "$RUN_CORE5" == "true" ]]; then
    ((++JOB_NUM))
    echo ""
    echo "[${JOB_NUM}/${TOTAL_JOBS}] Core Part 5 (entrez + refseq)"
    rm -rf ${OUT_DIR}/core_part5
    mkdir -p ${OUT_DIR}/core_part5

    # RefSeq uses --genome-taxids with same tax IDs as Ensembl
    CMD="./biobtree $BB_DEFAULT_PARAM -d \"${CORE_PART5}\" --genome-taxids ${ENSEMBL_TAXIDS} --out-dir \"${OUT_DIR}/core_part5\" -idx core_part5 update"

    if ! run_job_with_retry "Core Part 5 (entrez + refseq)" "$CMD" "logs/core_part5_entrez_refseq.log" "${OUT_DIR}/core_part5"; then
        echo "ERROR: Core Part 5 (entrez + refseq) failed after retries. Exiting."
        exit 1
    fi
fi

# 6. Run Ensembl job (if enabled)
if [[ "$RUN_ENSEMBL" == "true" ]]; then
    ((++JOB_NUM))
    echo ""
    echo "[${JOB_NUM}/${TOTAL_JOBS}] Ensembl"
    rm -rf ${OUT_DIR}/ensembl_model
    mkdir -p ${OUT_DIR}/ensembl_model

    CMD="./biobtree $BB_DEFAULT_PARAM --eo --tax ${ENSEMBL_TAXIDS} -d \"ensembl\" --out-dir \"${OUT_DIR}/ensembl_model\" -idx ensembl_model update"

    if ! run_job_with_retry "Ensembl" "$CMD" "logs/ensembl_model.log" "${OUT_DIR}/ensembl_model"; then
        echo "ERROR: Ensembl failed after retries. Exiting."
        exit 1
    fi
fi

echo ""
echo "============================================"
echo "UPDATE Phase Complete!"
echo "============================================"
echo ""
echo "All jobs completed successfully."
echo ""
echo "Log files:"
[[ "$RUN_CORE1" == "true" ]] && echo "  - logs/core_part1.log"
[[ "$RUN_CORE2" == "true" ]] && echo "  - logs/core_part2.log"
[[ "$RUN_CORE3" == "true" ]] && echo "  - logs/core_part3_dbsnp.log"
[[ "$RUN_CORE4" == "true" ]] && echo "  - logs/core_part4_pubchem.log"
[[ "$RUN_CORE5" == "true" ]] && echo "  - logs/core_part5_entrez_refseq.log"
[[ "$RUN_ENSEMBL" == "true" ]] && echo "  - logs/ensembl_model.log"
echo ""
echo "Next step: Run GENERATE phase with:"
echo "  ./scripts/data/model_organisms_generate.sh ${OUT_DIR}"
echo ""
echo "============================================"
