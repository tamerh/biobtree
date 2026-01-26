# BioGRID Dataset

## Overview
BioGRID (Biological General Repository for Interaction Datasets) is a curated database of protein-protein and genetic interactions extracted from the biomedical literature. Contains 2.8M+ interactions from 80,000+ publications covering multiple organisms.

**Source**: https://thebiogrid.org/
**Data Type**: Protein-protein and genetic interactions with experimental evidence

## Integration Architecture

### Storage Model
**Primary Entries**: BioGRID Interactor IDs (numeric, e.g., "112315", "108607")
**Searchable Text Links**: BioGRID ID, gene symbols
**Attributes Stored**: BiogridAttr protobuf with interaction details, counts, organisms, experimental systems

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
| partner_systematic_name | string | Partner's systematic name |
| experimental_system | string | Experimental method (e.g., "Two-hybrid") |
| experimental_system_type | string | "physical" or "genetic" |
| **throughput** | string | "Low Throughput" or "High Throughput" |
| **modification** | string | Genetic modification type (e.g., "Synthetic Lethality") |
| **qualifications** | string | Additional qualifications |
| **tags** | string | Curation tags |
| **phenotype** | string | Observed phenotype description |
| **ontology_term_id** | string | Phenotype ontology term ID |
| organism_a | int32 | Taxonomy ID for interactor A |
| organism_b | int32 | Taxonomy ID for interactor B |
| pubmed_id | string | Supporting PubMed ID |
| author | string | First author and year |
| score | string | Confidence score (if available) |
| source_database | string | Original source database |

### Cross-References
All cross-references are **bidirectional**:
| Target Dataset | Description |
|----------------|-------------|
| biogrid | Links to interaction partners |
| entrez | Links to Entrez Gene IDs |
| uniprot | Links to UniProt accessions (Swiss-Prot + TrEMBL) |
| refseq | Links to RefSeq IDs |
| taxonomy | Links to organism taxonomy IDs |
| pubmed | Links to supporting publications |

### Special Features
- **TAB3 Format**: Uses BioGRID TAB3 format (37 columns) for comprehensive data extraction
- **Throughput Classification**: Distinguishes low-throughput (high confidence) vs high-throughput experiments
- **Genetic Interaction Details**: Captures modification types (Synthetic Lethality, Dosage Rescue, etc.)
- **Bidirectional Interactions**: Same interaction stored under both interactors for easy lookup
- **Phenotype Ontology**: Links to ontology terms for phenotype data

## Use Cases

**1. Find Protein Interaction Partners**
```
Query: Gene symbol → BioGRID → List of interacting proteins
Use: Identify potential drug targets or pathway components
```

**2. Validate Drug Target Interactions**
```
Query: UniProt ID → BioGRID → Filter low-throughput interactions
Use: High-confidence interaction validation for drug discovery
```

**3. Identify Synthetic Lethal Partners**
```
Query: Gene → BioGRID genetic interactions → Filter "Synthetic Lethality"
Use: Find combination therapy opportunities
```

**4. Literature Evidence Lookup**
```
Query: BioGRID interaction → PubMed references
Use: Retrieve supporting publications for interaction claims
```

**5. Cross-Species Interaction Comparison**
```
Query: Gene → BioGRID → Filter by taxonomy
Use: Compare interaction networks across organisms
```

**6. Hub Protein Identification**
```
Query: Gene set → BioGRID → Count interaction partners
Use: Identify highly connected proteins as potential drug targets
```

## Test Cases

**Current Tests** (17 total):
- 11 declarative tests (JSON): ID lookup, cross-reference mappings, attribute validation
- 6 custom tests (Python): Basic lookup, attributes, interactions, mapping validations

**Coverage**:
- Basic BioGRID ID lookup
- Cross-references: Entrez, UniProt, RefSeq, PubMed, Taxonomy
- Partner-to-partner mappings
- Attribute field validation (biogrid_id, interaction_count, unique_partners)
- New TAB3 fields: throughput, modification

**Recommended Additions**:
- Throughput field validation ("Low Throughput" / "High Throughput")
- Genetic interaction modification field testing
- Phenotype/ontology term validation

## Performance

- **Test Build**: ~30s (100 interactions, human-human only in test mode)
- **Data Source**: https://downloads.thebiogrid.org/Download/BioGRID/Latest-Release/BIOGRID-ALL-LATEST.tab3.zip
- **Update Frequency**: Monthly releases
- **Total Entries**: ~2.8 million interactions, ~170 MB compressed, ~1.4 GB uncompressed

## Known Limitations

1. **Large Dataset**: Full BioGRID file is ~170MB compressed, ~1.4GB uncompressed
2. **Human Focus in Test Mode**: Test mode only processes human-human interactions (taxid:9606)
3. **Interaction Grouping**: Same interaction appears under both interactors (bidirectional)
4. **Score Availability**: Not all interactions have confidence scores (shown as "-" or empty)
5. **Phenotype Data**: Only available for some genetic interactions

## Future Work

- Add network topology metrics (degree, betweenness centrality)
- Integration with STRING for combined curated + predicted interactions
- Tissue-specific filtering using Bgee expression data
- Drug target druggability scoring for hub proteins

## Maintenance

- **Release Schedule**: Monthly updates from BioGRID
- **Data Format**: TAB3 (37 columns, tab-separated)
- **Test Data**: 100 interactions (human-human only)
- **License**: BioGRID is free for academic use

## References

- **Citation**: Oughtred R, et al. (2021) The BioGRID database: A comprehensive biomedical resource. Nucleic Acids Res.
- **Website**: https://thebiogrid.org/
- **License**: Free for academic use, commercial license available
