# ncRNA Interaction Dataset

## Overview

Experimentally-supported non-coding-RNA molecular interactions from **NPInter v5**
(ncRNA ↔ protein, ncRNA ↔ RNA). This is the interaction / function layer that gives
bare lncRNAs real partners — complementary to RNAcentral's Rfam/GO (which only covers
structured RNAs) and to the curated disease layer (`ncrna_disease`).

**Source**: NPInter v5 — `interaction_NPInterv5.txt.gz` (literature + high-throughput)
and `interaction_NPInterv5.expr.txt.gz` (expression-derived). The pure-computational
`.comp` cut is deferred (redundant with `mirdb`/`string`).
**License**: NPInter v5 (CC BY 4.0); cite NPInter.
**Data type**: ncRNA molecular interactions, carrying an evidence tier.

## Integration Architecture

### Storage Model
**Primary entries**: one record per interaction, id = NPInter `interID` (e.g. `ncRI-40000001`).

**Attributes stored** (protobuf `NcrnaInteractionAttr`):
- `ncrna_name`, `ncrna_type` (lncRNA/miRNA/circRNA/snoRNA/snRNA)
- `partner_name`, `partner_type` (protein / RNA / DNA), `interaction_class`, `level`
  (RNA-Protein / RNA-RNA / RNA-DNA)
- `experiment`, **`datasource`** (the evidence tier: "Literature mining",
  "High-throughput data", "miRanda with Ago CLIP data", …), `organism`,
  `tissue_or_cell`, `description`
- `source` (provenance: `NPInter`)

**Cross-references**:
- → `uniprot` — protein partners (NPInter's `tarID` is the accession; direct, no lookup)
- → `hgnc` / `ensembl` — the ncRNA gene, and RNA partners, recovered from the name via
  the lookup DB (cached by name, since names repeat across the ~1.5M rows)
- → `pubmed` — the supporting citation(s)

### Query paths
- `gene >> ncrna_interaction >> uniprot` — protein partners of an ncRNA
- `uniprot >> ncrna_interaction >> hgnc` — ncRNAs interacting with a protein
- lite-mode compact: `id|ncrna_name|ncrna_type|partner_name|partner_type|level|datasource`

### Evidence tiering (important)
Even NPInter's base file is mostly high-throughput/prediction; the `datasource`
attribute is the evidence label so consumers can filter (e.g. literature-curated only)
— nothing is silently dropped or exposed unlabeled.

## Notes
- The download is served behind a trailing-slash redirect and, despite the `.gz`
  name, the body has been observed as both gzip and a single-entry ZIP — the parser
  sniffs the magic bytes (`1f8b` gzip / `504b` zip) and handles either.
- `.comp` (pure computational predictions, ~1.5M rows) is deferred; it can be added
  later carrying the same `datasource` evidence tier.

## Maintenance
- **Update**: re-download the NPInter v5 files and re-index.
- **License**: NPInter v5, CC BY 4.0.
- **References**: Zhao et al. *NPInter v5.0* Nucleic Acids Research 2023.
