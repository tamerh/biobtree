# DrugCentral Dataset

## Overview
DrugCentral (University of New Mexico / Oprea Lab) is a curated resource of **approved and marketed drugs**, capturing drug→target mechanism-of-action and regulatory approval status. This dataset stores the drug-centric mechanism layer: for each drug, its human protein targets, which of those are the curated mechanism-of-action (MOA) targets, the pharmacological action type, which regulatory agencies (FDA / EMA / PMDA) have approved it, its **on-label indications**, and its **WHO ATC therapeutic classification**.

**Source**: https://drugcentral.org
**Data Type**: Curated approved/marketed drugs — drug→target (MOA) + regulatory approval + indications + ATC
**License**: CC BY-SA 4.0 (redistribution with attribution + share-alike)
**Dataset ID**: 801

## Integration Architecture

### Storage Model
**Primary Entries**: one per drug, keyed by DrugCentral `struct_id`
**Attributes Stored**: `name`, `inn`, `cas_rn`, `inchikey`, `targets` (UniProt accessions), `moa_targets` (MOA=1 accessions), `action_types`, `target_count`, `fda_approved`, `ema_approved`, `pmda_approved`, `indications` (on-label disease names), `atc_codes` (WHO ATC codes, e.g. `P01BA06`), `atc_classes` (ATC therapeutic class names, e.g. `ANTIMALARIALS`)
**Cross-References**:
- `uniprot` — human protein targets (one edge per accession; the MOA-flagged subset is also recorded in the `moa_targets` attribute)
- `hgnc` / `entrez` / `ensembl` — target gene (via the canonical human-gene resolver)
- `mondo` / `efo` — disease ontology ids for on-label indications, resolved from the indication disease name via the precise whole-term medical-term matcher (so approved indications surface on both drug and disease pages, e.g. donepezil/memantine → Alzheimer's disease). Free-text indications with no exact ontology term keep only the `indications` attribute string.
- text keywords — drug name, INN, **InChIKey**, and **ATC codes** (so `P01BA06` resolves to the drug). ChEMBL also indexes its molecule InChIKeys as keywords, so a shared structure resolves `drugcentral` ↔ `chembl_molecule` (and `pubchem`, which indexes compound synonyms) through the keyword index.

**Bucket Method**: `numeric` (struct_id is an integer)

### Download
The core drug→target + approval layer is built from the **public static downloads**, fetched at build time:
- `drug.target.interaction.tsv.gz` — drug→target interactions (columns: `DRUG_NAME, STRUCT_ID, TARGET_NAME, TARGET_CLASS, ACCESSION, GENE, SWISSPROT, ACT_VALUE, ACT_UNIT, ACT_TYPE, ACT_COMMENT, ACT_SOURCE, RELATION, MOA, MOA_SOURCE, ACT_SOURCE_URL, MOA_SOURCE_URL, ACTION_TYPE, TDL, ORGANISM`). Target edges are restricted to `ORGANISM == "Homo sapiens"` rows with a valid `ACCESSION`.
- `structures.smiles.tsv` — `SMILES, InChI, InChIKey, ID(struct_id), INN, CAS_RN` (merged in for INN / CAS / InChIKey).
- `FDA_Approved.csv`, `EMA_Approved.csv`, `PMDA_Approved.csv` — `struct_id,name` lists used to set the per-agency approval booleans.

**Indications + ATC (prep step).** These live only in the DrugCentral PostgreSQL dump (~1 GB gzipped), not the static TSVs, and the three tables needed sit near the end of the dump (streaming the whole file at every build would waste ~0.9 GB of download per build/test). A one-off prep script extracts them into `raw_data/drugcentral/`, which the parser reads locally (`extrasDir`):
```
python3 src/scripts/drugcentral/drugcentral_prepare.py   # -> raw_data/drugcentral/{atc,struct2atc,omop_indications}.tsv
```
Re-run only when bumping the DrugCentral release. If the files are absent the build still succeeds — just without indications/ATC. Tables used: `omop_relationship` (filtered to `relationship_name = indication`), `struct2atc` (struct_id → ATC code), `atc` (ATC code → level-1..4 class names). `atc_classes` is derived only from **single-substance** (mono) ATC codes — combination-product codes (`chemical_substance` "X and Y") would otherwise tag a drug with misleading classes (e.g. amlodipine → "HMG CoA reductase inhibitors" from a statin combo); the combo **codes** are still kept in `atc_codes`.

## Use Cases
- **Drug → target (mechanism)**: `>>drugcentral>>uniprot` → the protein targets of an approved drug; `moa_targets` attribute isolates the curated mechanism-of-action proteins (e.g. amlodipine → CACNA1C/CACNA1D calcium channels).
- **Target → approved drugs**: `>>uniprot>>drugcentral` (reverse edge, materialized during the UniProt reindex) → which approved drugs hit a protein.
- **Regulatory triage**: filter by `fda_approved == true` (or EMA/PMDA) to restrict to marketed agents.
- **Structure bridge**: shared InChIKey links a DrugCentral drug to its `chembl_molecule` / `pubchem` records.
- **Approved indications**: `indications` attribute lists the on-label diseases; `>>drugcentral>>mondo` (or `>>efo`) gives the ontology-resolved indication edges — catches approvals ChEMBL's trial-phase data misses (e.g. donepezil/memantine → Alzheimer's).
- **Therapeutic class**: `atc_classes` gives the WHO ATC class (e.g. amodiaquine → ANTIMALARIALS), a reliable drug-class signal that disambiguates ambiguous ChEBI chemical roles.

## Known Limitations
- **Indications/ATC need the prep step.** They come from the DrugCentral PostgreSQL dump (2020-09-18 release) via `src/scripts/drugcentral/drugcentral_prepare.py` → `raw_data/drugcentral/`; a build without that step omits them (not fatal). See **Download** above.
- **Indication → ontology is name-based and precise, not exhaustive.** An indication edge is created only when the DrugCentral disease name has an exact whole-term match in MONDO/EFO; free-text concepts with no clean ontology term (e.g. "Moderate to Severe Alzheimer's Type Dementia") keep only the `indications` attribute string. Only `relationship_name = indication` rows are ingested (contraindications / off-label use are not).
- **Entries are still drug→target-anchored.** A drug is emitted only if it has a human-target interaction row; drugs that have indications/ATC but no curated human target are not emitted (count is logged each build). Expanding the entry universe to all DrugCentral drugs is a possible future change.
- Target edges are **human-only** (non-human accessions from the interaction file are dropped).
- The drug→target interaction file is the 2021-09-01 release; the approval CSVs are the current static lists from drugcentral.org.

## Attribution
This product uses data from **DrugCentral** (https://drugcentral.org), licensed under **CC BY-SA 4.0**.

> Avram S, Wilson TB, Curpan R, et al. *DrugCentral 2023 extends human clinical data and integrates veterinary drugs.* Nucleic Acids Research. 2023;51(D1):D1276–D1287. doi:10.1093/nar/gkac1085
