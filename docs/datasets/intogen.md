# intOGen Dataset

## Overview
intOGen is a catalog of somatic cancer **driver genes**, aggregated across tumor cohorts. One entry per driver gene, summarizing the tumor types it drives and its consensus mode of action.

**Source**: https://www.intogen.org (Compendium release, 2024.09.20)
**Data Type**: Somatic cancer driver-gene catalog
**License**: CC0 1.0 (public domain)
**Dataset ID**: 756

## Integration Architecture

### Storage Model
**Primary Entries**: gene-centric, keyed by HUGO gene symbol
**Attributes Stored**: `role` (Act / LoF / ambiguous — majority vote), `cancer_types` (acronyms), `cancer_names`, `transcript`, `methods`, `num_cohorts`, `total_samples`, `total_mutations`

### Key edges
- `intogen` → `hgnc` / `entrez` / `ensembl` (gene hub, resolved from symbol)
- `intogen` → `mondo` (disease, via the shared name mapper on cohort cancer name — intOGen ships no DOID)
- `intogen` → `pubmed` (cohort reference PMIDs)

So `mondo >> intogen >> hgnc` returns somatic driver genes for a cancer, complementing CIViC.

## Use Cases
- **Driver-gene catalog**: `>>hgnc>>intogen` → is this a driver, what role (Act/LoF), in which tumors
- **Cancer → drivers**: `>>mondo>>intogen>>hgnc`
- Druggability via the driver gene → CIViC / ChEMBL

## Known Limitations
- **Filtered drivers only** (`Compendium_Cancer_Genes.tsv`, 633 high-confidence); unfiltered candidates not ingested.
- **Disease mapping is name-based** (no DOID/MONDO ids at source); broad cancer names may map to several MONDO subtypes.
- **ROLE is a majority vote** across per-cohort calls.
- **No drug links** (intOGen has none); requires a `--lookupdb` build.

## Maintenance
- **Update Frequency**: per intOGen release
- **Data Format**: TSV
- **License**: CC0 1.0

## References
- **Website**: https://www.intogen.org
- **Citation**: Martínez-Jiménez F, et al. (2020) A compendium of mutational cancer driver genes. Nat Rev Cancer.
