# Conservation Dataset

## Overview

Per-position **evolutionary conservation scores** — biobtree's per-base
conservation layer, and the platform's single biggest prior gap (no conservation
data existed before this dataset). Conservation quantifies how strongly a genomic
position is preserved across species: highly-conserved positions tolerate change
poorly, so a variant there is more likely to be functional/pathogenic. This
hardens variant-pathogenicity concordance (cross-checking AlphaMissense / SpliceAI
/ ClinVar calls against independent evolutionary evidence).

Three complementary scores are stored per position:

| Score | Provider | Meaning |
|-------|----------|---------|
| `phylop` | UCSC phyloP470way | Per-base conservation/acceleration; **can be negative** (positive = slower-than-neutral evolution = conserved; negative = accelerated) |
| `phastcons` | UCSC phastCons470way | Probability (0–1) that a base is in a conserved element (HMM-based) |
| `gerp` | GERP++ RS | Rejected-substitutions score; higher = more constrained |

**Data Type**: per-position numeric scores (genome-wide)
**Assembly**: GRCh38 / hg38
**Dataset ID**: `808`

## Why primary sources, not dbNSFP

dbNSFP is the usual one-stop shop for these scores, but it is licensed
**CC BY-NC-ND** (No-Derivatives) — redistributing a *subset* of it is a
derivative and therefore not permitted. So biobtree sources conservation from the
**primary, freely-redistributable providers** instead:

### phyloP (UCSC, hg38 470-way)
- Track: `phyloP470way` (multiz 470-way alignment)
- BigWig (whole genome): `hg38.phyloP470way.bw` (~11 GB, 2023-09-02)
- Dir: `https://hgdownload.soe.ucsc.edu/goldenPath/hg38/phyloP470way/`
- License: UCSC data is freely available for any use.

### phastCons (UCSC, hg38 470-way)
- Track: `phastCons470way` (multiz 470-way alignment)
- BigWig (whole genome): `hg38.phastCons470way.bw` (~4.7 GB, 2023-09-29)
- Dir: `https://hgdownload.soe.ucsc.edu/goldenPath/hg38/phastCons470way/`
- License: UCSC data is freely available for any use.

### GERP++ (RS score)
- GERP++ rejected-substitution scores, distributed via Ensembl Compara and UCSC.
- Ensembl: `https://ftp.ensembl.org/pub/` compara conservation-score dumps.
- UCSC (hg19 legacy) GERP track / SidowLab GERP++ releases.
- License: freely redistributable (GERP++ is academic-open; Ensembl is Apache-2.0/open).

> The bigWig tracks are **not** parsed by biobtree directly (biobtree ingests
> text). The coordinator pre-merges the three tracks into a single per-position
> TSV (see Input Format) using `bigWigToBedGraph` / `bigWigToWig` (UCSC utils),
> then feeds that to this parser. Real data is ~billions of positions and is
> ingested by the coordinator, not in this scaffold.

## Integration Architecture

### Storage Model

- **`conservation`** (id 808): one record per genomic position, keyed
  **`chr:pos`** (e.g. `17:43094464`), with the `chr` prefix stripped and
  chromosome normalized to `1`–`22`, `X`, `Y`, `MT`. Attributes: `chromosome`,
  `position`, `phylop`, `gerp`, `phastcons`.
- `hasFilter: yes` — CEL filtering is wired, e.g. `conservation.phylop > 2.0`,
  `conservation.phastcons >= 0.9`, `conservation.gerp > 4.0`.

### Input Format (pre-merged TSV, gz)

```
# chrom	pos	phylop	gerp	phastcons
1	69094	7.612	5.310	1.000
1	69096	-1.230	-2.410	0.012
```

One row per position. A missing provider value may be empty / `NA` / `.` and is
stored as `0`. Configured at `raw_data/conservation/conservation_hg38.tsv.gz`
(`useLocalFile: yes`). The committed file is a **tiny hand-crafted fixture** (12
positions) for unit tests only — NOT real UCSC/GERP data.

## Key scheme — positional join note (DECISION FOR MERGE REVIEW)

Conservation is keyed **`chr:pos`** (per-position, ref/alt-agnostic). This
**differs** from the variant datasets — `alphamissense`, `spliceai`,
`gnomad_variant`, `clinvar` — which key **`chr:pos:ref:alt`**. Because the keys
are not identical, a variant xref does **not** auto-join to a conservation record.

The intended join is **positional**:

```
variant  "chr:pos:ref:alt"  ->  strip ref/alt  ->  "chr:pos"  ->  conservation lookup
```

This scaffold deliberately does **not** implement that wiring, because it would
require modifying the variant parsers (out of scope). Options for the coordinator
to decide at merge:

1. **Query-time positional lookup** (preferred): when returning a variant, derive
   `chr:pos` from its key and do a secondary conservation lookup in the service
   layer. No re-keying, no parser changes, no data duplication.
2. **Ingest-time xref**: have each variant parser also emit an xref to the
   `chr:pos` conservation key. Simple graph traversal, but touches every variant
   parser and duplicates edges at billion-scale.
3. **Dual-key conservation**: additionally store conservation under
   `chr:pos:ref:alt` for every alt — rejected (multiplies a genome-wide dataset
   by ~3× and conflates position-level scores with allele-level semantics).

Recommendation: **option 1**. Left for review — no other datasets were modified.

## Query Examples

`conservation` is its own federation, so route the query into it with
`&s=conservation` (this is what makes the CEL filter see the conservation
attributes; without it the lookup resolves against `main` and the filter is
federation-shadowed).

```bash
# Per-position lookup (chr:pos)
curl "http://localhost:9292/ws/?i=17:43094464&s=conservation&d=1"

# Filter for highly-conserved positions (10:87864533 has phylop 11.83)
curl "http://localhost:9292/ws/?i=10:87864533&s=conservation&d=1&f=conservation.phylop>5.0"
```
