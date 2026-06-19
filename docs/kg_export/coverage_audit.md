# KG Export — Dataset Coverage Audit

Audit of `conf/source1.dataset.json` + `conf/source2.dataset.json` against what the
KG exporter emits (nodes via `categories.yaml`, edges via `predicates.yaml`
direct/reified, runtime builders `go`/`refseq`/`dbsnp`/`mesh`, and `ontology.py`
hierarchy). Datasets in other conf files (`xref*`) are derived / identifier-only
and out of scope.

## Automated drift check (run this when BioBTree changes)

```bash
python -m tools.kg_export.coverage_audit --conf /data/biobtree/conf
```

Classifies every source1/source2 dataset as **covered** / **skipped** / **UNEXPLAINED**
and exits non-zero on any unexplained gap — so it doubles as a post-build / CI
check. Backed by `mappings/coverage_skip.yaml`, which lists every *intentionally*
uncovered dataset with a reason. The "covered" set is derived live from the
mappings, so it stays current automatically.

**Workflow when a new dataset appears (flagged UNEXPLAINED):**
1. Sync conf if the worktree lags main:
   `git checkout main -- conf/source1.dataset.json conf/source2.dataset.json`
2. Run the audit; for each flagged dataset either **author coverage** (a rule /
   runtime builder) or **add a `coverage_skip.yaml` entry** with a reason.
3. Re-run until clean (0 unexplained).

*("Improved connections" on existing datasets need no action — they live in the
index data and flow through on re-export. Only new datasets need rules.)*

## Current state

191 source1+source2 datasets: **133 covered, 58 skipped (intentional), 0
unexplained.** The historical narrative below records how the gaps were closed.

"Not covered" is mostly **intentional** (identifier/derived, predictive, sub-entity
of a covered dataset, or qualifier-role) — all enumerated in `coverage_skip.yaml`.

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
expansion target — most fit existing mechanisms (direct or reified rules).

## Tier-A status (after the agent batch)

**Added** (verified against the real index; in `full_prod.sh`):

| Cluster | Edges (full) | Modeling |
|---|---|---|
| alliance_disease | 50,732 | bipartite, cross-species gene→DOID (`gene_associated_with_condition`); needed a new `extra_subjects` field for the 6 MOD gene namespaces |
| clinical_trials | 743,863 | bipartite drug→condition (`in_clinical_trials_for`) |
| ctd_gene_interaction | 2,087,632 | bipartite chemical→gene (`affects`); new `biolink:ChemicalEntity` node (MESH) |
| ortholog / paralog | ~53M | direct gene↔gene (`orthologous_to`/`paralogous_to`) via the ensembl forward |
| civic (variant/evidence/assertion) | ~18,500 | variant→gene + variant→disease/drug (`civic.vid` SequenceVariant) |
| cellxgene_celltype | 14,261 | bipartite cell-type→tissue (`located_in`) — *not* gene expression (no gene endpoint) |
| brenda | 53 | direct metabolite→EC (`participates_in`); brenda typed MolecularActivity (EC) |

**Added in round 2** (Atlas-validated — checked how `/data/sugi-atlas` uses them):

| Cluster | Edges (full) | Modeling | Atlas chain |
|---|---|---|---|
| mirdb | 6.65M | miRNA→transcript `affects` (prediction); reified `canonical()` falls back to the RefSeq runtime prefix | `>>hgnc>>refseq>>mirdb` |
| generif | 1.70M | publication→gene `mentions`; new `biolink:Publication` (PMID) node | `>>entrez>>generif` |
| jaspar | ~9k | TF-motif→protein `directly_physically_interacts_with` + `in_taxon`; new `biolink:NucleicAcidSequenceMotif` node | `>>uniprot>>jaspar` |
| mesh (disease subset) | 3.3k | `mesh.py` builder: disease-tree (C*/F03*) MeSH → Disease nodes (5.2k of 355k) + `mondo→mesh` `close_match` | `>>mondo>>mesh`, `>>mesh>>clinical_trials` |

**Still deferred (with reason):**
- **ctd_disease_association** — its disease object is full MeSH; only the disease
  *subset* is now typed (via mesh.py), and CTD's chemical↔disease needs both ends
  resolved — revisit once MeSH disease nodes are wired as edge targets.
- **jaspar** — done (motif→TF protein). NOT a gap (BioBTree team confirmed): a
  motif belongs to its TF, `uniprot→hgnc/ensembl` gives the TF gene, and the
  TF→target regulation layer is CollecTRI (present + connected, ~100k edges each
  to ensembl/entrez/hgnc). So `jaspar→uniprot→hgnc→collectri→target` is fully
  traversable.
- **fantom5_enhancer** — deferred by choice, not a gap. It already has ~1.2M
  enhancer→gene edges, but they are **proximity-based** (nearest genes via the
  gene-coordinate index), not curated regulation. Modeling it would need a
  `RegulatoryRegion` node + a proximity/association predicate; curated regulation
  would require ingesting FANTOM5's enhancer–TSS association file (a new source).
- **encode_ccre** — deferred. The source (ENCODE/SCREEN BED9+1: coordinates +
  cCRE class) ships **no target-gene assignments**, so there's nothing to link
  from the registry — links only to taxonomy. Not a BioBTree bug; cCRE catalogs
  don't include targets.
- **brenda_kinetics / brenda_inhibitor** — free-text substrate/inhibitor (no
  CURIE) + numeric Km/Ki (needs numeric-qualifier support).

Predictive (alphafold/alphamissense/spliceai) and identifier/derived datasets
stay out by design.
