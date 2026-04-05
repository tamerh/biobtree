# GtoPdb (Guide to Pharmacology) Dataset

## Overview

GtoPdb is the IUPHAR/BPS Guide to Pharmacology database - a curated resource of drug targets and pharmacological agents. It provides expert-curated information on drug targets (GPCRs, ion channels, kinases, enzymes) and their ligands with binding affinity data.

**Source**: https://www.guidetopharmacology.org/
**Data Type**: Drug targets, ligands, and target-ligand interactions with pharmacological data

## Integration Architecture

### Storage Model

**Primary Entries**:
- `gtopdb`: Target IDs (numeric, e.g., "1", "2", "3")
- `gtopdb_ligand`: Ligand IDs (numeric, e.g., "1", "2", "3")
- `gtopdb_interaction`: Composite IDs (target_ligand, e.g., "1_1")

**Searchable Text Links**:
- Target names (e.g., "5-HT1A receptor")
- Ligand names (e.g., "serotonin", "dopamine")
- Drug trade names and INN names

**Attributes Stored**:
- Targets: name, type (GPCR, LGIC, kinase), family, subunits
- Ligands: ADME properties (MW, logP, PSA, HBA, HBD), SMILES, InChIKey, approval status
- Interactions: affinity (pKi, pKd, pIC50), action type, selectivity, endogenous flag

**Cross-References**:
- Targets → UniProt, HGNC
- Ligands → PubChem, ChEMBL, ChEBI
- Interactions → PubMed

### Special Features

- **ADME Profiling**: Ligands include physico-chemical properties for ADME/drug-likeness assessment
- **Lipinski Rule-of-5**: HBA, HBD, rotatable bonds, rules broken count
- **Affinity Data**: Quantitative binding data with parameter types (pKi, pKd, pIC50, pEC50)
- **Action Classification**: Agonist, antagonist, inhibitor, modulator classifications
- **Peptide Sequences**: Amino acid sequences for peptide ligands
- **WHO Essential Medicines**: Flag for WHO essential medicine list membership

## Use Cases

**1. Target-based Drug Discovery**
```
Query: Find all ligands binding to a specific GPCR target
Use: Identify lead compounds for target validation
```

**2. ADME/Drug-likeness Filtering**
```
Query: Filter ligands by Lipinski properties (MW < 500, logP < 5)
Use: Prioritize drug-like compounds in screening
```

**3. Affinity-based Ranking**
```
Query: Get interactions with pKi > 8 for selectivity analysis
Use: Identify high-affinity selective compounds
```

**4. Mechanism of Action Analysis**
```
Query: Find all agonists vs antagonists for receptor family
Use: Understand pharmacological profiles across target families
```

**5. Protein-Ligand Network**
```
Query: Map ligand to all its targets via interactions
Use: Predict off-target effects and polypharmacology
```

**6. Cross-database Integration**
```
Query: Target → UniProt → protein structure (AlphaFold)
Use: Structure-based drug design with curated binding data
```

## Test Cases

**Current Tests** (22 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 18 custom tests covering all three datasets

**Coverage**:
- ✅ Target lookup and attributes (name, type, family)
- ✅ Ligand lookup and ADME properties
- ✅ Interaction lookup and affinity data
- ✅ Cross-references (UniProt, HGNC, PubChem, ChEMBL)
- ✅ Inter-dataset mappings (target↔interaction↔ligand)

**Recommended Additions**:
- WHO essential medicine filtering
- Approved drug validation
- Peptide sequence tests

## Performance

- **Test Build**: ~5-10s (100 entries per dataset)
- **Data Source**: GtoPdb CSV downloads
- **Update Frequency**: Quarterly releases
- **Total Entries**: ~3,000 targets, ~12,000 ligands, ~100,000 interactions
- **Special notes**: All three datasets built together from gtopdb trigger

## Known Limitations

- Species-specific mappings only for Human (UniProt, HGNC)
- Interaction IDs are composite (not directly searchable by affinity)
- Peptide sequences available only for peptide-type ligands
- Some ligands lack physico-chemical properties

## Future Work

- Add disease association mappings
- Include target family hierarchies
- Add ADME filtering in CEL queries
- Support species-specific target lookups
- Add peptide ligand sequence search

## Maintenance

- **Release Schedule**: Quarterly
- **Data Format**: CSV files
- **Test Data**: 100 targets (first 100 by ID)
- **License**: CC BY-SA 4.0
- **Special notes**: Data curated by IUPHAR/BPS expert committees

## References

- **Citation**: Harding SD, et al. (2024) Nucleic Acids Res. 52(D1):D1282-D1290
- **Website**: https://www.guidetopharmacology.org/
- **License**: Creative Commons Attribution-ShareAlike 4.0
