#!/bin/bash

# experimental currently not in use 
# Workaround script related with issue when there is a new ensembl release to build the paths
# Output of this script used with local ftp server

set -e

if [[ -z $1 ]]; then
    echo " Target directory path is requried."
    exit 1
fi

if [[ -z $2 ]]; then
    echo " Ensembl release number required."
    exit 1
fi

if [[ -z $3 ]]; then
    echo " Ensembl Genomes release number required."
    exit 1
fi

TARGET=$1
E_RELEASE=$2
EG_RELEASE=$3

rm -rf $TARGET
mkdir $TARGET

cd $TARGET

E_FTP="ftp.ensembl.org"
EG_FTP="ftp.ensemblgenomes.org"

# try these alternative in case of issue
#E_FTP="ftp.ebi.ac.uk/ensemblorg"
#EG_FTP="ftp.ebi.ac.uk/ensemblgenomes"


creat_dir_and_files () {
    CUR_DIR=`pwd`
    mkdir -p $1
    cd $1    
    while IFS= read -r line; do
        if [[ "$line" == *: ]]; then
            mkdir -p ${line%?} || true
        elif [[ "$line" == *.gz ]]; then
            touch $line
        fi
    done < $CUR_DIR'/'$2
    cd $CUR_DIR
}

creat_dir_and_files_json () {
    CUR_DIR=`pwd`
    mkdir -p $1
    cd $1    
    while IFS= read -r line; do
        if [[ "$line" == *: ]]; then
            mkdir -p ${line%?} || true
        elif [[ "$line" == *.json ]]; then
            touch $line
        fi
    done < $CUR_DIR'/'$2
    cd $CUR_DIR
}

creat_dir_and_files_mysql () {
    CUR_DIR=`pwd`
    mkdir -p $1
    cd $1    
    while IFS= read -r line; do
        if [[ "$line" == *: ]]; then
            mkdir -p ${line%?} || true
        elif [[ "$line" == *__efg_*.gz ]]; then
            touch $line
        fi
    done < $CUR_DIR'/'$2
    cd $CUR_DIR
}


lftp -e 'nlist -R; quit' $E_FTP'/pub/current_json' > e_json.txt
echo "e json done"
creat_dir_and_files_json 'ensembl/pub/current_json' 'e_json.txt'
lftp -e 'nlist -R; quit' $E_FTP'/pub/current_gff3' > e_gff3.txt
echo "e gff3 done"
creat_dir_and_files 'ensembl/pub/current_gff3' 'e_gff3.txt'
lftp -e 'nlist -R; quit' $E_FTP'/pub/current_mysql/ensembl_mart_'$E_RELEASE > e_mysql.txt
echo "e mysql done"
creat_dir_and_files_mysql 'ensembl/pub/current_mysql/ensembl_mart_'$E_RELEASE 'e_mysql.txt'

declare -a EG_BRANCHES=("fungi" "metazoa" "protists" "plants" "bacteria")
for branch in "${EG_BRANCHES[@]}"
    do
    lftp -e 'nlist -R; quit' $EG_FTP'/pub/current/'${branch}'/json' > eg_${branch}_json.txt
    echo "eg ${branch} json done"
    creat_dir_and_files_json 'ensemblg/pub/current/'${branch}'/json' 'eg_'${branch}'_json.txt'
    lftp -e 'nlist -R; quit' $EG_FTP'/pub/current/'${branch}'/gff3' > eg_${branch}_gff3.txt
    echo "eg ${branch} gff3 done"
    creat_dir_and_files 'ensemblg/pub/current/'${branch}'/gff3' 'eg_'${branch}'_gff3.txt'
    if [[ "$branch" != "bacteria" ]]; then
        lftp -e 'nlist -R; quit' $EG_FTP'/pub/current/'${branch}'/mysql/'${branch}'_mart_'$EG_RELEASE > eg_${branch}_mysql.txt
        echo "eg ${branch} mysql done"
        creat_dir_and_files_mysql 'ensemblg/pub/current/'${branch}'/mysql/'${branch}'_mart_'$EG_RELEASE 'eg_'${branch}'_mysql.txt'
    fi
    done

lftp -e 'get /pub/current/species.txt; quit' $EG_FTP

# lftp -e "mirror -R {local dir} {remote dir}" -u {username},{password} {host}

