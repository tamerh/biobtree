# Internal: per-dataset statistics — how they are computed and what is wrong

Status: all fixes implemented (B1-B6) — next update/generate picks them up · Last updated: 2026-11-06 · Context: main `db_v55` generating

This note records how each field in `dataset_state.json` is produced, and the
bugs found while checking it. Nothing here has been changed in code.

## The three independent counters

`dataset_state.json` mixes three different notions under similar names. They are
not comparable with each other.

### 1. `forward_edges` / `reverse_edges` / `total_edges`

Produced during the **update** phase by `HybridWriterPool` (`src/update/bucket_writer.go`):

- Every `addXref` call writes one forward line (`{source}/forward/`) and one
  reverse line (`{target}/from_{source}/`).
- `forwardXrefStats[source][target]++` on a successful forward write.
- `reverseXrefStats[source][target]++` on a successful reverse write (the source
  is attributed even though the line lands in the target's index).
- `addProp3` (entry properties) writes a forward line with target `"entry"`.
- `addInterval` (interval index) and text links write reverse lines with target
  `"textsearch"` (dataset ID `0`).

At the end of the update, `SetForwardEdges` / `SetReverseEdges`
(`src/update/update.go` ~305 and ~1500) replace the whole map for each dataset in
the run, and `recalculateTotal` sets:

```
total_edges = sum(forward_edges) + sum(reverse_edges)
```

So `total_edges` is **storage rows**: forward rows plus reverse rows. It is not a
count of logical relations.

### 2. `db_keys` / `db_values`

Produced during the **generate** phase by `Merge` (`src/generate/mergeg.go`):

- `perDatasetStats[datasetID].Keys++` for each `Xref` block written under that
  dataset's ID.
- `.Values += totalCount` for the xref entries in that block.
- Saved at the end of generate via `SetAllDatasetDBStats` into the existing state
  entry for `config.DataconfIDIntToString[datasetID]`.

These count rows in the merged index file named after the dataset. They include
**inbound reverse contributions** from other datasets, and do not count edges the
dataset wrote into other indexes. They are index/row counts, not graph edges.

### 3. Durations and misc

- `process_duration` — set by `MarkDatasetProcessed` from the update loop.
- `post_process_duration` — set by `SetPostProcessDuration(datasetName,
  concatDuration)` where `concatDuration` is the sort+concat time for the whole
  run, applied to every dataset in the run.
- `xref_count` — `MarkDatasetProcessed(..., 0, ...)` is called with a literal 0,
  so this field is always 0 (and omitted by `omitempty`).

## Semantics: three layers, not three edge sets

These are not bugs; they are why one "total" cannot mean one thing. The numbers
are accurate, the labels are not.

### A. Index edges are real edges, but a different layer

`reverse_edges["textsearch"]` counts edges of the **index layer**:

- text: `term -> entry` (one per token/synonym)
- interval: `1 kb bin -> feature` (one per bin a feature spans)

They are genuine edges in the database graph, but they are not entity-to-entity
relations. Both are synthetic keys pointing at an entity, like a book index.
They belong in their own category, not mixed with entity xrefs.

Measured in the current state:

| Dataset | total_edges | of which index edges |
|---|---|---|
| dbvar | 10,281,224 | 9,394,296 (91%) |
| vega | 219,334 | 55,055 |
| msigdb | 16,548,416 | 35,361 |
| wikipathways | 163,445 | 9,411 |

dbVar's 9.39 M index edges are interval bins from large SVs (~111 bins per
record). A 100 kb SV becomes ~100 `bin -> feature` edges.

### B. `forward_edges` and `reverse_edges` are the same edge stored twice

- `forward_edges[t]` = edges this dataset wrote toward `t`.
- `reverse_edges[t]` = the reverse-index copy of **the same edges**.

Both are needed so traversal works in either direction, and both are valid
storage counters. But summing them counts every xref twice. Note also that
`reverse_edges` is **not** "inbound": inbound would be other datasets'
`forward_edges[this]`.

The decomposition is exact:

