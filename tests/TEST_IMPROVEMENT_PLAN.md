# Test Suite Improvement Plan

**Created:** 2025-11-05
**Status:** Planning Phase

## Current Status

### README Coverage
- **Total datasets:** 29
- **With READMEs:** 2 (reactome ✅, string ✅)
- **Missing READMEs:** 27 ❌

### Test Coverage Analysis

| Coverage Level | Dataset Count | Datasets |
|----------------|---------------|----------|
| **Low (3-5 tests)** | 13 | hgnc, eco, efo, go, taxonomy, uniparc, uniprot, interpro, ensembl_bacteria, ensembl_fungi, ensembl_metazoa, ensembl_plants, ensembl_protists, uniref100, uniref50, uniref90 |
| **Medium (6-9 tests)** | 14 | chebi, chembl_activity, chembl_assay, chembl_cell_line, chembl_document, chembl_molecule, clinical_trials, patent, hmdb, mondo, string |
| **High (12-15 tests)** | 2 | chembl_target, reactome |

## Goals

### Phase 1: Documentation (Priority: HIGH)
Create comprehensive README.md for all 27 datasets without documentation

**README Template Structure** (based on reactome/README.md):
1. **Overview** - Dataset description and purpose
2. **Test Data** - Source, files, statistics
3. **Dataset Statistics** - Entry counts, cross-references
4. **Reference Data Format** - API structure and examples
5. **Integration Architecture** - Storage model, cross-references
6. **Usage** - Query examples
7. **Test Cases** - Description of each test
8. **Performance Notes** - Build times, mapping counts
9. **Known Limitations** - Current gaps
10. **Future Work** - Planned enhancements
11. **References** - Official sources, publications
12. **Maintenance** - Update procedures

### Phase 2: Test Enhancement (Priority: MEDIUM-HIGH)
Analyze and improve test coverage for datasets with insufficient tests

**Priorities by Dataset Importance:**

#### Tier 1 (Critical - Core Biobtree Datasets) 🔴
High usage, fundamental to biobtree value proposition
- **UniProt** (4 tests → target: 10-12)
  - Add: protein features, variants, sequence validation
  - Add: subcellular location, function tests
  - Add: reviewed vs unreviewed filtering

- **HGNC** (3 tests → target: 8-10)
  - Add: gene symbol search variations
  - Add: alias/previous symbol lookup
  - Add: locus type validation
  - Add: chromosome location tests

- **GO** (4 tests → target: 8-10)
  - Add: ontology hierarchy tests (parent/child)
  - Add: aspect validation (BP/MF/CC)
  - Add: definition and synonym tests
  - Add: obsolete term handling

- **Taxonomy** (4 tests → target: 8-10)
  - Add: lineage traversal tests
  - Add: rank validation (species, genus, family)
  - Add: scientific name vs common name
  - Add: parent/child relationship tests

#### Tier 2 (Important - Widely Used Datasets) 🟡
- **ChEMBL Molecule** (7 tests → target: 10-12)
  - Add: structure search tests
  - Add: property validation (MW, LogP)
  - Add: development phase tests
  - Add: InChI/SMILES validation

- **ChEMBL Target** (12 tests → already good ✅)
  - Maybe add: target family tests
  - Maybe add: organism specificity

- **HMDB** (9 tests → target: 10-12)
  - Add: disease association tests
  - Add: metabolite classification
  - Add: pathway membership tests

- **InterPro** (5 tests → target: 8-10)
  - Add: domain type validation
  - Add: hierarchy tests
  - Add: member database xrefs

#### Tier 3 (Specialized - Domain-Specific) 🟢
- **ECO/EFO** (4 tests each → target: 6-8 each)
  - Add: evidence/ontology hierarchy
  - Add: definition validation
  - Add: cross-ontology mappings

- **MONDO** (8 tests → already good ✅)
- **Patent** (8 tests → already good ✅)
- **Clinical Trials** (8 tests → already good ✅)
- **ChEBI** (8 tests → already good ✅)

#### Tier 4 (Reference - Lower Priority) ⚪
- **UniParc/UniRef** (4-5 tests → target: 6-8)
  - Add: cluster membership tests
  - Add: sequence identity validation

- **Ensembl divisions** (5 tests each → target: 7-9 each)
  - Add: genome-specific tests
  - Add: gene biotype validation
  - Add: coordinate system tests

## Implementation Strategy

### Phase 1: Quick Wins (Week 1-2)
1. **Create README templates** for all 27 datasets
   - Start with Tier 1 datasets (UniProt, HGNC, GO, Taxonomy)
   - Use reactome/README.md as the gold standard template
   - Document existing tests and cross-references

2. **Standardize README format** across all datasets
   - Consistent sections
   - Complete API documentation
   - Clear test case descriptions

### Phase 2: Systematic Test Analysis (Week 3-4)
For each Tier 1 & Tier 2 dataset:
1. **Review reference_data.json** - What data do we have?
2. **Check source dataset documentation** - What can we test?
3. **Analyze existing tests** - What are we missing?
4. **Identify test gaps** - Create improvement list
5. **Write new tests** - Add to test_cases.json or test_<dataset>.py

### Phase 3: Test Implementation (Week 5-6)
1. Start with Tier 1 datasets
2. Add tests incrementally (2-4 tests per dataset)
3. Run full test suite after each batch
4. Update README.md with new test descriptions

### Phase 4: Validation & Documentation (Week 7)
1. Run complete test suite for all datasets
2. Update main tests/README.md with new statistics
3. Document test patterns and best practices
4. Create test enhancement guidelines

