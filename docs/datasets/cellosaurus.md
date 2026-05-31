# Cellosaurus Dataset

## Overview
Cellosaurus (SIB) is a knowledge resource on cell lines. It adds the cell-line entity class as a connected hub linking species, diseases, mutated genes, tissue, literature, and the cell-line lineage.

**Source**: https://ftp.expasy.org/databases/cellosaurus/cellosaurus.txt
**Data Type**: Cell-line knowledge resource (UniProt-style flat file)
**License**: CC BY 4.0 (credit SIB / cellosaurus.org)
**Dataset ID**: 757

## Integration Architecture

### Storage Model
**Primary Entries**: one per `CVCL_` accession (~167k cell lines, all species)
**Attributes Stored**: name, synonyms, sex, age, category, diseases (NCIt/ORDO), species, parent / same_individual (CVCL hierarchy), external_refs (non-biobtree catalog cross-refs), comments (raw CC)

### Edges
- `taxonomy` (OX, every entry; multi-species)
- `orphanet` (DI ORDO) + `mondo` (DI disease name via shared name mapper)
- `hgnc` + `clinvar` + `dbsnp` (mined from `CC Sequence variation` lines)
- `uniprot` (CC mAb target), `uberon` (derived-from-site), `cl` (cell type), `chebi` (resistance/transformant)
- `cosmic`, `efo`, `mesh`, `chembl_cell_line`, `chembl_target` (DR)
- `pubmed` / `doi` / `patent` (RX)
- `cellosaurus` self-edges (HI parent / OI same-individual)

## Use Cases
- **Disease → cell-line model**: `>>mondo>>cellosaurus` or `>>orphanet>>cellosaurus`
- **Gene → cell lines**: `>>uniprot>>cellosaurus`, `>>hgnc>>...>>cellosaurus`
- **Cell-line lineage**: parent / same-individual CVCL self-edges

## Known Limitations
- **Full ingest, nothing skipped** (all 167k cell lines, all species).
- **Catalog cross-refs are attributes, not edges** for resources that aren't biobtree datasets (ATCC, DepMap, GDSC, ECACC, Wikidata, BTO, CLO …) — kept in `external_refs`.
- **ENCODE refs are biosamples** (`ENCBS…`), not cCREs — not edged to `encode_ccre`.
- Structured CC mining covers Sequence variation / Derived-from-site / Cell type / mAb target / transformant; deeper CC mining (HLA, STR) is future work.
- Requires a `--lookupdb` build for disease-name → MONDO resolution.

## Maintenance
- **Update Frequency**: per Cellosaurus release
- **Data Format**: UniProt-style flat file
- **License**: CC BY 4.0

## References
- **Website**: https://www.cellosaurus.org
- **Citation**: Bairoch A. (2018) The Cellosaurus, a cell-line knowledge resource. J Biomol Tech.
