# HPO (Human Phenotype Ontology) Integration Test Suite

This directory contains comprehensive tests for HPO integration in biobtree.

## About HPO

The Human Phenotype Ontology (HPO) provides a standardized vocabulary of phenotypic abnormalities encountered in human disease. It contains:

- **16,000+ phenotype terms** - Standardized descriptions of clinical features
- **Gene-phenotype associations** - Links between genes and phenotypic abnormalities
- **Hierarchical structure** - Parent-child relationships between phenotype terms
- **Cross-references** - Links to OMIM, DECIPHER, Orphanet, etc.
- **Clinical use** - Used in 100+ organizations worldwide for diagnosis and research

## Data Sources

- **OBO file**: `http://purl.obolibrary.org/obo/hp.obo`
  - Phenotype terms with names, synonyms, definitions, and hierarchy
  - Updated monthly by the HPO Consortium

- **Gene associations**: `genes_to_phenotype.txt`
  - Gene-phenotype associations from HPO
  - Maps HGNC gene symbols to HPO phenotype IDs

## Test Coverage

### Declarative Tests (test_cases.json)

Common tests that run on all datasets:
- ✓ ID lookup verification
- ✓ Attribute presence checks
- ✓ Multiple ID batch lookups
- ✓ Invalid ID handling

### Custom Tests (test_hpo.py)

HPO-specific functionality tests:

1. **Phenotype Attributes** - Verify terms have names and synonyms
2. **Hierarchical Structure** - Test parent-child relationships
3. **Gene Associations** - Verify gene-phenotype mappings
4. **Text Search** - Search by phenotype names and synonyms
5. **Cross-References** - Validate external database links
6. **Hierarchy Navigation** - Test traversing parent-child relationships

## Running Tests

### Quick Test (minimal database)
```bash
cd /data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/biobtreev2
./biobtree -d hpo test
```

This will:
1. Build a minimal database with 100 HPO test entries
2. Save processed IDs to test_out/reference/hpo_ids.txt
3. Run validation tests

### Full Integration Test

From the tests directory:
```bash
cd tests
python3 run_tests.py hpo
```

This runs comprehensive tests including:
- Phenotype term lookups
- Parent/child hierarchy validation
- Gene-phenotype association checks
- Text search validation

## Test Data Extraction

### Extract Reference Data

After running `./biobtree -d hpo test`, extract reference data:

```bash
cd tests/hpo
cp ../../test_out/reference/hpo_ids.txt .
./extract_reference_data.py
```

This script:
1. Downloads HPO OBO file and genes_to_phenotype.txt (or uses cached versions)
2. Extracts complete data for all test IDs
3. Saves to `reference_data.json` with all fields preserved

### Files Generated

- `hpo_ids.txt` - List of processed HPO IDs from test build
- `hp.obo` - Cached HPO OBO file
- `genes_to_phenotype.txt` - Cached gene associations file
- `reference_data.json` - Complete reference data for validation

## Implementation Details

### Storage Model

HPO data is stored with:
- **Main dataset (hpo, ID 29)**: Phenotype terms with attributes
- **Derived datasets**:
  - `hpoparent` (ID 308): Parent relationship links
  - `hpochild` (ID 309): Child relationship links

### Data Processing (hpo.go)

**Phase 1: Parse hp.obo**
```go
// Extract phenotype terms with:
// - ID (HP:0000001)
// - Name
// - Synonyms
// - Parent relationships (is_a)
// - Type set to "phenotype"
```

**Phase 2: Parse genes_to_phenotype.txt**
```go
// Create bidirectional links:
// - Gene symbol → HPO term (via hgnc dataset)
// - HPO term → Gene symbol (via hpo dataset)
```

### Cross-References Created

1. **Text search**: Phenotype names and synonyms → HP IDs
2. **Gene associations**: Gene symbols ↔ HP IDs (bidirectional)
3. **Hierarchy**: Parent ↔ Child relationships

### Attributes Stored (OntologyAttr)

```json
{
  "type": "phenotype",
  "name": "Phenotype name",
  "synonyms": ["synonym1", "synonym2"]
}
```

Note: OntologyAttr does not store definitions (unlike the OBO file which has them).

## Example Queries

### Lookup Phenotype by ID
```bash
curl "http://localhost:9292/ws/entry/?i=HP:0000001&s=hpo"
```

### Search by Phenotype Name
```bash
curl "http://localhost:9292/ws/entry/?i=seizure&s=hpo"
```

### Find Gene Associations
```bash
# Find phenotypes for a gene
curl "http://localhost:9292/ws/entry/?i=BRCA1&s=hgnc" | jq '.results[].links.hpo'

# Find genes for a phenotype
curl "http://localhost:9292/ws/entry/?i=HP:0001250&s=hpo" | jq '.results[].links.hgnc'
```

### Navigate Hierarchy
```bash
# Find parent phenotypes
curl "http://localhost:9292/ws/entry/?i=HP:0001250&s=hpo" | jq '.results[].links.hpoparent'

# Find child phenotypes
curl "http://localhost:9292/ws/entry/?i=HP:0000118&s=hpo" | jq '.results[].links.hpochild'
```

## Test Validation

Tests verify:

✓ **Data completeness**: All 100 test phenotypes processed
✓ **Attribute accuracy**: Names and synonyms correctly extracted
✓ **Hierarchy integrity**: Parent-child relationships properly linked
✓ **Gene associations**: Gene-phenotype mappings bidirectional
✓ **Text search**: Phenotype names and synonyms searchable
✓ **Cross-references**: External database links preserved

## Data Statistics

From test build (100 terms):
- **Phenotype terms**: 100 HPO IDs
- **Parent relationships**: ~100-200 links
- **Gene associations**: Varies per phenotype
- **Synonyms**: Multiple per phenotype
- **Text search entries**: Name + all synonyms

## Troubleshooting

### Test IDs Not Found

If reference extraction fails to find IDs:
```bash
# Verify test build created IDs file
ls -la ../../test_out/reference/hpo_ids.txt

# Check IDs file content
head ../../test_out/reference/hpo_ids.txt
```

### Build Failures

Check for:
- HPO OBO file download issues
- genes_to_phenotype.txt access problems
- Disk space for test database

### Test Failures

Common issues:
- Cached data files out of date (delete hp.obo and genes_to_phenotype.txt)
- Database not built (run `./biobtree -d hpo test` first)
- Web server not running (tests start it automatically)

## License

HPO is licensed under CC BY 4.0 (commercial use allowed).

## References

- HPO Website: https://hpo.jax.org/
- HPO GitHub: https://github.com/obophenotype/human-phenotype-ontology
- HPO Paper: Köhler et al. (2021) The Human Phenotype Ontology in 2021
