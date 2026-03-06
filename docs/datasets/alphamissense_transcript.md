# AlphaMissense Transcript Dataset

## Overview

AlphaMissense Transcript provides transcript-level pathogenicity summaries from DeepMind's AlphaMissense predictions. Each entry represents an Ensembl transcript with its **mean pathogenicity score** calculated across ALL possible missense variants in that transcript. Used to assess overall mutational burden tolerance of transcripts/genes.

**Source**: Google Cloud Storage (`gs://dm_alphamissense/AlphaMissense_gene_hg38.tsv.gz`)
**Data Type**: Transcript-level pathogenicity summaries (aggregated statistics)
**Related Dataset**: `alphamissense` (variant-level predictions, ID: 108)

## Data Explanation

**What this dataset contains:**
- One entry per Ensembl transcript
- Mean pathogenicity score = average of ALL variant predictions for that transcript
- Indicates how "mutation intolerant" a transcript is overall

**What this is NOT:**
- This is NOT individual variant predictions (those are in `alphamissense`)
- This is NOT variant-to-transcript mapping (that's from isoforms file via xrefs)

**Example interpretation:**
```
ENST00000335137.4 has mean_am_pathogenicity = 0.65
  → On average, missense variants in this transcript are predicted pathogenic
  → This transcript is relatively mutation-intolerant

ENST00000999999.1 has mean_am_pathogenicity = 0.25
  → On average, missense variants in this transcript are predicted benign
  → This transcript is more mutation-tolerant
```

**Biological context:**
- High mean pathogenicity → Essential genes, critical protein domains
- Low mean pathogenicity → Non-essential regions, tolerant to variation

## Integration Architecture

### Storage Model

**Primary Entries**: Ensembl transcript IDs with version
- Entry ID: `ENST00000335137.4`

**Searchable Text Links**:
- Transcript IDs indexed (with and without version)

**Attributes Stored** (`AlphaMissenseTranscriptAttr`):
- `transcript_id`: Ensembl transcript ID with version
- `mean_am_pathogenicity`: Average pathogenicity score (0.0-1.0)

### Cross-References

- **transcript**: Links to Ensembl transcript records (version stripped for compatibility)
- Variants in `alphamissense` link TO this dataset via xrefs

### Special Features

**Separate Dataset**:
- ID: 109 (alphamissense_transcript)
- Distinct from alphamissense (ID: 108) variant-level dataset
- Clean separation enables targeted queries

**CEL Filter Support**:
```
alphamissense_transcript.mean_am_pathogenicity >= 0.5
```

## Use Cases

**1. Transcript Tolerance Assessment**
```
Query: ENST00000335137.4 -> Get mean_am_pathogenicity
Use: Assess how tolerant a transcript is to missense mutations
```

**2. Find Mutation-Intolerant Transcripts**
```
Query: Filter alphamissense_transcript.mean_am_pathogenicity >= 0.6
Use: Identify transcripts where variants are likely pathogenic (essential genes)
```

**3. Find Mutation-Tolerant Transcripts**
```
Query: Filter alphamissense_transcript.mean_am_pathogenicity < 0.3
Use: Identify transcripts where variants are usually benign
```

**4. Gene Prioritization**
```
Query: Gene symbol -> Ensembl -> transcript -> alphamissense_transcript
Use: Prioritize genes by transcript-level pathogenicity burden
```

**5. Compare Transcript Isoforms**
```
Query: Multiple transcripts of same gene -> compare mean scores
Use: Identify which transcript isoforms are more critical
```

## Test Cases

**Current Tests** (5 total):
- Basic transcript lookup by ID
- Mean pathogenicity range validation (0-1)
- Transcript ID format validation (ENST pattern)
- Attribute consistency validation
- Pathogenicity distribution check

## Performance

- **Test Build**: ~5s (50,000 transcript entries)
- **Data Source**: Google Cloud Storage (public bucket)
- **Full Dataset**: ~19,000 transcripts (254KB compressed)

## Known Limitations

**Aggregated Data Only**:
- Contains mean score only (no min/max/median)
- No confidence intervals or variance
- No count of variants per transcript

**Human Transcripts Only**:
- GRCh38/hg38 reference genome
- No predictions for other organisms

**Entry ID Format**:
- Includes version number (e.g., ENST00000335137.4)
- Cross-references use base ID without version for Ensembl compatibility

## Future Work

- Add min/max pathogenicity per transcript
- Add variant count per transcript
- Add standard deviation / confidence intervals
- Gene-level aggregation (combining multiple transcripts)

## Maintenance

- **Release Schedule**: Updates with AlphaMissense releases
- **Current Version**: AlphaMissense v1.0 (2023)
- **Data Format**: TSV (tab-separated values)
- **License**: CC BY-NC-SA 4.0

## References

- **Citation**: Cheng J, et al. (2023) Accurate proteome-wide missense variant effect prediction with AlphaMissense. Science 381(6664):eadg7492.
- **Data**: https://console.cloud.google.com/storage/browser/dm_alphamissense
