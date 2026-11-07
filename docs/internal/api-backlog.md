# Internal: API and dataset backlog

Status: open (future work, not scheduled) · Last updated: 2026-11-06 · Context: v2.20.1, main `db_v55`

Items reported by benchmark/preprint agents but deliberately not fixed now.
The filter report (#1 below) was investigated and closed as not reproducible.

## 1. Map pagination token ergonomics (open)

**Problem.** The map `next_token` is internal page state:

```
-1[]HGNC:2345,10,HGNC:2345,75,3][
```

It contains `[`, `]` and commas. In the LAB-Bench run the model tried to "fix"
it (appended `[`, dropped `]`), got `Invalid paging request`, and looped. The
practical workaround (`[id=="..."]`) is not discoverable from the token.

This is genuine pagination, separate from the MCP structural truncation.

**Options.**

- **A (recommended, small): base64url-encode the token at the API boundary.**
  Decode in `parsePagingKey`; continue accepting legacy bracketed tokens.
  Document "pass next_token verbatim, do not edit". Removes the copy/mangle
  class without changing pagination semantics.
- **B (larger): integer `page=N` / `offset` for single-input maps.** Natural for
  the common case, but ambiguous for multi-input tokens and needs server-side
  replay or state.

**Acceptance.** Token contains only URL-safe characters; a corrupted token
returns a clear error (not zero results); legacy tokens still parse; tests in
`src/service/mapfilter_test.go`.

## 2. Ensembl cytoband (open, optional feature)

**Problem.** `EnsemblAttr` has `seq_region`, `start`, `end`, `biotype`, but no
cytoband. Ensembl REST `lookup/id` does not return `band`:

```
{"seq_region_name":"17","start":7661779,"end":7687546,"biotype":"protein_coding", ...}
```

So "which gene is at chr3p25 according to this Ensembl release" cannot be
answered from Ensembl alone. HGNC `location` covers human genes but can disagree
with older Ensembl releases.

**Implementation sketch.**

1. Obtain karyotype bands per assembly (Ensembl REST
   `/info/assembly/:species` returns `karyotype[].band/start/end/stain`, or the
   equivalent karyotype file).
2. Add `cytoband` to `EnsemblAttr` in `src/pbuf/attr.proto` (run `make proto`).
3. In the Ensembl parser, index bands per seq region and assign the band that
   contains `start`/`end` when building each gene attribute.
4. Regenerate the DB (`ensembl` is core, so this is a full build).

**Cost.** Proto + parser + a new per-assembly data source; not a small fix. Only
worth it if release-specific cytoband answers are required by the benchmark.

## 3. Benchmark feedback (2026-11-07)

Four items from the preprint/benchmark agent. Status per item.

### 3.1 GO-native enrichment and human GO annotations (open, largest gap)

`biobtree_enrich` scores MSigDB collections only. Terms that have no `C5` set
can never be returned (e.g. `GOBP_NEGATIVE_REGULATION_OF_ANION_TRANSMEMBRANE_TRANSPORT`,
`PENILE_ERECTION`, `GUANYLATE_CYCLASE_ACTIVITY`).

A GO-native mode would also need better human annotations: `GO:1903960` currently
maps to a single **mouse** gene (`ENSMUSG00000037012`, Hk1). Work:

- add a human GO annotation source (GOA/GOC) rather than relying on the
  cross-species Ensembl GO links, and
- add an enrichment collection that scores GO terms directly from those
  annotations.

This is a feature, not a quick fix.

### 3.2 dbSNP merged/retired rs IDs (open, data gap)

`rs397784008` is not found, but NCBI merged it into `rs59732348`, which exists.
dbSNP publishes `RsMergeArch.bcp.gz`; ingest it as a redirect/alias index
(old rs -> current rs) and rebuild the dbsnp federation (520 GB, so a
deliberate rebuild).

### 3.3 Entrez synonyms — not missing; ranking/ambiguity (investigated)

The Entrez parser **already indexes** `Synonyms` (`entrez.go` fields[4], text
search per synonym). The reported examples do appear:

- `DS` -> `1725` DHPS at rank 4 (top is DHDDS, which also texts as DS)
- `P113` -> STAT2 at rank 2
- `TFA` -> F3 at rank 3
- `Sph1` -> ANK1 first; SNCAIP is also a valid SPH1 alias (ambiguous gold)

A real ranking bug was found and fixed (`cd157a6`): the exact-match bonus
compared each row with **its own keyword** instead of the caller's query, so it
never fired. The remaining cases are genuinely ambiguous alias targets
(ANK1 vs SNCAIP for Sph1), not missing index coverage. If the benchmark needs a
deterministic winner, that requires an official-symbol/alias-source preference,
not more synonyms.

### 3.4 VEGA clone names (closed, source gap)

`RP11-173H9.2` and `CTD-2216L14.1` are absent from the VEGA release actually
used:

```bash
curl -sL https://ftp.ensembl.org/pub/vega/human/Homo_sapiens.VEGA68.68.gtf.gz -o vega68.gtf.gz
zgrep -E "RP11-173H9\.2|CTD-2216L14\.1" vega68.gtf.gz   # no matches
```

The benchmark golds are from a different VEGA release. No BioBTree fix.

### 3.5 Confirmed not gaps

- `NPIPA6` vs `NPIPA9`: Ensembl returns NPIPA6 for `ENSG00000183889`; gold stale.
- `ENSG10010137169.1` / `ENSG10010137820.1`: malformed IDs in the gold CSV.
- `SGO1` vs `SGOL1`: BioBTree resolves both to SGO1 (HGNC:25088).
- Gene–disease coverage: known DisGeNET boundary; CTD→gene is the pending lever.
- miRDB `NR_` targets: absent from miRDB v6.0 source.

## 4. Closed / no action

- **Unknown filter functions fail silently** — not reproducible on v2.20.1.
  `id.contains("3190")` works; `id.foo(...)` returns a proper type-check error;
  the reported `ZFPL1` example has no `HSA-MIR-3190-3P` target at all (both
  `==` and `contains` return 0). The `unmapped` field came from the MCP layer,
  not the HTTP map response. Paper team confirmed no issue.
- **miRDB non-coding targets** — miRDB v6.0 ships only `NM_`/`XM_` accessions
  (zero `NR_` rows), so `ZNF22-AS1` (`NR_126421`) is genuinely absent from the
  source. BioBTree ingested the file correctly; no change.
