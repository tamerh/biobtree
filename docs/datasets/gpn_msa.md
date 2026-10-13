# GPN-MSA Dataset

## Overview

**GPN-MSA** (Benegas et al., *Nat Biotechnol* 2025) — an **allele-specific**
genomic language-model score. It's the allele-aware complement to biobtree's
`conservation` layer (phyloP/phastCons are position-only): GPN-MSA scores each
specific substitution. More negative = more constrained/deleterious.

Keyed `chr:pos:ref:alt` (GRCh38), **score-only** (1 KV per variant, no links) —
the same minimal, direct-entry-lookup shape as `cadd`/`conservation`. Genome-wide
(~9B SNVs) so it lives in its **own `gpn_msa` federation**.

**License: MIT** — fully redistributable (export-clean, unlike CADD/REVEL).
**Caveat:** raw score, **NOT ACMG-calibrated** (evidence, not a classifier — same
status as raw CADD). **SNV-only** (no indels).

**Dataset ID:** `822` · **Federation:** `gpn_msa`

## Access

```
# direct variant lookup
/ws/?i=1:69091:A:G&s=gpn_msa&d=1      # -> GpnMsa { score }
# filter by constraint
/ws/?i=<ids>&d=1&f=double(gpn_msa.score)<-5.0
```

## Source & build

- **Source:** HuggingFace `songlab/gpn-msa-hg38-scores` — ~81 GB bgz, tabix-indexed,
  columns `chrom pos ref alt score`, all ~9B hg38 SNVs.
  ⚠ **Verify the exact bgz filename/URL before building** (conf `path` is a
  best-guess placeholder).
- **Parser:** `src/update/gpn_msa.go` — a near-literal clone of `cadd.go`
  (score-only attr, no links). Test mode reads `tests/datasets/gpn_msa/gpn_msa_fixture.tsv`.
- **Build (its own federation, ~360 GB, ~12–15h):**
  ```
  ./bb.sh out_prod --only gpn_msa --force --generate-after
  ```
  Build its dir on `/data` (like `cadd`) to keep it off `/data2`.

## Notes
- Billion-scale; excluded from the normal `main` DATASETS loop (built explicitly),
  exactly like `cadd`/`dbsnp`.
- Orthogonal axis (nucleotide, allele-aware) vs AlphaMissense (protein) / SpliceAI
  (splice) / phyloP (position conservation) — see VARIANT_PREDICTOR_ROADMAP.md.
