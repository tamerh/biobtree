 If you want to add more datasets later, the biggest opportunities are:
  1. OMIM - 10,038 xrefs (genetic disorders)
  2. DOID - 11,866 xrefs (Disease Ontology)
  3. GARD - 10,730 xrefs (rare diseases)
  4. MESH - 8,378 xrefs (medical terminology)
  5. NCIT - 7,550 xrefs (NCI Thesaurus)
  

# BiobtreeV2 Dataset Expansion Recommendations

Based on research of current bioinformatics databases (2025 NAR Database Issue) and analysis of BiobtreeV2's existing capabilities, here are recommendations for high-value datasets to integrate.

## Current BiobtreeV2 Datasets

**Genomics & Proteomics:**
- Ensembl (genomes, transcripts, genes)
- UniProt (proteins)
- Uniparc, Uniref
- HGNC (human gene nomenclature)

**Chemistry & Drug Discovery:**
- ChEMBL (bioactivity data)
- HMDB (human metabolome)
- SureChEMBL (patents & compounds)

**Ontologies & Taxonomy:**
- GO (Gene Ontology)
- EFO (Experimental Factor Ontology)
- ECO (Evidence & Conclusion Ontology)
- NCBI Taxonomy

---

## Top Priority Datasets (⭐⭐⭐)

### 1. STRING - Protein-Protein Interactions

**Why Add:**
- Already have proteins (UniProt), genes (HGNC, Ensembl) - adding interaction networks enables pathway/network analysis
- Critical for systems biology approaches

**Data:**
- 24M+ proteins across 3000+ organisms
- Functional, physical, and regulatory networks (now separated)
- Confidence scores for interactions
- Gene set enrichment analysis capabilities

**API/Access:**
- REST API: https://string-db.org/
- Bulk downloads available
- Well-documented

**Cross-References:**
- UniProt IDs
- Ensembl gene IDs
- RefSeq

**Example Queries:**
```bash
# Find all EGFR interaction partners
biobtree query "HGNC:EGFR >> uniprot >> string >> uniprot"

# Patent compounds → targets → interaction networks
biobtree query "US-20110053848-A1 >> surechembl >> chembl >> uniprot >> string"

# Gene → orthologs → conserved interactions
biobtree query "BRCA1 >> ensembl >> ortholog >> string"
```

**Implementation Effort:** Medium
- TSV/XML format downloads
- ~5-10GB compressed data
- Clear identifier mappings

---

### 2. Reactome - Pathway Database

**Why Add:**
- Complements GO ontology with detailed biological pathways
- Curated, high-quality pathway annotations
- Excellent cross-references to existing datasets

**Data:**
- 2,500+ curated pathways (human-focused)
- Reactions, complexes, interactions
- Disease pathways
- Drug/compound pathways

**API/Access:**
- REST API: https://reactome.org/
- GraphQL endpoint available
- RDF/OWL downloads
- Regular releases

**Cross-References:**
- UniProt
- ChEMBL
- Ensembl
- GO terms
- Disease ontologies

**Example Queries:**
```bash
# Compound → targets → pathways
biobtree query "CHEMBL203 >> chembl >> uniprot >> reactome"

# Patent → compounds → pathways → diseases
biobtree query "US-patent-123 >> surechembl >> chembl >> uniprot >> reactome >> disease"

# Gene → pathways → related genes
biobtree query "HGNC:EGFR >> reactome >> uniprot >> hgnc"
```

**Implementation Effort:** Medium
- Well-structured data model
- ~2-3GB data
- Clear documentation

---

### 3. ClinicalTrials.gov - Clinical Trial Registry

**Why Add:**
- **Perfect complement to patent data!**
- Links compounds/drugs to clinical outcomes
- Tracks drug development pipeline
- Essential for translational research

**Data:**
- 480,000+ clinical trials worldwide
- Interventions (drugs, biologics, devices)
- Diseases/conditions
- Trial status, phases, outcomes
- Sponsor information

**API/Access:**
- REST API: https://clinicaltrials.gov/api/
- XML/JSON bulk downloads
- Updated daily
- ~500MB compressed

**Cross-References:**
- Drug names (can link to ChEMBL, DrugBank)
- NCT IDs (unique identifiers)
- Disease terms
- Gene/protein names in trial descriptions

**Example Queries:**
```bash
# Find trials for aspirin
biobtree query "CHEMBL203 >> clinical_trials"

# Patent → compounds → trials
biobtree query "US-20110053848-A1 >> surechembl >> chembl >> clinical_trials"

# Gene → drugs → trials
biobtree query "HGNC:EGFR >> uniprot >> chembl >> clinical_trials"

# Competitor analysis: assignee → patents → compounds → trials
biobtree query "assignee:AstraZeneca >> patent >> surechembl >> clinical_trials"
```

