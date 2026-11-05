# gnomAD SV Dataset

## Overview

Structural variants (≥50 bp) from the gnomAD v4.1 SV sites release —
population allele frequencies and predicted gene effects for ~1.2M SV sites.
Complements `dbvar` (clinical assertions) with population frequency.

**Data Type**: structural variant (interval)
**Assembly**: GRCh38
**Dataset ID**: `827`
**Federation**: `gnomad`
**License**: CC0 1.0

## Access

One entry per SV site, keyed by the gnomAD SV VCF ID:

```
gnomAD-SV_v3_DUP_chr1_01c2781c
```

```
# direct entry
/ws/entry/?i=gnomAD-SV_v3_DUP_chr1_01c2781c&s=gnomad_sv

# coordinate key (chr:pos:end:svtype)
/ws/search/?i=1:10000:295666:DUP&mode=lite

# position -> small SVs covering it
/ws/map/?i=1:20000&m=>>gnomad_sv
```

SVs up to **1 Mb** are interval-indexed. Larger CNVs are intentionally not
indexed (a single 100 Mb CNV would add thousands of 16 kb bins); query them by
VCF ID or coordinate key.

## Fields

| field | meaning |
|-------|---------|
| `chromosome`, `start`, `end` | GRCh38 coordinates |
| `svtype` | DEL, DUP, INS, INV, CNV, BND, CPX, … |
| `svlen` | SV length |
| `af`, `ac`, `an` | global allele frequency/count/number |
| `grpmax_af` | group-max allele frequency |
| `af_afr`, `af_amr`, `af_asj`, `af_eas`, `af_fin`, `af_mid`, `af_nfe`, `af_rmi`, `af_sas` | ancestry-specific AFs |
| `filters` | VCF FILTER |
| `cpx_type` | complex-variant class |
| `predicted_genes` | genes in `PREDICTED_*` annotations |
| `predicted_effects` | e.g. `LOF:TP53`, `COPY_GAIN:OR4F5` |
| `intergenic` | `PREDICTED_INTERGENIC` flag |

## Cross-references

- `hgnc` — from `PREDICTED_LOF`, `PREDICTED_COPY_GAIN`,
  `PREDICTED_PARTIAL_EXON_DUP`, `PREDICTED_PROMOTER`, `PREDICTED_UTR`, etc.
  Gene symbols are resolved to the **exact** official HGNC symbol, not the
  first text-search hit.

## Source

- `https://storage.googleapis.com/gcp-public-data--gnomad/release/4.1/genome_sv/gnomad.v4.1.sv.sites.vcf.gz`
- ~1.2M sites, ~1.74 GB compressed.
- License: CC0 1.0 (gnomAD data).

## Notes

- The test fixture is `tests/datasets/gnomad_sv/gnomad_sv_fixture.vcf`; test
  mode uses it instead of the 1.74 GB remote file.
- The full build has not been run yet; it will live in the existing `gnomad`
  federation alongside `gnomad_variant`, `gnomad_rmc`, `gnomad_gnocchi`.
