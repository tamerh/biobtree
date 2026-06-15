# ZP (Zebrafish Phenotype Ontology) Dataset

## Overview

Zebrafish Phenotype Ontology (ZP) provides standardized phenotype terms for zebrafish. biobtree ingests it as a
first-class ontology so its phenotype terms resolve to names and a navigable hierarchy, and so
the [uPheno hub](upheno.md) can bridge ZP ⇄ human HP and other species' phenotypes.

**Source**: `http://purl.obolibrary.org/obo/zp.owl` · **License**: CC0 / OBO Foundry open

## Integration

- Generic OWL ontology parser (terms + synonyms + `zpparent`/`zpchild` hierarchy +
  text search). IDs as `ZP:...`.
- **Dataset id**: 788 (+ `zpparent` 789, `zpchild` 790).
- Cross-species: `>>zp>>upheno>>hpo` (zebrafish → human phenotype) via the uPheno hub.

## References
- OBO Foundry: http://obofoundry.org/ontology/zp.html
