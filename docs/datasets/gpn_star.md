# GPN-Star Dataset

## Overview

**GPN-Star** (Benegas et al., *Nature* 2026) — a **phylogeny-aware** genomic
language-model that predicts genome-wide functional constraint; it supersedes
GPN-MSA (same lab, deprecated). biobtree ingests the **V (100-vertebrate)**
model's **mutation-rate-calibrated** LLR (`llr_calibrated`). More negative = more
constrained/deleterious.

**Why V, not M (447-mammal):** our `conservation` dataset already serves UCSC
**phyloP470way** — a *mammalian* alignment. GPN-Star-M is trained on the same
mammalian alignment family, so it would **double-count** that signal (the ClinGen
SVI double-counting we avoid). GPN-Star-**V** uses a deeper, different (vertebrate)
species set → an **orthogonal** evolutionary axis. The `llr_calibrated` score is
mutation-rate-calibrated, which removes the local-mutation-rate confound that
`gnomad_gnocchi` exists to correct.

Keyed `chr:pos:ref:alt` (GRCh38), **score-only** (1 KV per variant, no links) —
same minimal, direct-entry-lookup shape as `cadd`/`conservation`. Genome-wide
(~9B SNVs) → its own `gpn_star` federation.

**License: Apache-2.0** — fully redistributable (export-clean).
**NOT ACMG-calibrated** by any body → present as **context/evidence, not a
weighted or ACMG-labeled signal** (same status as raw CADD). **SNV-only** (no indels).

**Dataset ID:** `822` · **Federation:** `gpn_star`

## Access

```
/ws/?i=1:69091:A:G&s=gpn_star&d=1            # -> GpnStar { score }
/ws/?i=<ids>&d=1&f=double(gpn_star.score)<-5.0   # filter by constraint
```

## Source & build

- **Source:** HuggingFace `songlab/gpn-star-scores` (Apache-2.0),
  `data/gpn-star-hg38-v100-200m/llr/llr_chr{1..22,X,Y}.parquet` (~59 GB),
  columns `chrom pos ref alt llr_calibrated abs_llr_calibrated`.
- **Prep (Go can't read Parquet):** `python3 src/scripts/gpn_star/gpn_star_prepare.py`
  downloads each chromosome parquet and emits
  `raw_data/gpn_star/gpn_star_v.tsv.gz` (`chrom pos ref alt score`, chrom without
  `chr`). No external sort — the Go side is `addProp3` and generate buckets/sorts.
- **Build (own federation, ~360 GB, ~12–15h):**
  ```
  python3 src/scripts/gpn_star/gpn_star_prepare.py
  ./bb.sh out_prod --only gpn_star --force --generate-after --federation gpn_star
  ```
  `--federation gpn_star` scopes generate to this fed only; build its dir on `/data`.

## Notes
- Billion-scale; excluded from the normal `main` DATASETS loop (built explicitly),
  like `cadd`/`dbsnp`.
- Orthogonal axis (vertebrate nucleotide constraint, allele-aware, rate-calibrated)
  vs AlphaMissense (protein) / SpliceAI (splice) / phyloP470way (mammalian
  conservation) / Gnocchi (non-coding constraint). See VARIANT_PREDICTOR_ROADMAP.md.
- Supersedes the earlier GPN-MSA plan (which tracked local mutation rate and had no
  calibration — the Variant team flagged it 2026-10-13).
