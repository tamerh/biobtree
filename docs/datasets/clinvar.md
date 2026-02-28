# ClinVar Dataset

## Overview

ClinVar is a freely accessible public archive of reports of the relationships among human variations and phenotypes, with supporting evidence. It aggregates information about genomic variation and its relationship to human health from multiple sources worldwide.

**Source**: NCBI ClinVar (https://ftp.ncbi.nlm.nih.gov/pub/clinvar/)
**Data Type**: Genetic variants with clinical significance annotations, phenotype associations, and genomic locations

## Integration Architecture

### Storage Model
**Primary Entries**: VariationID (numeric identifier) - main key for each variant
**Searchable Text Links**: Variant names (HGVS expressions), dbSNP IDs (rs numbers), gene symbols
**Attributes Stored**: Clinical significance, review status, genomic coordinates (GRCh38/GRCh37), variant type, alleles, phenotypes, gene associations, HGVS expressions, cross-references
**Cross-References**: HGNC genes (via HGNC_ID), dbSNP variants, HPO phenotypes, gene identifiers

### Special Features

- **Multi-file Integration**: Combines data from 4 NCBI files (variant_summary.txt.gz, var_citations.txt, allele_gene.txt, variant_summary_old.txt.gz for HGVS)
- **Bidirectional Gene Links**: Variants link to genes (via HGNC), genes can discover associated variants
- **Genomic Coordinates**: Stores both GRCh38 (primary) and GRCh37 coordinates when available
- **Text Search**: All HGVS expressions, dbSNP IDs, and gene symbols indexed for text search
- **Clinical Classification**: Stores germline and somatic clinical significance classifications
- **Review Status Tracking**: Captures assertion criteria and review status (stars)
- **Phenotype Associations**: Links variants to disease phenotypes and HPO terms
- **AlleleID Tracking**: Preserves NCBI's AlleleID for variant allele grouping

## Use Cases

**1. Clinical Variant Interpretation**
```
Query: Check pathogenicity of specific variant → VariationID lookup → Clinical significance + review status
Use: Determine if a patient's genetic variant is pathogenic, benign, or uncertain
```

**2. Gene-Variant Discovery**
```
Query: Find all variants in BRCA1 gene → Search by gene symbol → All BRCA1 variants
Use: Identify known pathogenic variants in cancer susceptibility genes
```

**3. dbSNP Cross-Reference**
```
Query: Look up rs number → dbSNP ID search → ClinVar variant + clinical data
Use: Connect population variant data to clinical significance
```

**4. Phenotype-Variant Mapping**
```
Query: Find variants associated with disease → Phenotype search → Variants + clinical classification
Use: Research genetic basis of specific diseases and conditions
```

**5. Variant Classification Research**
```
Query: All Pathogenic vs VUS variants → Filter by classification → Compare characteristics
Use: Study patterns in variant interpretation and clinical evidence
```

**6. Genomic Coordinate Search**
```
Query: Variants in specific genomic region → chr:start-stop → All overlapping variants
Use: Identify variants in regulatory regions or specific genes
```

## Test Cases

**Current Tests** (19 total):
- 9 declarative tests (ID lookup, attribute checks, multi-lookup, invalid ID)
- 10 custom tests (pathogenic/benign/VUS variants, dbSNP IDs, HGNC genes, SNV types, review status, phenotypes, alleles, genomic location)

**Coverage**:
- ✅ Basic variant lookup by VariationID
- ✅ Clinical significance classifications (Pathogenic, Benign, VUS)
- ✅ Variant types (SNV, indel, etc.)
- ✅ Genomic coordinates (chromosome, start, stop, assembly)
- ✅ dbSNP ID associations
- ✅ HGNC gene cross-references
- ✅ Review status validation
- ✅ Phenotype associations
- ✅ Allele information (reference/alternate)
- ✅ Multiple variant lookups

**Recommended Additions**:
- Test HGVS expression search (text search for variant names)
- Test somatic variant classifications
- Test HPO phenotype cross-references
- Test citation associations
- Test variants with multiple clinical significance values
- Test GRCh37 vs GRCh38 coordinate differences

## Performance

- **Test Build**: ~3-5s (100 variants)
- **Data Source**: NCBI FTP (https://ftp.ncbi.nlm.nih.gov/pub/clinvar/tab_delimited/)
- **Update Frequency**: Monthly releases from NCBI
- **Total Entries**: ~2.5M variants (as of 2024)
- **Source Files**:
  - variant_summary.txt.gz (~100 MB, main data file)
  - var_citations.txt (~200 MB, publication links)
  - allele_gene.txt (~60 MB, allele-gene mappings)
  - variant_summary_old.txt.gz (~400 MB, additional HGVS expressions)

## Known Limitations

- **HGNC Cross-References**: Only created when HGNC dataset is integrated
- **HPO Cross-References**: Require HPO dataset integration for phenotype links
- **GRCh37 Data**: Some older variants only have GRCh37 coordinates (GRCh38 preferred)
- **Allele Grouping**: Multiple VariationIDs can share same AlleleID (not currently aggregated)
- **Text Search**: HGVS expressions searchable but complex nomenclature may require exact matches
- **Missing Data**: Some variants have incomplete annotations (phenotypes, classifications)

## Future Work

- Add support for variant allele aggregation (group by AlleleID)
- Enhance text search for partial HGVS expression matching
- Add somatic variant classification support
- Integrate with ClinGen expert panel classifications
- Add support for structural variant representations
- Implement conflict resolution tracking (disagreements between submitters)
- Add submission history and version tracking

## Maintenance

- **Release Schedule**: Monthly updates from NCBI ClinVar
- **Data Format**: Tab-delimited text files (TSV), gzip compressed
- **Test Data**: 100 variant entries covering diverse variant types and classifications
- **License**: Public Domain (NCBI data)
- **Special Notes**:
  - Data combined from 4 separate NCBI files
  - Coordinates use both GRCh38 (primary) and GRCh37 assemblies
  - Clinical significance can have multiple values (e.g., "Pathogenic/Likely pathogenic")
  - Review status follows ClinVar star system (0-4 stars)

## References

- **Citation**: Landrum MJ, et al. ClinVar: improving access to variant interpretations and supporting evidence. Nucleic Acids Res. 2018
- **Website**: https://www.ncbi.nlm.nih.gov/clinvar/
- **License**: Public Domain
