# STRING Database Integration Plan for Biobtreev2

## Overview

STRING (Search Tool for the Retrieval of Interacting Genes/Proteins) is a protein-protein interaction database covering 24M+ proteins across 3000+ organisms with 20B+ interactions. This document outlines the phased approach for integrating STRING into biobtreev2.

## Priority & Rationale

**Priority**: ⭐⭐⭐ (Top Priority - from DATASET_RECOMMENDATIONS.md)

**Why STRING?**
- Adds **network analysis dimension** to existing UniProt/Ensembl proteins
- Enables **systems biology** and **pathway analysis** queries
- **24M+ proteins** with confidence-scored interactions
- **Well-documented** with clear identifier mappings
- **Moderate size**: 128GB full dataset, can start with organism subsets
- **Annual updates**: Manageable maintenance schedule

**Key Use Cases:**
- Find protein interaction partners: `HGNC:EGFR >> uniprot >> string >> uniprot`
- Patent → targets → networks: `US-patent >> surechembl >> chembl >> uniprot >> string`
- Clinical trials → drug targets → interactions: `NCT_ID >> chembl >> uniprot >> string`
- Gene → interactions → pathways: `BRCA1 >> uniprot >> string >> reactome`

## STRING Database Overview

### Data Source
- **URL**: https://string-db.org/
- **Downloads**: https://string-db.org/cgi/download
- **Version**: 12.0 (current)
- **License**: Creative Commons Attribution 4.0 International
- **Update Frequency**: Annual major releases

### Available Files

| File | Size | Description |
|------|------|-------------|
| protein.links.v12.0.txt.gz | 128.7 GB | Scored protein-protein links (main file) |
| protein.links.detailed.v12.0.txt.gz | 189.6 GB | Includes subscores per evidence channel |
| protein.links.full.v12.0.txt.gz | 199.6 GB | Distinguishes direct vs. interologs |
| protein.aliases.v12.0.txt.gz | ~5 GB | Maps STRING IDs to external IDs (UniProt, Ensembl, etc.) |
| protein.info.v12.0.txt.gz | ~3 GB | Protein annotations and descriptions |

### Organism-Specific Files

STRING provides per-organism downloads (recommended for testing):
- Human (9606): `9606.protein.links.v12.0.txt.gz` (~500 MB)
- Mouse (10090): `10090.protein.links.v12.0.txt.gz` (~300 MB)
- Rat (10116), Yeast (4932), E. coli (511145), etc.

## Phase 1: Data Understanding & Analysis

**Goal**: Download and thoroughly understand STRING data structure before any implementation.

### Step 1.1: Download Data for Analysis

**Location**: Download to a temporary analysis directory outside the project
```bash
# Create analysis directory
mkdir -p /data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/analysis/string_raw

cd /data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/analysis/string_raw

# Download human data for initial analysis (manageable size)
wget https://stringdb-downloads.org/download/protein.links.v12.0/9606.protein.links.v12.0.txt.gz
wget https://stringdb-downloads.org/download/protein.aliases.v12.0.txt.gz
wget https://stringdb-downloads.org/download/protein.info.v12.0.txt.gz

# Decompress
gunzip 9606.protein.links.v12.0.txt.gz
gunzip protein.aliases.v12.0.txt.gz
gunzip protein.info.v12.0.txt.gz
```

**Expected Downloads**:
- Human links: ~500 MB uncompressed
- Aliases (all organisms): ~15 GB uncompressed
- Info (all organisms): ~8 GB uncompressed

### Step 1.2: Analyze Data Structure

**Objectives**:
1. Understand file formats (delimiters, columns, data types)
2. Understand STRING identifier scheme (e.g., `9606.ENSP00000000233`)
3. Map STRING IDs to UniProt/Ensembl identifiers
4. Understand interaction scores and confidence levels
5. Analyze evidence channels (experimental, database, coexpression, etc.)
6. Determine organism taxonomic ID usage
7. Identify relational structure and how to adapt to biobtree's model

