# Repeat Regions (repeat) Dataset

## Overview

`repeat` is genomic repeat / low-complexity / segmental-duplication regions from UCSC hg38,
so a variant — especially a ClinVar **Microsatellite**-type one, which otherwise gets no
annotation — resolves to the repeats it falls in. Three freely-redistributable UCSC tracks:

- **RepeatMasker** (Dfam-based `rmskOutCurrent`, NOT the RepBase-licensed `rmsk`): class
  (Simple_repeat, Satellite, SINE, LINE, LTR, DNA, …), name, family.
- **Tandem Repeats** (`simpleRepeat` / TRF): the repeat motif.
- **Segmental Duplications** (`genomicSuperDups`): the duplicated partner region.

## Access

Interval-indexed — a variant coordinate returns every overlapping repeat:

```bash
curl "http://localhost:9292/ws/map/?i=1:10200&m=>>repeat"        # -> (TAACCC)n Simple_repeat, TRF motif, segdups
curl "http://localhost:9292/ws/map/?i=1:10200:A:G&m=>>repeat"    # variant-key form
```

## Attributes
`chromosome`, `start`, `end`, `repeat_type` (repeatmasker | tandem_repeat | segmental_dup),
`repeat_class`, `repeat_name`, `repeat_family`. Dataset id 817, main federation.
