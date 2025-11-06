# Ensembl Genome Annotation Database

## Overview

Ensembl provides comprehensive genome annotations for vertebrates and selected model organisms, integrating gene structures, transcripts, proteins, and regulatory elements with rich cross-references to external databases. Contains 70+ million genes across 300+ species with detailed metadata including genomic coordinates, biotypes (protein_coding, lncRNA, pseudogene, etc.), canonical transcripts, and extensive xref mappings. Essential for genome-wide analyses, variant interpretation, and functional genomics studies requiring authoritative gene annotations.

**Source**: Ensembl REST API (EMBL-EBI)
**Data Type**: Gene annotations with genomic coordinates and cross-references

## Integration Architecture

### Storage Model

**Primary Entries**:
- Ensembl Gene IDs (e.g., `ENSG00000290825`) serve as primary keys
- Comprehensive gene metadata stored as attributes

**Searchable Text Links**:
- Gene IDs indexed as keywords for direct lookup
- Gene symbols (display_name) indexed for symbol-based search

**Attributes Stored** (protobuf EnsemblAttr):
- `display_name`: Gene symbol/name (e.g., "DDX11L16")
- `biotype`: Gene classification (protein_coding, lncRNA, pseudogene, miRNA, etc.)
- `description`: Functional description with source annotation
- `strand`: Genomic strand orientation (+1 forward, -1 reverse)
- `start`, `end`: Genomic coordinates (base pairs)
- `seq_region_name`: Chromosome or contig identifier
- `assembly_name`: Genome assembly version (e.g., "GRCh38")
- `species`: Organism name (e.g., "homo_sapiens")
- `canonical_transcript`: Representative transcript ID
- `version`: Gene version number
- `source`: Annotation source (ensembl, havana, ensembl_havana)
- `logic_name`: Annotation pipeline identifier

**Cross-References**:
- **NCBI Gene (EntrezGene)**: Gene IDs for cross-species integration
- **UniProtKB**: Protein sequences and functional annotations
- **RefSeq**: NCBI reference sequences
- **HGNC**: Human gene nomenclature (human genes only)
- **MGI/RGD/ZFIN**: Model organism databases (mouse, rat, zebrafish)
- **GO**: Gene Ontology functional annotations
- **MIM**: OMIM disease associations
- **ArrayExpress**: Expression data
- **Many others**: WikiGene, miRBase, RFAM, etc.

### Special Features

**Multi-Species Architecture**:
- Main Ensembl: Vertebrates (human, mouse, rat, etc.)
- Ensembl Genomes divisions:
  - Bacteria: Bacterial and archaeal genomes
  - Fungi: Fungal species
  - Metazoa: Invertebrate animals
  - Plants: Plant genomes
  - Protists: Protist genomes
- Unified data model across all divisions

**REST API Streaming**:
- Real-time data fetching from Ensembl REST API
- No large downloads required
- Configurable genome selection via taxonomy IDs
- Test mode supports limited entry extraction

**Rich Biotype Classification**:
- 50+ gene biotypes for precise categorization
- protein_coding, lncRNA, miRNA, snRNA, snoRNA
- pseudogene (various subtypes)
- TEC (To be Experimentally Confirmed)
- IG/TR immunoglobulin/T-cell receptor genes

**Genomic Context**:
- Full coordinate information for genome browsers
- Strand orientation for directional analyses
- Assembly-specific coordinates
- Canonical transcript designation for isoform selection

## Use Cases

**1. Gene ID to Symbol Mapping**
```
Query: ENSG00000290825 → Retrieve display_name → "DDX11L16"
Use: Convert Ensembl IDs to human-readable gene names
```

**2. Genomic Coordinate Lookup**
```
Query: Gene ID → Extract chr:start-end:strand → chr1:11121-24894:+
Use: Visualize genes in genome browsers, overlap with variants
```

**3. Biotype-Based Filtering**
```
Query: All genes → Filter biotype="protein_coding" → Coding genes only
Use: Focus analyses on protein-coding genes vs. ncRNAs
```

**4. Cross-Database Integration**
```
Query: Ensembl gene → xrefs → EntrezGene/UniProt/HGNC IDs
Use: Link genomic data with protein databases and literature
```

