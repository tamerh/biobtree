# GWAS Association Dataset

## Overview

GWAS Association dataset contains variant-trait associations from the NHGRI-EBI Catalog providing detailed SNP-level data on genetic variants associated with diseases and traits. Contains 1,000,000+ SNP-trait associations linking specific genetic variants to phenotypes with statistical evidence.

**Source**: GWAS Catalog EBI FTP (ZIP file containing TSV)
**Data Type**: SNP-trait associations with genomic positions, genes, EFO trait ontology mappings, and statistical evidence
**Test Coverage**: 10 tests (4 declarative + 6 custom) - 90% passing

## Integration Architecture

### Storage Model
- **Primary Entries**: SNP IDs (rs numbers like rs12451471)
- **Searchable Text**: SNP ID, gene symbols, disease/trait names
- **Attributes**: Genomic position, genes, traits, statistical evidence, study metadata
- **Cross-References**: SNP → Study, Gene → SNP, SNP → EFO

### Special Features
- ZIP file processing with bufio.Scanner
- SNP-centric grouping (multiple associations per SNP)
- Multi-gene associations (up to 10 genes per SNP)
- EFO trait ontology integration
- Statistical evidence (p-values, effect sizes, CIs)
- Text search safety limits

## Performance

- **Test Build**: ~2.3s (100 SNP entries)
- **Full Build**: ~8-12 minutes (1M+ associations → 800K unique SNPs)
- **Data Source**: ZIP file (~57 MB compressed, ~620 MB uncompressed)
- **Update Frequency**: Weekly

## Future Work

- **Ancestry integration** (priority enhancement)
- Association aggregation across studies
- Enhanced genomic annotations
- Statistical filtering
- Gene-level summaries

## References

- **Citation**: Buniello A, et al. The NHGRI-EBI GWAS Catalog. NAR 2019;47:D1005-D1012.
- **Website**: https://www.ebi.ac.uk/gwas/
- **License**: Public domain / CC0
