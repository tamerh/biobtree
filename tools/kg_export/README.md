# kg_export — BioBTree → Knowledge Graph (KGX/biolink)

Standalone batch exporter that turns BioBTree's post-build sorted index files
into a biolink-typed, normalized knowledge graph (KGX). It reads only the
on-disk `*_sorted.*.index.gz` files — it does **not** touch the Go core, the
query service, or the MCP server at runtime.

Design & roadmap: [`docs/kg_export/plan.md`](../../docs/kg_export/plan.md).

## Status

- **Phase 0 (done):** dataset id→name registry, biolink category map, index-line
  reader. See modules below.

## Modules

| Module | Purpose |
|---|---|
| `datasets.py` | `DatasetRegistry` — resolve numeric dataset ids → names/metadata from `conf/*.dataset.json`. |
| `categories.py` | `CategoryMap` — dataset → biolink category + CURIE prefix, from `mappings/categories.yaml`. |
| `index.py` | Parse/stream sorted index lines (`RawXref`), distinguish edges from node properties (`-1` sentinel), resolve endpoints to categories. |

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
