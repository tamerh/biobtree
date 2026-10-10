# GTEx Dataset

## Overview

**GTEx** (Genotype-Tissue Expression) cis-QTL associations — the per-variant
**regulatory** evidence that was biobtree's biggest gap for non-coding/intronic
variants. For a given variant it answers: *which gene's expression (eQTL) or
splicing (sQTL) does this variant affect, in which tissue, and how strongly?*

Variant-keyed (`chr:pos:ref:alt`, GRCh38). **One entry per variant holds all its
significant associations across the 49 tissues** (full per-tissue), each tagged
`eqtl` or `sqtl`. Each distinct affected gene also gets a variant→`ensembl` xref,
so `gene >> gtex` resolves too.

**Data Type**: per-variant QTL associations
**Assembly**: GRCh38 / hg38
**Version**: GTEx **v10** (significant variant-gene pairs)
**Dataset ID**: `820`

## Access

```
# what does GTEx say about this variant?
/ws/?i=1:100000:A:G&d=1
# -> Gtex { associations: [ {qtl_type, gene_id, tissue, slope, pval, phenotype_id}, ... ] }

# which variants are QTLs for a gene?
/ws/map/?i=ENSG00000227232&m=>>gtex
```

Each association carries:

| field | meaning |
|-------|---------|
| `qtl_type` | `eqtl` (expression) or `sqtl` (splicing) |
| `gene_id` | Ensembl gene (ENSG, version stripped) |
| `tissue` | GTEx tissue (e.g. Whole_Blood, Brain_Cortex) |
| `slope` | regression slope / effect size (string; sign + a meaningful 0 preserved) |
| `pval` | nominal p-value |
| `phenotype_id` | sQTL intron-cluster phenotype id (empty for eQTL) |

## Source & preparation

Open-access QTL summary stats (GTEx Portal / `adult-gtex` public bucket):
- eQTL: `…/bulk-qtl/v10/single-tissue-cis-qtl/GTEx_Analysis_v10_eQTL.tar`
- sQTL: `…/bulk-qtl/v10/single-tissue-cis-qtl/GTEx_Analysis_v10_sQTL.tar`

v10 ships the significant pairs as **per-tissue Parquet** files (tissue in the
filename), so `src/scripts/gtex/gtex_prepare.py` downloads the tars, reads every
`*.signif_pairs.parquet` with pyarrow, and emits one combined, **variant-sorted**
TSV (`raw_data/gtex/gtex_associations.sorted.tsv.gz`). The Go parser streams that,
aggregating all associations for a variant in a single pass. Run the prepare
script before building (bb.sh notes this).

## Notes

- Only **significant** variant-gene pairs are ingested (not the full all-pairs
  tables), keeping the dataset to a few GB in the main federation.
- sQTL `gene_id` is the gene the intron-cluster phenotype maps to; the raw
  intron-cluster id is kept in `phenotype_id`.
