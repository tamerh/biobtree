# HGNC (HUGO Gene Nomenclature Committee) Dataset

## Overview

HGNC is the official authority for standardizing and approving human gene names. Provides approved gene symbols, names, aliases, chromosomal locations, and cross-references to major genomics databases. Essential for consistent gene nomenclature across research and clinical applications.

**Source**: HGNC REST API (https://www.genenames.org/)
**Data Type**: Human gene nomenclature with approved symbols and cross-database mappings

## Integration Architecture

### Storage Model

**Primary Entries**:
- HGNC IDs (e.g., `HGNC:5`) stored as main identifiers

**Searchable Text Links**:
- Gene symbols (e.g., `A1BG`, `TP53`) → self-referencing keywords
- Alias symbols (previous and alternative names) → self-referencing keywords
- Previous symbols (deprecated names) → self-referencing keywords
- Locus group/type (e.g., `protein-coding gene`) → text search
- Chromosomal location (e.g., `19q13.43`) → text search
- Gene groups (e.g., `Immunoglobulin like domain containing`) → text search

**Cross-References** (external database IDs):
- **UniProt**: Protein sequences (`uniprot_ids`)
- **Ensembl**: Genome annotations (`ensembl_gene_id`)
- **RefSeq**: NCBI reference sequences (`refseq_accession`)
- **OMIM**: Disease associations (`omim_id`)
- **COSMIC**: Cancer mutations (`cosmic`)
- **VEGA**: Manual annotations (`vega_id`)
- **CCDS**: Consensus CDS (`ccds_id`)
- **EMBL**: Nucleotide sequences (`ena`)
- **PubMed**: Literature references (`pubmed_id`)
- **Enzyme**: EC numbers (`enzyme_id`)

### Special Features

**Nomenclature History Tracking**:
- Approved symbols + previous symbols + alias symbols all searchable
- Allows finding genes by any historical name

**Status Field**:
- Approved, Entry Withdrawn, Symbol Withdrawn
- Tracks gene nomenclature lifecycle

**Attributes Stored** (protobuf):
- Symbols, aliases, previous symbols
- Names (approved + previous)
- Locus group, locus type, status
- Chromosomal location
- Gene groups

## Use Cases

**1. Gene Name Resolution**
```
Query: "TP53" → Find HGNC:11998 → Retrieve all aliases and previous symbols
Use: Resolve ambiguous gene names in literature mining
```

**2. Symbol History Lookup**
```
Query: Previous symbol "BCL2L" → Find current symbol "BCL2L1" (HGNC:987)
Use: Update old gene symbols in legacy datasets
```

**3. Multi-Database Gene Mapping**
```
Query: HGNC:5 → Get UniProt:P04217, Ensembl:ENSG00000121410, RefSeq:NM_130786
Use: Link gene information across genomics databases
```

**4. Chromosomal Location Search**
```
Query: "19q13.43" → Find all genes at this locus
Use: Identify genes in specific genomic regions
```

**5. Gene Family Discovery**
```
Query: "Immunoglobulin like domain containing" → Find gene group members
Use: Study functionally related gene families
```

**6. Clinical Variant Annotation**
```
Query: Gene symbol in variant report → Validate current approved symbol → Get OMIM/COSMIC refs
Use: Annotate clinical genomic variants with standardized nomenclature
```

## Test Cases

**Current Tests** (13 total):
- 8 declarative tests (lookup, alias/previous symbol resolution, attributes, case-insensitive)
- 5 custom tests (status validation, locus_group, location, gene_group, cross-references)

**Coverage**:
- ✅ HGNC ID and symbol lookup
- ✅ Alias and previous symbol resolution
- ✅ Case-insensitive search
- ✅ Approved status validation
- ✅ Locus group/type fields
- ✅ Chromosomal location
- ✅ Gene group classification
- ✅ Cross-reference availability

**Recommended Additions**:
- Withdrawn symbol handling
- Multiple alias resolution for same gene
- Gene name vs symbol disambiguation

## Performance

- **Test Build**: ~2.4s (100 gene entries)
- **Data Source**: JSON from HGNC REST API (downloads via HTTP)
- **Update Frequency**: Monthly releases
- **Total Genes**: ~40,000+ approved human gene symbols

## Known Limitations

**Cross-Reference Availability**:
- Not all genes have mappings to all databases (especially for non-coding RNAs, pseudogenes)
- Test data may have limited xrefs when built in isolation

**Name Field**:
- Gene names (full descriptions) not added as searchable text to avoid noise in search results
- Only symbols/aliases/locations are text-searchable

## Future Work

- Add validation for withdrawn symbols and their replacements
- Test gene group membership queries
- Add support for locus_group filtering (protein-coding vs non-coding)

## Maintenance

- **Release Schedule**: Monthly updates from HGNC
- **Data Format**: JSON from REST API (backward compatible)
- **Test Data**: Fixed 100 gene IDs spanning various gene types

## References

- **Citation**: HGNC Database, HUGO Gene Nomenclature Committee (HGNC), EMBL-EBI
- **Website**: https://www.genenames.org/
- **License**: Freely available for academic and commercial use
