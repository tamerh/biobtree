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
**Edge columns**: `id, subject, predicate, object, primary_knowledge_source, aggregator_knowledge_source, knowledge_level, agent_type`

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
- **Normalization** (gene-first): the HGNC/Ensembl/NCBIGene ids of one gene are
  merged into a single node. Merging is **cardinality-aware** — only 1:1
  mappings are merged, so two distinct genes that happen to share one Ensembl id
  are kept separate (no over-merge).
- **Stub nodes**: edges may reference entities BioBTree stores only for a subset
  of species (genes/proteins are taxid-scoped). A minimal `id + category` node is
  emitted for any such endpoint so the graph has no dangling edges.

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

- **Reified relations** (PPI, similarity, bioactivity) are joined from their
  intermediate entries; only the *asserted* binary pairs are emitted (no clique
  fabrication).
- **Provenance**: every edge carries its `primary_knowledge_source`.
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
# 3. direct edges, 4. reified edges (PPI/similarity/bioactivity/expression)
python -m tools.kg_export edges   --index-dir $IDX --id-map $O/id_map.tsv.gz --out $O/edges_direct.tsv.gz
python -m tools.kg_export reified --index-dir $IDX --id-map $O/id_map.tsv.gz --out $O/edges_reified.tsv.gz
# 5. assemble: merge + dedup + stub-nodes + JSONL + validate + manifest
python -m tools.kg_export assemble --nodes $O/nodes.tsv.gz,$O/go_nodes.tsv.gz \
    --edges $O/edges_direct.tsv.gz,$O/edges_reified.tsv.gz,$O/go_edges.tsv.gz \
    --out-dir $O/dump --data-version <release> --stub-nodes --gzip
```

`.gz` paths produce compressed output (~6× smaller). Memory stays modest
(~4 GB) because the giant datasets contribute *edges*, not nodes (stubs cover
their endpoints).

## Validation

`assemble` runs a structural check and stamps `manifest.status` `VALID`/`INVALID`:
dangling edges, duplicate node ids, non-biolink categories/predicates, and
non-canonical CURIE prefixes. A VALID dump has zero dangling edges and zero
duplicate node ids. (The full biolink-model-toolkit / KGX validator in CI is a
planned follow-up.)

## Coverage & limitations

- **Taxid scope**: BioBTree's genes/proteins cover 16 model organisms; edges to
  entities outside that scope appear as stub nodes (typed, unnamed).
- **Deferred edge types**: directed regulation (SIGNOR), ncRNA layers, CollecTRI,
  and (by run choice) very large variant sets (dbSNP) are not in the default
  export.
- **Prefixes**: SwissLipids is emitted as `SWISSLIPID` pending canonical `SLM` +
  zero-padded ids; `validate()` reports any non-canonical prefixes.
- **Qualifiers**: numeric edge qualifiers (IC50, confidence, trial phase) are a
  follow-up; the underlying values are present in the index but not yet attached.