| Dataset | forward | = props | + xrefs | reverse | = xrefs | + index |
|---|---|---|---|---|---|---|
| dbvar | 485,625 | 84,322 | 401,303 | 9,795,599 | 401,303 | 9,394,296 |
| vega | 109,667 | 55,055 | 54,612 | 109,667 | 54,612 | 55,055 |
| mirdb | 9,593,753 | 7,086 | 9,586,667 | 9,586,667 | 9,586,667 | 0 |

Every row balances. This is the strongest argument for keeping the raw counters
and adding labelled derived views instead of redefining one total.

### C. Properties are node payload, not relations

`addProp3` writes a forward row with target `"entry"`, one per entity. That is
the entity's attributes (name, coordinates, AF, ...), not an edge. For dbvar it
is 84,322; for vega 55,055 of 109,667.

## Real bugs (to fix)

### B1. A run can clobber another dataset's statistics (high impact, already hit)

`SetForwardEdges`/`SetReverseEdges` **replace** the map of whatever source dataset
a write was attributed to, even if that dataset was not being rebuilt.

Concrete case: the first miRDB version wrote gene links as `hgnc -> mirdb`
(forward from hgnc). The `--only mirdb` run therefore wrote 1.59 M forward lines
into `hgnc/forward/` and then called `SetForwardEdges("hgnc", {mirdb: 1,594,789})`,
replacing HGNC's real map. Current state for `hgnc` is only `{mirdb: ...}` even
though `hgnc_sorted` (Nov 5 09:10) is clean and contains all its other xrefs.

The bad bucket files are still on disk (`hgnc/forward/`, 90 files, Nov 5 19:44,
sample line `HGNC:100  10  HSA-LET-7F-2-3P  125`). They are not in any sorted
index, and `CleanupForIncrementalUpdate` deletes `{dataset}/forward/` on the next
HGNC rebuild, so data is self-healing — but the **statistics stay wrong** until
HGNC is rebuilt.

Reproduced without RefSeq using `msigdb -> go`: `msigdb` writes GO terms with
`from = GO's dataset id`, so a `msigdb`-only run replaces GO's stats:

| | go forward_edges | go reverse_edges | total |
|---|---|---|---|
| after building `go` | `{entry:3, go:4, gochild:2, goparent:2}` | `{textsearch:8}` | 19 |
| after `msigdb` only | `{msigdb:1}` | `{msigdb:1}` | 2 |

GO's own edges are gone from the state even though its index is untouched.
`hpo` is affected the same way in the same run.

### B2. `post_process_duration` is not per-dataset (low)

It is the whole-run sort+concat duration copied onto every dataset in the run.
Two datasets processed in the same run always show the same value.

### B3. Stale DB stats are never cleared (medium)

`SetAllDatasetDBStats` only writes datasets present in `perDatasetStats`; it does
not zero entries that disappeared. A dataset that shrinks to zero keeps its old
`db_keys`/`db_values`. A dataset that has never completed a generate in that
output dir has `null` (e.g. `wikipathways` during the current run).

Reproduced in a versioned throwaway dir (`out_stale`):

| step | wikipathways `db_keys` / `db_values` |
|---|---|
| generate v1 (with its index files) | 1956 / 78687 |
| remove `*wikipathways*.index.gz`, generate v2 | **1956 / 78687** (unchanged) |

The v2 log says `Per-dataset DB stats: 13 datasets tracked` and does not include
wikipathways, but the old numbers remain in the state.

### B4. Completeness check uses the mixed metric (medium)

`SaveDatasetState` compares `TotalEdges` against the previous build and warns at
<70%. Because `TotalEdges` includes properties, text tokens, interval bins, and
double-counted xrefs, a real 40% drop in xrefs can be masked by unchanged text
volume. Hubs whose content is mostly inbound (e.g. `go`) have tiny own-edge
totals, so inbound loss is not detected at all.

### B5. Dead/misleading helpers (low)

- `AddReverseEdges` is unused and **assigns** instead of adding despite its name.
- `xref_count` is always 0.
- `TotalEdges` is documented as "sum(forward)+sum(reverse)", which is accurate
  but easy to misread as logical edges.

### B6. Generate progress denominator counts `entry` from other federations (high impact)

