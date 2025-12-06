# GWAS Associations - Genome-Wide Association Study Catalog

## Overview

GWAS Associations dataset provides comprehensive SNP-trait-disease association data from the NHGRI-EBI GWAS Catalog. Contains 1,000,000+ curated associations linking specific genetic variants to human diseases, traits, and phenotypes with statistical evidence from published genome-wide association studies. Essential for understanding genetic contributions to complex diseases, identifying disease-associated variants, and investigating genotype-phenotype relationships.

**Source**: GWAS Catalog EBI FTP (gwas-catalog-associations.tsv in ZIP format)
**Data Type**: SNP-trait associations with statistical evidence, genomic context, and ontology mappings
**Relationships**: Many-to-many (one SNP in multiple studies, one study with multiple SNPs)

## Integration Architecture

### Entry Identifier Format: STUDYID_N

Entries use the format: **`STUDYID_N`** (e.g., `GCST006085_1`, `GCST006085_2`, ...)

Where:
- `STUDYID` = GWAS Catalog study accession (e.g., GCST006085)
- `N` = Sequential counter for each association row within that study

**Example:**
```
Study GCST006085 (Prostate cancer) found 25 associations:

GCST006085_1:  chr7:40835593,  SNP=rs17621345, p=7e-14, gene=SUGCT
GCST006085_2:  chr9:19055967,  SNP=rs1048169,  p=3e-12, gene=HAUS6
GCST006085_3:  chr9:34049781,  SNP=rs10122495, p=1e-08, gene=DCAF12
...
GCST006085_25: chr2:85432100,  SNP=rs789012,   p=5e-10, gene=ABC
```

### Why Counter-Based Keys (Not SNP IDs)?

**Original design** used `STUDYID_RSID` (e.g., `GCST006085_rs17621345`), but this caused problems:

1. **LMDB Key Length Limit**: LMDB has a 511-byte maximum key size

2. **Multiple SNPs Per Row**: Some GWAS Catalog rows contain multiple SNPs in a single field:
   ```
   SNPS field: "rs387673; rs12413638; rs7096965; rs1234567; ..."
   ```
   This would create keys exceeding the 511-byte limit.

3. **Why Multiple SNPs in One Row?**
   - **Linkage Disequilibrium (LD)**: SNPs inherited together, statistically correlated
   - **Haplotype associations**: Multiple SNPs that together mark the causal variant
   - **Interaction effects**: SNPs that jointly affect the trait

**Current design** uses a simple counter that:
- Guarantees short, unique keys (always under 30 bytes)
- Preserves all SNP associations via cross-references
- Maintains all original data (position, p-value, genes, etc.)
- Allows searching by any SNP ID to find the association

### What is an Association?

Each row in the GWAS Catalog file represents a **distinct genomic region** (not just a SNP) that a study identified as significantly associated with a trait. A single study typically scans the entire genome and finds multiple significant "hits" at different locations.

**Key Point**: The counter (1, 2, 3...) represents **row number within the study**, not SNP count. Each row is a different scientific finding with its own:
- Genomic position (usually the "lead" SNP position)
- P-value (statistical significance)
- Effect size (odds ratio or beta)
- Associated genes
- One or more SNP identifiers

### Handling Multiple SNPs in One Row

When a row has multiple SNPs (e.g., `"rs387673; rs12413638; rs7096965"`):

```
GCST006085_25 (single entry for the row):
├── position: chr7:50000000 (lead SNP position from the row)
├── p_value: 2e-09
├── effect_size: 1.15
├── genes: XYZ
├── snp_id: "rs387673" (primary/first SNP)
└── cross-references (all SNPs searchable):
    ├── rs387673 → GCST006085_25
    ├── rs12413638 → GCST006085_25
    └── rs7096965 → GCST006085_25
```

**Important**: We do NOT split rows into multiple entries. One row = one entry.
The position in the row corresponds to the **lead/index SNP** - typically the most significant one in that region.

## Relationship with GWAS Study Dataset

There are **two separate datasets**:

| Dataset | ID Format | Contains |
|---------|-----------|----------|
| `gwas_study` | GCST006085 | Study metadata (publication, sample size, trait description) |
| `gwas` | GCST006085_1, GCST006085_2, ... | Individual SNP-trait associations with statistical details |

