# RefSeq Exon Dataset

## Overview

Per-transcript exon coordinates from the human GRCh38 RefSeq annotation,
complementing the Ensembl `exon` dataset for RefSeq accessions. Built from the
NCBI RefSeq GFF3, not from the much larger per-assembly GBFF files.

**Dataset ID**: `830`
**Federation**: `main`
**Source**: `GCF_000001405.40_GRCh38.p14_genomic.gff.gz`
**Scope**: human RefSeq transcripts (`NM_`, `NR_`, `XM_`, `XR_`, `LRG_`)

## What is stored

One entry per exon, keyed `<transcriptBase>:exon<rank>`:

```
NM_001374258:exon1
```

Attributes (`RefSeqExonAttr`):

| field | meaning |
|---|---|
| `transcript` | versioned transcript accession (`NM_001374258.1`) |
| `transcript_base` | unversioned accession (matches the `refseq` key) |
| `exon_id` | GFF exon ID (`exon-NM_...-N`) |
| `exon_number` | 1-based rank from the GFF ID |
| `chromosome` | `NC_...` genomic accession |
| `start`, `end` | genomic coordinates (1-based) |
| `strand` | `+` or `-` |

Cross-reference: `refseq_exon → refseq` (so the reverse query
`NM_... >>refseq>>refseq_exon` returns the transcript's exons).

## Access

```
NM_001374258 >>refseq>>refseq_exon      # all exons, ordered by exon_number
```

Lite target schema:

```
id|exon_number|chromosome|start|end|strand
```

## Use case

Transcript-explicit exon numbering and NMD context: for a truncating variant,
whether it lies >50 bp from the last exon–exon junction differs by isoform.
This dataset supplies the exon coordinates for the RefSeq transcript the
variant is reported on.

## Limitations (v1)

- Human GRCh38 only.
- Exon coordinates only; `coding`, `cds_start` and `cds_end` are present in the
  schema but not populated yet. CDS ranges are needed for a full NMD-escape
  flag and are the next step.
- Rank comes from the GFF exon ID (`exon-NM_...-N`); GFF records without a
  parseable rank are skipped.
