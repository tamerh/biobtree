# Feasibility: Somatic-Cancer Datasets for BioBTree

Response to the feature request in `notes.txt` (requested 2026-05-26). Researched 2026-05-29.

**Bottom line:** The limiting factor is **licensing, not data quality**. BioBTree is AGPL-v3 and served on a public-facing site (sugi.bio), which requires *redistributable* data. Three of the four primary candidates (CGC, COSMIC, OncoKB) forbid redistribution / public-platform exposure and are **RED**. Two fully-open sources — **CIViC (CC0)** and **intOGen (CC0)** — together satisfy *all three* requested edge types and are **GREEN**. cBioPortal/TCGA is a **YELLOW** phase-2 option for mutation-frequency data.

## Verdict table

| Dataset | License | Redistributable on AGPL public site? | Gene IDs | Disease vocab | Drug links | Verdict |
|---|---|---|---|---|---|---|
| **CIViC** | CC0 1.0 (public domain) | **Yes, unconditionally** | HGNC symbol + Entrez | DOID → MONDO/EFO | Yes (NCIt → ChEMBL) | 🟢 **GREEN** |
| **intOGen** (2024 release) | CC0 1.0 (public domain) | **Yes** (use 2024.09.20; legacy ≤2020 was CC-BY-NC) | HGNC symbol + MANE/Ensembl | DOID → MONDO/EFO | None | 🟢 **GREEN** |
| cBioPortal / TCGA | ODbL + per-study (GDAC); GDC open tier = NIH GDS | Yellow — open MAFs only, share-alike, per-study filter, no controlled/germline | HGNC/Entrez/Ensembl | OncoTree → MONDO (lossy) | None | 🟡 **YELLOW** |
| Cancer Gene Census (CGC) | COSMIC T&C (reg-gated) | **No** — "no right to transfer/share/distribute … to any third party"; no public-platform exposure | HGNC sym+ID, Ensembl, Entrez (excellent) | free text | None | 🔴 **RED** |
| COSMIC | COSMIC non-commercial T&C; QIAGEN commercial | **No** — same clause; commercial license doesn't grant open re-publication | — | own histology codes | — | 🔴 **RED** |
| OncoKB | Proprietary (MSKCC), per-account token | **No** — token must not be shared/public; public site triggers paid commercial/clinical license; AI-training prohibited | HGNC sym + Entrez | OncoTree | Yes (but licensed content) | 🔴 **RED** |

## Why the RED ones are out

