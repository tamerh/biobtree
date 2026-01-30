# BioGRID Dataset

## Overview
BioGRID (Biological General Repository for Interaction Datasets) is a curated database of protein-protein and genetic interactions extracted from the biomedical literature. Contains 2.8M+ interactions from 80,000+ publications covering multiple organisms.

**Source**: https://thebiogrid.org/
**Data Type**: Protein-protein and genetic interactions with experimental evidence

## Dual Dataset Architecture

BioGRID data is stored in **two complementary datasets**:

1. **biogrid** - Lightweight summary entries for each interactor (statistics only)
2. **biogrid_interaction** - Individual interaction records with full experimental details

This architecture solves the "hub protein problem" where highly connected proteins (like UBC, TP53) would otherwise have extremely large response sizes (5-8 MB).

---

## Dataset 1: biogrid (Summary)

### Storage Model
**Primary Entries**: BioGRID Interactor IDs (numeric, e.g., "112315", "108607")
**Searchable Text Links**: BioGRID ID, gene symbols
**Purpose**: Quick access to interaction statistics without loading all interaction details

### Attributes (BiogridAttr)
| Field | Type | Description |
|-------|------|-------------|
| biogrid_id | string | Primary BioGRID interactor ID |
| interaction_count | int32 | Total number of interactions |
| unique_partners | int32 | Number of unique interaction partners |
| physical_count | int32 | Count of physical interactions |
| genetic_count | int32 | Count of genetic interactions |
| organisms | repeated int32 | Unique taxonomy IDs involved |
| experimental_systems | repeated string | Experimental methods used |
| pubmed_ids | repeated string | Supporting PubMed references |

### Cross-References (biogrid)
| Target Dataset | Description |
|----------------|-------------|
| biogrid_interaction | Links to individual interaction records |
| entrez | Links to Entrez Gene IDs |
| uniprot | Links to UniProt accessions (Swiss-Prot + TrEMBL) |
| refseq | Links to RefSeq IDs |
| taxonomy | Links to organism taxonomy IDs |
| pubmed | Links to supporting publications |

---

## Dataset 2: biogrid_interaction (Details)

### Storage Model
**Primary Entries**: BioGRID Interaction IDs (numeric, e.g., "103", "456789")
**Purpose**: Full experimental details for each interaction

### Attributes (BiogridInteractionAttr)
| Field | Type | Description |
|-------|------|-------------|
| interaction_id | string | BioGRID interaction ID |
| experimental_system | string | Experimental method (e.g., "Two-hybrid", "Affinity Capture-MS") |
| experimental_system_type | string | "physical" or "genetic" |
| author | string | First author and year |
| publication | string | PubMed ID |
| throughput | string | "Low Throughput", "High Throughput", or "Both" |
| score | string | Confidence score (if available) |
| modification | string | Post-translational modification |
| qualifications | string | Additional qualifications |
| tags | string | Curation tags |
| source_database | string | Original source database |
| interactor_a_id | string | Interactor A's identifier (UniProt/Entrez) |
| interactor_a_symbol | string | Interactor A's gene symbol |
| interactor_a_organism | int32 | Interactor A's NCBI Taxonomy ID |
| interactor_b_id | string | Interactor B's identifier (UniProt/Entrez) |
| interactor_b_symbol | string | Interactor B's gene symbol |
| interactor_b_organism | int32 | Interactor B's NCBI Taxonomy ID |
| phenotype | string | Phenotype term (genetic interactions) |
| ontology_term_id | string | Ontology term ID (GO, HP) |

### Cross-References (biogrid_interaction)
| Target Dataset | Description |
|----------------|-------------|
| biogrid | Links to both interactor summary entries |
| uniprot | Links to both interactors (if UniProt IDs available) |
| entrez | Links to both interactors (if Entrez IDs available) |
| pubmed | Links to supporting publication |
| taxonomy | Links to organism taxonomy IDs |

---

## Query Patterns

### Direct Queries
```bash
# Get summary for an interactor
curl "http://localhost:9292/ws/entry/?i=112315&d=biogrid"

# Get interaction details
curl "http://localhost:9292/ws/entry/?i=103&d=biogrid_interaction"
```

### Mapping Queries
```bash
# Protein to BioGRID summary statistics
curl "http://localhost:9292/ws/map/?i=P04637&m=>>uniprot>>biogrid"

# Protein to individual interactions (direct)
curl "http://localhost:9292/ws/map/?i=P04637&m=>>uniprot>>biogrid_interaction"

# Protein → Summary → Interactions (chained)
curl "http://localhost:9292/ws/map/?i=P04637&m=>>uniprot>>biogrid>>biogrid_interaction"
```

### Filtering Examples
```bash
# Physical interactions only
curl "http://localhost:9292/ws/map/?i=P04637&m=>>uniprot>>biogrid_interaction[biogrid_interaction.experimental_system_type=='physical']"

# Two-hybrid experiments only
curl "http://localhost:9292/ws/map/?i=P04637&m=>>uniprot>>biogrid_interaction[biogrid_interaction.experimental_system=='Two-hybrid']"

# Human interactions only
curl "http://localhost:9292/ws/map/?i=P04637&m=>>uniprot>>biogrid_interaction[biogrid_interaction.interactor_a_organism==9606]"

# Low throughput (high confidence)
curl "http://localhost:9292/ws/map/?i=P04637&m=>>uniprot>>biogrid_interaction[biogrid_interaction.throughput=='Low Throughput']"
```

---

