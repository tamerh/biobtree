# Knowledge Graph Export

## Overview

The KG export turns BioBTree's integrated graph into a **biolink-typed, normalized
knowledge graph** in [KGX](https://github.com/biolink/kgx) format — consumable by
Neo4j, RDF/SPARQL, and the Monarch/Translator tooling, and registerable as a
`KnowledgeGraph` / `GraphProduct`.

It is a **batch export over BioBTree's own data**, not a separate dataset: it reads
the post-build sorted index files and is verified to match the served graph
count-for-count (see [Alignment](#alignment)). It does not touch the Go core,
query service, or MCP server.

**Format**: KGX TSV + JSON-Lines, gzip-optional, with a manifest.
**Tooling/dev detail**: `tools/kg_export/README.md`.

## Output

| File | Contents |
|------|----------|
| `nodes.tsv` / `nodes.jsonl` | one node per canonical entity |
| `edges.tsv` / `edges.jsonl` | one edge per asserted relationship |
| `manifest.json` | counts, biolink version, data version, per-category/predicate/source breakdown, validation status |

**Node columns**: `id, category, name, equivalent_identifiers, provided_by`
**Edge columns**: `id, subject, predicate, object, primary_knowledge_source, aggregator_knowledge_source, knowledge_level, agent_type, has_evidence, qualifiers`

CURIEs use biolink/bioregistry-canonical prefixes (`HGNC:`, `UniProtKB:`,
`MONDO:`, `CHEMBL.COMPOUND:`, …). `aggregator_knowledge_source` is
`infores:biobtree`; `primary_knowledge_source` is the originating dataset.

## Node model

Each real-world entity is **one** node, biolink-typed, with its other identifiers
folded into `equivalent_identifiers`:

```
HGNC:1100   biolink:Gene   BRCA1   HGNC:1100|ENSEMBL:ENSG00000012048|NCBIGene:672   infores:biobtree
```

- **Typing**: dataset → biolink category (`mappings/categories.yaml`).
- **Runtime-typed datasets**: a few datasets split into several categories by a
  record field rather than a fixed mapping — **GO** by aspect (MolecularActivity/
  BiologicalProcess/CellularComponent) and **RefSeq** by accession type (mRNA →
  Transcript, ncRNA → NoncodingRNAProduct, NP_/XP_ → Protein). **OBA** biological
  attributes are typed PhenotypicFeature (closest existing class for GWAS traits).
- **Normalization** (gene-first): the HGNC/Ensembl/NCBIGene ids of one gene are
  merged into a single node. Merging is **cardinality-aware** — only 1:1
  mappings are merged, so two distinct genes that happen to share one Ensembl id
  are kept separate (no over-merge).
- **Stub nodes**: edges may reference entities BioBTree stores only for a subset
  of species (genes/proteins are taxid-scoped). A minimal `id + category` node is
  emitted for any such endpoint so the graph has no dangling edges.
- **Primary vs cross-reference datasets**: a category is backed by two kinds of
  dataset — *primary* sources with their own records (source1/source2 + the
  runtime builders → named nodes, e.g. HGNC/Ensembl genes) and *cross-reference /
  identifier* namespaces (BioBTree's xref layer, no own records → typed but
  nameless stub nodes, e.g. the cross-species MGI/RGD/ZFIN/… genes, EC, OMIM,
  PMID). They are genuinely distinct entities (a mouse gene ≠ a human gene), so
  they stay separate nodes, not `equivalent_identifiers`. The meta-graph explorer
  separates the two in each node's dataset panel.
- **Linking datasets**: a few source1 datasets (`ortholog`, `paralog`,
  `orthologentrez`, …) are BioBTree *linkdataset* tags — auto-derived gene↔gene
  relationships stored inside the `ensembl`/`entrez` forwards, not their own
  entities. They contribute **edges** (`orthologous_to`/`paralogous_to`), so the
  meta-graph shows them as edges and excludes them from the node-dataset panel.
- **Entry attributes → node properties** (`nodeattrs`): in BioBTree the entry holds
  its full attribute set and `compact_fields` is just an inline-mapping convenience;
  a materialized KG has no such split — a node *is* the entry — so each entry's
  attributes are attached directly as node properties. This is what makes the API's
  `/ws/filter` (CEL over attributes) reproducible as a Cypher `WHERE` (e.g.
  `n.entrez_type = 'protein-coding'`, `n.ensembl_biotype = 'protein_coding'`). Keys
  are **dataset-prefixed** so a merged node (HGNC+Ensembl+NCBIGene) collects every
  namespace's attributes without collision. Mode `all` (default) carries every field
  (scalars + scalar lists + one level of nested-dict flattening; lists-of-objects
  skipped as heavy/relational); mode `compact` carries only the dataset's conf
  `compact_fields` — the opt-in slim knob for heavy datasets. Config:
  `mappings/node_attributes.yaml`.
- **Numeric/value node attributes** (`attributes`): a few datasets aren't entities or
  relationships — their content **is a scalar about a *different* existing entity**:
  `gnomad_constraint` (pLI/LOEUF on a gene), `depmap` (essentiality on a gene),
  `alphafold` (mean pLDDT on a protein), `alphamissense_transcript` (mean
  pathogenicity on a transcript). The subject is canonicalized to that entity's node
  CURIE and the values merged onto it (`gnomad_pli`, `alphafold_mean_plddt`, …).
  Config: `mappings/attributes.yaml`. (Distinct from `nodeattrs`, which attaches an
  entry's *own* attributes to its *own* node.)

## Edge model

Each cross-reference / relation maps to a biolink predicate
(`mappings/predicates.yaml`):

| BioBTree relation | predicate |
|---|---|
| gene → protein | `biolink:has_gene_product` |
| variant → gene | `biolink:is_sequence_variant_of` |
| gene/protein → pathway | `biolink:participates_in` |
| protein ↔ protein (IntAct/STRING) | `biolink:physically_interacts_with` |
| protein similarity (DIAMOND/ESM2) | `biolink:similar_to` |
| gene/protein → GO term | `enables` / `actively_involved_in` / `located_in` (by aspect) |
| drug → disease (ChEMBL) | `biolink:treats_or_applied_or_studied_to_treat` |
| gene → tissue (Bgee) | `biolink:expressed_in` |
| transcript → protein (RefSeq) | `biolink:translates_to` |
| ontology term → parent term | `biolink:subclass_of` |
| cross-ontology mapping | `biolink:close_match` |
| gene → trait (GWAS) | `biolink:associated_with` |

- **Reified relations** (PPI, similarity, bioactivity) are joined from their
  intermediate entries; only the *asserted* binary pairs are emitted (no clique
  fabrication).
- **Ontology structure**: each ontology's `is_a` hierarchy (stored inline in
  BioBTree as `<ont>parent`/`<ont>child` tags) is emitted as `subclass_of` —
  including the full NCBI taxonomy tree. Cross-ontology mappings between
  same-category namespaces (MONDO↔DOID/OMIM/Orphanet/EFO; the uPheno hub ↔
  HP/MP/ZP/XPO/WBPhenotype/FYPO) are emitted as **`close_match`** — a deliberate
  under-claim: BioBTree doesn't retain the source skos predicate and we don't
  merge across namespaces, so `close_match` (not `exact_match`/`same_as`) is the
  honest assertion. *Not* emitted as `subclass_of`: Reactome (sub-pathway is
  `part_of`) and ChEMBL salt→parent (a chemical relation).
- **Sub-gene / protein structure**: the granular structural sub-entities — Ensembl
  **exons** and **CDS/translations**, UniProt **protein features** — are typed as
  nodes by the standard `nodes` builder; their containment edges split across two
  builders by which way the forward index carries the link. `transcript has_part
  exon` and `transcript has_part cds` (~18.8M) come from the `edges` builder
  (`transcript_sorted` carries them forward, mapped by `transcript>exon`/`>cds`).
  The `structure` builder emits the rest: `cds translates_to protein` (the
  Ensembl→UniProt coding link) and `protein has_part feature` — the latter is the
  first place **ECO evidence actually lands**, with each feature's inline `evidences`
  ECO codes written to `has_evidence` (~5M of ~5.8M feature edges). Feature
  `type`/`description`/`location` and exon coordinates ride along as node
  attributes. The whole layer (~25M edges) is emitted always-on; the seed-driven
  published subgraph filters it to the structure of seed entities.
- **Provenance**: every edge carries its `primary_knowledge_source`.
- **Qualifiers & evidence**: edges carry optional `has_evidence` (ECO CURIEs) and
  `qualifiers` (`slot=value`) columns. Populated: `assay_type` (BAO) on every
  ChEMBL bioactivity edge; **ECO evidence on protein-feature `has_part` edges**
  (from UniProt's inline `evidences`); plus **numeric/value qualifiers pulled from
  the entry's property JSON** — e.g. `splice_score`/`splice_effect` (SpliceAI),
  `pathogenicity` (AlphaMissense), `p_value`/`odds_ratio_beta` (GWAS),
  `confidence`/`inheritance` (PanelApp), `significance`/`evidence_level` (CIViC).
  Still deferred because BioBTree stores them at entry/study level, not per edge:
  ECO evidence elsewhere (per-protein annotation, not per GO term), PATO quality
  (per GWAS study, never co-located with a mapped trait), XCO condition (per
  metabolite).
- **Numeric node attributes**: scalars that describe an entity rather than relate
  two (gnomAD constraint, DepMap essentiality, AlphaFold pLDDT, AlphaMissense
  per-transcript mean) are attached as node properties — see *Node model* above.
- **No catch-all**: pairs without a mapping are dropped and counted, never
  emitted as a generic `related_to`.
- **Dedup**: the same logical edge arriving via different keys is collapsed to one
  (matching what the service serves at query time).

## Alignment

The export is generated from the index files but **verified against the live
service** (`/ws/entry`) to confirm it reproduces BioBTree's graph. Per-target edge
counts match exactly across entity types — e.g. for `P38398` (BRCA1 protein):
`string_interaction` 6120=6120, `corum` 40=40, `go` 71=71. The alignment check is
codified as a test (`tests/test_alignment.py`).

### API parity (what a Neo4j import can reproduce)

The goal is that someone who imports the dump can ask the same things as the
BioBTree API. Mapped against the four query types:

| API | reproducible on the KG | how |
|---|---|---|
| `/ws/entry` (lookup by any id) | ✅ | `id` + `equivalent_identifiers` resolve any namespace to the node |
| `/ws/map` (cross-dataset, multi-hop) | ✅ | map is stored-xref traversal (not transitive closure) and bidirectional → Cypher path match; same-entity id maps are folded into `equivalent_identifiers` |
| `/ws/filter` (CEL over attributes) | ✅ | the `nodeattrs` layer puts entry attributes on nodes → `WHERE n.<ds>_<field> = …` |
| `/ws/search` (text/keyword) | ⚠️ partial | a Neo4j full-text index covers the exported `name`; BioBTree's curated aliases/keywords are not yet exported (a `synonyms` property is the planned closer) |

So traversal + id-resolution + attribute filtering are reproducible; only curated
search aliases remain a gap. (Deliberately-deferred datasets, e.g. drugcentral, are
adapted separately.)

## Building

The export is a pipeline of CLI steps over a built index directory
(`out_prod/main/index`):

```bash
IDX=out_prod/main/index; O=out/kg
# 1. nodes (genes/proteins/diseases/...) + canonical id_map
python -m tools.kg_export nodes  --index-dir $IDX --datasets <core list> \
    --out $O/nodes.tsv.gz --id-map $O/id_map.tsv.gz
# 2. GO nodes + aspect-typed annotation edges
python -m tools.kg_export go      --index-dir $IDX --id-map $O/id_map.tsv.gz \
    --nodes-out $O/go_nodes.tsv.gz --edges-out $O/go_edges.tsv.gz
# 3. RefSeq transcript/protein/ncRNA nodes + edges (type-split)
python -m tools.kg_export refseq  --index-dir $IDX --id-map $O/id_map.tsv.gz \
    --nodes-out $O/refseq_nodes.tsv.gz --edges-out $O/refseq_edges.tsv.gz
# 4. ontology hierarchy (subclass_of) + cross-ontology close_match
python -m tools.kg_export ontology --index-dir $IDX --out $O/ontology_edges.tsv.gz
# 5. direct edges, 6. reified edges (PPI/similarity/bioactivity/expression/GWAS)
python -m tools.kg_export edges   --index-dir $IDX --id-map $O/id_map.tsv.gz --out $O/edges_direct.tsv.gz
python -m tools.kg_export reified --index-dir $IDX --id-map $O/id_map.tsv.gz --out $O/edges_reified.tsv.gz
# 6d. numeric/value NODE attributes (gnomad/depmap/alphafold/alphamissense_transcript)
python -m tools.kg_export attributes --index-dir $IDX --id-map $O/id_map.tsv.gz --out $O/node_attrs.tsv.gz
# 6e. sub-gene/protein structure: has_part + translates_to edges (+ ECO evidence, coords)
python -m tools.kg_export structure --index-dir $IDX --id-map $O/id_map.tsv.gz \
    --edges-out $O/structure_edges.tsv.gz --attrs-out $O/structure_attrs.tsv.gz
# 7. assemble: merge + dedup + stub-nodes + node-attributes + JSONL + validate + manifest
python -m tools.kg_export assemble --nodes $O/nodes.tsv.gz,$O/go_nodes.tsv.gz,$O/refseq_nodes.tsv.gz \
    --edges $O/edges_direct.tsv.gz,$O/edges_reified.tsv.gz,$O/go_edges.tsv.gz,$O/refseq_edges.tsv.gz,$O/ontology_edges.tsv.gz,$O/structure_edges.tsv.gz \
    --node-attributes $O/node_attrs.tsv.gz,$O/structure_attrs.tsv.gz \
    --out-dir $O/dump --data-version <release> --stub-nodes --gzip
```

The complete production pipeline is scripted in `tools/kg_export/full_prod.sh`.

`.gz` paths produce compressed output (~6× smaller). Memory stays modest
(~4 GB) because the giant datasets contribute *edges*, not nodes (stubs cover
their endpoints).

## Validation

`assemble` runs a structural check and stamps `manifest.status` `VALID`/`INVALID`:
dangling edges, duplicate node ids, non-biolink categories/predicates, and
non-canonical CURIE prefixes. A VALID dump has zero dangling edges and zero
duplicate node ids. (The full biolink-model-toolkit / KGX validator in CI is a
planned follow-up.)

**Billion-scale (memory-flat assemble).** The whole assemble runs in flat memory so
it survives the dbSNP layer (~1.1B nodes / ~3B edges): `merge_nodes` and
`merge_edges` dedup via external `sort -u` (disk-spill), and `add_stub_nodes`
computes "endpoints with no node" by `comm` over sorted id lists — none of them
hold a billion-entry Python set. Validation has two modes (`--validate-mode`):
- `full` (default) — exact, in-memory `node_ids`/`edge_ids` sets. Used for the small
  **published subgraph** (the real publish gate) and tests.
- `streaming` — billion-scale: one streaming pass for the per-row shape checks
  (CURIE/category/predicate/prefix), with dangling and duplicate counts taken from
  the construction steps (sort-dedup removed counts; stub's untyped-endpoint count)
  rather than recomputed with giant sets. `full_prod.sh` uses this.

## Coverage & limitations

- **Taxid scope**: BioBTree's genes/proteins cover 16 model organisms; edges to
  entities outside that scope appear as stub nodes (typed, unnamed).
- **Deferred edge types**: directed regulation (SIGNOR), ncRNA layers, and
  CollecTRI are not in the default export.
- **dbSNP (first-class, billion-scale)**: the dbSNP federation (~1.1B variants) is a
  first-class layer — `variant -is_sequence_variant_of-> gene/transcript` plus rich
  variant attributes. It lives in a separate ~118 GB-gz federation, so it's built by
  a dedicated parallel extractor (`tools/dbsnp_py/extract.py`: `zcat` decompresses,
  a Python multiprocessing pool parses/shards, ~1 hr/full pass) and wired into
  `full_prod.sh` (`WITH_DBSNP=1`, default on; set `0` to skip). The assemble step is
  memory-flat (sort-based merge/stub + `--validate-mode streaming`), so it survives
  the billion-scale layer — see *Validation*.
- **Prefixes**: SwissLipids is emitted as `SWISSLIPID` pending canonical `SLM` +
  zero-padded ids; `validate()` reports any non-canonical prefixes.
- **Deferred qualifiers**: ECO evidence, PATO quality, XCO condition, and numeric
  qualifiers (IC50, confidence, trial phase) are not yet attached — BioBTree
  stores them at entry/study level, not per edge. The `has_evidence`/`qualifiers`
  columns are in place; populating ECO needs a BioBTree-side change to emit
  evidence per annotation.
- **Ontologies left out**: ECO/PATO/OBA/BAO/XCO are qualifier/attribute
  ontologies, not entity types — only OBA is emitted as nodes (GWAS traits); the
  rest feed (or will feed) edge qualifiers. **OBI** is not emitted: in BioBTree it
  is isolated (only its own internal hierarchy, no links to any other dataset) —
  reported to the BioBTree team.
