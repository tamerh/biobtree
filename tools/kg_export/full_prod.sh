#!/bin/bash
# Full production run: core node datasets + stub-nodes, all high-value edge
# datasets (incl. string_interaction PPI, clinvar, rnacentral, similarity),
# gzip output. Reads index files only.
#
# dbSNP is a FIRST-CLASS layer (WITH_DBSNP=1 by default): a separate ~118 GB-gz
# federation (~1.1B variants) extracted by tools/dbsnp_py/extract.py -- zcat
# decompresses, a Python multiprocessing pool parses in parallel and shards KGX
# output (~1 hr full pass on a quiet box). Set WITH_DBSNP=0 to skip it for the
# faster/smaller runs used during alignment + assemble work.
#
# The assemble step is memory-flat (sort-based merge/stub + --validate-mode
# streaming), so it survives the billion-scale dbSNP layer. Disk for sort spill goes
# next to $O (on /data), so ensure scratch space before a full run.
set -e
PY=/data/miniconda3/envs/biobtree/bin/python
IDX=/data2/out_prod_v5/main/index
DBSNP_GZ=${DBSNP_GZ:-/data2/out_prod_v5/dbsnp/index/dbsnp_sorted.*.index.gz}
WITH_DBSNP=${WITH_DBSNP:-1}
DBSNP_WORKERS=${DBSNP_WORKERS:-12}
# Variant-effect PREDICTION layer (spliceai + alphamissense). OPT-IN: variant-scale
# (~tens of millions of predicted variant->gene edges), for the full-stats run only.
WITH_PREDICTIONS=${WITH_PREDICTIONS:-0}
O=out/kg/full
mkdir -p "$O"

# Core node datasets (with names); stubs cover compounds/variants/ncRNA/etc.
# Ontologies (disease/phenotype incl. cross-species uPheno family) materialized
# so their subclass_of/close_match edges (step 4) don't dangle.
NODE_DS=hgnc,ensembl,uniprot,transcript,mondo,doid,efo,orphanet,mim,hpo,mp,upheno,zp,xpo,wbphenotype,fypo,oba,chebi,chembl_molecule,hmdb,lipidmaps,swisslipids,drugbank,reactome,msigdb,uberon,cl,cellosaurus,taxonomy,interpro,corum,mgi,rgd,sgd,zfin,flybase,wormbase,civic_variant,ctd,brenda,jaspar,mirdb,xenbase,faers_reaction,fantom5_enhancer,pharmgkb_pathway,literature_mappings,chembl_document,patent,chembl_cell_line,exon,cds,ufeature,drugcentral
# Direct edges (incl. big high-value forwards: clinvar, rnacentral)
EDGE_DS=ensembl,uniprot,reactome,msigdb,cellosaurus,hmdb,swisslipids,chembl_molecule,orphanet,transcript,clinvar,rnacentral,jaspar,chembl_document,chembl_cell_line,entrez,ufeature,drugcentral
# Reified (incl. string_interaction PPI + similarity stars + bioactivity)
REIFIED_DS=intact,string_interaction,chembl_activity,pubchem_activity,bgee,depmap_dependency,fantom5_gene,diamond_similarity,esm2_similarity,gwas,alliance_disease,clinical_trials,cellxgene_celltype,ctd_gene_interaction,civic_variant,civic_evidence,civic_assertion,mirdb,generif,alliance_phenotype,faers,panelapp_gene,ctd_disease_association,fantom5_enhancer,pharmgkb_pathway,pharmgkb_guideline,patent

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

echo "### 4b/7 MeSH disease subset (Disease nodes + mondo close_match) $(date +%T)"
$PY -m tools.kg_export mesh --index-dir $IDX \
  --nodes-out $O/mesh_nodes.tsv.gz --edges-out $O/mesh_edges.tsv.gz --stats $O/mesh.stats.json

echo "### 5/7 direct edges $(date +%T)"
$PY -m tools.kg_export edges --index-dir $IDX --id-map $O/id_map.tsv.gz --datasets $EDGE_DS \
  --out $O/edges_direct.tsv.gz --stats $O/edges.stats.json

echo "### 6/7 reified edges $(date +%T)"
$PY -m tools.kg_export reified --index-dir $IDX --id-map $O/id_map.tsv.gz --datasets $REIFIED_DS \
  --out $O/edges_reified.tsv.gz --stats $O/reified.stats.json

