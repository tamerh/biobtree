# WikiPathways Dataset

## Overview

Community-curated biological pathways from
[WikiPathways](https://www.wikipathways.org). The dataset is built from the
released **GMT** files, one per species, and covers 18 species (human, mouse,
rat, zebrafish, fly, worm, yeast, Arabidopsis, and others).

**Data Type**: pathway → gene
**Dataset ID**: `825`
**License**: CC0 1.0 (public domain)

## Access

One entry per pathway, keyed by its WikiPathways id:

```
WP100
```

```
# pathway -> genes
/ws/map/?i=WP100&m=>>wikipathways>>hgnc

# gene -> pathways
/ws/map/?i=TP53&m=>>hgnc>>wikipathways

# direct entry
/ws/entry/?i=WP100&s=wikipathways
```

Human pathways link to **hgnc** (Entrez is used only for genes that do not
resolve to HGNC). Other species stay in the **entrez** namespace, so a mouse
pathway is not stapled onto human genes.

## Fields

| field | meaning |
|-------|---------|
| `name` | pathway name (e.g. `Glutathione metabolism`) |
| `species` | species name (e.g. `Homo sapiens`) |
| `tax_id` | NCBI taxonomy id |
| `gene_count` | number of unique genes in the pathway |
| `release` | WikiPathways release (e.g. `WikiPathways_20261010`) |

## Source

- `https://data.wikipathways.org/current/gmt/`
- One GMT file per species, named `wikipathways-YYYYMMDD-gmt-<Species>.gmt`.
- GMT line format: `name%WikiPathways_YYYYMMDD%WPID%Species`, URL, then Entrez ids.
- License: [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/).

## Notes

- The parser reads the directory index and picks the newest file per species,
  so monthly releases do not require a config change.
- Human genes are linked through a cached Entrez→HGNC resolution. The WPID is
  the primary key and is deliberately not self-indexed as a text link, which
  would duplicate map results.
