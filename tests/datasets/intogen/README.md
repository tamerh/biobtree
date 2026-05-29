# intOGen Test Suite

Tests for the intOGen somatic cancer driver-gene catalog integration.

## Dataset

`intogen` — gene-centric (one entry per driver gene, keyed by HUGO symbol), aggregated across the cohorts/tumor types where the gene is a significant driver.

| Field | Meaning |
|---|---|
| `role` | Consensus mode of action (majority vote of per-cohort calls): `Act` / `LoF` / `ambiguous` |
| `cancer_types` | intOGen tumor-type acronyms it drives |
| `cancer_names` | Full tumor-type names |
| `transcript`, `methods`, `num_cohorts`, `total_samples`, `total_mutations` | mutational/statistical features |

## Key edges

- `intogen` → `hgnc` / `entrez` / `ensembl` (gene hub; resolved from the gene symbol)
- `intogen` → `mondo` (disease, via the shared `collectOntologyIDs` name mapper on the cohort cancer name — intOGen ships no DOID)
- `intogen` → `pubmed` (cohort reference PMIDs)

So `mondo >> intogen >> hgnc` returns somatic driver genes for a cancer, complementing CIViC.

## Running

```bash
python3 tests/datasets/intogen/extract_reference_data.py   # sample driver genes from the CC0 release
python3 tests/run_tests.py intogen                          # needs a server with --lookupdb build
```

## Known limitations / scope decisions

- **Filtered drivers only.** Uses `Compendium_Cancer_Genes.tsv` (633 high-confidence drivers); `Unfiltered_drivers.tsv` (candidates) is not ingested.
- **Disease mapping is name-based.** intOGen provides no DOID/MONDO IDs — only tumor-type acronyms + free-text `CANCER_NAME`. Diseases are resolved by the shared normalization mapper (corrections/abbreviations/qualifier-stripping → exact MONDO lookup). Broad cancer names can map to several MONDO subtypes; the disease→gene direction stays sensible (e.g. melanoma → ~75 drivers).
- **ROLE is a per-cohort call**, not a single value; the stored role is the majority vote across cohorts.
- **No drug links.** intOGen has none; druggability comes via the driver gene → CIViC/ChEMBL.
- Requires a `--lookupdb` build (gene-symbol → HGNC/Ensembl and disease-name → MONDO resolution).
