// Atlas-style validation/demo queries for the subgraph in Neo4j. Each mirrors a
// BioBTree API capability; running them validates the import AND surfaces gaps (a
// query that *should* return rows but doesn't = a layer we dropped/mis-modeled).
// Run: docker exec -i biobtree-kg cypher-shell -u neo4j -p biobtreekg < neo4j_queries.cypher

// --- /ws/entry : lookup by id (any namespace via equivalent_identifiers) ----------
MATCH (n {id:'HGNC:1100'}) RETURN n.id, n.name, labels(n), n.equivalent_identifiers;
MATCH (n) WHERE 'NCBIGene:672' IN n.equivalent_identifiers RETURN n.id, n.name;

// --- /ws/search : keyword over name + curated synonyms ----------------------------
CALL db.index.fulltext.queryNodes('node_search','BRCA1') YIELD node, score
RETURN node.id, node.name, score LIMIT 5;
CALL db.index.fulltext.queryNodes('node_search','breast cancer') YIELD node
RETURN node.id, node.name, labels(node) LIMIT 5;

// --- /ws/filter : CEL-over-attributes -> Cypher WHERE -----------------------------
MATCH (n:Gene) WHERE n.entrez_type = 'protein-coding' RETURN count(n);
MATCH (n:Gene) WHERE n.gnomad_pli > 0.9 RETURN n.id, n.name, n.gnomad_pli ORDER BY n.gnomad_pli DESC LIMIT 10;
MATCH (n:SmallMolecule) WHERE n.chebi_formula IS NOT NULL RETURN count(n);

// --- /ws/map : multi-hop traversal ------------------------------------------------
// gene -> disease (1 hop)
MATCH (g:Gene {name:'BRCA1'})-[r]-(d:Disease) RETURN type(r), d.name LIMIT 10;
// drug -> target protein -> gene  (drug pharmacology, 2 hops)
MATCH (drug)-[:affects]->(p:Protein)<-[:has_gene_product]-(g:Gene {name:'EGFR'})
RETURN drug.id, drug.name LIMIT 10;
// disease -> gene -> pathway (3-hop join)
MATCH (d:Disease {name:'breast cancer'})-[]-(g:Gene)-[:participates_in]->(pw:Pathway)
RETURN DISTINCT g.name, pw.name LIMIT 15;

// --- evidence + qualifiers on edges (BioBTree edge-level data) ---------------------
MATCH (g)-[r]->(go) WHERE type(r) IN ['enables','actively_involved_in','located_in'] AND r.has_evidence IS NOT NULL
RETURN g.name, type(r), go.name, r.has_evidence LIMIT 5;
MATCH (a)-[r:related_to]->(b) WHERE r.qualifiers CONTAINS 'relationship='
RETURN a.name, b.name, r.qualifiers LIMIT 5;

// --- SHOWCASE : the giant layers on famous examples -------------------------------
// dbSNP variants of TP53 (showcase)
MATCH (v:SequenceVariant)-[:is_sequence_variant_of]->(g:Gene {name:'TP53'}) RETURN count(v) AS tp53_variants;
// EGFR's inhibitors / bioactivity compounds (pubchem/chembl showcase)
MATCH (c)-[:interacts_with]->(p:Protein)<-[:has_gene_product]-(g:Gene {name:'EGFR'})
RETURN DISTINCT c.id, c.name LIMIT 15;

// --- coverage probe : every node label + relationship type present -----------------
MATCH (n) RETURN labels(n)[0] AS label, count(*) AS n ORDER BY n DESC;
MATCH ()-[r]->() RETURN type(r) AS predicate, count(*) AS n ORDER BY n DESC;
