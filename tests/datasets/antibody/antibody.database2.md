  ---
  Antibody Database Integration for Biobtree - Final Implementation Plan

  Document Version: 2.0 (Updated after comprehensive IPD/IMGT investigation)
  Date: 2025-11-19
  Status: Ready for Implementation
  Integration Approach: Unified "Antibody" datasource with 4 complementary databases

  ---
  Executive Summary

  This document provides the complete implementation plan for integrating antibody databases into biobtree. After comprehensive investigation of EBI IPD and IMGT resources, we recommend integrating 4 
  complementary databases under a unified "antibody" datasource:

  1. TheraSAbDab - Therapeutic antibodies with clinical metadata
  2. IMGT/GENE-DB - Germline gene references (V, D, J, C segments)
  3. IMGT/LIGM-DB - Comprehensive antibody sequences (251K entries)
  4. SAbDab - Antibody 3D structures from PDB

  Total Implementation Time: 7-10 weeks
  Total Storage: ~2.3 GB (indexed)
  License: ✅ All free for academic and commercial use
  Integration Strategy: Unified "antibody" datasource with strong cross-references

  ---
  Table of Contents

  1. #1-database-overview
  2. #2-integration-architecture
  3. #3-database-specifications
  4. #4-implementation-roadmap
  5. #5-technical-implementation
  6. #6-cross-reference-strategy
  7. #7-query-examples
  8. #8-testing--validation
  9. #9-maintenance--updates
  10. #10-appendices

  ---
  1. Database Overview

  1.1 Why These Four Databases?

  Each database provides unique, non-redundant information that complements the others:

  | Database     | Primary Data      | Unique Value                                   | Coverage          |
  |--------------|-------------------|------------------------------------------------|-------------------|
  | TheraSAbDab  | Clinical metadata | Approval status, indications, clinical trials  | ~700 therapeutics |
  | IMGT/GENE-DB | Germline genes    | V(D)J gene references for antibody engineering | 17,290 alleles    |
  | IMGT/LIGM-DB | Sequences         | Complete nucleotide/protein sequences          | 251,608 sequences |
  | SAbDab       | 3D structures     | PDB structures, CDR annotations                | 6,000+ structures |

  1.2 Data Complementarity

  Example: Pembrolizumab (anti-PD-1 therapeutic antibody)

  TheraSAbDab provides:
  ├── INN name: "pembrolizumab"
  ├── Clinical stage: "Approved (FDA 2014)"
  ├── Indication: "Melanoma, NSCLC, etc."
  ├── Target: "PD-1 (PDCD1)"
  └── Format: "IgG4-kappa"

  IMGT/GENE-DB provides:
  ├── Heavy chain V gene: "IGHV3-23*01"
  ├── Heavy chain D gene: "IGHD3-10*01"
  ├── Heavy chain J gene: "IGHJ4*01"
  ├── Light chain V gene: "IGKV1-39*01"
  └── Light chain J gene: "IGKJ1*01"

  IMGT/LIGM-DB provides:
  ├── Full variable region sequences (nucleotide + protein)
  ├── CDR sequences (IMGT numbered)
  ├── Framework sequences
  └── GenBank/EMBL accessions

  SAbDab provides:
  ├── PDB structures: "5DK3", "5JXE", "5B8C"
  ├── Resolution: 2.8 Å, 2.5 Å, etc.
  ├── CDR conformations
  └── Antigen-binding interface

  Together, these four databases enable:
  - Drug discovery (target → therapeutic antibodies)
  - Antibody engineering (germline → humanization)
  - Structure-guided design (sequence → structure)
  - Clinical development tracking (therapeutic → trials)

  ---
  2. Integration Architecture

  2.1 Datasource Grouping Strategy

  We will create a unified "antibody" datasource that logically groups the four databases while maintaining independent data pipelines.

  ┌─────────────────────────────────────────────────────────┐
  │                  ANTIBODY DATASOURCE                     │
  │                  (Virtual Grouping)                      │
  ├─────────────────────────────────────────────────────────┤
  │                                                          │
  │  ┌──────────────┐  ┌──────────────┐                    │
  │  │ TheraSAbDab  │  │ IMGT/GENE-DB │                    │
  │  │ (Clinical)   │  │ (Germline)   │                    │
  │  │ ~700 entries │  │ 17K alleles  │                    │
  │  └──────────────┘  └──────────────┘                    │
  │                                                          │
  │  ┌──────────────┐  ┌──────────────┐                    │
  │  │ IMGT/LIGM-DB │  │   SAbDab     │                    │
  │  │ (Sequences)  │  │ (Structures) │                    │
  │  │ 251K seqs    │  │ 6K structures│                    │
  │  └──────────────┘  └──────────────┘                    │
  │                                                          │
  └─────────────────────────────────────────────────────────┘
                          ↓
           Strong Cross-References Between All Four
                          ↓
           ┌─────────────────────────────────┐
           │   Existing Biobtree Datasets    │
           ├─────────────────────────────────┤
           │ UniProt → Target proteins       │
           │ PDB → Structure details         │
           │ Clinical Trials → Trial data    │
           │ ChEMBL → Activity assays        │
           │ HGNC → Gene symbols             │
           │ Ensembl → Genomics              │
           └─────────────────────────────────┘

  2.2 Implementation Approach

  Hybrid Strategy: Individual parsers + unified query interface

  1. Data Processing: Each database has independent parser and dataset ID
  2. Cross-References: Strong bidirectional links between all four
  3. Query Interface: Users can query "antibody" (all four) or specific database
  4. Primary Key: INN name (therapeutic name) where applicable

  2.3 Dataset Naming Convention

  {
    "antibody": "Virtual grouping (not a real dataset)",
    "therasabdab": "Therapeutic antibodies (dataset ID: 46)",
    "imgt_gene": "IMGT germline genes (dataset ID: 47)",
    "imgt_ligm": "IMGT sequences (dataset ID: 48)",
    "sabdab": "Antibody structures (dataset ID: 49)"
  }

  ---
  3. Database Specifications

  3.1 TheraSAbDab - Therapeutic Antibodies

  Overview

  Curated database of WHO-recognized therapeutic antibodies including FDA/EMA approved drugs and clinical candidates.

  Technical Specifications

  | Attribute        | Value                                                          |
  |------------------|----------------------------------------------------------------|
  | Dataset ID       | 46                                                             |
  | Dataset Name     | therasabdab                                                    |
  | Source           | Oxford OPIG (University of Oxford)                             |
  | URL              | http://opig.stats.ox.ac.uk/webapps/therasabdab                 |
  | Download         | http://opig.stats.ox.ac.uk/webapps/therasabdab/therasabdab.csv |
  | Format           | CSV                                                            |
  | Size             | 50-100 MB (raw), ~200 MB (indexed)                             |
  | Update Frequency | Weekly                                                         |
  | License          | ✅ Free for academic and commercial use                         |
  | Primary Key      | INN name (International Nonproprietary Name)                   |

  Data Schema

  message TheraSAbDab {
    string inn_name = 1;              // e.g., "pembrolizumab"
    string format = 2;                // IgG, Fab, scFv, nanobody
    string isotype = 3;               // IgG1, IgG2, IgG4, etc.
    string light_chain = 4;           // kappa, lambda
    string clinical_stage = 5;        // Approved, Phase III, etc.
    string status = 6;                // Active, Discontinued, Withdrawn
    repeated string heavy_chain_seq = 7;
    repeated string light_chain_seq = 8;
    repeated string targets = 9;      // Target protein names
    repeated string indications = 10; // Disease indications
    repeated string pdb_ids = 11;     // Structure IDs
  }

  Cross-References

  TheraSAbDab → UniProt (target proteins)
  TheraSAbDab → HGNC (target genes)
  TheraSAbDab → PDB (structures)
  TheraSAbDab → Clinical Trials (NCT IDs)
  TheraSAbDab → ChEMBL (activity data)
  TheraSAbDab → IMGT/GENE-DB (germline genes via V/D/J assignment)
  TheraSAbDab → IMGT/LIGM-DB (full sequences)
  TheraSAbDab → SAbDab (structure annotations)
  TheraSAbDab → EFO/MONDO (disease indications)

  Implementation Priority

  ⭐⭐⭐⭐⭐ HIGHEST - Start here, easiest integration, highest immediate value

  ---
  3.2 IMGT/GENE-DB - Germline Gene Database

  Overview

  Reference germline gene sequences for immunoglobulins (IG) and T-cell receptors (TR). Essential for antibody engineering and V(D)J gene assignment.

  Technical Specifications

  | Attribute        | Value                                                  |
  |------------------|--------------------------------------------------------|
  | Dataset ID       | 47                                                     |
  | Dataset Name     | imgt_gene                                              |
  | Source           | IMGT (International ImMunoGeneTics Information System) |
  | URL              | http://www.imgt.org                                    |
  | Download         | http://www.imgt.org/download/GENE-DB/                  |
  | Format           | FASTA (multiple variants)                              |
  | Size             | 10-20 MB (raw), ~100 MB (indexed)                      |
  | Update Frequency | Weekly                                                 |
  | License          | ✅ Free for academic and commercial use (CC BY-ND)      |
  | Primary Key      | IMGT gene name (e.g., "IGHV3-23*01")                   |

  Data Coverage

  - 12,185 genes from 41 species
  - 17,290 alleles (including variants)
  - Gene segments: V (Variable), D (Diversity), J (Joining), C (Constant)
  - Species: Human, mouse, rat, rabbit, and 37 others
  - Functional classification: F (functional), ORF (open reading frame), P (pseudogene)

  FASTA Header Format

  >IMGT_accession|gene_name|species|functionality|region|start|end|frame|...
  Example:
  >M99641|IGHV3-23*01|Homo sapiens|F|V-REGION|1|296|+3|...

  15 fields separated by '|':
  1. IMGT accession number
  2. Gene and allele name
  3. Species
  4. Functionality (F/ORF/P)
  5. Region type
  6. Start position
  7. End position
  8. Frame
  9-15. Additional metadata

  File Variants Available

  IMGTGENE-DB-ReferenceSequences.fasta-nt-WithGaps-F+ORF+inframeP
    ↑ Nucleotide with IMGT gaps for alignment, functional + ORF + in-frame pseudo

  IMGTGENE-DB-ReferenceSequences.fasta-nt-WithoutGaps-F+ORF+inframeP
    ↑ Nucleotide without gaps, functional + ORF + in-frame pseudo

  IMGTGENE-DB-ReferenceSequences.fasta-nt-WithoutGaps-F+ORF+allP
    ↑ Nucleotide without gaps, functional + ORF + all pseudogenes

  IMGTGENE-DB-ReferenceSequences.fasta-aa-*
    ↑ Protein (amino acid) versions of above

  Recommendation: Use fasta-nt-WithoutGaps-F+ORF+inframeP for biobtree (functional + ORF only)

  Data Schema

  message IMGTGene {
    string imgt_accession = 1;        // e.g., "M99641"
    string gene_name = 2;             // e.g., "IGHV3-23*01"
    string allele = 3;                // e.g., "*01"
    string species = 4;               // e.g., "Homo sapiens"
    string functionality = 5;         // F, ORF, P
    string gene_type = 6;             // V, D, J, C
    string chain_type = 7;            // IGH, IGK, IGL, TRA, TRB, etc.
    string sequence_nt = 8;           // Nucleotide sequence
    string sequence_aa = 9;           // Protein sequence (if applicable)
    repeated string references = 10;  // Literature references
  }

  Cross-References

  IMGT/GENE-DB → TheraSAbDab (therapeutics using this V/D/J gene)
  IMGT/GENE-DB → IMGT/LIGM-DB (sequences containing this gene)
  IMGT/GENE-DB → UniProt (germline protein sequences)
  IMGT/GENE-DB → PubMed (literature)

  Text Search Indexing

  Index gene names: "IGHV3-23", "IGKV1-39", etc.
  Index species: "Homo sapiens", "Mus musculus"
  Index chain types: "IGH", "IGK", "IGL"
  Index functionality: "functional", "ORF", "pseudogene"

  Implementation Priority

  ⭐⭐⭐⭐⭐ CRITICAL - Essential for antibody engineering, integrate in Phase 1

  ---
  3.3 IMGT/LIGM-DB - Sequence Database

  Overview

  Comprehensive nucleotide sequence database of immunoglobulins (IG) and T-cell receptors (TR) from all species. Includes both germline and rearranged sequences.

  Technical Specifications

  | Attribute        | Value                                                  |
  |------------------|--------------------------------------------------------|
  | Dataset ID       | 48                                                     |
  | Dataset Name     | imgt_ligm                                              |
  | Source           | IMGT (International ImMunoGeneTics Information System) |
  | FTP              | ftp://ftp.ebi.ac.uk/pub/databases/imgt/LIGM-DB/        |
  | Format           | EMBL flat file (.dat.Z), FASTA (.fasta.Z), XML         |
  | Size             | 320 MB (EMBL), 90 MB (FASTA), 7 MB (XML)               |
  | Indexed Size     | ~1.5 GB                                                |
  | Update Frequency | Weekly                                                 |
  | License          | ✅ Free for academic and commercial use (CC BY-ND)      |
  | Primary Key      | IMGT accession number                                  |
  | Current Release  | 202324-0 (as of June 2023, check for latest)           |

  Data Coverage

  - 251,608 sequences from 368 species
  - Immunoglobulin (IG) sequences: Heavy chain (IGH), Light chain kappa (IGK), Light chain lambda (IGL)
  - T-cell receptor (TR) sequences: Alpha (TRA), Beta (TRB), Gamma (TRG), Delta (TRD)
  - Both germline and rearranged (V-D-J) sequences
  - Links to GenBank/EMBL accessions
  - Species range: Vertebrates (mammals, birds, fish, amphibians, reptiles)

  File Formats Available

  imgt.dat.Z         (320 MB) - EMBL flat file format (most comprehensive)
  imgt.fasta.Z       (90 MB)  - FASTA sequences (sequences only)
  ligmdb.xml         (7 MB)   - XML format (structured data)
  imgtblast.n*       (~100MB) - BLAST databases (for sequence search)
  accessionNumber.lst (2 MB)  - List of all accession numbers
  imgtrefseq.fasta   (4.3 MB) - IMGT reference sequences (from GENE-DB)

  Recommendation: Use imgt.dat.Z (EMBL format) for full annotations

  EMBL Flat File Structure

  ID   IMGT000001; SV 1; linear; genomic DNA; STD; VRT; 500 BP.
  XX
  AC   IMGT000001;
  XX
  DT   01-JAN-2000 (Rel. 1, Created)
  DT   01-JAN-2023 (Rel. 202324, Last updated)
  XX
  DE   Homo sapiens IGHV3-23*01
  XX
  KW   V-REGION; heavy chain; immunoglobulin.
  XX
  OS   Homo sapiens (human)
  OC   Eukaryota; Metazoa; Chordata; Craniata; Vertebrata; Euteleostomi;
  OC   Mammalia; Eutheria; Euarchontoglires; Primates; Haplorrhini;
  OC   Catarrhini; Hominidae; Homo.
  XX
  RN   [1]
  RP   1-500
  RA   Author A., Author B.;
  RT   "Title of paper";
  RL   Journal 123:456-789(2000).
  XX
  DR   UniProt; P01234; IGHV3_HUMAN.
  XX
  FH   Key             Location/Qualifiers
  FT   source          1..500
  FT                   /organism="Homo sapiens"
  FT   V_region        1..296
  FT                   /gene="IGHV3-23"
  FT                   /allele="IGHV3-23*01"
  FT   CDS             1..296
  FT                   /codon_start=1
  FT                   /translation="QVQLVQSGAEVKKPGA..."
  XX
  SQ   Sequence 500 BP; 120 A; 130 C; 125 G; 125 T; 0 other;
       caggtgcagc tggtgcagtc tggggctgag gtgaagaagc ctggggcctc agtgaaggtc
       ...
  //

  Data Schema

  message IMGTLIGM {
    string imgt_accession = 1;        // IMGT accession
    string genbank_accession = 2;     // GenBank/EMBL accession
    string gene_name = 3;             // e.g., "IGHV3-23*01"
    string species = 4;               // Species name
    string chain_type = 5;            // IGH, IGK, IGL, TRA, etc.
    string sequence_type = 6;         // germline, rearranged, cDNA
    string sequence_nt = 7;           // Nucleotide sequence
    string sequence_aa = 8;           // Protein translation
    repeated string v_genes = 9;      // V gene assignments
    repeated string d_genes = 10;     // D gene assignments
    repeated string j_genes = 11;     // J gene assignments
    repeated string cdr1 = 12;        // CDR1 sequence
    repeated string cdr2 = 13;        // CDR2 sequence
    repeated string cdr3 = 14;        // CDR3 sequence
    repeated string pubmed_ids = 15;  // Literature references
    repeated string uniprot_ids = 16; // UniProt cross-refs
  }

  Cross-References

  IMGT/LIGM-DB → IMGT/GENE-DB (germline gene references)
  IMGT/LIGM-DB → TheraSAbDab (therapeutic antibody sequences)
  IMGT/LIGM-DB → UniProt (protein sequences)
  IMGT/LIGM-DB → GenBank/EMBL (nucleotide databases)
  IMGT/LIGM-DB → PubMed (literature)

  Implementation Priority

  ⭐⭐⭐⭐ HIGH - Comprehensive sequences, integrate in Phase 2

  ---
  3.4 SAbDab - Structural Antibody Database

  Overview

  Comprehensive database of all antibody and nanobody structures deposited in the Protein Data Bank (PDB), with standardized annotations and CDR definitions.

  Technical Specifications

  | Attribute        | Value                                                                  |
  |------------------|------------------------------------------------------------------------|
  | Dataset ID       | 49                                                                     |
  | Dataset Name     | sabdab                                                                 |
  | Source           | Oxford OPIG (University of Oxford)                                     |
  | URL              | https://opig.stats.ox.ac.uk/webapps/sabdab-sabpred/sabdab              |
  | Download         | https://opig.stats.ox.ac.uk/webapps/sabdab-sabpred/sabdab/summary/all/ |
  | Format           | CSV (metadata) + PDB files (structures)                                |
  | Size             | 100 MB (CSV), 2-5 GB (PDB files)                                       |
  | Indexed Size     | ~500 MB (metadata only), ~10 GB (with structures)                      |
  | Update Frequency | Weekly (synced with PDB releases)                                      |
  | License          | ✅ Free for academic and commercial use                                 |
  | Primary Key      | PDB ID + chain IDs                                                     |

  Data Coverage

  - 6,000+ antibody structures (growing weekly)
  - All antibody/nanobody/Fab structures from PDB
  - Multiple antibody types: IgG, Fab, scFv, VHH (nanobodies), VH/VL domains
  - Species: Human, mouse, camelid, shark, synthetic
  - Experimental methods: X-ray crystallography, cryo-EM, NMR
  - Antigen-bound and unbound structures

  CSV Metadata Fields

  pdb,Hchain,Lchain,resolution,method,date,species,scfv,antigen_type,
  antigen_name,antigen_chain,heavy_subclass,light_subclass

  Example row:
  5DK3,H,L,2.80,X-RAY DIFFRACTION,2015-09-01,human,FALSE,protein,
  PD-1,A,IGHV1,IGKV1

  Data Schema

  message SAbDab {
    string pdb_id = 1;                // e.g., "5DK3"
    repeated string heavy_chains = 2; // e.g., ["H"]
    repeated string light_chains = 3; // e.g., ["L"]
    float resolution = 4;             // e.g., 2.80 (Angstroms)
    string method = 5;                // X-RAY, CRYO-EM, NMR
    string deposition_date = 6;       // YYYY-MM-DD
    string species = 7;               // human, mouse, camelid
    bool is_scfv = 8;                 // Single-chain Fv
    string antigen_type = 9;          // protein, peptide, hapten, carbohydrate
    string antigen_name = 10;         // e.g., "PD-1"
    repeated string antigen_chains = 11;
    string heavy_subclass = 12;       // IGHV1, IGHV3, etc.
    string light_subclass = 13;       // IGKV1, IGLV3, etc.

    // CDR sequences (Kabat, Chothia, IMGT numbering)
    string cdr_h1_kabat = 14;
    string cdr_h2_kabat = 15;
    string cdr_h3_kabat = 16;
    string cdr_l1_kabat = 17;
    string cdr_l2_kabat = 18;
    string cdr_l3_kabat = 19;

    // Optional: Store full structure as blob or external reference
    bytes structure_pdb = 20;         // Optional PDB file content
  }

  Numbering Schemes

  SAbDab provides CDR definitions in multiple numbering schemes:
  - Kabat - Traditional antibody numbering
  - Chothia - Structural definition of CDRs
  - IMGT - Standardized immunogenetics numbering
  - AHo - Antibody-specific scheme

  Recommendation: Store all numbering schemes, default to IMGT for consistency with IMGT databases

  Cross-References

  SAbDab → PDB (1:1 mapping - PDB already in biobtree!)
  SAbDab → TheraSAbDab (therapeutic antibody structures)
  SAbDab → IMGT/GENE-DB (via V gene assignment)
  SAbDab → IMGT/3Dstructure-DB (overlapping structures)
  SAbDab → UniProt (via PDB → UniProt mapping)
  SAbDab → HGNC (via antigen gene)

  Implementation Strategy

  Phase 1: Metadata Only (Recommended)
  - Download CSV with all annotations
  - Store structural metadata (resolution, method, CDRs, etc.)
  - Link to existing PDB cross-references in biobtree
  - Storage: ~500 MB
  - Effort: 1-2 weeks

  Phase 2: Full Structures (Optional, later)
  - Download PDB coordinate files
  - Store as BLOBs or external file references
  - Enable structure-based queries
  - Storage: ~10 GB additional
  - Effort: 2-3 weeks

  Recommendation: Start with Phase 1 (metadata only)

  Implementation Priority

  ⭐⭐⭐⭐ HIGH - Structure annotations, integrate in Phase 1 (metadata only)

  ---
  4. Implementation Roadmap

  4.1 Overview Timeline

  Phase 1: Core Foundation         (Weeks 1-3)  ⭐⭐⭐⭐⭐
  ├── TheraSAbDab integration
  ├── IMGT/GENE-DB integration
  └── SAbDab metadata integration

  Phase 2: Comprehensive Sequences (Weeks 4-7)  ⭐⭐⭐⭐
  └── IMGT/LIGM-DB integration

  Phase 3: Testing & Documentation (Week 8)     ⭐⭐⭐⭐⭐
  ├── Integration testing
  ├── Cross-reference validation
  └── Documentation & examples

  Phase 4: Optional Enhancements   (Weeks 9-10) ⭐⭐⭐
  ├── SAbDab full structures (optional)
  ├── OGRDB novel alleles (optional)
  └── Additional features

  Total Core Implementation: 7-8 weeks
  With Optional Enhancements: 9-10 weeks

  ---
  4.2 Phase 1: Core Foundation (Weeks 1-3)

  Week 1: TheraSAbDab Integration

  Goal: Integrate therapeutic antibody database

  Tasks:
  1. Add configuration to conf/source.dataset.json and conf/default.dataset.json
  2. Define protobuf schema in src/pbuf/attr.proto
  3. Compile protobuf: make proto
  4. Create parser src/update/therasabdab.go:
    - Download CSV from Oxford OPIG
    - Parse therapeutic antibody data
    - Store entries with addProp3(inn_name, dataset_id, marshaled_attrs)
    - Create text search: addXref(inn_name, text_link_id, inn_name, "therasabdab", true)
    - Create xrefs to UniProt targets: addXref(inn_name, dataset_id, uniprot_id, "uniprot", false)
  5. Update merge logic in src/generate/mergeg.go
  6. Add filter support in src/service/service.go and src/service/mapfilter.go
  7. Create test suite in tests/datasets/therasabdab/
  8. Build and test: ./biobtree -d "therasabdab,uniprot,hgnc" build

  Deliverables:
  - ✅ 700 therapeutic antibodies indexed
  - ✅ Cross-references to UniProt, HGNC working
  - ✅ Text search functional
  - ✅ Tests passing

  Estimated Effort: 5-7 days

  ---
  Week 2: IMGT/GENE-DB Integration

  Goal: Integrate germline gene reference database

  Tasks:
  1. Add configuration to conf/source.dataset.json and conf/default.dataset.json
  2. Define protobuf schema in src/pbuf/attr.proto
  3. Compile protobuf: make proto
  4. Create parser src/update/imgt_gene.go:
    - Download FASTA from http://www.imgt.org/download/GENE-DB/
    - Parse 15-field FASTA headers
    - Extract gene name, allele, species, functionality, sequence
    - Store entries with addProp3(gene_name, dataset_id, marshaled_attrs)
    - Create text search for gene names: addXref(gene_name, text_link_id, gene_name, "imgt_gene", true)
    - Create bidirectional xrefs with IMGT/LIGM-DB (prepare for Phase 2)
  5. Update merge logic in src/generate/mergeg.go
  6. Add filter support (filter by species, functionality, chain type)
  7. Create test suite in tests/datasets/imgt_gene/
  8. Build and test: ./biobtree -d "imgt_gene" build

  Deliverables:
  - ✅ 17,290 germline alleles indexed
  - ✅ Searchable by gene name, species, chain type
  - ✅ Tests passing

  Estimated Effort: 7-10 days

  ---
  Week 3: SAbDab Metadata Integration

  Goal: Integrate antibody structure annotations

  Tasks:
  1. Add configuration to conf/source.dataset.json and conf/default.dataset.json
  2. Define protobuf schema in src/pbuf/attr.proto
  3. Compile protobuf: make proto
  4. Create parser src/update/sabdab.go:
    - Download CSV from Oxford OPIG
    - Parse structure metadata (PDB ID, chains, resolution, CDRs, etc.)
    - Store entries with addProp3(pdb_id, dataset_id, marshaled_attrs)
    - Create xrefs to existing PDB entries: addXref(pdb_id, dataset_id, pdb_id, "pdb", false)
    - Create xrefs to TheraSAbDab via antibody name matching
  5. Update merge logic in src/generate/mergeg.go
  6. Add filter support (filter by resolution, species, antigen type)
  7. Create test suite in tests/datasets/sabdab/
  8. Build and test: ./biobtree -d "sabdab,pdb" build

  Deliverables:
  - ✅ 6,000+ structure annotations indexed
  - ✅ Linked to PDB cross-references
  - ✅ CDR annotations available
  - ✅ Tests passing

  Estimated Effort: 5-7 days

  Phase 1 Milestone: Core antibody integration complete (therapeutics + germline + structures)

  ---
  4.3 Phase 2: Comprehensive Sequences (Weeks 4-7)

  Weeks 4-7: IMGT/LIGM-DB Integration

  Goal: Integrate comprehensive antibody sequence database

  Tasks:
  1. Add configuration to conf/source.dataset.json and conf/default.dataset.json
  2. Define protobuf schema in src/pbuf/attr.proto
  3. Compile protobuf: make proto
  4. Create parser src/update/imgt_ligm.go:
    - Download EMBL flat file from ftp://ftp.ebi.ac.uk/pub/databases/imgt/LIGM-DB/imgt.dat.Z
    - Uncompress gzip file
    - Parse EMBL format (similar to UniProt parsing approach)
    - Extract: accession, gene assignment, sequence, species, cross-refs
    - Store entries with addProp3(imgt_accession, dataset_id, marshaled_attrs)
    - Create text search for accessions and gene names
    - Create xrefs to IMGT/GENE-DB: addXref(imgt_accession, dataset_id, gene_name, "imgt_gene", false)
    - Create xrefs to GenBank/UniProt where available
    - Handle 251K sequences efficiently (chunk processing)
  5. Update merge logic in src/generate/mergeg.go
  6. Add filter support (filter by species, gene type, sequence type)
  7. Create test suite in tests/datasets/imgt_ligm/
  8. Build and test: ./biobtree -d "imgt_ligm,imgt_gene" build
  9. Optimize storage and indexing for large dataset

  Deliverables:
  - ✅ 251,608 antibody sequences indexed
  - ✅ Linked to IMGT/GENE-DB germline references
  - ✅ Cross-references to UniProt/GenBank
  - ✅ Text search functional
  - ✅ Tests passing

  Estimated Effort: 2-3 weeks (larger dataset, EMBL parsing)

  Phase 2 Milestone: Complete antibody sequence coverage

  ---
  4.4 Phase 3: Testing & Documentation (Week 8)

  Week 8: Integration Testing & Validation

  Goal: Comprehensive testing and documentation

  Tasks:

  Testing:
  1. Unit tests for each parser (90%+ coverage)
  2. Integration tests for cross-references:
    - TheraSAbDab → UniProt → HGNC
    - TheraSAbDab → IMGT/GENE-DB (germline genes)
    - TheraSAbDab → IMGT/LIGM-DB (sequences)
    - TheraSAbDab → SAbDab (structures via PDB)
    - SAbDab → PDB → UniProt
  3. End-to-end query tests:
  # Known therapeutics
  ./biobtree query "pembrolizumab"
  ./biobtree query "trastuzumab"
  ./biobtree query "cetuximab"

  # Cross-database queries
  ./biobtree query "EGFR >> uniprot >> therasabdab"
  ./biobtree query "pembrolizumab >> imgt_gene"
  ./biobtree query "5DK3 >> sabdab"

  # Filter queries
  ./biobtree query "therasabdab[therasabdab.clinical_stage=='Approved']"
  ./biobtree query "imgt_gene[imgt_gene.species=='Homo sapiens']"
  4. Performance testing (query response time < 100ms)
  5. Storage validation (actual vs estimated sizes)

  Documentation:
  1. Update main README.md with antibody examples
  2. Create tests/datasets/antibody_integration.md with:
    - Overview of all four databases
    - Query examples
    - Use cases
  3. Create individual README.md for each dataset:
    - tests/datasets/therasabdab/README.md
    - tests/datasets/imgt_gene/README.md
    - tests/datasets/imgt_ligm/README.md
    - tests/datasets/sabdab/README.md
  4. Update web UI documentation
  5. Create API documentation for antibody queries

  Deliverables:
  - ✅ All tests passing
  - ✅ Cross-references validated
  - ✅ Documentation complete
  - ✅ Examples provided

  Estimated Effort: 1 week

  Phase 3 Milestone: Production-ready antibody integration

  ---
  4.5 Phase 4: Optional Enhancements (Weeks 9-10)

  Optional Enhancement 1: SAbDab Full Structures

  If needed: Download and store full PDB coordinate files

  Tasks:
  1. Download PDB files from SAbDab
  2. Store as BLOBs or external file references
  3. Enable structure download via API
  4. Add structure-based query capabilities

  Effort: 2 weeks
  Storage: +10 GB

  ---
  Optional Enhancement 2: OGRDB Novel Alleles

  If needed: Integrate community-curated novel germline alleles

  Source: https://ogrdb.airr-community.org/
  FTP: ftp://ftp.ncbi.nih.gov/blast/executables/igblast/release/database/airr/

  Tasks:
  1. Download OGRDB FASTA files
  2. Create parser similar to IMGT/GENE-DB
  3. Cross-reference with IMGT/GENE-DB
  4. Flag novel vs established alleles

  Effort: 1 week
  Storage: +50 MB

  ---
  Optional Enhancement 3: Additional Features

  - VDJbase integration (population genetics)
  - IMGT/3Dstructure-DB (IMGT numbering for structures)
  - Advanced filtering (CDR sequence search, homology search)
  - Visualization tools (structure viewer, gene usage plots)

  ---
  5. Technical Implementation

  5.1 Configuration Files

  conf/source.dataset.json

  {
    "therasabdab": {
      "id": "46",
      "name": "TheraSAbDab",
      "path": "http://opig.stats.ox.ac.uk/webapps/therasabdab/therasabdab.csv",
      "aliases": "TheraSAbDab,therapeutic antibody,monoclonal antibody,mAb",
      "url": "http://opig.stats.ox.ac.uk/webapps/therasabdab/therapeutics/£{id}",
      "useLocalFile": "no",
      "hasFilter": "yes",
      "test_entries_count": "50"
    },

    "imgt_gene": {
      "id": "47",
      "name": "IMGT_GENE",
      "path": "http://www.imgt.org/download/GENE-DB/IMGTGENE-DB-ReferenceSequences.fasta-nt-WithoutGaps-F+ORF+inframeP",
      "aliases": "IMGT,GENE-DB,germline genes,V genes,D genes,J genes",
      "url": "http://www.imgt.org/genedb/GENElect?query=2+£{id}",
      "useLocalFile": "no",
      "hasFilter": "yes",
      "test_entries_count": "100"
    },

    "imgt_ligm": {
      "id": "48",
      "name": "IMGT_LIGM",
      "path": "ftp://ftp.ebi.ac.uk/pub/databases/imgt/LIGM-DB/imgt.dat.Z",
      "aliases": "IMGT,LIGM-DB,antibody sequences,immunoglobulin sequences",
      "url": "http://www.imgt.org/ligmdb/view?id=£{id}",
      "useLocalFile": "no",
      "hasFilter": "yes",
      "test_entries_count": "200"
    },

    "sabdab": {
      "id": "49",
      "name": "SAbDab",
      "path": "https://opig.stats.ox.ac.uk/webapps/sabdab-sabpred/sabdab/summary/all/",
      "aliases": "SAbDab,structural antibody,antibody structure,PDB antibody",
      "url": "https://opig.stats.ox.ac.uk/webapps/sabdab-sabpred/sabdab/summary/£{id}",
      "useLocalFile": "no",
      "hasFilter": "yes",
      "test_entries_count": "100"
    }
  }

  conf/default.dataset.json

  Add full metadata for each dataset including attributes and filter configurations.

  ---
  5.2 Protocol Buffer Definitions

  src/pbuf/attr.proto

  syntax = "proto3";

  package pbuf;

  // TheraSAbDab - Therapeutic Antibodies
  message TheraSAbDab {
    string inn_name = 1;
    string format = 2;
    string isotype = 3;
    string light_chain = 4;
    string clinical_stage = 5;
    string status = 6;
    repeated string heavy_chain_seq = 7;
    repeated string light_chain_seq = 8;
    repeated string targets = 9;
    repeated string indications = 10;
    repeated string pdb_ids = 11;
  }

  // IMGT GENE-DB - Germline Genes
  message IMGTGene {
    string imgt_accession = 1;
    string gene_name = 2;
    string allele = 3;
    string species = 4;
    string functionality = 5;
    string gene_type = 6;
    string chain_type = 7;
    string sequence_nt = 8;
    string sequence_aa = 9;
    repeated string references = 10;
  }

  // IMGT LIGM-DB - Sequences
  message IMGTLIGM {
    string imgt_accession = 1;
    string genbank_accession = 2;
    string gene_name = 3;
    string species = 4;
    string chain_type = 5;
    string sequence_type = 6;
    string sequence_nt = 7;
    string sequence_aa = 8;
    repeated string v_genes = 9;
    repeated string d_genes = 10;
    repeated string j_genes = 11;
    repeated string cdr1 = 12;
    repeated string cdr2 = 13;
    repeated string cdr3 = 14;
    repeated string pubmed_ids = 15;
    repeated string uniprot_ids = 16;
  }

  // SAbDab - Structural Antibodies
  message SAbDab {
    string pdb_id = 1;
    repeated string heavy_chains = 2;
    repeated string light_chains = 3;
    float resolution = 4;
    string method = 5;
    string deposition_date = 6;
    string species = 7;
    bool is_scfv = 8;
    string antigen_type = 9;
    string antigen_name = 10;
    repeated string antigen_chains = 11;
    string heavy_subclass = 12;
    string light_subclass = 13;
    string cdr_h1_kabat = 14;
    string cdr_h2_kabat = 15;
    string cdr_h3_kabat = 16;
    string cdr_l1_kabat = 17;
    string cdr_l2_kabat = 18;
    string cdr_l3_kabat = 19;
  }

  Compile: make proto (generates pbuf.pb.go and JSON marshalers)

  ---
  5.3 Parser Implementations

  Pattern: Each database gets its own parser file

  src/update/
  ├── therasabdab.go     (TheraSAbDab parser)
  ├── imgt_gene.go       (IMGT/GENE-DB parser)
  ├── imgt_ligm.go       (IMGT/LIGM-DB parser)
  └── sabdab.go          (SAbDab parser)

  Parser Structure (Example: therasabdab.go)

  package update

  import (
      "encoding/csv"
      "github.com/tamerh/biobtree/src/pbuf"
      "github.com/pquerna/ffjson/ffjson"
  )

  type TheraSAbDab struct {
      *Update
  }

  func (t *TheraSAbDab) update() error {
      // Download CSV
      csvData := t.download()

      // Parse CSV
      reader := csv.NewReader(csvData)
      records, _ := reader.ReadAll()

      for _, record := range records[1:] { // Skip header
          // Extract data
          innName := record[0]
          format := record[1]
          isotype := record[2]
          // ... more fields

          // Create protobuf message
          entry := &pbuf.TheraSAbDab{
              InnName: innName,
              Format: format,
              Isotype: isotype,
              // ... more fields
          }

          // Marshal to JSON
          marshaled, _ := ffjson.Marshal(entry)

          // Store entry (primary data)
          t.addProp3(innName, t.config.ID, marshaled)

          // Add text search
          t.addXref(innName, t.textLinkID, innName, "therasabdab", true)

          // Add cross-references to UniProt targets
          for _, target := range targets {
              uniprotID := lookupUniProt(target)
              if uniprotID != "" {
                  // Bidirectional cross-reference
                  t.addXref(innName, t.config.ID, uniprotID, "uniprot", false)
                  t.addXref(uniprotID, getDatasetID("uniprot"), innName, "therasabdab", false)
              }
          }
      }

      return nil
  }

  Key Functions:
  - addProp3(id, datasetID, marshaledData) - Store primary entry
  - addXref(fromID, fromDatasetID, toID, toDatasetName, isTextSearch) - Create cross-reference
  - Always use dataset ID (int) for fromDatasetID, dataset name (string) for toDatasetName

  ---
  5.4 Merge Logic Updates

  src/generate/mergeg.go

  // Add to xref struct
  type xref struct {
      // ... existing fields
      TheraSAbDab *pbuf.TheraSAbDab
      IMGTGene    *pbuf.IMGTGene
      IMGTLIGM    *pbuf.IMGTLIGM
      SAbDab      *pbuf.SAbDab
  }

  // Add unmarshal cases
  switch datasetID {
      // ... existing cases

      case 46: // TheraSAbDab
          x.TheraSAbDab = &pbuf.TheraSAbDab{}
          ffjson.Unmarshal(value, x.TheraSAbDab)

      case 47: // IMGT/GENE-DB
          x.IMGTGene = &pbuf.IMGTGene{}
          ffjson.Unmarshal(value, x.IMGTGene)

      case 48: // IMGT/LIGM-DB
          x.IMGTLIGM = &pbuf.IMGTLIGM{}
          ffjson.Unmarshal(value, x.IMGTLIGM)

      case 49: // SAbDab
          x.SAbDab = &pbuf.SAbDab{}
          ffjson.Unmarshal(value, x.SAbDab)
  }

  Critical: Without this, attributes will appear empty in API responses!

  ---
  5.5 Filter Support

  src/service/service.go (CEL Declarations)

  // Add CEL variable declarations for antibody datasets
  env, _ := cel.NewEnv(
      // ... existing declarations

      // TheraSAbDab
      cel.Variable("therasabdab.inn_name", cel.StringType),
      cel.Variable("therasabdab.clinical_stage", cel.StringType),
      cel.Variable("therasabdab.isotype", cel.StringType),
      cel.Variable("therasabdab.status", cel.StringType),

      // IMGT/GENE-DB
      cel.Variable("imgt_gene.gene_name", cel.StringType),
      cel.Variable("imgt_gene.species", cel.StringType),
      cel.Variable("imgt_gene.functionality", cel.StringType),
      cel.Variable("imgt_gene.chain_type", cel.StringType),

      // IMGT/LIGM-DB
      cel.Variable("imgt_ligm.species", cel.StringType),
      cel.Variable("imgt_ligm.chain_type", cel.StringType),
      cel.Variable("imgt_ligm.sequence_type", cel.StringType),

      // SAbDab
      cel.Variable("sabdab.pdb_id", cel.StringType),
      cel.Variable("sabdab.species", cel.StringType),
      cel.Variable("sabdab.resolution", cel.DoubleType),
      cel.Variable("sabdab.antigen_type", cel.StringType),
  )

  src/service/mapfilter.go (Filter Evaluation)

  // Add filter evaluation cases
  func evaluateFilter(xref *xref, filter string) bool {
      activation := make(map[string]interface{})

      // ... existing cases

      // TheraSAbDab
      if xref.TheraSAbDab != nil {
          activation["therasabdab.inn_name"] = xref.TheraSAbDab.InnName
          activation["therasabdab.clinical_stage"] = xref.TheraSAbDab.ClinicalStage
          activation["therasabdab.isotype"] = xref.TheraSAbDab.Isotype
          activation["therasabdab.status"] = xref.TheraSAbDab.Status
      }

      // IMGT/GENE-DB
      if xref.IMGTGene != nil {
          activation["imgt_gene.gene_name"] = xref.IMGTGene.GeneName
          activation["imgt_gene.species"] = xref.IMGTGene.Species
          activation["imgt_gene.functionality"] = xref.IMGTGene.Functionality
          activation["imgt_gene.chain_type"] = xref.IMGTGene.ChainType
      }

      // ... more datasets

      // Evaluate filter expression
      out, _ := prg.Eval(activation)
      result, _ := out.Value().(bool)
      return result
  }

  ---
  6. Cross-Reference Strategy

  6.1 Cross-Reference Map

  TheraSAbDab (INN name: "pembrolizumab")
  ├── → UniProt (target: "Q15116" for PD-1)
  ├── → HGNC (target gene: "PDCD1")
  ├── → IMGT/GENE-DB (V gene: "IGHV3-23*01", J gene: "IGHJ4*01")
  ├── → IMGT/LIGM-DB (sequence accessions)
  ├── → SAbDab (PDB structures: "5DK3", "5JXE")
  ├── → PDB (via SAbDab)
  ├── → Clinical Trials (NCT IDs if available)
  ├── → ChEMBL (drug entries if available)
  └── → EFO/MONDO (indications: "melanoma", "NSCLC")

  IMGT/GENE-DB (gene: "IGHV3-23*01")
  ├── → TheraSAbDab (therapeutics using this gene)
  ├── → IMGT/LIGM-DB (sequences containing this gene)
  └── → PubMed (literature references)

  IMGT/LIGM-DB (accession: "IMGT000001")
  ├── → IMGT/GENE-DB (assigned V/D/J genes)
  ├── → GenBank/EMBL (original accessions)
  ├── → UniProt (protein sequences)
  └── → PubMed (literature)

  SAbDab (PDB: "5DK3")
  ├── → PDB (biobtree already has PDB)
  ├── → TheraSAbDab (therapeutic antibody structures)
  ├── → IMGT/GENE-DB (V gene classification)
  └── → UniProt (via PDB → UniProt mapping)

  6.2 Implementation Details

  Bidirectional Cross-References

  Always create both directions:

  // Example: TheraSAbDab → UniProt
  t.addXref("pembrolizumab", 46, "Q15116", "uniprot", false)

  // Reverse: UniProt → TheraSAbDab
  t.addXref("Q15116", uniprotDatasetID, "pembrolizumab", "therasabdab", false)

  Cross-Reference Priority

  1. Direct mappings (highest confidence):
    - TheraSAbDab → UniProt (via target protein names)
    - SAbDab → PDB (1:1 mapping)
    - IMGT/LIGM-DB → GenBank (accession mapping)
  2. Inferred mappings (medium confidence):
    - TheraSAbDab → IMGT/GENE-DB (via V/D/J assignment from literature)
    - SAbDab → TheraSAbDab (via antibody name matching)
  3. Fuzzy mappings (lower confidence, may need manual curation):
    - TheraSAbDab → Clinical Trials (via drug name parsing)
    - TheraSAbDab → ChEMBL (via synonyms)

  Cross-Reference Validation

  Test queries to validate:

  # Known therapeutic → target
  ./biobtree query "pembrolizumab >> uniprot"
  # Expected: Q15116 (PD-1/PDCD1)

  # Known therapeutic → structure
  ./biobtree query "pembrolizumab >> sabdab"
  # Expected: 5DK3, 5JXE, etc.

  # Known gene → therapeutic
  ./biobtree query "IGHV3-23*01 >> therasabdab"
  # Expected: Antibodies using this V gene

  # Structure → annotations
  ./biobtree query "5DK3 >> sabdab"
  # Expected: CDR annotations, species, resolution

  ---
  7. Query Examples

  7.1 Basic Queries

  Lookup by Name

  # Lookup therapeutic antibody
  ./biobtree query "pembrolizumab"

  # Lookup germline gene
  ./biobtree query "IGHV3-23*01"

  # Lookup structure
  ./biobtree query "5DK3 >> sabdab"

  7.2 Cross-Database Queries

  Gene to Drug Discovery

  # Find antibodies targeting a gene
  ./biobtree query "EGFR >> uniprot >> therasabdab"
  # Result: cetuximab, panitumumab, necitumumab

  # Gene → protein → antibodies → clinical trials
  ./biobtree query "BRCA1 >> uniprot >> therasabdab >> clinical_trials"

  # Pathway → proteins → antibodies
  ./biobtree query "R-HSA-162582 >> uniprot >> therasabdab"

  Antibody Engineering Workflows

  # Therapeutic → germline genes
  ./biobtree query "pembrolizumab >> imgt_gene"
  # Result: IGHV3-23*01, IGHJ4*01, IGKV1-39*01, IGKJ1*01

  # Therapeutic → full sequences
  ./biobtree query "pembrolizumab >> imgt_ligm"

  # Therapeutic → structure
  ./biobtree query "pembrolizumab >> sabdab"
  # Result: 5DK3, 5JXE, 5B8C

  Structure-Function Queries

  # PDB → antibody annotations
  ./biobtree query "5DK3 >> sabdab"

  # Find high-resolution human antibody structures
  ./biobtree query "sabdab[sabdab.species=='human' && sabdab.resolution<2.0]"

  # Structure → therapeutic
  ./biobtree query "5DK3 >> sabdab >> therasabdab"

  7.3 Filter Queries

  Filter by Clinical Stage

  # Approved therapeutics only
  ./biobtree query "therasabdab[therasabdab.clinical_stage=='Approved']"

  # Phase III candidates
  ./biobtree query "therasabdab[therasabdab.clinical_stage=='Phase III']"

  # Find approved antibodies for a target
  ./biobtree query "EGFR >> uniprot >> therasabdab[therasabdab.clinical_stage=='Approved']"

  Filter by Antibody Format

  # IgG antibodies only
  ./biobtree query "therasabdab[therasabdab.format~'IgG']"

  # Nanobodies (VHH)
  ./biobtree query "therasabdab[therasabdab.format=='VHH']"

  # Fab fragments
  ./biobtree query "therasabdab[therasabdab.format=='Fab']"

  Filter by Species (Germline)

  # Human V genes only
  ./biobtree query "imgt_gene[imgt_gene.species=='Homo sapiens']"

  # Functional genes only (exclude pseudogenes)
  ./biobtree query "imgt_gene[imgt_gene.functionality=='F']"

  # Heavy chain V genes in humans
  ./biobtree query "imgt_gene[imgt_gene.species=='Homo sapiens' && imgt_gene.chain_type=='IGH']"

  Filter by Structure Quality

  # High-resolution structures (<2 Angstrom)
  ./biobtree query "sabdab[sabdab.resolution<2.0]"

  # Human antibodies with antigens
  ./biobtree query "sabdab[sabdab.species=='human' && sabdab.antigen_type=='protein']"

  # Recent structures (2023+)
  ./biobtree query "sabdab[sabdab.deposition_date>='2023-01-01']"

  7.4 Complex Multi-Hop Queries

  Complete Drug Discovery Pipeline

  # Disease → genes → proteins → antibodies → trials → structures
  ./biobtree query "EFO:0000616 >> gwas >> hgnc >> uniprot >> therasabdab[therasabdab.clinical_stage=='Phase III'] >> clinical_trials"

  Competitive Landscape Analysis

  # Find all antibodies targeting same pathway
  ./biobtree query "R-HSA-162582 >> uniprot >> therasabdab"

  # Compare V gene usage across therapeutics
  ./biobtree query "therasabdab[therasabdab.clinical_stage=='Approved'] >> imgt_gene[imgt_gene.chain_type=='IGH']"

  Antibody Design Template Search

  # Find high-quality structures for specific V gene
  ./biobtree query "IGHV3-23*01 >> imgt_ligm >> therasabdab >> sabdab[sabdab.resolution<2.5]"

  # Find approved antibodies with structures for a target
  ./biobtree query "PD-1 >> uniprot >> therasabdab[therasabdab.clinical_stage=='Approved'] >> sabdab"

  7.5 Web Service API Examples

  REST API Queries

  # Simple lookup
  curl "http://localhost:9292/ws/?i=pembrolizumab"

  # Cross-database mapping
  curl "http://localhost:9292/ws/map/?i=EGFR&m=map(uniprot).map(therasabdab)"

  # Filter query
  curl "http://localhost:9292/ws/map/?i=EGFR&m=map(uniprot).map(therasabdab).filter(therasabdab.clinical_stage==\"Approved\")"

  # Get entry details
  curl "http://localhost:9292/ws/entry/?i=pembrolizumab&s=therasabdab"

  Python Examples

  import requests

  # Find antibodies for target protein
  response = requests.get(
      "http://localhost:9292/ws/map/",
      params={
          "i": "P00533",  # EGFR UniProt ID
          "m": "map(therasabdab)"
      }
  )
  antibodies = response.json()

  # Filter for approved drugs
  response = requests.get(
      "http://localhost:9292/ws/map/",
      params={
          "i": "P00533",
          "m": "map(therasabdab).filter(therasabdab.clinical_stage==\"Approved\")"
      }
  )
  approved_antibodies = response.json()

  ---
  8. Testing & Validation

  8.1 Test Dataset Structure

  Each dataset needs comprehensive tests in tests/datasets/:

  tests/datasets/
  ├── antibody_integration/
  │   ├── README.md                    # Overview of antibody integration
  │   └── integration_tests.sh          # Cross-database tests
  │
  ├── therasabdab/
  │   ├── README.md                    # Dataset documentation
  │   ├── test_data.csv                # Sample data (10-50 entries)
  │   ├── expected_results.json        # Expected query results
  │   └── test_queries.sh              # Test script
  │
  ├── imgt_gene/
  │   ├── README.md
  │   ├── test_data.fasta              # Sample germline genes
  │   ├── expected_results.json
  │   └── test_queries.sh
  │
  ├── imgt_ligm/
  │   ├── README.md
  │   ├── test_data.dat                # Sample EMBL entries
  │   ├── expected_results.json
  │   └── test_queries.sh
  │
  └── sabdab/
      ├── README.md
      ├── test_data.csv                # Sample structure annotations
      ├── expected_results.json
      └── test_queries.sh

  8.2 Unit Tests

  Test Coverage Requirements

  - Parser tests: 90%+ code coverage
  - Cross-reference tests: All xref types validated
  - Filter tests: All filter expressions working
  - Performance tests: Query response < 100ms

  Example Test Cases (tests/datasets/therasabdab/test_queries.sh)

  #!/bin/bash

  echo "Testing TheraSAbDab Integration..."

  # Test 1: Lookup known therapeutic
  result=$(./biobtree query "pembrolizumab")
  if echo "$result" | grep -q "IgG4"; then
      echo "✓ Test 1 passed: Pembrolizumab found"
  else
      echo "✗ Test 1 failed: Pembrolizumab not found"
      exit 1
  fi

  # Test 2: Cross-reference to UniProt
  result=$(./biobtree query "pembrolizumab >> uniprot")
  if echo "$result" | grep -q "Q15116"; then
      echo "✓ Test 2 passed: UniProt cross-reference working"
  else
      echo "✗ Test 2 failed: UniProt cross-reference broken"
      exit 1
  fi

  # Test 3: Filter by clinical stage
  result=$(./biobtree query "therasabdab[therasabdab.clinical_stage=='Approved']")
  count=$(echo "$result" | jq '.count')
  if [ "$count" -gt 0 ]; then
      echo "✓ Test 3 passed: Filter working ($count approved drugs found)"
  else
      echo "✗ Test 3 failed: No approved drugs found"
      exit 1
  fi

  # Test 4: Multi-hop query
  result=$(./biobtree query "EGFR >> uniprot >> therasabdab")
  if echo "$result" | grep -q "cetuximab"; then
      echo "✓ Test 4 passed: Multi-hop query working"
  else
      echo "✗ Test 4 failed: cetuximab not found for EGFR"
      exit 1
  fi

  echo "All tests passed!"

  8.3 Integration Tests

  Cross-Database Validation

  #!/bin/bash
  # tests/datasets/antibody_integration/integration_tests.sh

  echo "Testing Antibody Database Integration..."

  # Test cross-references between all four databases
  test_therapeutic="pembrolizumab"

  # 1. TheraSAbDab → IMGT/GENE-DB
  echo "Testing TheraSAbDab → IMGT/GENE-DB..."
  result=$(./biobtree query "$test_therapeutic >> imgt_gene")
  if echo "$result" | grep -q "IGHV3-23"; then
      echo "✓ Germline gene cross-reference working"
  else
      echo "✗ Germline gene cross-reference broken"
      exit 1
  fi

  # 2. TheraSAbDab → IMGT/LIGM-DB
  echo "Testing TheraSAbDab → IMGT/LIGM-DB..."
  result=$(./biobtree query "$test_therapeutic >> imgt_ligm")
  if [ $(echo "$result" | jq '.count') -gt 0 ]; then
      echo "✓ Sequence cross-reference working"
  else
      echo "✗ Sequence cross-reference broken"
      exit 1
  fi

  # 3. TheraSAbDab → SAbDab
  echo "Testing TheraSAbDab → SAbDab..."
  result=$(./biobtree query "$test_therapeutic >> sabdab")
  if echo "$result" | grep -q "5DK3\|5JXE"; then
      echo "✓ Structure cross-reference working"
  else
      echo "✗ Structure cross-reference broken"
      exit 1
  fi

  # 4. SAbDab → PDB
  echo "Testing SAbDab → PDB..."
  result=$(./biobtree query "5DK3 >> sabdab >> pdb")
  if [ $(echo "$result" | jq '.count') -gt 0 ]; then
      echo "✓ PDB cross-reference working"
  else
      echo "✗ PDB cross-reference broken"
      exit 1
  fi

  # 5. Complete pipeline test
  echo "Testing complete pipeline: Gene → Protein → Antibody → Structure..."
  result=$(./biobtree query "PDCD1 >> uniprot >> therasabdab >> sabdab")
  if [ $(echo "$result" | jq '.count') -gt 0 ]; then
      echo "✓ Complete pipeline working"
  else
      echo "✗ Complete pipeline broken"
      exit 1
  fi

  echo "All integration tests passed!"

  8.4 Validation Criteria

  Success Criteria

  Functional:
  - ✅ All parsers build without errors
  - ✅ All test queries return expected results
  - ✅ Cross-references bidirectional and verified
  - ✅ Filter expressions evaluate correctly
  - ✅ No data loss during updates

  Performance:
  - ✅ Single lookup: <10ms
  - ✅ Cross-reference query: <50ms
  - ✅ Multi-hop query: <100ms
  - ✅ Filter query: <200ms
  - ✅ Build time reasonable (<30 min for core datasets)

  Data Quality:
  - ✅ Known therapeutics found (pembrolizumab, trastuzumab, cetuximab)
  - ✅ Target mappings correct (verified against UniProt)
  - ✅ Structure links functional (verified against PDB)
  - ✅ Germline genes correctly assigned
  - ✅ No broken cross-references

  Storage:
  - ✅ Actual storage within 20% of estimates
  - ✅ Index sizes reasonable
  - ✅ No disk space issues

  ---
  9. Maintenance & Updates

  9.1 Update Frequencies

  | Database     | Update Schedule | Automation         |
  |--------------|-----------------|--------------------|
  | TheraSAbDab  | Weekly          | Automated download |
  | IMGT/GENE-DB | Weekly          | Automated download |
  | IMGT/LIGM-DB | Weekly          | Automated download |
  | SAbDab       | Weekly          | Automated download |

  9.2 Update Automation

  Automated Update Script

  #!/bin/bash
  # scripts/update_antibody_databases.sh

  set -e

  BIOBTREE_DIR="/path/to/biobtree"
  DATA_DIR="$BIOBTREE_DIR/data"
  LOG_DIR="$BIOBTREE_DIR/logs"
  DATE=$(date +%Y-%m-%d)

  echo "[$DATE] Starting antibody database updates..."

  # Update TheraSAbDab
  echo "Updating TheraSAbDab..."
  cd $BIOBTREE_DIR
  ./biobtree -d "therasabdab" build --update 2>&1 | tee $LOG_DIR/update_therasabdab_$DATE.log

  # Update IMGT/GENE-DB
  echo "Updating IMGT/GENE-DB..."
  ./biobtree -d "imgt_gene" build --update 2>&1 | tee $LOG_DIR/update_imgt_gene_$DATE.log

  # Update IMGT/LIGM-DB (large, run weekly)
  if [ $(date +%u) -eq 1 ]; then  # Monday only
      echo "Updating IMGT/LIGM-DB..."
      ./biobtree -d "imgt_ligm" build --update 2>&1 | tee $LOG_DIR/update_imgt_ligm_$DATE.log
  fi

  # Update SAbDab
  echo "Updating SAbDab..."
  ./biobtree -d "sabdab" build --update 2>&1 | tee $LOG_DIR/update_sabdab_$DATE.log

  # Verify integrity
  echo "Verifying database integrity..."
  ./biobtree test therasabdab
  ./biobtree test imgt_gene
  ./biobtree test sabdab

  echo "[$DATE] Antibody database updates complete!"

  Cron Schedule

  # Update antibody databases weekly (Monday 2 AM)
  0 2 * * 1 /path/to/scripts/update_antibody_databases.sh

  # Quick validation daily
  0 6 * * * /path/to/scripts/validate_antibody_databases.sh

  9.3 Version Tracking

  Track database versions for reproducibility:

  {
    "antibody_databases": {
      "last_updated": "2025-11-19",
      "versions": {
        "therasabdab": {
          "version": "2025-11-18",
          "entry_count": 704,
          "download_url": "http://opig.stats.ox.ac.uk/webapps/therasabdab/therasabdab.csv",
          "checksum": "sha256:abc123..."
        },
        "imgt_gene": {
          "version": "202345-0",
          "entry_count": 17290,
          "download_url": "http://www.imgt.org/download/GENE-DB/",
          "checksum": "sha256:def456..."
        },
        "imgt_ligm": {
          "version": "202345-0",
          "entry_count": 251608,
          "download_url": "ftp://ftp.ebi.ac.uk/pub/databases/imgt/LIGM-DB/",
          "checksum": "sha256:ghi789..."
        },
        "sabdab": {
          "version": "2025-11-18",
          "entry_count": 6284,
          "download_url": "https://opig.stats.ox.ac.uk/webapps/sabdab-sabpred/sabdab/summary/all/",
          "checksum": "sha256:jkl012..."
        }
      }
    }
  }

  9.4 Monitoring & Alerts

  Monitor for issues:

  1. Download failures: Alert if source unavailable
  2. Parse errors: Alert if entry count drops >10%
  3. Cross-reference breaks: Alert if known xrefs missing
  4. Performance degradation: Alert if queries >200ms
  5. Storage issues: Alert if disk usage >90%

  ---
  10. Appendices

  Appendix A: Comparison with Original Plan

  What Changed

  | Aspect         | Original Plan                    | Final Plan                                      | Reason                                         |
  |----------------|----------------------------------|-------------------------------------------------|------------------------------------------------|
  | Databases      | TheraSAbDab, IMGT/mAb-DB, SAbDab | TheraSAbDab, IMGT/GENE-DB, IMGT/LIGM-DB, SAbDab | Added germline genes, better sequence access   |
  | Count          | 3 databases                      | 4 databases                                     | IMGT/GENE-DB critical for antibody engineering |
  | IMGT/mAb-DB    | Included                         | Replaced with LIGM-DB                           | LIGM-DB has FTP, broader coverage              |
  | Germline genes | Not included                     | IMGT/GENE-DB added                              | Essential for V(D)J assignments                |
  | Storage        | ~2 GB                            | ~2.3 GB                                         | Added germline + sequences                     |
  | Timeline       | 4-7 weeks                        | 7-10 weeks                                      | Added fourth database                          |

  Why IMGT/GENE-DB Was Added

  - Critical for antibody engineering: V(D)J gene assignments
  - Complements therapeutics: Links clinical drugs to germline origins
  - Enables humanization queries: Essential for antibody design
  - Small size: Only ~100 MB indexed
  - Easy integration: FASTA format, 1-2 weeks effort

  Why IMGT/LIGM-DB Replaced IMGT/mAb-DB

  - Better access: FTP bulk download vs no clear bulk access
  - Broader coverage: 251K sequences vs 1,855 therapeutics
  - Automation: Easier to automate weekly updates
  - Same institution: EBI mirrors IMGT data
  - Complete coverage: Includes therapeutic + research sequences

  Appendix B: IPD Investigation Summary

  Key Finding: IPD (Immuno Polymorphism Database) is NOT relevant for antibody integration.

  IPD focuses on:
  - MHC/HLA genes (transplantation, autoimmunity)
  - KIR receptors (NK cell immunology)
  - Human Platelet Antigens

  IPD does NOT cover:
  - Immunoglobulin (antibody) genes
  - Antibody sequences
  - Therapeutic antibodies

  Conclusion: Use IMGT for antibodies, skip IPD entirely.

  Appendix C: Storage Breakdown

  Core Integration (Phase 1):
  ├── TheraSAbDab:     200 MB
  ├── IMGT/GENE-DB:    100 MB
  └── SAbDab:          500 MB
      Total Core:      800 MB

  Full Integration (Phase 2):
  ├── Core:            800 MB
  └── IMGT/LIGM-DB:  1,500 MB
      Total Full:    2,300 MB

  Optional (Phase 4):
  ├── SAbDab PDB:   10,000 MB
  ├── OGRDB:            50 MB
  └── Other:           150 MB
      Total with Optional: ~12.5 GB

  Recommendation: Start with Core (800 MB), add LIGM-DB in Phase 2 (2.3 GB total), skip optional unless needed.

  Appendix D: License Summary

  All databases free for academic and commercial use:

  | Database     | License  | Commercial Use | Attribution |
  |--------------|----------|----------------|-------------|
  | TheraSAbDab  | Free     | ✅ Yes          | Optional    |
  | IMGT/GENE-DB | CC BY-ND | ✅ Yes          | Required    |
  | IMGT/LIGM-DB | CC BY-ND | ✅ Yes          | Required    |
  | SAbDab       | Free     | ✅ Yes          | Optional    |

  License URLs:
  - IMGT: https://www.ebi.ac.uk/ipd/imgt/hla/licence/
  - TheraSAbDab/SAbDab: No explicit license, free academic resource

  Appendix E: Useful Resources

  Databases:
  - TheraSAbDab: http://opig.stats.ox.ac.uk/webapps/therasabdab
  - IMGT: http://www.imgt.org
  - SAbDab: https://opig.stats.ox.ac.uk/webapps/sabdab-sabpred/sabdab

  FTP Sites:
  - IMGT/LIGM-DB: ftp://ftp.ebi.ac.uk/pub/databases/imgt/LIGM-DB/
  - IMGT/GENE-DB: http://www.imgt.org/download/GENE-DB/

  Documentation:
  - IMGT Documentation: http://www.imgt.org/IMGTindex/IMGTdoc.html
  - IMGT Nomenclature: http://www.imgt.org/IMGTScientificChart/
  - SAbDab Methods: https://academic.oup.com/nar/article/42/D1/D1140/1063178

  Publications:
  - TheraSAbDab: Raybould MIJ et al. Nucleic Acids Res. 2020
  - SAbDab: Dunbar J et al. Nucleic Acids Res. 2014
  - IMGT: Lefranc MP et al. Nucleic Acids Res. 2015

  Appendix F: Build Commands Reference

  # Individual datasets
  ./biobtree -d "therasabdab" build
  ./biobtree -d "imgt_gene" build
  ./biobtree -d "imgt_ligm" build
  ./biobtree -d "sabdab" build

  # Core antibody integration (Phase 1)
  ./biobtree -d "therasabdab,imgt_gene,sabdab,uniprot,hgnc,pdb" build

  # Full antibody integration (Phase 2)
  ./biobtree -d "therasabdab,imgt_gene,imgt_ligm,sabdab,uniprot,hgnc,pdb" build

  # With clinical context
  ./biobtree -d "therasabdab,imgt_gene,sabdab,uniprot,hgnc,pdb,clinical_trials,chembl" build

  # Test individual datasets
  ./biobtree test therasabdab
  ./biobtree test imgt_gene
  ./biobtree test imgt_ligm
  ./biobtree test sabdab

  # Start web interface
  ./biobtree web

  # Example queries
  ./biobtree query "pembrolizumab"
  ./biobtree query "EGFR >> uniprot >> therasabdab"
  ./biobtree query "therasabdab[therasabdab.clinical_stage=='Approved']"

  ---
  Summary & Next Steps

  Implementation Checklist

  Phase 1 (Weeks 1-3):
  - TheraSAbDab configuration and parser
  - IMGT/GENE-DB configuration and parser
  - SAbDab configuration and parser
  - Protobuf schemas defined and compiled
  - Merge logic updated
  - Filter support added
  - Test suites created
  - Integration tests passing

  Phase 2 (Weeks 4-7):
  - IMGT/LIGM-DB configuration and parser
  - Cross-references validated
  - Performance optimizations
  - Tests passing

  Phase 3 (Week 8):
  - Documentation complete
  - All tests passing
  - Production deployment

  Phase 4 (Optional):
  - SAbDab structures (if needed)
  - OGRDB integration (if needed)
  - Advanced features

  Success Metrics

  Technical:
  - ✅ Build completes without errors
  - ✅ Query response time < 100ms
  - ✅ Test coverage > 90%
  - ✅ Storage within estimates

  Scientific:
  - ✅ Known therapeutics found
  - ✅ Cross-references validated
  - ✅ Use cases enabled

  User Experience:
  - ✅ Query syntax intuitive
  - ✅ Documentation clear
  - ✅ Examples working

  Contact & Support

  Implementation Questions:
  - Refer to this document
  - Check biobtree README: biobtreev2/README.md
  - Review existing dataset integrations: tests/datasets/*/README.md

  Database Issues:
  - TheraSAbDab/SAbDab: Oxford OPIG (opig@stats.ox.ac.uk)
  - IMGT: IMGT support (https://www.imgt.org/contact/)

  ---
  Document Version: 2.0
  Last Updated: 2025-11-19
  Status: ✅ READY FOR IMPLEMENTATION
  Total Databases: 4 (TheraSAbDab, IMGT/GENE-DB, IMGT/LIGM-DB, SAbDab)
  Total Storage: ~2.3 GB
  Total Timeline: 7-10 weeks
  Next Action: Begin Phase 1 - TheraSAbDab integration

  ---
  This is the final, comprehensive implementation plan. The document is ready to guide the complete integration of antibody databases into biobtree. Would you like me to proceed to exit plan mode so you can
  begin implementation?





