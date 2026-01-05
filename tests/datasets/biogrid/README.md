# BioGRID Dataset Integration Tests

## Overview
BioGRID (Biological General Repository for Interaction Datasets) is a curated database of protein-protein and genetic interactions extracted from the biomedical literature.

**Dataset ID:** 105
**Source:** https://downloads.thebiogrid.org/
**Format:** PSI-MITAB 2.5 (ZIP compressed)

## Data Size

| Metric | Value |
|--------|-------|
| Compressed (ZIP) | ~170 MB |
| Uncompressed | ~2.1 GB |
| Total Interactions | ~2.8 million |

## Data Structure

### Primary Identifier
- **BioGRID Interactor ID**: Numeric identifier (e.g., "112315", "106603")
- Found in Alt IDs column (column 2/3) as "biogrid:NNNNNN"

### Attributes (BiogridAttr)
| Field | Type | Description |
|-------|------|-------------|
| biogrid_id | string | Primary BioGRID interactor ID |
| interactions | repeated BiogridInteraction | List of interactions |
| interaction_count | int32 | Total number of interactions |
| unique_partners | int32 | Number of unique interaction partners |
| physical_count | int32 | Count of physical interactions |
| genetic_count | int32 | Count of genetic interactions |
| organisms | repeated int32 | Unique taxonomy IDs involved |
| experimental_systems | repeated string | Experimental methods used |
| pubmed_ids | repeated string | Supporting PubMed references |

### Interaction Details (BiogridInteraction)
| Field | Type | Description |
|-------|------|-------------|
| interaction_id | string | BioGRID interaction ID |
| partner_biogrid_id | string | Partner's BioGRID ID |
| partner_entrez_id | string | Partner's Entrez Gene ID |
| partner_symbol | string | Partner's gene symbol |
| experimental_system | string | Experimental method (e.g., "two hybrid") |
| experimental_system_type | string | "physical" or "genetic" |
| organism_a | int32 | Taxonomy ID for interactor A |
| organism_b | int32 | Taxonomy ID for interactor B |
| pubmed_id | string | Supporting PubMed ID |
| author | string | First author and year |
| score | string | Confidence score (if available) |
| source_database | string | Original source database |

## Cross-References Created

All cross-references are **bidirectional** (created using `addXref` which automatically generates forward and reverse mappings).

| Target Dataset | Description | Source Column |
|----------------|-------------|---------------|
| biogrid | Links to interaction partners | Parsed interactions |
| entrez | Links to Entrez Gene IDs | Column 0/1 (Primary IDs) |
| uniprot | Links to UniProt accessions | Column 2/3 (Alt IDs: `uniprot/swiss-prot:`) |
| refseq | Links to RefSeq IDs | Column 2/3 (Alt IDs: `refseq:`) |
| taxonomy | Links to organism taxonomy IDs | Column 9/10 |
| pubmed | Links to supporting publications | Column 8 |

### Text Search
- BioGRID ID (searchable)
- Gene symbols (searchable)

## Sample Queries

```bash
# Search by BioGRID ID
curl "http://localhost:9292/ws/?i=112315"

# Map from BioGRID to Entrez Gene
curl "http://localhost:9292/ws/map/?i=112315&m=>>biogrid>>entrez"

# Map from Entrez Gene to BioGRID
curl "http://localhost:9292/ws/map/?i=6416&m=>>entrez>>biogrid"

# Map from BioGRID to UniProt
curl "http://localhost:9292/ws/map/?i=112315&m=>>biogrid>>uniprot"

# Map from UniProt to BioGRID (find interactions for a protein)
curl "http://localhost:9292/ws/map/?i=P45985&m=>>uniprot>>biogrid"

# Map from BioGRID to RefSeq
curl "http://localhost:9292/ws/map/?i=112315&m=>>biogrid>>refseq"

# Map from RefSeq to BioGRID
curl "http://localhost:9292/ws/map/?i=NP_003001&m=>>refseq>>biogrid"

# Filter physical interactions only
curl "http://localhost:9292/ws/map/?i=112315&m=>>biogrid[biogrid.physical_count > 0]"

# Filter genetic interactions only
curl "http://localhost:9292/ws/map/?i=112315&m=>>biogrid[biogrid.genetic_count > 0]"
```

## Test Coverage

### test_cases.json
Declarative tests for:
- Basic BioGRID ID lookup
- BioGRID to Entrez mapping
- BioGRID to UniProt mapping
- BioGRID to RefSeq mapping
- UniProt to BioGRID reverse lookup
- BioGRID to PubMed mapping
- BioGRID to Taxonomy mapping
- Bidirectional partner lookups
- Attribute field validation
- Filter expression testing (physical_count)

### test_biogrid.py
Custom tests for:
- Interaction attribute completeness
- Cross-reference count validation
- Filter expression testing

## Known Limitations

1. **Large Dataset**: The full BioGRID file is ~170MB compressed, ~2GB uncompressed
2. **Human Focus in Test Mode**: Test mode only processes human-human interactions (taxid:9606)
3. **Interaction Grouping**: Interactions are grouped by interactor, so the same interaction appears under both interactors
4. **PubMed ID Limit**: Only first 10 PubMed IDs are cross-referenced per interactor to avoid excessive xrefs
5. **Partner Limit**: Only first 100 interaction partners are cross-referenced per interactor

## Data Source Details

**URL:** https://downloads.thebiogrid.org/Download/BioGRID/Latest-Release/BIOGRID-ALL-LATEST.mitab.zip

**File Format:** PSI-MITAB 2.5 (tab-separated values)

| Column | Field | Extracted Data |
|--------|-------|----------------|
| 0 | ID(s) interactor A | Entrez Gene ID (`entrez gene/locuslink:NNNN`) |
| 1 | ID(s) interactor B | Entrez Gene ID (partner) |
| 2 | Alt. ID(s) interactor A | BioGRID ID, UniProt, RefSeq |
| 3 | Alt. ID(s) interactor B | BioGRID ID, UniProt, RefSeq (partner) |
| 4 | Alias(es) interactor A | Gene symbol |
| 5 | Alias(es) interactor B | Gene symbol (partner) |
| 6 | Interaction detection method | Experimental system |
| 7 | Publication 1st author | Author |
| 8 | Publication Identifier(s) | PubMed ID |
| 9 | Taxid interactor A | Taxonomy ID |
| 10 | Taxid interactor B | Taxonomy ID (partner) |
| 11 | Interaction type(s) | Physical/Genetic classification |
| 12 | Source database(s) | Source database |
| 13 | Interaction identifier(s) | Interaction ID |
| 14 | Confidence value(s) | Score |

## Update Frequency
BioGRID releases monthly updates. Check https://thebiogrid.org/ for the latest release.
