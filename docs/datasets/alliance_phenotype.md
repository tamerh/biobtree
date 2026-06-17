# Alliance Phenotype Dataset

## Overview

Gene→phenotype associations from the **Alliance of Genome Resources (AGR)** per-MOD
PHENOTYPE files — the experimentally-observed phenotypes of model-organism genes, each
keyed to that organism's phenotype ontology. This is the natural sibling of
[Alliance Disease](alliance_disease.md): where that layer answers "what disease is this
gene a model of?", this one answers "what phenotype is observed when this gene is
perturbed?".

- mouse / rat gene → **MP** (Mammalian Phenotype Ontology)
- worm gene → **WBPhenotype** (C. elegans Phenotype Ontology)
- *Xenopus* gene → **XPO** (Xenopus Phenotype Ontology)

biobtree already carries these phenotype ontologies as first-class nodes (mp, zp,
wbphenotype, xpo); AGR populates them with real gene associations.

**Source**: per-MOD `PHENOTYPE_<MOD>.json.gz` (Alliance FMS, always-latest download URL)
**Data Type**: curated gene→phenotype-term associations across model organisms

## Scope (what is kept)

The Alliance phenotype files are per-MOD JSON (a `data` array of annotations). biobtree
keeps only the clean, joinable slice:

- **Gene-level `objectId` only** — routed by CURIE prefix to the per-species gene
  dataset: `MGI:`→`mgi` (keep prefix), `RGD:`→`rgd`, `ZFIN:ZDB-GENE-`→`zfin`,
  `WB:WBGene`→`wormbase`, `Xenbase:XB-GENE-`→`xenbase`. Allele / affected-genomic-model /
  genotype / transgene `objectId`s (e.g. `ZFIN:ZDB-ALT-*`, `ZFIN:ZDB-FISH-*`,
  `WB:WBVar*`, `WB:WBTransgene*`) are **dropped** (biobtree has no node for those).
- **Phenotype term must be an ontology we have a node for** — each
  `phenotypeTermIdentifiers[].termId` is routed `MP:`→`mp`, `ZP:`→`zp`,
  `WBPhenotype:`→`wbphenotype`, `XPO:`→`xpo`. Any other ontology is **skipped**.
- **MODs ingested**: MGI, RGD, WB, XBXL, XBXT.
- **MODs skipped**:
  - **ZFIN** — its PHENOTYPE file is composed as ZFA (anatomy) + PATO (quality) terms,
    **not ZP**; biobtree has no zfa/pato phenotype node, so it would yield zero joinable
    edges. (Routing for `ZP:`/`zfin` is wired and active if/when ZP terms appear.)
  - **FB** (DPO/FBcv), **SGD** (APO) — phenotype ontologies biobtree does not carry.
  - **HUMAN** (HP) — redundant with the existing HPOA layer.

## Integration Architecture

### Storage Model

**Primary entries**: one record per (gene, phenotype-term, reference), id `AGRP_<hash>`
(deterministic 64-bit FNV of `gene|term|ref`).

**Attributes stored** (protobuf `AlliancePhenotypeAttr`):
- `gene_symbol` (gene id/accession), `species` (routed gene dataset)
- `phenotype_term` (ontology term id), `phenotype_statement` (free-text description)
- `evidence_code`, `source` (`Alliance`)

**Cross-references**:
- → per-species **gene** dataset (`mgi`/`rgd`/`zfin`/`wormbase`/`xenbase`). The MOD
  datasets are optional — edges are emitted only when the target dataset is loaded.
- → phenotype **ontology** term (`mp`/`zp`/`wbphenotype`/`xpo`), in colon form.
- → `pubmed` — supporting citation (only when `publicationId` is a numeric PMID).

**Text search**: gene id/symbol + phenotype statement.

## Use Cases

**1. Model-organism phenotype lookup**
```
Query: >>mgi>>alliance_phenotype>>mp
e.g. MGI:87853 (Abl1) -> MP:0002075 (abnormal coat/hair pigmentation)
```

**2. Cross-species phenotype bridging**
```
Query: model-organism gene -> alliance_phenotype (phenotype) AND -> ortholog (human gene)
```

**3. Phenotype/gene text search** — by gene id or phenotype statement → association records.

## Identity / IDs

- **Dataset id**: 803 (`alliance_phenotype`)
- Record ids are synthetic (`AGRP_<16-hex>`); the phenotype is keyed to its ontology, the
  gene to its species dataset.

## Performance

- **Test Build**: a few seconds (100 associations,
  `./biobtree --include-optionals -d "alliance_phenotype" test`).
- MGI is by far the largest file (~1M annotations); the parser streams the JSON `data`
  array element-by-element rather than loading it whole.
- **Update Frequency**: per Alliance release (the download URLs always serve the latest).

## Known Limitations

- ZFIN currently contributes no edges (ZFA/PATO composition, not ZP) — see Scope.
- MOD gene edges require the optional MOD gene datasets (mgi/rgd/zfin/wormbase/xenbase)
  to be loaded (`--include-optionals`).
- Allele/genotype/transgene-level annotations are intentionally excluded.

## References

- **Website**: https://www.alliancegenome.org/
- **Downloads**: https://www.alliancegenome.org/downloads
- **License**: CC0 / CC-BY (Alliance-generated association files)
- **Citation**: Alliance of Genome Resources Consortium. *Updates to the Alliance of
  Genome Resources central infrastructure.* Genetics (2024).