PRED_EDGES=""
if [ "$WITH_PREDICTIONS" = "1" ]; then
  echo "### 6c/7 variant-effect predictions (OPT-IN; spliceai + alphamissense) $(date +%T)"
  $PY -m tools.kg_export reified --index-dir $IDX --id-map $O/id_map.tsv.gz \
    --datasets spliceai,alphamissense --out $O/edges_predictions.tsv.gz --stats $O/predictions.stats.json
  PRED_EDGES=",$O/edges_predictions.tsv.gz"
fi

DBSNP_NODES=""; DBSNP_EDGES=""; DBSNP_ATTRS=""
if [ "$WITH_DBSNP" = "1" ]; then
  echo "### 6b/7 dbSNP federation (~1.1B variants -> gene+transcript edges, rich attrs) $(date +%T)"
  mkdir -p $O/dbsnp
  zcat $DBSNP_GZ | $PY tools/dbsnp_py/extract.py --workers $DBSNP_WORKERS \
    --id-map $O/id_map.tsv.gz --out $O/dbsnp
  DBSNP_NODES=",$(ls $O/dbsnp/dbsnp_nodes.*.tsv.gz | paste -sd,)"
  DBSNP_EDGES=",$(ls $O/dbsnp/dbsnp_edges.*.tsv.gz | paste -sd,)"
  DBSNP_ATTRS=",$(ls $O/dbsnp/dbsnp_attrs.*.tsv.gz | paste -sd,)"
fi

echo "### 6d/7 node attributes (numeric/value scalars: gnomad/depmap/alphafold/alphamissense_transcript) $(date +%T)"
$PY -m tools.kg_export attributes --index-dir $IDX --id-map $O/id_map.tsv.gz \
  --out $O/node_attrs.tsv.gz --stats $O/node_attrs.stats.json

echo "### 6e/7 structure layer (exon/cds/ufeature has_part + cds translates_to + ECO evidence) $(date +%T)"
$PY -m tools.kg_export structure --index-dir $IDX --id-map $O/id_map.tsv.gz \
  --edges-out $O/structure_edges.tsv.gz --attrs-out $O/structure_attrs.tsv.gz \
  --stats $O/structure.stats.json

echo "### 6f/7 node attributes (entry attrs -> node props; makes /ws/filter queryable) $(date +%T)"
$PY -m tools.kg_export nodeattrs --index-dir $IDX --id-map $O/id_map.tsv.gz \
  --out $O/node_entry_attrs.tsv.gz --stats $O/node_entry_attrs.stats.json

echo "### 7/7 assemble (stub-nodes + node-attributes + gzip) $(date +%T)"
$PY -m tools.kg_export assemble \
  --nodes $O/nodes_core.tsv.gz,$O/go_nodes.tsv.gz,$O/refseq_nodes.tsv.gz,$O/mesh_nodes.tsv.gz$DBSNP_NODES \
  --edges $O/edges_direct.tsv.gz,$O/edges_reified.tsv.gz,$O/go_edges.tsv.gz,$O/refseq_edges.tsv.gz,$O/ontology_edges.tsv.gz,$O/mesh_edges.tsv.gz,$O/structure_edges.tsv.gz$PRED_EDGES$DBSNP_EDGES \
  --node-attributes $O/node_attrs.tsv.gz,$O/structure_attrs.tsv.gz,$O/node_entry_attrs.tsv.gz$DBSNP_ATTRS \
  --out-dir $O/dump --data-version out_prod_v5_full --stub-nodes --gzip \
  --validate-mode streaming

echo "### 8/8 published subgraph (human-scoped + per-source capped projection) $(date +%T)"
ATTRS=$O/node_attrs.tsv.gz,$O/structure_attrs.tsv.gz,$O/node_entry_attrs.tsv.gz$DBSNP_ATTRS
$PY -m tools.kg_export subgraph \
  --nodes $O/dump/nodes.tsv.gz --edges $O/dump/edges.tsv.gz --config mappings/subgraph.yaml \
  --out-nodes $O/sub/nodes.tsv.gz --out-edges $O/sub/edges.tsv.gz \
  --full-manifest $O/dump/manifest.json --stats $O/sub/subgraph.stats.json
$PY -m tools.kg_export assemble \
  --nodes $O/sub/nodes.tsv.gz --edges $O/sub/edges.tsv.gz --node-attributes $ATTRS \
  --out-dir $O/sub/dump --data-version out_prod_v5_subgraph --stub-nodes --gzip \
  --validate-mode full

echo "### DONE $(date +%T)"; df -h /data | tail -1
