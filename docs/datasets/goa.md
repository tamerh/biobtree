# GOA Human Dataset

## Overview

Human GO annotations from the GO Consortium (GAF 2.2), exposed as **one entry
per GO term** with its annotated human genes. This is the data behind the
GO-native enrichment mode; it is independent of the MSigDB `C5` collections,
which do not cover every GO term.

**Dataset ID**: `828`
**Federation**: `main`
**Source**: `https://current.geneontology.org/annotations/goa_human.gaf.gz`
**License**: GO Consortium annotations (CC BY 4.0)

## What is stored

For each GO term with at least one resolvable human gene:

- `standard_name` — GO term name
- `collection` — `GOA:BP`, `GOA:MF` or `GOA:CC`
- `gene_count` — number of annotated human genes in the set
- xrefs to `hgnc` for every annotated gene

Only `taxon:9606`, non-`NOT` annotations are used. Gene symbols are resolved to
their **official HGNC** symbol; unresolved symbols are skipped. A synthetic
`GOA:UNIVERSE` entry stores the number of unique human genes for the enrichment
background.

## Access

```
GO:0004383 >>goa>>hgnc        # genes annotated with guanylate cyclase activity
HGNC:4685 >>hgnc>>goa         # GO terms for GUCY1A1 (gene sets)
```

Lite target schema:

```
id|standard_name|collection|description|gene_count
```

## GO-native enrichment

```
biobtree_enrich(genes=["GUCY1A1","GUCY1B1",...], collection="GO")
biobtree_enrich(genes=[...], collection="GO:MF")
```

- `GO` — all namespaces
- `GO:BP` / `GO:MF` / `GO:CC` — one namespace
- The background is the `GOA:UNIVERSE` gene count (currently 19,500), not the
  MSigDB background.
- MSigDB collections remain available as `C5:GO`, `C5:GO:BP`, etc.

## Notes

- The GAF release is dated (May 2026 in the current download); terms annotated
  only in newer QuickGO data will still be missing. `GO:1903960` is one example:
  QuickGO has two human annotations (TCAF2, GOPC), the current GAF has none —
  the TCAF2 curation is assigned to `GO:0044325` there instead.
- Enrichment only tests sets of 5–500 genes, so even a term present with one or
  two annotations (e.g. `GO:1903960` after a QuickGO sync) would not appear in
  results. `tested_sets` and a term's absence are therefore not proof the
  namespace or the annotation source is missing.
- This dataset is the source for GO-native enrichment, not a replacement for
  the GO ontology (`go`), which carries the term hierarchy and definitions.
