# MaveDB Dataset

## Overview

**MaveDB** (Multiplexed Assays of Variant Effect) provides **direct experimental
functional-evidence** scores from deep mutational scanning (DMS) and other MAVE
assays. Unlike computational predictors (AlphaMissense, REVEL, SaProt), these are
*measured* variant effects — the strongest evidence class for the ACMG **PS3/BS3**
criteria — so they complement and anchor the prediction layer.

One record per measured variant, carrying its assay score and identity:

| Field | Meaning |
|-------|---------|
| `gene_symbol` | Target gene |
| `score_set` | MaveDB score-set URN (the experiment) |
| `score_set_title` | Human-readable assay description |
| `hgvs_pro` | Protein-level HGVS (e.g. `p.Thr167Cys`) |
| `hgvs_nt` / `hgvs_splice` | Nucleotide / splice HGVS where applicable |
| `score` | Measured functional score (raw, per score-set scale) |
| `uniprot` | UniProt accession of the target |
| `category` | Target category (e.g. `protein_coding`) |
| `license` | Per-score-set data license (e.g. `CC0`) |

**Data Type**: per-variant experimental functional measurements
**Dataset ID**: `807` · **Federation**: `main`

## Source & License

- Source: the MaveDB Zenodo **bulk dump** (`mavedb-dump.zip`) — `main.json`
  (nested `experimentSets[] → experiments[] → scoreSets[]`) + per-score-set
  `csv/<urn ":"→"-">.scores.csv`. Refreshed biannually (May/Nov).
- License: MaveDB data defaults to **CC0 1.0** (public domain) unless a score set
  notes otherwise; the per-set license is read and **stored on every record**
  (`license` attr) so downstream/export logic can treat sets individually. Nothing
  worse than CC-BY-NC-SA occurs → all sets are KG-export compatible.
- Human score sets only (taxId 9606). Real ingest: ~3.8 M variants / ~1,140 human
  score sets.

## Integration Architecture

- **`mavedb`** (id 807): one record per variant, keyed by the MaveDB URN
  `URN:MAVEDB:<scoreset>#<row>` (e.g. `URN:MAVEDB:00000081-A-1#1`).
- `hasFilter: yes` — e.g. `mavedb.category=="protein_coding"`.
- `compact_fields`: `gene_symbol,hgvs_pro,score,score_set,license`.
- **Joins** (bidirectional, via the score-set target): a variant links to
  `uniprot`, `hgnc`, `ensembl`, and `transcript`; the measured `score` is carried
  as **edge evidence** on those links, so a `gene/protein >> mavedb` traversal
  surfaces the functional score directly.

## Committed vs production path

The committed conf `path` points at a small **test fixture**
(`tests/datasets/mavedb/fixture/`); production ingest overrides it with the real
extracted Zenodo dump (`raw_data/mavedb/extracted`, git-ignored).

## Query Examples

```bash
# Search a gene's MaveDB variants
curl "http://localhost:9292/ws/?i=BRCA1&s=mavedb"

# Reverse join: protein/gene -> MaveDB functional scores
curl "http://localhost:9292/ws/map/?i=P38398&m=>>uniprot>>mavedb"

# Entry by URN
curl "http://localhost:9292/ws/entry/?i=URN:MAVEDB:00000081-A-1%231&s=mavedb"
```
