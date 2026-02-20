# ChEMBL Dataset

## Overview

ChEMBL is the world's largest open-access drug discovery database, containing bioactivity data for millions of drug-like molecules. It integrates binding, functional, and ADMET data from medicinal chemistry literature and deposited datasets.

**Source**: EMBL-EBI ChEMBL (https://www.ebi.ac.uk/chembl/)
**Data Type**: Drug discovery data - targets, molecules, activities, assays, documents, cell lines

## Integration Architecture

### Storage Model

**Primary Entries**: 6 entity types stored as primary entries
- Targets: CHEMBL203 (drug targets)
- Molecules: CHEMBL25 (drug compounds)
- Activities: CHEMBL_ACT_93229 (bioactivity measurements)
- Assays: CHEMBL1217643 (experimental protocols)
- Documents: CHEMBL1152233 (literature references)
- Cell Lines: CHEMBL3307243 (cell line metadata)

**Searchable Text Links**:
- Target names (preferred name)
- Molecule names and synonyms (trade names, research codes)
- Assay descriptions
- Document titles

**Attributes Stored**:
- Target: title, type, bindingSite, mechanism, subsetofs, tax
- Molecule: name, type, highestDevelopmentPhase, altNames, atcClassification, indications, childs, parent
- Activity: assay, molecule, target, value, type, units, uniprot
- Assay: description, type, target, document
- Document: title, docType, journal, year
- Cell Line: name, description, organism, tax

**Cross-References**:
- Target -> UniProt (direct), Taxonomy
- Molecule -> ChEMBL Target, EFO, MeSH (indications)
- Activity -> Molecule, Target, UniProt
- Assay -> Target, Document
- Document -> PubMed (literature_mappings)
- Cell Line -> Taxonomy, EFO

### Key Relationships: Target vs Molecule vs UniProt

```
chembl_molecule ──────→ chembl_target ──────→ uniprot
     (drug)        acts on    (target)    is     (protein)
```

- **Target**: The biological entity (protein) that a drug acts ON. Target IS a protein.
- **Molecule**: The chemical compound (drug). Molecule ACTS ON targets.
- **UniProt**: The protein sequence/annotation database.

**Query paths**:
- `molecule >> target`: "What targets does this drug act on?"
- `target >> uniprot`: "What protein is this target?"
- `molecule >> target >> uniprot`: "What proteins does this drug affect?"
- `uniprot >> chembl_target >> chembl_molecule`: "What drugs target this protein?"

### Special Features

**SQLite-Based Extraction**: Uses ChEMBL SQLite database directly instead of RDF parsing. Faster extraction and more complete data coverage.

**Direct Target-UniProt Links**: Targets link directly to UniProt accessions without intermediate target_component entity. Simplifies drug-target-protein queries.

**Semantic Data Model**: Molecule→Target→UniProt path preserves biological meaning (drug acts on target, target is protein). Avoids confusing direct molecule→uniprot shortcuts.

**Lean Data Model**: Molecular properties (SMILES, InChI, weight, formula) delegated to PubChem. Protein classifications delegated to UniProt/GO/InterPro. Reduces redundancy.

**Synonym Text Search**: Molecule synonyms (trade names, research codes) indexed for text search. Enables drug lookup by brand name.

**Indication Integration**: Drug indications link to EFO/MeSH disease ontologies with development phase information.

## Use Cases

**1. Drug Target Identification**
```
Query: Find protein targets for a drug class -> molecule synonyms -> target xrefs
Use: Identify all proteins targeted by kinase inhibitors
```

**2. Lead Compound Discovery**
```
Query: Find compounds active against target -> target -> activities -> molecules
Use: Screen for EGFR inhibitor candidates with IC50 data
```

**3. Drug Repurposing**
```
Query: Find approved drugs for disease -> indication EFO -> molecules with phase 4
Use: Identify existing drugs that may treat rare diseases
```

**4. Assay Protocol Research**
```
Query: Find assays for target type -> assays by description -> protocols
Use: Design screening assays based on published methods
```

**5. Literature Mining**
```
Query: Find publications for drug-target pair -> documents -> PubMed
Use: Review evidence for drug mechanism of action
```

**6. Cell Line Selection**
```
Query: Find cell lines for organism/tissue -> cell lines -> assays
Use: Select appropriate models for drug screening
```

## Test Cases

**Current Tests** (36 total):
- 14 declarative tests (ID lookup, attribute check, multi-lookup, invalid ID)
- 22 custom tests (entity-specific validation)

**Coverage**:
- Target: name, type, taxonomy, UniProt xref, SINGLE PROTEIN type
- Molecule: name, type, synonyms, small molecule type
- Activity: assay link, molecule link, target link, measurement values
- Assay: description, type, target link
- Document: PUBLICATION type, journal, PubMed ID
- Cell Line: name, organism, taxonomy

**Recommended Additions**:
- Molecule indication tests (EFO/MeSH xrefs)
- Molecule parent/child hierarchy tests
- Target binding site and mechanism tests
- Activity organism tests

## Performance

- **Test Build**: ~2-3 minutes (20 entries per entity type, ~120 total)
- **Data Source**: ChEMBL SQLite database (~35GB)
- **Update Frequency**: Quarterly releases
- **Total Entries**: ~2.4M molecules, ~15K targets, ~21M activities

## Known Limitations

- **Molecular Properties**: SMILES, InChI, weight not stored (use PubChem)
- **Protein Classifications**: Not stored (use UniProt/GO/InterPro)
- **Target Components**: Eliminated - use direct UniProt xrefs instead
- **Activity Filtering**: Only activities with targets are extracted (3.8M of 21M)

## Future Work

- Add mechanism of action queries
- Add ATC classification hierarchy navigation
- Add bioactivity filtering by potency thresholds
- Add drug-drug interaction inference

## Maintenance

- **Release Schedule**: ChEMBL releases quarterly
- **Data Format**: SQLite database
- **Test Data**: 20 entries per entity type (120 total)
- **License**: CC BY-SA 3.0

## References

- **Citation**: Mendez et al. ChEMBL: towards direct deposition of bioassay data. Nucleic Acids Research (2019)
- **Website**: https://www.ebi.ac.uk/chembl/
- **License**: Creative Commons Attribution-Share Alike 3.0 Unported License