`Merge.Merge` builds the progress denominator (`totalkvLine`) from
`dataset_state.json`:

- datasets in the federation: add `total_edges`
- datasets in other federations: add every `forward_edges[target]` whose target
  is in this federation

The second rule treats the pseudo-target `"entry"` as a dataset, because
`datasetFederationMap["entry"]` is missing and the code defaults unknown names to
`"main"`. Property rows for `cadd`, `conservation`, `esm1b`,
`gnomad_variant`, … are therefore counted as incoming lines to main, even though
they live in their own federation's index and are never read during main's
merge.

Measured on the running `db_v55` generate:

```
main total_edges                 11,954,367,423
reverse rows -> other federations   375,966,740   (also not read by main)
cross-fed forward as logged      18,975,031,055
  of which "entry" (wrong)       14,796,665,591
cross-fed real targets            4,178,365,464
logged denominator               30,929,398,478   <- matches log
corrected denominator            15,756,766,147
inflation                                  1.96x
```

At 6.68 B lines read the log says 21.6%; the real figure is 42.4%. That is the
"estimate never reaches 100%" symptom.

Small end-to-end reproduction (two federations, `vega` moved to `testfed`,
`wikipathways` in `main`):

```
Federation 'main': total edges from 1 datasets = 163445,
                   cross-federation incoming edges = 109667
Starting merge: 273112 total KV lines to process
actual lines in main/index: 218049   (max progress 79.8%)
```

`109,667` = `54612` real `ensembl_from_vega` lines + `55055` `entry` rows that
do not exist anywhere in main's index.

## Fixes applied (2026-11-06)

### Derived fields (A/B/C)

`DatasetBuildInfo` now carries, computed in `recalculateTotal` and backfilled on
`LoadDatasetState`:

| field | meaning |
|---|---|
| `properties` | `forward_edges["entry"]` |
| `xref_edges` | sum(forward) - properties |
| `index_edges` | `reverse_edges["textsearch"]` |
| `storage_rows` | sum(forward) + sum(reverse), same as `total_edges` |

`total_edges` is unchanged for compatibility. No query behaviour changed.

### Clobber guard (B1)

`DataUpdate.applyEdgeStats` now replaces a dataset's counters only if that
dataset's parser completed in this run. `showProgres` records completed parsers
in `parsedDatasets`; `progWg` makes the stats step wait for that bookkeeping to
drain. Cross-dataset writes (msigdb -> go/hpo, old mirdb -> hgnc) are skipped and
logged:

```
Edge stats: kept previous counters for 2 dataset(s) not rebuilt in this run
```

For datasets that are replaced, both maps are assigned even when empty, so a
rebuild that produces nothing clears stale counters instead of keeping them.
The resume path keeps the old "replace everything" behaviour, since all bucket
files there belong to the interrupted run.

Verified:

- tiny GO: 19 rows -> after `msigdb`-only still 19 rows (previously 2)
- normal `wikipathways` update: `{total 163445, properties 1956, xref_edges 76039,
  index_edges 9411, storage_rows 163445}`
- `python3 tests/run_tests.py wikipathways` passes

### Per-dataset post-process duration (B2)

`SortAllBuckets` now records sort time per dataset (`BucketStats.PerDatasetDuration`
does the same for concatenation). `applyEdgeStats` stores sort+concat for each
dataset instead of copying the whole-run duration onto all of them. Fallback to
the whole-run duration only for parsed datasets that produced no stats.

### DB stats reset (B3)

`SetAllDatasetDBStats` now takes the federation name and resets `db_keys` /
`db_values` for datasets in that federation that are absent from
`perDatasetStats`. Verified with the `out_stale` scenario: v2 now logs
`Reset DB stats for 1 dataset(s) ...` and wikipathways becomes `null` instead of
keeping `1956 / 78687`.

### Completeness on forward rows (B4)

The check now compares `properties + xref_edges` (`forwardRows`) instead of the
mixed storage total, so text/interval volume cannot mask a real drop and
property-only datasets (e.g. cadd) are not treated as empty. `SaveDatasetState`
backfills derived views on the state it re-reads, so old files are handled.

### Generate denominator (B6)

