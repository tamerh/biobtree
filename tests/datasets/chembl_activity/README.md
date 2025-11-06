# ChEMBL Activity Dataset

## Overview

ChEMBL Activity contains bioactivity measurements linking molecules to targets via assays. Each activity record represents a single experimental measurement (IC50, Ki, EC50, % inhibition, etc.) from a published assay. Essential for drug discovery, linking 21+ million activity measurements across molecules, targets, assays, and literature. Enables structure-activity relationship (SAR) analysis and target profiling.

**Source**: ChEMBL RDF (EMBL-EBI)
**Data Type**: Bioactivity measurements with standardized units and experimental context

## Integration Architecture

### Storage Model

**Primary Entries**:
- Activity IDs (e.g., `CHEMBL_ACT_93229`) stored as main identifiers

**Searchable Text Links**:
- Activity IDs indexed for lookup

**Attributes Stored** (protobuf ChemblActivityAttr):
- `activity_type`: Type of measurement (IC50, Ki, EC50, Inhibition, etc.)
- `value`: Numerical measurement value
- `units`: Measurement units (nM, µM, %, etc.)
- `relation`: Relationship operator (=, <, >, ~)
- `assay_chembl_id`: Link to assay protocol
- `molecule_chembl_id`: Link to tested compound
- `target_chembl_id`: Link to biological target
- `document_chembl_id`: Link to literature source
- `target_organism`: Species of biological target

**Cross-References**:
- **Molecules**: chembl_molecule (tested compounds)
- **Targets**: chembl_target (protein targets)
- **Assays**: chembl_assay (experimental protocols)
- **Documents**: chembl_document (literature references)
- **Cell lines**: chembl_cell_line (if cell-based assay)

### Special Features

**Multi-Entity Integration**:
- Links molecules, targets, assays, and documents in single activity record
- Enables comprehensive bioactivity queries

**Standardized Measurements**:
- ChEMBL normalizes diverse activity types
- Consistent units for cross-study comparison

**Target Organism Tracking**:
- Records species of biological target
- Critical for species selectivity analysis

**Literature Provenance**:
- Every activity linked to source publication
- Enables data quality assessment

## Use Cases

**1. Target-Based Drug Discovery**
```
Query: Target → Activities → Active molecules → SAR analysis
Use: Identify and optimize compounds for specific protein target
```

**2. Compound Profiling**
```
Query: Molecule → All activities → Target selectivity panel
Use: Assess off-target effects and selectivity
```

**3. Assay Cross-Reference**
```
Query: Assay → Activities → Reproducibility across labs
Use: Validate assay results and identify outliers
```

**4. Literature Mining**
```
Query: Document → Activities → Extract all bioactivity data
Use: Automated data extraction from publications
```

**5. SAR Analysis**
```
Query: Molecule series → Activities on target → Structure-activity relationships
Use: Guide medicinal chemistry optimization
```

**6. Species Selectivity**
```
Query: Molecule → Activities filtered by organism → Cross-species activity
Use: Identify species-specific effects for toxicology
```

## Test Cases

**Current Tests** (10 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 6 custom tests (assay ref, molecule ref, target ref, measurement value, document ref, organism)

**Coverage**:
- ✅ Activity ID lookup
- ✅ Cross-references to assay, molecule, target, document
- ✅ Measurement values with units
- ✅ Target organism tracking
- ✅ Attribute validation
- ✅ Batch lookups

**Recommended Additions**:
- Activity type distribution tests (IC50, Ki, EC50, etc.)
- Unit standardization validation
- Relation operator tests (=, <, >, ~)
- Multi-target activity tests
- Cell line cross-reference tests

## Performance

- **Test Build**: ~7.1s (20 activities)
- **Data Source**: ChEMBL RDF (EMBL-EBI FTP)
- **Update Frequency**: Quarterly ChEMBL releases
- **Total Activities**: 21+ million bioactivity measurements
- **Note**: Sparse RDF handling for incomplete measurements

## Known Limitations

**Data Completeness**:
- Not all activities have standardized values
- Some measurements lack units or relation operators
- Cell line information not always present

**ID Format**:
- Activity IDs require CHEMBL_ACT_ prefix (reformatted from RDF)
- Different from other ChEMBL ID formats

**Cross-References**:
- Requires chembl_molecule, chembl_target, chembl_assay, chembl_document for full integration
- Isolated builds may lack some linked entities

**Measurement Standardization**:
- ChEMBL attempts normalization but some heterogeneity remains
- Different assay types may not be directly comparable

## Future Work

- Add activity type classification tests
- Test unit standardization across measurements
- Add relation operator validation tests
- Test cell line cross-references
- Add multi-target activity tests
- Test dose-response curve data
- Add confidence score validation
- Test activity comment fields

## Maintenance

- **Release Schedule**: Quarterly from ChEMBL (currently v34+)
- **Data Format**: RDF/XML with activity ID reformatting
- **Test Data**: Fixed 20 activity IDs spanning measurement types
- **License**: CC BY-SA 3.0 - freely available with attribution
- **Coordination**: Part of ChEMBL suite (activity links molecule↔target↔assay↔document)

## References

- **Citation**: Zdrazil B et al. (2024) The ChEMBL Database in 2023. Nucleic Acids Res. 52(D1):D1180-D1189.
- **Website**: https://www.ebi.ac.uk/chembl/
- **API**: https://www.ebi.ac.uk/chembl/api/data/docs
- **License**: CC BY-SA 3.0
