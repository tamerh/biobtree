# ChEMBL Assay Dataset

## Overview

ChEMBL Assay contains experimental protocols and screening assays used to measure bioactivity. Each assay record describes the experimental conditions, target organism, tissue source, assay type (binding, functional, ADME), and quality confidence scores. Essential for interpreting bioactivity data, linking 1.4+ million assays that connect molecules to targets via experimental measurements. Provides critical context for understanding how activity values were obtained.

**Source**: ChEMBL RDF (EMBL-EBI)
**Data Type**: Assay protocols with experimental conditions and confidence scores

## Integration Architecture

### Storage Model

**Primary Entries**:
- Assay IDs (e.g., `CHEMBL615155`) stored as main identifiers

**Searchable Text Links**:
- Assay IDs indexed for lookup

**Attributes Stored** (protobuf ChemblAssayAttr):
- `description`: Detailed assay protocol description
- `assay_type`: Assay classification (B=Binding, F=Functional, A=ADME, T=Toxicity, P=Physicochemical)
- `assay_organism`: Species used in assay
- `assay_tax_id`: NCBI taxonomy ID of assay organism
- `assay_tissue`: Tissue source for assay
- `assay_cell_type`: Cell line or cell type used
- `assay_subcellular_fraction`: Subcellular localization
- `target_chembl_id`: Link to biological target
- `document_chembl_id`: Link to literature source
- `cell_chembl_id`: Link to cell line database
- `confidence_score`: Target assignment confidence (0-9, higher is better)
- `bao_format`: BioAssay Ontology format annotation

**Cross-References**:
- **Targets**: chembl_target (protein/organism targets)
- **Activities**: chembl_activity (measurement results)
- **Documents**: chembl_document (literature references)
- **Cell lines**: chembl_cell_line (experimental cell systems)
- **Molecules**: chembl_molecule (tested compounds)

### Special Features

**Confidence Scoring**:
- Target assignment confidence (0-9 scale)
- Relationship types: D=Direct, H=Homologous, M=Molecular, S=Subcellular, U=Unchecked
- Enables quality filtering of activity data

**BioAssay Ontology (BAO) Integration**:
- Standardized assay format annotations
- Links to BAO terms for assay categorization

**Multi-Dimensional Context**:
- Organism, tissue, cell type, subcellular fraction
- Enables species-specific and tissue-specific queries

**Sparse RDF Handling**:
- Smart tracking for incomplete assay annotations
- Not all assays have full metadata

## Use Cases

**1. Assay Reproducibility Assessment**
```
Query: Target → All assays → Compare protocols and conditions
Use: Identify consistent assay methods across publications
```

**2. Species-Specific Assay Selection**
```
Query: Target + organism filter → Assays for specific species
Use: Find human vs. animal model assays for translational studies
```

**3. Assay Type Filtering**
```
Query: Molecule → Activities → Filter by assay_type (Binding vs. Functional)
Use: Distinguish biochemical binding from cellular functional assays
```

**4. Tissue-Specific Bioactivity**
```
Query: Compound → Activities → Filter by assay_tissue
Use: Assess tissue-selective drug effects
```

**5. High-Confidence Activity Curation**
```
Query: Activities → Filter by assay confidence_score ≥ 8
Use: Focus on well-validated target assignments
```

**6. Cell-Based Assay Selection**
```
Query: Target → Assays with cell_chembl_id → Cell line details
Use: Design cell-based screening campaigns
```

## Test Cases

**Current Tests** (10 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 6 custom tests (description, type, organism, target ref, document ref, tissue)

**Coverage**:
- ✅ Assay ID lookup
- ✅ Assay descriptions
- ✅ Assay type classification (B, F, A, T, P)
- ✅ Organism and taxonomy tracking
- ✅ Tissue source validation
- ✅ Target cross-references
- ✅ Document cross-references
- ✅ Attribute validation

**Recommended Additions**:
- Confidence score distribution tests
- BioAssay Ontology (BAO) annotation tests
- Cell line cross-reference tests
- Assay classification hierarchy tests
- Subcellular fraction validation
- Relationship type tests (D, H, M, S, U)
- Assay parameter tests

## Performance

- **Test Build**: ~4.9s (20 assays)
- **Data Source**: ChEMBL RDF (EMBL-EBI FTP)
- **Update Frequency**: Quarterly ChEMBL releases
- **Total Assays**: 1.4+ million experimental protocols
- **Note**: Sparse RDF handling for incomplete assay metadata

## Known Limitations

**Data Completeness**:
- Not all assays have organism/tissue information
- Cell line data often missing for biochemical assays
- Subcellular fraction rarely specified
- Assay parameters not always standardized

**Confidence Scoring**:
- Confidence scores reflect target assignment quality
- Does not assess overall assay quality or reproducibility
- Manual curation level varies

**BAO Annotation**:
- BioAssay Ontology terms not universally applied
- Coverage improving but incomplete

**Cross-References**:
- Requires chembl_target, chembl_activity, chembl_document for full integration
- Cell line cross-references depend on chembl_cell_line dataset

**Assay Type Granularity**:
- High-level classification (5 categories)
- More detailed assay subtypes available in description field

## Future Work

- Add confidence score validation tests
- Test BioAssay Ontology (BAO) integration
- Add cell line cross-reference tests
- Test assay classification hierarchy
- Add subcellular fraction validation tests
- Test relationship type distribution
- Add assay parameter extraction tests
- Test organism diversity across assay types
- Add tissue distribution analysis tests

## Maintenance

- **Release Schedule**: Quarterly from ChEMBL (currently v34+)
- **Data Format**: RDF/XML with sparse data handling
- **Test Data**: Fixed 20 assay IDs spanning assay types
- **License**: CC BY-SA 3.0 - freely available with attribution
- **Coordination**: Part of ChEMBL suite (assay links activity↔target↔document↔cell_line)

## References

- **Citation**: Zdrazil B et al. (2024) The ChEMBL Database in 2023. Nucleic Acids Res. 52(D1):D1180-D1189.
- **Website**: https://www.ebi.ac.uk/chembl/
- **API**: https://www.ebi.ac.uk/chembl/api/data/docs
- **License**: CC BY-SA 3.0
