# kg_export — BioBTree → Knowledge Graph (KGX/biolink)

Standalone batch exporter that turns BioBTree's post-build sorted index files
into a biolink-typed, normalized knowledge graph (KGX). It reads only the
on-disk `*_sorted.*.index.gz` files — it does **not** touch the Go core, the
query service, or the MCP server at runtime.

Design & roadmap: [`docs/kg_export/plan.md`](../../docs/kg_export/plan.md).

## Status

- **Phase 0 (done):** dataset id→name registry, biolink category map, index-line
  reader.
- **Phase 1 (done):** node collection + typing + Option C normalization
  (own-clusters, gene-first) → KGX `nodes.tsv` with `equivalent_identifiers`,
  best-effort names, a merge-stats report, and an `id_map` (member→canonical).
  Validated on real HGNC data (44,993 gene clusters, no over-merge; e.g. BRCA1 →
  `HGNC:1100|ENSEMBL:ENSG00000012048|NCBIGene:672`).
- **Phase 2a (done):** direct (non-reified, non-GO) edges → KGX `edges.tsv`.
  Pair→predicate map (`mappings/predicates.yaml`) seeded from proposal §4 +
  prompts.py; endpoints rewritten to canonical CURIEs via the id_map; forward
  files only (reverse mirrors skipped); unmapped/skip pairs counted (no
  `related_to` catch-all). Real curated run: 33.4M edges across 14 biolink
  predicates.
- **Phase 2b (done):** reified edges → KGX edges. Intermediate-entry datasets
  (PPI, similarity, bioactivity, dependency, expression) joined by streaming
  group-by on the entry id; symmetric (undirected pairs) and bipartite
  (subject→object) kinds in `predicates.yaml`. Real run (5 datasets, no
  string_interaction): 26.0M edges (intact PPI 11.1M, bgee expression 8.6M,
  chembl bioactivity 3.9M, depmap dependency 2.2M, fantom5 0.2M); depmap entrez
  genes canonicalized to HGNC.
- **Phase 2c (done):** GO annotations (aspect-dependent). GO terms typed by
  `type` (MF→MolecularActivity, BP→BiologicalProcess, CC→CellularComponent);
  annotation edges `enables`/`actively_involved_in`/`located_in`. Real run:
  48,321 GO terms + 5.48M edges (uniprot+ensembl sources).
- **Phase 3 (done):** assemble — merge partial node/edge TSVs (node dedup by id),
  KGX JSON-Lines, lightweight structural validator (dangling-edge + CURIE +
  predicate checks), and a `manifest.json` (counts, biolink version, data
  version, per-category/predicate/source breakdowns).

## Running a full build

The builders write partial KGX files; `assemble` merges + serializes + validates.

```bash
IDX=/data/biobtree/out_prod/main/index
# 1. nodes — IMPORTANT: cover ALL node datasets so edges resolve (no dangling).
python -m tools.kg_export nodes  --index-dir $IDX \
    --datasets hgnc,ensembl,entrez,uniprot,uberon,cl,cellosaurus,chebi,mondo,doid,efo,orphanet,mim,hpo,reactome,interpro,taxonomy,chembl_molecule,pubchem,hmdb,lipidmaps,swisslipids,rnacentral,msigdb,transcript \
    --out out/kg/nodes.tsv --id-map out/kg/id_map.tsv --stats out/kg/nodes.stats.json
# 2. edges (direct), reified, GO
python -m tools.kg_export edges    --index-dir $IDX --id-map out/kg/id_map.tsv --out out/kg/edges_direct.tsv  --stats out/kg/edges.stats.json
python -m tools.kg_export reified  --index-dir $IDX --id-map out/kg/id_map.tsv --out out/kg/edges_reified.tsv --stats out/kg/reified.stats.json
python -m tools.kg_export go       --index-dir $IDX --id-map out/kg/id_map.tsv --nodes-out out/kg/go_nodes.tsv --edges-out out/kg/go_edges.tsv
# 3. assemble
python -m tools.kg_export assemble \
    --nodes out/kg/nodes.tsv,out/kg/go_nodes.tsv \
    --edges out/kg/edges_direct.tsv,out/kg/edges_reified.tsv,out/kg/go_edges.tsv \
    --out-dir out/kg/dump --data-version <release>
```

