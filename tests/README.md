# Biobtree Testing System

**Version:** 2.0
**Last Updated:** 2025-11-01
**Status:** Production Ready

---

## ⚠️ HOW TO RUN TESTS ⚠️

**CRITICAL**: Tests MUST be run via the orchestrator from the **root biobtree directory** (biobtreev2/)

```bash
# ✓ CORRECT - Always run from biobtreev2/ (root directory)
cd /path/to/biobtreev2
python3 tests/run_tests.py <dataset>

# Examples:
python3 tests/run_tests.py ensembl
python3 tests/run_tests.py hmdb,go,taxonomy
python3 tests/run_tests.py all

# ✗ WRONG - Never run from tests/ directory
cd tests && python3 run_tests.py <dataset>  # WILL FAIL

# ✗ WRONG - Never run test scripts directly
cd tests/ensembl && python3 test_ensembl.py  # WILL FAIL
```

**Why?** The orchestrator (`tests/run_tests.py`) handles:
- Building test database with correct parameters
- Starting/stopping web server
- Setting correct working directory
- Cleaning up processes

**Always use the orchestrator from the root directory!**

---

This directory contains the testing infrastructure for biobtree datasets.

## Test Coverage

- **28 Datasets**: HGNC, UniProt, GO, Taxonomy, UniParc, UniRef100, UniRef50, UniRef90, ECO, EFO, ChEBI, InterPro, HMDB, ChEMBL Document, ChEMBL Molecule, ChEMBL Activity, ChEMBL Assay, ChEMBL Target (with Target Component), ChEMBL Cell Line, Ensembl, Ensembl Bacteria, Ensembl Fungi, Ensembl Metazoa, Ensembl Plants, Ensembl Protists, MONDO, Patent, Clinical Trials
- **308 Total Tests**: 185 declarative (JSON) + 123 custom (Python)
- **9 Test Types**: ID lookup, symbol lookup, name lookup, alias lookup, cross-references, attribute checks, multi-lookup, case-insensitive, invalid ID handling

### Dataset-Specific Notes

**ChEBI**: Only cross-references stored (not primary entries)

**HMDB**: Uses static test file (`tests/hmdb/hmdb_test.zip`) due to zipstream limitations. Fast testing (~330ms) with 20 pre-extracted metabolites.

**ChEMBL**: All 6 datasets (document, molecule, activity, assay, target, cell_line) use smart tracking for sparse RDF data. Test builds: 10-35 seconds for 20 entries. Target component data validated through target tests (API embeds components in target endpoint).
- chembl_document: 10 tests (journal articles, patents, books)
- chembl_molecule: 10 tests (small molecules, compounds)
- chembl_activity: 10 tests (bioactivity measurements)
- chembl_assay: 10 tests (screening assays, protocols)
- chembl_target: 14 tests (drug targets + component validation)
- chembl_cell_line: 10 tests (cell line information)

**Ensembl**: Six divisions (ensembl, ensembl_bacteria, ensembl_fungi, ensembl_metazoa, ensembl_plants, ensembl_protists) with genome-specific test data. Uses `--genome-taxids` parameter to select 1 species per division (20 genes each). Test builds: ~4.7s for all 6 divisions. Each division: 7 tests (4 declarative + 3 custom).
- Test taxids: 9606 (human), 1268975 (E. coli), 330879 (A. fumigatus), 7227 (D. melanogaster), 3702 (A. thaliana), 36329 (P. falciparum)
- All divisions built together when any one is selected (shared genome infrastructure)
- **Limitation**: Ensembl Genomes API (rest.ensemblgenomes.org) has SSL certificate issues - reference data extraction only works for main Ensembl division (rest.ensembl.org). Test IDs generated from genome files instead.

**MONDO**: Uses OBO file parsing for reference data extraction (more reliable than EBI OLS API). 100 test disease terms covering disease hierarchies, synonyms, cross-references, and text search. 10 tests (4 declarative + 6 custom). Test builds: ~2.7s.

**Patent**: Uses local SureChEMBL data files (10 patents, 50 compounds, 100 mappings). Tests validate patent metadata (title, country, publication date), classification codes (IPC/CPC), assignees, patent families, and patent-compound relationships. 16 tests (9 declarative + 7 custom). Test builds: ~2.3s. Note: InChI Key/SMILES searchability requires ChEMBL integration.

**Clinical Trials**: Uses local test data (10 trials from test_data/clinical_trials/). Tests validate trial metadata (title, phase, status, study_type), interventions (drugs, therapies), medical conditions, and MONDO disease mappings. 16 tests (9 declarative + 7 custom). Test builds: ~2.3s. Source: Local JSON file with complete trial data including eligibility, outcomes, and facilities. Note: ChEMBL drug molecule cross-references require ChEMBL dataset integration.

## Philosophy

**Integration testing, not unit tests**:
- Build real database with limited data (20-100 entries)
- Start real web server (port 9292)
- Execute real queries against live service
- Validate actual results

Benefits: Deterministic, fast (3-5 min cycle), tests full stack, practical validation

