# Feasibility: Human Protein Atlas (HPA) for BioBTree

Researched 2026-06-09. HPA version at time of research: **v25.1** (built on Ensembl 109).

**Bottom line:** 🟢 **GREEN.** HPA is **CC BY 4.0** — redistributable on an AGPL public site (sugi.bio) with attribution, same class as Cellosaurus which we already accepted. The data is keyed by **Ensembl gene ID** with **UniProt / Entrez / gene symbol** built in, so it drops straight onto BioBTree's existing gene/protein hubs. Today BioBTree has **only a legacy antibody-ID xref stub** (`hpa`, id 658, in `xref2.optional`, harvested from Ensembl, dead `tissue_profile.php` URL) — **none of the actual HPA annotations** are present.

**Coverage choice (matters):** the `proteinatlas.tsv.zip` (7.1 MB) and `proteinatlas.json.gz` (12 MB) are the **gene-level summary *subset*** ("the data seen in the search result"). The **`proteinatlas.xml.gz` (696 MB compressed)** is the **full dataset** — per-cell-type IHC staining, per-sample cancer pathology + survival, RNA nTPM per tissue/cell-type/region/blood/cell-line, antibody-level validation, etc. Given BioBTree's cover-as-much-as-possible approach, **the XML is the right source**; the TSV is the cheap-but-lossy fallback. Recommended as a gene-keyed parent dataset (`hpa`) + a few children for the high-cardinality parts, parsed from the XML.

## Current state in BioBTree

| Item | Value |
|---|---|
| Existing conf | `hpa` (id 658) in `conf/xref2.optional.dataset.json`, `name:HPA`, url `tissue_profile.php?antibody_id=£{id}` |
| What it actually is | HPA **antibody IDs** only, created as a side-effect xref in `src/update/ensembl.go` (`e.xref(..., "HPA", "HPA")`) when Ensembl lists HPA cross-references |
| Loaded in standard builds? | No — `xref2.optional` is not wired into `bb.sh`/configs |
| Real HPA data present? | **None** — no expression, localization, pathology, or specificity data |

So the stub can be **upgraded in place** (reuse id 658, fix the URL, add real attrs) or superseded by a new dataset. Reusing 658 keeps any existing Ensembl→HPA antibody edges meaningful.

## License — 🟢 redistributable

