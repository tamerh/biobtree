# XPO (Xenopus Phenotype Ontology) Dataset

## Overview

Xenopus Phenotype Ontology (XPO) provides standardized phenotype terms for frog (Xenopus). biobtree ingests it as a
first-class ontology so its phenotype terms resolve to names and a navigable hierarchy, and so
the [uPheno hub](upheno.md) can bridge XPO ⇄ human HP and other species' phenotypes.

**Source**: `http://purl.obolibrary.org/obo/xpo.owl` · **License**: CC0 / OBO Foundry open

## Integration

- Generic OWL ontology parser (terms + synonyms + `xpoparent`/`xpochild` hierarchy +
  text search). IDs as `XPO:...`.
- **Dataset id**: 791 (+ `xpoparent` 792, `xpochild` 793).
- Cross-species: `>>xpo>>upheno>>hpo` (frog (Xenopus) → human phenotype) via the uPheno hub.

## References
- OBO Foundry: http://obofoundry.org/ontology/xpo.html
