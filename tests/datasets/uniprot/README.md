# UniProt Test Dataset

## Overview

UniProt (Universal Protein Resource) is a comprehensive resource for protein sequence and functional information. It consists of two main sections:
- **Swiss-Prot**: Manually annotated and reviewed entries (high quality)
- **TrEMBL**: Automatically annotated entries from translations of EMBL-Bank coding sequences (high coverage)

**Data Source**: UniProt FTP (uniprot_sprot.xml.gz and uniprot_trembl.xml.gz)

## Integration Architecture

### Storage Model

UniProt entries are stored with primary accession IDs as keys:

```
UniProt Entry (e.g., P27348)
  ├── Attributes:
  │     ├── reviewed: true/false (Swiss-Prot vs TrEMBL)
  │     ├── accessions: ["Q9UKX2", "Q9UKX3", ...] (alternative accessions)
  │     ├── names: ["14-3-3 protein theta"] (recommended names)
  │     ├── alternative_names: [...] (alternative protein names)
  │     ├── submitted_names: [...] (submitted names, for TrEMBL)
  │     ├── seq: "MEKTELIQKA..." (amino acid sequence)
  │     └── mass: 27764 (molecular weight in Da)
  │
  ├── Cross-references:
  │     ├── taxonomy: 9913 (organism)
  │     ├── embl: AF285106, ... (nucleotide sequences)
  │     ├── refseq: NP_777118.1, ... (NCBI RefSeq)
  │     ├── pdb: 2BTP, ... (protein structures)
  │     ├── drugbank: DB12345, ... (drug targets)
  │     ├── ensembl: ENSG00000..., ... (genes, 6 divisions)
  │     ├── ensembltranscript: ENST00000..., ... (transcripts)
  │     ├── orphanet: 123456, ... (rare diseases)
  │     ├── reactome: R-HSA-177929, ... (pathways)
  │     ├── go: GO:0005515, ... (Gene Ontology)
  │     └── eco: ECO:0000269, ... (evidence codes)
  │
  └── Features (stored separately with xref to parent protein):
        ├── ufeature: P27348_f0, P27348_f1, ... (domains, sites, variants)
        └── Each feature has: type, description, location, evidences
```

**Key Features:**
- Primary accession as main identifier (e.g., P27348)
- Alternative accessions and protein names stored as attributes
- Alternative accessions and names searchable via text links
- Reviewed status distinguishes Swiss-Prot from TrEMBL
- Protein features stored as separate entries linked to parent protein
- Variants (dbSNP) extracted from feature descriptions
- Bidirectional cross-references automatically created by biobtree

### Data Sources Processed

The integration processes XML data from UniProt FTP:

1. **uniprot_sprot.xml.gz**: Swiss-Prot (reviewed, ~570K entries)
2. **uniprot_trembl.xml.gz**: TrEMBL (unreviewed, ~250M entries)

Both files downloaded directly from: ftp://ftp.uniprot.org/pub/databases/uniprot/current_release/knowledgebase/complete/

### Cross-Reference Databases

The following databases are integrated via UniProt cross-references:

**Nucleotide Sequences:**
- EMBL (nucleotide sequences)
- RefSeq (NCBI reference sequences)

**Protein Structures:**
- PDB (3D protein structures)

**Drug Information:**
- DrugBank (drug targets and pharmacology)

**Genes and Transcripts:**
- Ensembl (6 divisions: ensembl, ensembl_bacteria, ensembl_fungi, ensembl_metazoa, ensembl_plants, ensembl_protists)
- Ensembl Transcripts (transcript-level mappings)

**Diseases:**
- Orphanet (rare diseases)

**Pathways:**
- Reactome (biological pathways)

**Functional Annotations:**
- GO (Gene Ontology: molecular function, biological process, cellular component)
- ECO (Evidence and Conclusion Ontology)

**Taxonomy:**
- NCBI Taxonomy (organism classification)

### Protein Features

UniProt protein features are stored as separate entries with feature IDs (e.g., `P27348_f0`). Each feature includes:

- **Type**: Domain, region, site, binding site, variant, etc.
- **Description**: Detailed feature description
- **Location**: Begin and end positions in the sequence
- **Evidences**: Evidence codes with source database and IDs
- **Variants**: Mutations with dbSNP cross-references

Features are linked to their parent protein via `ufeature` cross-references.

## Use Cases

UniProt integration enables protein-centric biological queries:

