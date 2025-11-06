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

   datasets/                    # Dataset-specific tests
      <dataset>/                # Per-dataset folder
          test_cases.json       # Declarative tests (JSON)
          test_<dataset>.py     # Custom tests + main
          extract_reference_data.py # Fetch from source API
          reference_data.json   # Complete API response data
          <dataset>_ids.txt     # Fixed test IDs
          README.md             # Dataset documentation
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
cd tests/datasets/<dataset>
cp ../../../test_out/reference/<dataset>_ids.txt .
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

**Helper Methods** (cross-reference validation):
- `runner.get_xrefs(result, "taxonomy")` - Get xrefs filtered by dataset name
- `runner.has_xref(result, "ensembl", "ENSG00000139618")` - Check if xref exists
- `runner.get_xref_count(result, "go")` - Count xrefs for dataset
- `runner.get_xref_datasets(result)` - List all dataset names with xrefs

Dataset names are case-insensitive and support aliases (dynamically loaded from config files).

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

**HMDB**: Static test file (`tests/datasets/hmdb/hmdb_test.zip`) due to zipstream limitations. Parser checks `path2` config in test mode.

**ChEMBL**: Smart tracking for sparse RDF data. Blank node filtering. Activity IDs need reformatting (CHEMBL_ACT_93229).

**ChEMBL Target**: Automatically includes chembl_target_component in build (dependency in run_tests.py)

## Dataset README Guidelines

Each dataset directory should include a `README.md` following a **standardized format** for consistency. The README should be concise and focus on dataset-specific information.

### Standard README Format

All dataset READMEs should follow this structure:

```markdown
# {Dataset Name} Dataset

## Overview
Brief description (2-3 sentences) covering:
- What the dataset is and its purpose
- Key statistics (total entries, coverage)
- Scientific importance and main applications

**Source**: Data source URL or organization
**Data Type**: Brief description of what data it contains

## Integration Architecture

### Storage Model
**Primary Entries**: ID format and storage approach
**Searchable Text Links**: What text searches are indexed
**Attributes Stored**: What's in the protobuf attributes
**Cross-References**: What other datasets link to/from this one

### Special Features
Unique aspects of this dataset's integration:
- Novel storage patterns
- Special indexing
- Bidirectional links
- Text search capabilities
- Hierarchical relationships

## Use Cases

6 biological/scientific scenarios (not query examples):
**1. {Use Case Name}**
```
Query: {Scientific question} → {What you query} → {Result}
Use: {Real-world application}
```

## Test Cases

**Current Tests** (N total):
- X declarative tests (list types)
- Y custom tests (list what they test)

**Coverage**:
- ✅ What's tested
- ✅ What's validated

**Recommended Additions**:
- Future test ideas
- Missing coverage areas

## Performance

- **Test Build**: ~Xs (N entries)
- **Data Source**: Where data comes from
- **Update Frequency**: How often updated
- **Total Entries**: Size of full dataset
- **Special notes**: Large file sizes, slow downloads, etc.

## Known Limitations

Dataset-specific issues:
- What doesn't work
- What's not stored
- Workarounds needed
- API issues

## Future Work

- Potential enhancements
- Missing features
- Integration improvements
- Test additions

## Maintenance

- **Release Schedule**: Update frequency
- **Data Format**: File format details
- **Test Data**: How many test entries
- **License**: Usage terms
- **Special notes**: Version info, coordination with other resources

## References

- **Citation**: Primary publication
- **Website**: Official URL
- **License**: Terms of use
```

### What to Include (Dataset-Specific Only)

✅ **Overview**: Brief description (2-3 sentences max) of dataset and data source
✅ **Integration Architecture**: Storage model, cross-references, special features (CRITICAL - this is the core unique content)
✅ **Use Cases**: 6 biological/scientific scenarios enabled by this dataset (NOT generic curl examples)
✅ **Test Cases**: Current tests (declarative + custom) and recommended additions
✅ **Performance**: Test build time, data source, update frequency, dataset size
✅ **Known Limitations**: Dataset-specific issues, disabled features, workarounds
✅ **Future Work**: Potential enhancements specific to this dataset
✅ **Maintenance**: Release frequency, data format, test data size, license
✅ **References**: Citation, website, license (concise - no detailed links)

### What to Exclude (Common to All Datasets)

❌ **Files section**: Standard structure documented in tests/README.md
❌ **Dataset statistics**: Detailed file listings, test entry counts
❌ **Extracting reference data**: Common workflow documented in tests/README.md
❌ **Reference data format**: Available in reference_data.json file
❌ **Building with test data**: Common commands documented in tests/README.md
❌ **Query examples**: Replace with biological use cases showing scientific applications
❌ **Detailed API documentation**: Just the essentials
❌ **Maintenance procedures**: Common workflow unless dataset-specific
❌ **Overly detailed explanations**: Keep it concise and focused

### Example READMEs

**Good examples following standard format**:
- `tests/hpo/README.md` - Ontology with hierarchies and gene associations
- `tests/hmdb/README.md` - Metabolomics database with chemical properties
- `tests/interpro/README.md` - Protein signatures with member database integration
- `tests/mondo/README.md` - Disease ontology with cross-database mappings
- `tests/efo/README.md` - Experimental factors ontology with text search
- `tests/eco/README.md` - Evidence codes ontology
- `tests/alphafold/README.md` - Protein structures with confidence scores

**Key Principles**:
1. **Consistency**: All READMEs follow same structure and section order
2. **Conciseness**: 150-250 lines maximum, focus on unique aspects
3. **Scientific context**: Use cases show real biological applications, not just queries
4. **Integration focus**: Emphasize how data is stored, linked, and searched in biobtree
5. **Avoid repetition**: Don't duplicate information already in tests/README.md

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
