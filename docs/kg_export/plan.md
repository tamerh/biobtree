# BioBTree → Knowledge Graph export — implementation plan

**Branch:** `kg-export`  **Worktree:** `/data/biobtree_kg`
**Status:** approved for build (2026-06-15)
**Goal:** publish a **biolink-typed, normalized KGX graph** of what BioBTree already
holds, so BioBTree can be registered as a `KnowledgeGraph` / `GraphProduct` in
KG-Registry and the "built on a knowledge graph" claim becomes machine-checkable.
See proposal: `/data/biobtree_kg.md`.

This is a **credibility / distribution artifact**, not a change to the live product.
It does not touch the Go core, the service, or the MCP server at runtime — it is a
**standalone batch exporter** that runs over post-build files.

---

## 1. Grounded facts (verified against the repo, not assumed)

These shape every decision below.

1. **The edge list already exists on disk.** The concat phase emits, per dataset,
   `out_prod/main/index/<dataset>_sorted.<chunk>.index.gz` (forward) plus
   `<target>_from_<source>_sorted.<chunk>.index.gz` (reverse). Column layout,
   verified:
   ```
   subject_id \t source_dataset_numeric_id \t object_id \t object_dataset_numeric_id [ \t evidence ] [ \t relationship ]
   ```
   Example: `HGNC:100 ⇥ 10 ⇥ 41 ⇥ 4` = HGNC:100 (hgnc, id 10) → 41 (entrez, id 4).
   ⇒ The exporter **streams these gzipped files**; it never queries LMDB or the
   service. Fully decoupled, runs cold, versions with the data release.

2. **Datasets are referenced by numeric `id`**, not name. We must build an
   `id → dataset name` map from `conf/{source1,source2,xref1,xref2.optional}.dataset.json`.

3. **Most edges carry no predicate.** The evidence/relationship columns are
   usually absent. ⇒ The biolink predicate must be **derived from the
   `(source_dataset, target_dataset)` pair**, with the on-disk `relationship`
   field used to refine when present.

4. **Edges are stored bidirectionally** (forward file + a `from_` reverse file).
   ⇒ The exporter must **deduplicate** and emit each semantic edge **once**, in a
   canonical direction defined by the predicate (e.g. always gene→protein for
   `has_gene_product`).

5. **No biolink category in config.** Config has `group`, `federation`,
   `linkdataset`, `derivedFrom` — useful, but category + canonical-namespace
   priority must be **authored** (one row per dataset).

6. **`mcp_srv/prompts.py` (the EDGES guide) is a curated, directed relation
   inventory** for ~78 datasets. It is the seed for the predicate table — reuse it,
   don't re-derive it. Long-term goal: single source of truth so the EDGES guide
   and the KG predicate table cannot drift.

---

## 2. Architecture

```
out_prod/main/index/*_sorted.*.index.gz   (+ dbsnp federation)
            │  stream (gzip)
            ▼
   ┌──────────────────────┐     conf/*.dataset.json ──► id→name map
   │   kg exporter (py)    │     mappings/categories.yaml ──► node biolink type + CURIE priority
   │  src: tools/kg_export │     mappings/predicates.yaml ──► (srcDS,objDS[,rel]) → biolink predicate
   └──────────────────────┘
            │
   ┌────────┴─────────┐
   ▼                  ▼
 PASS 1: nodes      PASS 2: edges
 - collect ids      - map each xref → predicate
 - type by dataset  - drop edges whose endpoints are dropped nodes
 - normalize        - rewrite endpoints to canonical CURIE
   (Option C)       - dedup + canonical direction
            │
            ▼
   KGX nodes.tsv + edges.tsv  (+ JSONL)  +  manifest.json
            │
            ▼
   KGX validator + biolink compliance  →  versioned dump  →  KG-Registry GraphProduct
```

Language: **Python** (lives alongside `mcp_srv/`, reuses its dataset-metadata
patterns; the heavy lifting is gzip streaming + dict lookups, not CPU). Output is
plain TSV/JSONL so downstream tooling (KGX, Neo4j, RDF) is standard.

Location in repo: `tools/kg_export/` (code) + `mappings/` (authored tables) +
`docs/kg_export/` (this plan + the eventual data dictionary).

---

## 3. Node model + normalization (Option C — the hard part)

**Decision: Option C (hybrid), gene-first 80/20.**

- **Type first, merge second.** Each dataset maps to exactly one biolink category
  in `mappings/categories.yaml`. A node's category comes from the dataset it was
  seen in. Category is what decides merge-vs-edge (same category across an xref =
  candidate node merge; different category = typed edge).
- **Normalization order of preference per node:**
  1. **External canonical CURIE** (Translator Babel / NodeNorm) for the
     well-covered core: gene, protein, disease, drug/chemical, variant. Gives us
     IDs that *compose* with Monarch/Translator — the whole point of normalizing.
  2. **Own connected-components** fallback for datasets Babel doesn't cover
     (ncRNA layer, cell lines, similarity nodes, custom/niche datasets — also our
     differentiation). Build components over **same-category identity edges only**,
     pick canonical by an authored namespace priority (e.g. gene: HGNC > Ensembl >
     NCBIGene). All other IDs become `equivalent_identifiers`.
- **80/20 scope for the MVP:** normalize **genes properly** (the dominant
  duplicate-node source: Ensembl/HGNC/NCBIGene triple), reuse external CURIEs
  where trivial for protein/disease/drug, and leave long-tail categories as
  1-id-1-node initially. Expand later.
