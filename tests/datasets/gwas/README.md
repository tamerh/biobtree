# GWAS Associations - Genome-Wide Association Study Catalog

## Overview

GWAS Associations dataset provides comprehensive SNP-trait-disease association data from the NHGRI-EBI GWAS Catalog. Contains 1,000,000+ curated associations linking specific genetic variants to human diseases, traits, and phenotypes with statistical evidence from published genome-wide association studies. Essential for understanding genetic contributions to complex diseases, identifying disease-associated variants, and investigating genotype-phenotype relationships.

**Source**: GWAS Catalog EBI FTP (gwas-catalog-associations.tsv in ZIP format)
**Data Type**: SNP-trait associations with statistical evidence, genomic context, and ontology mappings
**Relationships**: Many-to-many (one SNP in multiple studies, one study with multiple SNPs)

## Integration Architecture

### Composite Key Design: STUDYID_RSID

**Why Composite Keys?**

GWAS associations represent a **many-to-many relationship**:
- One SNP can be associated with **multiple diseases/traits**
- One SNP can appear in **multiple independent studies**
- Each association has **unique statistics** (p-values, effect sizes, risk alleles)

**Problem with Simple Keys:**
Using rs IDs alone (like `rs7903146`) would cause:
1. **ID collision with dbSNP dataset** (both use rs IDs as primary keys)
2. **Data loss** - only one association per SNP could be stored, losing all others

**Solution: Composite Key Format**
```
Format: STUDYID_RSID
Example: GCST000001_rs7903146

Components:
- STUDYID: GWAS Catalog study accession (e.g., GCST000001)
- RSID: dbSNP reference SNP ID (e.g., rs7903146)
```

**Benefits:**
✅ No ID collision with dbSNP (rs IDs) or GWAS Study (study accessions)
✅ All associations preserved (no data loss)
✅ Clear semantic meaning (which study + which SNP)
✅ Bidirectional navigation (study ↔ SNP ↔ genes ↔ traits)

### Storage Model

**Primary Entries**:
- Association IDs in format `STUDYID_RSID` (e.g., `GCST000001_rs7903146`)
- Each association is an independent entry with unique statistics

**Searchable Text Links**:
- Association IDs indexed for direct lookup
- rs IDs indexed as keywords (finds all associations for a SNP)
- Gene symbols indexed (finds all associations for a gene)

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

### Cross-References

**Bidirectional Links**:

1. **Association ↔ dbSNP**
   - `GCST000001_rs7903146` ↔ `rs7903146`
   - Query: `"rs7903146 >> gwas"` → All associations for this SNP
   - Query: `"GCST000001_rs7903146 >> dbsnp"` → Variant catalog data

2. **Association ↔ GWAS Study**
   - `GCST000001_rs7903146` ↔ `GCST000001`
   - Query: `"GCST000001 >> gwas"` → All SNP associations in this study
   - Query: `"GCST000001_rs7903146 >> gwas_study"` → Study metadata

3. **Association ↔ Ensembl Genes**
   - Via gene symbol lookup using `addXrefViaGeneSymbol()`
   - Handles paralogs by creating xrefs to all matching Ensembl genes
   - Query: `"TCF7L2 >> gwas"` → All associations for this gene
   - Query: `"GCST000001_rs7903146 >> ensembl"` → Gene annotations

4. **Association ↔ EFO Traits**
   - `GCST000001_rs7903146` ↔ `EFO:0001360` (Type 2 diabetes)
   - Query: `"EFO:0001360 >> gwas"` → All SNPs associated with this trait
   - Query: `"GCST000001_rs7903146 >> efo"` → Trait ontology terms

### Special Features

**Many-to-Many Relationship Handling**:
```
Example: rs7903146 (TCF7L2 variant)

One SNP → Multiple Studies:
  GCST000001_rs7903146 → Type 2 Diabetes (Study 1, p=1e-20, OR=1.37)
  GCST000002_rs7903146 → Type 2 Diabetes (Study 2, p=5e-18, OR=1.35)
  GCST000567_rs7903146 → Obesity (Study 567, p=1e-12, OR=1.15)

One Study → Multiple SNPs:
  GCST000001_rs7903146 → Type 2 Diabetes, TCF7L2
  GCST000001_rs9939609 → Type 2 Diabetes, FTO
  GCST000001_rs8050136 → Type 2 Diabetes, FTO
```

**Real Example: rs9939609 (FTO gene)**
- Associated with: Obesity, Type 2 Diabetes, BMI, Waist circumference, Hip circumference
- Found in: 100+ different GWAS studies
- Stored as: 100+ separate association entries with unique composite IDs
- Each preserving distinct p-values, effect sizes, and study contexts

**ZIP File Processing**:
- Streaming parse with bufio.Scanner (handles 620 MB uncompressed data)
- 1MB buffer for long trait descriptions
- No in-memory accumulation (processes one association at a time)

**Gene Symbol Resolution**:
- Uses `addXrefViaGeneSymbol()` for paralog-aware mapping
- Creates xrefs to ALL matching Ensembl genes (deterministic principle)
- Handles multi-gene associations (comma-separated reported genes)

**EFO Ontology Integration**:
- Extracts EFO IDs from MAPPED_TRAIT_URI field
- Converts URIs to clean IDs (e.g., "http://www.ebi.ac.uk/efo/EFO_0001360" → "EFO:0001360")
- Enables structured trait-based queries

## Use Cases

**1. Find All Associations for a SNP**
```
Query: rs7903146 >> gwas
Returns: GCST000001_rs7903146, GCST000002_rs7903146, GCST000567_rs7903146, ...
Use: Discover all diseases/traits associated with a variant
```

