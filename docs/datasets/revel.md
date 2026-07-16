# REVEL Dataset

## Overview

**REVEL** (Rare Exome Variant Ensemble Learner) is an **ensemble missense
pathogenicity** predictor (score 0–1, higher = more likely pathogenic). It is the
clinically-familiar, ClinGen-calibrated second missense predictor alongside
AlphaMissense — its value is recognition + published **ACMG PP3/BP4 evidence
tiers**, not state-of-the-art accuracy (it recombines older tools).

| Field | Meaning |
|-------|---------|
| `chromosome`, `position` | GRCh38 genomic coordinate (1-based) |
| `ref_allele`, `alt_allele` | Alleles |
| `aaref`, `aaalt` | Reference / alternate amino acid |
| `revel` | REVEL score (0–1, higher = more pathogenic) |
| `transcript_ids` | Ensembl transcript(s) the score applies to |

**Data Type**: per-missense-variant score
**Assembly**: GRCh38 · **Dataset ID**: `810` · **Federation**: `predictions`

## ClinGen / Pejaver-2022 calibrated thresholds

| REVEL | ACMG evidence |
|-------|---------------|
| ≥ 0.932 | PP3_Strong |
| 0.773–0.932 | PP3_Moderate |
| 0.644–0.773 | PP3_Supporting |
| 0.290–0.644 | indeterminate |
| 0.183–0.290 | BP4_Supporting |
| 0.016–0.183 | BP4_Moderate |
| ≤ 0.016 | BP4_Strong / VeryStrong |

Per ClinGen SVI, only **one** calibrated predictor should carry ACMG weight per
variant — REVEL is best used as a concordance signal, not additive evidence.

## Source & License

- Source: **REVEL v1.3** (Ioannidis et al. 2016, AJHG) — Zenodo DOI
  `10.5281/zenodo.7072866`, `revel-v1.3_all_chromosomes.zip` (CSV, ~82 M rows,
  both hg19 + GRCh38 coordinates in one file).
- License: dual — the site states "free for non-commercial", the Zenodo deposit is
  **ODbL**. **Ingest: OK; KG-export: EXCLUDED** (ODbL share-alike is incompatible
  with the CC BY-NC-SA export, same posture as gnomAD).

## Integration Architecture

- **`revel`** (id 810): one record per missense variant, keyed
  **`chr:pos:ref:alt`** (GRCh38, from the `grch38_pos` column), co-locating with
  AlphaMissense/SpliceAI on the same variant coordinate.
- Multi-transcript rows for one variant are collapsed to the **max REVEL** with all
  transcript ids retained; the `;`-separated `Ensembl_transcriptid` field is split;
  rows with a blank `grch38_pos` (ambiguous liftover) are skipped.
- `hasFilter: yes` — e.g. `revel.revel>0.9`. `compact_fields`: `revel,aaref,aaalt`.
- Looked up **by the variant's coordinate** (direct entry lookup); cross-refs to
  `transcript`.

## Committed vs production path

Committed conf `path` = the test fixture; production ingest overrides with the real
REVEL CSV (`--revel.file raw_data/revel/revel_grch38_all.csv`, via `bb.sh`
`OPTS_revel`).

## Query Examples

```bash
curl "http://localhost:9292/ws/entry/?i=1:943702:T:C&s=revel"
curl "http://localhost:9292/ws/?i=1:943702:T:C&s=revel&d=1&f=revel.revel>0.9"
```
