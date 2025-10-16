#!/bin/bash

# This script helps to process entire data using SGE cluster scheduler. Please check all the comments.

# Whole process can takes long times maybe few days and depends on computing power cpu,memory and disc
# Largest datasets are Ensembl bacteria, uniprot_unreviewed and uniparc

# Similar to mapreduce script has 2 parts, first all the datasets are processed individually or as a group
# and then all the outputs are merged into final biobtree database.


set -e

source ./scripts/data/common_sge.sh

# Hardcoded queue name
QUEUE="scc"

# Parse optional flags
WAIT_FOR_JOBS=false
RUN_GENERATE=false

if [[ -z $1 ]]; then
    echo "out dir parameter is required"
    echo "Usage: $0 <output_dir> [--wait] [--generate] [dataset1 dataset2 ...]"
    echo "Example: $0 /output/dir"
    echo "Example: $0 /output/dir --wait --generate"
    echo "Example: $0 /output/dir def uniref ensembl_fungi"
    echo "Example: $0 /output/dir --wait def uniref"
    echo ""
    echo "Options:"
    echo "  --wait      Wait for jobs to complete (useful for Ensembl datasets to avoid rate limiting)"
    echo "  --generate  Run the generate phase after update (merges all datasets into final DB)"
    exit 1
fi

OUT_DIR=$1
shift 1  # Remove output dir from arguments

# Collect selected datasets and flags from command line arguments
SELECTED_DATASETS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --wait)
            WAIT_FOR_JOBS=true
            shift
            ;;
        --generate)
            RUN_GENERATE=true
            shift
            ;;
        *)
            SELECTED_DATASETS+=("$1")
            shift
            ;;
    esac
done

