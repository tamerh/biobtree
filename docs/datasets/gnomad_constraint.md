# gnomAD Constraint Dataset

## Overview
gnomAD (Genome Aggregation Database, Broad Institute) computes **gene-level constraint metrics** from the observed-vs-expected counts of variation across ~800K exomes/genomes. This dataset stores the **per-gene constraint aggregate** — the germline-population counterpart to DepMap essentiality: "how intolerant is this gene to loss-of-function / missense variation?"

The headline metrics:
- **pLI** — probability a gene is loss-of-function intolerant (0-1; ≥0.9 = highly LoF-intolerant).
- **LOEUF** (`lof.oe_ci.upper`) — upper bound of the observed/expected LoF confidence interval; **lower = more constrained** (the field's preferred continuous metric; <0.35 ≈ strongly constrained).
- **o/e** ratios and **z-scores** for LoF, missense (`mis_z`), and synonymous variation.

**Source**: https://gnomad.broadinstitute.org/ (`gnomad.v4.1.constraint_metrics.tsv`, GRCh38)
**Data Type**: Per-gene loss-of-function / missense / synonymous constraint metrics
**License**: Open Database License (ODbL) v1.0
**Dataset ID**: 800

## Integration Architecture

### Storage Model
**Primary Entries**: per gene, keyed by **Ensembl gene id (ENSG, no version suffix)**
**Attributes Stored**: gene_symbol, gene_id, transcript, pli, loeuf, oe_lof, lof_z, oe_mis, mis_z, oe_syn, syn_z, obs_lof, exp_lof, constraint_flags
**Cross-References**: `ensembl` (direct), plus `hgnc`/`entrez`/`ensembl` reach via the gene symbol
**Bucket Method**: `numeric`

### Download
A single plain TSV (~91 MB) streamed directly from the gnomAD public GCS bucket at build time (`useLocalFile: no`). Set `useLocalFile: yes` and point `path` at a local copy to skip the download.

### Transcript selection
The source file is **per-transcript** and contains both Ensembl and RefSeq rows. The parser:
1. Skips any row whose `gene_id` does not start with `ENSG` (drops RefSeq rows).
2. Keeps **one transcript per gene**, preferring `mane_select == true`, falling back to `canonical == true`.
3. De-duplicates by ENSG.

Missing metric values (empty or `NA`) are parsed defensively and left at zero (rendered as empty in compact output).

## Use Cases
- **LoF intolerance / haploinsufficiency prioritization**: `>>ensembl>>gnomad_constraint` or `>>hgnc>>gnomad_constraint` → pLI/LOEUF for a gene (e.g. BRCA1, dominant disease genes have high pLI / low LOEUF).
- **Variant interpretation context** alongside ClinVar/ClinGen and AlphaMissense.
- **Filter** constrained genes, e.g. `gnomad_constraint.loeuf < 0.35` or `gnomad_constraint.pli > 0.9`.

## Known Limitations
- One representative transcript per gene (MANE-select / canonical); transcript-resolution constraint is not stored.
- pLI is largely superseded by LOEUF for ranking; both are provided.
- gnomAD v4.1 is GRCh38 only.

## Maintenance
- **Update Frequency**: per gnomAD major release (pinned to v4.1 in `path`)
- **Data Format**: TSV
- **License**: ODbL v1.0 — attribution to the Genome Aggregation Database (gnomAD) required.

## References
- **Website**: https://gnomad.broadinstitute.org/
- **Attribution**: Data from the Genome Aggregation Database (gnomAD), Broad Institute. Distributed under the Open Database License (ODbL) v1.0.
- **Citation**: Chen S, Francioli LC, Goodrich JK, et al. (2024) A genomic mutational constraint map using variation in 76,156 human genomes. *Nature* 625:92-100.
