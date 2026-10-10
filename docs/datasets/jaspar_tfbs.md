# JASPAR TFBS Dataset

## Overview

Genomic **transcription-factor binding-site (TFBS) motif features** — where
JASPAR TF motifs are predicted to occur in the genome. This is the *genomic*
companion to the `jaspar` dataset (which holds only the motif models: matrix_id,
name, class, family). Together they answer: *does a variant fall in a predicted
TF binding site, for which TF, and what is the motif?*

**Scoped source, not a genome-wide scan.** The raw genome-wide JASPAR track is
177 GB and ~99% noise (a short motif at p ≤ 0.05 matches everywhere). Instead this
dataset uses the **Ensembl Regulatory Build motif features** — JASPAR motifs
mapped *only within regulatory features* (promoters, enhancers, CTCF,
TF-binding, open-chromatin). That is the high-signal subset: motif matches that
sit inside an actual regulatory element. ~20 MB source → a small interval dataset.

**Data Type**: interval (genomic motif features)
**Assembly**: GRCh38 / hg38
**Dataset ID**: `819`
**Parent**: `jaspar` (motif-model metadata)

## Access — interval lookup by variant position

An interval dataset: query a variant's position through `/ws/map`:

```
/ws/map/?i=7:5527155&m=>>jaspar_tfbs
# -> ENSM00000000101 { score, strand, binding_matrix_id, transcription_factors:[HNF4A,NR2F1,RXRA] }
```

Each motif feature also links to **hgnc** for every TF it binds, so
`HNF4A >> jaspar_tfbs` returns that TF's motif features (reverse of the per-TF
xref).

Score filtering uses the direct endpoint (`double(jaspar_tfbs.score)`); the
position→interval map path does not thread an inline CEL filter.

## Fields

| field | meaning |
|-------|---------|
| `chromosome`, `start`, `end` | motif feature interval (GRCh38) |
| `score` | Ensembl motif score (string; a meaningful 0 is preserved) |
| `strand` | + / − |
| `binding_matrix_id` | Ensembl PFM id (`ENSPFM…`) for the motif model |
| `transcription_factors` | TF gene symbols bound by this motif (→ hgnc) |

## Source

- `https://ftp.ensembl.org/pub/release-114/regulation/homo_sapiens/GRCh38/annotation/Homo_sapiens.GRCh38.motif_features.v114.gff3.gz`
- GFF3, type `TF_binding_site`; attributes carry `ID`, `binding_matrix_id`, `transcription_factor`.
- License: Ensembl Regulation (freely redistributable; cite JASPAR, NAR 2024;52:D174).

## Notes

- `binding_matrix_id` is an Ensembl PFM id, which is **not** the JASPAR `MA…`
  accession the `jaspar` dataset is keyed by — so there is no direct
  `jaspar_tfbs → jaspar` xref today. Adding an ENSPFM→MA mapping could enable it
  later.
- Other scoping options for the genome-wide track (∩ cCRE, score-threshold, raw)
  are evaluated in the internal `JASPAR_TFBS_OPTIONS.md`.
