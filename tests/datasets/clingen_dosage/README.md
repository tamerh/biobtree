# ClinGen Dosage Sensitivity (`clingen_dosage`)

Per-gene haploinsufficiency (HI) and triplosensitivity (TS) dosage scores curated
by the ClinGen Dosage Sensitivity working group.

- **Source:** https://ftp.clinicalgenome.org/ClinGen_gene_curation_list_GRCh38.tsv
- **License:** CC0 1.0 (public domain)
- **Dataset id:** 140 — keyed by NCBI/Entrez gene id (`numeric` bucket)
- **Canonical example:** BRCA1 (Entrez `672`) — HI score `3`, TS score `0`

## Edges
- `entrez` / `hgnc` / `ensembl` — the gene (direct Entrez id + symbol lookup)
- `mondo` / `mim` — HI and TS disease ids (MONDO/OMIM prefixed in source)
- `pubmed` — supporting HI/TS PMIDs

## Score encoding (HI and TS)
`3` sufficient evidence (dosage-sensitive) · `2` emerging · `1` little · `0` no
evidence · `30` gene associated with autosomal-recessive phenotype · `40` dosage
sensitivity unlikely.

## Known Limitations
- **Curated subset only (~1642 genes).** ClinGen has only evaluated a subset of
  genes. **Absence from this dataset does NOT mean "not dosage-sensitive"** — it
  means "not yet curated". Only curated genes are ingested; query absence
  accordingly.
- **Region curations excluded.** The companion
  `ClinGen_region_curation_list` (513 recurrent-CNV / ISCA regions) is not
  ingested: ISCA region ids form an orphan namespace with no link into the
  biobtree identifier graph (no member-gene lists in the file).
- **GRCh37 build skipped.** Only the GRCh38 file is used.
- Genes whose `Gene ID` column is non-numeric are skipped (logged).

## Tests
`python3 tests/run_tests.py clingen_dosage` (needs a running server on :9292).
Regenerate reference data: `python3 extract_reference_data.py`.
