# MP (Mammalian Phenotype Ontology) Dataset

## Overview

The Mammalian Phenotype Ontology (MP) provides standardized terms for mammalian phenotypes,
used heavily by MGI (mouse) and RGD (rat) to annotate gene→phenotype associations. biobtree
ingests MP as a first-class ontology so mouse/rat phenotype terms resolve to names and a
navigable hierarchy, and so the [uPheno hub](upheno.md) can bridge MP ⇄ human HP and other
species.

**Source**: `http://purl.obolibrary.org/obo/mp.owl` · **License**: CC0 / OBO Foundry open

## Integration

- Generic OWL ontology parser (terms + `hasExactSynonym` synonyms + `mpparent`/`mpchild`
  hierarchy + text search). IDs as `MP:nnnnnnn`.
- **Dataset id**: 785 (+ `mpparent` 786, `mpchild` 787).
- Cross-species: `>>mp>>upheno>>hpo` (mouse → human phenotype) via the uPheno hub.

## References
- Website: https://www.informatics.jax.org/vocab/mp_ontology
- OBO Foundry: http://obofoundry.org/ontology/mp.html
