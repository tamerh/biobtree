#!/bin/bash
# Bounded production run: core node datasets (<=400M each; excludes the giants
# pubchem/entrez/rnacentral/clinvar/dbsnp) to produce a real VALID-ish KGX dump
# and measure peak memory of the in-RAM nodes pass (the B1 risk).
set -e
PY=/data/miniconda3/envs/biobtree/bin/python
export PYTHONPATH="src/scripts:${PYTHONPATH:-}"
IDX=/data2/out_prod_v5/main/index
O=out/kg/prod
mkdir -p "$O"

NODE_DS=hgnc,ensembl,uniprot,transcript,mondo,efo,orphanet,hpo,chebi,chembl_molecule,hmdb,lipidmaps,swisslipids,drugbank,reactome,msigdb,uberon,cl,cellosaurus,taxonomy,interpro,corum
EDGE_DS=ensembl,uniprot,reactome,msigdb,cellosaurus,hmdb,swisslipids,chembl_molecule,orphanet,transcript
REIFIED_DS=intact,chembl_activity,bgee,depmap_dependency,fantom5_gene

echo "### 1/5 nodes (peak-mem measured) $(date +%T)"
/usr/bin/time -v $PY -m kg_export nodes --index-dir $IDX --datasets $NODE_DS \
  --out $O/nodes_core.tsv --id-map $O/id_map.tsv --stats $O/nodes.stats.json \
  2> $O/nodes.time.txt || true
grep -E "Maximum resident|Elapsed" $O/nodes.time.txt || true
tail -3 $O/nodes.time.txt 2>/dev/null || true

echo "### 2/5 GO nodes+edges $(date +%T)"
$PY -m kg_export go --index-dir $IDX --id-map $O/id_map.tsv \
  --nodes-out $O/go_nodes.tsv --edges-out $O/go_edges.tsv --stats $O/go.stats.json --sources uniprot,ensembl

echo "### 3/5 direct edges $(date +%T)"
$PY -m kg_export edges --index-dir $IDX --id-map $O/id_map.tsv --datasets $EDGE_DS \
  --out $O/edges_direct.tsv --stats $O/edges.stats.json

echo "### 4/5 reified edges $(date +%T)"
$PY -m kg_export reified --index-dir $IDX --id-map $O/id_map.tsv --datasets $REIFIED_DS \
  --out $O/edges_reified.tsv --stats $O/reified.stats.json

echo "### 5/5 assemble $(date +%T)"
$PY -m kg_export assemble \
  --nodes $O/nodes_core.tsv,$O/go_nodes.tsv \
  --edges $O/edges_direct.tsv,$O/edges_reified.tsv,$O/go_edges.tsv \
  --out-dir $O/dump --data-version out_prod_v5_bounded

echo "### DONE $(date +%T)"
