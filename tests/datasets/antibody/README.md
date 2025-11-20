# Antibody Dataset

## Overview

Unified antibody database integrating therapeutic antibodies (TheraSAbDab), structural antibodies (SAbDab), germline immunoglobulin genes (IMGT/GENE-DB), and immunoglobulin sequences (IMGT/LIGM-DB). Provides comprehensive coverage of antibody therapeutics, 3D structures, germline gene alleles, and sequence diversity.

**Sources**:
- TheraSAbDab (Oxford Protein Informatics Group) - ~1,100 therapeutic antibodies
- SAbDab (Structural Antibody Database) - ~10,000 antibody structures
- IMGT/GENE-DB - ~3,500 germline immunoglobulin genes
- IMGT/LIGM-DB - ~250,000 immunoglobulin sequences

**Data Type**: Therapeutic antibodies, structural antibodies, germline genes, immunoglobulin sequences with cross-references to PDB, UniProt, and other databases

## Integration Architecture

### Storage Model
**Primary Entries**:
- Therapeutic: INN name (e.g., "Abciximab")
- Structural: Composite PDB ID (e.g., "1a2y_B_A" = pdb_Hchain_Lchain)
- Germline genes: Composite ID (e.g., "IGHA*01_V-REGION" = gene\*allele_region)
- Sequences: IMGT/LIGM-DB accession (e.g., "A00001")

**Searchable Text Links**:
- INN names (therapeutic antibodies)
- PDB IDs (structural antibodies)
- Gene names and alleles (IMGT/GENE-DB)
- Pure gene names (e.g., "IGHA" searches all alleles)
- Accession numbers (IMGT/LIGM-DB)

**Attributes Stored**:
- Source (therasabdab, sabdab, imgt_gene, imgt_ligm)
- Antibody type (therapeutic, structure, germline, sequence)
- Heavy/light chain sequences
- Format (Fab, scFv, whole mAb, etc.)
- Isotype (IgG1, IgG2, IgG4, etc.)
- Clinical information (phase, status, indications, targets)
- Structural information (PDB ID, resolution, method)
- Gene information (organism, functionality, allele)

**Cross-References**:
- Antibody → PDB (structural data)
- Antibody → UniProt (target proteins)
- PDB → Antibody (bidirectional)
- IMGT/GENE-DB → IMGT/LIGM-DB (accession links)

### Special Features

**Multi-source Unification**: Single "antibody" dataset integrating 4 heterogeneous sources with unified schema

**Composite IDs**: Prevents collisions between sources:
- SAbDab: pdb_Hchain_Lchain (e.g., "1a2y_B_A")
- IMGT/GENE-DB: gene*allele_region (e.g., "IGHA*01_V-REGION")

**Hierarchical Gene Search**: Three-level search for germline genes:
- Broad: "IGHA" → all alleles and regions
- Medium: "IGHA*01" → all regions of *01 allele
- Specific: "IGHA*01_V-REGION" → exact region

**.Z Decompression Support**: Handles Unix compress format for IMGT/LIGM-DB data

**UTF-8 BOM Handling**: Strips byte order mark from TheraSAbDab CSV headers

**Silent Duplicate Handling**: Tracks and summarizes duplicate entries instead of verbose warnings

## Use Cases

**1. Therapeutic Antibody Discovery**
```
Query: Find approved therapeutic antibody → INN name → Targets and indications
Use: Drug repurposing for similar disease conditions
```

**2. Antibody Structure Analysis**
```
Query: PDB structure → Antibody details → Heavy/light chain isotypes
Use: Structure-function relationship analysis for antibody engineering
```

**3. Germline Gene Analysis**
```
Query: Gene name (e.g., IGHA) → All alleles and regions → Sequence diversity
Use: Understanding antibody diversity and V(D)J recombination patterns
```

**4. Clinical Trial Tracking**
```
Query: Therapeutic antibody → Clinical phase and status → Disease indications
Use: Monitoring antibody therapeutic development pipeline
```

**5. Structure-Sequence Integration**
```
Query: PDB ID → Structural antibody → Heavy chain sequence → Germline gene match
Use: Tracing antibody structures back to germline origins
```

**6. Target Protein Mapping**
```
Query: Therapeutic antibody → Target proteins (UniProt) → Related antibodies
Use: Finding antibodies targeting the same protein for comparison
```

## Test Cases

