# KG Export — Dataset Coverage Audit

Audit of `conf/source1.dataset.json` (125 datasets) and `conf/source2.dataset.json`
(59) against what the KG exporter actually emits (nodes via `categories.yaml`,
edges via `predicates.yaml` direct/reified, runtime builders `go`/`refseq`/`dbsnp`,
and `ontology.py` hierarchy). Datasets in other conf files are derived /
identifier-only and out of scope here.

| Source | Covered | Not covered |
|---|---|---|
| source1 | 62 / 125 | 63 |
| source2 | 45 / 59 | 14 |

"Not covered" is mostly **intentional** (identifier/derived, predictive, sub-entity
of a covered dataset, or qualifier-role). The genuinely actionable gap is the
**Tier-A** list below.

## source2 — effectively complete

All meaningful ontologies are covered (nodes + `subclass_of` + cross-ontology
`close_match`). The 14 "not covered" are:

- **Qualifier / evidence ontologies** (deferred to qualified-edges): `eco`, `pato`,
  `xco` (+ their `*parent`/`*child`). `bao` itself **is** used (assay-type qualifier
  on bioactivity); only `baochild`/`baoparent` are unused.
- **Isolated**: `obi` (+ hierarchy) — no cross-dataset links in BioBTree (reported).

No action needed for source2 beyond the already-tracked qualifier work.

## source1 — not covered, by reason

### Intentional — identifier / derived / structural (no KG action)
`my_data`, `neighborentrez`, `relatedentrez`, `orthologentrez`, `exon`, `cds`,
`ufeature`, `uniparc`, `uniref50`, `uniref90`, `uniref100`, `literature_mappings`,
`bgee_evidence`, `gwas_study`, `string` (container; `string_interaction` covered),
`biogrid` (container; `biogrid_interaction` covered), `antibody`, `hpa_antibody`.
*(`taxparent`/`taxchild` show as uncovered but are in fact consumed by `ontology.py`
via the parent-dataset override.)*

### Intentional — predictive (deferred by decision)
`alphafold`, `alphamissense`, `alphamissense_transcript`, `spliceai`. A future
clearly-labeled `prediction` layer, separate from the asserted graph.

### Intentional — sub-entity, functionally covered via parent/sibling
`chembl_assay`, `chembl_target`, `chembl_cell_line`, `chembl_document`,
`pubchem_assay`, `hpa_expression`, `hpa_pathology`, `scxa_expression`,
`scxa_gene_experiment`, `pharmgkb_gene`, `pharmgkb_guideline`, `depmap`
(covered via `depmap_dependency`), `fantom5_enhancer` (deferred — needs a
`RegulatoryRegion` node type).

### Tier-A — genuine gaps (meaningful relationships NOT in the KG)

| Dataset(s) | Relationship | Proposed biolink | Value |
|---|---|---|---|
| `alliance_disease` | model-organism gene → disease (cross-species) | `gene_associated_with_condition` | High (cross-species) |
| `ctd_disease_association`, `ctd_gene_interaction` (+`ctd`) | chemical ↔ disease, chemical ↔ gene | `affects` / `associated_with` | High (tox/pharmacology) |
| `civic`, `civic_variant`, `civic_evidence`, `civic_assertion` | clinical variant interpretation (cancer) | variant→disease/drug | High (now in prod) |
| `clinical_trials` | drug → condition (trials) | `in_clinical_trials_for` | Med-High |
| `mirdb` | miRNA → target gene | `affects` / `regulates` | Med (ncRNA) |
| `generif` | gene → literature functional claim | `mentioned_in` / publications | Med |
| `jaspar` | TF → gene (binding motif) | `regulates` (DNA-binding) | Med (regulatory) |
| `encode_ccre` | candidate regulatory element → gene | needs `RegulatoryRegion` | Med (regulatory) |
| `brenda`, `brenda_kinetics`, `brenda_inhibitor` | enzyme function / kinetics / inhibitors | `enables` / `affects` | Med (enzymology) |
| `cellxgene`, `cellxgene_celltype` | gene → cell type / tissue (single-cell) | `expressed_in` | Med (expression) |
| `ortholog`, `paralog` | gene ↔ gene homology | `orthologous_to` / `paralogous_to` | Med (comparative) |
| `mesh` | medical vocabulary | `Disease`/`SmallMolecule` + `close_match` | Med (vocab/xref) |
| `pharmgkb_pathway` | pharmacogenomic pathway | `participates_in` | Low-Med |
| `patent`, `patent_compound` | compound → patent | `mentioned_in` | Low |

## Recommendation

source2 needs no further coverage work. For source1, the Tier-A list is the real
expansion target — ~12 dataset clusters. Most fit existing mechanisms (direct or
reified rules); a few need a new node type (`RegulatoryRegion` for
encode_ccre/fantom5_enhancer) or are best modeled with qualifiers (clinical
significance on civic). Predictive and identifier/derived datasets stay out by
design.
