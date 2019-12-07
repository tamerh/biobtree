#!/bin/bash

set -e

if [[ -z $1 ]]; then
    echo " queue paramater is requried."
    exit 1
fi

if [[ -z $2 ]]; then
    echo " out dir paramater is requried."
    exit 1
fi

waitJobsForCompletion() {
    sleep 600
    echo "Starting check if jobs are finished with following command--> bjobs -P $@ | wc -l"
    while [ true ]
    do
        BJOBS_RESULT=`bjobs -P $@ | wc -l`
        
        if [ "$BJOBS_RESULT" == 0  ]
        then
            echo "All jobs are now finished"
            break
        fi
        sleep 120
    done
    echo "Check on jobs completed"
}

# common parameters
JOB_CPU=8
JOB_MEMORY=16000
BB_DEFAULT_PARAM="--include-optionals"

declare -a DATASETS=("def;uniprot,go,eco,hgnc,chebi,taxonomy,interpro,hmdb,literature_mappings,chembl,efo" "uniref;uniref50,uniref90,uniref100" "uniparc;uniparc" "uniprot_unreviewed;uniprot_unreviewed")
for dt in "${DATASETS[@]}"
    do
    arrDataset=(${dt//;/ })
    echo ${2}/${arrDataset[0]}
    rm -rf ${2}/${arrDataset[0]}
    mkdir -p ${2}/${arrDataset[0]}
    bsub -oo ${arrDataset[0]}.log -P "${arrDataset[0]}" -n $JOB_CPU -M $JOB_MEMORY -R "rusage[mem=${JOB_MEMORY}] span[hosts=1]" -J "${arrDataset[0]}" -q "$1" ./biobtree $BB_DEFAULT_PARAM -d ${arrDataset[2]} --out-dir "${2}/${arrDataset[0]}" -idx ${arrDataset[0]} update
    done

declare -a ENS_DATASETS=("ensembl_fungi" "ensembl_metazoa" "ensembl_protists" "ensembl_plants" "ensembl" "ensembl_bacteria")
BB_ENSEMBL_PARAM="--eoa --genome all"
for ens in "${ENS_DATASETS[@]}"
    do
    rm -rf ${2}/${ens}
    mkdir -p ${2}/${ens}
    bsub -oo ${ens}.log -P "$ens" -n $JOB_CPU -M $JOB_MEMORY -R "rusage[mem=${JOB_MEMORY}] span[hosts=1]" -J "$ens" -q "$1" ./biobtree $BB_DEFAULT_PARAM $BB_ENSEMBL_PARAM -d $ens --out-dir "${2}/${ens}" -idx $ens update
    waitJobsForCompletion $ens
    sleep 300
    done