**Current Tests** (16 total):
- 9 declarative tests (test_cases.json):
  - ID lookup (therapeutic, structural)
  - Attribute checks (source, type, sequences, targets)
  - PDB cross-reference access
  - Case-insensitive search
  - Invalid ID handling

- 7 custom tests (test_antibody.py):
  - TheraSAbDab therapeutic antibody validation
  - SAbDab structural antibody via PDB mapping
  - Bidirectional PDB cross-references
  - Multi-source unification verification
  - Sequence presence validation
  - Clinical indication data checks
  - Source attribution correctness

**Coverage**:
- ✅ All 4 sources (TheraSAbDab, SAbDab, IMGT/GENE-DB, IMGT/LIGM-DB)
- ✅ Therapeutic antibody attributes
- ✅ Structural antibody PDB mapping
- ✅ Heavy/light chain sequences
- ✅ Clinical data (targets, indications, phase, status)
- ✅ Cross-references (PDB, UniProt)
- ✅ Composite ID formats

**Recommended Additions**:
- IMGT/GENE-DB germline gene lookup tests
- IMGT/LIGM-DB sequence accession tests
- Gene-to-allele-to-region hierarchical search tests
- Format diversity tests (Fab, scFv, bispecific)
- Isotype distribution tests (IgG1, IgG2, IgG4)

## Performance

- **Test Build**: ~2.5s (200 entries: 50 per source)
- **Data Sources**:
  - TheraSAbDab: HTTPS download (CSV)
  - SAbDab: HTTPS download (TSV)
  - IMGT/GENE-DB: HTTPS download (FASTA)
  - IMGT/LIGM-DB: HTTPS download (.Z compressed FASTA)
- **Update Frequency**:
  - TheraSAbDab/SAbDab: Updated as new antibodies are approved/structures deposited
  - IMGT: Updated with new germline alleles and sequences
- **Total Entries**: ~265,000 (1,133 therapeutic + 10,000 structural + 3,500 germline + 250,000 sequences)
- **Special Notes**:
  - IMGT/LIGM-DB requires Unix uncompress tool for .Z decompression
  - TheraSAbDab CSV has UTF-8 BOM that must be stripped
  - Duplicate entries occur in IMGT/GENE-DB (same gene/allele, different regions)

## Known Limitations

**Reference Data Extraction**: No unified REST API across 4 sources - tests validate biobtree integration directly rather than comparing against external APIs

**IMGT/GENE-DB Duplicates**: Multiple regions per gene allele (V-REGION, CH1, CH2, etc.) are expected and handled with composite IDs

**TheraSAbDab Coverage**: Only antibodies with INN (International Nonproprietary Names) are included - research/preclinical antibodies without INN names are excluded

**Sequence Search**: Full-text search on sequences not implemented - only ID/name-based lookup supported

**PDB Dependencies**: SAbDab cross-references require PDB dataset to be built for bidirectional mapping

## Future Work

- Add BLAST-like sequence similarity search for antibody sequences
- Implement antigen/epitope information from SAbDab
- Add CDR (Complementarity-Determining Region) extraction and indexing
- Integrate AbRSA (Antibody Reactivity Sequence Annotation) data
- Add humanization scores and immunogenicity predictions
- Link to clinical trial data (ClinicalTrials.gov integration)
- Add patent information for therapeutic antibodies

## Maintenance

- **Release Schedule**:
  - TheraSAbDab/SAbDab: Continuous updates as new data added
  - IMGT: Quarterly releases
- **Data Format**: CSV, TSV, FASTA, compressed FASTA (.Z)
- **Test Data**: 200 entries (50 from each source)
- **License**:
  - TheraSAbDab/SAbDab: Academic use
  - IMGT: Non-commercial academic use
- **Special Notes**:
  - Requires `uncompress` command for IMGT/LIGM-DB .Z files
  - UTF-8 BOM handling required for TheraSAbDab CSV parsing

## References

- **TheraSAbDab**: Raybould MIJ et al., Nucleic Acids Res. 2020
- **SAbDab**: Dunbar J et al., Nucleic Acids Res. 2014
- **IMGT**: Lefranc MP et al., Nucleic Acids Res. 2015
- **Website**:
  - https://opig.stats.ox.ac.uk/webapps/sabdab-sabpred/therasabdab
  - https://opig.stats.ox.ac.uk/webapps/sabdab-sabpred/sabdab
  - https://www.imgt.org/
- **License**: Academic/non-commercial use
