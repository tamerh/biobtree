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
  best-effort names, and a merge-stats report. Validated on real HGNC data
  (44,993 gene clusters, no over-merge; e.g. BRCA1 →
  `HGNC:1100|ENSEMBL:ENSG00000012048|NCBIGene:672`).

## Modules

| Module | Purpose |
|---|---|
| `datasets.py` | `DatasetRegistry` — resolve numeric dataset ids → names/metadata from `conf/*.dataset.json`. |
| `categories.py` | `CategoryMap` — dataset → biolink category + CURIE prefix + identity pairs, from `mappings/categories.yaml`. |
| `index.py` | Parse/stream sorted index lines (`RawXref`), distinguish edges from node properties (`-1` sentinel), resolve endpoints to categories. |
| `curie.py` | Render biolink CURIEs from dataset prefix + raw id (prefix-aware). |
| `nodes.py` | Phase 1: union-find clustering, canonical-CURIE selection, name extraction → KGX `nodes.tsv` + stats. |
| `__main__.py` | CLI: `python -m tools.kg_export nodes ...`. |

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
