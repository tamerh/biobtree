# DrugCentral Dataset

## Overview
DrugCentral (University of New Mexico / Oprea Lab) is a curated resource of **approved and marketed drugs**, capturing drug→target mechanism-of-action and regulatory approval status. This dataset stores the drug-centric mechanism layer: for each drug, its human protein targets, which of those are the curated mechanism-of-action (MOA) targets, the pharmacological action type, and which regulatory agencies (FDA / EMA / PMDA) have approved it.

**Source**: https://drugcentral.org
**Data Type**: Curated approved/marketed drugs — drug→target (MOA) + regulatory approval
**License**: CC BY-SA 4.0 (redistribution with attribution + share-alike)
**Dataset ID**: 801

## Integration Architecture

### Storage Model
**Primary Entries**: one per drug, keyed by DrugCentral `struct_id`
**Attributes Stored**: `name`, `inn`, `cas_rn`, `inchikey`, `targets` (UniProt accessions), `moa_targets` (MOA=1 accessions), `action_types`, `target_count`, `fda_approved`, `ema_approved`, `pmda_approved`
**Cross-References**:
- `uniprot` — human protein targets (one edge per accession; the MOA-flagged subset is also recorded in the `moa_targets` attribute)
- `hgnc` / `entrez` / `ensembl` — target gene (via the canonical human-gene resolver)
- text keywords — drug name, INN, and **InChIKey**. ChEMBL also indexes its molecule InChIKeys as keywords, so a shared structure resolves `drugcentral` ↔ `chembl_molecule` (and `pubchem`, which indexes compound synonyms) through the keyword index.

**Bucket Method**: `numeric` (struct_id is an integer)

### Download
Built entirely from the **public static downloads** (no PostgreSQL dump needed), fetched at build time:
- `drug.target.interaction.tsv.gz` — drug→target interactions (columns: `DRUG_NAME, STRUCT_ID, TARGET_NAME, TARGET_CLASS, ACCESSION, GENE, SWISSPROT, ACT_VALUE, ACT_UNIT, ACT_TYPE, ACT_COMMENT, ACT_SOURCE, RELATION, MOA, MOA_SOURCE, ACT_SOURCE_URL, MOA_SOURCE_URL, ACTION_TYPE, TDL, ORGANISM`). Target edges are restricted to `ORGANISM == "Homo sapiens"` rows with a valid `ACCESSION`.
- `structures.smiles.tsv` — `SMILES, InChI, InChIKey, ID(struct_id), INN, CAS_RN` (merged in for INN / CAS / InChIKey).
- `FDA_Approved.csv`, `EMA_Approved.csv`, `PMDA_Approved.csv` — `struct_id,name` lists used to set the per-agency approval booleans.

## Use Cases
- **Drug → target (mechanism)**: `>>drugcentral>>uniprot` → the protein targets of an approved drug; `moa_targets` attribute isolates the curated mechanism-of-action proteins (e.g. amlodipine → CACNA1C/CACNA1D calcium channels).
- **Target → approved drugs**: `>>uniprot>>drugcentral` (reverse edge, materialized during the UniProt reindex) → which approved drugs hit a protein.
- **Regulatory triage**: filter by `fda_approved == true` (or EMA/PMDA) to restrict to marketed agents.
- **Structure bridge**: shared InChIKey links a DrugCentral drug to its `chembl_molecule` / `pubchem` records.

## Known Limitations
- **ATC codes and indication terms are NOT included.** They are not present in the public static download files — they live only in the DrugCentral PostgreSQL dump / live instance. Rather than connect to a live external database at build time or ingest noisy free-text indications, this layer is intentionally limited to the high-quality drug→target + approval data the TSVs provide. ATC/indication can be added later as a separate enrichment pass if a stable static export becomes available.
- Target edges are **human-only** (non-human accessions from the interaction file are dropped).
- The drug→target interaction file is the 2021-09-01 release; the approval CSVs are the current static lists from drugcentral.org.

## Attribution
This product uses data from **DrugCentral** (https://drugcentral.org), licensed under **CC BY-SA 4.0**.

> Avram S, Wilson TB, Curpan R, et al. *DrugCentral 2023 extends human clinical data and integrates veterinary drugs.* Nucleic Acids Research. 2023;51(D1):D1276–D1287. doi:10.1093/nar/gkac1085
