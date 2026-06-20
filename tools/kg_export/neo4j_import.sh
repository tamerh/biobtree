#!/bin/bash
# Bulk-import the subgraph KGX dump into the Neo4j docker container (biobtree-kg,
# neo4j:5). The container's /import is bind-mounted to out/kg/sample, so the CSVs go
# there. neo4j-admin bulk import (offline) is used -- right tool at subgraph scale.
#
#   bash tools/kg_export/neo4j_import.sh [DUMP_DIR]      # default out/kg/full/sub/dump
set -e
PY=/data/miniconda3/envs/biobtree/bin/python
DUMP=${1:-out/kg/full/sub/dump}
IMPORT=out/kg/sample
CONTAINER=biobtree-kg
PASS=biobtreekg

echo "### 1/4 convert KGX -> neo4j CSVs  $(date +%T)"
$PY -m tools.kg_export.neo4j_import \
  --nodes "$DUMP/nodes.jsonl.gz" --edges "$DUMP/edges.tsv.gz" --out-dir "$IMPORT"

echo "### 2/4 neo4j-admin bulk import (offline; reuses the container's volumes)  $(date +%T)"
docker stop $CONTAINER 2>/dev/null || true
docker run --rm --volumes-from $CONTAINER neo4j:5 \
  neo4j-admin database import full neo4j \
  --nodes=/import/neo4j_nodes.csv --relationships=/import/neo4j_edges.csv \
  --array-delimiter=';' --skip-bad-relationships=true --skip-duplicate-nodes=true \
  --overwrite-destination=true

echo "### 3/4 start neo4j + indexes  $(date +%T)"
docker start $CONTAINER
for i in $(seq 1 30); do
  docker exec $CONTAINER cypher-shell -u neo4j -p $PASS "RETURN 1;" >/dev/null 2>&1 && break
  sleep 3
done
# id lookup (every node carries the NamedThing label) + full-text search over name+synonym
docker exec $CONTAINER cypher-shell -u neo4j -p $PASS \
  "CREATE INDEX node_id IF NOT EXISTS FOR (n:NamedThing) ON (n.id);" || true
docker exec $CONTAINER cypher-shell -u neo4j -p $PASS \
  "CREATE FULLTEXT INDEX node_search IF NOT EXISTS FOR (n:NamedThing) ON EACH [n.name, n.synonym];" || true

echo "### 4/4 counts  $(date +%T)"
docker exec $CONTAINER cypher-shell -u neo4j -p $PASS \
  "MATCH (n) RETURN count(n) AS nodes;"
docker exec $CONTAINER cypher-shell -u neo4j -p $PASS \
  "MATCH ()-[r]->() RETURN count(r) AS rels;"
echo "### DONE -- browser at http://localhost:7474 (neo4j/$PASS)  $(date +%T)"
