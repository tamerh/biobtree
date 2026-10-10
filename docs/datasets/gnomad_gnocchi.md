# gnomAD Non-coding Constraint — Gnocchi (gnomad_gnocchi)

## Overview

`gnomad_gnocchi` is gnomAD's non-coding **genomic constraint** (Gnocchi; Chen et al.,
Nature 2024;625:92): genome-wide **1 kb windows**, each with a depletion-of-variation
**Z-score** (higher = more constrained). It is the only per-position constraint signal for
**intronic / UTR / intergenic** variants, where coding constraint (gnomad_constraint) and
missense tools don't apply.

**Source**: gnomAD v3.1 `genomic_constraint/constraint_z_genome_1kb.qc` (CC0, hg38).

## Access

Interval-indexed — a variant coordinate resolves to its covering window:

```bash
curl "http://localhost:9292/ws/map/?i=1:783500&m=>>gnomad_gnocchi"      # -> window + z
curl "http://localhost:9292/ws/map/?i=1:783500:A:G&m=>>gnomad_gnocchi"  # variant-key form
```

## Attributes
`chromosome`, `start`, `end`, `z` (Gnocchi Z-score), `oe`, `obs`, `exp`, `possible`.
Metrics are strings for full fidelity. Dataset id 816, main federation, CC0.
