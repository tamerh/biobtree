# CTD (Comparative Toxicogenomics Database) Dataset

## Overview
The Comparative Toxicogenomics Database (CTD) is a curated database that advances understanding of environmental influences on human health. It provides manually curated information about chemical-gene/protein interactions, chemical-disease and gene-disease relationships, along with supporting pathways and annotations.

**Source**: https://ctdbase.org/
**Data Type**: Chemical-gene interactions, chemical-disease associations, gene-disease relationships

## Integration Architecture

### Storage Model
**Primary Entries**: MeSH Chemical ID (e.g., "D000082" for Acetaminophen)
**Searchable Text Links**: Chemical name, synonyms
**Attributes Stored**: Chemical info, gene interactions (with organisms, actions, PubMed refs), disease associations (with inference scores, OMIM refs)
**Cross-References**: MeSH (chemicals/diseases), Entrez Gene, NCBI Taxonomy, PubMed, PubChem, OMIM

### Files Processed
1. **CTD_chemicals.tsv.gz**: Chemical vocabulary with MeSH IDs, synonyms, classifications
2. **CTD_chem_gene_ixns.tsv.gz**: Curated chemical-gene interaction data (~2.5M rows)
3. **CTD_chemicals_diseases.tsv.gz**: Chemical-disease associations (~8.3M rows)

### Special Features
- **Comprehensive chemical data**: Aggregates vocabulary, gene interactions, and disease associations per chemical
- **Curated interactions**: Each gene interaction includes organism context and PubMed evidence
- **Inference scoring**: Disease associations include CTD inference scores for prioritization
- **Multi-species support**: Gene interactions span human, mouse, rat, and other model organisms

## Use Cases

**1. Toxicogenomics Research**
```
Query: Find genes affected by a chemical
D000082 >> ctd >> entrez → Genes interacting with Acetaminophen
Use: Understand molecular mechanisms of toxicity
```

**2. Chemical-Disease Associations**
```
Query: Find diseases linked to chemical exposure
D000082 >> ctd → Disease associations with evidence
Use: Environmental health risk assessment
```

**3. Drug Safety**
```
Query: Identify adverse pathways
drug >> ctd >> mesh (disease) → Linked diseases
Use: Safety profiling during drug development
```

**4. Biomarker Discovery**
```
Query: Find gene biomarkers for chemical exposure
chemical >> ctd → Responsive genes
Use: Develop exposure biomarker panels
```

**5. Cross-Database Integration**
```
Query: Link toxicogenomic data to protein structures
chemical >> ctd >> entrez >> uniprot >> alphafold
Use: Structural toxicology analysis
```

**6. Organism-Specific Queries**
```
Query: Filter interactions by species
ctd.gene_interactions.organism_id == 9606 → Human interactions only
Use: Focus on human health relevance
```

## Test Cases

**Current Tests** (16 total):
- 4 declarative tests (ID lookup, attribute check, multi-lookup, invalid ID)
- 6 attribute tests (chemical name, gene interactions, disease associations, synonyms, MeSH tree, text search)
- 6 cross-reference mapping tests (MeSH, Entrez, MONDO, EFO, Taxonomy, PubChem)

**Attribute Tests**:
- Primary ID lookup and attribute validation
- Gene interaction presence and structure
- Disease association data with inference scores
- Text search by chemical name
- Synonym and MeSH tree classification

**Mapping Tests** (Critical for integration):
- CTD → MeSH (chemical vocabulary mapping)
- CTD → Entrez Gene (gene interaction targets)
- CTD → MONDO (disease ontology via OMIM)
- CTD → EFO (Experimental Factor Ontology via MeSH)
- CTD → Taxonomy (organism context for interactions)
- CTD → PubChem (compound structure linking)

## Performance

- **Test Build**: ~15-30s (500 entries)
- **Data Source**: TSV files from ctdbase.org/reports/
- **Update Frequency**: Monthly
- **Total Entries**: ~180,000 chemicals, ~2.5M gene interactions, ~8.3M disease associations
- **Special notes**: Streaming download of gzipped TSV files

## Known Limitations

- **Gene-disease file excluded**: CTD_genes_diseases.tsv is ~100M rows; currently not processed to avoid excessive size
- **Aggregation limits**: Top 50 gene interactions and 30 disease associations stored per chemical
- **Inferred associations**: Many chemical-disease links are inferred via gene triangulation
- **PubChem format**: Some PubChem CIDs in CTD include "CID:" prefix requiring normalization

## Data Model

### CtdAttr (Chemical Entry)
- `chemical_id`: MeSH ID
- `chemical_name`: Preferred name
- `cas_rn`: CAS Registry Number
- `definition`: Chemical definition
- `synonyms`: Alternative names
- `mesh_tree_numbers`: MeSH classification
- `pubchem_cid`: PubChem Compound ID
- `inchi_key`: InChI Key
- `gene_interactions`: Array of CtdGeneInteraction
- `disease_associations`: Array of CtdDiseaseAssociation
- `inferred_genes`: Genes linked via disease triangulation

### CtdGeneInteraction
- Gene symbol, ID, organism context
- Interaction text and action codes
- PubMed references

### CtdDiseaseAssociation
- Disease name and ID (MeSH/OMIM)
- Direct evidence type
- Inference score
- OMIM and PubMed references

## Future Work

- Process gene-disease associations (with size optimization)
- Add pathway enrichment data
- Support exposure study data
- Integrate phenotype relationships

## Maintenance

- **Release Schedule**: Monthly updates from CTD
- **Data Format**: Tab-separated values (gzipped)
- **Test Data**: 500 entries for fast testing
- **License**: Free for academic use, citation required

## References

- **Citation**: Davis AP, et al. (2025) Comparative Toxicogenomics Database (CTD): 2025 update. Nucleic Acids Res.
- **Website**: https://ctdbase.org/
- **License**: Free for academic use
