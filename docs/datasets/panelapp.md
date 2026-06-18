# PanelApp Dataset

## Overview

Clinical gene panels from **Genomics England PanelApp** — expert-curated,
diagnostic-grade gene panels used in the NHS Genomic Medicine Service. Each panel
targets a clinical indication (an R-code / disorder) and lists the genes whose
variants are reportable for that indication, each rated by a traffic-light
confidence (green = diagnostic, amber = borderline, red = not yet).

This adds biobtree's *clinical gene-panel* layer: it answers "which diagnostic
panels include this gene, and at what confidence?" and "which genes are on the
panel for this disorder?", bridging genes (HGNC / Ensembl) to clinical indications
(OMIM / MONDO).

**Source**: `https://panelapp.genomicsengland.co.uk/api/v1/panels/` (REST/JSON, paginated)
**License**: CC BY 4.0 (Genomics England)
**Data Type**: curated (panel, gene) memberships with clinical confidence

## Scope (what is kept)

- Only **green (confidence_level 3)** and **amber (2)** genes are ingested. Red
  (level 1) and level-0 genes are low-evidence noise and dropped.
- Only the GRCh38 Ensembl gene id is extracted from each gene's nested
  `ensembl_genes` dict (GRCh37 is dropped).

## Integration Architecture

### Storage Model (MASTER / CHILD)

- **`panelapp`** (id 805, MASTER): one record per panel, keyed by the integer
  panel id as a string (e.g. `1207`). Attributes: `name`, `disease_group`,
  `disease_sub_group`, `relevant_disorders` (comma-joined R-codes), `version`,
  `number_of_genes`.
- **`panelapp_gene`** (id 806, CHILD): one record per (panel, gene), keyed
  `<panelId>_<geneSymbol>` (e.g. `1207_HMBS`). Attributes: `gene_symbol`,
  `panel_name`, `confidence` (green/amber), `mode_of_inheritance`.

### Edges

| From | To | Meaning |
|------|----|---------|
| `panelapp` | `panelapp_gene` | panel → its gene records (master→child) |
| `panelapp_gene` | `hgnc` | gene → HGNC id (`gene_data.hgnc_id`) |
| `panelapp_gene` | `ensembl` | gene → GRCh38 ENSG |
| `panelapp_gene` | `mim` | gene → OMIM (from `omim_gene` + `OMIM:` phenotype tokens) |
| `panelapp_gene` | `mondo` | gene → MONDO (from `MONDO:` phenotype tokens) |

Panel names (master) and gene symbols (child) are indexed for text search.

## Query Examples

```bash
# Panel -> its genes -> HGNC
curl "http://localhost:9292/ws/map?i=1207&f=panelapp&t=panelapp_gene"

# Gene panel membership: gene -> panelapp_gene
curl "http://localhost:9292/ws/search?q=HMBS"   # then >>panelapp_gene

# Full chain: panel -> gene -> HGNC
# >>panelapp>>panelapp_gene>>hgnc
```

```
# biobtree map chains
HMBS>>panelapp_gene                  # which panels list this gene
1207>>panelapp>>panelapp_gene>>hgnc  # panel genes resolved to HGNC
HMBS>>panelapp_gene>>mondo           # disorder terms for a panel gene
```
