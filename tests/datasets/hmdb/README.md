# HMDB (Human Metabolome Database) Dataset

## Overview

HMDB is a comprehensive resource for human metabolites. Contains detailed information on small molecule metabolites found in the human body including chemical properties, biological roles, disease associations, and biospecimen locations. Essential for metabolomics research, biomarker discovery, and systems biology.

**Source**: HMDB XML dump (FTP download)
**Data Type**: Human metabolite encyclopedia with chemical structures and biological context

## Integration Architecture

### Storage Model

**Primary Entries**:
- HMDB IDs (e.g., `HMDB0000001`) stored as main identifiers
- Secondary accessions (old format like `HMDB00001`) mapped to primary IDs

**Searchable Text Links**:
- Metabolite names (e.g., `1-Methylhistidine`) → self-referencing keywords
- Synonyms and IUPAC names → text search
- InChI Keys → text search (enables structure-based lookup)
- Chemical formulas → text search

**Attributes Stored** (protobuf):
- Chemical properties: formula, SMILES, InChI, InChI Key, molecular weight
- Names: common name, IUPAC name, traditional IUPAC, synonyms
- Biological context: disease associations, biospecimen locations, tissue locations
- Pathway information: metabolic pathway memberships
- Ontology classifications: chemical taxonomy, role, application

**Cross-References**:
- **Chemical databases**: ChEBI, PubChem, KEGG Compound, CAS Registry, DrugBank
- **Biological**: KEGG pathways, BioCyc, SMPDB (Small Molecule Pathway Database)
- **Literature**: PubMed references
- **Protein interactions**: UniProt IDs for enzymes/transporters

### Special Features

**Multiple Name Forms**:
- Primary name + IUPAC name + traditional IUPAC + synonyms all searchable
- Historical accessions (HMDB00001 → HMDB0000001) supported

**Chemical Structure Search**:
- InChI Key enables structure-based lookups
- SMILES notation stored for cheminformatics integration

**Clinical Context**:
- Disease associations for biomarker identification
- Biospecimen locations (blood, urine, saliva, CSF, etc.)
- Tissue distribution information

**Metabolic Pathways**:
- Pathway memberships link metabolism to biological processes
- Enables metabolic network reconstruction

## Use Cases

**1. Metabolite Identification**
```
Query: InChI Key from mass spec → Find HMDB metabolite → Get identity and properties
Use: Unknown peak identification in metabolomics experiments
```

**2. Biomarker Discovery**
```
Query: HMDB metabolite → Check disease associations → Identify potential biomarkers
Use: Clinical diagnostics, disease mechanism research
```

**3. Metabolic Pathway Analysis**
```
Query: HMDB ID → Get pathway information → Map to metabolic networks
Use: Systems biology, flux analysis, metabolic modeling
```

**4. Chemical Property Lookup**
```
Query: Metabolite name → Get molecular weight, formula, SMILES → Use in calculations
Use: Concentration conversion, structure prediction, LC-MS method development
```

**5. Biospecimen Analysis**
```
Query: HMDB metabolite → Check biospecimen locations → Prioritize sample types
Use: Experimental design for metabolomics studies
```

**6. Cross-Database Integration**
```
Query: HMDB ID → Get KEGG/ChEBI/PubChem IDs → Link to other resources
Use: Comprehensive metabolite annotation across platforms
```

## Test Cases

**Current Tests** (16 total):
- 8 declarative tests (ID lookup, name, synonyms, secondary accessions, attributes, multi-lookup, case-insensitive, invalid ID)
- 8 custom tests (chemical formula, SMILES, molecular weight, diseases, pathways, biospecimens, KEGG xref, InChI Key search)

**Coverage**:
- ✅ HMDB ID and secondary accession lookup
- ✅ Metabolite name and synonym search
- ✅ Case-insensitive search
- ✅ InChI Key searchability (structure-based lookup)
- ✅ Chemical properties (formula, SMILES, molecular weight)
- ✅ Disease associations
- ✅ Pathway information
- ✅ Biospecimen locations
- ✅ KEGG cross-references

**Recommended Additions**:
- Tissue location validation
- Chemical taxonomy/classification tests
- Protein interaction cross-references (enzymes/transporters)

## Performance

- **Test Build**: ~2.2s (20 metabolite entries)
- **Data Source**: XML dump from HMDB (FTP download)
- **Update Frequency**: Regular updates (1-2 times per year)
- **Total Metabolites**: ~220,000+ entries (primary + predicted metabolites)

## Known Limitations

**Cross-Reference Completeness**:
- Not all metabolites have mappings to all external databases
- Chemical databases (ChEBI, PubChem, DrugBank) may not be in isolated test builds

**Predicted vs Detected**:
- HMDB contains both experimentally detected and computationally predicted metabolites
- Test data uses detected metabolites for reliability

**Protein Interactions**:
- Enzyme and transporter associations stored but not heavily tested
- Requires UniProt integration for full validation

## Future Work

- Add tissue location validation tests
- Test chemical taxonomy classification fields
- Add protein interaction tests (enzymes, transporters)
- Test BioCyc and SMPDB pathway cross-references
- Add multi-database test (HMDB + ChEBI + PubChem) to validate chemical xrefs
- Test concentration and reference range data

## Maintenance

- **Release Schedule**: Major updates 1-2 times per year from HMDB
- **Data Format**: XML (stable format)
- **Test Data**: Fixed 20 metabolite IDs spanning various chemical classes and biological roles
- **Version**: Currently using HMDB 5.0+ format

## References

- **Citation**: Wishart DS et al. HMDB 5.0: the Human Metabolome Database for 2022. Nucleic Acids Research.
- **Website**: https://hmdb.ca/
- **License**: Creative Commons Attribution-NonCommercial 4.0 - free for academic use
