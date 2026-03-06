# UniParc (UniProt Archive) Dataset

## Overview

UniParc (UniProt Archive) is a comprehensive sequence archive containing all publicly available protein sequences from major databases. Each UniParc entry represents a unique protein sequence with extensive cross-references to source databases (UniProtKB, RefSeq, Ensembl, EMBL, PDB, etc.), tracking all occurrences of that sequence across different organisms and annotation states. Contains 600+ million sequences, providing a non-redundant protein sequence collection essential for sequence-based searches and tracking sequence provenance across databases.

**Source**: UniProt FTP (EBI)
**Data Type**: Non-redundant protein sequences with extensive cross-references

## Integration Architecture

### Storage Model

**Primary Entries**:
- UniParc IDs (e.g., `UPI00000001A2`) stored as main identifiers

**Searchable Text Links**:
- UniParc IDs indexed for lookup
- Sequence checksums for deduplication

**Attributes Stored** (protobuf UniParcAttr):
- `sequence`: Amino acid sequence
- `sequence_length`: Length in residues
- `sequence_checksum`: CRC64 checksum for uniqueness
- `uniparc_cross_references`: Array of database cross-references (see below)

**Cross-Reference Attributes**:
- `database`: Source database (UniProtKB/Swiss-Prot, UniProtKB/TrEMBL, RefSeq, Ensembl, EMBL, PDB, etc.)
- `id`: Identifier in source database
- `version`: Entry version
- `active`: Current status (active/inactive)
- `created`: First appearance date
- `lastUpdated`: Last modification date
- `geneName`: Gene name (when available)
- `proteinName`: Protein description
- `organism`: Species information (scientific name, common name, taxon ID)
- `proteomeId`: Reference proteome identifier
- `component`: Genomic location (chromosome, plasmid)

**Cross-References**:
- **UniProtKB**: Swiss-Prot (reviewed) and TrEMBL (unreviewed) entries
- **RefSeq**: NCBI Reference Sequence entries
- **Ensembl**: Protein predictions from genome assemblies
- **EMBL**: Nucleotide sequence database entries
- **PDB**: Protein structure database entries
- **Plus 100+ other databases**: PIR, H-InvDB, PATRIC, WormBase, etc.

### Special Features

**Sequence-Centric Integration**:
- Single sequence → all database occurrences
- Tracks sequence provenance across databases
- Identifies identical sequences with different annotations

**Version Tracking**:
- Records creation and update dates for each xref
- Tracks active/inactive status
- Maintains version history

**Multi-Organism Coverage**:
- Same sequence across multiple species
- Organism information for each cross-reference
- Enables cross-species sequence analysis

**Comprehensive Cross-References**:
- 100+ source databases
- Genomic, proteomic, structural databases
- Active and historical entries

## Use Cases

**1. Sequence Deduplication**
```
Query: Protein sequence → UniParc checksum → UniParc ID
Use: Identify if sequence already exists in public databases
```

**2. Cross-Database Mapping**
```
Query: UniParc ID → All xrefs → Map between databases (UniProt ↔ RefSeq ↔ Ensembl)
Use: Convert identifiers across different annotation systems
```

**3. Sequence Provenance Tracking**
```
Query: Sequence → UniParc entry → All source submissions
Use: Track origin and history of sequence submissions
```

**4. Redundancy Analysis**
```
Query: Multiple sequences → UniParc IDs → Identify duplicates
Use: Detect redundant sequences in datasets
```

**5. Historical Sequence Tracking**
```
Query: UniParc ID → Inactive xrefs → Track deleted/merged entries
Use: Understand sequence history and database changes
```

**6. Cross-Species Sequence Analysis**
```
Query: Sequence → UniParc xrefs → Filter by organism
Use: Identify orthologous sequences across species
```

## Test Cases

