# ClinGen Dosage Sensitivity Dataset

## Overview
ClinGen Dosage Sensitivity provides per-gene haploinsufficiency (HI) and triplosensitivity (TS) dosage scores, curated by the ClinGen Dosage Sensitivity working group. It answers "does losing (or gaining) a copy of this gene cause disease?".

**Source**: https://ftp.clinicalgenome.org/ClinGen_gene_curation_list_GRCh38.tsv
**Data Type**: Per-gene dosage scores (TSV)
**License**: CC0 1.0 (public domain)
**Dataset ID**: 140

## Integration Architecture

### Storage Model
**Primary Entries**: keyed by NCBI/Entrez gene id (e.g. `672` = BRCA1)
**Searchable Text Links**: gene symbol
**Attributes Stored**: gene symbol, gene id, cytoband, genomic location, HI score + label + disease id, TS score + label + disease id, date last evaluated
**Cross-References**: Entrez/HGNC/Ensembl (gene), MONDO/MIM (HI & TS diseases), PubMed
**Bucket Method**: `numeric`

### Score encoding (HI and TS)
`3` sufficient evidence (dosage-sensitive) · `2` emerging · `1` little · `0` no evidence · `30` gene associated with autosomal-recessive phenotype · `40` dosage sensitivity unlikely.

## Use Cases

**1. Haploinsufficiency lookup**
```
Query: Is BRCA1 haploinsufficient? → >>hgnc>>clingen_dosage → haplo_score=3 (sufficient evidence)
Use: CNV / loss-of-function variant interpretation
```

**2. Dosage gene → disease**
```
Query: What disease does losing this gene cause? → >>clingen_dosage>>mondo
```

**3. Triplosensitivity**
```
Query: Does an extra copy matter? → check triplo_score
Use: Duplication CNV interpretation
```

## Known Limitations
- **Curated subset only (~1642 genes).** Absence does NOT mean "not dosage-sensitive" — it means "not yet curated".
- **Region curations excluded** (`ClinGen_region_curation_list`, 513 ISCA recurrent-CNV regions): ISCA region ids form an orphan namespace with no link into the biobtree graph.
- GRCh37 build not used; non-numeric Gene ID rows skipped.

## Maintenance
- **Update Frequency**: nightly at source
- **Data Format**: TSV
- **Test Data**: 100 entries
- **License**: CC0 1.0

## References
- **Website**: https://clinicalgenome.org/curation-activities/dosage-sensitivity/
- **Terms**: https://clinicalgenome.org/docs/terms-of-use/ (CC0 1.0)