**No ID Collisions**:
- dbSNP uses: `rs*` (e.g., `rs7903146`)
- GWAS Study uses: `GCST*` (e.g., `GCST000001`)
- GWAS Associations uses: `GCST*_N` (e.g., `GCST000001_1`)

### Cross-References

**Bidirectional Links**:

1. **Association ↔ dbSNP**
   - `GCST000001_1` ↔ `rs7903146`
   - Query: `"rs7903146"` → Finds all associations containing this SNP
   - Query: `"GCST000001_1 >> dbsnp"` → Variant catalog data

2. **Association ↔ GWAS Study**
   - `GCST000001_1` ↔ `GCST000001`
   - Query: `"GCST000001 >> gwas"` → All SNP associations in this study
   - Query: `"GCST000001_1 >> gwas_study"` → Study metadata

3. **Association ↔ Ensembl Genes**
   - Via gene symbol lookup using `addXrefViaGeneSymbol()`
   - Handles paralogs by creating xrefs to all matching Ensembl genes
   - Query: `"TCF7L2 >> gwas"` → All associations for this gene

4. **Association ↔ EFO Traits**
   - `GCST000001_1` ↔ `EFO:0001360` (Type 2 diabetes)
   - Query: `"EFO:0001360 >> gwas"` → All SNPs associated with this trait

### Storage Model

**Primary Entries**:
- Association IDs in format `STUDYID_N` (e.g., `GCST000001_1`)
- Each association is an independent entry with unique statistics

**Searchable Text Links**:
- Association IDs indexed for direct lookup
- Study accessions indexed (search "GCST000001" finds associations)
- rs IDs indexed as keywords (search "rs7903146" finds associations)

**Attributes Stored** (protobuf GwasAttr):

#### Primary Identifiers
- `snp_id`: RefSNP ID (e.g., "rs7903146")
- `study_accession`: Link to gwas_study dataset (e.g., "GCST000001")

#### SNP Details
- `strongest_snp_risk_allele`: SNP-risk allele combination (e.g., "rs7903146-T")
- `chr_id`: Chromosome (e.g., "10")
- `chr_pos`: Chromosomal position (1-based coordinate)
- `region`: Cytogenetic region (e.g., "10q25.2")
- `context`: Variant context (intergenic, intronic, missense, etc.)
- `intergenic`: Boolean flag for intergenic variants

#### Gene Context
- `reported_genes`: Gene symbols from publication (array, e.g., ["TCF7L2", "WFS1"])
- `mapped_gene`: Primary mapped gene symbol
- `upstream_gene_id`: Upstream gene Ensembl ID
- `downstream_gene_id`: Downstream gene Ensembl ID
- `snp_gene_ids`: Ensembl gene IDs for SNP locus (array)

#### Disease/Trait Information
- `disease_trait`: Free text disease or trait description (e.g., "Type 2 diabetes")
- `mapped_traits`: Curated trait terms (array)
- `efo_traits`: EFO ontology IDs (array, e.g., ["EFO:0001360"])

#### Statistical Evidence
- `p_value`: Association p-value (e.g., 1.23e-20)
- `pvalue_mlog`: -log10(p-value) for easier visualization
- `or_beta`: Odds ratio or beta coefficient
- `ci_95`: 95% confidence interval (text format)
- `risk_allele_frequency`: Risk allele frequency in study population

#### Study Metadata (Denormalized)
- `pubmed_id`: PubMed citation ID
- `first_author`: First author surname
- `date`: Publication date
- `platform`: Genotyping platform and SNP count

## Use Cases

**1. Find All Associations for a SNP**
```
Query: rs7903146
Returns: GCST000001_1, GCST000002_5, GCST000567_12, ...
Use: Discover all diseases/traits associated with a variant
```

**2. Find All SNPs in a Study**
```
Query: GCST000001
Returns: GCST000001_1, GCST000001_2, GCST000001_3, ...
Use: Extract all significant variants from a published GWAS
```

**3. Find All Associations for a Gene**
```
Query: TCF7L2 >> gwas
Returns: All GWAS associations where TCF7L2 is the reported gene
Use: Understand genetic contribution of a gene to disease
```

