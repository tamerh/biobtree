# Wikidata Symptom (P780) Dataset

## Overview

`wikidata_symptom` adds a **common-disease symptom/sign layer** to biobtree, built from
Wikidata property **P780** ("symptoms and signs"). Each statement links a disease to a
symptom term; biobtree keeps the statements whose disease subject carries a **Mondo**
(P5270) or **DOID** (P699) mapping, so every edge anchors to a disease node biobtree
already has.

### Why this dataset exists

The authoritative *structured* biomedical disease→phenotype source is **HPO's disease
annotations** (`phenotype.hpoa`), which biobtree already ingests (`hpo` dataset). But HPO's
disease annotations are curated through OMIM / Orphanet / DECIPHER and are therefore
**rare / Mendelian-disease skewed**: common diseases such as type 2 diabetes, asthma,
migraine and hypertension carry few or no HPO phenotypes. SNOMED CT does cover
common-disease clinical findings, but it is license-restricted and cannot be
redistributed. Wikidata P780 is **CC0** and, being editor-curated from general medical
knowledge, fills exactly the common-disease symptom gap the structured ontologies leave
empty.

`wikidata_symptom` therefore **complements** (does not duplicate) HPO:

| Source | Coverage | Nature |
|--------|----------|--------|
| `hpo` (phenotype.hpoa) | rare / Mendelian disease, dense | curated clinical phenotype |
| `wikidata_symptom` (P780) | common disease, broad | general-knowledge symptoms (CC0) |

> **Provenance note:** this is general-knowledge, crowd-curated data, not clinical-grade
> curation. It is intended as a common-disease symptom hint layer, not a diagnostic source.

**Source**: Wikidata Query Service (SPARQL over property P780), CC0
**Data Type**: disease → symptom/sign associations, gated to Mondo/DOID-mapped diseases

## Integration Architecture

### Storage Model

**Primary Entries**:
- Symptom terms keyed by their **Wikidata QID** (e.g. `Q86` = headache), stored as
  protobuf `OntologyAttr` (shared with the OBO ontologies) with `type = "symptom"` and the
  English label as `name`.

**Searchable Text Links**:
- Symptom labels indexed for name-based lookup (e.g. searching `headache` returns `Q86`).

**Cross-References** (bidirectional):
- Disease → symptom edges onto the `mondo` and/or `doid` term for each kept statement, so
  both `>>mondo>>wikidata_symptom` / `>>doid>>wikidata_symptom` and the reverse
  (`Q86 >>doid`, `Q86 >>mondo`) resolve.

### Scope / gating

Only P780 statements whose disease subject is Mondo- or DOID-mapped are ingested. Unmapped
P780 subjects in Wikidata are overwhelmingly chemical / toxicology items (poisons, drug
toxicity) and are dropped as noise. At the time of writing the gated set is roughly **900
diseases, ~3,800 disease→symptom edges, ~760 distinct symptom terms**.

## Data Preparation

The dataset is produced by a SPARQL extract, not a static download:

```bash
python3 src/scripts/wikidata/extract_wikidata_symptoms.py \
    --output raw_data/wikidata/wikidata_symptoms.tsv
```

Output TSV columns: `symptom_qid`, `symptom_label`, `mondo`, `doid` (one row per
disease-symptom edge; Wikidata's `MONDO_xxxxxxx` is normalised to `MONDO:xxxxxxx`, DOID is
already `DOID:xxxxx`). The Go parser runs this script automatically when the TSV is absent
(same pattern as ChEMBL). In test mode the committed fixture
(`tests/datasets/wikidata_symptom/wikidata_symptoms_fixture.tsv`) is used instead, so the
suite runs offline with no conf fixture↔live toggle.

## Example Queries

Symptom terms (like all ontology terms in biobtree) are mapping **targets**, not query
roots. Reach them from a disease **name** (first `>>` is a text lookup) through the `doid` /
`mondo` disease hub, or read them directly off a disease with the entry endpoint.

```
# Disease name -> symptoms (map chain)
Query: >>doid>>wikidata_symptom
e.g. migraine -> Q86 (headache), Q186889 (nausea), Q281289 (photophobia), Q127076 (vomiting)

Query: >>mondo>>wikidata_symptom
e.g. type 2 diabetes mellitus -> polydipsia, polyphagia, polyuria

# Symptom name -> diseases that present it (reverse, bidirectional)
Query: >>wikidata_symptom>>doid
e.g. headache -> 75+ DOID diseases

# Terminal filter (wikidata_symptom reuses OntologyAttr, same as mondo/doid filters)
Query: >>doid>>wikidata_symptom[name=="headache"]
```

```bash
# As HTTP map calls
curl "http://localhost:9292/ws/map/?i=migraine&m=>>doid>>wikidata_symptom"
curl "http://localhost:9292/ws/map/?i=headache&m=>>wikidata_symptom>>doid"

# Read a disease's symptoms directly off the term (entry endpoint needs the source dataset s=)
curl "http://localhost:9292/ws/entry/?i=DOID:6364&s=doid"     # xrefs: wikidata_symptom|5
curl "http://localhost:9292/ws/entry/?i=MONDO:0015887&s=mondo" # xrefs: wikidata_symptom|3

# Find a symptom term by its label (text search)
curl "http://localhost:9292/ws/?i=headache&d=0"                # -> Q86
```

## Known Limitations

- Disease-ontology IDs (`MONDO:`, `DOID:`) and symptom QIDs are **not valid map roots** — this
  is biobtree-wide, not specific to this dataset. Start a chain from a disease/symptom **name**
  (text lookup) or from a gene/variant root, or use the entry endpoint for a known ID.
- General-knowledge, crowd-curated data — a common-disease symptom hint layer, not clinical-grade
  curation. Use [`hpo`](hpo.md) for curated (rare-disease-dense) phenotype annotations.
- Gated to Mondo/DOID-mapped diseases; non-mapped P780 subjects (chemical/toxicology noise) are
  dropped.

## Notes

- Reuses the shared `OntologyAttr` message, so no new protobuf type / compact extractor was
  needed — only the attr dispatch (mergeg), CEL filter decl + eval activation, and the conf
  entry.
- Dataset id `813`, federation: main. See also [`hpo`](hpo.md), [`mondo`](mondo.md),
  [`doid`](doid.md).