**5. Variant Annotation**
```
Query: Variant position → Overlapping genes → Gene IDs and impact
Use: Determine which genes are affected by variants
```

**6. Orthology and Comparative Genomics**
```
Query: Human gene → Multi-species Ensembl → Find orthologs
Use: Cross-species functional studies
```

## Test Cases

**Current Tests** (7 total):
- 4 declarative tests (JSON-based)
- 3 custom tests (Python logic)

**Coverage**:
- ✅ Basic gene ID lookup
- ✅ Attribute presence validation
- ✅ Multiple ID batch lookup (3 genes)
- ✅ Invalid ID handling
- ✅ Gene symbol (display_name) check
- ✅ Biotype annotation validation
- ✅ Strand orientation presence

**Recommended Additions**:
- Canonical transcript validation
- Cross-reference integrity (xrefs to UniProt, NCBI Gene)
- Genomic coordinate validity (start < end, valid chromosomes)
- Species-specific tests (human, mouse, etc.)
- Assembly version consistency
- Biotype distribution across test set
- Multi-species test coverage (bacteria, fungi, etc.)
- Gene length range validation
- Source annotation validation (ensembl/havana)

## Performance

- **Test Build**: ~5s (20 genes from human + 6 model organisms)
- **Data Source**: REST API streaming from Ensembl
- **Full Build**: Hours to days (depends on species selection)
- **Total Genes**: 70M+ across all divisions
- **Species Coverage**: 300+ genomes
- **Test Organisms**:
  - Human (homo_sapiens, 9606)
  - E. coli (escherichia_coli, 1268975)
  - A. fumigatus (aspergillus_fumigatus, 330879)
  - D. melanogaster (drosophila_melanogaster, 7227)
  - A. thaliana (arabidopsis_thaliana, 3702)
  - P. falciparum (plasmodium_falciparum, 36329)
- **Test Database Size**: ~1.5 MB (20 genes/organism × 6 organisms)

## Known Limitations

**API Rate Limiting**:
- Ensembl REST API has request rate limits
- Large builds may require retries and backoff
- Test mode uses minimal requests (20 genes/species)

**Genome Selection**:
- Must specify taxonomy IDs via `--genome-taxids`
- No "all species" mode (would be too large)
- Test mode pre-selects 6 representative organisms

**Version Tracking**:
- Ensembl releases quarterly (4 releases/year)
- No historical version storage
- Assembly versions tracked per gene

**Coordinate Systems**:
- Coordinates are assembly-specific
- Liftover needed for cross-assembly analyses
- Patch sequences not fully integrated

**Data Completeness**:
- Annotation quality varies by species
- Model organisms have richest annotations
- Non-model species may lack xrefs

**Divisions Synchronization**:
- Main Ensembl and Ensembl Genomes on different release cycles
- Division-specific features may not be uniform
- Test mode builds all divisions together

## Future Work

- Add transcript-level data (currently gene-level only)
- Integrate exon/CDS coordinates for variant effect prediction
- Add protein domain annotations (Pfam, InterPro via xrefs)
- Implement assembly liftover support
- Add regulatory region annotations (promoters, enhancers)
- Test coverage for all 6 Ensembl divisions
- Species-specific test suites (separate for human, mouse, etc.)
- Homology/orthology relationship mapping
- Gene tree and phylogeny integration
- Variation data integration (dbSNP, ClinVar via xrefs)

## Maintenance

- **Release Schedule**: Quarterly (Ensembl main), variable (Genomes divisions)
- **Current Version**: Ensembl 113 (November 2024)
- **Data Format**: REST API JSON responses
- **Test Data**: 120 genes (20 per species × 6 organisms)
- **License**: Open data (no restrictions)
- **API Documentation**: https://rest.ensembl.org/

## References

- **Citation**: Martin FJ, et al. (2023) Ensembl 2023. Nucleic Acids Res. 51(D1):D933-D941.
- **Website**: https://www.ensembl.org/
- **REST API**: https://rest.ensembl.org/
- **FTP**: https://ftp.ensembl.org/pub/
- **Genomes**: https://ensemblgenomes.org/
- **License**: Open data