1. **Protein Function Discovery**: Query protein → GO terms → biological processes/molecular functions
2. **Disease Association**: Protein → pathways (Reactome) → disease contexts
3. **Structure-Function Relationship**: Protein → PDB structures → domains/features → functional sites
4. **Drug Target Analysis**: Protein → DrugBank → therapeutic compounds
5. **Evolutionary Analysis**: Protein → taxonomy → organism lineage
6. **Multi-omics Integration**: Protein ↔ genes (Ensembl) ↔ pathways ↔ compounds
7. **Variant Analysis**: Protein features → variants → dbSNP IDs

## Test Cases

### Current Tests (17 total: 8 declarative + 9 custom)

#### 1. Basic Protein Lookup (Declarative)
**Test**: UniProt proteins can be queried directly
- Query protein ID (e.g., `P27348`)
- Verify entry exists in uniprot dataset

#### 2. Protein Attributes (Declarative)
**Test**: Proteins have complete attribute information
- Every protein should have attributes accessible via `Attributes.Uniprot`
- Check for names, sequence, mass, etc.

#### 3. Reviewed Protein (Custom)
**Test**: Swiss-Prot proteins are marked as reviewed
- Query a known Swiss-Prot entry
- Verify `reviewed` attribute is true

#### 4. Protein Name Present (Custom)
**Test**: Proteins have recommended names
- Query protein and check for name field
- Verify name is non-empty string

#### 5. Unreviewed Entry (Custom)
**Test**: Validates TrEMBL (unreviewed) entries
- Skips if no TrEMBL entries in test data
- Verifies unreviewed status

#### 6. Sequence Present (Custom)
**Test**: Verifies sequence and molecular weight
- Check sequence length and molecular weight fields
- Validates data is present

#### 7. Taxonomy Cross-Reference (Custom)
**Test**: Validates taxonomy ID cross-reference
- Every protein should link to taxonomy dataset
- Verifies taxonomy ID matches organism

#### 8. Ensembl Gene Cross-Reference (Custom)
**Test**: Validates Ensembl gene mappings
- Skips if no Ensembl xrefs (e.g., viral/bacterial proteins)
- Verifies gene cross-references when present

#### 9. Alternative Protein Names (Custom)
**Test**: Validates alternative protein names
- Checks for alternative_names attribute
- Verifies when entries have aliases

#### 10. Protein Features (Custom)
**Test**: Validates protein features
- Checks for feature cross-references (domains, sites, variants)
- Verifies feature data is linked to parent protein

#### 11. Multiple Cross-Reference Types (Custom)
**Test**: Validates multiple cross-reference types
- Checks proteins have varied database xrefs
- Verifies integration breadth

**Note**: Tests 7-11 are newly added and may need refinement based on test data characteristics (e.g., viral vs. eukaryotic proteins have different xref patterns).

## Performance Notes

- **Full UniProt Swiss-Prot**: ~570,000 reviewed proteins
- **Full UniProt TrEMBL**: ~250,000,000 unreviewed proteins
- **Test dataset**: 20 proteins (mixed Swiss-Prot and TrEMBL)
- **Processing time**:
  - Test data: ~1 second
  - Swiss-Prot only: ~10-15 minutes
  - Swiss-Prot + TrEMBL: several hours (with taxonomy filter recommended)
- **Cross-references**: Millions of mappings to other databases
- **Taxonomy filtering**: Use `--tax` flag to limit to specific organisms (dramatically reduces processing time)

## Known Limitations

1. **Gene names disabled**: Gene names from UniProt XML are currently disabled (see lines 427-436 in uniprot.go) because genes come from HGNC or Ensembl. UniProt name field already contains identifiers like "VAV_HUMAN".

2. **Comment sections skipped**: UniProt comment sections (function, subcellular location, disease associations, etc.) are currently skipped during XML parsing for performance reasons.

3. **Literature scope limited**: Reference/citation data is extracted but scope, title, and interaction details are not stored (see line 464 in uniprot.go).

4. **TrEMBL size**: Full TrEMBL integration is very large and slow. Recommend using Swiss-Prot only or applying taxonomy filters.

## Future Work

1. **Enhanced feature testing**: Add comprehensive tests for protein features (domains, sites, variants)
2. **Subcellular location**: Consider extracting comment sections for location data
3. **Function descriptions**: Consider storing protein function text from comments
4. **Isoforms**: UniProt contains isoform information that could be integrated
5. **Post-translational modifications**: PTM data available in features could be highlighted
6. **Disease associations**: Disease annotations from comments could be extracted

## Maintenance

**Release Frequency**: UniProt releases new versions every 8 weeks (6-7 releases per year). Swiss-Prot grows by ~10K entries/year, TrEMBL by millions.

## References

The UniProt Consortium. UniProt: the universal protein knowledgebase. Nucleic Acids Res. 2023 Jan 6;51(D1):D523-D531.

**License**: CC BY 4.0

---
Last updated: 2025-11-05
