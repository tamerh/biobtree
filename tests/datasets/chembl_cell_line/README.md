# ChEMBL Cell Line Dataset

## Overview

ChEMBL Cell Line contains curated information about cell lines used in bioactivity assays. Each cell line record includes organism source, tissue origin, disease state, and cross-references to major cell line databases (Cellosaurus, CLO, EFO, CL-LINCS). Essential for understanding cellular context of screening assays, with 2,300+ cell lines linking to ChEMBL assays and activities. Enables cell-type-specific analysis of drug effects and target expression patterns.

**Source**: ChEMBL RDF (EMBL-EBI)
**Data Type**: Cell line metadata with organism, tissue, and ontology cross-references

## Integration Architecture

### Storage Model

**Primary Entries**:
- Cell line IDs (e.g., `CHEMBL3307241`) stored as main identifiers

**Searchable Text Links**:
- Cell line names indexed for text search
- Cell line descriptions searchable

**Attributes Stored** (protobuf ChemblCellLineAttr):
- `cell_name`: Standard cell line name (e.g., HeLa, HEK293)
- `cell_description`: Descriptive information
- `cell_source_organism`: Species of origin
- `cell_source_tax_id`: NCBI taxonomy ID
- `cell_source_tissue`: Tissue or organ source
- `cellosaurus_id`: Cellosaurus database identifier
- `clo_id`: Cell Line Ontology identifier
- `efo_id`: Experimental Factor Ontology identifier
- `cl_lincs_id`: LINCS cell line identifier

**Cross-References**:
- **Assays**: chembl_assay (cell-based screening)
- **Activities**: chembl_activity (cell line-specific measurements)
- **Ontologies**: Cellosaurus, CLO, EFO, CL-LINCS
- **Taxonomy**: NCBI taxonomy (organism source)

### Special Features

**Multi-Ontology Integration**:
- Cellosaurus: Primary cell line reference database
- CLO: Semantic cell line classification
- EFO: Experimental factors and disease context
- CL-LINCS: NIH LINCS program cell lines

**Organism Tracking**:
- Source organism with NCBI taxonomy ID
- Enables cross-species cell line analysis

**Tissue Context**:
- Tissue/organ of origin
- Disease state information in tissue field

**Sparse Data Handling**:
- Not all cell lines have complete ontology mappings
- CL-LINCS, CLO, and EFO often null for non-standard lines

## Use Cases

**1. Cell-Type-Specific Drug Screening**
```
Query: Target → Assays → Filter by cell_chembl_id → Cell line details
Use: Design cell-based assays with appropriate cell models
```

**2. Tissue-Specific Bioactivity Analysis**
```
Query: Compound → Activities → Cell lines → Filter by tissue
Use: Assess tissue-selective drug effects
```

**3. Cross-Species Cell Line Selection**
```
Query: Cell lines → Filter by organism (human vs. mouse vs. hamster)
Use: Choose appropriate model systems for translational studies
```

**4. Disease Model Selection**
```
Query: Cell lines → Tissue contains "carcinoma" or "adenocarcinoma"
Use: Identify cancer cell line models for oncology research
```

**5. Cell Line Standardization**
```
Query: Cell name → Cellosaurus ID → Standard nomenclature
Use: Resolve cell line naming inconsistencies across studies
```

**6. LINCS Integration**
```
Query: Cell lines with cl_lincs_id → NIH LINCS data
Use: Connect ChEMBL bioactivity to LINCS perturbation signatures
```

## Test Cases

**Current Tests** (6 total):
- All 6 custom tests (no declarative tests)

**Coverage**:
- ✅ Cell line ID lookup
- ✅ Cell line names and descriptions
- ✅ Source organism validation
- ✅ Taxonomy ID tracking
- ✅ Source tissue information
- ✅ Cellosaurus cross-references

**Recommended Additions**:
- CLO (Cell Line Ontology) cross-reference tests
- EFO (Experimental Factor Ontology) cross-reference tests
- CL-LINCS cross-reference tests
- Disease state extraction from tissue field
- Multi-ontology mapping validation
- Cell line name search tests
- Organism distribution tests

## Performance

- **Test Build**: ~4.8s (20 cell lines)
- **Data Source**: ChEMBL RDF (EMBL-EBI FTP)
- **Update Frequency**: Quarterly ChEMBL releases
- **Total Cell Lines**: 2,300+ curated cell lines
- **Note**: Sparse RDF handling for incomplete ontology mappings

## Known Limitations

**Ontology Coverage**:
- Cellosaurus IDs present for most cell lines
- CLO, EFO, CL-LINCS often missing for non-standard lines
- Not all cell lines have complete ontology mappings

**Tissue Field Heterogeneity**:
- Tissue field contains both normal tissue and disease states
- No standardized disease ontology terms
- Inconsistent nomenclature (e.g., "Lyphoma" vs. "Lymphoma")

**Cell Line Naming**:
- Multiple naming conventions across publications
- Cellosaurus provides standardized names but aliases not stored

**Limited Metadata**:
- No cell line characteristics (morphology, growth properties)
- No culture conditions or passage information
- No authentication/contamination status

**Cross-References**:
- Requires chembl_assay and chembl_activity for functional links
- Cellosaurus, CLO, EFO datasets not included in biobtree

## Future Work

- Add CLO cross-reference validation tests
- Test EFO ontology integration
- Add CL-LINCS cross-reference tests
- Test disease state extraction from tissue field
- Add multi-ontology mapping consistency tests
- Test cell line name search functionality
- Add organism diversity analysis tests
- Test cell line-assay relationship validation
- Add tissue categorization tests
- Test integration with external cell line databases

## Maintenance

- **Release Schedule**: Quarterly from ChEMBL (currently v34+)
- **Data Format**: RDF/XML with sparse data handling
- **Test Data**: Fixed 20 cell line IDs spanning organisms/tissues
- **License**: CC BY-SA 3.0 - freely available with attribution
- **Coordination**: Part of ChEMBL suite (cell_line links to assay and activity)

## Cell Line Database Cross-References

| Database | Coverage | Purpose |
|----------|----------|---------|
| **Cellosaurus** | ~2,300 | Primary cell line reference, nomenclature standardization |
| **CLO** | Partial | Cell Line Ontology, semantic classification |
| **EFO** | Partial | Experimental Factor Ontology, disease context |
| **CL-LINCS** | Partial | NIH LINCS program, perturbation data integration |

## References

- **Citation**: Zdrazil B et al. (2024) The ChEMBL Database in 2023. Nucleic Acids Res. 52(D1):D1180-D1189.
- **Website**: https://www.ebi.ac.uk/chembl/
- **API**: https://www.ebi.ac.uk/chembl/api/data/docs
- **Cellosaurus**: https://web.expasy.org/cellosaurus/
- **License**: CC BY-SA 3.0