The cross-federation loop skips `entry`/`textsearch`, and reverse rows written
into another federation are subtracted. Verified with the two-federation repro:

```
before: cross-federation incoming = 109667, Starting merge: 273112
after:  cross-federation incoming =  54612, Starting merge: 218057
actual index lines: 218049
```

On the real main federation this changes the estimate from 30.9 B to ~15.8 B.

### Dead/misleading helpers (B5)

`AddReverseEdges` now adds instead of assigning (and recalculates), and
`xref_count` was removed from the state (it was always 0).

Note: the Go test files in `src/update` cannot compile (`update_test.go` imports
`generate`, which imports `update`), so these fixes are covered by the integration
reproductions above rather than Go unit tests.

## Proposed model (remaining)

Keep the raw counters exactly as they are — they are accurate storage counters.
Add labelled views so the UI/user chooses what to show, and stop calling the sum
"edges".

1. ~~Add derived fields.~~ Done (see above).
2. ~~Keep `total_edges` as an alias of `storage_rows`.~~ Done.
3. ~~Skip/merge stats for sources not rebuilt in this run.~~ Done (skip).
4. ~~Set `post_process_duration` per dataset.~~ Done (sort + concat per dataset).
5. ~~Zero DB stats for datasets absent from `perDatasetStats`.~~ Done.
6. ~~Base the completeness check on forward rows.~~ Done.
7. ~~Drop `AddReverseEdges` misuse and `xref_count`.~~ Done.
8. ~~Fix the generate denominator.~~ Done (skip pseudo-targets, subtract
   cross-federation reverse rows).

All items are implemented; the next update/generate will produce corrected
state. `db_v55` itself was built with the old binary.

## Reproducing on small datasets

Before changing code, reproduce the three layers on a throwaway DB:

```bash
make build
./biobtree --lookupdb --force --clean -d "vega,mirdb,dbvar,wikipathways" test
jq '.datasets | map_values({forward_edges, reverse_edges, total_edges})' \
  test_out/dataset_state.json
```

Expected, for each dataset: `sum(forward) = properties + xrefs`,
`sum(reverse) = xrefs + index`, and `total_edges = sum(forward) + sum(reverse)`.
If the identities hold, the counters are behaving as described and the work is
about labels/derived views plus the bugs in B1-B5.

### Confirmed reproduction (2026-11-06, `test_out`, fresh binary)

| Dataset | entries | forward | = props | + xrefs | reverse | = xrefs | + index | total | index share |
|---|---|---|---|---|---|---|---|---|---|
| vega | 55,055 | 109,667 | 55,055 | 54,612 | 109,667 | 54,612 | 55,055 | 219,334 | 25% |
| mirdb | 100 | 126,746 | 100 | 126,646 | 126,646 | 126,646 | 0 | 253,392 | 0% |
| dbvar | 500 | 3,476 | 500 | 2,976 | 96,610 | 2,976 | 93,634 | 100,086 | 94% |
| wikipathways | 49 | 1,251 | 49 | 1,202 | 1,398 | 1,202 | 196 | 2,649 | 7% |

All identities hold exactly. Even a 500-record dbVar test is 94% interval-index
edges, which is the clearest illustration of why one total cannot mean "edges".

Also confirmed in the same run: `post_process_duration` is `1s` for all four
datasets, i.e. the whole-run sort+concat time, not a per-dataset measurement
(B2).

## Reproduction lab

All reproductions live in a throwaway directory (never `out_prod`):

- **Semantics (A/B/C)** — the table above, from
  `./biobtree -d "vega,mirdb,dbvar,wikipathways" test`.
- **ETA denominator (B6)** — two federations: copy `conf/`, set
  `vega.federation="testfed"`, build `vega` + `wikipathways`, then
  `generate --federation main`. The log's `cross-federation incoming edges`
  includes vega's `entry` rows; actual `main/index` lines are 218,049 vs the
  logged 273,112.
- **Clobber (B1)** — build a tiny GO (`go.owl` with 3 classes, `useLocalFile`),
  then `test -d go`, then `test -d msigdb` in the same `test_out`. GO's stats go
  from 19 rows to `{msigdb:1}`. No RefSeq needed.