## Special Features
- **Dual Dataset Model**: Summary stats (biogrid) + full details (biogrid_interaction)
- **Efficient Hub Proteins**: TP53 summary is small; individual interactions fetched on demand
- **TAB3 Format**: Uses BioGRID TAB3 format (37 columns) for comprehensive data extraction
- **Throughput Classification**: Distinguishes low-throughput (high confidence) vs high-throughput experiments
- **Genetic Interaction Details**: Captures modification types (Synthetic Lethality, Dosage Rescue, etc.)
- **Bidirectional Interactions**: Each interaction links to both interactors
- **Phenotype Ontology**: Links to ontology terms for phenotype data

## Use Cases

**1. Get Interaction Statistics for a Protein**
```
Query: UniProt ID → biogrid → Summary stats (interaction_count, physical_count, etc.)
Use: Quick overview of interaction landscape without loading all details
```

**2. Find Detailed Interaction Partners**
```
Query: UniProt ID → biogrid_interaction → Full interaction details
Use: Get experimental methods, authors, PubMed references for each interaction
```

**3. Validate Drug Target Interactions (High Confidence)**
```
Query: UniProt ID → biogrid_interaction[throughput=='Low Throughput']
Use: Filter for manually validated, high-confidence interactions
```

**4. Identify Synthetic Lethal Partners**
```
Query: Gene → biogrid_interaction[experimental_system_type=='genetic']
Use: Find genetic interactions for combination therapy opportunities
```

**5. Literature Evidence Lookup**
```
Query: biogrid_interaction → pubmed
Use: Retrieve supporting publications for specific interactions
```

**6. Cross-Species Interaction Comparison**
```
Query: Gene → biogrid_interaction[interactor_a_organism==9606 && interactor_b_organism==10090]
Use: Find human-mouse cross-species interactions
```

**7. Hub Protein Analysis**
```
Query: UniProt ID → biogrid → Check unique_partners count
Use: Quickly identify highly connected proteins without loading all interactions
```

## Test Cases

### Declarative Tests (test_cases.json)

**biogrid dataset:**
- Basic lookup by BioGRID ID
- Attribute validation (biogrid_id, interaction_count, unique_partners, physical_count, genetic_count)
- Cross-references: entrez, uniprot, refseq, pubmed, taxonomy
- Reverse mappings: uniprot→biogrid, entrez→biogrid
- Filter by physical_count

**biogrid_interaction dataset:**
- Basic lookup by interaction ID
- Attribute validation (interaction_id, experimental_system, experimental_system_type)
- Interactor fields (interactor_a_id, interactor_a_symbol, interactor_b_id, interactor_b_symbol)
- Cross-references: biogrid_interaction→uniprot, biogrid_interaction→pubmed
- Filter by experimental_system_type

### Custom Tests (test_biogrid.py)

**biogrid tests:**
- Summary statistics validation (interaction_count > 0)
- Physical vs genetic counts consistency
- Cross-reference to biogrid_interaction

**biogrid_interaction tests:**
- Both interactors present (interactor_a_id, interactor_b_id)
- Experimental system type is valid ("physical" or "genetic")
- Throughput field validation
- Protein mapping: uniprot → biogrid_interaction
- Chained mapping: uniprot → biogrid → biogrid_interaction

**Coverage**:
- Both datasets (biogrid and biogrid_interaction)
- All major attributes
- Cross-reference mappings in both directions
- CEL filtering on both datasets

## Performance

- **Test Build**: ~30s (100 interactions, human-human only in test mode)
- **Data Source**: https://downloads.thebiogrid.org/Download/BioGRID/Latest-Release/BIOGRID-ALL-LATEST.tab3.zip
- **Update Frequency**: Monthly releases
- **Total Entries**:
  - ~200,000 interactor summaries (biogrid)
  - ~1,800,000 individual interactions (biogrid_interaction)
- **Response Sizes**:
  - biogrid summary: ~1-2 KB per entry (statistics only)
  - biogrid_interaction: ~1-2 KB per interaction
  - Hub proteins (TP53, UBC): Fast summaries, details fetched on demand

## Known Limitations

1. **Dual Dataset Requirement**: Both biogrid and biogrid_interaction are created together (`./biobtree -d biogrid update`)
2. **Human Focus in Test Mode**: Test mode only processes human-human interactions (taxid:9606)
3. **Score Availability**: Not all interactions have confidence scores
4. **Phenotype Data**: Only available for some genetic interactions
5. **Cross-Species**: Some interactions have different organisms for interactor A and B

## Comparison with IntAct

| Feature | BioGRID | IntAct |
|---------|---------|--------|
| Evidence | Experimental | Experimental |
| Curation | Literature-based | Manual IMEx standard |
| Data Model | Dual (summary + interactions) | Interaction-centric |
| Genetic Interactions | Yes (extensive) | Limited |
| Confidence Scores | Some | MIscore (all) |
| Update Frequency | Monthly | Monthly |
| Use Case | Genetic screens, large-scale PPI | High-quality validation |

**Recommendation**: Use both! BioGRID for genetic interactions and large-scale datasets, IntAct for high-quality manually curated PPIs.

## Future Work

- Add network topology metrics (degree, betweenness centrality)
- Integration with STRING for combined curated + predicted interactions
- Tissue-specific filtering using Bgee expression data
- Drug target druggability scoring for hub proteins

## Maintenance

- **Release Schedule**: Monthly updates from BioGRID
- **Data Format**: TAB3 (37 columns, tab-separated)
- **Test Data**: 100 interactions (human-human only)
- **Build Command**: `./biobtree -d biogrid update` (creates both biogrid and biogrid_interaction)
- **License**: BioGRID is free for academic use

## References

- **Citation**: Oughtred R, et al. (2021) The BioGRID database: A comprehensive biomedical resource. Nucleic Acids Res.
- **Website**: https://thebiogrid.org/
- **License**: Free for academic use, commercial license available