**Implementation Effort:** Medium-High
- Large dataset but well-structured
- Need fuzzy matching for drug names
- Regular updates recommended

---

### 4. DrugBank - Approved Drugs Database

**Why Add:**
- FDA/EMA approved drugs with extensive annotations
- Natural bridge between ChEMBL and clinical use
- Small molecule and biologic drugs
- Drug-drug interactions

**Data:**
- 15,000+ drugs (2,800+ FDA approved)
- Drug targets (proteins)
- Pharmacology data
- Chemical structures (SMILES, InChI)
- Drug-drug interactions
- Metabolism pathways

**API/Access:**
- Requires registration (free for academic)
- XML/JSON/CSV downloads
- REST API available
- ~200MB compressed

**Cross-References:**
- ChEMBL IDs
- UniProt IDs
- PubChem CIDs
- Patent numbers
- Clinical trial IDs
- KEGG, Reactome pathways

**Example Queries:**
```bash
# Map patented compounds to approved drugs
biobtree query "US-patent >> surechembl >> chembl >> drugbank"

# Find all EGFR inhibitors (approved drugs)
biobtree query "HGNC:EGFR >> uniprot >> drugbank"

# Drug → targets → pathways
biobtree query "drugbank:DB00945 >> uniprot >> reactome"

# Drug-drug interaction check
biobtree query "drugbank:DB00945 >> drug_interaction >> drugbank"
```

**Implementation Effort:** Low-Medium
- Clean, well-structured XML
- Clear identifier mappings
- ~200MB data

---

### 5. DisGeNET - Gene-Disease Associations

**Why Add:**
- Connects genes/proteins/variants to diseases
- Integrates data from GWAS, animal models, literature
- Essential for disease-focused queries

**Data:**
- 1,134,942 gene-disease associations (GDAs)
- 369,554 variant-disease associations (VDAs)
- 30,170 genes
- 30,000+ diseases/phenotypes
- Evidence scores and sources

**API/Access:**
- REST API: https://www.disgenet.org/
- TSV downloads
- Cytoscape plugin available
- ~500MB compressed

**Cross-References:**
- HGNC gene symbols
- UniProt IDs
- Ensembl IDs
- Disease ontologies (UMLS, MeSH, OMIM, etc.)
- dbSNP (variants)

**Example Queries:**
```bash
# Gene to diseases
biobtree query "HGNC:BRCA1 >> disgenet >> disease"

# Patent → compound → target → disease
biobtree query "US-patent >> surechembl >> chembl >> uniprot >> disgenet"

# Disease → genes → drugs
biobtree query "disease:cancer >> disgenet >> uniprot >> chembl >> drugbank"
```

**Implementation Effort:** Low
- Simple TSV format
- Straightforward mappings
- Moderate size

---

## Secondary Priority Datasets (⭐⭐)

### 6. PubChem - Comprehensive Chemical Database

**Why Add:**
- Massive compound database (100M+ compounds)
- Complements ChEMBL/HMDB with broader chemical space
- Bioassay data

**Data:**
- 111M+ compounds
- 1.5M+ bioassays
- Chemical structures, properties
- Patent references

**Considerations:**
- **Very large dataset** (~100GB+ raw data)
- May want to integrate selectively (e.g., only compounds with bioactivity)
- Strong overlap with ChEMBL

**Implementation Effort:** High (due to size)

---

### 7. GTEx - Genotype-Tissue Expression

**Why Add:**
- Links genes to tissue-specific expression
- Enables tissue-context queries
- Useful for drug side-effect prediction

**Data:**
- RNA-seq from 54 human tissues
- 17,382 samples from 948 donors
- Gene expression levels

**API/Access:**
- Portal: https://gtexportal.org/
- BigQuery dataset
- ~10GB compressed

**Example Queries:**
```bash
# Gene expression in specific tissue
biobtree query "HGNC:EGFR >> gtex:lung"

# Drug target expression profile
biobtree query "drugbank:DB00945 >> uniprot >> gtex"
```

**Implementation Effort:** Medium-High
- Large dataset
- Expression values (quantitative data)

---

### 8. PDB - Protein Data Bank (3D Structures)

**Why Add:**
- 3D structures for structure-based drug design
- Protein-ligand complexes
- Complements UniProt sequence data

**Data:**
- 200,000+ structures
- X-ray, NMR, cryo-EM structures

**Implementation Effort:** Medium
- Well-structured format (PDBx/mmCIF)
- Large file sizes for structures

---

### 9. IntAct / BioGRID - Molecular Interactions

**Why Add:**
- Complements STRING with curated experimental data
- More detailed interaction types

