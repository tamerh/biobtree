# FYPO (Fission Yeast Phenotype Ontology) Dataset

## Overview

Fission Yeast Phenotype Ontology (FYPO) provides standardized phenotype terms for fission yeast (S. pombe). biobtree ingests it as a
first-class ontology so its phenotype terms resolve to names and a navigable hierarchy, and so
the [uPheno hub](upheno.md) can bridge FYPO ⇄ human HP and other species' phenotypes.

**Source**: `http://purl.obolibrary.org/obo/fypo.owl` · **License**: CC0 / OBO Foundry open

## Integration

- Generic OWL ontology parser (terms + synonyms + `fypoparent`/`fypochild` hierarchy +
  text search). IDs as `FYPO:...`.
- **Dataset id**: 797 (+ `fypoparent` 798, `fypochild` 799).
- Cross-species: `>>fypo>>upheno>>hpo` (fission yeast (S. pombe) → human phenotype) via the uPheno hub.

## References
- OBO Foundry: http://obofoundry.org/ontology/fypo.html
