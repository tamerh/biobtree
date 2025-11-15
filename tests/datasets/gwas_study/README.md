# GWAS Study Dataset Tests

Tests for GWAS Catalog study metadata integration.

## Dataset Overview

**GWAS Study** provides metadata for genome-wide association studies from the NHGRI-EBI GWAS Catalog. Each study represents a published GWAS with information about:
- Publication details (PubMed ID, authors, journal)
- Disease/trait being investigated
- Sample populations and ancestry
- Genotyping platforms and methods
- Number of SNP associations found

The dataset contains 182,000+ studies and is updated regularly from the GWAS Catalog.

## Test Architecture

The test suite validates:
1. **Study lookup** by accession ID (GCST*)
2. **Publication metadata** extraction (PubMed, date, author)
3. **Disease/trait mappings** to EFO ontology
4. **Association counts** for each study
5. **Text search** for disease terms
6. **Cross-references** to EFO traits

Tests use 100 fixed study IDs for reproducibility.

## Running Tests

```bash
# Run gwas_study tests only
python3 tests/run_tests.py gwas_study

# Run from this directory
python3 test_gwas_study.py
```

## Test Data

- **gwas_study_ids.txt**: 100 study accession IDs
- **reference_data.json**: Complete API responses from GWAS Catalog
- **test_cases.json**: Declarative test definitions
- **extract_reference_data.py**: Script to refresh reference data

## Relationship with GWAS Dataset

This dataset provides study **metadata**. The separate `gwas` dataset provides SNP-trait **associations**. Together they enable queries like:

```bash
# Find studies about diabetes
biobtree query "diabetes >> gwas_study"

# Find SNPs from a specific study
biobtree query "GCST010481 >> gwas"

# Find studies that found associations for a gene
biobtree query "BRCA1 >> gwas >> gwas_study"
```

## Performance

- Study lookup: ~10ms
- Text search: ~50ms
- Full test suite: ~30 seconds