## Detailed Improvement Plans

### UniProt (Current: 4 tests → Target: 10-12 tests)

**Current Tests:**
1. ID lookup
2. Attributes check
3. Cross-references
4. Invalid ID

**Tests to Add:**
5. **Reviewed vs Unreviewed** - Validate reviewed field (Swiss-Prot vs TrEMBL)
6. **Protein Features** - Check features array (domains, regions, sites)
7. **Sequence Validation** - Verify sequence and mass attributes
8. **Organism Specificity** - Tax ID cross-reference accuracy
9. **Gene Name Mapping** - Gene names and synonyms
10. **Alternative Names** - Alternative protein names and aliases
11. **Subcellular Location** - Cellular component annotations
12. **Function Description** - Protein function text presence

**Data Available in reference_data.json:**
- ✅ Reviewed status
- ✅ Sequence and mass
- ✅ Names and alternative names
- Need to check: features, locations, functions

### HGNC (Current: 3 tests → Target: 8-10 tests)

**Current Tests:**
1. ID lookup
2. Symbol lookup
3. Cross-references

**Tests to Add:**
4. **Alias Lookup** - Previous symbols and aliases
5. **Locus Type** - Validate locus_type field (gene, pseudogene, RNA)
6. **Chromosome Location** - Chromosome and location data
7. **Gene Status** - Approved, withdrawn, etc.
8. **Name Variations** - Symbol case sensitivity
9. **Ensembl Mapping** - HGNC → Ensembl cross-reference
10. **UniProt Mapping** - HGNC → UniProt cross-reference

### GO (Current: 4 tests → Target: 8-10 tests)

**Current Tests:**
1. ID lookup
2. Attributes check
3. Cross-references
4. Invalid ID

**Tests to Add:**
5. **Aspect Validation** - Verify aspect (biological_process, molecular_function, cellular_component)
6. **Parent Terms** - GO hierarchy parent relationships
7. **Child Terms** - GO hierarchy child relationships
8. **Definition Present** - Every term has definition
9. **Synonyms** - Alternative names for terms
10. **Obsolete Handling** - Obsolete terms flagged correctly

### Taxonomy (Current: 4 tests → Target: 8-10 tests)

**Current Tests:**
1. ID lookup
2. Attributes check
3. Cross-references
4. Invalid ID

**Tests to Add:**
5. **Scientific Name** - Validate scientific_name field
6. **Common Name** - Common name when available
7. **Rank Validation** - Species, genus, family, etc.
8. **Lineage Path** - Parent organism navigation
9. **Child Taxa** - Child organisms for higher ranks
10. **NCBI Taxonomy Consistency** - Verify tax IDs match NCBI

## Test Pattern Library

Common patterns to reuse across datasets:

### Basic Patterns (Already in use)
- ✅ ID lookup
- ✅ Attributes check
- ✅ Cross-references exist
- ✅ Invalid ID handling

### Advanced Patterns (To standardize)
- **Hierarchy Tests** - Parent/child relationships (GO, Taxonomy, Reactome)
- **Field Validation** - Specific attribute value checks
- **Multi-entity Lookup** - Batch queries
- **Filtering Tests** - Query with filters
- **Case Sensitivity** - Case-insensitive searches
- **Alias/Synonym Lookup** - Alternative identifiers
- **Bidirectional Xrefs** - A→B and B→A both work

## Success Metrics

### Documentation
- [ ] All 29 datasets have comprehensive README.md files
- [ ] READMEs follow consistent template structure
- [ ] All API sources documented
- [ ] All test cases explained

### Test Coverage
- [ ] No dataset has fewer than 6 tests
- [ ] Tier 1 datasets have 10-12 tests each
- [ ] Tier 2 datasets have 8-10 tests each
- [ ] Total test count: 185 → 250+

### Test Quality
- [ ] All major dataset features tested
- [ ] Cross-reference bidirectionality validated
- [ ] Edge cases covered (invalid IDs, missing data)
- [ ] Attribute completeness verified

### Maintenance
- [ ] Clear update procedures documented
- [ ] Test regeneration guidelines written
- [ ] Best practices guide created
- [ ] Continuous integration ready

## Timeline

| Week | Phase | Deliverables |
|------|-------|--------------|
| 1-2  | README Creation | 27 new README files (Tier 1 first) |
| 3    | Test Analysis | Gap analysis for all Tier 1 & 2 datasets |
| 4    | Test Planning | Detailed test plans for priority datasets |
| 5-6  | Test Implementation | Add 65+ new tests across datasets |
| 7    | Validation | Full test suite run + documentation update |

## Priority Order for README Creation

1. **Tier 1** (Week 1): UniProt, HGNC, GO, Taxonomy
2. **Tier 2** (Week 1-2): ChEMBL datasets, HMDB, InterPro
3. **Tier 3** (Week 2): ECO, EFO, MONDO, Patent, Clinical Trials, ChEBI
4. **Tier 4** (Week 2): UniParc, UniRef, Ensembl divisions

## Notes

- Use reactome/README.md as the **gold standard template**
- string/README.md is good but shorter - use as secondary reference
- Keep READMEs practical and focused on testing
- Include performance metrics from test builds
- Document known limitations and future work
- Always link to official dataset sources

## Next Steps

1. **Review this plan** with team
2. **Create README template** based on reactome/README.md
3. **Start with UniProt README** as first implementation
4. **Iterate and refine** template based on feedback
5. **Scale to remaining datasets** in priority order
