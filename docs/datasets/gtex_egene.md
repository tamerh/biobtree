# GTEx eGene Dataset

## Overview

**GTEx eGene / sGene** — the **gene-level** companion to the variant-keyed
[`gtex`](gtex.md) dataset. For a given gene it answers: *in which tissues is this
gene a significant eGene (eQTL) or sGene (sQTL), with what q-value, lead variant
and effect size?*

This is the view the variant-keyed `gtex` dataset **cannot** be aggregated back
into: from a variant you can list the genes it regulates, but you cannot walk
from a gene to "all its QTL evidence" (that would mean scanning every variant).
So the per-gene summary is ingested as its own gene-keyed dataset, reachable as
`ensembl >> gtex_egene`.

Gene-keyed (Ensembl `ENSG`, version stripped). **One entry per gene holds its
per-tissue call** across the ~50 eQTL and ~50 sQTL tissues. Each gene also gets a
`gtex_egene`→`ensembl` xref so `ensembl >> gtex_egene` resolves.

**Data Type**: per-gene, per-tissue eGene/sGene summary
**Assembly**: GRCh38 / hg38
**Version**: GTEx **v10** (eGenes/sGenes, FastQTL q-value)
**Dataset ID**: `821`

## Access

```
# gene-level GTEx summary
/ws/?i=ENSG00000141510&d=1
# -> GtexEgene { gene_name, tissues: [ {qtl_type, tissue, is_egene, qval, lead_variant, slope}, ... ] }

# reach it from a gene
/ws/map/?i=ENSG00000141510&m=>>gtex_egene
/ws/map/?i=TP53&m=>>hgnc>>ensembl>>gtex_egene

# compact projection (mode=lite): gene_name|egene_count|egene_tissues|qtl_types
/ws/map/?i=ENSG00000141510&m=>>gtex_egene&mode=lite
```

Each tissue row carries:

| field | meaning |
|-------|---------|
| `qtl_type` | `eqtl` (eGene) or `sqtl` (sGene) |
| `tissue` | GTEx tissue (e.g. Whole_Blood, Brain_Cortex) |
| `is_egene` | genome-wide significant in this tissue (`qval` ≤ 0.05) |
| `qval` | Storey q-value (string; a meaningful 0 preserved) |
| `lead_variant` | top-association variant key `chr:pos:ref:alt` |
| `slope` | lead-variant effect size (string; sign + 0 preserved) |

## Source & preparation

GTEx v10 ships the eGene/sGene tables as per-tissue `.txt.gz` members
(`*.v10.eGenes.txt.gz` in the eQTL tar, `*.v10.sGenes.txt.gz` in the sQTL tar) —
the same two tarballs the `gtex` dataset uses. `src/scripts/gtex/gtex_prepare.py`
reads every per-tissue member (parsing by **header name**, since eGenes and
sGenes differ in column order), emits one row per gene with its tissue, q-value,
lead variant and slope, and sorts by gene id:

```
raw_data/gtex/gtex_egenes.sorted.tsv.gz
gene_id  qtl_type  tissue  gene_name  is_egene  qval  lead_variant  slope
```

Run `python3 src/scripts/gtex/gtex_prepare.py` (builds both the variant and gene
files) or `--egenes-only` to (re)build just this one from tars already on disk.
The Go parser (`src/update/gtex_egene.go`) streams the gene-sorted TSV and
aggregates all tissue rows for a gene in a single pass.

## Notes

- `bucketMethod: ensembl_hybrid` — keys are `ENSG` ids and MUST bucket exactly as
  `ensembl` does, or the merge collides (two datasets writing the same ENSG key to
  different buckets → `MDB_KEYEXIST`).
- `main` federation. CC BY-NC / GTEx open-access terms, same as `gtex`.
