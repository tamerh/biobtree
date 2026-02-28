# CELLxGENE Dataset

## Overview

CZ CELLxGENE Census is the largest standardized repository of single-cell transcriptomics data, containing 80M+ cells across 1,800+ datasets from 345 collections. Biobtree integrates two consolidated datasets: dataset metadata with cell type/tissue/disease annotations, and comprehensive cell type data aggregated from Census observations.

**Source**: CZ CELLxGENE Census (https://cellxgene.cziscience.com/)
**Data Type**: Single-cell RNA-seq dataset metadata and cell type aggregations

## Integration Architecture

### Storage Model

**Primary Entries (cellxgene)**:
- ID format: UUID (e.g., `d7476ae2-e320-4703-8304-da5c42627e71`)
- Dataset metadata: title, collection, citation, cell count
- Annotations: organism, assay types, cell types, tissues, diseases
- Ontology IDs: NCBITaxon, EFO, CL, UBERON, MONDO/PATO

**Primary Entries (cellxgene_celltype)**:
- ID format: CL ID with underscore (e.g., `CL_0000540`)
- Cell type metadata: name, total cells
- Tissue distribution: found_in_tissues with UBERON IDs
- Disease associations: associated_diseases with MONDO IDs
- Expression data: per-tissue cell counts

**Searchable Text Links**:
- Dataset: title, collection name, organism name
- Cell type: name, tissue names, disease names

**Attributes Stored** (Protobuf):
- `Cellxgene`: dataset_id, title, collection_name, cell_count, organism, assay_types, cell_types, tissues, diseases
- `CellxgeneCelltype`: cell_type_cl, name, found_in_tissues, associated_diseases, total_cells, expression_by_tissue

**Cross-References**:
- cellxgene → taxonomy (organism taxid)
- cellxgene → cl (cell type CL IDs)
- cellxgene → uberon (tissue UBERON IDs)
- cellxgene → efo (assay EFO IDs)
- cellxgene → mondo (disease MONDO IDs)
- cellxgene_celltype → cl (cell type ontology)
- cellxgene_celltype → uberon (tissue ontology)
- cellxgene_celltype → mondo (disease ontology)

### Special Features

- **Automatic Data Extraction**: Python script extracts data from Census API if local files don't exist
- **Dual Data Sources**: Curation API for dataset metadata (100% coverage), Census obs for cell types (68% coverage)
- **Bidirectional Links**: Query datasets by cell type, or find cell types across datasets
- **Ontology Integration**: Full integration with CL, UBERON, MONDO, EFO ontologies
- **Spatial Transcriptomics Support**: Includes Slide-seq, Visium, and other spatial modalities

## Use Cases

**1. Find Datasets for a Cell Type**
```
Query: "Which datasets contain neurons?" → CL:0000540 → cellxgene
Use: Identify single-cell studies relevant to neurological research
```

**2. Explore Cell Type Tissue Distribution**
```
Query: "Where are T cells found?" → CL_0000084 → found_in_tissues
Use: Understand tissue-specific immune cell populations
```

**3. Disease-Associated Datasets**
```
Query: "Datasets studying breast cancer" → MONDO:0007254 → cellxgene
Use: Find scRNA-seq data for oncology research
```

**4. Assay Type Discovery**
```
Query: "Spatial transcriptomics datasets" → EFO:0030062 → cellxgene
Use: Locate Slide-seqV2 and other spatial datasets
```

**5. Cross-Species Comparison**
```
Query: "Mouse brain datasets" → taxonomy:10090 + UBERON:0000955 → cellxgene
Use: Compare cell types across human and mouse brain studies
```

**6. Cell Type Ontology Enrichment**
```
Query: "Cell types in kidney" → UBERON:0002113 → cellxgene_celltype
Use: Discover cell type diversity in specific organs
```

## Test Cases

**Current Tests** (20 total):
- 13 declarative tests (ID lookup, attribute checks, xref validation)
- 7 custom tests (organism mapping, cell type links, reverse mappings)

**Coverage**:
- Dataset ID lookup and attributes
- Cell type ID lookup and attributes
- Taxonomy cross-references (human/mouse)
- CL, UBERON, EFO, MONDO cross-references
- Reverse mapping from ontologies to datasets

**Recommended Additions**:
- Multi-organism dataset tests
- Expression by tissue validation
- Collection-level queries

## Performance

- **Test Build**: ~3-5s (100 entries per dataset)
- **Data Source**: CELLxGENE Census API + Curation API
- **Update Frequency**: Census releases monthly
- **Total Entries**: 1,845 datasets, 1,014 cell types
- **Extraction Time**: ~4 minutes for full extraction

## Known Limitations

- **Cell types limited to Census**: Only scRNA-seq datasets in Census have cell type data; spatial transcriptomics datasets have organism/assay/tissue but not cell types
- **No marker genes**: CellGuide marker data not accessible via public API
- **Definition/synonyms from CL**: Cell type definitions come via CL cross-reference, not stored directly
- **PATO disease IDs**: "normal" disease state uses PATO:0000461, not MONDO

## Future Work

- Add marker gene integration if CellGuide API becomes available
- Support filtering by expression quality scores
- Add dataset-level expression matrices
- Integrate with Bgee for cross-platform expression comparison

## Maintenance

- **Release Schedule**: Census updated monthly
- **Data Format**: JSON lines extracted from Census/Curation APIs
- **Test Data**: 100 datasets, 100 cell types
- **License**: CC-BY 4.0 (CZ CELLxGENE data)
- **Auto-extraction**: Python script runs automatically if data files missing

## References

- **Citation**: CZ CELLxGENE Discover (https://cellxgene.cziscience.com/)
- **Census Documentation**: https://chanzuckerberg.github.io/cellxgene-census/
- **License**: CC-BY 4.0
