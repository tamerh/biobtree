# STRING Test Dataset

## Overview

This directory contains test data for the STRING (Search Tool for the Retrieval of Interacting Genes/Proteins) dataset integration.

STRING is a database of known and predicted protein-protein interactions, including direct (physical) and indirect (functional) associations derived from computational prediction, knowledge transfer between organisms, and interactions aggregated from other (primary) databases.

## Test Data

### Source

The test data consists of a subset of human (Homo sapiens, taxonomy ID: 9606) protein interactions from STRING version 12.0.

### Files

```
tests/string/
├── string_ids.txt                           # Test UniProt IDs (50 IDs)
├── extract_reference_data.py                # Reference data extraction script
├── reference_data.json                      # STRING API reference data (50 entries, 500 interactions)
├── test_string.py                           # Custom Python tests (5 tests)
├── test_cases.json                          # Declarative test cases (8 tests)
└── README.md                                # This file
```

### Dataset Statistics

- **Test Proteins**: 50 UniProt IDs
- **Interactions**: 500 total (10 per protein from STRING API)
- **Taxonomy**: Human (9606) only
- **Score Threshold**: 400 (default)
- **Evidence Types**: experimental, database, textmining, coexpression, etc.
- **Reference Data Source**: STRING REST API (https://string-db.org/api)

### Extracting Reference Data

Reference data is fetched from the official STRING REST API:

```bash
cd tests/string
python3 extract_reference_data.py      # Extract all 50 IDs
python3 extract_reference_data.py 10   # Extract first 10 IDs only
```

The script:
1. Reads UniProt IDs from `string_ids.txt`
2. Maps each to STRING IDs via `/api/json/get_string_ids`
3. Fetches interaction partners via `/api/json/interaction_partners`
4. Saves complete API responses to `reference_data.json`

Rate limiting: 1.2 seconds per ID (~60 seconds for all 50 IDs)

## Reference Data Format

The `reference_data.json` file contains complete STRING API responses for each test protein:

```json
{
  "uniprot_id": "Q75LH1",
  "string_mapping": {
    "stringId": "9606.ENSP00000384700",
    "preferredName": "PAPOLB",
    "annotation": "poly(A) polymerase beta."
  },
  "interactions": [
    {
      "stringId_A": "9606.ENSP00000384700",
      "stringId_B": "9606.ENSP00000230640",
      "preferredName_B": "MTREX",
      "score": 0.995,      // Combined score (0-1)
      "escore": 0.135,     // Experimental evidence
      "dscore": 0,         // Database evidence
      "tscore": 0.994      // Text mining evidence
      // ... other evidence scores
    }
  ]
}
```

## STRING Integration Architecture

### Storage Model

STRING data is stored as a **dedicated dataset** with STRING IDs as primary keys:

```
STRING Entry (e.g., 9606.ENSP00000000233)
  ├── string_id: "9606.ENSP00000000233"
  ├── organism_taxid: 9606
  ├── preferred_name: "ARF4"
  ├── protein_size: 180
  ├── annotation: "ADP-ribosylation factor 4"
  └── interactions: [
        {
          partner: "P26437",  // UniProt AC
          score: 173,
          has_experimental: true,
          has_database: false,
          has_textmining: true,
          has_coexpression: true
        },
        ...
      ]
```

**Key Features:**
- STRING IDs (e.g., `9606.ENSP00000000233`) are the primary entry keys
- UniProt IDs (e.g., `P26437`) are keywords that link to STRING entries
- Both STRING IDs and UniProt IDs can be used to query STRING data
- Interaction partners are stored as UniProt IDs for cross-dataset linking

### Evidence Channels

STRING provides multiple evidence types for each interaction:

| Channel | Description | Source |
|---------|-------------|--------|
| **neighborhood** | Gene neighborhood | Genomic context |
| **fusion** | Gene fusion events | Protein fusions |
| **cooccurrence** | Phylogenetic profiles | Co-occurrence across genomes |
| **coexpression** | Gene co-expression | Expression data |
| **experimental** | Experimentally determined | Literature, databases |
| **database** | Curated databases | Pathway databases |
| **textmining** | Text mining | PubMed abstracts |
| **combined_score** | Integrated score | Combination of all channels |

### Score Threshold

The default score threshold is **400** (out of 1000), providing a balance between coverage and quality. This can be configured in `conf/source.dataset.json`.

## Usage

### Building with Test Data

```bash
# Build with human STRING test data
./biobtree -d "uniprot,string" --tax 9606 build

# Build with test mode (limits to 50 entries per dataset)
./biobtree -d "uniprot,string" --tax 9606 -testmode build
```

### Query Examples

```bash
# Query by STRING ID directly
curl "http://localhost:8888/ws/?i=9606.ENSP00000000233"
# Returns STRING dataset entry with interactions

# Query by UniProt ID (resolves to STRING via keyword)
curl "http://localhost:8888/ws/?i=P26437"
# Returns STRING dataset entry (if P26437 has STRING data)

# Get full STRING entry with interactions
curl "http://localhost:8888/ws/entry/?i=9606.ENSP00000000233&s=string"
# Returns complete STRING data with protein attributes and interactions

# Query chain: HGNC → UniProt → STRING
# (UniProt IDs serve as keywords to STRING entries)
```

## Test Cases

### 1. Basic Dataset Lookup

**Test**: STRING entries can be queried directly
- Query STRING ID (e.g., `9606.ENSP00000384700`)
- Verify entry exists in STRING dataset (dataset ID 27)
- Check `string_id`, `organism_taxid`, `preferred_name` fields
- Verify `interactions` array is populated

### 2. Interaction Bidirectionality

**Test**: Interactions are bidirectional (when both proteins are in dataset)
- If protein A has partner B in interactions
- Partner B should also have interaction data (if present in dataset)
- Partners are stored as UniProt IDs for cross-dataset linking

### 3. Evidence Channel Flags

**Test**: Evidence booleans are set when available
- Check `has_experimental`, `has_database`, etc. when present in source
- Note: Some interactions may only have `score` without individual flags
- This is valid STRING data (combined score without breakdown)

### 4. UniProt Keyword Lookup

**Test**: UniProt IDs resolve to STRING entries
- Query by UniProt ID (e.g., `Q75LH1`)
- Should return STRING dataset entry (via keyword link)
- UniProt IDs from interaction partners should be queryable

### 5. Score Filtering

**Test**: Only interactions above threshold are stored
- With threshold=400, verify no interactions with score < 400
- Count interactions matches expected for threshold
- Score range: 0-1000

### 6. Cross-references

**Test**: Both STRING and UniProt IDs work as entry points
- STRING ID query: Returns STRING entry directly
- UniProt ID query: Returns STRING entry via keyword (if linked)

## Multi-Organism Support

While this test dataset only includes human (9606), the STRING integration supports multiple organisms:

```bash
# Human + Mouse + Rat
./biobtree -d "string" --tax 9606,10090,10116 build

# All model organisms
./biobtree -d "string" --tax 9606,10090,10116,4932,7227,6239 build
```

Each organism's data is processed independently and stored with its taxonomy ID.

## Performance Notes

- **Full STRING (human)**: ~19,699 proteins, ~13.7M interactions, ~800 MB compressed
- **Test dataset**: 501 proteins, 500 interactions, <5 MB
- **Processing time**: ~1-2 seconds for test data (streaming download + processing)

## Known Limitations

1. **One-to-many mappings**: Some STRING proteins map to multiple UniProt ACs (isoforms). The current implementation creates separate entries for each.

2. **Organism isolation**: Interactions are strictly intra-organism. Cross-organism queries require ortholog mappings (via Ensembl).

3. **Evidence details**: Individual evidence scores are not stored, only boolean flags (present/absent). The `combined_score` is the authoritative confidence measure.

## References

- **STRING Database**: https://string-db.org/
- **STRING Downloads**: https://stringdb-downloads.org/
- **Publication**: Szklarczyk D, et al. (2023) STRING v12: protein-protein association networks with increased coverage, supporting functional discovery in genome-wide experimental datasets. Nucleic Acids Res. 51(D1):D638-D646.
- **Data License**: CC BY 4.0

## Maintenance

To update the test dataset with newer STRING versions:

1. Update download URLs in the data extraction commands
2. Regenerate test files using the provided scripts
3. Re-run `extract_reference_data.py` to update expected results
4. Verify tests pass with new data

Last updated: 2025-11-04
