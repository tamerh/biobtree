#!/bin/bash
# Full production run: core node datasets + stub-nodes, all high-value edge
# datasets (incl. string_interaction PPI, clinvar, rnacentral, similarity),
# gzip output. Defers dbsnp (769M) per scope decision. Reads index files only.
set -e
PY=/data/miniconda3/envs/biobtree/bin/python
IDX=/data2/out_prod_v5/main/index
O=out/kg/full
mkdir -p "$O"

# Core node datasets (with names); stubs cover compounds/variants/ncRNA/etc.
# Ontologies (disease/phenotype incl. cross-species uPheno family) materialized
# so their subclass_of/close_match edges (step 4) don't dangle.
NODE_DS=hgnc,ensembl,uniprot,transcript,mondo,doid,efo,orphanet,mim,hpo,mp,upheno,zp,xpo,wbphenotype,fypo,oba,chebi,chembl_molecule,hmdb,lipidmaps,swisslipids,drugbank,reactome,msigdb,uberon,cl,cellosaurus,taxonomy,interpro,corum
# Direct edges (incl. big high-value forwards: clinvar, rnacentral)
EDGE_DS=ensembl,uniprot,reactome,msigdb,cellosaurus,hmdb,swisslipids,chembl_molecule,orphanet,transcript,clinvar,rnacentral
# Reified (incl. string_interaction PPI + similarity stars + bioactivity)
REIFIED_DS=intact,string_interaction,chembl_activity,pubchem_activity,bgee,depmap_dependency,fantom5_gene,diamond_similarity,esm2_similarity,gwas

echo "### 1/7 nodes (peak-mem) $(date +%T)"
/usr/bin/time -v $PY -m tools.kg_export nodes --index-dir $IDX --datasets $NODE_DS \
  --out $O/nodes_core.tsv.gz --id-map $O/id_map.tsv.gz --stats $O/nodes.stats.json \
  2> $O/nodes.time.txt || true
grep -E "Maximum resident|Elapsed \(wall" $O/nodes.time.txt || true

echo "### 2/7 GO $(date +%T)"
$PY -m tools.kg_export go --index-dir $IDX --id-map $O/id_map.tsv.gz \
  --nodes-out $O/go_nodes.tsv.gz --edges-out $O/go_edges.tsv.gz --stats $O/go.stats.json --sources uniprot,ensembl

echo "### 3/7 RefSeq (transcript/protein/ncRNA nodes + edges) $(date +%T)"
$PY -m tools.kg_export refseq --index-dir $IDX --id-map $O/id_map.tsv.gz \
  --nodes-out $O/refseq_nodes.tsv.gz --edges-out $O/refseq_edges.tsv.gz --stats $O/refseq.stats.json

echo "### 4/7 ontology (subclass_of + cross-ontology close_match) $(date +%T)"
$PY -m tools.kg_export ontology --index-dir $IDX \
  --out $O/ontology_edges.tsv.gz --stats $O/ontology.stats.json

echo "### 5/7 direct edges $(date +%T)"
$PY -m tools.kg_export edges --index-dir $IDX --id-map $O/id_map.tsv.gz --datasets $EDGE_DS \
  --out $O/edges_direct.tsv.gz --stats $O/edges.stats.json

echo "### 6/7 reified edges $(date +%T)"
$PY -m tools.kg_export reified --index-dir $IDX --id-map $O/id_map.tsv.gz --datasets $REIFIED_DS \
  --out $O/edges_reified.tsv.gz --stats $O/reified.stats.json

echo "### 7/7 assemble (stub-nodes + gzip) $(date +%T)"
$PY -m tools.kg_export assemble \
  --nodes $O/nodes_core.tsv.gz,$O/go_nodes.tsv.gz,$O/refseq_nodes.tsv.gz \
  --edges $O/edges_direct.tsv.gz,$O/edges_reified.tsv.gz,$O/go_edges.tsv.gz,$O/refseq_edges.tsv.gz,$O/ontology_edges.tsv.gz \
  --out-dir $O/dump --data-version out_prod_v5_full --stub-nodes --gzip

echo "### DONE $(date +%T)"; df -h /data | tail -1
