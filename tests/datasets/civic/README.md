# CIViC Test Suite

Tests for the CIViC (Clinical Interpretation of Variants in Cancer) somatic-cancer integration.

## Datasets

CIViC is a parent dataset (`civic`, genes/features) with three children:

| Dataset | Key | Content |
|---|---|---|
| `civic` | feature_id | Gene/feature summaries (HGNC symbol, Entrez, description) |
| `civic_variant` | variant_id | Variants (HGVS, ClinVar, allele registry, SO types) |
| `civic_evidence` | evidence_id | Clinical evidence (disease, therapy, level, significance) |
| `civic_assertion` | assertion_id | Curated assertions (AMP category, ACMG, NCCN, FDA) |

## Key edges

- `civic` → `hgnc` / `entrez` / `ensembl` (gene hub; Ensembl resolved from the authoritative Entrez ID)
- `civic_variant` → `civic` (parent gene), `clinvar`
- `civic_evidence` / `civic_assertion` → `civic_variant`, `civic` (parent gene), `hgnc` (direct, one-hop disease→gene), `doid`, `chembl_molecule`, `pubmed`, `clinical_trials`
- `doid` ↔ `mondo` (emitted from mondo.obo's `xref: DOID:` lines), so `mondo/efo >> civic_evidence >> hgnc` and disease druggability chains resolve.

## Running

```bash
python3 tests/datasets/civic/extract_reference_data.py   # sample IDs from CIViC nightly TSVs
python3 tests/run_tests.py civic                          # needs a server with --lookupdb build
```

## Known limitations / scope decisions

- **Scope = Accepted & Submitted.** We ingest the `AcceptedAndSubmitted` EID/AID files (≈11.3k evidence, 225 assertions; 912 genes / 389 diseases) rather than accepted-only. `evidence_status` (`accepted`/`submitted`) is a filterable attribute so consumers can restrict to peer-reviewed evidence.
- **Molecular profiles are folded**, not a standalone dataset. Evidence/assertion edges fan out to *every* gene/variant in the profile (including combination profiles). The profile name + ID are kept as evidence attributes.
- **Therapies are name-matched to `chembl_molecule`** (the bulk TSVs carry drug names only; NCIt IDs are API-only). Resolution tries the raw name, upper-cased, and salt-stripped forms (e.g. `Imatinib Mesylate` → `Imatinib`). Therapies that don't resolve are still stored as attributes and indexed for text search, so no information is lost — only the hard ChEMBL map edge is skipped for unmatched names.
- **Phenotypes** are stored as names only (the bulk TSV omits HPO IDs), so there is no `civic_evidence → hpo` map edge.
- **VCF and VariantGroup files are not ingested** — the VCF is fully redundant with VariantSummaries; VariantGroups (30 rows) add little.
- **DOID nodes carry no names of their own** — names/synonyms come from the bridged MONDO term; DOID is a cross-reference hub.
