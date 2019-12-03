#!/bin/bash

set -e 

if [[ -z $1 ]]; then
    echo " Version paramater is requried."
    exit 1
fi

VERSION=$1

if [[ ! -f biobtree ]]; then

    if [[ "$OSTYPE" == "linux-gnu" ]]; then
        OS="Linux"
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        OS="MacOS"
    fi

    rm -f biobtree_*_64bit.tar.gz

    bbLatestVersion=`curl -Ls -o /dev/null -w %{url_effective} https://github.com/tamerh/biobtree/releases/latest | rev | cut -d '/' -f 1 | rev`

    curl -OL https://github.com/tamerh/biobtree/releases/download/$bbLatestVersion/biobtree_${OS}_64bit.tar.gz 

    tar -xzvf biobtree_${OS}_64bit.tar.gz

fi




if [[ "$OSTYPE" == "linux-gnu" ]]; then
GNUSORT="sort"
elif [[ "$OSTYPE" == "darwin"* ]]; then
GNUSORT="/usr/local/opt/coreutils/libexec/gnubin/sort"
fi



clearConfs(){
    rm -rf out
    rm -rf conf
    rm -rf ensembl
    rm -rf website
    rm -rf LICENSE*
}
prepCache(){

    cd out/index

    mkdir -p tmp
    mv *.gz tmp/
    cd tmp
    gunzip *.gz

    LC_ALL=C $GNUSORT -m -u * > ../cache.${1}.index
    cd ..
    rm -rf tmp
    gzip cache.${1}.index

    cd ../..

    #EXCLUDES="--exclude=biobtree --exclude=data.sh  --exclude=LICENSE* --exclude=*website* --exclude=*ensembl* --exclude=*conf* --exclude=*.git* --exclude=*build.sh  --exclude=*notes.txt --exclude=*.zip --exclude=*.DS_Store*"

    tar -czvf biobtree-conf-${VERSION}-${1}r.tar.gz out/

    ./biobtree --lmdbsize ${2} generate

    tar -czvf biobtree-conf-${VERSION}-${1}d.tar.gz out/

    rm -rf out

}

#### genomes note
#homo_sapiens 9606 --> ensembl
#danio_rerio 7955 zebrafish --> ensembl
# gallus_gallus 9031 chicken --> ensembl
#mus_musculus 10090 --> ensembl
# Rattus norvegicus 10116 ---> ensembl
#saccharomyces_cerevisiae 4932--> ensembl
#arabidopsis_thaliana 3702--> ensembl
#drosophila_melanogaster 7227 --> ensembl
#caenorhabditis_elegans 6239 --> ensembl
#Escherichia coli 562 --> ensembl_bacteria
#Escherichia coli str. K-12 substr. MG1655 511145 --> ensembl_bacteria
#Escherichia coli K-12 83333 --> ensembl_bacteria
# taxids 9606,10090,4932,3702,7227,6239,562,511145,83333,7955


clearConfs

# for test
# ./biobtree -d hgnc --idx cacheset1 update
# prepCache "set1"

### CACHE 1 datasets with above ensembl genomes except mouse strains. ~ 5.2 db size

./biobtree -d hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -tax 9606,10090,4932,3702,7227,6239,562,511145,83333,7955,9031,10116 -x --skip-ensembl -idx cacheset1 update
./biobtree -tax 9606,4932,3702,7227,6239,562,511145,83333,7955,9031,10116 -keep --ensembl-orthologs -idx cacheset12 update
./biobtree --genome mus_musculus -keep --ensembl-orthologs -idx cacheset13 update
prepCache "set1" "5600000000"

### CACHE 2 datasets with ensembl human and all mouse strains genomes
./biobtree -d hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -tax 9606,10090 -x -idx cacheset2 update
prepCache "set2" "4100000000"

### CACHE 3 datasets with no esembl and full uniprot ~ 3.2 db size
./biobtree -d hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -x -idx cacheset3 update
prepCache "set3" "3600000000"


clearConfs
