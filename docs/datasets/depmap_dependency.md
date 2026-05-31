# DepMap Dependency Dataset

## Overview
`depmap_dependency` is the per-cell-line detail behind the [`depmap`](depmap.md) per-gene aggregate: one entry per (cell line, gene) pair where the gene is a CRISPR dependency. It is the edge that connects **cell lines to the genes they depend on**, bridging the cellosaurus cell-line hub to the gene graph.

**Source**: https://depmap.org (CRISPRGeneEffect.csv + Model.csv)
**Data Type**: Per-cell-line × gene dependency edge
**License**: CC BY 4.0
**Dataset ID**: 144 (produced by the `depmap` parser)

## Integration Architecture

### Storage Model
**Primary Entries**: keyed by `<model_id>_<gene_id>` (one per dependency edge, Chronos gene effect < −0.5)
**Attributes Stored**: gene_symbol, gene_id, model_id (ACH-…), cell_line_name, rrid (Cellosaurus CVCL), oncotree_lineage, gene_effect
**Cross-References**: `entrez` (gene), `cellosaurus` (cell line via Model.csv RRID)
**Bucket Method**: `alphanum`

## Use Cases
- **Which cell lines depend on a gene**: `>>hgnc>>...>>depmap_dependency>>cellosaurus`
- **What is a cell line dependent on**: `cellosaurus >> depmap_dependency >> entrez`
- Selective-vulnerability discovery for a tumor lineage

## Known Limitations
- **Only dependency edges are stored** (gene effect < −0.5); non-dependencies are summarized in `depmap` (`pct_dependent`), not materialized here, to keep the dataset to meaningful relationships.
- Cell-line bridge requires the line to have an RRID (Cellosaurus CVCL) in Model.csv.

## Maintenance
- **Update Frequency**: quarterly DepMap releases (with `depmap`)
- **Data Format**: CSV (matrix)
- **License**: CC BY 4.0

## References
- **Website**: https://depmap.org
