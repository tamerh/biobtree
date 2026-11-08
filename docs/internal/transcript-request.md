# Internal: Sugi Variant transcript request

Status: Stage A + `refseq_exon` live in main `db_v56` · Last updated: 2026-11-08 · Context: v2.20.6

Request from Sugi Variant: transcript-explicit variant pages. Four asks, mapped to
what exists and what needs building.

## 1. MANE Plus Clinical — done

Already stored in `refseq` (`is_mane_select`, `is_mane_plus_clinical`) and now
exposed in lite rows together with the accession version and Ensembl transcript:

```
id|symbol|type|status|chromosome|is_mane_select|is_mane_plus_clinical|version|ensembl_transcript
NM_001374258|BRAF|mRNA|REVIEWED|7|false|true|NM_001374258.1|ENST00000644969.2
```

Filter works today:

```
hgnc >> ensembl >> refseq[is_mane_plus_clinical == true]
```

The flags are independent; in the current MANE summary an accession has one
status, but the schema can carry both. Commit: `b6fa846`. No data rebuild.

## 2. Per-transcript `hgvs_p`, consequence, NMD — partially cheap, VEP for the rest

Investigation: ClinVar XML already carries `ProteinExpression` and
`MolecularConsequence` inside `HGVS` blocks, but `clinvar_xml.go` parses only
`NucleotideExpression`. That is why BioBTree's `hgvs_expressions` has no p.
forms.

Plan:

- **Stage A (implemented):** add a structured repeated field
  `hgvs_transcripts` to `ClinvarAttr`, one record per (variation, transcript):
  `transcript` (with version), `hgvs_c`, `hgvs_p`, `consequence`, `so_id`,
  `mane_select`, `mane_plus_clinical`.
  Pairing note: `ProteinExpression` is sometimes in a separate `HGVS` block
  (e.g. `NM_002111.6` in one, `NP_002102.4` in another), so link via the
  transcript's `protein_accession`/RCV title, not only block adjacency.
- **Stage B (future):** VEP offline for transcripts where ClinVar has no p. or
  consequence, and for **NMD escape**. NMD needs exon junctions + CDS end, so
  it depends on the RefSeq exon work below.

## 3. Per-transcript exon coordinates — build `refseq_exon` (option A)

Current state: Ensembl exons are queryable (`ENST... >>transcript>>exon`), and
RefSeq entries now expose `ensembl_transcript`, so human RefSeq accessions with
an Ensembl mapping can reach exon coordinates indirectly. RefSeq itself only
stores `exon_count`; it does not store ranges.

Source: human GRCh38 RefSeq GFF3

```
https://ftp.ncbi.nlm.nih.gov/genomes/all/GCF/000/001/405/GCF_000001405.40_GRCh38.p14/GCF_000001405.40_GRCh38.p14_genomic.gff.gz
```

Exon lines carry `Parent=rna-NM_...`, `ID=exon-NM_...-N`, start/end, strand.
`CDS` lines carry the same `Parent`.

Design (option A, chosen): new dataset **`refseq_exon` (ID 830, main)**.

- One entry per exon: key `NM_001374258:exonN` (base accession).
- Attributes (new proto message `RefSeqExonAttr`): transcript accession with
  version, exon id/number, chromosome, start, end, strand, coding flag,
  CDS start/end (or overlap).
- Xref `refseq_exon → refseq` so `NM_... >>refseq>>refseq_exon` returns the
  ordered exons.
- Parser streams the GFF; no full RefSeq GBFF rebuild needed.
- This also unlocks NMD-escape computation (last exon–exon junction + CDS end).

## 4. Accession versions — done

`refseq.version` stores the full versioned accession and is exposed in lite
rows; the entry key remains unversioned. The stored version is the current one
for the build. No separate “current” flag is needed for the display case.

## Acceptance mapping

| Request | Status |
|---|---|
| MANE Plus discoverable, distinguishable | done (lite fields + filter) |
| per-transcript `hgvs_c`/`hgvs_p`/consequence/MANE role | Stage A implemented (ClinVar XML); VEP future |
| exon coordinates per transcript | `refseq_exon` implemented (exons; CDS/NMD pending) |
| versions + current identifiable | done (version field) |

## Build order

1. ~~`refseq_exon` parser/dataset + proto; ClinVar `hgvs_transcripts` parser/proto.~~
2. Rebuild `refseq_exon` (GFF) and `clinvar` (XML), then main generate.
3. VEP/NMD remains a separate feature.

Note: the pending AlphaMissense/GOA/CTD generate requires a binary that knows
the `alphamissense_aa` attribute; do not generate with v2.20.3.
