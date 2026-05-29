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
