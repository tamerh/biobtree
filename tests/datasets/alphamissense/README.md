# AlphaMissense Dataset

## Overview

AlphaMissense is DeepMind's deep learning model for predicting the pathogenicity of missense variants (single amino acid changes) in the human proteome. Contains ~71 million variant predictions with pathogenicity scores and classifications. Essential for variant interpretation, clinical genomics, and understanding protein function through missense variant impact assessment.

**Source**: Google Cloud Storage (`gs://dm_alphamissense/`)
**Data Type**: Missense variant pathogenicity predictions with genomic coordinates
**Related Dataset**: `alphamissense_transcript` (transcript-level summaries, Dataset ID: 109)

## Data Files Explained

AlphaMissense provides three data files with different purposes:

| File | Purpose | Content |
|------|---------|---------|
| `AlphaMissense_hg38.tsv.gz` | Main predictions | One row per variant with canonical transcript + UniProt ID |
| `AlphaMissense_isoforms_hg38.tsv.gz` | Isoform mapping | One row per variant-transcript pair (same variant appears multiple times) |
| `AlphaMissense_gene_hg38.tsv.gz` | Transcript summaries | One row per transcript with mean pathogenicity score |

**Key distinction:**
- **Isoforms file**: Maps variants to ALL transcripts they affect (alternative splicing creates multiple transcripts/isoforms per gene)
- **Gene file**: Aggregates pathogenicity across all variants IN a transcript (summary statistic)

Example:
```
Variant 1:69094:G:T affects 3 transcripts (from isoforms file):
  → ENST00000335137.4, pathogenicity=0.8
  → ENST00000123456.2, pathogenicity=0.7
  → ENST00000789012.1, pathogenicity=0.9

Transcript ENST00000335137.4 has mean=0.42 (from gene file):
  → Average of ALL variants affecting this transcript
```

## Integration Architecture

### Storage Model

**Primary Entries**: Genomic coordinate-based variant IDs
- Entry ID: `{chr}:{pos}:{ref}:{alt}` (e.g., `1:69094:G:T`)

**Searchable Text Links**:
- UniProt IDs indexed for protein-based search
- Protein variants indexed (e.g., "V2L" finds variants causing that change)

**Attributes Stored** (`AlphaMissenseAttr`):
- `chromosome`: Chromosome (1-22, X, Y, MT)
- `position`: Genomic position (GRCh38/hg38)
- `ref_allele`, `alt_allele`: Reference and alternate alleles
- `uniprot_id`: UniProt protein identifier
- `transcript_ids`: Canonical transcript from main file (single element array)
- `protein_variant`: Protein-level change (e.g., "V2L", "R175H")
- `am_pathogenicity`: Pathogenicity score (0.0-1.0)
- `am_class`: Classification (likely_benign, ambiguous, likely_pathogenic)

### Cross-References

- **UniProt**: Links to protein records via uniprot_id
- **transcript**: Links to Ensembl transcripts (canonical + all isoforms)
- **alphamissense_transcript**: Links to transcript-level summaries

### Special Features

**Isoform Support (Memory Efficient)**:
- Isoforms file is streamed, NOT loaded into memory
- `transcript_ids` attribute contains only the canonical transcript
- ALL affected transcripts (including isoforms) accessible via xrefs
- Query: `variant >> transcript` returns all affected transcripts

**Two Separate Datasets**:
- `alphamissense` (ID: 108): Variant-level pathogenicity predictions
- `alphamissense_transcript` (ID: 109): Transcript-level mean scores
- Clean separation enables targeted queries without ambiguity

**Classification Thresholds**:
- `likely_benign`: score < 0.34
- `ambiguous`: 0.34 <= score < 0.564
- `likely_pathogenic`: score >= 0.564

**CEL Filter Support**:
```
alphamissense.am_class == "likely_pathogenic"
alphamissense.am_pathogenicity >= 0.9
```

## Use Cases

**1. Variant Pathogenicity Lookup**
```
Query: 1:69094:G:T -> Get pathogenicity score and classification
Use: Assess clinical significance of a specific missense variant
```

**2. Protein-Focused Variant Search**
```
Query: P12345 >> alphamissense -> All variants affecting this protein
Use: Comprehensive variant catalog for a protein of interest
```

**3. High-Pathogenicity Variant Filtering**
```
Query: Filter alphamissense.am_pathogenicity >= 0.9
Use: Identify high-confidence pathogenic variants for prioritization
```

**4. Find All Transcripts Affected by Variant**
```
Query: 1:69094:G:T >> transcript -> All transcripts (including isoforms)
Use: Understand variant impact across alternative splicing
```

**5. Classification-Based Filtering**
```
Query: Filter alphamissense.am_class == "likely_pathogenic"
Use: Focus analysis on variants predicted to be damaging
```

## Test Cases

**Current Tests** (9 total):
- Basic variant lookup by ID
- Attribute validation (all fields present)
- Pathogenicity score range validation (0-1)
- Classification threshold consistency
- UniProt cross-reference validation
- Transcript IDs format validation (ENST pattern)
- Protein variant format validation (X#Y pattern)
- Entry ID format validation (chr:pos:ref:alt)
- Allele consistency (ID matches attributes)

## Performance

- **Test Build**: ~15s (10,000 variants)
- **Data Source**: Google Cloud Storage (public bucket)
- **Full Dataset**: ~71M variants (643MB compressed main file)
- **Isoform File**: ~1.2GB compressed (streamed, not loaded into memory)

## Known Limitations

**Missense Variants Only**:
- AlphaMissense predicts pathogenicity ONLY for missense variants (single amino acid substitutions)
- Does NOT cover: frameshift, nonsense (stop-gain), splice site, insertions, deletions, or structural variants
- For other variant types, use ClinVar, dbSNP, or VEP annotations

**Transcript Storage**:
- `transcript_ids` attribute contains only canonical transcript
- All isoforms accessible via xrefs (not stored in attribute)

**Species Coverage**:
- Human proteome only (GRCh38/hg38)
- No predictions for other organisms

## Future Work

- Add AlphaMissense_aa_substitutions.tsv.gz (protein-centric view)
- ClinVar pathogenicity comparison
- VEP consequence enrichment
- Population frequency correlation
- Protein domain mapping

## Maintenance

- **Release Schedule**: Periodic updates from DeepMind
- **Current Version**: AlphaMissense v1.0 (2023)
- **Data Format**: TSV (tab-separated values)
- **Test Data**: 10,000 variants
- **License**: CC BY-NC-SA 4.0

## References

- **Citation**: Cheng J, et al. (2023) Accurate proteome-wide missense variant effect prediction with AlphaMissense. Science 381(6664):eadg7492.
- **Website**: https://alphamissense.hegelab.org/
- **Data**: https://console.cloud.google.com/storage/browser/dm_alphamissense
- **License**: CC BY-NC-SA 4.0