- **CGC & COSMIC** are the *same* licensing regime (CGC is a COSMIC module behind the same registration wall). Verbatim T&C (amended 2025-10-30): *"no right is granted for You to transfer, grant access to, display, share or otherwise distribute COSMIC to any third party in any form or manner whatsoever"* and users *"cannot expose COSMIC data on a free to access platform (including … public facing websites)."* No derived-works grant either, so derived xref edges are still encumbered. Incompatible with AGPL network-copyleft. A paid QIAGEN commercial license is for internal/product use, **not** open re-publication.
- **OncoKB** is proprietary, token-gated per account, "do not share / do not put in public repos," explicitly non-commercial for the free tier, and a public-facing site triggers a paid commercial/clinical license. Even cBioPortal (its canonical consumer) does **not** redistribute OncoKB — each deployer supplies their own token. Bundling its edges into a redistributable DB is not a sanctioned path. *(Verification gap: oncokb.org/terms is a JS SPA that wouldn't render to fetchers; RED rests on OncoKB FAQ + cBioPortal docs + VICC license FAQ. A human should read the terms page to confirm exact wording — but the conclusion is not in doubt.)*

## The two GREEN sources cover the full ask

Requested edges vs. coverage:

| Requested edge | CIViC | intOGen |
|---|---|---|
| `mondo/efo → <somatic> → hgnc` (cancer → driver genes) | ✅ DOID disease → gene | ✅ DOID tumor type → driver gene |
| `hgnc → <somatic>` (gene → role / tumor types) | ✅ oncogenic evidence | ✅ mode of action (Act/LoF = oncogene/TSG) + tumor types per cohort |
| `driver → chembl_target` (druggability) | ✅ variant → therapy (NCIt → ChEMBL) | ❌ none |

**CIViC** is the all-rounder: gene + variant + disease (DOID) + drug (NCIt) + evidence level (A–E) + clinical significance, nightly TSV releases, ~475 genes / ~3,200 variants (small, a few MB), no registration. It directly delivers the druggability edge the request cares about.

**intOGen** complements it with a computational *driver-gene catalog per tumor type* (~633 genes × 266 cohorts), `drivers.tsv` with mode-of-action and tumor type, DOID-coded — the cleanest analogue to CGC's "driver list + role" but openly licensed. Infrequent releases (~every few years) = low maintenance.

Note: neither provides COSMIC-style per-mutation *frequencies*. If frequency data is later needed, that's the cBioPortal/TCGA (YELLOW) phase-2 job — pull open somatic MAFs **directly from GDC** (cleaner NIH terms than the cBioPortal ODbL bundle), filter to open-access non-commercial studies, exclude controlled/germline, and compute gene-frequency-per-cancer-type yourself.

## Integration effort (per dataset)

Both map cleanly onto the existing **GenCC** germline parser as a template — same shape (disease ontology + gene-symbol routing). Estimated ~5–7 hrs each:

1. **Config** `conf/source1.dataset.json` — copy the `gencc` block, new numeric `id`, `attrs`, `compact_fields`, `path` (CIViC nightly TSV / intOGen 2024 download URL).
2. **Parser** `src/update/civic.go` / `src/update/intogen.go` — TSV parse; per row: store attrs, add text search on gene/disease, resolve gene symbol → HGNC/Entrez/Ensembl (existing symbol-resolution helper used by GenCC/GWAS), add disease edge to mondo/efo (via DOID mapping), and for CIViC add a drug edge toward ChEMBL.
3. **Register** the new dataset in the `src/update/update.go` dispatch switch.
4. **bb.sh** — add to the `DATASETS` array.
5. **Tests** — `tests/datasets/<x>/test_<x>.py` + entries in `tests/xintegration/integration_tests.json` (e.g. `breast cancer → civic → hgnc` returns PIK3CA/ESR1; `TP53 → intogen → tumor types`).

Open code questions to confirm before building (the GenCC method names below were reported by exploration and should be checked against `src/update/update.go`):
- Exact gene-symbol→hub helper (reported as `addHumanGeneXrefsAll`) and xref/attr helpers (`addXref`, `addProp3`).
- Whether DOID identifiers already resolve in the graph or need a DOID→MONDO crosswalk step. Germline parsers already emit MONDO/EFO/Orphanet edges, and MONDO subsumes DOID, so a small SSSOM-style crosswalk is the likely missing piece.
- `chembl_target` reach: no existing parser builds `gene → chembl_target` directly; the druggability path is `entry → chembl_target` (curated) or via `uniprot → hgnc`. For CIViC the natural edge is `variant/gene → drug (chembl_molecule)` rather than `→ chembl_target` — confirm which side the disease page needs.

## Recommendation

1. **Ingest CIViC first** (highest value: only open source that delivers the disease→gene→drug druggability edge with evidence levels).
2. **Add intOGen** for the broad computational driver catalog + oncogene/TSG role per tumor type.
3. Together they unblock the germline+somatic disease page with fully AGPL-compatible data. **Drop CGC, COSMIC, OncoKB** — license-blocked.
4. **Defer cBioPortal/TCGA** to a phase 2 only if per-tumor mutation *frequencies* prove necessary; budget extra time for the per-study license filter and GDC aggregation.

### Sources
- CIViC license — docs.civicdb.org/en/latest/about/faq.html (CC0 1.0); releases — civicdb.org/releases
- intOGen — intogen.org/download + intogen.org/faq (CC0 1.0 for 2024.09.20; legacy ≤2020 CC-BY-NC; MANE v1.2; DOID)
- COSMIC/CGC T&C — cosmickb.org/terms ; cancer.sanger.ac.uk/census ; licensing via QIAGEN — cosmickb.org/licensing
- OncoKB — faq.oncokb.org/licensing ; cBioPortal OncoKB data-access docs ; VICC license FAQ (docs.cancervariants.org)
- cBioPortal/TCGA — docs.cbioportal.org/user-guide/faq/ ; gdc.cancer.gov/access-data/data-access-policies