- **Stale DB stats (B3)** — versioned throwaway dir: build `hgnc,wikipathways`,
  generate v1, delete `*wikipathways*.index.gz`, generate v2. `db_keys` stays
  1956 in both.

Do not run the `test` subcommand twice into the same `test_out` if you need the
second generate to succeed: the second run appends into the existing dev DB and
can panic with `MDB_KEYEXIST` on the `GENOME` meta key. The update/state part
still completes, which is what the clobber check needs.

## Verified counts (authoritative for the paper)

The state counters are **write-time**. The merge dedups each xref by
`key + target dataset + value`, and some datasets write duplicates (GO 6.9%
within one file, string 2x), so the state overcounts. Stale sorted files from
older chunk runs add more copies (the merge dedups them too).

`src/cmd/statsverify` recomputes the post-dedup numbers from the sorted index
files:

```bash
cd src
go run ./cmd/statsverify \
  -index ../out_prod/main/index,../out_prod/gnomad/index,../out_prod/dbsnp/index,../out_prod/cadd/index,../out_prod/conservation/index,../out_prod/predictions/index \
  -state ../out_prod/dataset_state.json \
  -conf ../conf \
  > ../out_prod/stats_verified_all.csv
```

Pass every federation's `index` directory in one run so `edges_in` includes
cross-federation inbound edges. A single-directory run is fine for out-edges but
gives partial inbound.

Output columns: `dataset, multi_file, nodes_state, nodes_actual, nodes_delta,
edges_out_state, edges_out_actual, edges_out_delta, index_edges_state,
index_edges_actual, index_edges_delta, edges_in_actual`.

Rules it applies:

- `nodes` = unique keys that have an entry/property row (target `-1`).
- `edges_out` = unique `(key, target dataset, value)` rows in
  `{dataset}_sorted`.
- `index_edges` = unique `(key, value)` rows in `textsearch_{dataset}_sorted`.
- `edges_in` = sum of other datasets' unique out-edges whose target is this
  dataset. Only complete when every dataset in the same index directory is
  scanned.
- If a dataset has several `{dataset}_sorted.*` files (stale chunk runs), the
  newest is used and `multi_file=yes` is reported.

Known current caveats:

- `cellxgene_celltype` still differs after newest-file selection; rebuild or
  clean its stale files before using its numbers.
- `esm1b` (predictions) and `protein_similarity` (main) have **no sorted index
  files** in `out_prod`; their state rows are stale and they are absent from the
  verified table. Rebuild them before any generate, otherwise the new DB will
  silently omit them.
- A dataset whose properties are written more than once has
  `state properties > verified nodes`, which makes `state xref_edges`
  (= forward writes − property writes) *lower* than the verified unique xrefs.
  This is why a few datasets show `edges_out_actual > edges_out_state`; it is
  not missing data. Use the verified columns.
- Counts are per federation index directory. Run with all six `index` dirs for
  complete cross-federation inbound. Results: `out_prod/stats_verified_all.csv`
  / `.json` (CTD rows refreshed after its update; raw scan kept as
  `stats_verified_all_scan.csv`).

## Verification commands

```bash
# per-dataset state (projected)
jq '.datasets.dbvar | {forward_edges, reverse_edges, total_edges, db_keys, db_values}' \
  out_prod/dataset_state.json

# confirm the hgnc clobber
jq '.datasets.hgnc | {forward_edges, reverse_edges, last_build_time}' \
  out_prod/dataset_state.json
zcat out_prod/main/index/hgnc/forward/bucket_010.txt.gz | head -2

# textsearch contribution
jq '.datasets.dbvar.reverse_edges.textsearch' out_prod/dataset_state.json

# rows in an index file vs stated edges
zcat out_prod/main/index/dbvar_sorted.*.index.gz | wc -l
```

## Immediate consequence

The running `db_v55` generate does not change `forward_edges`/`reverse_edges`
(those come from the update phase). HGNC's stats will still show only `mirdb`
after v55. A `--only hgnc --force` rebuild fixes both the stats and the stray
forward buckets; that is an update, not a generate, but it should be scheduled
deliberately.
