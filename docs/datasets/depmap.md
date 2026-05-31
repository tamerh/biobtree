# DepMap Dataset

## Overview
DepMap (Cancer Dependency Map, Broad Institute) provides genome-wide CRISPR knockout fitness screens across cancer cell lines. This dataset stores the **per-gene essentiality aggregate** — the target-tractability signal for drug discovery ("is this gene a cancer dependency?").

**Source**: https://depmap.org (CRISPRGeneEffect.csv)
**Data Type**: Per-gene CRISPR Chronos gene-effect aggregate
**License**: CC BY 4.0
**Dataset ID**: 143 (per-cell-line detail in `depmap_dependency`, id 144)

## Integration Architecture

### Storage Model
**Primary Entries**: per gene, keyed by Entrez gene id
**Attributes Stored**: gene_symbol, gene_id, mean_gene_effect, num_lines, num_dependent, pct_dependent, common_essential, strongly_selective
**Cross-References**: `entrez`/`hgnc`/`ensembl` (gene)
**Bucket Method**: `numeric`

### Download
DepMap moved off figshare; files are resolved at build time via the download API (`https://depmap.org/portal/download/api/downloads`) for the pinned release (`depmapRelease`, e.g. *DepMap Public 26Q1*), cached under the output dir, with a `depmapLocalDir` override — same pattern as ChEMBL. Bump `depmapRelease` to upgrade.

### Dependency semantics
A gene is counted as a dependency in a cell line when its Chronos **gene effect < −0.5** (DepMap's standard threshold). `pct_dependent` is the fraction of screened lines meeting that bar; `common_essential` ≈ pan-essential (≥90% of lines), `strongly_selective` ≈ a selective target (dependent in ≤5%).

## Use Cases
- **Target tractability**: `>>hgnc>>depmap` → is the gene a cancer dependency, how broadly (KRAS = strongly selective; pan-essential genes = poor targets)
- **Drug-target prioritization** alongside CIViC/intOGen driver evidence

## Known Limitations
- Aggregate uses **all** screened lines; the per-line detail (which lines, scores) lives in `depmap_dependency`.
- Pinned to a specific release (`depmapRelease`); not blind-latest, since column formats can shift.

## Maintenance
- **Update Frequency**: quarterly DepMap releases
- **Data Format**: CSV (matrix)
- **License**: CC BY 4.0

## References
- **Website**: https://depmap.org
- **Citation**: Tsherniak A, et al. (2017) Defining a Cancer Dependency Map. Cell.