if [[ ${#SELECTED_DATASETS[@]} -gt 0 ]]; then
    echo "Running selected datasets: ${SELECTED_DATASETS[@]}"
else
    echo "Running all datasets"
fi

echo "Wait for jobs: $WAIT_FOR_JOBS"
echo "Run generate phase: $RUN_GENERATE"

# Function to check if a dataset is selected (or if all datasets should run)
should_run_dataset() {
    local dataset=$1
    # If no selection, run all
    if [[ ${#SELECTED_DATASETS[@]} -eq 0 ]]; then
        return 0
    fi
    # Check if dataset is in selected list
    for selected in "${SELECTED_DATASETS[@]}"; do
        if [[ "$selected" == "$dataset" ]]; then
            return 0
        fi
    done
    return 1
}

# Create logs directory if it doesn't exist
mkdir -p logs

################################################ UPDATE phase ################################################

JOB_CPU=8
JOB_MEMORY=16000
# Runtime: 7 days in seconds (604800)
JOB_RUNTIME=604800
BB_DEFAULT_PARAM="--include-optionals"

declare -a DATASETS=("def;uniprot,go,eco,hgnc,taxonomy,interpro,hmdb,literature_mappings,chembl,efo" "uniref;uniref50,uniref90,uniref100" "uniparc;uniparc" "uniprot_unreviewed;uniprot_unreviewed")
declare -a SUBMITTED_DATASETS=()
for dt in "${DATASETS[@]}"
    do
    arrDataset=(${dt//;/ })
    if should_run_dataset "${arrDataset[0]}"; then
        rm -rf ${OUT_DIR}/${arrDataset[0]}
        mkdir -p ${OUT_DIR}/${arrDataset[0]}
        # Create wrapper script
        cat > run_${arrDataset[0]}.sh <<EOF
#!/bin/bash
cd ${PWD}
./biobtree $BB_DEFAULT_PARAM -d ${arrDataset[1]} --out-dir "${OUT_DIR}/${arrDataset[0]}" -idx ${arrDataset[0]} update
EOF
        chmod +x run_${arrDataset[0]}.sh
        # SGE: qsub -cwd -V -q queue -N jobname -pe smp threads -l h_vmem=XXXM -l h_rt=runtime -o logfile -j y script
        qsub -cwd -V -q "$QUEUE" -N "${arrDataset[0]}" -pe smp $JOB_CPU -l h_vmem=${JOB_MEMORY}M -l h_rt=${JOB_RUNTIME} -o logs/${arrDataset[0]}.log -j y ./run_${arrDataset[0]}.sh
        SUBMITTED_DATASETS+=("${arrDataset[0]}")
        echo "Submitted job: ${arrDataset[0]}"
    fi
    done

# Ensembls are processed one by one like here via wait to avoid being temporarily rejected from Ensembl servers
# biobtree also has its internal configurable sleep to avoid being rejected
# So ideally this works but if you get any connection related error wait for a while and try again for the given dataset manually to avoid repeating previous steps.
declare -a ENS_DATASETS=("ensembl_fungi" "ensembl_metazoa" "ensembl_protists" "ensembl_plants" "ensembl" "ensembl_bacteria")
BB_ENSEMBL_PARAM="--eoa --genome all"
for ens in "${ENS_DATASETS[@]}"
    do
    if should_run_dataset "$ens"; then
        rm -rf ${OUT_DIR}/${ens}
        mkdir -p ${OUT_DIR}/${ens}
        # Create wrapper script
        cat > run_${ens}.sh <<EOF
#!/bin/bash
cd ${PWD}
./biobtree $BB_DEFAULT_PARAM $BB_ENSEMBL_PARAM -d $ens --out-dir "${OUT_DIR}/${ens}" -idx $ens update
EOF
        chmod +x run_${ens}.sh
        qsub -cwd -V -q "$QUEUE" -N "$ens" -pe smp $JOB_CPU -l h_vmem=${JOB_MEMORY}M -l h_rt=${JOB_RUNTIME} -o logs/${ens}.log -j y ./run_${ens}.sh
        SUBMITTED_DATASETS+=("$ens")
        echo "Submitted job: $ens"
        if [[ "$WAIT_FOR_JOBS" == "true" ]]; then
            waitJob $ens
            sleep 300
        fi
    fi
    done

# Wait for non-ensembl jobs if --wait flag is set
if [[ "$WAIT_FOR_JOBS" == "true" ]]; then
    echo "Waiting for non-ensembl jobs to complete..."
    for dt in "${DATASETS[@]}"
        do
        arrDataset=(${dt//;/ })
        if should_run_dataset "${arrDataset[0]}"; then
            waitJob ${arrDataset[0]} "true"
        fi
        done
fi


################################################ GENERATE phase ################################################

if [[ "$RUN_GENERATE" == "true" ]]; then
    echo "Running generate phase..."
    # validate by checking meta jsons are created
    for dt in "${DATASETS[@]}"
        do
        arrDataset=(${dt//;/ })
        if [[ ! -f ${OUT_DIR}/${arrDataset[0]}/index/${arrDataset[0]}.meta.json ]]; then
            echo "ERROR meta json file not found for dataset ${arrDataset[0]}. Check its logs"
            exit 1
        fi
        done

    for ens in "${ENS_DATASETS[@]}"
        do
        if [[ ! -f ${OUT_DIR}/${ens}/index/${ens}.meta.json ]]; then
            echo "ERROR meta json file not found for dataset ${ens}. Check its logs"
            exit 1
        fi
        done

    # move all indexes to the root for generate and clean update phase folders.

    mkdir -p ${OUT_DIR}/index

    for dt in "${DATASETS[@]}"
        do
        arrDataset=(${dt//;/ })
        mv ${OUT_DIR}/${arrDataset[0]}/index/* ${OUT_DIR}/index
        rm -rf ${OUT_DIR}/${arrDataset[0]}
        done

    for ens in "${ENS_DATASETS[@]}"
        do
        mv ${OUT_DIR}/${ens}/index/* ${OUT_DIR}/index
        rm -rf ${OUT_DIR}/${ens}
        done


    # Generate process. It is not CPU intensive but ideally requires a dedicated machine with large available memory

    nohup ./biobtree --keep --out-dir ${OUT_DIR} generate > logs/generate.log 2>&1  &

    # After recommended to create a backup of the db folder. Index folder content can be deleted to save disc space.
    # Finally biobtree can be used as -> nohup ./biobtree --out-dir ${OUT_DIR} web > logs/web.log 2>&1  &

    echo "All done! Generate phase completed."
else
    echo ""
    echo "============================================"
    echo "Update phase completed!"
    echo "Submitted datasets: ${SUBMITTED_DATASETS[@]}"
    echo ""
    if [[ "$WAIT_FOR_JOBS" == "false" ]]; then
        echo "Jobs are still running. Monitor them with: qstat"
        echo "Wait for all jobs to complete before running generate phase."
        echo ""
    fi
    echo "To run the generate phase manually:"
    echo "  ./biobtree --keep --out-dir ${OUT_DIR} generate"
    echo ""
    echo "Or rerun this script with --generate flag after jobs complete:"
    echo "  $0 ${OUT_DIR} --generate"
    echo "============================================"
fi
