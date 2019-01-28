# Biobtree

Biobtree is a bioinformatics tool to search, map and visualize bioinformatics identifiers and special keywords. Check related [article](https://www.biorxiv.org/content/early/2019/01/16/520841.1) for more detail and cite.

#### Usage

After installing latest version of Biobtree, from command line 3 main phases needs to be followed. For any problem
or question feel free to create a issue.

#### 1- Update Phase

```sh
# this command updates the default datasets which are 
# uniprot_reviewed,taxonomy,hgnc,chebi,interpro,my_data,literature_mappings,hmdb
$ biobtree update 
```

```sh
# For only specific dataset d parameters is passed like following, 
$ biobtree --d hgnc update 
```

```sh
# From selected dataset if you are intrested only certain target datasets specify t paramater e.g,
# This command updates hgnc dataset with only related uniprot and pubmed identifers. 
$ biobtree --d hgnc --t UniProtKB,PubMed  update 
```

```sh
# you can use 2 alternatives Uniprot ftp mirror with f parmater
$ biobtree --f USA update
$ biobtree --f Switzerland update 
```

#### 2- Generate Phase

```sh
# to generate LMDB database based on updated data following command used
$ biobtree generate
```

#### 3- Web Phase

```sh
# to start using web services and web interface following command used
$ biobtree web
```


### License
Biobtree is an open source project with [BSD-3 license](https://opensource.org/licenses/BSD-3-Clause). This means you
can freely use and redistribute its source code and binary. You should only include 3 license files when using it.
