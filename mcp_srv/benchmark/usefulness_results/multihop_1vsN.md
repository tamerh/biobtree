# biobtree multi-hop: one call vs N tools

| # | Question | biobtree chain (1 call) | targets | latency | manual: #DBs/tools |
|---|----------|-------------------------|--------:|--------:|-------------------:|
| 1 | What proteins does the drug imatinib target (mechanism-level)? | `>>chembl_molecule>>chembl_target>>uniprot` | 75 | 4 ms | 3 |
| 2 | Curated mechanism-of-action + approval for imatinib? | `>>chembl_molecule>>drugcentral` | 1 | 0 ms | 2 |
| 3 | Which Reactome pathways involve TP53? | `>>hgnc>>uniprot>>reactome` | 46 | 1 ms | 2 |
| 4 | GO terms (function) for BRCA1? | `>>hgnc>>uniprot>>go` | 71 | 1 ms | 2 |
| 5 | Drugs in clinical trials for Parkinson disease? | `>>mondo>>clinical_trials>>chembl_molecule` | 75 | 3 ms | 3 |
| 6 | Cancer-driver evidence for KRAS? | `>>hgnc>>intogen` | 1 | 0 ms | 2 |
| 7 | ClinGen gene-disease validity tier for PTEN? | `>>hgnc>>clingen_gene_validity` | 1 | 0 ms | 2 |
| 8 | Cell lines associated with the EGFR protein? | `>>hgnc>>uniprot>>cellosaurus` | 36 | 1 ms | 2 |
| 9 | Tissues expressing SCN9A (Bgee)? | `>>hgnc>>ensembl>>bgee` | 1 | 0 ms | 2 |
| 10 | MaveDB functional-assay scores for BRCA1 variants? | `>>hgnc>>uniprot>>mavedb` | 75 | 1 ms | 2 |
| 11 | Pharmacology targets + affinity for the ligand quinine (GtoPdb)? | `>>gtopdb_ligand>>gtopdb_interaction>>gtopdb>>uniprot` | 11 | 1 ms | 4 |
| 12 | Diseases genetically associated with the HPO term 'Seizure' via genes? | `>>hpo>>gencc>>hgnc` | 0 | 0 ms | 3 |

**Summary:** 11/12 tasks answered in a single call · median latency 1 ms · each replaces 2–4 separate resource lookups (mean 2.4).

## Manual-reproduction appendix (what the same answer needs elsewhere)

1. **What proteins does the drug imatinib target (mechanism-level)?** — biobtree: 1 call. Manual: name→ChEMBL ID (ChEMBL search) → ChEMBL→mechanism targets (ChEMBL API) → target→UniProt (UniProt ID mapping) (3 steps).
2. **Curated mechanism-of-action + approval for imatinib?** — biobtree: 1 call. Manual: name→ChEMBL/DrugCentral ID → DrugCentral MOA/approval lookup (2 steps).
3. **Which Reactome pathways involve TP53?** — biobtree: 1 call. Manual: symbol→HGNC/UniProt (HGNC or UniProt) → UniProt→Reactome (Reactome API) (2 steps).
4. **GO terms (function) for BRCA1?** — biobtree: 1 call. Manual: symbol→UniProt (UniProt) → UniProt→GO (QuickGO/UniProt) (2 steps).
5. **Drugs in clinical trials for Parkinson disease?** — biobtree: 1 call. Manual: disease→MONDO (OLS/Mondo) → MONDO→trials (ClinicalTrials.gov) → trial drug→ChEMBL (ChEMBL) (3 steps).
6. **Cancer-driver evidence for KRAS?** — biobtree: 1 call. Manual: symbol→gene id → intOGen driver lookup (intOGen portal) (2 steps).
7. **ClinGen gene-disease validity tier for PTEN?** — biobtree: 1 call. Manual: symbol→HGNC → ClinGen gene-validity lookup (ClinGen portal) (2 steps).
8. **Cell lines associated with the EGFR protein?** — biobtree: 1 call. Manual: symbol→UniProt → UniProt→Cellosaurus (Cellosaurus) (2 steps).
9. **Tissues expressing SCN9A (Bgee)?** — biobtree: 1 call. Manual: symbol→Ensembl (BioMart) → Ensembl→Bgee expression (Bgee API) (2 steps).
10. **MaveDB functional-assay scores for BRCA1 variants?** — biobtree: 1 call. Manual: symbol→UniProt → UniProt→MaveDB (MaveDB search + score CSV parse) (2 steps).
11. **Pharmacology targets + affinity for the ligand quinine (GtoPdb)?** — biobtree: 1 call. Manual: name→GtoPdb ligand → ligand→interactions (GtoPdb) → interaction→target (GtoPdb) → target→UniProt (UniProt) (4 steps).
12. **Diseases genetically associated with the HPO term 'Seizure' via genes?** — biobtree: 1 call. Manual: phenotype→HPO id (HPO) → HPO→disease/gene (GenCC) → →HGNC (HGNC) (3 steps).
