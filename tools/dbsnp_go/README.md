# dbsnp_poc — dbSNP federation → KGX (proof of concept)

dbSNP is a separate BioBTree federation: one ~118 GB gzip forward index
(`dbsnp_sorted.*.index.gz`, ~1.1B variants) + a 493 GB LMDB. The gene/transcript
links and rich per-variant annotations are **all in the forward stream**, so a
single sequential pass extracts everything — no LMDB random lookups needed.

This is a **proof of concept** to settle feasibility and throughput before wiring
dbSNP in as a first-class KG layer (the goal: largest biolink KG ever, ~1.1B
variant nodes connected to genes + transcripts). Not yet wired into
`full_prod.sh`; the full run happens after the KG shape is agreed.

## Design

Decompression of one gzip stream is the throughput ceiling (~328 MB/s with C
zlib; Go's stdlib gzip is slower). So we **let `zcat` decompress and pipe plaintext
into a pure Go parser**, which fans each (subject-contiguous) variant block out to
N workers for the CPU-heavy JSON parse / format / md5. Output is sharded into N
parts per type, each a valid KGX TSV that drops straight into `assemble`.

```
zcat dbsnp_sorted.*.index.gz | dbsnp_poc -workers 8 -max 5000000 -out out/dbsnp_poc
```

Per variant it emits:

| output | content |
|---|---|
| `dbsnp_nodes.N.tsv.gz` | `DBSNP:<rs>  biolink:SequenceVariant` |
| `dbsnp_edges.N.tsv.gz` | `variant -is_sequence_variant_of-> gene` (entrez, ds 4) and `-> transcript` (refseq, ds 8) |
| `dbsnp_attrs.N.tsv.gz` | `DBSNP:<rs>  {variant_type, variant_class, chromosome, consequence, mane_select, is_common}` |

Edge ids are `biobtree:md5(subject|predicate|object|primary)[:16]` — **identical to
the Python pipeline**, so dbSNP edges dedup consistently at assemble.

## Measured throughput (this box, 32 cores; 5M-variant bounded runs)

| config | rate | extrapolated full (~1.1B) |
|---|---|---|
| Python (current `dbsnp.py`), 1 core | — | ~5–15 hr (json-bound) |
| Go, 1 worker, gzip L6 output | 65 MB/s | ~3.5 hr |
| Go, 1 worker, gzip L1 output | 65 / 173 MB/s (attrs / edges-only) | ~3.5 hr / ~1.3 hr |
| **Go, 8 workers, gzip L1, full attrs** | **292 MB/s, 393K variants/s** | **~47 min** |

8 workers saturate the single `zcat` stream (the ~328 MB/s decompression ceiling);
more workers don't help. **To go below ~47 min** the source must be sharded so
decompression itself parallelizes (a biobtree-side change) — with ~8–16 shards on
32 cores this drops toward ~5–10 min.

## Full-layer scale (extrapolated from measured densities)

- ~1.1B `SequenceVariant` nodes
- ~720M `variant → gene` edges
- ~2.2B `variant → transcript` edges
- rich node attributes on every variant

This is ~100× the largest existing biolink KG.

## Not done here (production wiring, after KG is agreed)

- Gene canonicalization via the Phase-1 `id_map` (POC emits raw `NCBIGene:` ids).
- dbSNP id case normalization (`DBSNP:RS10` vs canonical `dbsnp:rs10`).
- Per-edge transcript consequence as a qualifier (consequence currently only on
  the node attrs; the per-transcript consequence lives in `hgvs_transcripts`).
- **Billion-scale `assemble`**: the current `validate` builds an in-memory set of
  all node ids — at 1.1B that needs a sorted-merge / external-sort dangling check.
