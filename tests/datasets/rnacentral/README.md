# RNACentral Non-Coding RNA Database

## Overview

RNACentral is a comprehensive database of non-coding RNA sequences that aggregates data from 56 expert databases into a single resource. Provides curated RNA sequence information with metadata including RNA type classification, organism distribution, and cross-references to source databases. Contains 49.8+ million unique RNA sequences covering all major ncRNA families. Essential for RNA biology research, non-coding RNA annotation, and functional genomics studies.

**Source**: RNACentral Database (EMBL-EBI FTP)
**Data Type**: Non-coding RNA sequences with metadata (rRNA, tRNA, miRNA, lncRNA, etc.)

## Integration Architecture

### Storage Model

**Primary Entries**:
- RNACentral IDs (e.g., `URS000149A9AF`) serve as primary keys
- Metadata stored as attributes (no sequences stored initially)

**Searchable Text Links**:
- RNACentral IDs indexed as keywords for direct lookup

**Attributes Stored** (protobuf RnacentralAttr):
- `rna_type`: RNA classification (rRNA, miRNA, lncRNA, tRNA, snoRNA, etc.)
- `description`: Human-readable description from FASTA header
- `length`: Sequence length in nucleotides
- `organism_count`: Number of distinct organisms
- `databases`: Array of source database names (e.g., ["ENA", "RefSeq", "miRBase"])
- `is_active`: Boolean indicating if entry is active (non-obsolete)
- `md5`: MD5 checksum of sequence (for future validation)

**Cross-References** (via id_mapping.tsv.gz):
- Self-reference for keyword lookup
- Ensembl (gene + transcript IDs)
- RefSeq
- ENA (accession IDs, version stripped)
- PDB (structure IDs, chain stripped)
- HGNC
- Model organism databases: MGI, RGD, FlyBase, SGD, TAIR

### Special Features

**FASTA Streaming Architecture**:
- Processes 8.4 GB compressed FASTA without full extraction
- Memory-efficient streaming from FTP source
- Early termination in test mode (100 entries)

**Intelligent RNA Type Detection**:
- Parses RNA type from FASTA description
- Normalizes to standard nomenclature (rRNA, miRNA, lncRNA, etc.)
- Handles 20+ RNA type variations

**Organism Distribution**:
- Extracts organism count from description
- Tracks multi-organism consensus sequences
- Supports filtering by organism prevalence

**Active Status Tracking**:
- Uses rnacentral_active.fasta.gz (active sequences only)
- Marks entries as active in attributes
- Excludes obsolete/deprecated sequences

## Use Cases

**1. Non-Coding RNA Annotation**
```
Query: Gene coordinates → Find overlapping ncRNAs → RNACentral metadata
Use: Annotate ncRNA features in genome assemblies
```

**2. RNA Type Distribution Analysis**
```
Query: All RNAs in organism → Filter by rna_type → Count distribution
Use: Study ncRNA repertoire across species
```

**3. Conserved RNA Discovery**
```
Query: RNAs with organism_count > 10 → Check conservation → Identify functional RNAs
Use: Find evolutionarily conserved regulatory RNAs
```

**4. Database Coverage Assessment**
```
Query: RNA sequence → Check distinct_databases → Evaluate annotation quality
Use: Determine which databases have curated this RNA
```

**5. RNA Length Classification**
```
Query: Filter by length ranges → Classify as small/long ncRNA → Functional prediction
Use: Distinguish miRNAs (<30nt) from lncRNAs (>200nt)
```

**6. Quality Filtering**
```
Query: is_active = true AND organism_count > 1 → High-confidence RNAs only
Use: Filter for well-supported ncRNA annotations
```

## Test Cases

**Current Tests** (20 total):
- 9 declarative tests (JSON-based)
- 11 custom tests (Python logic)

