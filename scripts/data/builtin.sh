#!/bin/bash

set -e 

source ./scripts/data/common.sh

if [[ -z $1 ]]; then
    echo " Version paramater is requried."
    exit 1
fi

VERSION=$1


mkdir -p builtins

prepCache(){

    cd out/index

    mkdir -p tmp
    mv *.gz tmp/
    cd tmp
    gunzip *.gz

    LC_ALL=C $GNUSORT -m -u * > ../merged.${1}.index
    cd ..
    rm -rf tmp
    gzip merged.${1}.index

    cd ../..

    tar -czvf builtins/biobtree-conf-${VERSION}-${1}r.tar.gz out/

    ./biobtree --lmdbsize ${2} generate

    tar -czvf builtins/biobtree-conf-${VERSION}-${1}.tar.gz out/

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


# ### CACHE 0 small demo dataset for biobtreeR ~ 36 MB db size
# ./biobtree -d go,hgnc,uniprot,ensembl,interpro --uniprot.file test_data/RdemoData/uniprot_sample.xml.gz --interpro.file test_data/RdemoData/interpro_sample.xml.gz --ensembl.file test_data/RdemoData/human.chr.21.gff3.gz --go.file test_data/RdemoData/go_sample.owl -tax 9606 -idx demo update
# prepCache "demo" "36000000"

# ### CACHE 1 datasets with above ensembl genomes except mouse strains. ~ 5.2 db size
# ./biobtree --d hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -tax 9606,10090,4932,3702,7227,6239,562,511145,83333,7955,9031,10116 --skip-ensembl -idx builtinset1 update
# ./biobtree --d ensembl --tax 9606,4932,7227,6239,7955,9031,10116 -keep --otaxids 9606,10090,4932,3702,7227,6239,562,511145,83333,7955,9031,10116 -idx builtinset12 update
# ./biobtree --d ensembl_bacteria --genome "escherichia_coli,escherichia_coli_str_k_12_substr_mg1655,escherichia_coli_k_12" --keep --otaxids 9606,10090,4932,3702,7227,6239,562,511145,83333,7955,9031,10116 -idx builtinset13 update
# ./biobtree --d ensembl_plants --tax 3702 --keep --otaxids 9606,10090,4932,3702,7227,6239,562,511145,83333,7955,9031,10116 -idx builtinset14 update
# ./biobtree --d ensembl --genome mus_musculus -keep --otaxids 9606,10090,4932,3702,7227,6239,562,511145,83333,7955,9031,10116  -idx builtinset15 update
# prepCache "set1" "4600000000"

# ### CACHE 2 datasets with ensembl human and all mouse strains genomes ~ 4 db size
# ./biobtree -d hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -tax 9606,10090 -idx builtinset2 update
# prepCache "set2" "4100000000"

# ### CACHE 3 datasets with no esembl and full uniprot ~ 3.2 db size
# ./biobtree -d hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -idx builtinset3 update
# prepCache "set3" "3600000000"

### CACHE 4 datasets with no esembl and full uniprot and chembl ~ 11.5 db size
./biobtree -d chembl,hgnc,hmdb,uniprot,taxonomy,go,efo,eco,chebi,interpro -idx builtinset4 update
prepCache "set4" "12000000000"

### set4 
echo "All done."