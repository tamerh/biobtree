#!/bin/bash

set -e 

source ./scripts/data/common.sh

if [[ -z $1 ]]; then
    echo " Version paramater is requried."
    exit 1
fi

VERSION=$1


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

    ./biobtree --keep --lmdbsize ${2} generate

    tar -czvf biobtree-conf-${VERSION}-${1}d.tar.gz out/

    rm -rf out

}

#### genomes note
#homo_sapiens 9606 --> ensembl
#danio_rerio 7955 zebrafish --> ensembl
# gallus_gallus 9031 chicken --> ensembl
#mus_musculus 10090 --> ensembl
# Rattus norvegicus 10116 ---> ensembl
#saccharomyces_cerevisiae 4932--> ensembl,ensembl_fungi
#arabidopsis_thaliana 3702--> ensembl_plants
#drosophila_melanogaster 7227 --> ensembl,ensembl_metazoa
#caenorhabditis_elegans 6239 --> ensembl,ensembl_metazoa
#Escherichia coli 562 --> ensembl_bacteria
#Escherichia coli str. K-12 substr. MG1655 511145 --> ensembl_bacteria
#Escherichia coli K-12 83333 --> ensembl_bacteria
# taxids 9606,10090,4932,3702,7227,6239,562,511145,83333,7955


### CACHE 0 small demo dataset for biobtreeR ~ 36 MB db size
./biobtree -d go,hgnc,uniprot,ensembl,interpro --uniprot.file test_data/RdemoData/uniprot_sample.xml.gz --interpro.file test_data/RdemoData/interpro_sample.xml.gz --ensembl.file test_data/RdemoData/human.chr.21.gff3.gz --go.file test_data/RdemoData/go_sample.owl -tax 9606 -idx demo update
prepCache "demo" "36000000"

### CACHE 1 datasets with above ensembl genomes except mouse strains. ~ 5.2 db size
./biobtree --d hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -tax 9606,10090,4932,3702,7227,6239,562,511145,83333,7955,9031,10116 --skip-ensembl -idx cacheset1 update
./biobtree --d ensembl --tax 9606,4932,7227,6239,7955,9031,10116 -keep --ensembl-orthologs -idx cacheset12 update
./biobtree --d ensembl_bacteria --tax 562,511145,83333 --keep --ensembl-orthologs -idx cacheset13 update
./biobtree --d ensembl_plants --tax 3702 --keep --ensembl-orthologs -idx cacheset14 update
./biobtree --d ensembl --genome mus_musculus -keep --ensembl-orthologs -idx cacheset15 update

prepCache "set1" "5600000000"

### CACHE 2 datasets with ensembl human and all mouse strains genomes
./biobtree -d hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -tax 9606,10090 -idx cacheset2 update
prepCache "set2" "4100000000"


### CACHE 3 datasets with no esembl and full uniprot ~ 3.2 db size
./biobtree -d hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -idx cacheset3 update
prepCache "set3" "3600000000"

