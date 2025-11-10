# Reactome Test Dataset

## Overview

This directory contains test data for the Reactome Pathways dataset integration.

Reactome is a free, open-source, curated and peer-reviewed pathway database that provides intuitive bioinformatics tools for the visualization, interpretation and analysis of pathway knowledge to support basic research, genome analysis, modeling, systems biology and education.

## Test Data

### Source

The test data consists of pathway entries from multiple species, primarily Bos taurus (cattle, taxonomy ID: 9913), from Reactome release 89 (September 2025).

### Files

```
tests/reactome/
├── reactome_ids.txt                         # Test pathway IDs (50 IDs)
├── extract_reference_data.py                # Reference data extraction script
├── reference_data.json                      # Reactome API reference data (50 pathways)
├── test_reactome.py                         # Custom Python tests (12 tests)
├── test_cases.json                          # Declarative test cases (8 tests)
└── README.md                                # This file
```

### Dataset Statistics

- **Test Pathways**: 50 pathway IDs
- **Species Coverage**: Multi-species (primarily R-BTA-*, with R-HSA-*, R-MMU-* in test cases)
- **GO:BP Mappings**: 1,011 unique human pathway→GO term mappings (applied to all species)
- **Reference Data Source**: Reactome Content Service API (https://reactome.org/ContentService/)
- **API Endpoints Used**:
  - `/data/query/{id}` - Pathway details (name, species, etc.)
  - `/data/participants/{id}` - Participating molecules (proteins, compounds)
  - `/data/pathways/low/diagram/entity/{id}/allForms` - Hierarchy relationships

### Extracting Reference Data

Reference data is fetched from the official Reactome Content Service API:

```bash
cd tests/reactome
python3 extract_reference_data.py      # Extract all 50 IDs
python3 extract_reference_data.py 10   # Extract first 10 IDs only
```

The script:
1. Reads pathway IDs from `reactome_ids.txt`
2. Fetches pathway details via `/data/query/{id}`
3. Fetches participants via `/data/participants/{id}`
4. Fetches hierarchy via `/data/pathways/low/diagram/entity/{id}/allForms`
5. Saves complete API responses to `reference_data.json`

Rate limiting: 1.0 second per pathway + 0.5s per API call (~2 seconds per pathway, ~100 seconds for all 50 IDs)

## Reference Data Format

The `reference_data.json` file contains complete Reactome API responses for each test pathway:

```json
{
  "pathway_id": "R-HSA-177929",
  "pathway_details": {
    "stId": "R-HSA-177929",
    "displayName": "Signaling by EGFR",
    "schemaClass": "Pathway",
    "species": {
      "dbId": 48887,
      "displayName": "Homo sapiens",
      "taxId": "9606"
    }
  },
  "participants": [
    {
      "refEntity": {
        "identifier": "P00533",
        "databaseName": "UniProt"
      }
    },
    {
      "refEntity": {
        "identifier": "CHEBI:15996",
        "databaseName": "ChEBI"
      }
    }
  ],
  "hierarchy": {
    // Parent/child pathway relationships
  }
}
```

## Reactome Integration Architecture

### Storage Model

Reactome pathways are stored as a **dedicated dataset** with pathway IDs as primary keys:

```
Reactome Pathway Entry (e.g., R-HSA-177929)
  ├── pathway_id: "R-HSA-177929"
  ├── name: "Signaling by EGFR"
  ├── tax_id: 9606
  ├── is_disease_pathway: true/false ✅ NEW (2025-11-05)
  ├── alt_names: ["EGFR signaling", ...] (optional, for future)
  └── cross-references (with evidence codes ✅):
        ├── taxonomy: 9606
        ├── go: GO:0007173 (GO Biological Process) ✓
        ├── ensembl: ENSG00000157764, ... (genes) ✅ NEW (2025-11-05)
        ├── reactomeparent: R-HSA-162582 (parent pathways)
        ├── reactomechild: R-HSA-177929 (sub-pathways)
        ├── uniprot: P00533 [TAS], ... (with evidence codes) ✅ NEW (2025-11-05)
        └── chebi: CHEBI:15996 [IEA], ... (with evidence codes) ✅ NEW (2025-11-05)
```

**Key Features:**
- Pathway IDs follow format: R-{SPECIES}-{ID} (e.g., R-HSA-177929 for human, R-MMU-177929 for mouse)
- Species stored as taxonomy ID with cross-reference to taxonomy dataset
- Hierarchical organization via reactomeparent/reactomechild datasets
- Cross-references to proteins (UniProt) and compounds (ChEBI)
- Bidirectional linking automatically created by biobtree

### Species Support

Reactome supports 16 model organisms. The integration processes all species:

| Species Code | Common Name | Taxonomy ID |
|--------------|-------------|-------------|
| HSA | Homo sapiens (human) | 9606 |
| MMU | Mus musculus (mouse) | 10090 |
| RNO | Rattus norvegicus (rat) | 10116 |
| BTA | Bos taurus (cattle) | 9913 |
| SSC | Sus scrofa (pig) | 9823 |
| CFA | Canis familiaris (dog) | 9615 |
| GGA | Gallus gallus (chicken) | 9031 |
| DRE | Danio rerio (zebrafish) | 7955 |
| DME | Drosophila melanogaster (fruit fly) | 7227 |
| CEL | Caenorhabditis elegans (worm) | 6239 |
| DDI | Dictyostelium discoideum (slime mold) | 44689 |
| SCE | Saccharomyces cerevisiae (yeast) | 559292 |
| SPO | Schizosaccharomyces pombe (fission yeast) | 284812 |
| ATH | Arabidopsis thaliana (thale cress) | 3702 |
| OSA | Oryza sativa (rice) | 39947 |
| PFA | Plasmodium falciparum (malaria parasite) | 36329 |

### Hierarchy Datasets

Reactome pathways are organized hierarchically. Two derived datasets handle this:

- **reactomeparent** (ID 306): Parent pathways (broader categories)
- **reactomechild** (ID 307): Child pathways (sub-pathways)

These are defined in `conf/default.dataset.json` as derived datasets.

### Data Files Processed

The integration processes seven data files from Reactome:

1. **ReactomePathways.txt**: Pathway IDs, names, and species
2. **ReactomePathwaysRelation.txt**: Parent-child hierarchy
3. **UniProt2Reactome.txt**: Protein → pathway mappings
4. **ChEBI2Reactome.txt**: Compound → pathway mappings
5. **Pathways2GoTerms_human.txt**: GO Biological Process terms → pathways ✅
6. **Ensembl2Reactome.txt**: Gene → pathway mappings (NEW 2025-11-05) ✅
7. **HumanDiseasePathways.txt**: Disease pathway annotations (NEW 2025-11-05) ✅

All files are streamed directly from https://reactome.org/download/current/

## Usage

### Building with Test Data

```bash
# Build with Reactome test data (50 pathways, all species)
./biobtree -d "reactome" test

# Build with full Reactome data
./biobtree -d "reactome" build
```

### Query Examples

```bash
# Query pathway directly
curl "http://localhost:9292/ws/?i=R-HSA-177929"
# Returns pathway entry with attributes and cross-references

# Get pathway name and taxonomy
curl "http://localhost:9292/ws/entry/?i=R-HSA-177929&s=reactome"
# Returns: name="Signaling by EGFR", tax_id=9606

# Traverse to taxonomy
curl "http://localhost:9292/ws/?i=R-HSA-177929>>taxonomy"
# Returns: 9606 (Homo sapiens)

# Get parent pathways
curl "http://localhost:9292/ws/?i=R-HSA-177929>>reactomeparent"
# Returns parent pathway IDs

# Get child pathways
curl "http://localhost:9292/ws/?i=R-HSA-177929>>reactomechild"
# Returns sub-pathway IDs

# Find pathways for a protein
curl "http://localhost:9292/ws/?i=P00533>>reactome"
# Returns all pathways containing EGFR protein

# Find pathways for a compound
curl "http://localhost:9292/ws/?i=CHEBI:15996>>reactome"
# Returns all pathways containing this compound

# Find pathways for a gene (NEW: Ensembl integration)
curl "http://localhost:9292/ws/?i=ENSG00000157764>>reactome"
# Returns all pathways containing this gene

# Find genes in a pathway (NEW: Ensembl integration)
curl "http://localhost:9292/ws/?i=R-HSA-177929>>ensembl"
# Returns all genes participating in this pathway
```

## Test Cases

### 1. Basic Pathway Lookup

**Test**: Reactome pathways can be queried directly
- Query pathway ID (e.g., `R-HSA-177929`)
- Verify entry exists in reactome dataset (dataset ID 28)
- Check `name` and `tax_id` attributes

### 2. Pathway Attributes

**Test**: Pathways have complete attribute information
- Every pathway should have a `name` attribute (non-empty string)
- Every pathway should have a `tax_id` attribute (integer)
- Attributes accessible via `Attributes.Reactome.name` and `Attributes.Reactome.tax_id`
- `alt_names` field available for future population

### 3. Taxonomy Cross-References

**Test**: Pathways link to taxonomy dataset
- Each pathway has a cross-reference to taxonomy (dataset ID 3)
- The taxonomy ID matches the `tax_id` attribute
- Taxonomy entry can be retrieved via traversal

### 4. GO Biological Process Cross-References ✓ NEW

**Test**: Pathways link to GO Biological Process terms
- Pathways have cross-references to GO dataset (dataset ID 4)
- GO terms follow format `GO:XXXXXXX`
- GO mappings applied to all species (from human pathway mappings)
- Bidirectional queries work: `pathway → go` and `GO:XXXXXXX → reactome`

### 5. UniProt Cross-References

**Test**: Pathways link to participating proteins
- Pathways have cross-references to UniProt dataset (dataset ID 1)
- UniProt IDs can be used to find pathways (bidirectional)
- Count of proteins varies by pathway

### 6. ChEBI Cross-References (Optional)

**Test**: Some pathways link to participating compounds
- Pathways may have cross-references to ChEBI dataset (dataset ID 10)
- Not all pathways have compounds (optional relationship)
- ChEBI IDs can be used to find pathways (bidirectional)

### 7. Ensembl Cross-References ✅ NEW (2025-11-05)

**Test**: Pathways link to participating genes
- Pathways have cross-references to Ensembl dataset (dataset ID 2)
- Ensembl gene IDs (e.g., ENSG00000157764) can be used to find pathways (bidirectional)
- Provides gene-level pathway queries (complementing protein-level via UniProt)
- 1.1M gene → pathway mappings (3.6x more than UniProt)

### 8. Disease Pathway Attribute ✅ NEW (2025-11-05)

**Test**: Pathways have disease annotation attribute
- `is_disease_pathway` boolean attribute indicates if pathway is disease-related
- Attribute accessible via `Attributes.Reactome.is_disease_pathway`
- 763 pathways marked as disease-related (from HumanDiseasePathways.txt)
- Applied to all species via numeric pathway ID mapping

### 6. Pathway Hierarchy - Parent

**Test**: Parent-child relationships work
- Pathways have cross-references to reactomeparent dataset (ID 306)
- Parent pathway IDs can be retrieved
- Parent pathways are valid Reactome entries

### 7. Pathway Hierarchy - Child

**Test**: Child pathway relationships work
- Pathways have cross-references to reactomechild dataset (ID 307)
- Child pathway IDs can be retrieved
- Child pathways are valid Reactome entries

### 9. Multi-Species Support

**Test**: Pathways from different species are supported
- Pathway IDs contain species codes (R-HSA, R-MMU, R-BTA, etc.)
- Each species has correct taxonomy ID mapping
- Species-specific pathways can be queried

### 10. Evidence Codes in Cross-References ✅ NEW (2025-11-05)

**Test**: Cross-references include evidence codes for curation quality
- UniProt, ChEBI, and Ensembl xrefs have evidence codes (TAS, IEA, IEP)
- Evidence codes accessible via `xrefs[].evidence` field in API responses
- Evidence describes quality of each specific relationship assertion
- Same entity can have multiple evidence codes for different assertions

### 11. Evidence Code Coverage ✅ NEW (2025-11-05)

**Test**: Evidence codes have comprehensive coverage
- 99.3% of xrefs have evidence codes (1320/1329 xrefs)
- Missing evidence only for taxonomy and hierarchy relationships (which don't have evidence in source files)
- All biological cross-references (UniProt, ChEBI, Ensembl) include evidence codes

## Performance Notes

- **Full Reactome**: ~23,157 pathways (all 16 species)
- **Test dataset**: 50 pathways
- **Processing time**: ~3 seconds for test data, ~15 seconds for full data (streaming download + processing)
- **Mappings**:
  - ~18,000 GO:BP → Reactome cross-references ✓
  - ~1,126,782 Ensembl → Reactome cross-references ✅ NEW (2025-11-05)
  - 313,472 UniProt → Reactome cross-references
  - 109,788 ChEBI → Reactome cross-references
  - 23,259 hierarchy relationships
- **Disease annotations**: 763 pathways marked as disease-related ✅ NEW (2025-11-05)
- **Total xrefs**: ~1,590,000 (242% increase from Ensembl integration!) 🚀
- **Evidence codes**: 99.3% coverage (1320/1329 xrefs tested) ✅ NEW (2025-11-05)

## Evidence Codes Feature ✅

**Status**: Fully implemented as of 2025-11-05

Evidence codes indicate the quality and source of each cross-reference assertion in Reactome:

### Evidence Code Types

- **TAS** (Traceable Author Statement): Manually curated by Reactome experts
  - High confidence, peer-reviewed assertions
  - Based on published literature and expert knowledge

- **IEA** (Inferred by Electronic Annotation): Computationally inferred
  - Automated projections from other species or datasets
  - Lower confidence but broader coverage

- **IEP** (Inferred from Expression Pattern): Inferred from expression data
  - Based on gene/protein expression patterns
  - Less common in the dataset

### Implementation Details

Evidence codes are stored in the `XrefEntry` protobuf message (app.proto):

```protobuf
message XrefEntry {
  uint32 dataset = 1;
  string identifier = 2;
  string evidence = 3;  // Evidence code (TAS, IEA, IEP)
}
```

**Key characteristics:**
- Evidence is **per-relationship**, not per-entity
- Same protein can have multiple evidence codes in the same pathway (e.g., both TAS and IEA)
- Evidence describes the quality of each specific cross-reference assertion
- Stored in kvdata files as 5th optional field: `key\tdb\tvalue\tvaluedb\tevidence`

### Storage Format

Tab-delimited kvdata files:
```
R-HSA-177929    reactome    P00533    uniprot    TAS
R-HSA-177929    reactome    P00533    uniprot    IEA
R-HSA-177929    reactome    CHEBI:15996    chebi    IEA
```

### API Usage

Evidence codes appear in the xrefs array of query results:

```bash
curl "http://localhost:9292/ws/entry/?i=R-BTA-73843&s=reactome" | jq '.xrefs'
```

Example response:
```json
"xrefs": [
  {
    "dataset": "uniprot",
    "identifier": "P01111",
    "evidence": "TAS"
  },
  {
    "dataset": "chebi",
    "identifier": "CHEBI:15996",
    "evidence": "IEA"
  }
]
```

### Coverage Statistics

From test suite validation:
- **Total xrefs tested**: 1,329
- **Xrefs with evidence codes**: 1,320
- **Coverage**: 99.3%
- **Missing evidence**: 9 xrefs (taxonomy and hierarchy relationships, which don't have evidence codes in source files)

### Bug Fix History

**Issue discovered**: After building full Reactome dataset, evidence codes were missing from 99% of xrefs (only 51/1329 had codes).

**Root cause**: In `src/generate/mergeg.go` lines 782-788, when creating `kvMessage` from parsed line data, the evidence field was not being included:

```go
// BEFORE (buggy):
*ch.d.mergeCh <- kvMessage{
    key:     line[0],
    db:      line[1],
    value:   line[2],
    valuedb: line[3],
    // Missing: evidence: line[4]
}

// AFTER (fixed):
*ch.d.mergeCh <- kvMessage{
    key:      line[0],
    db:       line[1],
    value:    line[2],
    valuedb:  line[3],
    evidence: line[4],  // Include evidence field (optional 5th field)
}
```

**Result**: Evidence code coverage increased from 3.8% to 99.3%.

### Test Coverage

Two comprehensive tests validate evidence codes:

1. **test_evidence_codes_in_xrefs** (Test 11): Verifies evidence codes exist for UniProt/ChEBI/Ensembl xrefs
2. **test_evidence_codes_complete_coverage** (Test 12): Ensures all expected xrefs have evidence codes

See tests/reactome/test_reactome.py for implementation details.

## Known Limitations

1. **Alternative names**: The `alt_names` protobuf field is defined but not yet populated. Future enhancement will populate this from pre-generated file (see Future Work section below).

2. **GO Cellular Component**: GO:CC compartment mappings are available via API but not yet integrated (future enhancement).

3. **Pathway compartments**: Reactome pathways can occur in specific cellular compartments. This information is available via the API but not currently stored in the integration.

4. **Reactions and events**: Reactome contains detailed reaction mechanisms and events. The current integration focuses on pathway-level data only.

## Future Work

The following enhancements have been identified for future implementation:

### 1. OMIM Disease Mappings

**Status**: Deferred (2025-11-05)

Reactome provides a file `Reactome2OMIM.txt` that maps pathways to disease phenotypes. Initial investigation revealed this file contains UniProt → Pathway mappings rather than actual OMIM IDs:

```
Format: UniProt_ACC  Reactome_Pathway_ID  URL  Event_Name  Evidence  Species
Example: P35232  R-HSA-983168  https://reactome.org/...  Antigen processing...  TAS  Homo sapiens
```

**Implementation considerations:**
- Need to clarify the actual OMIM → Pathway mapping source
- OMIM is already in biobtree default dataset as `mim` (dataset name)
- Could provide disease phenotype → pathway associations for clinical research
- Requires further analysis of Reactome data files or API endpoints

**Priority**: Medium - useful for clinical applications but not critical for core pathway functionality

### 2. Alternative Pathway Names

**Status**: Not yet implemented

The `alt_names` protobuf field is defined but not populated. Implementation plan:

- Populate from pre-generated file or API data
- Store synonyms and alternative pathway names for improved searchability
- See `analysis/reactome/REACTOME_ENHANCEMENTS.md` for detailed implementation plan

**Priority**: Low - nice to have but not essential for current use cases

### 3. GO Cellular Component Mappings

**Status**: Not yet implemented

GO:CC compartment mappings are available via Reactome API but not yet integrated:

- Would add subcellular localization context to pathway annotations
- Complements existing GO:BP (Biological Process) mappings
- Available via `/data/participants/{id}` API endpoint (compartment field)

**Priority**: Low - GO:BP provides core pathway functionality

### 4. Pathway Compartments

**Status**: Not yet implemented

Reactome pathways can occur in specific cellular compartments (cytoplasm, nucleus, membrane, etc.):

- Compartment information available via API
- Could be stored as pathway attribute or separate xref
- Provides subcellular context for pathway interpretation

**Priority**: Low - specialized use case

### 5. Reaction and Event Details

**Status**: Out of scope for current integration

Reactome contains detailed reaction mechanisms, catalytic events, and molecular interactions:

- Current integration focuses on pathway-level data only
- Full reaction/event integration would require significant schema changes
- Pathway-level data is sufficient for most biological interpretation use cases

**Priority**: Very Low - would require major architectural changes

## References

- **Reactome Database**: https://reactome.org/
- **Reactome Downloads**: https://reactome.org/download/current/
- **Reactome Content Service API**: https://reactome.org/ContentService/
- **Publication**: Gillespie M, et al. (2022) The reactome pathway knowledgebase 2022. Nucleic Acids Res. 50(D1):D687-D692.
- **Data License**: CC BY 4.0

## Maintenance

To update the test dataset with newer Reactome versions:

1. Run test mode to generate new pathway IDs: `./biobtree -d "reactome" test`
2. Copy new IDs: `cp test_out/reference/reactome_ids.txt tests/reactome/`
3. Re-run `extract_reference_data.py` to update expected results
4. Verify tests pass with new data
5. Update version number in `test_cases.json` if needed

Last updated: 2025-11-05 (Added Ensembl2Reactome, HumanDiseasePathways, and Evidence Codes features)