**Analysis Questions to Answer**:
- [ ] What is the format of protein.links file? (TSV, space-separated?)
- [ ] How are STRING protein IDs structured? (taxid.identifier format?)
- [ ] How do we map STRING IDs to UniProt accessions?
- [ ] What are the score ranges? (0-1000? thresholds for low/medium/high confidence?)
- [ ] How many interactions per protein on average?
- [ ] What evidence types are available in detailed file?
- [ ] How do we handle bidirectional interactions? (A-B vs B-A)
- [ ] What is the relationship between aliases file and links file?

**Analysis Commands**:
```bash
# Examine file structures
head -n 20 9606.protein.links.v12.0.txt
head -n 20 protein.aliases.v12.0.txt | grep "^9606\."
head -n 20 protein.info.v12.0.txt | grep "^9606\."

# Count records
wc -l 9606.protein.links.v12.0.txt
grep "^9606\." protein.aliases.v12.0.txt | wc -l

# Examine score distribution
cut -d' ' -f3 9606.protein.links.v12.0.txt | sort -n | uniq -c | tail -n 20

# Find UniProt mappings in aliases
grep "^9606\." protein.aliases.v12.0.txt | grep -i "uniprot" | head -n 20

# Sample specific protein (e.g., EGFR-related)
grep "EGFR" protein.aliases.v12.0.txt | head -n 10
```

### Step 1.3: Document Data Structure Findings

**Create**: `/data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/analysis/STRING_DATA_ANALYSIS.md`

**Document**:
- File formats and column definitions
- Identifier schemes and mapping strategy
- Score distributions and recommended thresholds
- Evidence channel types and meanings
- Sample data examples with annotations
- Relationship diagrams showing how files connect
- Challenges and solutions for biobtree adaptation

### Step 1.4: Design Biobtree Adaptation Strategy

**Key Questions**:
1. **Identifier Strategy**:
   - Use STRING IDs as primary? Or map to UniProt?
   - How to handle proteins without UniProt mapping?

2. **Storage Model**:
   - Store interactions as attributes on UniProt entries?
   - Create separate STRING protein entries?
   - How to represent bidirectional relationships?

3. **Cross-references**:
   - `UNIPROT_ID ↔ STRING_ID`?
   - `UNIPROT_A → UNIPROT_B` (via STRING interaction)?
   - Text-based search for "protein_network"?

4. **Score Handling**:
   - Store all interactions or filter by score?
   - Store score as attribute?

5. **Evidence Types**:
   - Store basic links or detailed evidence channels?
   - How to represent multiple evidence types?

## Phase 2: Create Test Dataset

**Goal**: Create a small, representative test dataset in `test_data/string/`

### Step 2.1: Select Test Proteins

**Criteria**:
- ~100-500 proteins with diverse interaction patterns
- Include proteins already in test datasets (HGNC, UniProt)
- Include well-known proteins (EGFR, BRCA1, TP53, etc.)
- Mix of highly-connected and less-connected proteins

### Step 2.2: Extract Test Data

```bash
# Create test directory
mkdir -p /data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/biobtreev2/test_data/string

# Extract subset based on analysis findings
# (Commands will be determined after understanding data structure)
```

**Test Files to Create**:
```
test_data/string/
├── protein_links_test.txt      # Subset of interactions
├── protein_aliases_test.txt    # ID mappings for test proteins
├── protein_info_test.txt       # Metadata for test proteins
└── README.md                   # Documentation of test dataset
```

### Step 2.3: Validate Test Dataset

- [ ] Verify test proteins exist in biobtree's UniProt/Ensembl data
- [ ] Confirm interactions are complete (all partners included)
- [ ] Check identifier mappings are present
- [ ] Document expected query results for validation

## Phase 3: Implementation (Post-Analysis)

**Note**: This phase will be refined after completing data analysis.

### High-Level Implementation Steps

1. **Add to `conf/source.dataset.json`**
   - Define STRING dataset configuration
   - Specify test data path
   - Define attributes to store

2. **Create `src/update/string.go`**
   - Follow clinical_trials.go pattern
   - Implement data processing logic
   - Handle identifier mapping
   - Create cross-references

3. **Define Protobuf Messages** (if needed)
   - Add to `src/pbuf/biobtree.proto`
   - Define StringInteractionAttr message

4. **Testing**
   - Build with test dataset
   - Verify cross-references
   - Test query chains
   - Benchmark performance

5. **Production Deployment**
   - Expand to full human dataset
   - Add other model organisms
   - Document query examples
   - Update README

