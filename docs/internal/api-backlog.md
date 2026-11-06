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

## 3. Closed / no action

- **Unknown filter functions fail silently** — not reproducible on v2.20.1.
  `id.contains("3190")` works; `id.foo(...)` returns a proper type-check error;
  the reported `ZFPL1` example has no `HSA-MIR-3190-3P` target at all (both
  `==` and `contains` return 0). The `unmapped` field came from the MCP layer,
  not the HTTP map response. Paper team confirmed no issue.
- **miRDB non-coding targets** — miRDB v6.0 ships only `NM_`/`XM_` accessions
  (zero `NR_` rows), so `ZNF22-AS1` (`NR_126421`) is genuinely absent from the
  source. BioBTree ingested the file correctly; no change.
