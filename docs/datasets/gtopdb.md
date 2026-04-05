# GtoPdb (Guide to Pharmacology) Dataset

## Overview
The IUPHAR/BPS Guide to Pharmacology (GtoPdb) is a curated database of drug targets and their ligands. It provides expert-reviewed information on GPCRs, ion channels, nuclear hormone receptors, kinases, enzymes, and transporters, along with their interactions with approved drugs, clinical candidates, and research compounds.

**Source**: https://www.guidetopharmacology.org/
**Data Type**: Drug targets, ligands, target-ligand interactions with binding affinity data

## Datasets (3 Total)

| Dataset ID | Name | Description | Key Attributes |
|------------|------|-------------|----------------|
| 136 | gtopdb | Drug targets (GPCRs, ion channels, enzymes, etc.) | name, type, family_name, family_ids, subunit_ids, complex_ids |
| 137 | gtopdb_ligand | Ligands (drugs, compounds, peptides) | name, type, approved, smiles, inchi_key, molecular_weight, logp |
| 138 | gtopdb_interaction | Target-ligand binding data | target_name, ligand_name, action_type, action, affinity, affinity_parameter |

## Integration Architecture

### Storage Model
**Primary Entries**: GtoPdb IDs (e.g., "1" for 5-HT1A receptor, "4139" for aspirin)
**Searchable Text Links**: Target names, ligand names, INN names, synonyms, significant words from multi-word names
**Cross-References**: UniProt, HGNC, PubChem, ChEMBL, ChEBI, PubMed

### Files Processed
1. **targets_and_families.csv**: Drug targets with family relationships (~3,000 entries)
2. **ligands.csv**: Ligands with drug properties and approval status (~12,000 entries)
3. **interactions.csv**: Target-ligand binding data with affinity values (~180,000 interactions)
4. **ligand_id_mapping.csv**: Cross-references to PubChem, ChEMBL, ChEBI
5. **ligand_physchem_properties.csv**: ADME-related physicochemical properties
6. **peptides.csv**: Peptide sequences for peptide ligands
7. **GtP_to_UniProt_mapping.csv**: Target to UniProt mappings
8. **GtP_to_HGNC_mapping.csv**: Target to HGNC mappings

### Special Features
- **Target types**: gpcr, ion_channel, nuclear_hormone_receptor, kinase, enzyme, transporter, catalytic_receptor, other_protein
- **Approved drugs**: Boolean flag for regulatory approval status
- **Affinity data**: Binding affinity with parameter type (Ki, Kd, IC50, EC50, pKi, pKd, etc.)
- **Endogenous ligands**: Flagged for natural/physiological ligands
- **Selectivity**: Primary vs secondary target designation
- **Physicochemical properties**: MW, LogP, HBA, HBD, PSA, rotatable bonds, Lipinski violations
- **Synonym text search**: Individual significant words indexed for partial phrase matching

## Use Cases

**1. Find Drug Targets by Type**
```
Query: Find all GPCR drug targets
>>gtopdb[type=="gpcr"]
Use: Identify GPCRs for drug discovery campaigns
```

**2. Find Approved Drugs**
```
Query: Find approved drugs in GtoPdb
>>gtopdb_ligand[approved==true]
Use: Focus on clinically validated compounds
```

**3. Target to Protein Mapping**
```
Query: Find UniProt IDs for serotonin receptors
serotonin >> gtopdb >> uniprot → P08908, P28223, P28221...
Use: Link pharmacology targets to protein sequence/structure
```

**4. Drug to Target Interactions**
```
Query: Find all targets of aspirin
aspirin >> gtopdb_ligand >> gtopdb_interaction >> gtopdb
Use: Understand polypharmacology and off-target effects
```

**5. Affinity-Based Filtering**
```
Query: Find high-affinity interactions
>>gtopdb_interaction[affinity_value<10 && affinity_parameter=="pKi"]
Use: Identify potent ligands for target validation
```

**6. Endogenous Ligand Discovery**
```
Query: Find endogenous ligands for a target
1 >> gtopdb >> gtopdb_interaction[endogenous==true] >> gtopdb_ligand
Use: Identify natural receptor ligands
```

**7. Cross-Database Integration**
```
Query: Link GtoPdb targets to disease associations
target >> gtopdb >> hgnc >> clinvar >> mondo
Use: Drug repurposing based on disease genetics
```

**8. Chemical Structure Lookup**
```
Query: Get PubChem/ChEMBL IDs for a ligand
aspirin >> gtopdb_ligand >> pubchem
aspirin >> gtopdb_ligand >> chembl_molecule
Use: Access additional chemical data and bioactivity
```

## Test Cases

**Total Tests**: 22+ unit tests covering all 3 datasets

### gtopdb (Target) Tests
- Target lookup by ID
- Target name and type validation
- Family relationships (family_name, family_ids)
- Subunit and complex relationships
- UniProt cross-references
- HGNC cross-references
- Text search by target name
- Text search by synonyms

