# CADD Dataset

## Overview

**CADD** (Combined Annotation Dependent Depletion) is a genome-wide
variant-deleteriousness score. It integrates dozens of annotations (conservation,
regulatory, protein-effect, etc.) into a single machine-learning model that
contrasts observed human-derived variants against simulated ones, so **every
possible variant** gets a score — not just observed ones. This is CADD's defining
value: it scores novel/private variants that have never been seen in any cohort.

Two numbers exist upstream; biobtree stores only the first:

| Score | Stored | Meaning |
|-------|--------|---------|
| `phred` | ✅ | PHRED-scaled C-score. Higher = more deleterious. ~10 ≈ top 10%, **~20 ≈ top 1%** (a common pathogenicity cutoff), ~30 ≈ top 0.1% of all possible SNVs. |
| raw_score | ❌ dropped | Raw SVM output; only meaningful for within-version custom re-ranking. Dropping it saves ~21% of on-disk size for no practical loss. |

**Data Type**: per-variant numeric score (genome-wide)
**Assembly**: GRCh38 / hg38
**Dataset ID**: `23` (chosen < 128 so the dataset id is a 1-byte protobuf varint — a deliberate ~2% size saving on a multi-billion-row set)
**Federation**: `cadd` (its own federation; ~350 GB built on `/data`)

## Key scheme & access

Keyed **`chr:pos:ref:alt`** (GRCh38, no `chr` prefix — e.g. `7:117559593:G:A`).
Each (ref, alt) at a position is a distinct entry.

CADD is a **direct-lookup** layer — exactly like the `conservation` federation it
has **no reverse, gene, or text links** (1 KV per variant, the minimal footprint).
Reach a score by looking up the variant key directly:

```
# direct entry lookup
/ws/?i=7:117559593:G:A&d=1
# -> { "Cadd": { "phred": "35.900" } }

# filter to deleterious variants (phred is stored as a string; convert in CEL)
/ws/?i=<variant list>&d=1&f=double(cadd.phred)>20.0
```

Because `phred` is stored as a string (to never drop a meaningful low/zero score
to proto3 omitempty), numeric filters use CEL's `double(cadd.phred)` conversion.

## Source

- Whole-genome SNVs: `https://krishna.gs.washington.edu/download/CADD/v1.7/GRCh38/whole_genome_SNVs.tsv.gz` (~8.6 B SNVs; exhaustive)
- gnomAD indels: `https://krishna.gs.washington.edu/download/CADD/v1.7/GRCh38/gnomad.genomes.r4.0.indel.tsv.gz` (observed indels only)
- Version: CADD GRCh38 **v1.7**
- Columns: `Chrom  Pos  Ref  Alt  RawScore  PHRED`

## Size

Measured ~**40 bytes/variant** on disk (LMDB; dominated by the ~15-byte key +
per-KV overhead — the `phred` value is tiny). Full genome-wide ≈ **345–360 GB**,
its own `cadd` federation. Comparable to the dbsnp federation (520 GB).

## Notes

- Why not key by dbSNP rs id? ~88% of CADD's SNVs are novel/unobserved and have no
  rs id, so an rs key would discard most of CADD and add a failing lookup hop for
  exactly the novel variants CADD exists to score. Coordinate is the natural key.
- Testing: `tests/datasets/cadd/` (hand-crafted fixture, NOT real CADD data).
