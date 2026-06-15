# Alliance Disease Dataset

## Overview

Gene→disease associations from the **Alliance of Genome Resources (AGR)** harmonized
disease file — curated disease links for model organisms (mouse, rat, zebrafish, fly,
worm, yeast, frog) **and** human, all keyed to the Disease Ontology (DOID). This is
biobtree's cross-species / model-organism disease layer: it answers "what disease is
this model-organism gene a model of / implicated in?", and (via biobtree's existing
ortholog edges) bridges those to the human ortholog.

biobtree's other disease edges are almost entirely human (GenCC, ClinVar, CTD, Orphanet);
AGR adds the model-organism dimension and populates DOID — now a [first-class
ontology](doid.md) — with real gene associations.

**Source**: `DISEASE-ALLIANCE_COMBINED.tsv.gz` (Alliance FMS, always-latest download URL)
**Data Type**: curated gene→disease associations across 9 species

## Scope (what is kept)

The Alliance file is large and partly redundant; biobtree keeps only the clean,
differentiated slice:

- **`DBobjectType == gene`** only — allele / affected-genomic-model rows are dropped
  (biobtree has no allele/AGM node).
- **Direct curation only** — association types `is_implicated_in` and `is_marker_for`.
  Orthology-**inferred** rows (`*_via_orthology`, ~78% of the file) are dropped: they are
  derivable from biobtree's Compara orthologs + human disease, and would bloat the index.
- Negative assertions (`is_not_*`) are dropped.
- **Orthology / phenotype / expression** AGR files are **not** ingested here: orthology is
  redundant with Ensembl Compara; phenotype/expression use species ontologies biobtree
  does not yet have as nodes.

## Integration Architecture

### Storage Model

**Primary entries**: one record per association, id `AGRD_<hash>` (deterministic from
gene id + DOID + association type + reference).

**Attributes stored** (protobuf `AllianceDiseaseAttr`):
- `gene_symbol`, `species`
- `association_type` (`is_implicated_in` / `is_marker_for`)
- `disease_name` (DOID term name), `evidence_code`
- `source` (`Alliance`)

**Cross-references**:
- → per-species **gene** dataset, routed from the Alliance CURIE prefix:
  `HGNC:`→`hgnc`, `MGI:`→`mgi`, `RGD:`→`rgd`, `SGD:`→`sgd`, `ZFIN:`→`zfin`,
  `FB:`→`flybase`, `WB:`→`wormbase`, `Xenbase:`→`xenbase`. (MGI/HGNC keep the prefix
  to match biobtree's stored form; the others drop it. The MOD datasets are optional —
  edges are emitted only when the target dataset is loaded.)
- → `doid` — the disease term.
- → `pubmed` — supporting citation (only when the `Ref` is a numeric PMID).

**Text search**: gene symbol + disease name.

## Use Cases

**1. Model-organism disease modeling**
```
Query: >>mgi>>alliance_disease>>doid
e.g. MGI:102674 (Umod) -> DOID:0060062 (autosomal dominant tubulointerstitial kidney disease)
```

**2. Cross-species disease bridging**
```
Query: model-organism gene -> alliance_disease (disease) AND -> ortholog (human gene)
Use: connect a disease to its modeling genes across species
```

**3. Gene/disease text search** — by gene symbol or disease name → association records.

## Identity / IDs

- **Dataset id**: 781 (`alliance_disease`)
- Record ids are synthetic (`AGRD_<16-hex>`); the disease is keyed to DOID, the gene to its
  species dataset.

## Performance

- **Test Build**: ~5s (100 associations, `./biobtree --include-optionals -d "doid,alliance_disease" test`)
- **Update Frequency**: per Alliance release (the download URL always serves the latest)

## Known Limitations

- DOID-keyed (not MONDO); reach MONDO via the existing MONDO→DOID bridge.
- MOD gene edges require the optional MOD gene datasets (mgi/rgd/zfin/sgd/wormbase/xenbase)
  to be loaded (`--include-optionals`); hgnc + flybase are always available.
- Allele/genotype-level and orthology-inferred associations are intentionally excluded.

## References

- **Website**: https://www.alliancegenome.org/
- **Downloads**: https://www.alliancegenome.org/downloads
- **License**: CC0 / CC-BY (Alliance-generated association files; the bundled ONTOLOGY
  blobs are not ingested)
- **Citation**: Alliance of Genome Resources Consortium. *Updates to the Alliance of
  Genome Resources central infrastructure.* Genetics (2024).
