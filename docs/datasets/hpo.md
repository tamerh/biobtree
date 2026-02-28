# HPO (Human Phenotype Ontology) Integration Test Suite

This directory contains comprehensive tests for HPO integration in biobtree.

## About HPO

The Human Phenotype Ontology (HPO) provides a standardized vocabulary of phenotypic abnormalities encountered in human disease. It contains:

- **18,000+ phenotype terms** - Standardized descriptions of clinical features
- **280,000+ disease-phenotype annotations** - From phenotype.hpoa file
- **Gene-phenotype associations** - Links between genes and phenotypic abnormalities
- **Hierarchical structure** - Parent-child relationships between phenotype terms
- **Evidence-based annotations** - Evidence codes (PCS, TAS, IEA) and frequency data
- **Clinical use** - Used in 100+ organizations worldwide for diagnosis and research

## Data Sources

Biobtree integrates three HPO data files:

### 1. OBO Ontology File (`hp-base.obo`)
- Phenotype terms with names, synonyms, definitions
- Hierarchical parent-child relationships
- Updated monthly by the HPO Consortium
- Source: `https://github.com/obophenotype/human-phenotype-ontology/releases/`

### 2. Gene Associations (`genes_to_phenotype.txt`)
- Gene-phenotype associations from HPO
- Maps HGNC gene symbols to HPO phenotype IDs
- Enables gene → phenotype queries

### 3. Disease Annotations (`phenotype.hpoa`) **NEW**
- **~280,000 disease-phenotype annotations**
- Links HPO terms to diseases in OMIM and Orphanet
- Includes rich metadata:
  - **Evidence codes**: PCS (published clinical study), TAS (traceable author statement), IEA (inferred)
  - **Frequency**: How often the phenotype occurs (e.g., "3/8" = 37.5% of patients)
  - **Onset**: Age of onset (HP term)
  - **Sex specificity**: Male/female-specific phenotypes
  - **References**: PMID citations

## Cross-References Created

| Source | Target | Count | Notes |
|--------|--------|-------|-------|
| HPO | OMIM (mim) | ~160K | Disease-phenotype with evidence |
| HPO | Orphanet | ~116K | Disease-phenotype with evidence |
| HPO | Ensembl | ~180K | Gene-phenotype associations |
| HPO | HPO Parent | ~18K | Ontology hierarchy |
| HPO | HPO Child | ~18K | Ontology hierarchy |

## Attributes Stored (HPOAttr)

HPO uses a dedicated `HPOAttr` protobuf message (not generic OntologyAttr):

```protobuf
message HPOAttr {
  string name = 1;              // "Seizure"
  repeated string synonyms = 2; // ["Epileptic seizure", "Seizures", ...]
  string definition = 3;        // Term definition

  // Disease associations (for future queryable embedded data)
  repeated DiseaseAssoc diseases = 4;

  message DiseaseAssoc {
    string id = 1;             // "619340" or "Orphanet:558"
    string dataset = 2;        // "mim", "orphanet"
    string name = 3;           // Disease name
    string evidence = 4;       // "PCS", "TAS", "IEA"
    double frequency = 5;      // 0.375 for "3/8"
    string frequency_raw = 6;  // "3/8"
    string onset = 7;          // "HP:0003581"
    string sex = 8;            // "MALE", "FEMALE"
    string reference = 9;      // "PMID:31675180"
  }
}
```

## Example Queries

### Basic Phenotype Lookup
```bash
# Get phenotype entry
curl "http://localhost:9292/ws/entry/?i=HP:0001250&s=hpo"

# Search by phenotype name
curl "http://localhost:9292/ws/?i=seizure&mode=lite"
```

### Phenotype → Disease Mappings
```bash
# Find OMIM diseases associated with a phenotype
curl "http://localhost:9292/ws/map/?i=HP:0001250&m=>>hpo>>mim&mode=lite"

# Find Orphanet diseases
curl "http://localhost:9292/ws/map/?i=HP:0001250&m=>>hpo>>orphanet&mode=lite"
```

### Disease → Phenotype Mappings (Reverse)
```bash
# Find phenotypes for Marfan syndrome (OMIM)
curl "http://localhost:9292/ws/map/?i=154700&m=>>mim>>hpo&mode=lite"

# Find phenotypes for an Orphanet disease
curl "http://localhost:9292/ws/map/?i=Orphanet:558&m=>>orphanet>>hpo&mode=lite"
```

### Gene-Phenotype Associations
```bash
# Find phenotypes associated with a gene
curl "http://localhost:9292/ws/map/?i=BRCA1&m=>>ensembl>>hpo&mode=lite"

# Find genes associated with a phenotype
curl "http://localhost:9292/ws/map/?i=HP:0001250&m=>>hpo>>ensembl&mode=lite"
```

