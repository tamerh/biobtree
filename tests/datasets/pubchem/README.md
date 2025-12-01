# PubChem Test Suite

Tests for PubChem Compound dataset integration in biobtree.

## Overview

PubChem is the world's largest chemical database (119M+ compounds). This integration focuses on a biotech-relevant subset:

- **P0 (Phase 1 - IMPLEMENTED)**: FDA-approved drugs (~20K compounds)
- **P1 (Future)**: Clinical trial compounds (~30-50K)
- **P2 (Future)**: Natural products (~300-500K)
- **P3 (Future)**: Bioassay-tested compounds (~500K-1M)

## Test Dataset

The test uses 20 FDA-approved drugs (P0 priority):

| CID      | Drug Name           | Category           |
|----------|--------------------|--------------------|
| 2244     | Aspirin            | Analgesic          |
| 3672     | Ibuprofen          | NSAID              |
| 5090     | Metformin          | Antidiabetic       |
| 60823    | Atorvastatin       | Statin             |
| 5284371  | Losartan           | ARB                |
| 5311304  | Lisinopril         | ACE inhibitor      |
| 6918485  | Simvastatin        | Statin             |
| 6433272  | Amlodipine         | Calcium channel    |
| 3001055  | Gabapentin         | Anticonvulsant     |
| 5743     | Ciprofloxacin      | Antibiotic         |
| 60838    | Omeprazole         | PPI                |
| 3032771  | Montelukast        | Leukotriene        |
| 5311508  | Levothyroxine      | Thyroid hormone    |
| 444403   | Sertraline         | SSRI               |
| 656832   | Citalopram         | SSRI               |
| 2662     | Acetaminophen      | Analgesic          |
| 3652     | Albuterol          | Bronchodilator     |
| 4046     | Prednisone         | Corticosteroid     |
| 5362129  | Warfarin           | Anticoagulant      |
| 5281040  | Rosuvastatin       | Statin             |

## Test Coverage

### Declarative Tests (test_cases.json)
- ID lookups for multiple FDA-approved drugs
- Attribute validation (molecular properties, drug flags)
- Invalid ID handling

### Custom Tests (test_pubchem.py)

1. **test_fda_approved_flag**: Validates FDA-approved flag and compound_type
2. **test_molecular_properties**: Checks SMILES, InChI Key, molecular formula/weight
3. **test_lipinski_properties**: Validates Rule of Five properties (HBD, HBA, XLogP)
4. **test_synonyms_present**: Verifies synonym storage and common names
5. **test_chembl_cross_reference**: Checks bidirectional PubChem ↔ ChEMBL links
6. **test_chebi_cross_reference**: Checks bidirectional PubChem ↔ ChEBI links
7. **test_text_search_by_smiles**: Validates SMILES text search indexing
8. **test_text_search_by_inchi_key**: Validates InChI Key text search indexing

## Data Attributes

The PubChem parser extracts:

### Core Identifiers
- `cid`: PubChem Compound ID
- `inchi`: International Chemical Identifier
- `inchi_key`: InChI Key (hashed version)
- `smiles`: Simplified molecular-input line-entry system
- `isomeric_smiles`: SMILES with stereochemistry

### Names and Synonyms
- `title`: Primary compound name
- `iupac_name`: IUPAC systematic name
- `synonyms[]`: List of alternative names (up to 20 most common)

### Molecular Properties
- `molecular_formula`: Chemical formula
- `molecular_weight`: Molecular weight (g/mol)
- `exact_mass`: Monoisotopic mass
- `xlogp`: Octanol-water partition coefficient (lipophilicity)
- `hydrogen_bond_donors`: H-bond donor count
- `hydrogen_bond_acceptors`: H-bond acceptor count
- `rotatable_bonds`: Number of rotatable bonds
- `tpsa`: Topological polar surface area

### Drug/Bioactivity Flags
- `is_fda_approved`: FDA approval status
- `is_clinical_trial`: Clinical trial participation
- `is_natural_product`: Natural product flag
- `has_bioactivity`: Bioassay activity flag
- `compound_type`: "drug", "clinical_trial", "natural_product", or "bioactive"

### Cross-References
- `chebi_ids[]`: ChEBI identifiers
- `chembl_ids[]`: ChEMBL identifiers
- `hmdb_ids[]`: HMDB identifiers
- `clinical_trial_ids[]`: ClinicalTrials.gov IDs
- `pmids[]`: PubMed literature references
- `mesh_terms[]`: Medical Subject Headings

### Metadata
- `drug_names[]`: Drug-specific names
- `bioassay_count`: Number of associated bioassays
- `total_pmid_count`: Total literature citation count

## Cross-References

The parser creates bidirectional links:

1. **PubChem → ChEBI**: Via CHEBI accession mapping file
2. **ChEBI → PubChem**: Reverse link creation
3. **PubChem → ChEMBL**: Via compound synonyms
4. **ChEMBL → PubChem**: Reverse link creation
5. **PubChem → HMDB**: Via metabolite IDs
6. **HMDB → PubChem**: Reverse link creation

## Text Search

The following are indexed for text search:
- SMILES (canonical)
- InChI Key
- Synonyms (top 20)

## Running Tests

### Via Orchestrator (Recommended)
```bash
# Run from main directory
python tests/run_tests.py pubchem
```

### Standalone
```bash
cd tests/datasets/pubchem
# Start biobtree server first
../../../biobtree --out-dir /path/to/test_out web

# Run tests
python3 test_pubchem.py
```

## Expected Results

All tests should pass when:
1. Database built with PubChem P0 data (FDA drugs)
2. At least 20 test CIDs are present
3. Molecular properties correctly extracted
4. Cross-references created (if source datasets present)

## Troubleshooting

### No results for CID lookups
- Verify database was built with `--source=pubchem` or `--all`
- Check `pubchem_ids.txt` matches test data
- Ensure FDA drug mapping file was processed

### Missing molecular properties
- Verify SDF processing completed (Phase 2)
- Check mapping files loaded correctly
- Review parser logs for errors

### Missing cross-references
- Cross-references only appear if target datasets (ChEMBL, ChEBI, HMDB) are in the database
- Tests gracefully skip cross-reference validation if not present

### Text search not working
- Verify `addXref(..., textLinkID, ...)` calls in parser
- Check biobtree's text indexing is enabled
- Review parser logs for synonym/SMILES processing

## Future Enhancements

### Phase 2 (P1: Clinical Trials)
- Add clinical trial compound tests
- Verify clinical_trial_ids cross-references
- Test compound progression tracking

### Phase 3 (P2: Natural Products)
- Add natural product tests
- Verify taxonomy cross-references
- Test biosynthetic pathway links

### Phase 4 (P3: Bioassay)
- Add bioassay activity tests
- Verify target protein links
- Test activity value filtering

## Files

- `pubchem_ids.txt`: 20 FDA-approved drug CIDs
- `test_cases.json`: Declarative test cases
- `test_pubchem.py`: Custom Python tests
- `README.md`: This file
- `reference_data.json`: (Optional) Expected results from PubChem API
- `extract_reference_data.py`: (Optional) Script to fetch reference data

## Related Documentation

- [PUBCHEM_INTEGRATION_ANALYSIS_V2.md](../../../PUBCHEM_INTEGRATION_ANALYSIS_V2.md): Integration strategy
- [CHEMBL_INTEGRATION_ANALYSIS.md](../../../CHEMBL_INTEGRATION_ANALYSIS.md): ChEMBL comparison
- [src/update/pubchem.go](../../../src/update/pubchem.go): Parser implementation
- [conf/source.dataset.json](../../../conf/source.dataset.json): Configuration
