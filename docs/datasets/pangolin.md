# Pangolin Dataset

## Overview

Pangolin (Zeng & Li, 2022) precomputed splice-disruption scores — an **independent**
predictor to SpliceAI, useful as a cross-check. Precomputed for **all SNVs in
protein-coding genes** (hg38, GENCODE-masked); biobtree keeps the per-variant scores
filtered to **largest delta ≥ 0.2**. Keyed `chr:pos:ref:alt`.

**Source**: Zenodo `Pangolin_hg38_snvs_masked.zip` (record 15649338, **CC BY 4.0**) — a
zip of ~20k per-gene TSVs (one per Ensembl gene).

## Attributes
`chromosome`, `position`, `ref_allele`, `alt_allele`, `gene` (ENSG), `score` (largest
delta = max of gain / |loss|), `effect` (gain | loss), `gain_score`, `loss_score`,
`gain_pos`, `loss_pos`.

```bash
curl "http://localhost:9292/ws/?i=20:44187187:T:G&s=pangolin"       # gain 0.95
curl "http://localhost:9292/ws/map/?i=BRCA1&m=>>ensembl>>pangolin"
```

## Notes
- Independent of SpliceAI (different model); use both for splice evidence. GENCODE-masked
  (no official MANE precompute exists for Pangolin). SNV-only, coding genes. Dataset id
  818, main federation. Parser downloads the 13 GB zip once, iterates its per-gene members.
