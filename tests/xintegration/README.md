# Integration Tests

Tests real biological workflows against the biobtree web server.

## Usage

```bash
# Run all tests
python3 run_integration_tests.py --server http://localhost:9292

# Run specific category
python3 run_integration_tests.py --server http://localhost:9292 --category gene_mapping

# List categories
python3 run_integration_tests.py --list-categories

# Verbose output
python3 run_integration_tests.py --server http://localhost:9292 --verbose
```

## Test File

`integration_tests.json` contains test cases with:

```json
{
  "category": "gene_mapping",
  "name": "Gene to protein mapping",
  "query": ">>ensembl>>uniprot",
  "why": "Map gene symbols to UniProt proteins",
  "should_pass": ["BRCA1", "TP53"],
  "should_fail": ["INVALID_GENE"]
}
```

## Categories

- `gene_mapping` - Gene symbols to proteins, GO, transcripts
- `protein_features` - Domains, mutations, variants, structures
- `chembl_drug` - Compounds, targets, bioactivity
- `genomics` - Coordinates, exons, orthologs
- `pathways` - Reactome, STRING networks
- `ontologies` - GO, EFO, Taxonomy navigation
