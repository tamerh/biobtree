# ChEMBL Molecule Dataset

## Overview

ChEMBL Molecule contains small molecule compounds with drug-like properties from the ChEMBL database. Includes chemical structures (SMILES, InChI), molecular properties (MW, logP, HBA/HBD), drug development status, and ATC classifications. Part of EMBL-EBI's ChEMBL bioactivity database with 2.3+ million compounds covering approved drugs, clinical candidates, and bioactive molecules.

**Source**: ChEMBL RDF (EMBL-EBI)
**Data Type**: Small molecule compounds with chemical properties and drug development annotations

## Integration Architecture

### Storage Model

**Primary Entries**:
- ChEMBL IDs (e.g., `CHEMBL6939`) stored as main identifiers

**Searchable Text Links**:
- Compound names indexed for text search
- Trade names and synonyms searchable

**Attributes Stored** (protobuf ChemblMoleculeAttr):
- `smiles`: Canonical SMILES structure notation
- `inchi_key`: Standard InChI Key identifier
- `molecular_formula`: Chemical formula (e.g., C17H27NO3)
- `molecular_weight`: MW in Daltons
- `alogp`: Calculated logP for lipophilicity
- `hba/hbd`: Hydrogen bond acceptors/donors
- `molecule_type`: Small molecule, protein, antibody, etc.
- `max_phase`: Clinical development phase (0-4, null=preclinical)
- `atc_codes`: Anatomical Therapeutic Chemical classification

**Cross-References**:
- **Drug targets**: chembl_target (protein targets)
- **Bioactivity**: chembl_activity (assay measurements)
- **Assays**: chembl_assay (screening protocols)
- **Documents**: chembl_document (literature references)
- **Patents**: SureChEMBL patent database
- **Clinical trials**: Clinical trial cross-references

### Special Features

**Chemical Structure Search**:
- SMILES and InChI Key for structure-based lookups
- Enables cheminformatics integration

**Drug Development Tracking**:
- max_phase indicates clinical trial status
- ATC codes for approved drugs
- Black box warnings flagged

**Molecular Properties**:
- Lipinski Rule of 5 compliance data
- Physicochemical properties for ADME prediction

**Sparse RDF Handling**:
- Smart tracking for incomplete data in ChEMBL RDF
- Handles molecules with partial annotations

## Use Cases

**1. Drug Discovery**
```
Query: Target protein → ChEMBL bioactivity → Active molecules → Chemical structures
Use: Identify lead compounds for drug development
```

**2. Structure-Activity Relationship (SAR)**
```
Query: Molecule series → SMILES structures → Activity data
Use: Optimize compound potency and selectivity
```

**3. ADME/Tox Prediction**
```
Query: Molecule → Molecular properties (MW, logP, HBA/HBD) → Predict absorption
Use: Filter compounds for drug-likeness
```

**4. Clinical Drug Lookup**
```
Query: Drug name → max_phase + ATC codes → Clinical status
Use: Check drug approval status and therapeutic class
```

**5. Chemical Space Analysis**
```
Query: Compound set → Molecular properties → Diversity analysis
Use: Design diverse screening libraries
```

**6. Patent/Literature Search**
```
Query: Molecule → Documents + Patents → Prior art
Use: Freedom-to-operate analysis
```

## Test Cases

**Current Tests** (9 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 5 custom tests (SMILES, InChI, properties, formula, type)

**Coverage**:
- ✅ ChEMBL molecule ID lookup
- ✅ SMILES structure notation
- ✅ InChI Key identifiers
- ✅ Molecular properties (MW, logP, etc.)
- ✅ Molecular formula
- ✅ Molecule type classification
- ✅ Attribute validation

**Recommended Additions**:
- max_phase clinical development status tests
- ATC code validation for approved drugs
- Cross-reference tests (chembl_target, chembl_activity)
- Black box warning flag tests
- Lipinski Rule of 5 compliance tests

## Performance

- **Test Build**: ~9.6s (20 molecules)
- **Data Source**: ChEMBL RDF (EMBL-EBI FTP)
- **Update Frequency**: Quarterly ChEMBL releases
- **Total Molecules**: 2.3+ million compounds
- **Note**: Smart tracking handles sparse RDF data efficiently

## Known Limitations

**Sparse Data**:
- Not all molecules have complete property sets
- Some compounds lack clinical development info
- ATC codes only for approved drugs

**RDF Processing**:
- Blank node filtering required for clean data
- Some cross-references may be incomplete in test builds

**Structure Searchability**:
- InChI Key text search depends on keyword indexing
- Full substructure/similarity search requires external tools

**Cross-References**:
- Requires other ChEMBL datasets (target, activity, assay) for full integration
- Patent/literature links depend on those databases being configured

## Future Work

- Add max_phase clinical status tests
- Test ATC code integration for approved drugs
- Add cross-reference validation (chembl_target, chembl_activity)
- Test black box warning flags
- Add Lipinski Rule of 5 compliance tests
- Test molecule hierarchy (parent-child relationships)
- Add trade name/synonym search validation
- Test integration with SureChEMBL patents

## Maintenance

- **Release Schedule**: Quarterly from ChEMBL (currently v34+)
- **Data Format**: RDF/XML with blank node handling
- **Test Data**: Fixed 20 molecule IDs spanning drug types
- **License**: CC BY-SA 3.0 - freely available with attribution
- **Coordination**: Part of ChEMBL suite (molecule, target, activity, assay, document, cell_line)

## References

- **Citation**: Zdrazil B et al. (2024) The ChEMBL Database in 2023. Nucleic Acids Res. 52(D1):D1180-D1189.
- **Website**: https://www.ebi.ac.uk/chembl/
- **API**: https://www.ebi.ac.uk/chembl/api/data/docs
- **License**: CC BY-SA 3.0
