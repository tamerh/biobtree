# SaProt Dataset

## Overview

**SaProt** is a **structure-aware protein language-model** variant-effect score.
It is biobtree's owned, **unsupervised** deep predictor — an independent second
opinion to the (weakly-supervised) AlphaMissense: when they agree, high
confidence; when they diverge, a flag worth surfacing.

| Field | Meaning |
|-------|---------|
| `uniprot_id` | UniProt accession / isoform (e.g. `P01116`, `P01116-2`) |
| `protein_variant` | Single-letter WT+pos+mut (e.g. `G12D`) |
| `position` | 1-based residue position |
| `saprot_llr` | SaProt log-likelihood ratio: **≤ 0, more negative = more damaging** |
| `gene_symbol` | Gene symbol |

**Data Type**: per-missense-substitution score (whole proteome)
**Dataset ID**: `812` · **Federation**: `predictions`

## Source & License — computed in-house (export-clean)

- Model: **SaProt-650M** (Su et al., ICLR 2024; `westlake-repl/SaProt`, **MIT**),
  structure-aware via AlphaFold 3Di tokens (Foldseek).
- Scores are **computed by the bioyoda GPU pipeline** from MIT weights (wt-marginal
  LLR), so the output is ours → **KG-export eligible** (unlike the CC-BY-NC ESM1b
  published scores). ~421.7 M variants across ~41,672 UniProt accessions with an
  AlphaFold structure (98.6% of the isoform set; the ~1.4% without a structure fall
  back to AlphaMissense).
- Validated vs the ESM1b reference (Spearman 0.751): correlated enough to confirm
  correctness, independent enough to be a real second opinion.

## Integration Architecture

- **`saprot`** (id 812): one record per substitution, keyed
  **`uniprot:protein_variant`** (e.g. `P01116:G12D`) — protein-level, NOT genomic.
- Joins to a variant via **(uniprot_id, protein_variant)** — the same single-letter
  notation AlphaMissense exposes, so a client constructs the key from an
  AlphaMissense record. Cross-refs to `uniprot`.
- `hasFilter: yes` — e.g. `saprot.saprot_llr < -5.0`.
  `compact_fields`: `saprot_llr,protein_variant`.

## Compute → serve handoff

bioyoda computes the scores and drops a TSV
(`uniprot<TAB>protein_variant<TAB>position<TAB>llr<TAB>gene_symbol`); biobtree
ingests it unchanged (`--saprot.file …`, via `bb.sh` `OPTS_saprot`). The committed
conf `path` is a small test fixture.

## Query Examples

```bash
curl "http://localhost:9292/ws/entry/?i=P01116:G12D&s=saprot"
curl "http://localhost:9292/ws/?i=P01116:G12D&d=1&f=saprot.saprot_llr<-5.0"
```
