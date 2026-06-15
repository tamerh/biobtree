# WBPhenotype (C. elegans Phenotype Ontology) Dataset

## Overview

C. elegans Phenotype Ontology (WBPhenotype) provides standardized phenotype terms for worm (C. elegans). biobtree ingests it as a
first-class ontology so its phenotype terms resolve to names and a navigable hierarchy, and so
the [uPheno hub](upheno.md) can bridge WBPhenotype ⇄ human HP and other species' phenotypes.

**Source**: `http://purl.obolibrary.org/obo/wbphenotype.owl` · **License**: CC0 / OBO Foundry open

## Integration

- Generic OWL ontology parser (terms + synonyms + `wbphenotypeparent`/`wbphenotypechild` hierarchy +
  text search). IDs as `WBPhenotype:...`.
- **Dataset id**: 794 (+ `wbphenotypeparent` 795, `wbphenotypechild` 796).
- Cross-species: `>>wbphenotype>>upheno>>hpo` (worm (C. elegans) → human phenotype) via the uPheno hub.

## References
- OBO Foundry: http://obofoundry.org/ontology/wbphenotype.html
