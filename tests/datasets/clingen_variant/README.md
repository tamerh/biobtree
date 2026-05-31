# ClinGen Variant Pathogenicity (`clingen_variant`)

Expert-panel (VCEP) ACMG variant interpretations from the ClinGen Evidence
Repository.

- **Source:** https://erepo.clinicalgenome.org/evrepo/api/summary/classifications/download (TSV)
- **License:** CC0 1.0 (public domain)
- **Dataset id:** 141 — keyed by Allele Registry CA id (`alphanum` bucket)
- **~12.6K variants**, each with an ACMG assertion (Pathogenic / Likely
  Pathogenic / VUS / Likely Benign / Benign) and the applied evidence codes.

## Edges
- `clinvar` — via the *ClinVar Variation Id* column. **This is the key bridge:**
  the existing ClinVar hub already links out to dbSNP (rs), gene and every
  disease ontology, so ClinGen variants inherit that whole graph
  (`clingen_variant >> clinvar >> dbsnp`).
- `hgnc` / `entrez` / `ensembl` — the gene (HGNC symbol lookup)
- `mondo` — the disease
- `pubmed` — supporting articles

## Known Limitations
- **Key = Allele Registry CA id**, which is always present. *ClinVar Variation
  Id* is sometimes blank (e.g. "not yet submitted to ClinVar"); when blank, the
  Evidence Repo UUID is used as a fallback key and no `clinvar` edge is created
  for that row.
- No direct dbSNP edge is written; dbSNP is reached transitively through ClinVar.
- The interpretation `summary` field is long free text; stored verbatim.

## Tests
`python3 tests/run_tests.py clingen_variant` (needs a running server on :9292).
Regenerate reference data: `python3 extract_reference_data.py`.
