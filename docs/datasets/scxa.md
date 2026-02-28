# SC Expression Atlas (SCXA) Dataset Family

## Overview

EMBL-EBI Single Cell Expression Atlas datasets provide comprehensive single-cell RNA sequencing data. This test directory covers **three related datasets** that share the same data source:

1. **scxa** - Experiment metadata (accessions, technology, cell counts, ontology annotations)
2. **scxa_expression** - Gene expression summaries (aggregated stats per gene)
3. **scxa_gene_experiment** - Gene-experiment details (cluster-level expression, derived)

**Source**: EBI FTP (https://ftp.ebi.ac.uk/pub/databases/microarray/data/atlas/sc_experiments/)
**Data Type**: Single-cell experiment metadata, gene expression statistics, cluster-level data

## Dataset Architecture

### scxa (Experiment-Centric)
- **Primary Entries**: Experiment accessions (e.g., `E-MTAB-6386`, `E-ANND-1`)
- **Attributes**: description, species, technology_types, cell counts, experimental_factors
- **Cross-References**: taxonomy, CL (cell ontology), UBERON (anatomy), Ensembl (marker genes)

### scxa_expression (Gene-Centric Summaries)
- **Primary Entries**: Gene IDs (e.g., `ENSG00000000971`)
- **Attributes**: total_experiments, marker_experiment_count, max_mean_expression, avg_mean_expression
- **Cross-References**: scxa (experiments), scxa_gene_experiment (details), Ensembl
- **Note**: Contains ONLY summary statistics, no detailed expression arrays

### scxa_gene_experiment (Gene-Experiment Details, Derived)
- **Primary Entries**: Composite keys `{gene_id}_{experiment_id}` (e.g., `ENSG00000019582_E-MTAB-6386`)
- **Attributes**: clusters array (mean, median, p_value, is_marker per cluster)
- **Cell Type Labels**: For E-CURD experiments, clusters include `cell_type_name` (e.g., "naive B cell")
- **Cross-References**: scxa_expression (gene summary), scxa (experiment), CL (cell ontology)
- **Note**: Derived dataset - created by scxa_expression parser, not built independently

### Storage Model

```
scxa (experiments)
  └── marker genes → Ensembl
  └── taxonomy, CL, UBERON xrefs

scxa_expression (gene summaries)
  └── summary stats only (no expression arrays)
  └── xrefs → scxa_gene_experiment (details)
  └── xrefs → scxa (experiments)

scxa_gene_experiment (gene-exp details, DERIVED)
  └── cluster-level expression data
  └── xrefs → scxa_expression, scxa
```

## Build Commands

```bash
# Build all SCXA datasets (experiments + gene expression)
./biobtree -d scxa,scxa_expression update

# Test mode
./biobtree -d scxa,scxa_expression test

# Note: scxa_gene_experiment is created automatically by scxa_expression parser
# Do NOT build scxa_gene_experiment independently
```

## Use Cases

**1. Experiment Discovery**
```
Query: Cell type or tissue → Find SCXA experiments
Use: Identifying datasets for cell-type-specific analysis
```

**2. Gene Expression Overview**
```
Query: Gene ID → Get scxa_expression summary stats
Use: Quick overview of gene expression across single-cell atlas
```

**3. Marker Gene Screening**
```
Query: Filter by marker_experiment_count → Find consistent markers
Use: Identifying genes that are markers across multiple experiments
```

**4. Drill-Down to Cluster Details**
```
Query: Gene ID → scxa_expression → scxa_gene_experiment entries
Use: Navigating from summary to cluster-level expression
```

**5. Cross-Species Comparison**
```
Query: Gene ID → Find experiments across species where gene is marker
Use: Evolutionary conservation of cell type markers
```

**6. Technology-Filtered Analysis**
```
Query: Filter experiments by technology type → Compare annotations
Use: Technical validation, protocol optimization
```

**7. Cell-Type-Specific Expression (E-CURD experiments)**
```
Query: Gene ID → scxa_gene_experiment → filter by cell_type_name
Example: CD19 expression in naive B cell = 70.47 TPM, memory B cell = 117.78 TPM
Use: Comparing gene expression across specific cell type subtypes
```

**8. Cell Ontology → Gene Expression**
```
Query: CL:0000788 (naive B cell) → scxa_gene_experiment → marker genes
Use: Finding marker genes for specific cell types via ontology queries
Note: Limited to cell types with CL mappings in EBI source data
```

## Test Cases

**Current Tests**:
- Declarative: Experiment lookup, attribute validation, multi-lookup, invalid ID
- Custom: Cell count, technology types, experiment type, factors, taxonomy xrefs

**Coverage**:
- scxa: Experiment metadata, taxonomy xrefs, ontology links
- scxa_expression: Gene summary lookup, stats validation (planned)
- scxa_gene_experiment: Composite key lookup, cluster data (planned)

**Recommended Additions**:
- scxa_expression: Gene ID lookup, summary stats presence
- scxa_gene_experiment: Composite key lookup, cluster array validation
- Cross-dataset navigation tests

## Performance

- **Test Build**: ~30s (scxa: 50 experiments, scxa_expression: 1000 genes from 10 experiments)
- **Data Source**: EBI FTP (streaming)
- **Update Frequency**: Monthly with Expression Atlas
- **Total Data**: 380+ experiments, millions of gene-experiment pairs

## Known Limitations

**scxa**:
- Marker genes limited to 500 per experiment
- Not all experiments have clean ontology annotations

**scxa_expression**:
- Summary only (no expression arrays in gene entries)
- Species taxid not always available

**scxa_gene_experiment**:
- Cannot be built independently (derived)
- Requires scxa_expression to create/update

**Cell Ontology (CL) Mapping Limitations**:
- CL IDs are sourced from EBI's `inferred_cell_type_-_ontology_labels_ontology` column in experiment metadata
- **Not all cell types have CL mappings in EBI source data** - this is an upstream limitation
- Common unmapped cell types include:
  - Tissue-resident cell types (e.g., "tissue-resident effector memory CD8-positive T cell")
  - Specialized subtypes (e.g., "gut tissue-resident, CD8-positive, memory T cell")
  - Doublet populations (e.g., "T cell/B cell doublet")
- Cell type **names** (`cell_type_name`) are always available for LLM interpretation
- CL xrefs enable `>>cl>>scxa_gene_experiment` queries for mapped cell types
- Unmapped cell types can still be queried by text matching on `cell_type_name`

## Maintenance

- **Release Schedule**: Monthly with Expression Atlas
- **Build**: `./biobtree -d scxa,scxa_expression update` (creates all three)
- **Test Data**: 50 experiments + 1000 genes (test mode)
- **License**: EMBL-EBI terms of use

## References

- **Citation**: Moreno P et al. Expression Atlas update. Nucleic Acids Research 2022.
- **Website**: https://www.ebi.ac.uk/gxa/sc/
- **FTP**: https://ftp.ebi.ac.uk/pub/databases/microarray/data/atlas/sc_experiments/
