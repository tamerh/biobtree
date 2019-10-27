# Biobtree

<!--[![Build Status](https://dev.azure.com/biobtree/biobtree/_apis/build/status/tamerh.biobtree?branchName=master)](https://dev.azure.com/biobtree/biobtree/_build/latest?definitionId=1&branchName=master) -->

Biobtree is a bioinformatics tool which process large datasets effectively and provide uniform search and mapping functionalities with web interface and web services for genomic research.

https://www.ebi.ac.uk/~tgur/biobtree/ Running instance with all the datasets and example use cases


### Getting started

First install [latest](https://github.com/tamerh/biobtree/releases/latest) biobtree executable available for Windows, Mac or Linux. Then extract the downloaded file to a new folder and open a terminal in this new folder directory and starts the biobtree.

```sh
# Windows
biobtree.exe start

# Mac or Linux
./biobtree start 
```
This command fetches and process default datasets which are  `taxonomy` `ensembl(homo_sapiens)` `uniprot(reviewed)` `hgnc` `go` `eco` `efo` `chebi` `interpro`. When processing data completed biobtree web interface opens with address http://localhost:8888/ui and web services can be used.

### Starting biobtree with different datasets, species or options
```sh

# to start biobtree with previously processed datasets use web instead of start
biobtree web 

# process specific datasets only 
biobtree -d "uniprot,taxonomy,hgnc" start

# default datasets plus chembl and hmdb via + symbol 
biobtree -d "+chembl,hmdb" start

# multiple genomes seperated by comma
biobtree -s "homo_sapiens,mus_musculus" start

# ensembl genomes metazoa,fungi,plants,protists,bacteria 
biobtree -d "+ensembl_metazoa" -s "caenorhabditis_elegans,drosophila_melanogaster" start
biobtree -d "+ensembl_fungi" -s "saccharomyces_cerevisiae" start
biobtree -d "+ensembl_plants,ensembl_protists" -s "arabidopsis_thaliana,phytophthora_parasitica" start
biobtree -d "+ensembl_bacteria" -s "salmonella_enterica" start

# genomes also can be selected with comma seperated name patterns via -sp option
# following command process all the genomes which contains one of the given term in its name 
# list of selected genomes also written to a json file starts with genomes_*.json in the same directory for later reference
biobtree -d "+ensembl_bacteria" -sp "serovar_infantis,serovar_virchow" start

# to select all the genomes for ensembl or ensembl genomes use all 
biobtree -d "+ensembl_plants" -s "all" start

# to see all options and datasets use help
biobtree help

```


<!--### Using biobtree from R 

Although webservices can be used directly from any language it requires some effort using from exsiting piplelines. To address this and provide more convinient interface to biobtree, dedicated [biobtreeR](https://github.com/tamerh/biobtreeR) bioconductor R package can be used. Similar Python package will be also added. -->


### Web service endpoints
```sh
# Meta
# datasets meta informations 
localhost:8888/ws/meta

# Search 
# i is the only mandatory parameter
localhost:8888/ws/?i={terms}&s={dataset}&p={page}&f={filter}

# Mapping 
# i and m are mandatory parameters
localhost:8888/ws/map/?i={terms}&m={mapfilter_query}&s={dataset}&p={page}

# Retrieve dataset entry. Both paramters are mandatory
localhost:8888/ws/entry/?i={identifier}&s={dataset}

# Retrieve entry with filtered mapping entries. Only page parameter is optional
localhost:8888/ws/filter/?i={identifier}&s={dataset}&f={filter_datasets}&p={page}

# Retrieve entry results with page index. All the parameters are mandatory 
localhost:8888/ws/page/?i={identifier}&s={dataset}&p={page}&t={total}

```

### Integrating your dataset

User data can be integrated to biobtree. Since biobtree has capability to process large datasets, this feature creates an alternative for  mapping related data to be indexed with biobtree. Data should be gzipped and in an xml format compliant with UniProt xml schema [definition](ftp://ftp.uniprot.org/pub/databases/uniprot/current_release/knowledgebase/complete/uniprot.xsd). Once data has been prepared, file location needs to be configured in biobtree configuration file which is located at `conf/source.dataset.json`. After these configuration dataset used similarly with other dataset like 

```sh
biobtree -d "+my_data" start
```

### Article
https://f1000research.com/articles/8-145/v2

### Building source 

biobtree is written with GO for the data processing and Vue.js for the web application part. To build and the create biobtree executable install go>=1.13 and run

```sh
go build
```

To build the web application for development in the web directory run

```sh
npm install
npm run serve
```

To build the web package run

```sh
npm run build
```