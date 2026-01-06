# DrugCentral Dataset Tests

## Overview
DrugCentral is a comprehensive drug database that provides information on active pharmaceutical ingredients, drug-target interactions, and pharmacological data. This test suite validates the biobtree integration.

## Data Source
- **Website**: https://drugcentral.org/
- **Download**: https://unmtid-dbs.net/download/DrugCentral/
- **Format**: TSV (Tab-Separated Values)
- **Key Files**:
  - `drug.target.interaction.tsv.gz` - Drug-target interactions with activity data
  - `structures.smiles.tsv` - Chemical structure information (SMILES, InChI, CAS RN)

## Primary Identifier
- **STRUCT_ID**: DrugCentral's unique numeric structure identifier (e.g., "89", "720", "1594")

## Attributes Indexed
- `struct_id` - DrugCentral structure ID
- `drug_name` - Primary drug name
- `inn_name` - International Nonproprietary Name
- `cas_rn` - CAS Registry Number
- `smiles` - SMILES structure representation
- `inchi` - InChI string
- `inchi_key` - InChI Key (searchable)
- `targets` - Array of target interactions
- `target_count` - Number of unique targets
- `action_types` - Aggregated action types (e.g., BLOCKER, AGONIST, INHIBITOR)
- `target_classes` - Aggregated target classes (e.g., GPCR, Ion channel, Enzyme)
- `organisms` - Aggregated target organisms

### Target Interaction Attributes
Each target in the `targets` array contains:
- `target_name` - Target protein name
- `target_class` - Target classification
- `uniprot_accession` - UniProt accession(s)
- `gene_symbol` - Gene symbol
- `swissprot_entry` - SwissProt entry name
- `act_value` - Activity value (e.g., IC50, Ki)
- `act_unit` - Activity unit (e.g., nM)
- `act_type` - Activity type (e.g., IC50, Ki, EC50)
- `act_comment` - Activity description
- `act_source` - Data source (e.g., CHEMBL, DRUG LABEL)
- `has_moa` - True if mechanism of action
- `action_type` - Mechanism action type
- `tdl` - Target Development Level (Tclin, Tchem, Tbio, Tdark)
- `organism` - Target organism

## Cross-References Created
- **DrugCentral -> UniProt**: Via target UniProt accessions
- **Text search**: Drug names, INN names, CAS RN, InChI Keys, target names, gene symbols

## Sample Queries

### Basic lookup by STRUCT_ID
```
curl "http://localhost:9292/ws/?i=89"
```

### Text search by drug name
```
curl "http://localhost:9292/ws/?i=adenine"
```

### Text search by InChI Key
```
curl "http://localhost:9292/ws/?i=GFFGJBXGBJISGV-UHFFFAOYSA-N"
```

### Mapping to UniProt
```
curl "http://localhost:9292/ws/map/?i=89&m=>>drugcentral>>uniprot"
```

## Running Tests

### Prerequisites
1. Build the test database:
```bash
./biobtree -d drugcentral test
```

2. Extract reference data:
```bash
cd tests/datasets/drugcentral
python3 extract_reference_data.py
```

3. Start the web server:
```bash
./biobtree --out-dir test_out web &
```

### Run tests
```bash
# From biobtree root directory
python3 tests/run_tests.py drugcentral

# Or run individual test file
cd tests/datasets/drugcentral
python3 test_drugcentral.py
```

## Known Limitations

1. **Data Version**: Currently using 2021_09_01 release from DrugCentral
   - Newer releases may be available at https://unmtid-dbs.net/download/DrugCentral/

2. **Multiple UniProt IDs**: Some targets have pipe-separated UniProt accessions
   - All accessions are indexed for cross-referencing
   - TDL values may be duplicated when multiple accessions exist

3. **Activity Data**: Not all drug-target interactions have activity values
   - Some entries only have mechanism of action (MOA) data

4. **Structure Data**: Some drugs may lack SMILES/InChI data
   - Structures file is loaded separately from interactions file