## Directory Structure

```
tests/
   run_tests.py                 # Main orchestrator (run from parent dir)
   README.md                    # This file
   common/                      # Reusable framework
      __init__.py
      test_runner.py            # TestRunner class
      test_types.py             # Common test patterns
      query_helpers.py          # Query helper methods

   <dataset>/                   # Per-dataset tests
       test_cases.json          # Declarative tests (JSON)
       test_<dataset>.py        # Custom tests + main
       extract_reference_data.py # Fetch from source API
       reference_data.json      # Complete API response data
       <dataset>_ids.txt        # Fixed test IDs
```

## Test Workflow

### 1. Add Test Limit to Config
Edit `conf/source.dataset.json` and add `test_entries_count` (100 for most, 10 for large datasets, 20 for ChEMBL)

### 2. Build Test Database
```bash
./biobtree -d "<dataset>" test
```
Creates database in `test_out/db/` and logs IDs to `test_out/reference/<dataset>_ids.txt`

### 3. Extract Reference Data
Copy IDs to test directory and run extraction script:
```bash
cd tests/<dataset>
cp ../../test_out/reference/<dataset>_ids.txt .
python3 extract_reference_data.py
```
**Important**: Always fetch COMPLETE API response (not selective fields) to preserve all data for future tests

### 4. Create Test Files
- `test_cases.json`: Declarative tests (common patterns)
- `test_<dataset>.py`: Custom Python tests (dataset-specific logic)

### 5. Add to Orchestrator
Edit `tests/run_tests.py`:
- Add to help text
- Add to `all_datasets` dictionary
- Add dependency handling if needed (e.g., chembl_target auto-includes chembl_target_component)

### 6. Run Tests
```bash
python3 tests/run_tests.py <dataset>
```
Orchestrator handles everything: database build, server start/stop, test execution

## Test Types

### Declarative (JSON)
Common patterns defined in `test_cases.json`:
- `id_lookup`: Lookup by primary ID
- `symbol_lookup`: Lookup by symbol/name
- `xref_exists`: Check cross-reference exists
- `attribute_check`: Verify attributes present
- `multi_lookup`: Test multiple IDs
- `case_insensitive`: Case-insensitive search
- `invalid_id`: Invalid ID handling
- `lookup`: Basic lookup test
- `attributes`: Check entry has data

### Custom (Python)
Dataset-specific tests using `@test` decorator. Full access to TestRunner API for complex logic.

### Reference Value Resolution
Use `@reference[index].field` in JSON tests to reference `reference_data.json` values

## API Source Examples

Extract reference data from official APIs:
- **HGNC**: `https://rest.genenames.org/fetch/hgnc_id/{id}`
- **UniProt**: `https://rest.uniprot.org/uniprotkb/{id}`
- **GO**: `https://www.ebi.ac.uk/QuickGO/services/ontology/go/terms/{id}`
- **ECO/EFO**: `https://www.ebi.ac.uk/ols/api/ontologies/{ont}/terms/{encoded_iri}`
- **ChEMBL**: `https://www.ebi.ac.uk/chembl/api/data/{type}/{id}.json`
  - Types: document, molecule, activity, assay, target, cell_line
  - Note: target_component embedded in target endpoint

## Configuration

**Test Limits** (`conf/source.dataset.json`):
- Standard: `"test_entries_count": "100"`
- Large datasets: `"test_entries_count": "10"` (UniParc, UniRef)
- ChEMBL: `"test_entries_count": "20"`

**Test Mode Support**:
Most parsers support test mode automatically. Add for new datasets:
- Track processed IDs on first appearance
- Filter blank nodes (containing "#")
- Early exit when limit reached
- Log IDs to test reference directory

## Best Practices

1. **Keep test IDs fixed**: Don't regenerate unless source data changes
2. **Extract complete API responses**: Preserve all fields for future tests
3. **Use declarative tests first**: Only write custom Python when needed
4. **Test cross-references**: Critical for biobtree value
5. **Fast feedback**: Tests should complete in seconds
6. **Run via orchestrator**: `python3 tests/run_tests.py <dataset>` from parent directory

## Special Cases

**HMDB**: Static test file due to zipstream limitations. Parser checks `path2` config in test mode.

**ChEMBL**: Smart tracking for sparse RDF data. Blank node filtering. Activity IDs need reformatting (CHEMBL_ACT_93229).

**ChEMBL Target**: Automatically includes chembl_target_component in build (dependency in run_tests.py)

## Adding New Datasets

1. Add `test_entries_count` to `conf/source.dataset.json`
2. Add test mode support to parser if needed
3. Build test database: `./biobtree -d "<dataset>" test`
4. Create test directory and files
5. Extract reference data from official API
6. Write declarative and custom tests
7. Add to `tests/run_tests.py` orchestrator
8. Run: `python3 tests/run_tests.py <dataset>`

## Reference

This document serves as a quick reference for adding and maintaining dataset tests. Focus on key points rather than detailed examples. The test framework is designed for simplicity and maintainability.