### gtopdb_ligand Tests
- Ligand lookup by ID
- Ligand name and type validation
- Approved drug flag
- Physicochemical properties (MW, LogP, PSA)
- SMILES and InChIKey presence
- PubChem cross-references
- ChEMBL cross-references
- ChEBI cross-references
- Text search by ligand name
- Text search by INN name

### gtopdb_interaction Tests
- Interaction lookup
- Target-ligand linkage
- Action type and action
- Affinity data (value, parameter, units)
- Endogenous ligand flag
- Primary target flag
- PubMed cross-references

### Integration Tests (in xintegration/)
- gtopdb to uniprot mapping
- gtopdb to hgnc mapping
- gtopdb_ligand to pubchem mapping
- gtopdb_ligand to chembl_molecule mapping
- gtopdb filter by type
- gtopdb_ligand filter by approved
- Multi-hop: gtopdb >> gtopdb_interaction >> gtopdb_ligand >> pubchem

## Performance

- **Test Build**: ~5-10s (processes all CSV files)
- **Data Source**: CSV files from guidetopharmacology.org/DATA/
- **Update Frequency**: Quarterly (follows GtoPdb release schedule)
- **Total Entries**: ~3,000 targets, ~12,000 ligands, ~180,000 interactions

## Data Model

### GtopdbAttr (Target Entry - ID 136)
- `name`: Target name (e.g., "5-HT<sub>1A</sub> receptor")
- `type`: Target type (gpcr, ion_channel, enzyme, etc.)
- `family_name`: Target family (e.g., "5-Hydroxytryptamine receptors")
- `family_ids`: Array of family IDs
- `subunit_ids`: Array of subunit target IDs (for complexes)
- `complex_ids`: Array of complex IDs (for subunits)

### GtopdbLigandAttr (Ligand Entry - ID 137)
- `name`: Ligand name
- `type`: Ligand type (Synthetic organic, Peptide, Antibody, etc.)
- `inn`: International Nonproprietary Name
- `approved`: Boolean - regulatory approval status
- `withdrawn`: Boolean - withdrawn from market
- `who_essential`: Boolean - on WHO Essential Medicines list
- `antibacterial`: Boolean - antibacterial activity
- `radioactive`: Boolean - radioactive compound
- `labelled`: Boolean - isotope-labelled
- `smiles`: SMILES chemical structure
- `inchi_key`: InChIKey identifier
- `molecular_weight`: Molecular weight (Da)
- `logp`: Partition coefficient
- `hba`: Hydrogen bond acceptors
- `hbd`: Hydrogen bond donors
- `psa`: Polar surface area
- `rotatable_bonds`: Number of rotatable bonds
- `lipinski_broken`: Number of Lipinski violations
- `one_letter_seq`: Peptide sequence (1-letter)
- `three_letter_seq`: Peptide sequence (3-letter)

### GtopdbInteractionAttr (Interaction Entry - ID 138)
- `target_id`: Target GtoPdb ID
- `ligand_id`: Ligand GtoPdb ID
- `target_name`: Target name
- `ligand_name`: Ligand name
- `target_species`: Species (Human, Mouse, Rat, etc.)
- `action_type`: Type of action (Agonist, Antagonist, Inhibitor, etc.)
- `action`: Specific action
- `selectivity`: Selectivity classification
- `endogenous`: Boolean - endogenous ligand
- `primary_target`: Boolean - primary vs secondary target
- `affinity`: Affinity value as string
- `affinity_parameter`: Parameter type (Ki, Kd, IC50, EC50, pKi, pKd, etc.)
- `affinity_units`: Units (nM, M, etc.)
- `affinity_value`: Numeric affinity value

## Cross-Reference Summary

| From | To | Description |
|------|-----|-------------|
| gtopdb | uniprot | Target protein sequences |
| gtopdb | hgnc | Target gene symbols |
| gtopdb | gtopdb_interaction | Target-ligand interactions |
| gtopdb_ligand | pubchem | PubChem compound IDs |
| gtopdb_ligand | chembl_molecule | ChEMBL compound IDs |
| gtopdb_ligand | chebi | ChEBI compound IDs |
| gtopdb_ligand | gtopdb_interaction | Ligand-target interactions |
| gtopdb_interaction | gtopdb | Parent target |
| gtopdb_interaction | gtopdb_ligand | Parent ligand |
| gtopdb_interaction | pubmed | Supporting publications |

## Maintenance

- **Release Schedule**: Quarterly updates from GtoPdb
- **Data Format**: CSV files (UTF-8, comma-separated)
- **Test Data**: Full processing (small dataset, no test mode needed)
- **License**: Creative Commons Attribution-ShareAlike 4.0 (CC BY-SA 4.0)

## References

- **Citation**: Harding SD, et al. (2024) The IUPHAR/BPS Guide to PHARMACOLOGY in 2024. Nucleic Acids Res.
- **Website**: https://www.guidetopharmacology.org/
- **Data Download**: https://www.guidetopharmacology.org/DATA/
- **License**: CC BY-SA 4.0