## Phase 4: Production Considerations

### Organism Prioritization

**Phase 1**: Human only (9606)
- ~20K proteins, ~11M interactions
- ~500 MB data
- ~30 minutes processing time

**Phase 2**: Add model organisms
- Mouse (10090), Rat (10116), Yeast (4932)
- +15M interactions
- +1 GB data

**Phase 3**: Match existing Ensembl genomes
- Process only organisms already in biobtree
- Align with user's selected tax IDs

**Phase 4**: Full dataset (optional)
- All 3000+ organisms
- 128 GB data
- 10-12 hours processing

### Performance Estimates

| Scope | Proteins | Interactions | Storage | Processing |
|-------|----------|--------------|---------|------------|
| Test dataset | 500 | ~5K | <10 MB | <1 min |
| Human only | 20K | 11M | ~2 GB | ~30 min |
| Top 10 organisms | 150K | 50M | ~8 GB | ~2 hours |
| Full dataset | 24M | 20B | ~200 GB | ~10 hours |

### Score Filtering

**Recommended Thresholds** (to be validated during analysis):
- **High confidence**: score ≥ 700
- **Medium confidence**: score ≥ 400
- **Low confidence**: score ≥ 150

**Default**: Start with medium confidence (≥ 400) to balance coverage and noise.

### Update Strategy

- **Frequency**: Annual (following STRING release schedule)
- **Method**: Full rebuild (interactions change significantly between versions)
- **Monitoring**: Track release announcements at https://string-db.org/

## Timeline Estimate

### Phase 1: Data Understanding (Current Phase)
- Download data: 1-2 hours
- Analyze structure: 1-2 days
- Document findings: 0.5 days
- Design adaptation: 1 day
- **Total: 3-4 days**

### Phase 2: Test Dataset Creation
- Select proteins: 0.5 days
- Extract data: 0.5 days
- Validate: 0.5 days
- **Total: 1-2 days**

### Phase 3: Implementation
- Configuration: 0.5 days
- Core processing code: 2-3 days
- Testing: 1-2 days
- Bug fixes: 1 day
- **Total: 4-6 days**

### Phase 4: Production
- Human dataset: 0.5 days
- Model organisms: 1 day
- Documentation: 1 day
- **Total: 2-3 days**

**Overall Timeline**: 10-15 days

## Success Criteria

### Phase 1 (Analysis) Complete When:
- [ ] All data files downloaded and examined
- [ ] Data structure fully documented
- [ ] Identifier mapping strategy defined
- [ ] Biobtree adaptation approach designed
- [ ] Analysis document created

### Phase 2 (Test Data) Complete When:
- [ ] test_data/string/ directory created
- [ ] Test dataset representative and manageable
- [ ] Proteins mappable to existing biobtree data
- [ ] Expected results documented

### Phase 3 (Implementation) Complete When:
- [ ] Biobtree builds successfully with STRING data
- [ ] Cross-references verified
- [ ] Query chains work end-to-end
- [ ] Performance acceptable (<1 min for test data)

### Phase 4 (Production) Complete When:
- [ ] Human dataset integrated
- [ ] Query examples documented
- [ ] README updated
- [ ] Performance benchmarked

## Next Steps

1. **Execute Phase 1.1**: Download STRING data to analysis directory
2. **Execute Phase 1.2**: Analyze data structure using provided commands
3. **Execute Phase 1.3**: Document findings in STRING_DATA_ANALYSIS.md
4. **Execute Phase 1.4**: Design biobtree adaptation strategy
5. **Review & Iterate**: Adjust plan based on discoveries

## References

- **STRING Database**: https://string-db.org/
- **Download Page**: https://string-db.org/cgi/download
- **API Documentation**: https://string-db.org/help/api/
- **FAQ**: https://string-db.org/help/faq/
- **Biobtreev2 README**: `/biobtreev2/README.md`
- **DATASET_RECOMMENDATIONS**: `/biobtreev2/DATASET_RECOMMENDATIONS.md` (lines 28-66)
- **Clinical Trials Integration**: `/biobtreev2/src/update/clinical_trials.go` (reference pattern)

---

**Document Version**: 1.0
**Created**: 2025-10-24
**Status**: Phase 1 - Data Understanding (In Progress)
