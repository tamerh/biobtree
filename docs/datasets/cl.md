# CL (Cell Ontology) Test Suite

This directory contains tests for the Cell Ontology (CL) dataset integration.

## Overview

CL is a structured vocabulary for cell types in animals, containing over 2,700 cell type classes. It provides high-level cell type classes as mapping points for cell type classes across species.

**Data Source**: http://purl.obolibrary.org/obo/cl.owl

## Test Structure

### Declarative Tests (`test_cases.json`)
Standard ontology tests:
- ID lookup
- Attribute validation
- Multi-ID batch lookup
- Invalid ID handling

### Custom Tests (`test_cl.py`)
CL-specific tests:
1. **Cell type with descriptive name** - Verify terms have meaningful names
2. **Terms with synonyms** - Check synonym extraction
3. **Text search by name** - Validate text search functionality
4. **Text search by synonym** - Test synonym-based lookup
5. **Hierarchical relationships** - Parent-child cell type navigation
6. **Cross-reference to Bgee** - Verify cell type → gene expression links

## Files

- `test_cl.py` - Main test script (6 custom tests)
- `test_cases.json` - Declarative test definitions (4 tests)
- `extract_reference_data.py` - Extracts reference data from CL OWL file
- `cl_ids.txt` - Test IDs (auto-generated during test build)
- `reference_data.json` - Reference data for validation (auto-generated)
- `README.md` - This file

## Running Tests

```bash
# From project root
./biobtree -d "cl" test
cd tests
python3 run_tests.py cl

# Or run all tests including CL
python3 run_tests.py
```

## Test Data

Test dataset includes ~100 random CL terms from the full ontology. The test build extracts a representative sample covering:
- Common cell types (e.g., monocyte, granulocyte, neuron)
- Cell lineages and differentiation states
- Tissue-specific cell types
- Stem cells and progenitor cells

## Integration

CL integrates with:
- **Bgee**: Cell type → gene expression data
- **UBERON**: Cell types in anatomical context (cells within tissues)
- **GO**: Cell type-specific biological processes

## Example Queries

```bash
# Lookup cell type
curl "localhost:9292/ws/map/?i=CL:0000576&mode=lite"  # monocyte

# Find genes expressed in monocytes
curl "localhost:9292/ws/map/?i=CL:0000576 >> bgee&mode=lite"

# Find genes expressed in granulocytes
curl "localhost:9292/ws/map/?i=CL:0000094 >> bgee&mode=lite"

# Search by cell type name
curl "localhost:9292/ws/map/?i=monocyte&mode=lite"

# Gene to cell types where expressed
curl "localhost:9292/ws/map/?i=ENSG00000139618 >> bgee >> cl&mode=lite"
```

## Notes

- CL uses hierarchical relationships (parent-child) for cell lineages
- Some cell types may have UBERON context (tissue localization)
- Not all CL terms will have Bgee links (depends on available expression data)
- Text search supports both official names and synonyms
