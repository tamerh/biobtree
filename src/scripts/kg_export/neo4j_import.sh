#!/bin/bash
# Bulk-import the subgraph KGX dump into a Neo4j 5 container.
#
# The DB store and the import CSVs both live on /data (the big disk), NOT on the
# Docker root volume -- a 30M-node subgraph store is ~40GB and the root fs is far
# too small (an out-of-space mid-import surfaces as a misleading "Error in input
# data"). We therefore bind-mount $DB:/data into a fresh container.
#
#   bash src/scripts/kg_export/neo4j_import.sh [DUMP_DIR]      # default out/kg/full/sub/dump
set -e
PY=/data/miniconda3/envs/biobtree/bin/python
export PYTHONPATH="src/scripts:${PYTHONPATH:-}"
DUMP=${1:-out/kg/full/sub/dump}
IMPORT=$(cd "$(dirname "$0")/../.." && pwd)/out/kg/sample   # CSVs (absolute; on /data)
DB=$(cd "$(dirname "$0")/../.." && pwd)/out/kg/neo4jdb       # DB store (absolute; on /data)
CONTAINER=biobtree-kg
PASS=biobtreekg
mkdir -p "$IMPORT" "$DB"

echo "### 1/4 convert KGX -> neo4j CSVs  $(date +%T)"
$PY -m kg_export.neo4j_import \
  --nodes "$DUMP/nodes.jsonl.gz" --edges "$DUMP/edges.tsv.gz" --out-dir "$IMPORT"

echo "### 2/4 neo4j-admin bulk import (offline; DB store on /data)  $(date +%T)"
docker rm -f $CONTAINER 2>/dev/null || true
docker run --rm -v "$DB":/data -v "$IMPORT":/import neo4j:5 \
  neo4j-admin database import full neo4j \
  --nodes=/import/neo4j_nodes.csv --relationships=/import/neo4j_edges.csv \
  --array-delimiter=';' --skip-bad-relationships=true --skip-duplicate-nodes=true \
  --overwrite-destination=true

echo "### 3/4 start neo4j + indexes  $(date +%T)"
docker run -d --name $CONTAINER -v "$DB":/data \
  -e NEO4J_AUTH=neo4j/$PASS \
  -e NEO4J_server_memory_heap_max__size=6G \
  -e NEO4J_server_memory_pagecache_size=6G \
  -p 7474:7474 -p 7687:7687 neo4j:5
for i in $(seq 1 40); do
  docker exec $CONTAINER cypher-shell -u neo4j -p $PASS "RETURN 1;" >/dev/null 2>&1 && break
  sleep 3
done
# id lookup (every node carries the NamedThing label) + full-text search over name+synonym
docker exec $CONTAINER cypher-shell -u neo4j -p $PASS \
  "CREATE INDEX node_id IF NOT EXISTS FOR (n:NamedThing) ON (n.id);" || true
docker exec $CONTAINER cypher-shell -u neo4j -p $PASS \
  "CREATE FULLTEXT INDEX node_search IF NOT EXISTS FOR (n:NamedThing) ON EACH [n.name, n.synonym];" || true
docker exec $CONTAINER cypher-shell -u neo4j -p $PASS "CALL db.awaitIndexes(600);" || true

echo "### 4/4 counts  $(date +%T)"
docker exec $CONTAINER cypher-shell -u neo4j -p $PASS \
  "MATCH (n) RETURN count(n) AS nodes;"
docker exec $CONTAINER cypher-shell -u neo4j -p $PASS \
  "MATCH ()-[r]->() RETURN count(r) AS rels;"
echo "### DONE -- browser at http://localhost:7474 (neo4j/$PASS)  $(date +%T)"