- **CC BY 4.0** for all copyrightable parts of the database (proteinatlas.org/about/licence). Commercial use **and** redistribution allowed; only **attribution** required.
- Attribution obligation (must honor on sugi.bio): cite a primary HPA publication, reference **proteinatlas.org**, and when integrating into a site *"cite the source clearly and link to it; content must never be displayed without such citation."* Mechanically the same as the ClinPGx/Cellosaurus attribution we already carry.
- **Caveat — third-party-constrained subsets.** HPA integrates some upstream data with its own terms: **GTEx** and **FANTOM5** RNA, **TCGA** cancer, **Allen** mouse brain. The license says users are responsible for not infringing those third-party rights.
  - Practical line: ingest **HPA-generated summary calls** (subcellular location from IF, tissue/cell-type *specificity* classifications, secretome, blood concentration, antibody reliability, protein class). These are HPA's own derived annotations under CC BY 4.0.
  - The **`Cancer prognostics … (TCGA)`** columns (cols 88–118) are TCGA-derived prognostic *summaries* (published by HPA under CC BY). Low risk as derived statistics, but if we want to be conservative we can mark them optional / omit the `(TCGA)` ones and keep the `(validation)` columns (HPA's own cohort).
  - We are **not** ingesting raw GTEx/single-cell matrices, so the heavy third-party concerns don't arise.

Compatible with the redistributability constraint — see [[dataset-redistributability-constraint]].

## Data products (sizes verified 2026-06-09)

| Source | Static URL | Size | Scope |
|---|---|---|---|
| **`proteinatlas.xml.gz`** | `/download/proteinatlas.xml.gz` | **696 MB** gz | ✅ **FULL dataset** — all granularity below. Ensembl 109. **The max-coverage source.** |
| `proteinatlas.tsv.zip` | `/download/proteinatlas.tsv.zip` | 7.1 MB | Gene-level **summary subset** (20,162 genes × 119 cols). Lossy fallback. |
| `proteinatlas.json.gz` | `/download/proteinatlas.json.gz` | 12 MB | ≈ same **subset** as TSV (JSON form). **Not** the full data. |
| `proteinatlas.xsd` | `/download/proteinatlas.xsd` | 49 KB | XML schema (validates the `<entry>` structure). |
| `cell.svg` | `/download/cell.svg` | — | Subcellular-structure schematic (not data). |
| Per-category TSVs (`normal_tissue`, `subcellular_location`, `pathology`, …) | — | — | ⚠️ **404 as static in v25** — only via the search/query API now. Irrelevant: the XML already contains this granularity. |

So there is no static per-category bulk; the **XML is the single full-coverage download**, and JSON/TSV are the summary subset.

## Full coverage: the `proteinatlas.xml.gz` schema

One `<entry>` per gene. Granularity available (from `proteinatlas.xsd`), none of which is in the TSV:

- **Identity / xref:** `identifier`, `synonym`, `xref` (UniProt + cross-DB), `proteinClasses`, `proteinEvidence`, `proteinstructure`, `interactionstructure`.
- **Antibody-level:** `antibody` → `antibodyData`, `westernBlot`, `proteinArray`, `antigenSequence`, `validation`, `verification`, assay images, RRID.
- **Tissue IHC (per cell type):** `tissueExpression` → `tissue` → `tissueCell` → `cellType` + `level` (staining intensity, `percentageStainedCells`, `positiveStaining`) — i.e. staining **per cell type within each tissue**.
- **Subcellular (IF):** `subcellData` → `location`, `predictedLocation`, `cellcycledependent` (CCD).
- **RNA expression (with nTPM values):** `rnaExpression` → `RNASample`/`quantity` across tissue, `cellTypeExpression`/`singleCellTypeExpression`, brain `region`, `immuneCell`/`lineage` (blood), `cellLine`, plus `rnaSpecificity`/`rnaDistribution` and `rnaExpressionCluster`/`cellTypeExpressionCluster`.
- **Pathology / cancer (per sample):** `cancerExpression` → `pathlogyExpression-type`, `patient`/`patientId`, `age`/`sex`, `snomed`/`snomedParameters`, `survivalAnalysis`, `positiveStaining`.
- **Other:** `cellLine` expression, `mouseBrainStaining`, blood concentration, secretome, lots of `imageUrl`/`imageUrlTif`.

(Images we'd skip or keep only a representative URL. SNOMED in pathology is a bonus xref target.)

## `proteinatlas.tsv` structure (what we'd actually get)

20,162 rows, 119 columns. Identifiers + the high-value annotation fields:

- **IDs:** `Gene` (symbol), `Gene synonym`, `Ensembl` (ENSG, primary key), `Uniprot`, `Chromosome`, `Position`, `Antibody RRID`.
- **Function/class:** `Protein class`, `Biological process`, `Molecular function`, `Disease involvement`, `Evidence` (+ HPA/UniProt/NeXtProt evidence).
- **Subcellular:** `Subcellular location`, `Subcellular main location`, `Subcellular additional location`, `Secretome location`, `Secretome function`.
- **Specificity calls (HPA-derived):** `RNA tissue specificity`/`…distribution`/`…score`, `RNA single cell type specificity`, `RNA cancer specificity`, `RNA blood cell specificity`, `RNA brain regional specificity`, `Protein tissue specificity`, `Protein cell type specificity` (each with distribution + score + the specific tissue/cell + nTPM).
- **Blood:** `Blood concentration - Conc. blood IM/MS [pg/L]`, `Blood expression cluster`.
- **Antibody validation:** `Antibody`, `Reliability (IH)`, `Reliability (IF)`, `Reliability (Mouse Brain)`.
- **Cancer:** `Cancer prognostics - <type> (TCGA)` and `(validation)` (cols 88–118).
- **Other:** `Interactions`, `CCD Protein/Transcript`, expression cluster IDs.

## Identifiers & BioBTree fit — 🟢 excellent

- **Primary key = Ensembl gene ID (ENSG)** → BioBTree already has `ensembl` as a hub; `hpa` entry keyed by ENSG resolves instantly.
- Built-in `Uniprot`, `Entrez` (via symbol), and gene symbol → reverse edges to `uniprot` (the central hub), `hgnc`, `entrez` for free using the existing `addHumanGeneXrefsAll` / Ensembl-resolution helpers.
- Enables the natural traversals sugi-atlas would want: `gene >> hgnc/ensembl >> hpa` (where is this protein expressed? subcellular location? tissue-specific? secreted? blood-detectable? antibody-validated?), and `uniprot >> hpa`.

## Value for BioBTree / sugi-atlas

Fills a real gap — BioBTree has expression-ish data (Bgee, CELLxGENE, SCXA) but **no protein-level localization / specificity / antibody-validation** annotations. HPA adds, per gene: **subcellular location**, **tissue & single-cell-type specificity**, **secretome** classification, **blood detectability/concentration**, **protein class**, **antibody reliability**, and **cancer prognostic** associations — all the canonical "what/where/how-validated" protein facts researchers and AI agents cite. High per-row value, tiny footprint (7.5 MB → 20k entries).

## Integration approach (max coverage from the XML)

Streaming XML parse (BioBTree already does this — `src/update/clinvar_xml.go` is the template; the 696 MB gz is streamed, never fully buffered, like dbSNP/ClinVar). Gene-keyed **parent + children** so high-cardinality data stays queryable rather than bloating one entry:

- **`hpa`** (parent, reuse id 658, ENSG-keyed) — gene summary: protein class, evidence, subcellular main/additional location, secretome, RNA/protein tissue & single-cell specificity calls, blood concentration, antibody RRIDs/reliability. Xref ENSG→`ensembl`, UniProt→`uniprot`, symbol→`hgnc`/`entrez` (bidirectional). This alone ≈ the TSV subset.
- **`hpa_tissue`** (child) — per-tissue / per-cell-type IHC staining level + RNA nTPM. High cardinality (45 tissues × cell types × 20k genes).
- **`hpa_pathology`** (child) — per-cancer expression + `survivalAnalysis`; SNOMED → potential xref to a disease vocab.
- **`hpa_antibody`** (child) — antibody validation (WB/IF/IH/PA reliability, antigen, RRID).
- *(optional)* `hpa_subcell` if we want subcellular as its own queryable node rather than a parent field.

Wiring per dataset is the now-standard checklist (proto message + `Xref` field, `gen.sh` regen, `compact.go`/`enrich.go`/`service.go`/`mapfilter.go`/`mergeg.go` ×2, `bb.sh` DATASETS entry, unit + integration tests) — identical to the `pharmgkb_var_annotation` work. conf: `downloadBaseUrl https://www.proteinatlas.org/download/`, `hpaFile: proteinatlas.xml.gz`, `url https://www.proteinatlas.org/£{id}`, `childDatasets`, `bucketMethod ontology`/`numeric`.

**Phasing option:** ship the `hpa` parent first (covers the TSV-equivalent summary, immediate value), then add the children from the same XML in a follow-up. Lets us land value fast without blocking on the full child schema.

## Effort estimate

**~2–3 days** for full coverage (parent + children, streaming XML parser, nested proto schema). **~1 day** if we ship only the `hpa` parent summary first (could even seed it from the 7 MB TSV and swap to XML later). The XML parser + the rich proto schema are the bulk of the work; `clinvar_xml.go` is a direct precedent.

## Risks / caveats

- **Size/time** — 696 MB gz (multi-GB uncompressed); the parse + index is heavier than a flat TSV (still far smaller than dbSNP). Streaming keeps memory bounded.
- **Third-party data** — TCGA-derived cancer prognostics + GTEx/FANTOM RNA carry upstream terms. Ingest HPA's own derived calls/staining; treat TCGA pathology as optional/flagged. Not ingesting raw GTEx/single-cell matrices.
- **Versioning** — HPA pins to an Ensembl release (109 for v25.1); ENSG keys must tolerate HPA lagging current Ensembl. ~yearly releases → low maintenance.
- **Attribution must be displayed** wherever HPA data surfaces on sugi.bio (CC BY 4.0).

## Recommendation

Proceed, and ingest from the **XML** for the cover-as-much-as-possible goal (not the lossy TSV subset). Suggested path: land the **`hpa` parent** (gene summary: localization, specificity, secretome, blood, protein class, antibody reliability) first for fast value, then add **`hpa_tissue` / `hpa_pathology` / `hpa_antibody`** children from the same XML. CC BY 4.0 clears licensing; Ensembl/UniProt keying makes xrefs cheap; `clinvar_xml.go` is the parser template. Mark TCGA pathology optional.

## Sources

- [HPA Licence](https://www.proteinatlas.org/about/licence)
- [HPA Downloadable data](https://www.proteinatlas.org/about/download)
- [HPA data access help](https://www.proteinatlas.org/about/help/dataaccess)
- `proteinatlas.tsv.zip` inspected directly (2026-06-09): 20,162 genes × 119 columns.
