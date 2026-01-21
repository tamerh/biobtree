# PharmGKB (Pharmacogenomics Knowledge Base) Dataset

## Overview
PharmGKB is a comprehensive pharmacogenomics knowledge resource that captures clinically actionable gene-drug associations, dosing guidelines, and drug labels. It integrates data from CPIC (Clinical Pharmacogenetics Implementation Consortium), DPWG (Dutch Pharmacogenetics Working Group), CPNDS (Canadian Pharmacogenomics Network), and RNPGx (French Network of Pharmacogenomics) guidelines.

**Source**: https://www.clinpgx.org/
**Data Type**: Drug-gene associations, clinical annotations, dosing guidelines, pharmacogenomic variants, pathways

## Datasets (6 Total)

| Dataset ID | Name | Description | Key Attributes |
|------------|------|-------------|----------------|
| 110 | pharmgkb | Drug/chemical entries | drug_labels, related_genes, dosing_guideline_sources |
| 111 | pharmgkb_gene | Pharmacogenes | is_vip, has_cpic_guideline, genomic coordinates |
| 112 | pharmgkb_clinical | Clinical variant annotations | level_of_evidence, phenotypes, chemicals |
| 113 | pharmgkb_variant | Variant annotations | synonyms (HGVS), summary enrichment |
| 114 | pharmgkb_guideline | Dosing guidelines (CPIC/DPWG/CPNDS/RNPGx) | source, has_dosing_info, is_pediatric |
| 115 | pharmgkb_pathway | Pharmacokinetic/Pharmacodynamic pathways | is_pharmacokinetic, is_pharmacodynamic, image_link |

## Integration Architecture

### Storage Model
**Primary Entries**: PharmGKB Accession ID (e.g., "PA451906" for warfarin)
**Searchable Text Links**: Drug name, generic names, trade names, gene symbols, rsIDs
**Cross-References**: PubChem, HGNC, Entrez, Ensembl, dbSNP, MeSH

### Files Processed
1. **chemicals.zip**: Drug/chemical vocabulary with cross-references (~5,000 entries)
2. **genes.zip**: Pharmacogenes with VIP flags and genomic coordinates (~25,000 entries)
3. **clinicalVariants.zip**: Clinical variant-drug annotations with evidence levels (~5,000 entries)
4. **variants.zip**: Variant details with HGVS synonyms (~20,000 entries)
5. **guidelineAnnotations.json.zip**: CPIC/DPWG/CPNDS/RNPGx dosing guidelines (~500 entries)
6. **pathways.json.zip**: Pharmacokinetic and pharmacodynamic pathways (~150 entries)
7. **relationships.zip**: Gene-drug relationships with evidence types (~127,000 relationships)
8. **drugLabels.zip**: FDA/EMA/HCSC drug label annotations (~3,000 entries)
9. **phenotypes.zip**: Phenotype vocabulary with MeSH mappings (used for cross-reference enrichment)
10. **summaryAnnotations.zip**: Variant summary scores (used for variant enrichment)

### Special Features
- **Evidence levels**: Clinical annotations use levels 1A, 1B, 2A, 2B, 3, 4 (1A = strongest evidence)
- **VIP genes**: Very Important Pharmacogenes highlighted for clinical relevance
- **Dosing guidelines**: CPIC, DPWG, CPNDS, RNPGx recommendations with source tracking
- **FDA labels**: Tracks which drugs have FDA pharmacogenomic labeling
- **Phenotype cross-refs**: Phenotype names mapped to MeSH IDs for clinical/variant entries
- **Summary enrichment**: Variants enriched with level_of_evidence, score, phenotype_categories

## Use Cases

**1. Drug-Gene Interactions**
```
Query: Find genes affecting warfarin metabolism
warfarin >> pharmgkb >> hgnc → Related genes (CYP2C9, VKORC1, CYP4F2)
Use: Identify pharmacogenes for dosing decisions
```

**2. Clinical Variant Lookup**
```
Query: Find clinical significance of CYP2C9 variants
rs1799853 >> pharmgkb_clinical → Clinical annotations with evidence level
Use: Guide drug selection and dosing
```

