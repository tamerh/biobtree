# SpliceAI Dataset

## Overview

Per-variant splice-disruption predictions. biobtree sources SpliceAI from the
**Ensembl MANE** VCFs (MANE v1.4, replacing the old GENCODE-v24 Illumina tables), all
SNV alts, and the **four delta scores**. Keyed `chr:pos:ref:alt` (GRCh38). Kept at
**raw delta ≥ 0.2** (ClinGen SVI's Walker et al. 2023 PP3/BP4 calibration used raw).

Three sources merge into the single `spliceai` dataset (one entry per variant):
- `path` — raw SNV scores → `ds_ag/al/dg/dl` (+ positions `dp_*`)
- `pathMasked` — masked SNV scores → `ds_ag_masked/al/dg/dl`. Masking zeroes a loss
  at an unannotated site and a gain that only strengthens an existing site; this is
  what **Ensembl VEP** and the **Broad lookup** serve. Masking only ever zeroes a
  delta (`masked ≤ raw`), so `raw ≥ 0.2` is already the raw∪masked union — no variant
  is masked-only. A kept variant can therefore carry all-zero masked deltas, which is
  real negative evidence ("masked says no effect"), distinct from "not in the release".
- `pathIndel` — Illumina GENCODE-based **indel** scores (Ensembl publishes SNV-only).
  Raw-only (no masked indel file exists); `ds_*_masked` are empty for indels. GENCODE-
  rather than MANE-based, so not an exact transcript match to the SNV set. Same CC BY-NC
  licence; KG-excluded like the SNV scores.

**Which to display:** `ds_*` (raw) is the ClinGen-calibrated set for PP3/BP4; `ds_*_masked`
matches VEP/Broad. `/ws/meta` carries `scoreVariant`, `threshold` and `keyScheme` so a
consumer can render the right sentence (and tell "below threshold" from "not assessed").

## Attributes
`chromosome`, `position`, `ref_allele`, `alt_allele`, `effect` (dominant: acceptor/donor
gain/loss), `score` (max delta), `gene_symbol` (MANE), and the four deltas + positions:
`ds_ag`, `ds_al`, `ds_dg`, `ds_dl`, `dp_ag`, `dp_al`, `dp_dg`, `dp_dl` (strings, so a
meaningful 0.00 survives).

```bash
curl "http://localhost:9292/ws/?i=2:175937343:C:G&s=spliceai"   # all alts kept; donor_loss + DS_DL
curl "http://localhost:9292/ws/map/?i=BRCA1&m=>>hgnc>>spliceai"
```

## Notes
- Migration from the Broad/tgg-viewer Illumina (GENCODE v24) source fixes Sugi #1 (all
  alts), #2 (the four deltas → correct gain-vs-loss mechanism) and #4 (stale annotation).
  The old `process_spliceai.py` prep script is retired (the MANE VCF is read directly).
- SNV-only (the MANE release has no indels). Dataset id 124, main federation.
