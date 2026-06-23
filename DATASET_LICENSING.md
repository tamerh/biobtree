# Dataset Licensing Policy

BioBTree is **AGPL-v3** and served on a **public-facing site (sugi.bio)**. The decisive gate for ingesting any new dataset is therefore **license / redistributability**, not data quality.

## Rule

Ingested data — and any cross-reference edges derived from it — **must be freely redistributable on a public platform**. AGPL's network-copyleft requires that served data be available to all users; a license that forbids redistribution or public-platform exposure is incompatible and disqualifies the source.

- ✅ **Acceptable:** CC0, CC-BY, ODbL (attribution + share-alike, with per-study care), NIH open-access tiers.
- ⚠️ **Conditional:** non-commercial-only (CC-BY-NC) and per-study-licensed sources — only if a public/commercial-facing deployment is genuinely exempt; usually treat as blocked.
- ❌ **Disqualified:** registration-gated no-redistribution licenses, per-account token gating, "no public-facing exposure" clauses.

## Somatic-cancer feature decision (2026-05-26 request → 2026-05-29 research)

Full analysis: [`docs/somatic_cancer_feasibility.md`](docs/somatic_cancer_feasibility.md).

| Dataset | License | Verdict |
|---|---|---|
| **CIViC** | CC0 1.0 | 🟢 **Ingest** (1st — delivers disease→gene→drug druggability) |
| **intOGen** (2024.09.20) | CC0 1.0 | 🟢 **Ingest** (driver catalog, oncogene/TSG role per tumor) |
| cBioPortal / TCGA | ODbL + per-study; GDC open tier | 🟡 Deferred (phase 2 — only if mutation frequencies needed) |
| Cancer Gene Census (CGC) | COSMIC T&C | 🔴 Rejected — no redistribution / no public-site exposure |
| COSMIC | COSMIC non-commercial / QIAGEN commercial | 🔴 Rejected — same clause |
| OncoKB | Proprietary, per-account token | 🔴 Rejected — token-gated, non-commercial, no redistribution |

CIViC + intOGen (both CC0) together satisfy all three requested edge types: `mondo/efo → driver → hgnc`, `hgnc → role/tumor-types`, and `driver → ChEMBL` (CIViC).

## ClinGen feature decision (2026-05-31)

All ClinGen (Clinical Genome Resource, NIH-funded) curated content is released under **CC0 1.0 (public domain)** — attribution requested as a courtesy only. Three of ClinGen's four curation activities ingested as a family; files refresh nightly.

| Dataset (id) | License | Verdict |
|---|---|---|
| **ClinGen Gene-Disease Validity** (139) | CC0 1.0 | 🟢 **Ingest** — gene→disease evidence tier (Definitive..Refuted) + MOI |
| **ClinGen Dosage Sensitivity** (140) | CC0 1.0 | 🟢 **Ingest** — per-gene haploinsufficiency/triplosensitivity |
| **ClinGen Variant Pathogenicity** (141) | CC0 1.0 | 🟢 **Ingest** — VCEP ACMG assertions, bridged to ClinVar |
| Clinical Actionability | CC0 1.0 | 🟡 Deferred — smallest, age-split REST API |
| Dosage region curations (ISCA) | CC0 1.0 | 🟡 Deferred — orphan ISCA namespace, no graph link |

All keys (HGNC, Entrez, MONDO, OMIM, Orphanet, ClinVar) land on existing biobtree datasets; variant pathogenicity joins the ClinVar hub via its ClinVar Variation Id, inheriting dbSNP/gene/disease links.

## Gene-page / target datasets (2026-05-31)

Batch reviewed alongside ClinGen.

| Dataset | License | Verdict |
|---|---|---|
| **GeneRIF** (142) | NCBI U.S. public domain | 🟢 **Ingest** — cited per-gene functional claims; entrez+pubmed edges |
| **DepMap** (143) + **depmap_dependency** (144) | CC BY 4.0 | 🟢 **Ingest** — CRISPR essentiality (gene aggregate + per-cell-line, bridged to cellosaurus) |
| Entrez gene summary | NCBI public domain | ✅ already integrated (entrez `summary` attr) |
| IMPC mouse knockouts | CC BY 4.0 | 🟡 Later — cross-species, new MP ontology |
| TCGA / cBioPortal mutation freq | ODbL + per-study (GDC open tier) | 🟡 Phase-2 (deliberate own project) |
| **DGIdb** | MIT code; data = 44 mixed-license sources | 🔴 Rejected — wrapper-of-a-wrapper; open partition duplicates ChEMBL/GtoPdb/PharmGKB/CIViC we already carry |
| **SIDER** | CC BY-NC-SA 4.0 (non-commercial) | 🔴 Rejected — non-commercial, disqualified like COSMIC/OncoKB |
| **OFFSIDES / TWOSIDES** | no explicit license | 🔴 Rejected — ambiguous/not redistributable |

Drug-safety gap is real but the clean path is **FDA FAERS (U.S. public domain)**, not SIDER/OFFSIDES, if/when wanted.

## KG export redistribution (2026-06-23)

The published **subgraph KGX export** (Zenodo download) is a stronger redistribution
act than serving on the site, and a single license must cover the whole archive.
Target: **CC BY-NC-SA 4.0** (non-commercial is the safe choice; share-alike is
forced by ChEMBL/PharmGKB/DrugCentral anyway). NC also lets us keep the CC BY-NC
sources (DrugBank, HMDB) rather than dropping them.

Excluded from the export (`omit_sources` in `mappings/subgraph.yaml`):

| Excluded | Reason |
|---|---|
| CTD (`ctd`, `ctd_gene_interaction`, `ctd_disease_association`) | custom terms: commercial prohibited + downstream renegotiation — not freely redistributable |
| `panelapp_gene` | Genomics England: no explicit redistribution license found |
| `mirdb` | no explicit license; predicted miRNA targets |
| spliceai / alphamissense (predictions) | CC BY-NC; already excluded via `WITH_PREDICTIONS=0` |

Kept (compatible with CC BY-NC-SA): DrugBank, HMDB (CC BY-NC); ChEMBL, PharmGKB,
DrugCentral (CC BY-SA); everything else (CC0 / CC BY / public domain).
**OMIM is kept** — our graph carries only bare MIM identifiers as cross-reference
endpoints (no OMIM names/titles/text), so no OMIM content is redistributed.
MSigDB: keep gene-set↔gene membership edges; KEGG-derived sets are noted as
restricted in the release README.
