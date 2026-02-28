# CORUM Dataset Tests

## Overview

CORUM (Comprehensive Resource of Mammalian Protein Complexes) is a manually curated database of experimentally characterized protein complexes from mammalian organisms.

**Source**: https://mips.helmholtz-muenchen.de/corum/
**Dataset ID**: 117
**Bucket Method**: numeric

## Data Statistics

- **Total complexes**: ~7,969
- **Unique UniProt IDs**: ~8,832
- **Unique Entrez IDs**: ~8,250
- **Unique GO IDs**: ~2,684
- **Organisms**: Human (~70%), Mouse (~16%), Rat (~8%), others

## Cross-references Created

| Target Dataset | Source Field | Description |
|---------------|--------------|-------------|
| uniprot | subunits[].swissprot.uniprot_id | Subunit protein accessions |
| entrez | subunits[].swissprot.entrez_id | Subunit gene IDs |
| go | functions[].go.go_id | Functional annotations |
| pubmed | pmid | Literature references |
| taxonomy | organism (mapped) | Organism taxonomy |

## Attributes Stored

- `name` - Complex name
- `synonyms` - Alternative names
- `organism` - Organism (Human, Mouse, Rat, etc.)
- `cell_line` - Cell line where characterized
- `comment` - Complex description/function
- `comment_disease` - Disease associations
- `comment_drug` - Drug associations
- `subunit_genes` - Gene symbols of subunits
- `subunit_count` - Number of subunits
- `purification_methods` - PSI-MI method names
- `pmid` - Primary PubMed ID
- `has_drug_targets` - Boolean flag (true if any subunit has drug associations)
- `has_splice_variants` - Boolean flag (true if complex has splice variant info)
- `subunits` - Nested subunit details (UniProt IDs, gene names, drugs)

Note: GO IDs/names and UniProt IDs are available via cross-references (not duplicated as attributes).

## Test Files

| File | Description |
|------|-------------|
| `test_cases.json` | Declarative tests for ID lookup and attributes |
| `test_corum.py` | Python test runner with custom tests |
| `reference_data.json` | Cached reference data from source |
| `corum_ids.txt` | Test entry IDs (generated in test mode) |

## Running Tests

```bash
# Run via orchestrator (recommended)
python3 tests/run_tests.py corum

# Or run directly (requires biobtree web server running)
cd tests/datasets/corum
python3 test_corum.py
```

## Query Examples

```bash
# Direct lookup
biobtree query "1"                           # Complex by ID
biobtree query "BCL6-HDAC4 complex"          # By name

# Mapping queries
biobtree query "P41182 >> uniprot >> corum"  # Protein to complexes
biobtree query "1 >> corum >> uniprot"       # Complex to subunit proteins
biobtree query "1 >> corum >> go"            # Complex to GO terms

# Filtered queries
biobtree query "P04637 >> uniprot >> corum[organism==\"Human\"]"
biobtree query "P04637 >> uniprot >> corum[subunit_count>5]"
biobtree query "P04637 >> uniprot >> corum[has_drug_targets==true]"
```

## Known Limitations

1. **Stoichiometry data**: Only ~288 complexes have stoichiometry information
2. **Drug associations**: Embedded in subunits (gene.drugs), not complex-level
3. **OMIM associations**: Available but not indexed as cross-references
4. **FCG (Functional Complex Groups)**: Parsed but not exposed as separate xrefs

## Source Files

The CORUM data is parsed from:
- `raw_data/CORUM/corum_allComplexes.json` - Main complexes file (22.5 MB)

Other available files (subsets, same data):
- `corum_drugs.json` - Complexes with drug associations (3,911 entries)
- `corum_spliceComplexes.json` - Complexes with splice variants (295 entries)
- `corum_partialComplexes.json` - Partial complexes (10 entries)
