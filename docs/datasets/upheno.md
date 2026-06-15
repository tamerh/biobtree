# uPheno (Unified Phenotype Ontology) — cross-species phenotype hub

## Overview

uPheno is the **cross-species phenotype hub**: it unifies the species-specific phenotype
ontologies (human HP, mouse MP, zebrafish ZP, xenopus XPO, worm WBPhenotype, fission-yeast
FYPO) under common `UPHENO:` grouping classes, so a phenotype in one species can be related
to the equivalent phenotype in another.

biobtree ingests uPheno as a labeled hub ontology **plus** its cross-species mapping file,
producing `species ⇄ UPHENO` edges. Combined with biobtree's existing **HP** (which already
links to genes and diseases), this lets a human phenotype reach its model-organism
equivalents:

```
HP:0001844  >>hpo>>upheno>>mp  ->  MP:0009049   (human -> mouse phenotype)
```

This is the connective tissue for cross-species work: with the [Alliance disease layer](alliance_disease.md)
already in place and model-organism gene→phenotype data as a future phase, a model-organism
gene's phenotype can be bridged to the human phenotype and on to human disease.

**Source**: `upheno.owl` (grouping terms + hierarchy) + `upheno-cross-species.sssom.tsv` (the bridge)
**Data Type**: cross-species phenotype grouping ontology + exact-match mappings

## Companion species phenotype ontologies

Ingested as first-class ontologies via biobtree's generic OWL parser (terms + synonyms +
hierarchy + text search), so the hub edges connect **real, labeled** nodes — never dangling:

| Dataset | Ontology | Species |
|---|---|---|
| [mp](mp.md) | Mammalian Phenotype Ontology | mouse (and rat) |
| [zp](zp.md) | Zebrafish Phenotype Ontology | zebrafish |
| [xpo](xpo.md) | Xenopus Phenotype Ontology | frog |
| [wbphenotype](wbphenotype.md) | C. elegans Phenotype Ontology | worm |
| [fypo](fypo.md) | Fission Yeast Phenotype Ontology | fission yeast |

(Human **HP** is already ingested as `hpo`.)

## Integration Architecture

### Storage Model

**uPheno terms**: `UPHENO:xxxxxxx` grouping classes stored as ontology entries (name,
synonyms) with `uphenoparent`/`uphenochild` hierarchy. Text-searchable.

**The bridge** (`upheno-cross-species.sssom.tsv`, predicate `semapv:crossSpeciesExactMatch`):
the hub is built by processing rows whose **subject is a `UPHENO:` class** and whose **object
is a species phenotype term in a registered dataset** (HP/MP/ZP/XPO/WBPhenotype/FYPO). Each
becomes a direct `UPHENO ⇄ species` edge (bidirectional). Rows to unregistered prefixes
(e.g. fly FBcv, planaria PLANP) are skipped so no bare nodes are created.

Cross-species traversal flows through the hub:
```
>>hpo>>upheno>>mp    HP phenotype -> mouse equivalents
>>mp>>upheno>>zp     mouse phenotype -> zebrafish equivalents
>>upheno>>hpo        a UPHENO class -> its human phenotype(s)
```

### How it is wired

The SSSOM step lives in the generic ontology parser (`src/update/ontology.go`,
`processCrossSpeciesSssom`), gated on the dataset's `pathSssom` config key — a no-op for
every ontology except uPheno. The species ontologies need no custom code (generic OWL parser).

## Identity / IDs

- `upheno` 782 (+ `uphenoparent` 783, `uphenochild` 784)
- `mp` 785, `zp` 788, `xpo` 791, `wbphenotype` 794, `fypo` 797 (each + parent/child)

## Performance

- **Test Build**: `./biobtree -d "hpo,mp,upheno" test`; hub creates ~25K species↔UPHENO edges from the SSSOM (HP 5466, MP 8153, ZP 6943, XPO 2550, WBPhenotype 548, FYPO 400).
- **Update Frequency**: per uPheno release.

## Known Limitations

- Only the species ontologies listed above are bridged; fly (FBcv prefix, low coverage) and
  niche ontologies (DDPHENO/PHIPO/PLANP/MGPO/APO) are excluded.
- The hub uses `crossSpeciesExactMatch` only (uniform predicate; not stored per edge).
- Connectivity is via shared `UPHENO:` classes — direct species↔species curated pairs that
  do not share a UPHENO class are not separately added.

## References

- **Website / GitHub**: https://github.com/obophenotype/upheno
- **Mappings**: https://github.com/obophenotype/upheno-dev/blob/master/src/mappings/upheno-cross-species.sssom.tsv
- **OBO Foundry**: http://obofoundry.org/ontology/upheno.html
- **Paper**: Matentzoglu N et al. *The Unified Phenotype Ontology (uPheno).* (2024) — PMC11429889
- **License**: CC0 1.0 (uPheno OWL + SSSOM); component species ontologies are OBO Foundry open-license.
