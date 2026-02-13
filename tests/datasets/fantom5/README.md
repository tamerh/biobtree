# FANTOM5 Dataset Tests

This directory contains tests for the FANTOM5 (Functional Annotation of the Mammalian Genome 5) integration.

## Datasets

| Dataset | ID | Description | Expected Entries |
|---------|-----|-------------|------------------|
| `fantom5_promoter` | 126 | CAGE peak/promoter expression (parent) | ~185,000 |
| `fantom5_enhancer` | 127 | Active enhancer expression (child) | ~65,000 |
| `fantom5_gene` | 128 | Gene-level expression aggregation (child) | ~20,000 |

## Parent/Child Dataset Pattern

`fantom5_promoter` is the parent dataset. When you update it, all three datasets are processed automatically:

```bash
# This updates fantom5_promoter, fantom5_enhancer, AND fantom5_gene
./biobtree -d "fantom5_promoter" update
```

The child datasets (`fantom5_enhancer`, `fantom5_gene`) do not need to be specified separately.

## Test Cases

### Search Tests

```bash
# Search for promoter by gene symbol
curl "http://localhost:9292/ws/?i=TP53&d=fantom5_promoter"

# Search for promoter by peak name
curl "http://localhost:9292/ws/?i=p1@TP53"

# Search for enhancers
curl "http://localhost:9292/ws/?i=BRCA1&d=fantom5_enhancer"

# Search for gene expression
curl "http://localhost:9292/ws/?i=EGFR&d=fantom5_gene"
```

### Mapping Tests

```bash
# Gene to promoters
curl "http://localhost:9292/ws/map/?i=TP53&m=>>ensembl>>fantom5_promoter"

# Gene to enhancers
curl "http://localhost:9292/ws/map/?i=BRCA1&m=>>ensembl>>fantom5_enhancer"

# Gene to gene-level expression
curl "http://localhost:9292/ws/map/?i=EGFR&m=>>ensembl>>fantom5_gene"

# Tissue to promoters (via UBERON)
curl "http://localhost:9292/ws/map/?i=brain&m=>>uberon>>fantom5_promoter"

# Cell type to promoters (via CL)
curl "http://localhost:9292/ws/map/?i=neuron&m=>>cl>>fantom5_promoter"
```

### Filter Tests

```bash
# High expression promoters
curl "http://localhost:9292/ws/map/?i=TP53&m=>>ensembl>>fantom5_promoter[fantom5_promoter.tpm_average>10.0]"

# Tissue-specific promoters
curl "http://localhost:9292/ws/map/?i=brain&m=>>uberon>>fantom5_promoter[fantom5_promoter.expression_breadth==\"tissue_specific\"]"

# Highly expressed genes
curl "http://localhost:9292/ws/map/?i=*&m=>>fantom5_gene[fantom5_gene.tpm_max>1000.0]"
```

### Combined Queries with CollecTRI

```bash
# Find transcription factors regulating genes in brain promoters
curl "http://localhost:9292/ws/map/?i=brain&m=>>uberon>>fantom5_promoter>>ensembl>>collectri"

# Find target genes of a TF with their expression patterns
curl "http://localhost:9292/ws/map/?i=TP53&m=>>collectri>>ensembl>>fantom5_gene"
```

## Attributes

### fantom5_promoter
- `fantom5_peak_id` - Original FANTOM5 peak ID (chr:start..end,strand)
- `fantom5_peak_name` - Peak name (e.g., p1@TP53)
- `chromosome`, `start`, `end`, `strand` - Genomic coordinates
- `gene_symbol`, `gene_id`, `entrez_id`, `uniprot_id`, `hgnc_id` - Gene annotations
- `tpm_average`, `tpm_max` - Expression levels
- `samples_expressed` - Number of samples with expression
- `expression_breadth` - "ubiquitous", "broad", or "tissue_specific"
- `top_tissues` - Array of top expressing tissues with TPM and ontology IDs
- `top_cell_types` - Array of top expressing cell types with TPM and ontology IDs

### fantom5_enhancer
- `fantom5_enhancer_id` - Enhancer ID (chr:start-end)
- `chromosome`, `start`, `end` - Genomic coordinates
- `tpm_average`, `tpm_max` - Expression levels
- `samples_expressed` - Number of samples with expression
- `associated_genes` - Predicted target genes
- `top_tissues` - Array of top expressing tissues

### fantom5_gene
- `gene_id`, `gene_symbol`, `entrez_id` - Gene identifiers
- `tpm_average`, `tpm_max` - Expression levels
- `samples_expressed` - Number of samples with expression
- `expression_breadth` - "ubiquitous", "broad", or "tissue_specific"
- `top_tissues` - Array of top expressing tissues

## Cross-References

| From | To | Purpose |
|------|-----|---------|
| fantom5_promoter | ensembl | Gene association |
| fantom5_promoter | entrez | Entrez Gene ID |
| fantom5_promoter | uniprot | Protein mapping |
| fantom5_promoter | hgnc | HGNC gene symbol |
| fantom5_promoter | uberon | Tissue expression |
| fantom5_promoter | cl | Cell type expression |
| fantom5_promoter | taxonomy | Species (9606) |
| fantom5_enhancer | ensembl | Target genes |
| fantom5_enhancer | uberon | Tissue expression |
| fantom5_enhancer | taxonomy | Species (9606) |
| fantom5_gene | ensembl | Gene mapping |
| fantom5_gene | entrez | Entrez Gene ID |
| fantom5_gene | uberon | Tissue expression |
| fantom5_gene | taxonomy | Species (9606) |

## Data Source

- **URL**: https://fantom.gsc.riken.jp/5/datafiles/reprocessed/hg38_latest/extra/
- **License**: CC BY 4.0
- **Reference**: FANTOM Consortium, Nature 507:462-470 (2014)
