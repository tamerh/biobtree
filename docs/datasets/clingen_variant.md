# ClinGen Variant Pathogenicity Dataset

## Overview
ClinGen Variant Pathogenicity provides expert-panel (VCEP) ACMG variant interpretations from the ClinGen Evidence Repository — ~12.6K variants, each with a clinical assertion and the applied ACMG evidence codes.

**Source**: https://erepo.clinicalgenome.org/evrepo/api/summary/classifications/download
**Data Type**: Variant pathogenicity classifications (TSV)
**License**: CC0 1.0 (public domain)
**Dataset ID**: 141

## Integration Architecture

### Storage Model
**Primary Entries**: keyed by Allele Registry CA id (e.g. `CA281951`)
**Searchable Text Links**: gene symbol, variation name
**Attributes Stored**: variation name, ClinVar Variation Id, Allele Registry id, gene symbol, disease, MONDO id, MOI, assertion, ACMG evidence codes (met / not met), interpretation summary, VCEP, guideline, approval/published dates, Evidence Repo link, UUID
**Cross-References**: **ClinVar** (the bridge), HGNC/Entrez/Ensembl (gene), MONDO (disease), PubMed
**Bucket Method**: `alphanum`

### The ClinVar bridge
The variant joins biobtree's existing **ClinVar hub** via its *ClinVar Variation Id* column. ClinVar already cross-references dbSNP (rs), gene and every disease ontology, so ClinGen variants inherit that whole graph for free:
```
>>clingen_variant>>clinvar>>dbsnp
```

### Assertion values
Pathogenic · Likely Pathogenic · Uncertain Significance · Likely Benign · Benign

## Use Cases

**1. Expert ACMG interpretation**
```
Query: How did a VCEP classify this variant? → lookup CA id → assertion + applied ACMG codes
Use: Clinical variant classification
```

**2. Gene → expert variant calls**
```
Query: Which variants in this gene have VCEP calls? → >>hgnc>>clingen_variant
```

**3. Bridge to ClinVar / dbSNP**
```
Query: >>clingen_variant>>clinvar (then >>dbsnp for rsIDs)
Use: Reconcile expert calls with ClinVar submissions
```

## Known Limitations
- **Key = Allele Registry CA id** (always present). *ClinVar Variation Id* is sometimes blank ("not yet submitted to ClinVar"); when blank the Evidence Repo UUID is used as the key and no ClinVar edge is created for that row.
- No direct dbSNP edge; dbSNP is reached transitively via ClinVar.

## Maintenance
- **Update Frequency**: nightly at source
- **Data Format**: TSV
- **Test Data**: 100 entries
- **License**: CC0 1.0

## References
- **Website**: https://erepo.clinicalgenome.org/
- **Terms**: https://clinicalgenome.org/docs/terms-of-use/ (CC0 1.0)
