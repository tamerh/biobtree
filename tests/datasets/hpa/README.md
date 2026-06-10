# HPA (Human Protein Atlas) Tests

Source: https://www.proteinatlas.org/ — `proteinatlas.xml.gz` (the full dataset; the TSV/JSON are a subset). License: CC BY 4.0.

Datasets (gene-keyed parent + children, mirroring Bgee):

- **hpa** (773) — gene "card": protein class, evidence, subcellular location (→ GO), RNA/protein specificity calls, top expressed tissues. Xrefs: ensembl, uniprot, hgnc, entrez, go.
- **hpa_expression** (774) — per (gene, tissue/cell) RNA nTPM + IHC staining, keyed `ENSG|entityID`. Xrefs: hpa (gene), uberon (tissues, sorted by expression score), cellosaurus (cell lines).
- **hpa_pathology** (775) — per (gene, cancer) prognostic survival association.
- **hpa_antibody** (776) — HPA validation antibodies (reliability, antigen). Distinct from the therapeutic-antibody dataset (id 40).

## Known Limitations

- **Source is HPA v25.1**, pinned to Ensembl 109; ENSG keys may lag the current Ensembl release.
- **Cell-type → CL mapping is deferred.** The XML provides UBERON IDs inline for tissues and GO IDs for subcellular locations, but cell types (single-cell, tissue cell types) are names only — so `hpa_expression` single-cell entities are name-keyed (`name:<cell type>`) and not yet linked to Cell Ontology. Tissue expression (UBERON) and cell-line (Cellosaurus) are linked.
- **Cancer → MONDO/SNOMED mapping is deferred.** `hpa_pathology` stores the cancer type name; no disease-ontology xref yet (the XML doesn't carry a disease ID for the prognostic cancer types).
- **Third-party-derived data flagged, not excluded.** Cancer prognostics carry `data_source` (often TCGA); GTEx/FANTOM RNA are integrated upstream by HPA. Only HPA's own derived calls are ingested.
- **Reliability** (Enhanced/Supported/Approved/Uncertain) IS populated: gene card carries `reliability_ih` (from tissueExpression verification) + `reliability_if` (from cellExpression verification); `hpa_antibody` carries per-antibody `reliability_ih` + `validations` (RNAConsistency / literatureConformity notes).
- **`blood_concentration` and antibody `rrid`** are not populated — `rrid` is not present in the HPA bulk XML at all (would need an external Antibody Registry mapping); blood concentration is phase-2.
- **`antigen_sequence`** is populated for HPA-prefixed antibodies; older `CAB…` antibodies have no antigen sequence in the source.
- Image URLs are not stored.

## Test entries

- `ENSG00000000003` (TSPAN6) — first stream entry, always present (incl. test mode).
- `HPA004109` — a TSPAN6 validation antibody.
