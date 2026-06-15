# DOID (Human Disease Ontology) Dataset

## Overview

The Human Disease Ontology (DO) is a community-driven, OBO Foundry disease vocabulary
that provides standardized disease concepts with a logically-defined hierarchy and
extensive cross-references to other medical vocabularies. DOID terms (e.g. `DOID:0001816`)
are widely used as disease keys by external resources — notably the Alliance of Genome
Resources, which keys its model-organism disease associations to DOID.

biobtree ingests DOID as a first-class ontology (terms, synonyms, parent/child hierarchy,
text search), the same way it ingests MONDO/HPO/GO/Uberon/CL. Previously DOID existed only
as a passive xref stub; promoting it to a full ontology means DOID-keyed edges resolve to
named, navigable terms.

**Source**: DOID OWL file (`http://purl.obolibrary.org/obo/doid.owl`)
**Data Type**: Disease ontology with hierarchical classification and synonyms

> Not to be confused with **DOI** (Digital Object Identifier, dataset `doi`) — DOID is the
> *Disease Ontology ID*, an unrelated namespace.

## Integration Architecture

### Storage Model

**Primary Entries**:
- DOID IDs (e.g. `DOID:0001816`) stored as main identifiers, colon-prefixed form

**Searchable Text Links**:
- Disease names and synonyms indexed for name-based lookups

**Attributes Stored** (protobuf `OntologyAttr`, shared with the other OBO ontologies):
- Name (primary disease name)
- Synonyms (alternative disease names)
- Type (OBO namespace, `disease_ontology`)

**Cross-References**:
- **Hierarchical**: parent/child relationships via the `doidparent` / `doidchild` virtual datasets
- **Bridge to MONDO**: MONDO already ingests ~11,800 `xref: DOID:` equivalences, so DOID is
  reachable from biobtree's main disease graph via MONDO (e.g. `>>mondo>>doid`).

### How it is parsed

DOID rides the generic OWL ontology parser (`src/update/ontology.go`) with
`idPrefix:"DOID:"` and `prefixURL:"http://purl.obolibrary.org/obo/"`, identical to the
GO/Uberon/CL path. No dataset-specific parser code.

## Use Cases

**1. Resolve DOID-keyed associations** — Alliance / CIViC diseases coded as DOID resolve to
real disease names and synonyms instead of bare IDs.

**2. Disease hierarchy navigation**
```
Query: DOID term → >>doid>>doidparent → broader disease class
e.g. DOID:0001816 (angiosarcoma) → DOID:175 (vascular cancer)
```

**3. Bridge DOID ↔ MONDO**
```
Query: DOID:xxxx → >>doid (xref) ... or MONDO:yyyy → >>mondo>>doid
Use: move between the two disease vocabularies for downstream mapping
```

**4. Disease text search** — lookup by disease name or synonym returns the DOID term.

## Identity / IDs

- **Dataset id**: 751 (`doid`); virtual hierarchy datasets `doidparent` (748), `doidchild` (749)
- **Namespace**: all terms use the `DOID:` prefix

## Performance

- **Test Build**: ~5s (100 DOID entries, `./biobtree -d doid test`)
- **Update Frequency**: regular OBO Foundry releases

## Known Limitations

- Largely overlaps MONDO conceptually (MONDO subsumes DOID); ingested primarily as a
  resolution target for DOID-keyed external data.
- Definitions are not stored (only name, type, synonyms in `OntologyAttr`).
- Parent/child navigation requires the `doidparent` / `doidchild` virtual datasets.

## References

- **Website**: https://disease-ontology.org/
- **GitHub**: https://github.com/DiseaseOntology/HumanDiseaseOntology
- **OBO Foundry**: http://obofoundry.org/ontology/doid.html
- **License**: CC0 1.0 (public domain dedication)
