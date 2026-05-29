# Cellosaurus Test Suite

Tests for the Cellosaurus cell-line knowledge resource integration (SIB, CC BY 4.0).

## Dataset

`cellosaurus` — one entry per `CVCL_` accession (167k cell lines, all species). Adds the cell-line entity class as a connected hub.

Attributes: `name`, `synonyms`, `sex`, `age`, `category`, `diseases` (NCIt/ORDO), `species`, `parent`/`same_individual` (CVCL hierarchy), `external_refs` (non-biobtree catalog cross-refs), `comments` (raw CC).

## Edges

- `taxonomy` (OX, every entry; multi-species supported)
- `orphanet` (DI ORDO) + `mondo` (DI disease name via the shared `collectOntologyIDs` mapper)
- `hgnc` + `clinvar` + `dbsnp` (mined from `CC Sequence variation` lines)
- `uniprot` (CC mAb target), `uberon` (CC derived-from-site), `cl` (CC cell type), `chebi` (CC resistance/transformant)
- `cosmic`, `efo`, `mesh`, `chembl_cell_line`, `chembl_target` (DR; ChEMBL-Cells closes the loop on `chembl_cell_line.cellosaurusId`)
- `pubmed` / `doi` / `patent` (RX)
- `cellosaurus` self-edges (HI parent / OI same-individual)

## Running

```bash
python3 tests/datasets/cellosaurus/extract_reference_data.py   # sample CVCL_ ids from the CC BY release
python3 tests/run_tests.py cellosaurus                          # needs a server with --lookupdb build
```

## Known limitations / scope decisions

- **Full ingest, nothing skipped.** All 167k cell lines (all species); every line is captured.
- **Catalog cross-refs are attributes, not edges (for now).** DR resources that aren't biobtree datasets (ATCC, DepMap, GDSC, ECACC, Wikidata, BTO, CLO, …) are stored in `external_refs` rather than dropped. TODO (see `CELLOSAURUS_FEASIBILITY.md`): promote them to derived xref-only datasets using Cellosaurus's `Db_URL` templates.
- **ENCODE refs are biosamples** (`ENCBS…`), not cCREs, so they are NOT edged to `encode_ccre` — kept in `external_refs`.
- **`CC` raw text is stored** in `comments`; structured CC mining covers Sequence variation / Derived from site / Cell type / mAb target / transformant. Deeper CC mining (HLA, STR) is future work.
- Requires a `--lookupdb` build for the disease-name → MONDO resolution.
- Attribution: Cellosaurus is CC BY 4.0 (credit SIB / cellosaurus.org).
