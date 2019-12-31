# Biobtree

<!--[![Build Status](https://dev.azure.com/biobtree/biobtree/_apis/build/status/tamerh.biobtree?branchName=master)](https://dev.azure.com/biobtree/biobtree/_build/latest?definitionId=1&branchName=master) -->

Biobtree is a bioinformatics tool which allows mapping the bioinformatics datasets
via identifiers and special keywors with simple or advance chain query capability.

## Features

* **Datasets** - supports wide datasets such as `Ensembl` `Uniprot` `ChEMBL` `HMDB` `Taxonomy` `GO` `EFO` `HGNC` `ECO` `Uniparc` `Uniref`  with tens of more via cross references
by retrieving latest data from providers

* **MapReduce** - processes small or large datasets based on users selection and build B+ tree based uniform local database via specialized MapReduce based tecnique with efficient storage usage

* **Query** - Allow simple or advance chain queries between datasets with intiutive syntax which allows writing RDF or graph like queries

* **Genome** - supports querying full Ensembl genomes coordinates with `transcript`, `CDS`, `exon`, `utr` with several attiributes, mapped datasets and identifiers such as `ortholog` ,`paralog` or probe identifers belongs `Affymetrix` or `Illumina`

* **Protein** - Uniprot proteins including protein features with variations and mapped datasets.

* **Chemistry** - `ChEMBL` and `HMDB` datasets supported for chemistry, disease and drug releated analaysis

* **Taxonomy & Ontologies** - `Taxonomy` `GO` `EFO` `ECO` data with mapping to other datasets and child and parent query capability

* **Web UI** - Web interface for easy explorations and examples

* **Web Services** - REST or gRPC services

* **R & Python** - [Bioconductor R](https://github.com/tamerh/biobtreeR) and [Python](https://github.com/tamerh/biobtreePy) wrapper packages to use from existing pipelines easier with built-in databases

## Demo

Demo instance of biobtree web interface which covers all the datasets with query examples

https://www.ebi.ac.uk/~tgur/biobtree/ 

### Usage

First install [latest](https://github.com/tamerh/biobtree/releases/latest) biobtree executable available for Windows, Mac or Linux. Then extract the downloaded file to a new folder and open a terminal in this new folder directory and starts the biobtree. Alternatively R and Python based [biobtreeR](https://github.com/tamerh/biobtreeR) and [biobtreePy](https://github.com/tamerh/biobtreePy) wrapper packages can be used instead of using the executable directly for eaiser integration.

#### Starting biobtree with target datasets or genomes
```sh

# build ensembl genomes by tax id with uniprot&taxonomy datasets
biobtree  --tax 595,984254 -d "uniprot,taxonomy" build 

# build datasets only 
biobtree -d "uniprot,taxonomy,hgnc" build 
biobtree -d "hgnc,chembl,hmdb" build

# once data is built start web for using ws and ui
biobtree web

# to see all options and datasets use help
biobtree help

```

#### Starting biobtree with built-in databases

```sh
# 4 built-in database provided with commonly studied datasets and organism genomes
# Check following file for each database content https://github.com/tamerh/biobtreeR/blob/master/R/buildData.R

biobtree --pre-built 1 install
biobtree web
```

### Web service endpoints
```ruby
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

<!-- ### Integrating your dataset

User data can be integrated to biobtree. Since biobtree has capability to process large datasets, this feature creates an alternative for  mapping related data to be indexed with biobtree. Data should be gzipped and in an xml format compliant with UniProt xml schema [definition](ftp://ftp.uniprot.org/pub/databases/uniprot/current_release/knowledgebase/complete/uniprot.xsd). Once data has been prepared, file location needs to be configured in biobtree configuration file which is located at `conf/source.dataset.json`. After these configuration dataset used similarly with other dataset like 

```sh
biobtree -d "+my_data" start
``` -->

### Article
https://f1000research.com/articles/8-145/v2 (Currently being updated)

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