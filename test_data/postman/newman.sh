#!/bin/bash
set -e

# TODO run fully with builtins

rm -rf UseCases*.json

# ideally this newman  should run against all_data 
# but it can be also partially run with builtins 1 and 4 with checking each group and ignoring mix_4all,gene_4all
newman run --environment biobtree.postman_environment.json biobtree_usecases.postman_collection.json > newman_result.json

# these are for generating usecase in the web interface it use newman test output
# in order to run these successfully newman test must be passed
go run main.go -db UseCases1 -cat "mix,gene,protein,chembl,taxonomy"
go run main.go -db UseCases3 -cat "mix,protein,taxonomy"
go run main.go -db UseCases4 -cat "mix,protein,chembl,taxonomy"
go run main.go -db UseCases  -cat "mix,mix_4all,gene_4all,gene,protein,chembl,taxonomy"
rm newman_result.json