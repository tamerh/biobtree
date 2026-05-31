# ClinGen Gene-Disease Validity (`clingen_gene_validity`)

Strength-of-evidence classifications for gene-disease causality, curated by
ClinGen Gene Curation Expert Panels (GCEPs).

- **Source:** https://search.clinicalgenome.org/kb/gene-validity/download (CSV)
- **License:** CC0 1.0 (public domain)
- **Dataset id:** 139 — keyed by the ClinGen assertion UUID (`alphanum` bucket),
  extracted from the online-report URL (`...CGGV:assertion_<uuid>-<date>`)
- **Classification values:** Definitive · Strong · Moderate · Limited · Disputed ·
  Refuted · No Known Disease Relationship · Animal Model Only

## Edges
- `hgnc` / `entrez` / `ensembl` — the gene (HGNC id + symbol lookup)
- `mondo` — the curated disease

## Known Limitations
- **One row per gene-disease assertion**, not per gene. A gene with several
  curated diseases yields several assertion entries.
- **Assertion UUID is the key.** When a report URL lacks the `assertion_` marker
  a fallback key `"<gene_symbol>_<mondo_id>"` is synthesized; rows with neither a
  parseable id nor gene+MONDO are skipped (logged).
- Mode of inheritance is stored as the source's short form (e.g. `AD`, `AR`).
- Overlaps conceptually with GenCC (which aggregates ClinGen among other
  submitters); this dataset is ClinGen's primary curation, kept separate.

## Tests
`python3 tests/run_tests.py clingen_gene_validity` (needs a running server on
:9292). Regenerate reference data: `python3 extract_reference_data.py`.
