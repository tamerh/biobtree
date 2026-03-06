# EFO (Experimental Factor Ontology) Dataset

## Overview

EFO is a systematic classification of experimental variables (factors) used in genomics, transcriptomics, and clinical research. Developed by EMBL-EBI, it integrates terms from multiple ontologies (UBERON anatomy, ChEBI chemicals, Cell Ontology) to provide unified vocabulary for diseases, anatomical entities, cell types, and other experimental conditions. Essential for ArrayExpress, Gene Expression Atlas, GWAS Catalog, and Open Targets platform.

**Source**: EFO OWL file (EMBL-EBI)
**Data Type**: Multi-domain ontology covering experimental factors with hierarchical structure

## Integration Architecture

### Storage Model

**Primary Entries**:
- EFO IDs (e.g., `EFO:0000094`) stored as main identifiers

**Searchable Text Links**:
- Term names indexed for text search (e.g., "B-cell acute lymphoblastic leukemia")
- Enables name-based lookups in addition to ID lookups

**Attributes Stored** (protobuf OntologyAttr):
- Name (primary disease/entity name)
- Type (entity classification)
- Synonyms (alternative names, abbreviations)

**Cross-References**:
- **Hierarchical**: Parent-child relationships via `efoparent` and `efochild` virtual datasets
- **External ontologies**: UBERON, ChEBI, Cell Ontology, Disease Ontology, MONDO
- **Clinical databases**: OMIM, Orphanet, ICD codes
- **Research resources**: ArrayExpress, GWAS Catalog

### Special Features

**Multi-Domain Coverage**:
- Diseases (cancers, genetic disorders, infections)
- Anatomical structures (tissues, organs, cell types)
- Chemicals and drugs (via ChEBI integration)
- Cell lines (HeLa, MCF7, etc.)
- Measurement types and assay technologies

**Text Search Enabled**:
- Unlike most ontologies in biobtree, EFO term names ARE indexed for text search
- Allows lookup by disease name, anatomical term, or cell type name

**Cross-Ontology Integration**:
- Imports and aligns terms from multiple specialized ontologies
- Provides unified access layer across disease, anatomy, and chemical spaces

## Use Cases

**1. Gene Expression Studies**
```
Query: Disease name → Find EFO term → Link to gene expression experiments
Use: ArrayExpress/Expression Atlas data annotation and retrieval
```

**2. GWAS Data Integration**
```
Query: EFO disease term → Find associated genetic variants
Use: GWAS Catalog disease-variant associations
```

**3. Disease Research**
```
Query: "lymphoblastic leukemia" → EFO:0000094 → Related diseases, cell types
Use: Explore disease hierarchies and related conditions
```

**4. Experimental Design**
```
Query: EFO terms → Standardized experimental factor annotation
Use: Protocol standardization across genomics studies
```

**5. Drug Target Discovery**
```
Query: EFO disease → Open Targets platform → Therapeutic targets
Use: Identify drug targets for specific diseases
```

**6. Cross-Database Integration**
```
Query: EFO term → Map to MONDO, Disease Ontology, ICD codes
Use: Link clinical and research databases using common vocabulary
```

## Test Cases

**Current Tests** (6 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 2 custom tests (descriptive name, synonyms)

**Coverage**:
- ✅ EFO ID lookup
- ✅ Attribute validation (name, type, synonyms)
- ✅ Batch lookups
- ✅ Invalid ID handling
- ✅ Disease/entity names
- ✅ Synonym presence
- ✅ Text search capability (names indexed)

**Recommended Additions**:
- Parent-child hierarchy navigation tests
- Text search validation (disease name → EFO ID)
- Cross-ontology mapping tests (EFO → MONDO, Disease Ontology)
- Multi-domain coverage tests (disease, anatomy, cell type)
- Integration tests with expression databases

## Performance

- **Test Build**: ~4 minutes (100 EFO entries)
- **Data Source**: OWL file from EMBL-EBI
- **Update Frequency**: Regular updates (monthly releases)
- **Total Terms**: ~30,000+ terms across all experimental factor types
- **Note**: Build time reflects large OWL file size and complex ontology structure

## Known Limitations

**Hierarchy Navigation**:
- Parent-child relationships stored via virtual datasets (`efoparent`, `efochild`)
- Requires additional queries to navigate hierarchy

**Cross-References**:
- External database mappings depend on those databases being configured
- Some xrefs may not be available in isolated test builds

**Import Complexity**:
- EFO imports terms from multiple ontologies
- Term provenance (which ontology it came from) not explicitly stored

## Future Work

- Add text search validation tests (disease name → EFO ID lookups)
- Test parent-child hierarchy navigation
- Add cross-ontology mapping tests (EFO → MONDO, UBERON, etc.)
- Test multi-domain coverage (diseases, anatomy, cell types, chemicals)
- Add integration tests with ArrayExpress/Expression Atlas annotations
- Test GWAS Catalog disease term mappings
- Add Open Targets platform integration tests

## Maintenance

- **Release Schedule**: Monthly releases from EMBL-EBI
- **Data Format**: OWL/RDF (complex import structure)
- **Test Data**: Fixed 100 EFO IDs spanning diseases and other experimental factors
- **License**: Apache 2.0 - freely available for all uses
- **Namespace**: All terms use EFO: prefix

## References

- **Citation**: Malone J et al. Modeling sample variables with an Experimental Factor Ontology. Bioinformatics (2010).
- **Website**: https://www.ebi.ac.uk/efo/
- **OLS Browser**: https://www.ebi.ac.uk/ols/ontologies/efo
- **GitHub**: https://github.com/EBISPOT/efo
- **Documentation**: Part of the SPOT team resources at EMBL-EBI