**Coverage**:
- ✅ Basic ID lookup and data retrieval
- ✅ RNA type annotation validation
- ✅ Description format and content checks
- ✅ Sequence length range validation (10-50,000 nt)
- ✅ Organism count validation
- ✅ Active status verification
- ✅ All required fields present check
- ✅ RNA type diversity across test set
- ✅ Cross-reference validation (id_mapping.tsv integrated)
- ✅ Ensembl cross-reference tests
- ✅ ENA cross-reference tests
- ✅ Reverse xref lookup tests

**Recommended Additions**:
- MD5 checksum validation
- Specific RNA type tests (miRNA, lncRNA, tRNA)
- Multi-organism consensus sequence tests
- Database source distribution tests
- Isoform/variant handling tests

## Performance

- **Test Build**: ~2.3s (100 sequences from active FASTA)
- **Data Source**: FTP streaming from EMBL-EBI (no local download)
- **Active FASTA**: 8.4 GB compressed (49.8M sequences)
- **ID Mapping**: 1.7 GB compressed (cross-references)
- **Full Build**: Hours (49.8M sequences)
- **Memory Usage**: Streaming architecture, minimal memory footprint
- **Test Database Size**: ~5 MB

## Known Limitations

**Sequence Storage**:
- Currently stores metadata only (not full sequences)
- Sequences available via RNACentral API
- Future: Optional sequence storage for local queries

**Cross-References**:
- ID mapping file (id_mapping.tsv.gz) now integrated
- Some databases excluded due to incompatible ID formats (see below)

**Excluded ID Mappings** (incompatible formats):

| Database | RNACentral Format | Biobtree Format | Reason |
|----------|-------------------|-----------------|--------|
| IntAct | `INTACT:URS...` | `EBI-xxxxx` | Wrong ID type (uses URS IDs instead of interaction IDs) |
| WormBase | `WBGene00000005` | `4R79.1A` | Incompatible ID types (gene IDs vs cosmid IDs) |
| PomBase | `SPNCRNA.817.1` | `SPAC1002.01` | Incompatible ID types (RNA IDs vs gene IDs) |

**ID Format Normalization** (handled automatically):

| Database | RNACentral Format | Normalized To |
|----------|-------------------|---------------|
| ENA | `GU786683.1:1..200:rRNA` | `GU786683` (strip coordinates + version) |
| PDB | `157D_A` | `157D` (strip chain suffix) |
| TAIR | `AT1G01270.1` | `AT1G01270` (strip version) |

**RNA Type Coverage**:
- Detection based on description parsing
- May miss complex/novel RNA types
- Standardization for 20+ types (some ambiguity possible)

**Version Tracking**:
- Currently stores latest version only (Release 25)
- Historical versions not tracked
- Update frequency: quarterly

**Organism Information**:
- Count extracted from description (may be approximate)
- Detailed organism taxonomy not stored
- Links to NCBI Taxonomy not yet integrated

## Future Work

- Add optional sequence storage (configurable)
- Implement per-species filtering (via taxonomy IDs)
- Add RNA secondary structure data (from secondary_structures files)
- Test coverage for specific RNA types (miRNA-specific tests)
- Add sequence similarity search capabilities
- Integrate with Rfam for RNA family annotations
- Add publication links (via publications API)
- Test multi-fragment/isoform handling

## Maintenance

- **Release Schedule**: Quarterly updates from RNACentral
- **Data Format**: FASTA.gz (streaming) + TSV.gz (mappings)
- **Test Data**: 100 entries (diverse RNA types)
- **Current Version**: Release 25 (May 2025)
- **License**: CC0 1.0 (public domain dedication)
- **Full Dataset**: 49.8M active sequences (~23 TB uncompressed)

## References

- **Citation**: The RNACentral Consortium (2021) RNACentral 2021: secondary structure integration, improved sequence search and new member databases. Nucleic Acids Res. 49(D1):D212-D220.
- **Website**: https://rnacentral.org/
- **API**: https://rnacentral.org/api/v1/
- **FTP**: https://ftp.ebi.ac.uk/pub/databases/RNAcentral/
- **License**: CC0 1.0
