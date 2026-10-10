# gnomAD Regional Missense Constraint (gnomad_rmc) Dataset

## Overview

`gnomad_rmc` is gnomAD v4.1.1 **regional missense constraint** — missense-constrained
regions (MCRs). Each protein-coding transcript is split into sub-regions, each with
observed/expected missense counts and the O/E ratio. It carries the first
**ClinGen-calibrated regional threshold**: a region with **oe < 0.36** gives moderate
pathogenic support for missense variants falling in it.

**Source**: `gs://gcp-public-data--gnomad/papers/2026-rmc/gnomad_v4.1.1_all_mcrs.tsv`
(CC0, GRCh38, ~40k regions).
**MPC** (the per-variant score) ships only as a Hail Table in the same release and needs a
Hail export step, so it is **not** ingested here — only the regional constraint.

## Access

Interval-indexed, so a variant coordinate resolves to its covering region:

```bash
# variant position -> covering constrained region
curl "http://localhost:9292/ws/map/?i=1:67000&m=>>gnomad_rmc"       # -> ENST…:65565, oe=0, constrained
curl "http://localhost:9292/ws/map/?i=1:67000:C:A&m=>>gnomad_rmc"   # variant-key form

# from a gene/transcript
curl "http://localhost:9292/ws/map/?i=BRCA1&m=>>hgnc>>ensembl>>transcript>>gnomad_rmc"
```

Each region links to its Ensembl `transcript`. `constrained` (bool) flags oe < 0.36.

## Attributes

`transcript`, `chromosome`, `start`, `end`, `obs`, `exp`, `oe`, `oe_chisq`, `oe_chisq_p`,
`constrained`, `is_high_coverage`. The constraint metrics (obs/exp/oe/chisq) are stored as
strings so a meaningful **oe = 0** (the most-constrained regions) is preserved — proto3
would drop a numeric zero.

## Notes
- Dataset id 815, main federation. Uses the point-in-interval primitive (see encode_ccre).
- CC0; KG-export safe.
