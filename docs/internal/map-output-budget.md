# Internal: map output budget vs MCP result size

Status: open (future work) · Last updated: 2026-11-05 · Context: v2.19.1, db_v54

This note records a design gap found while fixing multi-input map pagination.
The reported bugs are fixed; the remaining item is a UX/architecture improvement
that is deliberately deferred until map traffic justifies it.

## What was fixed (v2.19.1)

- `parsePagingKey` returned inside the loop over per-input page segments, so
  page 2+ of a multi-gene map carried only one input and silently dropped the
  rest. Fixed; Go tests added (`src/service/mapfilter_test.go`).
- Enrichment undercounted because of the above. Now the 10-gene GeneTuring
  set reports M22795 at 10/10 and the 3-gene set at 3/3. Ranking is FDR first,
  then fold.
- MCP structural truncation invalidated the continuation token and reports
  `dropped_inputs`, so truncation can no longer silently skip mappings.
- Map docs now say `stats.total` is target rows in this response (summed across
  inputs), not the number of mappings.

## The remaining gap

The service page and the MCP output budget are sized independently:

| Quantity | Value |
|---|---|
| `maxMappingResultLite` (targets per input per page) | 100 |
| MCP `max_result_length` | 50,000 chars |
| MSigDB target size (measured) | ~220 bytes |
| 10 genes × 100 targets | ~220,000 chars (1,000 targets) |

So a normal 10-gene `>>hgnc>>msigdb` page is ~4.4× the MCP budget. The MCP keeps
only ~2 mappings and the caller must re-query the other 8 (now reported in
`dropped_inputs`).

This is not a correctness bug after v2.19.1, but it is poor ergonomics for the
common "map a handful of genes to a high-fanout dataset" case.

## Why input batching is the wrong fix

Hard-capping the number of input terms does not solve the problem, because the
size is driven by fanout per gene, not by gene count:

- 5 genes × 100 targets ≈ 110 KB — still over budget.
- 1 gene can exceed it alone (AKT1 is in 1,733 MSigDB sets).
- For low-fanout chains (e.g. `>>hgnc>>uniprot`) 10 genes is ~10 rows, so a cap
  only adds friction.

Do **not** add a hard input-count limit.

## Proposed work (when needed)

1. **Optional `limit` on `/ws/map/`** — max targets per input per page.
   - Thread through `web.go` → a new `MapFilterLiteLimit` (or an extra arg on
     `MapFilterWithLimit`), clamped to `1..maxMappingResultLite`.
   - Pagination is tracked by absolute `entryIndex` per input, so changing
     `limit` between pages is safe; the token stays valid.
   - 10 genes × 20 targets ≈ 44 KB — fits, with all mappings present.
2. **Adaptive MCP retry** — in `execute_tool` for `biobtree_map`, if the compact
   payload exceeds `max_result_length`, re-fetch once with
   `limit ≈ budget / (n_inputs × bytes_per_target)`, then truncate only as a
   last resort. Add `limit` to `BiobtreeClient.map()`.
3. **Steer gene-list questions to `biobtree_enrich`** — it already paginates
   internally and aggregates. Mention the fanout case explicitly in the map
   description: "for a gene list against a high-fanout dataset (msigdb, go,
   reactome), use biobtree_enrich".
4. Keep the v2.19.1 truncation safety net (token invalidation +
   `dropped_inputs`) for pathological single entries.

## Tests to add with the change

- Integration (`type: "pagination"`): 10 genes → `>>hgnc>>msigdb` with the
  adaptive limit returns all 10 mappings under 50 KB, and following the token
  still yields new targets (no overlap, no loss).
- Unit: retry picks a limit that fits; a single oversized entry still truncates
  safely.
- Regression: `limit` smaller than the default does not rewind or skip targets
  across pages.

## Acceptance criteria

- A 10-gene map to MSigDB fits the MCP budget in one call with all inputs.
- A caller can paginate a `limit=N` result to completion without duplicates.
- No hard cap on input terms; low-fanout chains behave exactly as today.
