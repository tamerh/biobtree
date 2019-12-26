#!/bin/bash
set -e

rm -rf UseCases*.json

newman run --environment biobtree.postman_environment.json biobtree_usecases.postman_collection.json > newman_result.json
go run main.go -db UseCases1 -cat "mix,gene,protein,chembl,taxonomy"
go run main.go -db UseCases3 -cat "mix,protein,taxonomy"
go run main.go -db UseCases4 -cat "mix,protein,chembl,taxonomy"
go run main.go -db UseCases  -cat "mix,mix_4all,gene_4all,gene,protein,chembl,taxonomy"
rm newman_result.json

# TODO run against builtindbs