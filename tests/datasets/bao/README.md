# BAO (BioAssay Ontology) Dataset Tests

## Overview
Tests for the BioAssay Ontology (BAO) dataset integration in biobtree.

## Data Source
- **Source**: BioAssay Ontology GitHub repository
- **URL**: https://raw.githubusercontent.com/BioAssayOntology/BAO/master/bao_complete_merged.owl
- **Format**: OWL/RDF-XML
- **License**: CC BY-SA 4.0

## Dataset Characteristics
- **ID Format**: `BAO:XXXXXXX` (e.g., `BAO:0000001`)
- **ID Count**: ~2,700+ BAO terms
- **Attributes**: name, synonyms, type (namespace)
- **Relationships**: parent/child hierarchy via rdfs:subClassOf

## Test Coverage
1. **ID Lookup**: Direct lookup by BAO ID
2. **Text Search**: Search by term name and synonyms
3. **Hierarchy Navigation**: Navigate parent/child relationships
4. **Attribute Validation**: Verify name, synonyms are present

## Running Tests
```bash
# From project root
python3 tests/run_tests.py bao

# Or with verbose output
python3 tests/run_tests.py bao -v
```

## Known Limitations
1. **Synonym Newlines**: Some BAO entries contain multiple synonyms separated by newlines in a single field. The parser splits these into separate synonyms.

2. **External References**: BAO terms may reference external ontologies (CHEBI, CL, etc.) but cross-references to these are not currently extracted.

3. **No Definition Field**: Unlike some other ontologies, definitions (obo:IAO_0000115) are not extracted - only names and synonyms.

## Sample Queries

```bash
# Lookup a BAO term
curl "http://localhost:9292/ws/?i=BAO:0000001"

# Text search
curl "http://localhost:9292/ws/?i=FRET"
curl "http://localhost:9292/ws/?i=fluorescence"

# Navigate to parent terms
curl "http://localhost:9292/ws/map/?i=BAO:0000001&m=bao>>baoparent"

# Navigate to child terms
curl "http://localhost:9292/ws/map/?i=BAO:0000001&m=bao>>baochild"
```

## Files
- `test_cases.json` - Declarative test definitions
- `test_bao.py` - Custom Python tests
- `reference_data.json` - Sample BAO entries for validation
