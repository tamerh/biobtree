#!/bin/bash

# This script helps to process entire data. Please check all the comments. 

# Whole process can takes long times maybe few days and depends on computing power cpu,memory and disc
# Largest datasets are Ensembl bacteria, uniprot_unreviewed and uniparc

# Similar to mapreduce script has 2 parts, first all the datasets are processed individually or as a group 
# and then all the outputs are merged into final biobtree database.


set -e

source ./scripts/data/common.sh

if [[ -z $1 ]]; then
    echo " queue paramater is requried."
    exit 1
fi

if [[ -z $2 ]]; then
    echo " out dir paramater is requried."
    exit 1
fi


################################################ UPDATE phase ################################################

JOB_CPU=8
JOB_MEMORY=16000
BB_DEFAULT_PARAM="--include-optionals"

declare -a DATASETS=("def;uniprot,go,eco,hgnc,chebi,taxonomy,interpro,hmdb,literature_mappings,chembl,efo" "uniref;uniref50,uniref90,uniref100" "uniparc;uniparc" "uniprot_unreviewed;uniprot_unreviewed")
for dt in "${DATASETS[@]}"
    do
    arrDataset=(${dt//;/ })
    rm -rf ${2}/${arrDataset[0]}
    mkdir -p ${2}/${arrDataset[0]}
    bsub -oo ${arrDataset[0]}.log -P "${arrDataset[0]}" -n $JOB_CPU -M $JOB_MEMORY -R "rusage[mem=${JOB_MEMORY}] span[hosts=1]" -J "${arrDataset[0]}" -q "$1" ./biobtree $BB_DEFAULT_PARAM -d ${arrDataset[1]} --out-dir "${2}/${arrDataset[0]}" -idx ${arrDataset[0]} update
    done

# Ensembls are recommended to be processed one by one like here via wait to avoid being temporarily rejected from Ensembl servers due to the high traffic and limitation.
# In addition biobtree also has its internal configurable sleeps to avoid being rejected
# So ideally this works but if you get any connection related error wait for a while and try again for the given dataset manually.
declare -a ENS_DATASETS=("ensembl_fungi" "ensembl_metazoa" "ensembl_protists" "ensembl_plants" "ensembl" "ensembl_bacteria")
BB_ENSEMBL_PARAM="--eoa --genome all"
for ens in "${ENS_DATASETS[@]}"
    do
    rm -rf ${2}/${ens}
    mkdir -p ${2}/${ens}
    bsub -oo ${ens}.log -P "$ens" -n $JOB_CPU -M $JOB_MEMORY -R "rusage[mem=${JOB_MEMORY}] span[hosts=1]" -J "$ens" -q "$1" ./biobtree $BB_DEFAULT_PARAM $BB_ENSEMBL_PARAM -d $ens --out-dir "${2}/${ens}" -idx $ens update
    waitJob $ens
    sleep 300
    done

# Now make sure all the jobs are finished
for dt in "${DATASETS[@]}"
    do
    arrDataset=(${dt//;/ })
    waitJob ${arrDataset[0]} "true"
    done


################################################ GENERATE phase ################################################

# validate  meta jsons are created
for dt in "${DATASETS[@]}"
    do
    arrDataset=(${dt//;/ })
    if [[ ! -f ${2}/${arrDataset[0]}/index/${arrDataset[0]}.meta.json ]]; then
        echo "ERROR meta json file not found for dataset ${arrDataset[0]}. Check its logs"
        exit 1
    fi
    done

for ens in "${ENS_DATASETS[@]}"
    do
    if [[ ! -f ${2}/${ens}/index/${ens}.meta.json ]]; then
        echo "ERROR meta json file not found for dataset ${ens}. Check its logs"
        exit 1
    fi
    done

# now move all indexes to the root for final generation and clean update phase folders.

mkdir -p ${2}/index

for dt in "${DATASETS[@]}"
    do
    arrDataset=(${dt//;/ })
    mv ${2}/${arrDataset[0]}/index/* ${2}/index
    rm -rf ${2}/${arrDataset[0]}
    done

for ens in "${ENS_DATASETS[@]}"
    do
    mv ${2}/${ens}/index/* ${2}/index
    rm -rf ${2}/${ens}
    done

 
# Generate process. It is not CPU intensive but ideally requires a dedicated machine with large available memory such as 128GB.

nohup ./biobtree --keep --out-dir ${2} generate > generate.log 2>&1  & 

# After recommended to create a backup of the db folder. Index folder content can be deleted to save disc space. 
# Finally biobtree can be used as -> nohup ./biobtree --out-dir ${2} web > web.log 2>&1  &

echo "All done"