### Hierarchy Navigation
```bash
# Find parent phenotypes
curl "http://localhost:9292/ws/map/?i=HP:0001250&m=>>hpo>>hpoparent&mode=lite"

# Find child phenotypes
curl "http://localhost:9292/ws/map/?i=HP:0001250&m=>>hpo>>hpochild&mode=lite"

# Traverse multiple levels
curl "http://localhost:9292/ws/map/?i=HP:0001250&m=>>hpo>>hpoparent>>hpoparent&mode=lite"
```

### Filter by Attributes
```bash
# Filter HPO terms by name pattern
curl "http://localhost:9292/ws/map/?i=HP:0001250&m=>>hpo[name.contains(\"Seizure\")]&mode=lite"
```

## Test Coverage

### Declarative Tests (test_cases.json)

Common tests that run on all datasets:
- ID lookup verification
- Attribute presence checks
- Multiple ID batch lookups
- Invalid ID handling

### Custom Tests (test_hpo.py)

HPO-specific functionality tests:

1. **Phenotype Attributes** - Verify terms have names and synonyms
2. **Hierarchical Structure** - Test parent-child relationships
3. **Gene Associations** - Verify gene-phenotype mappings
4. **Disease Associations** - Verify HPO → OMIM/Orphanet links
5. **Text Search** - Search by phenotype names and synonyms
6. **Evidence Codes** - Verify PCS/TAS/IEA annotations preserved
7. **Reverse Mappings** - Disease → HPO lookups

## Running Tests

### Quick Test (minimal database)
```bash
cd /path/to/biobtreev2
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

## Implementation Details

### Data Processing (hpo.go)

**Phase 1: Parse hp-base.obo**
- Extract phenotype terms (HP:XXXXXXX)
- Parse names, synonyms, definitions
- Build parent-child hierarchy via `is_a` relationships
- Create text search entries for names and synonyms

**Phase 2: Parse genes_to_phenotype.txt**
- Create bidirectional gene ↔ phenotype links
- Link via Ensembl gene IDs

**Phase 3: Parse phenotype.hpoa** (NEW)
- Parse ~280K disease-phenotype annotations
- Create HPO ↔ OMIM cross-references
- Create HPO ↔ Orphanet cross-references
- Preserve evidence codes and frequency in xref evidence field

### Storage Model

- **Main dataset (hpo, ID 74)**: Phenotype terms with HPOAttr
- **Derived datasets**:
  - `hpoparent` (ID 89): Parent relationship links
  - `hpochild` (ID 90): Child relationship links

## Known Limitations

1. **MONDO not in phenotype.hpoa**: The phenotype.hpoa file only contains OMIM, Orphanet, and DECIPHER disease IDs - not MONDO. To get HPO→MONDO links, chain through OMIM:
   ```
   HP:0001250 >> hpo >> mim >> mondo
   ```

2. **DECIPHER skipped**: DECIPHER disease annotations (~47 diseases) are present in phenotype.hpoa but not integrated (DECIPHER dataset not in biobtree).

3. **Embedded diseases field**: The `HPOAttr.diseases` repeated field is defined but currently populated via cross-references rather than embedded data. This allows the xref-based query system to work while enabling future enhancement for fully embedded queryable disease data.

4. **Frequency as string**: Frequency data is stored in evidence string format (e.g., "PCS;freq=3/8") rather than as a queryable numeric field. Future enhancement could parse and store as double for numeric filtering.

## Data Statistics

Full build statistics:
- **Phenotype terms**: ~18,000 HPO IDs
- **Disease annotations**: ~280,000 (OMIM + Orphanet)
- **Gene associations**: ~180,000 links
- **Parent relationships**: ~18,000 links
- **Text search entries**: ~50,000+ (names + synonyms)

## Evidence Code Reference

| Code | Meaning | Quality |
|------|---------|---------|
| PCS | Published Clinical Study | Highest - from peer-reviewed literature |
| TAS | Traceable Author Statement | Medium - author-attributed |
| IEA | Inferred from Electronic Annotation | Lower - computationally derived |

## Troubleshooting

### No Disease Mappings Found

If `HP:XXXXX >> hpo >> mim` returns empty:
1. Verify phenotype.hpoa was downloaded during build
2. Check the HPO term exists and is not obsolete
3. Not all phenotypes have disease associations

### Hierarchy Not Working

If parent/child queries fail:
1. Verify hpoparent/hpochild datasets were generated
2. Check the term has parent relationships in OBO file
3. Root term (HP:0000001) has no parents

### Gene Associations Missing

If gene-phenotype queries return empty:
1. Verify genes_to_phenotype.txt was processed
2. Check gene symbol matches HGNC nomenclature
3. Not all genes have HPO annotations

## References

- HPO Website: https://hpo.jax.org/
- HPO GitHub: https://github.com/obophenotype/human-phenotype-ontology
- phenotype.hpoa format: https://obophenotype.github.io/human-phenotype-ontology/annotations/phenotype_hpoa/
- HPO Paper: Köhler et al. (2021) The Human Phenotype Ontology in 2021

## License

HPO is licensed under CC BY 4.0 (commercial use allowed with attribution).