**2. Find All SNPs in a Study**
```
Query: GCST000001 >> gwas
Returns: GCST000001_rs7903146, GCST000001_rs9939609, GCST000001_rs8050136, ...
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

**5. Navigate Association Network**
```
Query Path: rs7903146 >> gwas >> gwas_study
Result: rs7903146 → Associations → Study metadata (sample size, ancestry, etc.)
Use: Full context from variant to publication
```

**6. Cross-Database Integration**
```
Query Path: rs7903146 >> gwas >> ensembl
Result: Association data → Gene annotations → Protein sequences
Use: Link genetic associations with functional annotations
```

**7. Specific Association Lookup**
```
Query: GCST000001_rs7903146
Returns: Exact association entry with study-specific statistics
Use: Get precise p-value, OR, and context for one study
```

**8. Gene-Disease-Variant Triangle**
```
Query: TCF7L2 >> gwas >> efo
Returns: All traits associated with TCF7L2 variants
Use: Understand phenotypic spectrum of gene mutations
```

## Data Model Relationships

```
Three Interconnected Datasets:

┌─────────────┐         ┌──────────────────────┐         ┌─────────────┐
│   dbSNP     │◄───────►│   GWAS Associations  │◄───────►│ GWAS Study  │
│             │         │                      │         │             │
│ rs7903146   │         │ GCST000001_rs7903146 │         │ GCST000001  │
│             │         │ GCST000002_rs7903146 │         │ GCST000002  │
│ (variant    │         │                      │         │             │
│  catalog)   │         │ (SNP-trait-study)    │         │ (study      │
│             │         │                      │         │  metadata)  │
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

**No ID Collisions**:
- dbSNP uses: `rs*` (e.g., `rs7903146`)
- GWAS Study uses: `GCST*` (e.g., `GCST000001`)
- GWAS Associations uses: `GCST*_rs*` (e.g., `GCST000001_rs7903146`)

## Test Cases

**Current Tests** (10 total):
- 4 declarative tests (JSON-based)
- 6 custom tests (Python logic)

**Coverage**:
- ✅ Basic association ID lookup (composite key format)
- ✅ Attribute presence validation
- ✅ Multiple ID batch lookup
- ✅ Invalid ID handling
- ✅ SNP to study cross-references
- ✅ Gene symbol associations
- ✅ EFO trait mappings
- ✅ Statistical evidence (p-values, effect sizes)
- ✅ Genomic position data
- ✅ Multi-study associations for single SNP

**Recommended Additions**:
- Test composite key uniqueness (no duplicates)
- Test bidirectional navigation (association → dbSNP → association)
- Validate all associations preserved (no data loss from grouping)
- Test paralog gene handling (genes on multiple chromosomes)
- Test multi-gene associations (reported_genes array)
- Verify EFO ID extraction from URIs
- Test intergenic variant flags
- Validate chromosome normalization
- Test study metadata denormalization accuracy

## Performance

- **Test Build**: ~2.3s (100 associations)
- **Full Build**: ~8-12 minutes (1,000,000+ associations)
- **Data Source**: ZIP file (~57 MB compressed, ~620 MB uncompressed TSV)
- **Processing**: Streaming (no in-memory accumulation)
- **Memory Usage**: Low (one association at a time)
- **Database Size**: ~500 MB (1M associations with full attributes)
- **Update Frequency**: Weekly (GWAS Catalog releases)

## Known Limitations

**Historical Note - Resolved**:
- ❌ **OLD**: Grouped associations by SNP, saved only first (massive data loss)
- ✅ **NEW**: All associations saved individually with composite keys

**Current Limitations**:

**EFO Mapping Coverage**:
- Not all traits have EFO mappings
- Some associations rely on free-text disease_trait field
- Ontology coverage improving over time

**Ancestry Information**:
- Not yet integrated (planned enhancement)
- Study-level ancestry available in gwas_study dataset
- Important for transferability across populations

**Association Aggregation**:
- Each study stored independently
- No meta-analysis across studies (by design)
- Users must perform own statistical aggregation

**Gene Mapping Complexity**:
- Some variants have multiple reported genes
- Intergenic variants may lack clear gene assignment
- Upstream/downstream genes provided for context

**Denormalized Study Metadata**:
- Basic study info duplicated in association entries
- Full metadata available in gwas_study dataset
- Tradeoff for query convenience

## Future Work

- Add ancestry information per association
- Implement association aggregation across studies (meta-analysis)
- Enhanced genomic annotations (consequence predictions)
- Statistical filtering (p-value thresholds, effect size ranges)
- Gene-level summaries (all associations per gene)
- Tissue/cell-type specificity from study metadata
- Conditional analysis results (independent signals)
- Fine-mapping probability integration
- Polygenic risk score support
- Linkage disequilibrium-aware grouping

## Maintenance

- **Release Schedule**: Weekly updates from GWAS Catalog
- **Current Version**: GWAS Catalog (updated weekly)
- **Data Format**: TSV in ZIP archive
- **Total Associations**: 1,000,000+ (growing continuously)
- **Unique SNPs**: ~800,000
- **Unique Studies**: ~6,000+
- **License**: Public domain / CC0
- **FTP**: ftp://ftp.ebi.ac.uk/pub/databases/gwas/releases/latest/

## References

- **Citation**: Buniello A, et al. (2019) The NHGRI-EBI GWAS Catalog of published genome-wide association studies, targeted arrays and summary statistics 2019. Nucleic Acids Res. 47(D1):D1005-D1012.
- **Website**: https://www.ebi.ac.uk/gwas/
- **API**: https://www.ebi.ac.uk/gwas/rest/docs/api
- **Documentation**: https://www.ebi.ac.uk/gwas/docs/
- **License**: CC0 (public domain)
- **FTP**: ftp://ftp.ebi.ac.uk/pub/databases/gwas/