**Data:**
- Protein-protein interactions
- Experimental evidence codes

**Note:** Significant overlap with STRING

---

### 10. OMIM - Online Mendelian Inheritance in Man

**Why Add:**
- Human genetic disorders
- Gene-phenotype relationships
- Clinical descriptions

**Data:**
- 25,000+ entries
- Gene-disease relationships

**Implementation Effort:** Low
- API available
- Requires license for commercial use

---

## Implementation Strategy Recommendations

### Phase 1: Core Network & Clinical Data (High ROI)
1. **DrugBank** - Easiest to implement, immediate value
2. **ClinicalTrials.gov** - Perfect complement to patents
3. **STRING** - Adds network dimension

**Estimated Timeline:** 2-3 months
**Data Size:** ~3-5GB total

### Phase 2: Disease & Pathway Context
4. **DisGeNET** - Gene-disease links
5. **Reactome** - Pathway annotations

**Estimated Timeline:** 2 months
**Data Size:** ~3GB

### Phase 3: Specialized/Advanced (Optional)
6. **GTEx** - Tissue expression
7. **PubChem** (selective) - Extended chemical space
8. **PDB** - 3D structures

---

## Killer Query Examples with New Datasets

### Drug Discovery Pipeline
```bash
# Patent → Compound → Target → Disease → Clinical Trial
"US-patent >> surechembl >> chembl >> uniprot >> disgenet >> clinical_trials"

# Competitor drug pipeline
"assignee:Pfizer >> patent >> surechembl >> chembl >> drugbank >> clinical_trials"
```

### Target Validation
```bash
# Gene → Disease → Drugs → Clinical Evidence
"HGNC:EGFR >> disgenet >> drugbank >> clinical_trials"

# Gene → Protein Interactions → Pathways
"BRCA1 >> uniprot >> string >> reactome"
```

### Chemical Biology
```bash
# Compound → Targets → Interactions → Pathways → Diseases
"CHEMBL203 >> chembl >> uniprot >> string >> reactome >> disgenet"
```

### Patent Landscape Analysis
```bash
# Technology area → Patents → Compounds → Approved Drugs
"ipc:A61K31 >> patent >> surechembl >> chembl >> drugbank"

# Patent family → Clinical development
"family:12345 >> patent >> surechembl >> chembl >> clinical_trials"
```

---

## Data Integration Priorities

**Best Overall ROI:**
1. **DrugBank** - Small, clean, high value
2. **STRING** - Network context for all proteins
3. **ClinicalTrials.gov** - Natural extension of patent data

**Best for Drug Discovery:**
1. ClinicalTrials.gov
2. DrugBank
3. DisGeNET
4. Reactome

**Best for Systems Biology:**
1. STRING
2. Reactome
3. GTEx (if tissue context needed)

---

## Technical Considerations

### Data Size Estimates (Total)
- Phase 1: ~5GB compressed, ~20GB uncompressed
- Phase 2: ~3GB compressed, ~10GB uncompressed
- Phase 3: ~50GB+ (if including PubChem/GTEx)

### Update Frequencies
- STRING: Annual major releases
- Reactome: Quarterly
- ClinicalTrials.gov: Daily (practical: monthly)
- DrugBank: Quarterly
- DisGeNET: Annual

### API Rate Limits
- Most databases: 1-10 requests/second
- Bulk downloads recommended for initial build
- Incremental updates via API

---

## Questions to Consider

1. **Research Focus**: Drug discovery vs. systems biology vs. clinical genomics?
2. **User Base**: Who are the primary users? What queries matter most?
3. **Infrastructure**: Storage/compute capacity for large datasets?
4. **Update Cadence**: How frequently rebuild the database?
5. **Commercial Use**: Any licensing restrictions?

---

## Recommendation Summary

**Start with these 3:**
1. ✅ **DrugBank** - Quick win, immediate value, easy implementation
2. ✅ **ClinicalTrials.gov** - Unique value, perfect complement to patents
3. ✅ **STRING** - Game-changer for network analysis

These three will:
- Extend your patent→drug pipeline to clinical outcomes
- Add network/pathway context to all protein data
- Enable competitive intelligence on drug development
- Require moderate implementation effort (~3 months)
- Add ~5GB total data

**Next priority:** Reactome + DisGeNET for pathway and disease context.

---

## References

- 2025 Nucleic Acids Research Database Issue: https://academic.oup.com/nar/issue/53/D1
- STRING: https://string-db.org/
- Reactome: https://reactome.org/
- ClinicalTrials.gov: https://clinicaltrials.gov/
- DrugBank: https://go.drugbank.com/
- DisGeNET: https://www.disgenet.org/
