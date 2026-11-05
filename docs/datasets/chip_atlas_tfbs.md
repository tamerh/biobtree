# ChIP-Atlas TFBS Dataset

## Overview

Experimentally called **transcription-factor binding targets** from
[ChIP-Atlas](https://chip-atlas.org) — which genes a TF binds near, based on
public ChIP-seq peaks. This is the experimental counterpart to `jaspar_tfbs`
(predicted motif sites): ChIP-Atlas reports *observed* binding, not a motif
match.

**Data Type**: gene→gene (TF → target gene)
**Genomes**: hg38, hg19, mm10, mm9, rn6, dm6, dm3, ce11, ce10, sacCer3, TAIR12
**Dataset ID**: `824`
**License**: CC BY 4.0 (attribution required)

## Access

One entry per `(genome, TF, TSS window)`:

```
CHIPATLAS:hg38:TP53:1
```

`1` is the TSS window in kb (±1 kb, promoter). Separate entries exist for
±1, ±5 and ±10 kb; targets are genes whose TSS falls within that window of a
ChIP-seq peak for the TF.

```
# TF -> target genes
/ws/map/?i=TP53&m=>>hgnc>>chip_atlas_tfbs>>hgnc

# direct entry
/ws/lookup/?i=CHIPATLAS:hg38:TP53:1
```

Human targets link to **hgnc**; other species link to **entrez** filtered by
the species taxid, so a mouse target is not stapled onto the human gene with
the same symbol.

## Fields

| field | meaning |
|-------|---------|
| `tf` | antigen / TF symbol (e.g. `TP53`) |
| `genome` | assembly (`hg38`, `mm10`, …) |
| `tax_id` | NCBI taxonomy id |
| `distance_kb` | TSS window (1, 5, or 10 kb) |
| `target_count` | number of target genes in this window |
| `experiment_count` | ChIP-seq experiments aggregated into the table |
| `cell_type_count` | distinct cell types among those experiments |
| `cell_types` | distinct cell types (capped at 50) |
| `top_targets_schema` | `target|avg_score` |
| `top_targets` | top 20 targets by average MACS2 score |

## Source

- `https://chip-atlas.dbcls.jp/data/metadata/analysisList.tab` — antigens with target-gene tables
- `https://chip-atlas.dbcls.jp/data/{genome}/target/{TF}.{distance}.tsv`
- Target genes are built from UCSC refFlat **protein-coding** genes; non-coding
  genes are not targets.
- License: [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). Cite
  ChIP-Atlas (NAR Web Server issue; https://doi.org/10.1093/nar/gkag378).

## Notes

- Target tables are aggregated across all experiments for the antigen. We keep
  the gene→gene edge and summarise the per-experiment columns into counts and
  top targets, rather than storing every per-experiment score.
- This is the **Target Genes** product. Peak-level data (Peak Browser,
  per-experiment BED/BigWig) is not ingested; it is a separate, much larger
  dataset.
- All three TSS windows (±1, ±5, ±10 kb) are ingested as separate entries, so
  the same TF can be queried at promoter or broader-window resolution.