- **Guardrail against over-merging:** only edges whose `relationship`/dataset-pair
  is on an authored **identity allowlist** participate in clustering. xrefs built
  for *traversal* are not automatically treated as *identity*. Log every merge that
  collapses >N ids for spot-checking.

KGX node row: `id` (canonical CURIE), `category`, `name`, `equivalent_identifiers`,
`provided_by: biobtree`.

---

## 4. Edge model (predicate mapping)

`mappings/predicates.yaml`: key = `(source_dataset, object_dataset)` (optionally
`+ relationship`), value = `{ predicate, direction, qualifiers }`. Seeded from
proposal §4 and `mcp_srv/prompts.py`. Examples (from §4):

| src→obj dataset | biolink predicate |
|---|---|
| ensembl → uniprot | `biolink:has_gene_product` |
| gencc/clinvar/clingen → gene–disease | `biolink:gene_associated_with_condition` (causal/risk qualifier) |
| chembl/gtopdb drug → target | `biolink:interacts_with` / `affects` |
| chembl indications | `biolink:treats` (phase qualifier) |
| reactome | `biolink:participates_in` |
| go | `enables` / `actively_involved_in` / `located_in` |
| string/intact | `physically_interacts_with` (score qualifier) |
| ensembl compara | `orthologous_to` / `paralogous_to` |
| intogen/civic/depmap | cancer driver / dependency (role qualifier) |
| ncbi taxonomy | `in_taxon` |

KGX edge row: `subject`, `predicate`, `object`, `primary_knowledge_source` (the
originating dataset), `aggregator_knowledge_source: infores:biobtree`, plus
qualifiers when present. **Same-category identity xrefs are folded into node
`equivalent_identifiers`, not emitted as `same_as` edges** (resolves proposal open
question — cleaner, standard KGX idiom).

Anything whose dataset-pair isn't in `predicates.yaml` is **dropped and counted**
in the manifest (no silent `related_to` catch-all — that's what makes a graph
undifferentiated).

---

## 5. Output + validation + registration

- **KGX TSV** (`nodes.tsv`, `edges.tsv`) + **JSONL** — register this first.
- **`manifest.json`**: `node_count`, `edge_count`, biolink version, biobtree data
  version (from the `_sorted.<chunk>` chunk id / `data_version`), per-source counts,
  per-predicate counts, dropped-edge counts, merge stats.
- **Validation:** run the official KGX validator + biolink-model compliance check
  in CI before publish.
- **Neo4j / RDF-Turtle:** later, incremental on top of KGX.
- **Registration:** ship as a `GraphProduct` under the existing aggregator resource
  in KG-Registry first; promote to standalone `KnowledgeGraph` only if it earns a
  release cadence.

---

## 6. Differentiation (so this isn't "Monarch but smaller")

Lead the MVP edge set with datasets Monarch-KG / SPOKE **lack**: ChEMBL/pubchem
bioactivity depth, ESM2/Diamond protein similarity, the ncRNA layer
(rnacentral/ncrna_*), AlphaFold/AlphaMissense, expression (cellxgene/scxa/HPA),
Cellosaurus. Gene–disease is included but is not the headline.

---

## 7. Phased milestones (each is a shippable checkpoint)

- **Phase 0 — scaffold + dictionaries.** `tools/kg_export/` skeleton; build the
  numeric-`id → dataset name` map from configs; author `categories.yaml` for the
  MVP entity set (Gene, Protein, Disease, Drug, Variant, Pathway + the
  differentiating sets). *Deliverable:* loader that resolves any sorted file's
  columns to typed datasets; unit test on a known file.
- **Phase 1 — nodes.** Stream all sorted files, collect + type nodes, implement
  Option C normalization (gene-first), emit `nodes.tsv` + `equivalent_identifiers`.
  *Deliverable:* node file + merge-stats report; manual spot-check of gene cliques.
- **Phase 2 — edges.** Author `predicates.yaml` (seed from prompts.py + §4),
  map/dedup/canonicalize-direction, rewrite endpoints to canonical CURIEs, drop +
  count unmapped. *Deliverable:* `edges.tsv` + per-predicate counts.
- **Phase 3 — serializers + manifest.** JSONL output, `manifest.json`, KGX
  validator pass. *Deliverable:* a complete versioned dump that validates clean.
- **Phase 4 — publish + register.** Versioned release; KG-Registry `GraphProduct`
  entry. *Deliverable:* live listing.

**Rough effort:** Phases 0–3 (KGX, MVP coverage) ≈ 1–2 weeks. Full coverage +
RDF/Neo4j + external-normalizer polish is incremental.

---

## 8. Resolved decisions (was: open questions)

- **Normalization:** Option C, gene-first 80/20. ✔
- **`same_as` edges:** folded into node `equivalent_identifiers`, not emitted. ✔
- **Unmapped predicates:** dropped + counted, no `related_to` catch-all. ✔
- **Generation site:** standalone batch over `_sorted.*.index.gz`, not the hot path. ✔

Still to pin during build:
- Exact biolink version to target (follow current Monarch release line).
- Whether to vendor a Babel/NodeNorm snapshot vs. call the live service.
- Gene canonical namespace priority (proposed: HGNC > Ensembl > NCBIGene).