**3. Dosing Guidelines**
```
Query: Find CPIC guidelines for warfarin
warfarin >> pharmgkb >> pharmgkb_guideline
Filter: pharmgkb_guideline.source == 'CPIC'
Use: Identify drugs with actionable pharmacogenomic recommendations
```

**4. Pathway Analysis**
```
Query: Find warfarin metabolism pathway
warfarin >> pharmgkb >> pharmgkb_pathway
Filter: pharmgkb_pathway.is_pharmacokinetic == true
Use: Understand drug metabolism and targets
```

**5. FDA Label Information**
```
Query: Find drugs with FDA pharmacogenomic testing
Filter: pharmgkb.top_fda_label_level != ""
Use: Regulatory compliance and labeling decisions
```

**6. Cross-Database Integration**
```
Query: Link pharmacogenomic data to protein structures
drug >> pharmgkb >> hgnc >> uniprot >> alphafold
Use: Structural pharmacogenomics analysis
```

## Test Cases

**Total Tests**: 35+ tests covering all 6 datasets

### pharmgkb (Chemical/Drug) Tests (7 tests)
- Drug name lookup
- Generic names presence
- Related genes enrichment
- Drug labels enrichment (FDA, HCSC sources)
- Dosing guideline sources (CPIC, DPWG, CPNDS, RNPGx)
- Mapping to PubChem
- Mapping to HGNC

### pharmgkb_gene Tests (5 tests)
- Gene lookup by symbol (CYP2C9)
- VIP (Very Important Pharmacogene) flag
- CPIC guideline flag
- Genomic coordinates (GRCh38)
- Cross-references (HGNC, Entrez, Ensembl)

### pharmgkb_clinical Tests (4 tests)
- Lookup by rsID
- Evidence level presence
- MeSH cross-references from phenotype mapping
- Gene mapping via HGNC

### pharmgkb_variant Tests (4 tests)
- Lookup by rsID
- Summary annotation enrichment (level_of_evidence, score, phenotype_categories)
- HGVS synonyms
- dbSNP cross-reference

### pharmgkb_guideline Tests (5 tests)
- Drug to guideline mapping
- CPIC guideline presence
- DPWG guideline presence
- Guideline attributes (source, has_dosing_info, summary)
- Gene cross-references

### pharmgkb_pathway Tests (5 tests)
- Drug to pathway mapping
- Pharmacokinetic pathway (is_pharmacokinetic)
- Pharmacodynamic pathway (is_pharmacodynamic)
- Pathway attributes (image_link, biopax_link)
- Gene cross-references

### Cross-Dataset Integration Tests (3 tests)
- Gene to guideline mapping (CYP2C9 >> hgnc >> pharmgkb_guideline)
- Gene to clinical mapping (CYP2C9 >> hgnc >> pharmgkb_clinical)
- Gene to variant mapping (CYP2C9 >> hgnc >> pharmgkb_variant)

## Performance

- **Test Build**: ~10-20s (test mode processes all data due to small file sizes)
- **Data Source**: ZIP files from clinpgx.org/downloads
- **Update Frequency**: Monthly
- **Total Entries**: ~5,000 chemicals, ~25,000 genes, ~5,000 clinical annotations, ~500 guidelines, ~150 pathways

## Data Model

### PharmgkbAttr (Chemical/Drug Entry - ID 110)
- `pharmgkb_id`: PharmGKB Accession ID
- `name`: Preferred drug name
- `type`: Entry type (Drug, Metabolite, etc.)
- `generic_names`: Generic drug names
- `trade_names`: Brand names
- `smiles`, `inchi`: Chemical structure
- `rxnorm_ids`, `atc_codes`, `pubchem_cids`: Cross-references
- `clinical_annotation_count`, `variant_annotation_count`, `pathway_count`: Counts
- `top_clinical_level`: Best evidence level (1A, 1B, 2A, 2B, 3, 4)
- `top_fda_label_level`, `top_any_label_level`: FDA label testing levels
- `has_dosing_guideline`, `has_prescribing_info`: Guideline flags
- `dosing_guideline_sources`: Array of sources (CPIC, DPWG, CPNDS, RNPGx)
- `related_genes`: Array of PharmgkbRelatedGene (gene_symbol, relationship_type, evidence_type)
- `drug_labels`: Array of PharmgkbDrugLabel (source, testing_level, genes, variants)

