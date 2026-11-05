# dbVar Dataset

## Overview

Clinical **structural variants** (deletions, duplications, insertions,
inversions, CNVs) from NCBI [dbVar](https://www.ncbi.nlm.nih.gov/dbvar/).
BioBTree was SNV-heavy; this adds the structural-variant class with clinical
significance and disease links.

**Data Type**: structural variant (interval)
**Assembly**: GRCh38
**Dataset ID**: `826`
**License**: NCBI public domain

## Access

One entry per dbVar call, keyed by the `nssv…` accession:

```
nssv16255736
```

```
# direct entry
/ws/entry/?i=nssv16255736&s=dbvar

# position -> structural variants covering it
/ws/map/?i=1:10001&m=>>dbvar
```

The dataset is interval-indexed: a `chr:pos` query returns every SV whose
`start..end` spans that position.

## Fields

| field | meaning |
|-------|---------|
| `chromosome`, `start`, `end` | GRCh38 coordinates (1-based) |
| `svtype` | DEL, DUP, INS, INV, CNV, BND, … |
| `svlen` | signed length when present |
| `clinical_significance` | Pathogenic, Likely pathogenic, VUS, Benign, … |
| `origin` | germline, de novo, somatic, … |
| `phenotype_ids` | MONDO / OMIM / Orphanet / MedGen / MeSH ids |
| `clinvar_accessions` | VCV/RCV accessions |
| `region_id` | parent dbVar region (`nsv…`) |
| `description` | dbVar `DESC` |
| `somatic` | somatic flag |

## Cross-references

- `clinvar` — VCV variation ids from `CLNACC`
- `mondo`, `mim`, `orphanet`, `medgen`, `mesh`, `hpo` — from `PHENO`
- `pubmed` — from `LINKS`

ClinVar carries the gene/disease path, so dbVar → ClinVar → gene works without
duplicating gene assignments here.

## Source

- Base: `https://ftp.ncbi.nlm.nih.gov/pub/dbVar/data/Homo_sapiens/by_assembly/GRCh38/vcf/`
- Ingested files (clinical subsets):
  - `GRCh38.variant_call.clinical.pathogenic_or_likely_pathogenic.vcf.gz`
  - `GRCh38.variant_call.clinical.uncertain_significance.vcf.gz`
  - `GRCh38.variant_call.clinical.conflicting_or_other.vcf.gz`
  - `GRCh38.variant_call.clinical.benign_or_likely_benign.vcf.gz`
- The full `variant_call.all` archive (527 MB) is not ingested; it is noisier
  and much larger. It can be added later as a separate federation.

## Notes

- dbVar is an archive: clinical significance is submitter-provided. Use ClinVar
  assertions where a VCV link exists.
- The interval index is built for the clinical subset. Indexing the full
  archive would produce too many 16 kb bin entries for large CNVs.
