# PDB Dataset Tests

Tests for the Protein Data Bank (PDB) dataset integration in biobtree.

## Overview

PDB is the worldwide repository for 3D structural data of biological macromolecules. This test suite verifies that biobtree correctly processes PDB entries and their cross-references.

## Data Sources

- **Core metadata**: `entries.idx` from EBI FTP
- **Cross-references**: SIFTS CSV files (UniProt, GO, Pfam, InterPro, etc.)

## Test Files

| File | Purpose |
|------|---------|
| `pdb_ids.txt` | List of test PDB IDs (well-known structures) |
| `extract_reference_data.py` | Fetches reference data from RCSB PDB API |
| `reference_data.json` | Cached reference data (auto-generated) |
| `test_cases.json` | Declarative test definitions |
| `test_pdb.py` | Custom Python test methods |

## Running Tests

### Via Orchestrator (Recommended)

```bash
# From project root
python3 tests/run_tests.py pdb
```

### Standalone

```bash
# Generate reference data first
cd tests/datasets/pdb
python3 extract_reference_data.py

# Run tests (requires biobtree web server running)
python3 test_pdb.py
```

## Test Coverage

### Declarative Tests (test_cases.json)
- ID lookup
- Attribute presence check
- Multi-ID lookup
- Case-insensitive search
- Invalid ID handling

### Custom Tests (test_pdb.py)
- X-ray structure with resolution
- Cryo-EM structure search
- NMR structure search
- UniProt cross-references
- GO cross-references
- Multi-chain structures
- Title attribute
- Reverse mapping (UniProt -> PDB)

## Attributes Tested

| Attribute | Description |
|-----------|-------------|
| `method` | Experimental method (X-RAY DIFFRACTION, ELECTRON MICROSCOPY, NMR) |
| `resolution` | Resolution in Angstroms (when applicable) |
| `title` | Structure title/description |
| `header` | Classification category |
| `release_date` | Deposition date |
| `source_organism` | Source organism |
| `chain_count` | Number of polymer chains |

## Cross-References Tested

| Target | Description |
|--------|-------------|
| UniProt | Protein sequences |
| GO | Gene Ontology terms |
| InterPro | Protein families/domains |
| Pfam | Protein families |
| Taxonomy | Organism taxonomy |

## Example Queries

```bash
# Search structure
curl "http://localhost:9292/ws/?i=4HHB"

# PDB to UniProt
curl "http://localhost:9292/ws/map/?i=4HHB&m=>>pdb>>uniprot"

# UniProt to PDB (reverse)
curl "http://localhost:9292/ws/map/?i=P69905&m=>>uniprot>>pdb"

# Filter by resolution
curl "http://localhost:9292/ws/map/?i=P04637&m=>>uniprot>>pdb[pdb.resolution<2.0]"
```