> A partial nodes pass (e.g. hgnc-only) makes the validator report dangling
> edges — that is expected; the nodes pass must cover every dataset that appears
> as an edge endpoint.

## Edge schema (KGX)

`id, subject, predicate, object, primary_knowledge_source,
aggregator_knowledge_source, knowledge_level, agent_type`. Edge `id` is a
deterministic hash of `subject|predicate|object|primary`. Curated source edges
are `knowledge_assertion`/`manual_agent`; similarity edges (diamond/esm2) are
`prediction`/`automated_agent`.

## Remaining / deferred

Edge types: collectri (TF→gene; needs role disambiguation), ncrna_*
(disease/interaction/drug), cellphonedb, gtopdb_interaction, **signor** (directed
regulation — needs role/sign), entrez>go (~119M), dbsnp>entrez (~769M).

Compliance follow-ups (post-review): map BioBTree datasets → **registered
infores ids** (currently `infores:<dataset>`, well-formed but unregistered) and
register `infores:biobtree`; run the **official KGX/biolink-model-toolkit
validator** in CI (the internal `validate()` checks dangling/CURIE/category/
predicate/dup but isn't the full biolink check); expand node `category` to the
full biolink **ancestor chain** (currently leaf + `biolink:NamedThing`); **dedup
edges** by id at assemble (duplicate_edges is surfaced in validation — PPI
legitimately repeats the same pair across experiments); numeric **qualifiers**
(IC50/score, clinical significance, trial phase).

## Modules

| Module | Purpose |
|---|---|
| `datasets.py` | `DatasetRegistry` — resolve numeric dataset ids → names/metadata from `conf/*.dataset.json`. |
| `categories.py` | `CategoryMap` — dataset → biolink category + CURIE prefix + identity pairs, from `mappings/categories.yaml`. |
| `index.py` | Parse/stream sorted index lines (`RawXref`), distinguish edges from node properties (`-1` sentinel), resolve endpoints to categories. |
| `curie.py` | Render biolink CURIEs from dataset prefix + raw id (prefix-aware). |
| `nodes.py` | Phase 1: union-find clustering, canonical-CURIE selection, name extraction → KGX `nodes.tsv` + `id_map` + stats. |
| `predicates.py` | `PredicateMap` — dataset pair → biolink predicate, from `mappings/predicates.yaml`. |
| `edges.py` | Phase 2a: map direct xrefs → biolink edges, rewrite endpoints to canonical CURIEs → KGX `edges.tsv` + stats. |
| `reified.py` | Phase 2b: join intermediate-entry datasets (PPI/similarity/bioactivity/expression) → reified KGX edges + stats. |
| `go.py` | Phase 2c: GO term typing + aspect-dependent annotation edges. |
| `kgx.py` | Phase 3: merge, JSON-Lines, structural validation, manifest. |
| `__main__.py` | CLI: `python -m tools.kg_export {nodes,edges,reified,go,assemble} ...`. |

## Build nodes (CLI)

```bash
python -m tools.kg_export nodes \
  --index-dir /data/biobtree/out_prod/main/index \
  --conf conf --categories mappings/categories.yaml \
  --out out/kg/nodes.tsv --stats out/kg/nodes.stats.json \
  [--datasets hgnc,ensembl,entrez] [--max-lines N]
```

## Mapping tables

- `mappings/categories.yaml` — node typing (authored).
- `mappings/predicates.yaml` — edge predicate mapping (Phase 2).

## Run tests

From the repo root:

```bash
python3 -m unittest tools.kg_export.tests.test_phase0 -v
```

Optional real-data smoke test — point at a built index dir (skipped if absent):

```bash
BIOBTREE_INDEX_DIR=/data/biobtree/out/main/index \
  python3 -m unittest tools.kg_export.tests.test_phase0 -v
```
