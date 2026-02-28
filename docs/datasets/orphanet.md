# Orphanet (Rare Disease Database) Integration Test Suite

This directory contains comprehensive tests for Orphanet integration in biobtree.

## About Orphanet

Orphanet is a unique resource for rare diseases, providing:

- **11,500+ rare disease entries** - Standardized nomenclature and classification
- **HPO phenotype associations** - Disease-phenotype links with frequency data
- **Gene associations** - Disease-gene links with association types
- **Cross-references** - Links to OMIM, MONDO, MeSH, and other databases
- **Clinical use** - Reference database for rare disease diagnosis and research

## Data Sources

Biobtree integrates three Orphadata XML files (CC BY 4.0 license):

### 1. Product 1 (`en_product1.xml`)
- **~11,456 disorders** with names, synonyms, definitions
- Cross-references to OMIM, MONDO, MeSH
- Disorder types (Disease, Group of disorders, etc.)
- Source: https://www.orphadata.com/data/xml/en_product1.xml

### 2. Product 4 (`en_product4.xml`)
- **~4,337 disorders** with HPO phenotype associations
- Frequency data (Obligate, Very frequent, Frequent, Occasional, Very rare)
- HPO term names included
- Source: https://www.orphadata.com/data/xml/en_product4.xml

### 3. Product 6 (`en_product6.xml`)
- **~4,128 disorders** with gene associations
- Ensembl and HGNC gene identifiers
- Association types (Disease-causing, Modifying, etc.)
- PMID references for evidence
- Source: https://www.orphadata.com/data/xml/en_product6.xml

## Cross-References Created

| Source | Target | Description |
|--------|--------|-------------|
| Orphanet | HPO | Phenotype associations with frequency |
| Orphanet | OMIM | Disease mapping from ExternalReference |
| Orphanet | MONDO | Ontology mapping from ExternalReference |
| Orphanet | MeSH | Medical subject heading mapping |
| Orphanet | Ensembl | Gene associations |
| Orphanet | HGNC | Gene symbol mapping |

## Attributes Stored (OrphanetAttr)

```protobuf
message OrphanetAttr {
  string name = 1;              // "Marfan syndrome"
  repeated string synonyms = 2; // ["MFS", ...]
  string definition = 3;        // Disease description
  string disorder_type = 4;     // "Disease", "Group of disorders", etc.
  repeated PhenotypeAssociation phenotypes = 5;
  int32 gene_count = 6;         // Number of associated genes
  int32 phenotype_count = 7;    // Number of associated phenotypes

  message PhenotypeAssociation {
    string hpo_id = 1;          // "HP:0001519"
    string hpo_term = 2;        // "Disproportionate tall stature"
    string frequency = 3;       // "Very frequent (99-80%)"
    double frequency_value = 4; // 0.895
  }
}
```

## Frequency Values

| Orphanet Frequency | Numeric Value |
|-------------------|---------------|
| Obligate (100%) | 1.0 |
| Very frequent (99-80%) | 0.895 |
| Frequent (79-30%) | 0.545 |
| Occasional (29-5%) | 0.17 |
| Very rare (<4-1%) | 0.025 |
| Excluded (0%) | 0.0 |

## Example Queries

### Basic Disorder Lookup
```bash
# Get disorder entry by OrphaCode
curl "http://localhost:9292/ws/entry/?i=558&s=orphanet"

# Search by disease name
curl "http://localhost:9292/ws/?i=Marfan%20syndrome"
```

### Disorder -> Phenotype Mappings
```bash
# Find HPO phenotypes for a disorder
curl "http://localhost:9292/ws/map/?i=558&m=>>orphanet>>hpo"
```

### Disorder -> Gene Mappings
```bash
# Find genes associated with a disorder
curl "http://localhost:9292/ws/map/?i=558&m=>>orphanet>>ensembl"
curl "http://localhost:9292/ws/map/?i=558&m=>>orphanet>>hgnc"
```

### Cross-Database Mappings
```bash
# Find OMIM disease mapping
curl "http://localhost:9292/ws/map/?i=558&m=>>orphanet>>mim"

# Find MONDO mapping
curl "http://localhost:9292/ws/map/?i=558&m=>>orphanet>>mondo"
```

### Filter by Attributes
```bash
# Filter by disorder type
curl "http://localhost:9292/ws/map/?i=558&m=>>orphanet[disorder_type==\"Disease\"]"

# Filter phenotypes by frequency
curl "http://localhost:9292/ws/?i=558&filter=orphanet.phenotypes.exists(p, p.frequency_value > 0.8)"
```

## Test Coverage

### Declarative Tests (test_cases.json)
- ID lookup verification
- Attribute presence checks
- Multiple ID batch lookups
- Invalid ID handling

### Custom Tests (test_orphanet.py)
1. **Disorder Attributes** - Verify disorders have names, synonyms, types
2. **Phenotype Associations** - Verify HPO phenotype links with frequency
3. **Frequency Data** - Verify numeric frequency values
4. **Cross-References** - Verify links to other databases
5. **Text Search** - Search by disease names
6. **Mapping Queries** - Orphanet -> HPO mappings

## Running Tests

### Quick Test (minimal database)
```bash
cd /path/to/biobtreev2
./biobtree -d orphanet test
```

### Full Integration Test
```bash
cd tests
python3 run_tests.py orphanet
```

## Data Statistics

Full build statistics:
- **Disorder entries**: ~11,456 OrphaCodes
- **Phenotype associations**: ~115,000+ HPO links
- **Gene associations**: ~8,300 gene links
- **Text search entries**: ~26,000+ (names + synonyms)

## References

- Orphanet: https://www.orpha.net/
- Orphadata: https://www.orphadata.com/
- Orphanet nomenclature: https://www.orpha.net/orphacom/cahiers/docs/GB/Orphanet_Nomenclature_Pack.pdf

## License

Orphadata files are licensed under CC BY 4.0 (commercial use allowed with attribution).
Attribution: Orphadata, INSERM, Paris