**4. Find All SNPs Associated with a Trait**
```
Query: EFO:0001360 >> gwas (Type 2 Diabetes)
Returns: All SNP associations linked to this EFO term
Use: Compile catalog of disease-associated variants
```

**5. Specific Association Lookup**
```
Query: GCST000001_1
Returns: Exact association entry with study-specific statistics
Use: Get precise p-value, OR, and context for one study finding
```

## Data Model Diagram

```
Three Interconnected Datasets:

┌─────────────┐         ┌──────────────────────┐         ┌─────────────┐
│   dbSNP     │◄───────►│   GWAS Associations  │◄───────►│ GWAS Study  │
│             │         │                      │         │             │
│ rs7903146   │         │ GCST000001_1         │         │ GCST000001  │
│             │         │ GCST000001_2         │         │ GCST000002  │
│ (variant    │         │ GCST000002_1         │         │             │
│  catalog)   │         │                      │         │ (study      │
│             │         │ (SNP-trait-study)    │         │  metadata)  │
└─────────────┘         └──────────────────────┘         └─────────────┘
      │                           │                             │
      │                           │                             │
      ▼                           ▼                             ▼
┌─────────────┐         ┌──────────────────────┐         ┌─────────────┐
│  Ensembl    │         │    EFO Ontology      │         │  PubMed     │
│   Genes     │         │                      │         │             │
│             │         │ EFO:0001360          │         │ PMID:123456 │
│ ENSG000001  │         │ (Type 2 Diabetes)    │         │             │
└─────────────┘         └──────────────────────┘         └─────────────┘
```

## Test Cases

**Current Tests** (10 total):
- 4 declarative tests (JSON-based)
- 6 custom tests (Python logic)

**Coverage**:
- Lookup by association ID (STUDYID_N format)
- Attribute presence validation
- Multiple ID batch lookup
- Invalid ID handling
- Genomic position data (chr, pos, region)
- Association to study cross-references
- Gene symbol associations
- EFO trait mappings
- SNP ID text search (search rs ID → find association)
- Statistical evidence (p-values, effect sizes)

### Running Tests

```bash
# Via orchestrator (recommended)
python tests/run_tests.py gwas

# Direct execution (requires running server)
cd tests/datasets/gwas
python3 test_gwas.py
```

### Example Queries

```bash
# Lookup specific association
curl "http://localhost:9292/ws/search?i=GCST006085_1"

# Search by SNP ID (finds associations containing this SNP)
curl "http://localhost:9292/ws/search?i=rs17621345"

# Search by study accession (finds all associations in study)
curl "http://localhost:9292/ws/search?i=GCST006085"

# Get cross-references
curl "http://localhost:9292/ws/search?i=GCST006085_1&x=true"
```

## Performance

- **Test Build**: ~2-3s (100 associations)
- **Full Build**: ~8-12 minutes (1,000,000+ associations)
- **Data Source**: ZIP file (~57 MB compressed, ~620 MB uncompressed TSV)
- **Processing**: Streaming (no in-memory accumulation)
- **Memory Usage**: Low (one association at a time)

## Historical Note - Key Format Evolution

**Original Design (STUDYID_RSID)**:
- Format: `GCST000001_rs7903146`
- Problem: LMDB 511-byte key limit exceeded when SNPS field contained multiple SNPs

**Current Design (STUDYID_N)**:
- Format: `GCST000001_1`, `GCST000001_2`, ...
- Solution: Counter-based keys always short, SNPs accessible via xrefs
- All data preserved, just different identifier scheme

## References

- **Citation**: Buniello A, et al. (2019) The NHGRI-EBI GWAS Catalog of published genome-wide association studies, targeted arrays and summary statistics 2019. Nucleic Acids Res. 47(D1):D1005-D1012.
- **Website**: https://www.ebi.ac.uk/gwas/
- **API**: https://www.ebi.ac.uk/gwas/rest/docs/api
- **Documentation**: https://www.ebi.ac.uk/gwas/docs/
- **License**: CC0 (public domain)
- **FTP**: ftp://ftp.ebi.ac.uk/pub/databases/gwas/
