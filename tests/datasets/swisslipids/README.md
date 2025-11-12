# SwissLipids Dataset

## Overview

SwissLipids is a comprehensive lipid knowledge resource providing curated information on lipid structures, nomenclature, and their biological roles. It combines expert curation with computational approaches to deliver detailed lipid annotations, cross-references to proteins, GO terms, tissues, and evidence codes.

**Source**: SwissLipids API (https://www.swisslipids.org/#/downloads)
**Data Type**: Lipid structures with chemical descriptors, biological annotations, and protein associations

## Integration Architecture

### Storage Model

**Primary Entries**:
- SwissLipids IDs (e.g., `SLM:000094711`) stored as main identifiers

**Searchable Text Links**:
- Lipid names → stored in attributes
- Abbreviations → stored in attributes
- Synonyms → stored in attributes (array)

**Attributes Stored** (protobuf SwisslipidsAttr):
- Name (e.g., "Cholesterol")
- Abbreviation (e.g., "Chol")
- Synonyms (repeated string, alternative names)
- Category (lipid category classification)
- Main class (main lipid class)
- Sub class (lipid subclass)
- Level (hierarchical level: Species, Molecular species, Class, etc.)
- SMILES (simplified molecular-input line-entry system)
- InChI (International Chemical Identifier)
- InChI key (hashed InChI for exact matching)
- Formula (molecular formula at pH 7.3)
- Mass (molecular mass at pH 7.3)
- Charge (charge state at pH 7.3)

**Cross-References**:
- **Proteins**: UniProt IDs (from lipids2uniprot.tsv)
- **GO Terms**: Gene Ontology Biological Process (from go.tsv) ✓
- **Tissues**: Uberon tissue/organ annotations (from tissues.tsv)
- **Evidence**: ECO evidence codes for data quality tracking (from evidences.tsv) ✓
- **Chemical databases**: ChEBI, HMDB, LIPID MAPS (from lipids.tsv)
- **Reactions**: Rhea biochemical reactions (from enzymes.tsv - TODO)

### Special Features

**Multi-File Integration**:
- Combines data from 6 TSV files for complete biological context
- Main lipids.tsv (655MB): 779,257 lipid structures
- lipids2uniprot.tsv (382MB): Protein-lipid associations
- go.tsv (47KB): GO Biological Process annotations
- tissues.tsv (95KB): Uberon tissue localization
- enzymes.tsv (964KB): Rhea reaction participation
- evidences.tsv (528KB): ECO evidence codes for curation quality

**Test Mode Optimization**:
- Intelligent filtering: processes 100 lipids + only their cross-references
- Prevents loading millions of irrelevant cross-references during testing
- Uses ID tracking map to filter secondary TSV files

**Gzip Detection**:
- Automatic detection of gzipped vs plain TSV files
- Uses magic number detection (0x1f 0x8b) for reliable decompression
- Handles mixed compression across different files

**Chemical Descriptors**:
- Complete structural information (SMILES, InChI, InChI Key)
- Molecular properties (formula, mass, charge at pH 7.3)
- Exact m/z values for mass spectrometry (stored in TSV, not in protobuf)

**Evidence-Based Cross-References**:
- ECO evidence codes track quality of each annotation
- Supports experimental vs computational evidence distinction
- Enables confidence-based filtering

## Use Cases

**1. Lipid Structure Lookup**
```
Query: SwissLipids ID → Lipid entry → Name, abbreviation, chemical structure
Use: Identify lipid structure from database identifier
```

**2. Chemical Property Search**
```
Query: Lipid ID → SMILES, InChI, formula, mass
Use: Get chemical descriptors for computational analysis or mass spectrometry
```

**3. Protein-Lipid Associations**
```
Query: Lipid ID → UniProt cross-references → Associated proteins
Query: UniProt ID → SwissLipids cross-references → Lipids interacting with protein
Use: Study protein-lipid interactions, membrane biology, lipid metabolism
```

**4. GO Biological Process Annotations**
```
Query: Lipid ID → GO cross-references → Biological processes
Query: GO term → SwissLipids cross-references → Lipids involved in process
Use: Functional annotation, pathway analysis, systems biology
```

**5. Tissue/Organ Localization**
```
Query: Lipid ID → Uberon cross-references → Tissues where lipid is found
Use: Tissue-specific lipid metabolism, biomarker discovery
```

**6. Evidence-Based Data Quality**
```
Query: Lipid ID → ECO cross-references → Evidence codes for annotations
Use: Filter by annotation quality, assess curation confidence
```

**7. Cross-Database Integration**
```
Query: Lipid ID → ChEBI/HMDB/LIPID MAPS cross-references
Use: Link SwissLipids to other chemical/lipid databases
```

## Test Cases

**Current Tests** (10 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 6 custom tests (name, chemical data, UniProt xrefs, ECO evidence, synonyms, mass/formula)

**Coverage**:
- ✅ SwissLipids ID lookup
- ✅ Attribute validation (name, abbreviation, level, chemical descriptors)
- ✅ Batch lookups
- ✅ Invalid ID handling
- ✅ Chemical descriptor presence (SMILES, InChI, formula, mass)
- ✅ UniProt protein cross-references
- ✅ ECO evidence code cross-references
- ✅ Synonym arrays
- ✅ Molecular mass and formula

**Recommended Additions**:
- GO cross-reference navigation tests
- Uberon tissue cross-reference tests
- ChEBI/HMDB/LIPID MAPS bidirectional cross-reference tests
- Hierarchy navigation tests (parent-child lipid relationships)
- Test mode filtering validation (ensure only tracked IDs have xrefs)
- Rhea reaction integration tests (when implemented)

## Test Data

### Source

The test data consists of 100 lipid entries spanning various lipid classes from the SwissLipids database.

### Files

```
tests/datasets/swisslipids/
├── swisslipids_ids.txt                  # Test lipid IDs (100 IDs)
├── extract_reference_data.py            # Reference data extraction script
├── reference_data.json                  # Complete TSV data (all 29 columns)
├── test_swisslipids.py                  # Custom Python tests (6 tests)
├── test_cases.json                      # Declarative test cases (4 tests)
└── README.md                            # This file
```

### Dataset Statistics

- **Test Lipids**: 100 lipid IDs
- **Cross-References** (test mode):
  - ~665 UniProt protein associations
  - ~1 GO Biological Process term
  - ~2 Uberon tissue annotations
  - ~7,170 ECO evidence codes
- **Reference Data Source**: SwissLipids REST API
- **API Endpoint**: https://www.swisslipids.org/api/file.php?cas=download_files&file={filename}

### Extracting Reference Data

Reference data is fetched from the official SwissLipids API:

```bash
cd tests/datasets/swisslipids
python3 extract_reference_data.py      # Extract all 100 IDs
python3 extract_reference_data.py 10   # Extract first 10 IDs only
```

The script:
1. Reads lipid IDs from `swisslipids_ids.txt`
2. Downloads all 6 TSV files from SwissLipids API
3. Extracts complete rows (all 29 columns) for each test ID
4. Saves to `reference_data.json` with complete TSV data preserved

Rate limiting: 0.5 seconds between API calls

## Reference Data Format

The `reference_data.json` file contains complete TSV data for each test lipid:

```json
{
  "metadata": {
    "total_ids": 100,
    "fetched": 100,
    "failed": 0,
    "note": "Complete TSV data with all columns preserved for reference"
  },
  "entries": [
    {
      "id": "SLM:000094711",
      "lipids_data": {
        "headers": ["Lipid ID", "Level", "Name", "Abbreviation*", ...],
        "columns": ["SLM:000094711", "Species", "Cholesterol", "Chol", ...]
      },
      "lipids2uniprot": [
        ["SLM:000094711", "P12345"],
        ["SLM:000094711", "Q67890"]
      ],
      "go": [
        ["SLM:000094711", "GO:0008203"]
      ],
      "tissues": [],
      "enzymes": [],
      "evidences": [
        ["SLM:000094711", "ECO:0000269"],
        ["SLM:000094711", "ECO:0000501"]
      ]
    }
  ]
}
```

## Performance

- **Test Build**: ~4 seconds (100 lipids with filtered cross-references)
- **Data Source**: SwissLipids REST API (https://www.swisslipids.org/)
- **Update Frequency**: Regular updates (monthly releases)
- **Total Entries**: 779,257 lipids (full dataset)
- **Source Files**:
  - lipids.tsv (~655 MB) - main lipid structures
  - lipids2uniprot.tsv (~382 MB) - protein associations
  - go.tsv (~47 KB) - GO annotations
  - tissues.tsv (~95 KB) - tissue localization
  - enzymes.tsv (~964 KB) - reaction participation
  - evidences.tsv (~528 KB) - evidence codes

## Known Limitations

**Hierarchical Classification**:
- Category, main_class, sub_class fields not yet populated
- "Lipid class*" column contains parent ID references (e.g., SLM:000399814)
- TODO: Parse hierarchical relationships for class navigation

**Rhea Reactions**:
- enzymes.tsv file processed but Rhea dataset not yet integrated
- Cross-references created but no bidirectional Rhea → SwissLipids queries
- TODO: Implement Rhea dataset for biochemical reaction integration

**Uberon Tissues**:
- tissues.tsv file processed and Uberon dataset (ID 35) integrated ✅
- Bidirectional cross-references: SwissLipids ↔ Uberon tissue queries enabled
- Enables tissue-specific lipid discovery and anatomical localization

**Text Search**:
- Lipid names and abbreviations stored in attributes only
- Not currently indexed for full-text search
- Must use SwissLipids IDs directly for lookups

**Cross-Database Mappings**:
- ChEBI, HMDB, LIPID MAPS IDs extracted from lipids.tsv
- Cross-references created but require target datasets for bidirectional queries
- ChEBI (ID 10) already integrated in biobtree
- HMDB (ID 12) already integrated in biobtree
- LIPID MAPS (ID 33) already integrated in biobtree

## Future Work

**1. Hierarchical Classification** (Priority: High)
- Parse "Lipid class*" parent ID references
- Build hierarchical relationships (parent-child)
- Create derived datasets (swisslipidsparent, swisslipidschild)
- Enable class navigation and lipid family browsing

**2. Rhea Reaction Integration** (Priority: Medium)
- Implement Rhea dataset (biochemical reactions database)
- Enable bidirectional lipid ↔ reaction queries
- Support enzyme-catalyzed reaction discovery
- ~964 KB of enzyme/reaction data available

**3. Uberon Tissue Integration** ✅ COMPLETED
- Uberon dataset (anatomy ontology) now integrated (ID 35)
- Bidirectional lipid ↔ tissue queries enabled
- Tissue-specific lipid discovery supported
- ~95 KB of tissue annotation data processed

**4. Text Search Enhancement** (Priority: Low)
- Index lipid names for full-text search
- Index abbreviations for quick lookup
- Index synonyms for alternative name search

**5. Mass Spectrometry Data** (Priority: Low)
- Store exact m/z values in protobuf (currently in TSV only)
- Add attributes for [M+H]+, [M+Na]+, [M+K]+, [M-H]-, etc.
- Enable mass-based queries for metabolomics

**6. MetaNetX Integration** (Priority: Low)
- Create cross-references to MetaNetX metabolic network database
- Enable metabolic pathway discovery
- Support systems biology applications

## Maintenance

- **Release Schedule**: Monthly updates from SwissLipids
- **Data Format**: TSV files (gzip compressed or plain text)
- **Test Data**: 100 lipid entries spanning diverse lipid classes
- **License**: CC BY 4.0 (https://creativecommons.org/licenses/by/4.0/)

## References

- **SwissLipids Website**: https://www.swisslipids.org/
- **SwissLipids Downloads**: https://www.swisslipids.org/#/downloads
- **Publication**: Aimo L, et al. (2015) The SwissLipids knowledgebase for lipid biology. Bioinformatics. 31(17):2860-6.
- **Data License**: CC BY 4.0 - freely available with attribution

## Building with Test Data

```bash
# Build with SwissLipids test data (100 lipids, filtered xrefs)
./biobtree -d swisslipids test

# Build with full SwissLipids data (779,257 lipids)
./biobtree -d swisslipids build

# Copy test IDs to test infrastructure
cp test_out/reference/swisslipids_ids.txt tests/datasets/swisslipids/
```

## Query Examples

```bash
# Query lipid directly
curl "http://localhost:9292/ws/?i=SLM:000094711"
# Returns lipid entry with attributes and cross-references

# Get lipid name and properties
curl "http://localhost:9292/ws/entry/?i=SLM:000094711&s=swisslipids"
# Returns: name, abbreviation, chemical descriptors

# Traverse to UniProt proteins
curl "http://localhost:9292/ws/?i=SLM:000094711>>uniprot"
# Returns all proteins associated with this lipid

# Find lipids for a protein
curl "http://localhost:9292/ws/?i=P12345>>swisslipids"
# Returns all lipids interacting with this protein

# Traverse to GO terms
curl "http://localhost:9292/ws/?i=SLM:000094711>>go"
# Returns GO Biological Process terms for this lipid

# Find lipids in a GO process
curl "http://localhost:9292/ws/?i=GO:0008203>>swisslipids"
# Returns all lipids involved in this biological process

# Check evidence codes
curl "http://localhost:9292/ws/?i=SLM:000094711>>eco"
# Returns ECO evidence codes for data quality assessment

# Find tissue localization
curl "http://localhost:9292/ws/?i=SLM:000094711>>uberon"
# Returns Uberon anatomical locations where this lipid is found

# Find lipids in a tissue
curl "http://localhost:9292/ws/?i=UBERON:0000955>>swisslipids"
# Returns all lipids found in brain tissue
```

## Integration with LIPID MAPS

SwissLipids complements the already-integrated LIPID MAPS dataset:

- **LIPID MAPS** (ID 33): 49,065 entries, primarily experimental structures
- **SwissLipids** (ID 34): 779,257 entries, includes theoretical + experimental
- **Overlap**: Bidirectional cross-references via LIPID MAPS IDs in lipids.tsv
- **Complementary**: SwissLipids adds protein associations, GO terms, evidence codes

Both datasets can be queried together for comprehensive lipid coverage.
