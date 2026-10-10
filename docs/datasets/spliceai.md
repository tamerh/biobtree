# SpliceAI Dataset

## Overview

Per-variant splice-disruption predictions. biobtree now sources SpliceAI from the
**Ensembl MANE** VCF (`spliceai_scores.raw.snv.ensembl_mane_v1.4.grch38.vcf.gz`) — MANE
v1.4 (current annotation, replacing the old GENCODE-v24 Illumina precomputed tables), all
SNV alts, and the **four delta scores**. Keyed `chr:pos:ref:alt` (GRCh38). Loaded at the
**raw**, **delta ≥ 0.2** threshold (ClinGen's Walker et al. calibration used raw).

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