### PharmgkbGeneAttr (Pharmacogene Entry - ID 111)
- `pharmgkb_id`: PharmGKB Gene ID
- `symbol`: Gene symbol (e.g., CYP2C9)
- `name`: Full gene name
- `alternate_names`, `alternate_symbols`: Aliases
- `is_vip`: Very Important Pharmacogene flag
- `has_variant_annotation`, `has_cpic_guideline`: Annotation flags
- `chromosome`, `start_grch37/38`, `end_grch37/38`: Genomic coordinates
- `hgnc_id`, `entrez_id`, `ensembl_id`: Cross-references

### PharmgkbClinicalAttr (Clinical Annotation - ID 112)
- `variant`: Variant identifier (rsID or star allele)
- `gene`, `gene_symbol`: Associated gene
- `type`: Annotation type (Metabolism/PK, Toxicity, Efficacy, Dosage)
- `level_of_evidence`: Evidence level (1A-4)
- `chemicals`: Associated drugs
- `phenotypes`: Related phenotypes

### PharmgkbVariantAttr (Variant Entry - ID 113)
- `variant_id`: PharmGKB Variant ID
- `variant_name`: rsID or star allele
- `gene_ids`, `gene_symbols`: Associated genes
- `location`: Genomic location
- `synonyms`: HGVS nomenclature and other aliases
- `variant_annotation_count`, `clinical_annotation_count`: Counts
- **Summary Enrichment Fields**:
  - `level_of_evidence`: Best evidence level
  - `score`: Numeric score
  - `phenotype_categories`: Categories (Metabolism/PK, Toxicity, Efficacy)
  - `associated_drugs`: Drugs in annotations
  - `associated_phenotypes`: Phenotypes in annotations
  - `pmid_count`: Number of supporting publications

### PharmgkbGuidelineAttr (Guideline Entry - ID 114)
- `guideline_id`: PharmGKB Guideline ID
- `name`: Guideline title
- `source`: Source organization (CPIC, DPWG, CPNDS, RNPGx)
- `gene_symbols`, `gene_ids`: Associated genes
- `chemical_names`, `chemical_ids`: Associated drugs
- `has_dosing_info`, `has_testing_info`, `has_recommendation`: Content flags
- `alternate_drug_available`: Alternative drug recommendation
- `is_pediatric`, `is_cancer_genome`: Special flags
- `summary`: Guideline summary text
- `pmids`: Supporting publications

### PharmgkbPathwayAttr (Pathway Entry - ID 115)
- `pathway_id`: PharmGKB Pathway ID
- `name`: Pathway name
- `is_pharmacokinetic`, `is_pharmacodynamic`: Pathway type
- `is_pediatric`: Pediatric relevance
- `gene_symbols`, `gene_ids`: Genes in pathway
- `chemical_names`, `chemical_ids`: Chemicals in pathway
- `disease_names`, `disease_ids`: Associated diseases
- `summary`, `description`: Pathway text
- `biopax_link`: BioPAX OWL file link
- `image_link`: Pathway diagram image URL

## Maintenance

- **Release Schedule**: Monthly updates from PharmGKB
- **Data Format**: TSV files in ZIP archives, JSON for guidelines/pathways
- **Test Data**: Full processing in test mode (small dataset)
- **License**: Creative Commons (CC BY-SA 4.0)

## References

- **Citation**: Whirl-Carrillo M, et al. (2025) PharmGKB: A worldwide resource for pharmacogenomics information. Nucleic Acids Res.
- **Website**: https://www.clinpgx.org/
- **License**: CC BY-SA 4.0
