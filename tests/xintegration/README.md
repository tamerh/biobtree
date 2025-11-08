# Integration Tests & Use Cases

**Purpose**: Test real biological workflows, find bugs, validate new datasets

**Database**: scc2:9292 (Model Organisms)

---

## Quick Start

```bash
# Run all tests against scc2
python3 run_integration_tests.py integration_tests_expanded.json --server http://scc2:9292

# Run against local instance
python3 run_integration_tests.py integration_tests_expanded.json --server http://localhost:9292

# Verbose output
python3 run_integration_tests.py integration_tests_expanded.json --server http://scc2:9292 --verbose

# Results saved to reports/YYYY-MM-DD_HHMM_analysis.md
```

---

## Test Suite

**File**: `integration_tests_expanded.json`

- **114 biological use cases** converted from v1 usecases.json
- **6 categories**: gene_mapping, protein_features, chembl_drug, genomics, pathways, ontologies
- Each test has `why` field explaining biological context
- `should_pass`: IDs expected to return results
- `should_fail`: Invalid IDs for error handling tests

### Current Status (2025-11-08)
- ✅ **96 passing** (84.2%)
- ❌ **18 failing** (see latest report in `reports/`)

---

## Adding New Tests

Edit `integration_tests_expanded.json`:

```json
{
  "category": "gene_mapping",
  "name": "Short descriptive name",
  "query": ">>dataset1>>dataset2[filter]",
  "why": "One sentence: why researchers need this",
  "should_pass": ["ID1", "ID2"],
  "should_fail": ["INVALID_ID"]
}
```

Run tests to validate. Report generated automatically.

---

## Query Syntax (Quick Reference)

### Basic
```bash
BRCA1 >> uniprot                    # Auto-detect source, map to uniprot
BRCA1 >> uniprot >> go              # Multi-hop
```

### Filters (Working)
```bash
>>uniprot[uniprot.reviewed==true]                      # Boolean
>>go[go.type=="biological_process"]                    # Equality
>>chembl_activity[chembl.activity.value > 10.0]        # Comparison
>>ensembl[ensembl.overlaps(114129278,114129328)]       # Genomic range
```

### Filters (Known Issues)
```bash
>>reactome[reactome.pathway.contains("signaling")]     # ❌ .contains() not working
>>ensembl>>ortholog                                     # ❌ Ortholog mapping broken
```

---

## Understanding Reports

Reports are auto-generated in `reports/YYYY-MM-DD_HHMM_analysis.md` after each test run.

**What's in a report:**
- Summary: pass/fail counts, percentage
- Passing tests grouped by category
- Failing tests with error details and URLs
- Test coverage breakdown

**Use reports to:**
- Track regression between builds
- Identify broken features after data updates
- Document issues for debugging
- Validate new dataset integration

---

## Categories

| Category | Tests | Focus |
|----------|-------|-------|
| gene_mapping | 14 | Gene symbols → proteins, GO, transcripts |
| protein_features | 10 | Domains, mutations, variants, structures |
| chembl_drug | 8 | Compounds, targets, bioactivity |
| genomics | 12 | Coordinates, exons, orthologs, probes |
| pathways | 4 | Reactome, STRING networks |
| ontologies | 8 | GO, EFO, Taxonomy navigation |

---

## Common Workflows (Examples)

All from `integration_tests_expanded.json`:

```bash
# Cancer gene panel to reviewed proteins
BRCA1,BRCA2,TP53 >> uniprot[uniprot.reviewed==true]

# Find protein mutations
TP53 >> uniprot >> ufeature[ufeature.type=="mutagenesis site"]

# Drug target discovery
EGFR >> uniprot >> chembl_target_component >> chembl_target

# Pathway annotation
TP53 >> uniprot >> go[go.type=="biological_process"]

# Genomic coordinates
BRCA1 >> ensembl

# Microarray probe conversion
202763_at >> transcript >> ensembl >> hgnc
```

---

## Troubleshooting

### Tests return 400 errors
- Invalid ID (expected for `should_fail`)
- Unsupported filter syntax (e.g., `.contains()`)
- Dataset not in build

### Tests return empty `{}`
- ID not found in database
- Filter excludes all results
- Missing cross-references

### Slow execution
- Each test hits live API
- 114 tests ≈ 2 minutes
- Use `--no-report` to skip report generation

---

## Files

```
tests/xintegration/
├── README.md                           # This file
├── integration_tests_expanded.json     # Test suite (114 tests)
├── run_integration_tests.py            # Test runner
└── reports/                            # Auto-generated reports
    └── YYYY-MM-DD_HHMM_analysis.md
```

---

## Session Log

- **2025-11-07**: Arrow `>>` syntax fixed for Web API
- **2025-11-07**: Enrichment nil pointer bug fixed
- **2025-11-08**: Comprehensive test suite created (114 tests, 83.3% pass rate)
- **2025-11-08**: URL encoding fix - test runner now uses requests params dict (84.2% pass rate)

**Next**: Run tests after data reprocessing, new dataset addition, or bug fixes

---

## Known Issues (from latest test run)

See latest report in `reports/` for details:

1. **String `.contains()` filter** - Not implemented (3 failures)
2. **Ortholog mapping** - Returns 400 error (6 failures)
3. **Error handling** - Invalid IDs return 400 instead of empty results
4. **MONDO disease mapping** - Empty (not in Model Organisms build)

**Last Test Run**: 2025-11-08 16:23
**Pass Rate**: 84.2% (96/114)