**Current Tests** (6 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 2 custom tests (cross-references, sequence information)

**Coverage**:
- ✅ UniParc ID lookup
- ✅ Attribute validation
- ✅ Multi-ID batch lookups
- ✅ Invalid ID handling
- ✅ Cross-reference count validation
- ✅ Sequence length validation

**Recommended Additions**:
- Sequence checksum validation tests
- Active/inactive xref filtering tests
- Organism diversity tests
- Database type distribution tests (UniProtKB, RefSeq, Ensembl, etc.)
- Version tracking tests
- Gene name extraction tests
- Proteome ID validation tests
- Date range tests (created, lastUpdated)

## Performance

- **Test Build**: ~0.5s (10 sequences from 200 XML files)
- **Data Source**: UniProt FTP (200 gzipped XML files)
- **Update Frequency**: Weekly UniProt releases
- **Total Sequences**: 600+ million unique protein sequences
- **Full Build**: Several hours (processes all 200 files)
- **Note**: Test mode processes only first file until limit reached

## Known Limitations

**Data Volume**:
- 600M+ sequences = very large dataset
- Full build requires significant time and storage
- Test mode recommended for development

**Cross-Reference Completeness**:
- Not all xrefs have gene names or protein names
- Organism information may be missing for some sources
- Proteome IDs only for reference proteomes

**Active/Inactive Status**:
- Inactive xrefs indicate deleted/merged entries
- May create confusion if not filtered
- Historical data valuable but increases volume

**Version Tracking**:
- Version increments not always consecutive
- Some databases don't use versions
- Interpretation varies by source database

**FTP Access**:
- Requires stable FTP connection
- Large file downloads (200 files, each 200MB-2GB compressed)
- Network issues can interrupt build

## Future Work

- Add sequence checksum validation tests
- Test active/inactive xref filtering
- Add organism diversity analysis tests
- Test database type distribution
- Add version comparison tests
- Test gene name search functionality
- Add proteome ID filtering tests
- Test date-based queries (recent updates)
- Add cross-species sequence analysis tests
- Test sequence identity search
- Add xref count distribution analysis

## Maintenance

- **Release Schedule**: Weekly from UniProt
- **Data Format**: 200 gzipped XML files (~100-400MB each compressed)
- **Test Data**: Fixed 10 UniParc IDs from first file (uniparc_p1.xml.gz)
- **License**: CC BY 4.0 - freely available with attribution
- **Full Build**: Processes all 200 files sequentially
- **Coordination**: Part of UniProt suite (links to UniProtKB, TrEMBL)

## Database Cross-Reference Coverage

UniParc cross-references span 100+ databases including:

| Category | Databases | Examples |
|----------|-----------|----------|
| **Protein Sequences** | UniProtKB, PIR | Swiss-Prot, TrEMBL |
| **Genomic** | RefSeq, Ensembl, EMBL | RefSeq proteins, Ensembl transcripts |
| **Structural** | PDB | Experimental 3D structures |
| **Model Organisms** | WormBase, FlyBase, SGD | C. elegans, Drosophila, yeast |
| **Pathogens** | PATRIC, CGD | Bacterial genomes, fungal genomes |
| **Families/Domains** | Pfam, InterPro, PANTHER | Protein families and domains |
| **Expression** | GeneID, H-InvDB | Gene expression databases |
| **Patents** | JPO, KIPO, EPO | Patent office submissions |

## Sequence Uniqueness

**CRC64 Checksums**:
- Each sequence has unique checksum
- Enables fast duplicate detection
- Collision-free for practical purposes

**Deduplication Strategy**:
- Identical sequences → same UniParc ID
- Different sequences → different UniParc IDs
- Minor modifications → new UniParc IDs

## References

- **Citation**: UniProt Consortium (2025) UniProt: the Universal Protein Knowledgebase in 2025. Nucleic Acids Res. 53(D1):D523-D531.
- **Website**: https://www.uniprot.org/help/uniparc
- **FTP**: https://ftp.uniprot.org/pub/databases/uniprot/current_release/uniparc/
- **API**: https://rest.uniprot.org/uniparc/
- **License**: CC BY 4.0
