# Feasibility: Cellosaurus Cell-Line Resource for BioBTree

Investigated 2026-05-29. Scope decision: **ingest the full Cellosaurus (all species)**.

**Bottom line:** Cellosaurus is a **strong, near-ideal fit** for BioBTree. It is a SIB/ExPASy
knowledge resource (Amos Bairoch — same author as UniProt) distributed as a **114 MB flat file
in literal UniProt line format**, so BioBTree's existing UniProt parser is a direct template. It is
**CC BY 4.0** (cleanly redistributable on the AGPL public site, attribution required), and its
cross-reference / disease / species lines connect directly into nodes BioBTree already has —
adding an entire missing entity class (**cell lines**) as a richly-connected hub.

This investigation arose from a look at [osteosarc.com](https://osteosarc.com/) (GitLab co-founder
Sid Sijbrandij's openly-shared **n=1** osteosarcoma multi-omics data). That resource was rejected as
an ingestion source — it is primary, single-patient raw data, the wrong shape for a reference
cross-reference graph, and license-unconfirmed. Its only reference-grade slice was a hand-curated
cell-lines page (HOS, U-2 OS, CAL-72) copied from Cellosaurus/DepMap — which pointed directly at
**Cellosaurus** as the properly-shaped upstream source to ingest instead.

## What it is

| | |
|---|---|
| **Resource** | Cellosaurus — knowledge resource on cell lines (SIB CALIPHO group, ExPASy) |
| **Version** | v55, March 2026 |
| **Size** | **167,186 cell lines** (122,835 human, 31,079 mouse, 3,233 rat; 904 species) |
| **Cross-refs** | 471,228 to **115 resources**; 169,465 literature refs to 29,959 publications |
| **Disease/anatomy** | 2,598 NCIt + 1,403 Orphanet (ORDO) disease terms; UBERON / CL tissue & cell-type |
| **License** | **CC BY 4.0** — copy/redistribute/remix, even commercially; attribution + license link required |
| **Source** | `https://ftp.expasy.org/databases/cellosaurus/cellosaurus.txt` (114 MB flat file; also OBO 111 MB, XML 628 MB, JSON) |
| **ID format** | `CVCL_xxxx` (e.g. `CVCL_0030` = HeLa); 4-char alphanumeric suffix |

## Why it fits

The flat file is line-coded exactly like UniProt, so `src/update/uniprot.go` is a direct parser
template and the mapping onto BioBTree's model is mechanical:

| Cellosaurus line | Content | → BioBTree |
|---|---|---|
| `AC` | `CVCL_xxxx` accession | primary entry ID (`addProp3`) |
| `ID` / `SY` | recommended name + synonyms | text-search links (`addXref` text) |
| `OX` | species, `NCBI_TaxID=9606` | edge → **taxonomy** (present on every entry) |
| `DI` | disease, `NCIt` + `ORDO` codes | edge → **orphanet** (direct); NCIt via crosswalk |
| `DR` | cross-refs to 115 resources | edges → existing datasets (see overlap below) |
| `RX` | `PubMed=` / `DOI=` / `Patent=` | edges → **PubMed / doi / patent** |
| `CC` | structured comments | parseable: UniProtKB targets, `UBERON=` tissue, `CL=` cell type, `ChEBI=` transformants, HLA typing |
| `HI` / `OI` | parent line / same individual | self-edges (CVCL → CVCL hierarchy) |
| `SX` / `AG` / `CA` | sex / age / category | attributes |

## Edge overlap — the real value

Cellosaurus's `DR`/`DI`/`OX`/`RX`/`CC` lines connect into nodes BioBTree already has, making the
new cell-line node a connected hub rather than an island:

- **Genes / proteins:** UniProtKB, HGNC, MGI, RGD, VGNC
- **Chemistry:** ChEBI, DrugBank, PubChem, **ChEMBL-Cells** (→ existing `chembl_cell_line`, closing
  the loop on the `cellosaurusId` attribute it already carries), **ChEMBL-Targets**
- **Disease / anatomy:** Orphanet/ORDO, MeSH, EFO, CL, UBERON (+ MONDO via crosswalk)
- **Variants:** dbSNP, ClinVar, Cosmic *(outbound ID reference only — fine under licensing policy)*
- **Literature:** PubMed, DOI, PMC, Patent
- **Expression / other:** ENCODE, FANTOM5, FlyBase, NCBI Taxonomy

Cell-line catalog-only resources (DepMap, ATCC, ECACC, DSMZ, JCRB, RIKEN, GDSC, PharmacoDB,
Cell_Model_Passport, …) are not BioBTree datasets — store as attributes / xref-only stubs, or skip.

**Osteosarcoma payoff (original motivation):** HOS, U-2 OS, CAL-72, SAOS-2, MG-63 become
first-class `CVCL_` nodes with disease (osteosarcoma → Orphanet/NCIt), species, gene/UniProt links,
and literature — reference-scale and properly licensed, unlike the osteosarc.com hand-copied page.

## Existing footprint in BioBTree

BioBTree already *references* Cellosaurus indirectly: the `chembl_cell_line` dataset
(`conf/source1.dataset.json`) carries a `chembl.cellLine.cellosaurusId` attribute via ChEMBL. Today
those `CVCL_` IDs have no authoritative target. Ingesting Cellosaurus gives them one and makes the
cell-line entity first-class.

## Scope decision

**Ingest all 167,186 cell lines (all species).** Full coverage is cheap at this scale (114 MB
flat file) and avoids arbitrary cutoffs; human-only would drop mouse/rat models that connect to MGI/RGD.

## DR → dataset mapping (RESOLVED 2026-05-29)

Intersected all 144 Cellosaurus resources (`cellosaurus_xrefs.txt`) against every
biobtree conf. **No data is skipped** — matched resources become graph edges;
everything else is stored as a structured attribute on the cell-line entry.

**Real edges — matched existing biobtree datasets (20):**
`UniProtKB`→uniprot, `HGNC`, `MGI`[xref2.opt], `RGD`[xref2.opt], `VGNC`,
`ChEBI`, `DrugBank`, `PubChem`, `dbSNP`/`RS`→dbsnp, `ClinVar`, `Cosmic`[xref2.opt]
(outbound ref), `EFO`, `MeSH`, `UBERON`, `CL`, `ENCODE`→encode_ccre,
`PubMed`, `DOI`, `Patent`.

**Real edges — name-translation (exist under a different key):**
`ChEMBL-Cells`→`chembl_cell_line` (loop-closer for its `cellosaurusId` attr),
`ChEMBL-Targets`→`chembl_target`.

**Non-DR edges:** `DI`→orphanet (ORDO; NCIt via shared `collectOntologyIDs`),
`OX`→taxonomy, `CC`-embedded `UniProtKB`/`UBERON=`/`CL=`, `HI`/`OI`→cellosaurus self-edges.

**Stored as attributes for now (NOT skipped):** the remaining ~120 catalog/other
resources (ATCC, DepMap, GDSC, GEO, PharmacoDB, Wikidata, BTO, CLO, ECACC, DSMZ,
JCRB, RIKEN, …) → a repeated `external_refs` attribute (`"ATCC:HTB-30"`, …).

> **TODO (follow-up):** promote the non-biobtree catalog resources to **derived
> xref-only datasets** (like `doi`/`doid` stubs in xref1). Cellosaurus provides a
> `Db_URL` template for every resource in `cellosaurus_xrefs.txt` (`%s` = the id),
> so these can be auto-generated → each external ref becomes a navigable,
> resolvable xref (e.g. `DepMap:ACH-000001 >> cellosaurus`) instead of a flat
> attribute. Deferred to keep the first pass focused; no data lost in the meantime.

## Open questions (deferred to implementation)

These do not block the decision; they will be resolved while building:

1. **Bucket method.** `AC` is `CVCL_` + 4-char alphanumeric. Existing `alphanum` (37 buckets) on the
   suffix likely suffices; a dedicated `cellosaurus` method (cf. `interpro` / `hmdb`) is the clean
   option for 167k keys. Decide during implementation.
2. **DR → dataset mapping table.** Finalize which of the 115 resources become real edges vs stored
   attributes (proposed cut is the overlap list above).
3. **NCIt disease linking.** Orphanet/ORDO links directly; NCIt needs a crosswalk to MONDO/EFO or
   store as attribute (same open question raised for the intOGen somatic work).
4. **CC-line parsing depth.** How much structured comment mining (HLA typing, UniProtKB targets,
   `UBERON=` / `CL=` / `ChEBI=`) to do in the first pass vs storing `CC` as free text initially.

## Integration effort

~1 day for a solid first pass. The parser is a `uniprot.go`-style flat-file reader (stream by line,
entries terminated by `//`); the real work is the DR→dataset mapping, CC-line parsing, and tests.
Follows the standard dataset-integration checklist in `docs/development/adding-datasets.md`
(config → protobuf → parser → mergeg.go → CEL filter → tests).

### Sources
- Cellosaurus — `https://www.cellosaurus.org` ; FTP `https://ftp.expasy.org/databases/cellosaurus/`
  (flat-file format documented in the header of `cellosaurus.txt`; resource list in `cellosaurus_xrefs.txt`)
- License — CC BY 4.0 (stated in `cellosaurus.txt` header and release notes)
- Bairoch A. *The Cellosaurus, a Cell-Line Knowledge Resource.* J Biomol Tech 2018 (PMC5945021)
- osteosarc.com — Sid Sijbrandij's Osteosarcoma Data (investigated, rejected as ingestion source: n=1 primary data, license-unconfirmed)
