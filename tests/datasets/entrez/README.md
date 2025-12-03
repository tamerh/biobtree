# Entrez Gene Dataset

## Overview
NCBI Entrez Gene is the authoritative source for gene-level information across all sequenced organisms. It provides standardized nomenclature, gene symbols, descriptions, chromosomal locations, and extensive cross-references. Key resource for gene-centric research with >30 million gene records.

**Source**: NCBI (National Center for Biotechnology Information)
**Data Type**: Gene records with symbols, descriptions, locations, and relationships

## Integration Architecture

### Storage Model
**Primary Entries**: Entrez Gene IDs (numeric, e.g., "672" for BRCA1)
**Searchable Text Links**: Gene symbols, synonyms, descriptions
**Attributes Stored**: name, symbol, type, synonyms, chromosome, summary, start_position, end_position, orientation, genomic_accession
**Cross-References**: GO terms (gene2go), publications (gene2pubmed), orthologs (gene_orthologs), neighbors (gene_neighbors)

### Special Features
- **Gene Neighbor Relationships**: Stores left/right neighbor genes with "relationship" field (6th column in index)
- **Ortholog Links**: Cross-species gene mappings with taxonomy references
- **Gene Groups**: Related genes grouped together
- **Multiple Data Files**: Integrates gene_info, gene2go, gene_summary, gene_orthologs, gene2pubmed, gene_group, gene_neighbors

## Use Cases

**1. Gene Function Lookup**
```
Query: What does BRCA1 do? -> Query "672" -> Get description, GO terms
Use: Cancer research, understanding tumor suppressor function
```

**2. Genomic Location**
```
Query: Where is TP53 located? -> Query "7157" -> Chromosome 17
Use: Cytogenetic mapping, chromosomal aberration analysis
```

**3. Ortholog Discovery**
```
Query: Mouse orthologs of human gene -> Query gene -> Get ortholog xrefs
Use: Model organism selection, evolutionary conservation studies
```

**4. Gene Neighborhood Analysis**
```
Query: Neighboring genes -> Query gene -> Get left/right neighbors
Use: Synteny analysis, gene cluster identification
```

**5. Literature Association**
```
Query: Publications for gene -> Query gene -> Get PubMed xrefs
Use: Research background, experimental evidence collection
```

**6. Functional Annotation**
```
Query: GO terms for gene -> Query gene -> Get GO xrefs
Use: Pathway analysis, functional enrichment studies
```

## Test Cases

**Current Tests** (11 total):
- 5 declarative tests (ID lookup, symbol lookup, attribute check, multi-lookup, invalid ID)
- 6 custom tests (symbol validation, description check, chromosome location, gene type, cross-references, taxonomy reference)

**Coverage**:
- Gene ID lookup
- Symbol-based search
- Attribute validation (name, symbol, type, chromosome)
- Cross-reference presence (taxonomy)

**Recommended Additions**:
- Ortholog relationship validation
- Neighbor gene relationship tests (left_neighbor, right_neighbor)
- GO term cross-reference validation
- Synonym/alias search tests

## Performance

- **Test Build**: ~30s (1000 entries)
- **Data Source**: NCBI FTP (gene_info.gz ~3GB compressed)
- **Update Frequency**: Daily at NCBI
- **Total Entries**: >30 million genes across all species
- **Special notes**: Large download, uses multiple supplementary files

## Known Limitations

- Gene neighbors only available for reference genomes
- Summary field not available for all genes
- Some organisms have limited annotation
- Deprecated/discontinued genes marked but still present
- **LMDB key length limit**: Some long text keywords (gene descriptions, synonyms) are excluded from text search indexing due to LMDB's maximum key size constraint (~511 bytes). This will be resolved when full-text search is implemented for keywords.

## Future Work

- Add tests for ortholog relationships (orthologentrez link dataset)
- Add tests for neighbor relationships (neighborentrez link dataset)
- Validate GO term cross-references
- Test gene group associations
- Add tests for related genes (relatedentrez link dataset)

## Maintenance

- **Release Schedule**: Daily updates at NCBI
- **Data Format**: Tab-delimited text (gene_info.gz)
- **Test Data**: 20 genes (human, well-characterized)
- **License**: Public domain (NCBI data)

## References

- **Citation**: NCBI Resource Coordinators. Database resources of the National Center for Biotechnology Information. Nucleic Acids Res. 2024.
- **Website**: https://www.ncbi.nlm.nih.gov/gene/
- **License**: Public domain
