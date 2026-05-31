# CIViC Dataset

## Overview
CIViC (Clinical Interpretation of Variants in Cancer) provides expert-curated clinical interpretations of somatic cancer variants — genes, variants, clinical evidence, and assertions.

**Source**: https://civicdb.org (nightly TSV releases)
**Data Type**: Somatic cancer variant clinical interpretations
**License**: CC0 1.0 (public domain)
**Dataset IDs**: 752 (civic), 753 (civic_variant), 754 (civic_evidence), 755 (civic_assertion)

## Integration Architecture

CIViC is a parent dataset (`civic`, genes/features) with three children:

| Dataset | Key | Content |
|---|---|---|
| `civic` | feature_id | Gene/feature summaries (HGNC symbol, Entrez, description) |
| `civic_variant` | variant_id | Variants (HGVS, ClinVar, allele registry, SO types) |
| `civic_evidence` | evidence_id | Clinical evidence (disease, therapy, level, significance) |
| `civic_assertion` | assertion_id | Curated assertions (AMP category, ACMG, NCCN, FDA) |

### Key edges
- `civic` → `hgnc` / `entrez` / `ensembl` (gene hub)
- `civic_variant` → `civic` (parent gene), `clinvar`
- `civic_evidence` / `civic_assertion` → `civic_variant`, `civic`, `hgnc` (one-hop disease→gene), `doid`, `chembl_molecule`, `pubmed`, `clinical_trials`
- `doid` ↔ `mondo` (from mondo.obo `xref: DOID:` lines), so `mondo/efo >> civic_evidence >> hgnc` and disease-druggability chains resolve.

## Use Cases
- **Cancer variant interpretation**: `>>hgnc>>civic>>civic_variant>>civic_evidence` → disease/therapy/evidence level
- **Disease → druggable drivers**: `>>mondo>>civic_evidence>>chembl_molecule`
- **Somatic clinical evidence** complementing germline sources (GenCC/ClinVar/OMIM/Orphanet)

## Known Limitations
- **Scope = Accepted & Submitted** (`evidence_status` is a filterable attribute).
- **Molecular profiles are folded**, not a standalone dataset; edges fan out to every gene/variant in the profile.
- **Therapies name-matched to `chembl_molecule`** (bulk TSVs carry names only); unmatched names kept as attributes/text only.
- **Phenotypes stored as names only** (no `civic_evidence → hpo` edge); VCF/VariantGroup files not ingested.

## Maintenance
- **Update Frequency**: nightly at source
- **Data Format**: TSV
- **License**: CC0 1.0

## References
- **Website**: https://civicdb.org
- **Citation**: Griffith M, et al. (2017) CIViC. Nat Genet.
