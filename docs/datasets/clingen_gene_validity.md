# ClinGen Gene-Disease Validity Dataset

## Overview
ClinGen Gene-Disease Validity provides strength-of-evidence classifications for gene-disease causality, curated by ClinGen Gene Curation Expert Panels (GCEPs). Each entry is a single curated assertion linking a gene to a disease with an evidence tier and mode of inheritance.

**Source**: https://search.clinicalgenome.org/kb/gene-validity/download
**Data Type**: Gene-disease validity assertions (CSV)
**License**: CC0 1.0 (public domain)
**Dataset ID**: 139

## Integration Architecture

### Storage Model
**Primary Entries**: keyed by the ClinGen assertion UUID (extracted from the online-report URL, `...CGGV:assertion_<uuid>-<date>`)
**Searchable Text Links**: gene symbol, disease label
**Attributes Stored**: gene symbol, HGNC id, disease label, MONDO id, mode of inheritance, SOP version, classification, GCEP, classification date, report URL
**Cross-References**: HGNC/Entrez/Ensembl (gene via symbol lookup), MONDO (disease)
**Bucket Method**: `alphanum`

### Classification values
Definitive · Strong · Moderate · Limited · Disputed · Refuted · No Known Disease Relationship · Animal Model Only

## Use Cases

**1. Gene-disease evidence tier**
```
Query: How strongly is BRCA1 linked to its disease? → >>hgnc>>clingen_gene_validity → classification (Definitive..Refuted)
Use: Variant interpretation, panel design
```

**2. Disease → gene (curated)**
```
Query: Which genes are validated for a disease? → >>mondo>>clingen_gene_validity>>hgnc
Use: Diagnostic gene panels
```

**3. Mode of inheritance**
```
Query: AD or AR? → check moi attribute → genetic counseling
```

## Relationship to GenCC
GenCC aggregates ClinGen among other submitters. This dataset is ClinGen's primary curation, kept as a separate, ClinGen-only source.

## Known Limitations
- One row per gene-disease assertion, not per gene.
- When a report URL lacks the `assertion_` marker, a fallback key `<gene_symbol>_<mondo_id>` is synthesized; rows with neither are skipped.

## Maintenance
- **Update Frequency**: nightly at source
- **Data Format**: CSV
- **Test Data**: 100 entries
- **License**: CC0 1.0

## References
- **Website**: https://clinicalgenome.org/curation-activities/gene-disease-validity/
- **Terms**: https://clinicalgenome.org/docs/terms-of-use/ (CC0 1.0)
