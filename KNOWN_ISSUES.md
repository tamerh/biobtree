# Known Issues

## 1. Residual build-time nondeterminism in clinical_trials → MONDO mapping (~0.015%)

**Status:** Known, low priority. Build-time only — does NOT affect the live search.

**Symptom:** Two separate `clinical_trials` build runs (same code, same data, same
lookup DB) map a tiny, *intermittent* subset of conditions differently — about
**2–4 conditions out of ~19,000 (~0.015%)** flip between "mapped" and "not mapped".
The flipping set is small and changes from run to run (e.g. `Gonadal Dysgenesis`,
`Muir-Torre Syndrome`, `Diabetes Mellitis Type 2`, `PCP`). They are all
"hard" conditions resolved deep in the normalization cascade.

**Confirmed NOT the cause (each tested directly):**
- ❌ LMDB `NoLock` read mode — dropped `NoLock` (plain `Readonly`), re-ran, no change.
- ❌ Concurrency — `clinical_trials` ingestion is single-threaded (no goroutines).
- ❌ Decode errors skipping trials — instrumented the `decoder.Decode` `continue`
  path, **0** decode errors across runs.
- ❌ Varying trial count — total processed is steady at **587,105** every run.
- ❌ Normalization cascade map iteration — **this WAS a real source and is now fixed**
  (see "Fixed" below); residual persists at similar magnitude, so it is a *separate*
  effect.

**Confirmed solid:**
- ✅ The **live `/ws/` search** is deterministic (same query → identical results, 5/5).
- ✅ `collectOntologyIDs` is deterministic **within a single process** (100/100 calls
  for a fixed condition return the same result).
- ✅ Total trial count and decode behavior are deterministic.

**Leading characterization:** deterministic within a process, varies across separate
processes, on a handful of edge-case conditions only. Points to a subtle per-process
init effect on a few borderline cascade decisions. Not yet root-caused.

**How to investigate later (the expensive path we deferred):** add full cascade-decision
tracing — for every condition, log which cascade step produced the hit and the exact
lookup result — to two separate runs, then diff to isolate the exact per-process source.

**Impact:** ~2–4 trial→disease edges out of millions differ between builds. Negligible
for production; only matters for exact build reproducibility.

---

## 2. Relevance ranking — Phase 2 (cross-dataset tier) not implemented

**Status:** Future enhancement. Phase 1 is done and live.

**Phase 1 (DONE):** text-search results now carry a relevance tier in the
priority field (7): primary-name match > synonym match > partial-word match.
The textsearch bucket sorts each key's entries by it and the merge preserves
that order (stable sort). So within a dataset, the exact/name match leads and
survives the result cap (verified: `lymphoma`, `melanoma`, `neuroblastoma`,
`carcinoma` all return their canonical term first). See `indexSearchText` in
`src/update/update.go` and the stable text-link sorts in `src/generate/mergeg.go`.

**Phase 2 (NOT done):** the tier currently orders entries **within each
dataset** (the merge's text-link sort is primary-keyed on dataset priority, then
stable-preserves the tier). The exact term wins as long as it lives in a
reasonably-prioritized dataset (mondo/efo) — true in the common case. The
**unhandled** case: a query's exact term exists *only* in a low-priority dataset
while a higher-priority dataset contributes many partial-word matches — the
exact term could still be pushed past the cap.

**Phase 2 fix (deferred):** make the tier the PRIMARY sort key across datasets,
above dataset priority. Requires threading the tier into the merge's line parse
(`[6]string` → `[7]string` in `mergeg.go`) so it survives to the text-link sort,
rather than being stripped after the bucket sort. Hot-path change → only do it
if Phase 1's within-dataset ordering proves insufficient in practice.

---

### Fixed in the same work (for context — these are NOT issues, they are resolved)
- Normalization cascade (`ApplySpellingVariations`, `ApplyCancerAbbreviations`,
  `ApplySpecificPatterns`, `ApplyAnatomicalTerms`, `DiseaseCorrections`) now iterate
  **sorted keys** instead of randomized Go maps → deterministic variant selection.
- `/ws/` paged results (page 2+) now iterate **sorted keys** (`mapfilter.go`) →
  stable pagination.
- Lookup failures in the disease cascade now **retry once and log** instead of being
  silently swallowed.

## 3. Generate progress percentage caps below 100% for value-dense federations (cosmetic)

**Status:** Known, low priority. Display-only — does NOT affect the generated DB.

**Symptom:** The `generate` progress log (`Progress:` / `Checkpoint:` lines) tops out
around **~64%** for the **dbsnp** federation even though the build completes correctly
and writes the full database. Other federations (e.g. `main`) read closer to 100%.

**Root cause:** the percentage mixes two different units
(`src/generate/mergeg.go:506` and `:551`):
```
progressPercent = float64(d.totalLinesRead) / float64(d.totalkvLine) * 100
```
- numerator `totalLinesRead` = **physical lines** read from the index files
- denominator `totalkvLine` = **key–value pairs**

A single physical line can carry a key plus *multiple* values, so KV-pairs > lines.
For the last dbsnp build: `5,630,657,282` lines vs `8,698,327,811` KV-pairs →
`5.63B / 8.70B = 64.7%`. dbSNP is value-dense (many values per line) so its ratio is
~0.65; sparse datasets (≈1 value/line) read ~100%.

**Confirmed NOT a data problem:** dbsnp `db_v2` is complete — 493 GB on disk,
`totalKVLine 8,698,327,811` (slightly *larger* than `db_v1`'s 8,698,248,493 due to the
added pharmgkb_var_annotation reverse links). All input files were read to completion.

**Fix options (not yet applied — pending decision):**
- (a) make the numerator count KV-pairs (increment by values-per-line) so units match — accurate but touches the merge inner loop;
- (b) make the denominator the expected *physical-line* total instead of KV-pairs — smaller change.
