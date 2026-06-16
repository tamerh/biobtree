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
  Merge is **cardinality-aware**: only 1:1 identity mappings union; a many:1 xref
  (e.g. two HGNC genes sharing one Ensembl id — SLC2A9-AS1 vs -AS3) is left
  unmerged + counted (`ambiguous_identity_edges`), and `suspect_clusters` guards
  that no cluster has >1 id from one namespace. e.g. BRCA1 →
  `HGNC:1100|ENSEMBL:ENSG00000012048|NCBIGene:672`.
- **Phase 2a (done):** direct (non-reified, non-GO) edges → KGX `edges.tsv`.
  Pair→predicate map (`mappings/predicates.yaml`) seeded from proposal §4 +
  prompts.py; endpoints rewritten to canonical CURIEs via the id_map; forward
  files only (reverse mirrors skipped); unmapped/skip pairs counted (no
  `related_to` catch-all). Real curated run: 33.4M edges across 14 biolink
  predicates.
- **Phase 2b (done):** reified edges → KGX edges. Intermediate-entry datasets
  joined by streaming group-by on the entry id, with three **kinds** that emit
  only asserted edges (no clique fabrication):
  - `pairwise` (intact, string_interaction): the real binary pair is named in
    each property line's JSON (`protein_a`/`protein_b`) — emit exactly those.
  - `star` (diamond/esm2 similarity): query → each hit (never hit↔hit).
  - `bipartite` (chembl_activity, bgee, depmap, fantom5): subject-role × object-
    role from edge lines.
  Real intact run: **1.65M** asserted pairs (the earlier clique emitted 11.1M,
  ~85% fabricated); depmap entrez genes canonicalized to HGNC.
- **Phase 2c (done):** GO annotations (aspect-dependent). GO terms typed by
  `type` (MF→MolecularActivity, BP→BiologicalProcess, CC→CellularComponent);
  annotation edges `enables`/`actively_involved_in`/`located_in`. Real run:
  48,321 GO terms + 5.48M edges (uniprot+ensembl sources).
- **Phase 3 (done):** assemble — merge partial node/edge TSVs (node dedup by id),
  KGX JSON-Lines, lightweight structural validator (dangling-edge + CURIE +
  predicate checks), and a `manifest.json` (counts, biolink version, data
  version, per-category/predicate/source breakdowns).

## Bounded production run (measured)

`run_bounded.sh` runs 22 core node datasets (≤400M each; excludes the giants
pubchem/entrez/rnacentral/clinvar) → a real dump, to measure scale before the
full run. Result on out_prod_v5:
- **Nodes pass peak RSS ≈ 4.15 GB** (13.06M nodes, 22 datasets, ~7 min). The box
  has 125 GB, so the full run (≈10× data) should fit in RAM — the B1 in-RAM
  concern is **not a blocker on this hardware** (on-disk rework still good hygiene).
- Dump: **13.06M nodes / 49.3M edges**, 18 categories; edge dedup removed 1.47M;
  `bad_category`/`bad_predicate`/`duplicate_node_ids` = 0.
- `status=INVALID`: ~11% dangling edges, dominated by **ENSEMBL (5.3M) + UniProtKB
  (148k)** subjects. Root cause: biobtree's ensembl/uniprot are taxid-scoped to 16
  model organisms, but bgee/intact/GO reference more species → those endpoints
  have no node. Fixed with **stub-node generation** (`assemble --stub-nodes`):
  emits a minimal `id + category` node (category from the CURIE prefix; ENSEMBL
  disambiguated by id pattern) for any edge endpoint missing a node. Re-assembled
  bounded dump → **`status=VALID`**: 13.3M nodes / 49.3M edges, 250,774 stubs
  (209k Gene + 42k Protein), dangling=0, dups=0. (`non_biolink_prefixes` still
  lists SLM/swisslipids + ~90 trypanosome `TB927.*` data-quality ids — reported,
  not gating.)

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
predicate/dup/non-biolink-prefix but isn't the full biolink check); expand node
`category` to the full biolink **ancestor chain** (currently leaf +
`biolink:NamedThing`); numeric **qualifiers** (IC50/score, clinical significance,
trial phase).

**CURIE prefix alignment (bioregistry) — mostly done.** Canonicalized (verified
against bioregistry): `cellosaurus` (ids keep `CVCL_`), `interpro` (ids keep
`IPR`), `corum`, `lipidmaps` (ids keep `LM…`), `orphanet`. CLINVAR, DBSNP,
MSigDB, GTOPDB, HMDB were already canonical. `validate()` reports
`non_biolink_prefixes` against the accepted set; the only remaining offender is
**`SWISSLIPID` → `SLM`** (canonical ids are zero-padded 9-digit, needs an id
transform — deferred; note some swisslipids xref values already arrive as `SLM:`).

## Edge dedup at assemble

The generate/merge step that builds BioBTree's LMDB dedups xrefs only *per source
key*; the same logical edge arriving via different keys (one protein pair across
two intact entries, a gene-protein edge via two gene namespaces) is collapsed by
the query **service** at read time, not in storage. A materialized KG has no
read-time layer, so `assemble` dedups by the deterministic edge id via an
external `sort -u` (disk-spilling → scales past RAM); the count collapsed is
reported in `manifest.edge_dedup` (real example, intact: 12.83M → 7.26M edges,
−5.56M duplicate PPI pairs; `validation.duplicate_edges` then 0).

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

## Testing (4 layers)

**L1 — unit tests** (fast, no data; ~46 cases): parsing, normalization, dedup,
reification, GO, assemble.
```bash
python3 -m unittest discover -s tools/kg_export/tests -p "test_*.py"
```

**L2 — real-data smoke** (skipped unless an index dir is given): parses a real
sorted file and resolves endpoints.
```bash
BIOBTREE_INDEX_DIR=/data/biobtree/out/main/index \
  python3 -m unittest discover -s tools/kg_export/tests -p "test_*.py"
```

**L3 — structural validation**: `assemble` runs `validate()` (dangling edges,
dup ids, bad categories/predicates) and stamps the manifest `status`.

**L4 — golden-entity (semantic) tests** (slow, opt-in): builds nodes/edges from a
real index dir and asserts known biology (BRCA1/TP53/EGFR normalization; a
canonicalized `has_gene_product` edge).
```bash
BIOBTREE_INDEX_DIR=/data/biobtree/out_prod/main/index \
  python3 -m unittest tools.kg_export.tests.test_golden -v
```

Other correctness cross-checks: compare per-predicate edge counts against
`dataset_state.json`; run the official KGX/biolink-model-toolkit validator
(deferred) for full biolink compliance.
