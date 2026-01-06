# MSigDB Test Suite

Tests for the MSigDB (Molecular Signatures Database) dataset integration.

## Overview

MSigDB is a collection of annotated gene sets for use with GSEA (Gene Set Enrichment Analysis). This integration parses the MSigDB SQLite database and creates cross-references to:
- **HGNC**: Gene symbols contained in each gene set
- **PubMed**: Publications associated with gene sets
- **GO**: Gene Ontology terms linked to gene sets
- **HPO**: Human Phenotype Ontology terms linked to gene sets

## Test Coverage

### Attribute Tests
- `test_entry_with_standard_name` - Verify standard names (e.g., HALLMARK_APOPTOSIS)
- `test_entry_with_collection` - Verify collection classification (H, C1-C8)
- `test_entry_with_gene_symbols` - Verify gene symbol lists
- `test_entry_with_description` - Verify gene set descriptions
- `test_entry_with_go_terms` - Verify GO term associations (when present)
- `test_entry_with_hpo_terms` - Verify HPO term associations (when present)
- `test_entry_with_pmid` - Verify PubMed ID associations

### Search Tests
- `test_text_search_by_standard_name` - Text search by gene set name
- `test_hallmark_collection` - Hallmark (H) collection entries

### Cross-Reference Tests
- `test_mapping_hgnc_to_msigdb` - Gene symbol → gene sets mapping
- `test_mapping_msigdb_to_pubmed` - Gene set → publication mapping
- `test_mapping_msigdb_to_go` - Gene set → GO term mapping
- `test_mapping_msigdb_to_hpo` - Gene set → HPO term mapping

### CEL Filter Tests
- `test_cel_filter_collection` - Filter by collection (e.g., `msigdb.collection=='H'`)

## Files

- `test_cases.json` - Declarative test cases
- `test_msigdb.py` - Test runner with custom tests
- `extract_reference_data.py` - Extract reference data from index files
- `reference_data.json` - Generated reference data (after test build)

## Running Tests

```bash
# Run all tests
python3 tests/run_tests.py msigdb

# Or run directly
cd tests/datasets/msigdb
python3 extract_reference_data.py  # Generate reference data
python3 test_msigdb.py             # Run tests
```

## Known Limitations

1. **GO/HPO terms may be sparse**: Not all gene sets have external term associations in the SQLite database. Tests for these handle missing data gracefully.

2. **Collection filtering**: The collection field contains the main collection (H, C1-C8), and sub_collection contains the sub-classification (e.g., CGP, MIR, TFT).

3. **Gene symbols only**: The integration uses human gene symbols. Entrez Gene IDs are not directly indexed but can be reached via HGNC → Entrez mapping.
