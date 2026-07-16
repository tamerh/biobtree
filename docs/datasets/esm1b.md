# ESM1b Dataset

> **STATUS: DEACTIVATED (2026-07-14).** Superseded by [SaProt](saprot.md) — a
> better (0.457 vs 0.41 ProteinGym), structure-aware, unsupervised, and
> **export-clean** protein-LM score that is ~0.75-correlated with ESM1b (so keeping
> both is redundant). The ESM1b index chunk was moved aside
> (`predictions/_deactivated_esm1b/`); the raw data and parser remain, so it can be
> reactivated with `./bb.sh out_prod --only esm1b --force` + a predictions
> regenerate. Final fate deferred.

## Overview

**ESM1b** is a protein language-model variant-effect score (whole human proteome).
It was the first protein-keyed predictor ingested — its integration is the
reusable pattern SaProt now uses.

| Field | Meaning |
|-------|---------|
| `uniprot_id` | UniProt accession / isoform |
| `protein_variant` | Single-letter WT+pos+mut (e.g. `G12D`) |
| `position` | 1-based residue position |
| `esm1b_llr` | ESM1b log-likelihood ratio: **≤ 0, more negative = more damaging** |
| `gene_symbol` | Gene symbol |

**Data Type**: per-missense-substitution score
**Dataset ID**: `811` · **Federation**: `predictions`

## Source & License

- Source: the published whole-proteome ESM1b scores (Brandes et al., Nat Genet
  2023; HuggingFace `ntranoslab/esm_variants`, `ALL_hum_isoforms_ESM1b_LLR.zip`,
  ~462 M substitutions / 42,286 isoforms), melted to a per-variant TSV.
- License: the published release is **CC-BY-NC-4.0** → ingest-only, **KG-export
  excluded**. (A self-computed ESM run from the MIT weights would be export-clean —
  but SaProt already fills that role, hence the deactivation.)

## Integration Architecture

- **`esm1b`** (id 811): keyed **`uniprot:protein_variant`** (same scheme as SaProt),
  joins to a variant via `(uniprot_id, protein_variant)`, cross-refs to `uniprot`.
- `hasFilter: yes`. `compact_fields`: `esm1b_llr,protein_variant`.

## Query Examples (when active)

```bash
curl "http://localhost:9292/ws/entry/?i=P01116:G12D&s=esm1b"
```
