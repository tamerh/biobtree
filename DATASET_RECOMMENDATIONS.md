# BiobtreeV2 Dataset Recommendations (2025)

This document provides a comprehensive analysis of potential datasets for integration into BiobtreeV2, based on recent bioinformatics literature (2024-2025 NAR Database Issues), biological knowledge graph research, and current biobtree capabilities.

**Key Considerations:**
- ✅ Open access/free academic use ONLY (biobtree will be commercial)
- 🔗 Strong cross-references to existing biobtree datasets
- 📊 Manageable data size and update frequency
- 💡 High scientific value and query potential

---

## Currently Integrated Datasets (2025)

### Core Datasets
| Dataset | Type | Status | Size | Description |
|---------|------|--------|------|-------------|
| **Ensembl** | Genomics | ✅ Integrated | Large | Genomes, genes, transcripts across species |
| **UniProt** | Proteins | ✅ Integrated | Large | Protein sequences, annotations, features |
| **Uniparc/Uniref** | Proteins | ✅ Integrated | Large | Protein sequence archives and clusters |
| **HGNC** | Gene Nomenclature | ✅ Integrated | Small | Human gene symbols and nomenclature |
| **ChEMBL** | Chemistry | ✅ Integrated | Large | Bioactivity data, drug-like molecules |
| **HMDB** | Metabolomics | ✅ Integrated | Medium | Human metabolome compounds |
| **SureChEMBL** | Patents | ✅ Integrated | Very Large | 43M+ patents, 30M+ compounds |
| **ClinicalTrials.gov** | Clinical | ✅ Integrated | Medium | Trial metadata, interventions, conditions |
| **Reactome** | Pathways | ✅ Integrated | Medium | 23K+ curated pathways, 16 species |
| **STRING** | Interactions | ✅ Integrated | Large | Protein-protein interactions, 24M+ proteins |
| **Mondo** | Disease Ontology | ✅ Integrated | Small | Unified disease ontology |
| **HPO** | Phenotype Ontology | ✅ Integrated | Small | Human phenotypes, gene-phenotype associations |
| **GO** | Ontology | ✅ Integrated | Medium | Gene Ontology terms |
| **EFO** | Ontology | ✅ Integrated | Medium | Experimental Factor Ontology |
| **ECO** | Ontology | ✅ Integrated | Small | Evidence & Conclusion Ontology |
| **Taxonomy** | Taxonomy | ✅ Integrated | Medium | NCBI Taxonomy |
| **InterPro** | Protein Families | ✅ Integrated | Medium | Protein domains and families |

---

## High Priority Datasets (⭐⭐⭐)

### 1. HPO (Human Phenotype Ontology) - ✅ INTEGRATED (2025)

**Status:** ✅ **Integrated and tested** (16,000+ phenotypes, gene-phenotype associations, hierarchical relationships)

See **Currently Integrated Datasets** section above for details.

---

### 2. AlphaFold Protein Structure Database

**Priority:** ⭐⭐⭐⭐

| Attribute | Details |
|-----------|---------|
| **License** | ✅ Open Access (CC-BY-4.0) - commercial use OK |
| **Size** | Very Large (214M+ structures) |
| **Update Frequency** | Continuous |
| **API** | REST API + FTP |
| **Download** | Per-species downloads available |

**Data Content:**
- 214M+ predicted protein structures (2024)
- Covers nearly entire UniProt
- Confidence scores (pLDDT)
- 3D coordinates
- PAE (predicted aligned error)

**Cross-References:**
- ✅ UniProt IDs → Direct mapping to biobtree
- ✅ Ensembl IDs → Available

**Value Proposition:**
- Structural context for ALL proteins
- Enables structure-based drug design
- Complements sequence data with 3D information
- AI-predicted but highly accurate

**Implementation Considerations:**
- **Selective Integration:** Don't download all 214M structures
- **Strategy 1:** Link to AlphaFold IDs only, fetch on demand
- **Strategy 2:** Download structures for organisms in biobtree only
- **Strategy 3:** Download high-confidence structures only (pLDDT > 70)

**Example Queries:**
```bash
# Gene → protein → structure
biobtree query "HGNC:EGFR >> uniprot >> alphafold"

# Patent compound → targets → structures
biobtree query "US-patent >> surechembl >> chembl >> uniprot >> alphafold"

# Check structure availability
biobtree query "P00533 >> alphafold" # Returns structure ID and confidence
```

**Implementation Effort:** Medium (need storage strategy for structures)

---

### 3. BioGRID - Protein Interactions (Complement to STRING)

**Priority:** ⭐⭐⭐

| Attribute | Details |
|-----------|---------|
| **License** | ✅ Open Access (MIT License) |
| **Size** | ~500MB compressed |
| **Update Frequency** | Monthly |
| **API** | REST API available |
| **Download** | Multiple formats (TSV, PSI-MI XML) |

**Data Content:**
- 2.9M+ protein and genetic interactions
- 87,393 publications curated
- Experimental evidence codes
- Physical vs genetic interactions
- Post-translational modifications

**Cross-References:**
- ✅ UniProt IDs
- ✅ Ensembl IDs
- ✅ HGNC symbols
- PubMed IDs

**Value Proposition:**
- **Complements STRING:** BioGRID = experimental only, STRING = computational + experimental
- More detailed interaction types
- Experimental evidence annotations
- PTM data
- Curated from literature

**Note:** Significant overlap with STRING, but provides orthogonal validation and additional experimental detail.

**Example Queries:**
```bash
# Compare STRING vs BioGRID interactions
biobtree query "P53 >> string"
biobtree query "P53 >> biogrid"

# Find experimentally validated interactions
biobtree query "HGNC:BRCA1 >> uniprot >> biogrid[biogrid.experimental==true]"
```

**Implementation Effort:** Low-Medium (similar structure to STRING)

---

### 4. RNAcentral - Non-coding RNA Database

**Priority:** ⭐⭐⭐

| Attribute | Details |
|-----------|---------|
| **License** | ✅ Open Access (CC-BY-4.0) |
| **Size** | ~2GB compressed |
| **Update Frequency** | Quarterly |
| **API** | REST API available |
| **Download** | FASTA, JSON, RDF |

**Data Content:**
- 18M+ ncRNA sequences (2024)
- 44 RNA databases integrated
- Secondary structure (13M+ sequences)
- Wide range of organisms
- RNA types: miRNA, lncRNA, rRNA, tRNA, etc.

**Cross-References:**
- ✅ Ensembl gene IDs
- ✅ UniProt (for RNA-binding proteins)
- PubMed IDs
- GO terms
- RefSeq

**Value Proposition:**
- Biobtree currently lacks ncRNA data
- Critical for gene regulation studies
- miRNA-target interactions
- lncRNA functional annotations
- Completes the "central dogma" (DNA→RNA→Protein)

**Example Queries:**
```bash
# Find miRNAs for a gene
biobtree query "HGNC:EGFR >> ensembl >> rnacentral[rnacentral.type=='miRNA']"

# Regulatory RNA → pathways
biobtree query "miR-21 >> rnacentral >> ensembl >> uniprot >> reactome"

# Disease → genes → regulatory RNAs
biobtree query "disease:cancer >> disgenet >> ensembl >> rnacentral"
```

**Implementation Effort:** Medium (RNA-specific data types, secondary structures)

---

### 5. Bgee - Gene Expression Database

**Priority:** ⭐⭐⭐

| Attribute | Details |
|-----------|---------|
| **License** | ✅ Open Access (CC0 - public domain) |
| **Size** | ~10GB (processed data) |
| **Update Frequency** | Annual |
| **API** | REST API + R package |
| **Download** | TSV files per species |

**Data Content (2024 Update):**
- Bulk RNA-seq: 14,000+ libraries
- Single-cell RNA-seq: hundreds of curated 10X datasets
- Multiple species (emphasis on vertebrates)
- Tissue/developmental stage annotations
- Curated, standardized, processed

**Cross-References:**
- ✅ Ensembl gene IDs
- ✅ UniProt IDs
- Anatomy ontologies (Uberon)

**Value Proposition:**
- Tissue-specific expression context
- Enables "where is this gene expressed?" queries
- Drug side-effect prediction (off-target tissues)
- Developmental biology insights
- Single-cell data integration (2024 feature)

**Example Queries:**
```bash
# Gene expression in specific tissue
biobtree query "HGNC:EGFR >> bgee:lung"

# Drug target → expression profile
biobtree query "drugbank:DB00945 >> uniprot >> bgee"

# Find tissue-specific disease genes
biobtree query "disease:cancer >> disgenet >> ensembl >> bgee:breast"
```

**Implementation Effort:** Medium-High (quantitative data, tissue annotations)

---

## Medium Priority Datasets (⭐⭐)

### Summary Table

| Dataset | Type | License | Size | Value | Effort |
|---------|------|---------|------|-------|--------|
| **PubChem** | Chemistry | ✅ Public Domain | Very Large (100GB+) | Comprehensive compound space | High |
| **ClinVar** | Variants | ✅ Public Domain | ~5GB | Clinical variant interpretation | Medium |
| **dbSNP** | Variants | ✅ Public Domain | Large | Genetic variants | Medium |
| **PDB** | Structures | ✅ Public Domain | Large | Experimental 3D structures | Medium |
| **GTEx** | Expression | ✅ Open Access | ~10GB | Human tissue expression | High |
| **OMIM** | Disease | ⚠️ Registration required | ~100MB | Genetic disorders | Low |
| **miRBase** | RNA | ✅ Public Domain | Small (~10MB) | miRNA sequences & annotations | Low |
| **PeptideAtlas** | Proteomics | ✅ Open Access | Medium | Observed peptides/proteins | Medium |
| **DisGeNET** | Disease | ❌ CC BY-NC-SA | ~500MB | Gene-disease associations | Low (no commercial) |

### 6. PubChem - Comprehensive Chemical Database

**License:** ✅ Public Domain
**Priority:** ⭐⭐ (due to size)

**Pros:**
- 111M+ compounds (vs ChEMBL's ~2M)
- 1.5M+ bioassays
- Patent references
- Free, comprehensive

**Cons:**
- **Massive size** (~100GB+ raw)
- Significant overlap with ChEMBL
- Lower quality/curation than ChEMBL

**Recommendation:**
- Skip full integration initially
- Consider selective import (compounds with bioactivity data only)
- Or use as backup/lookup service via API

---

### 7. ClinVar - Clinical Variant Database

**License:** ✅ Public Domain
**Priority:** ⭐⭐

**Data:**
- 2.5M+ variants
- Clinical significance (pathogenic, benign, etc.)
- Disease associations
- Expert-curated

**Value:**
- Essential for clinical genomics
- Variant interpretation
- Precision medicine

**Cross-refs:** Ensembl, HGNC, OMIM, dbSNP

**Recommendation:** Good complement to DisGeNET for variant-level data

---

### 8. PDB - Protein Data Bank (Experimental Structures)

**License:** ✅ Public Domain
**Priority:** ⭐⭐

**Data:**
- 210,000+ experimental structures
- X-ray, NMR, cryo-EM
- Protein-ligand complexes

**Value:**
- Complements AlphaFold (experimental vs predicted)
- Gold standard structures
- Ligand binding sites

**Note:** AlphaFold covers more proteins, but PDB has experimental validation

---

### 9. OMIM - Online Mendelian Inheritance in Man

**License:** ⚠️ **Registration Required** (free academic, restricted commercial)
**Priority:** ⭐⭐

**Data:**
- 25,000+ genetic disorder entries
- Gene-phenotype relationships
- Clinical descriptions

**⚠️ Commercial Use Restriction:**
- Requires license for commercial applications
- Biobtree commercial use = need license or exclude dataset

**Recommendation:**
- **Do NOT integrate** if biobtree will be commercial without OMIM license
- HPO is a good alternative (includes data from OMIM but with open license)

---

## Low Priority / Specialized Datasets (⭐)

### Summary Table

| Dataset | Type | Why Low Priority |
|---------|------|------------------|
| **KEGG** | Pathways | ❌ **Commercial license required** |
| **DrugBank** | Drugs | ❌ **Commercial license required** (~$5k+/year) |
| **MetaboLights** | Metabolomics | Specialized, HMDB already integrated |
| **PRIDE** | Proteomics | Raw data repository, specialized |
| **GEO** | Expression | Raw data repository, Bgee provides processed |
| **PeptideAtlas** | Proteomics | Specialized, medium value |

### Commercial Datasets (For Reference Only)

⚠️ **Cannot be integrated into commercial biobtree without licensing:**

| Dataset | License Cost | Notes |
|---------|-------------|-------|
| **KEGG** | ~$2,000+/year | Pathways (Reactome is free alternative) |
| **DrugBank** | ~$5,000+/year | Approved drugs, targets |
| **DisGeNET** | License required | Gene-disease associations (HPO is free alternative) |
| **OMIM** | License required | Genetic disorders (HPO alternative) |
| **HGMD** | License required | Disease mutations |
| **MetaBase** | Commercial | Metabolic pathways |

---

## Knowledge Graph & AI-Focused Resources (2024-2025)

Recent biomedical knowledge graph research highlights these resources:

### 10. Petagraph (2024)

**License:** ✅ Open Access
**Priority:** ⭐ (specialized)

**Data:**
- 32M+ nodes, 118M+ relationships
- Integrates 180+ ontologies
- Quantitative genomics data

**Note:** More of a framework/schema than standalone dataset. Potential for schema alignment.

---

### 11. BioKG/iKraph (2024)

**License:** ✅ Academic access (https://biokde.insilicom.com)
**Priority:** ⭐ (specialized)

**Data:**
- Extracted from PubMed abstracts
- 40 public databases integrated
- High-throughput genomics inferences

**Note:** Focus on literature-mined relationships. Potential overlap with existing data.

---

## Implementation Recommendations

### Phase 1: Phenotype/Disease & Structure Context (6-8 months)
**Goal:** Add phenotype/disease associations and structural biology

1. **HPO (Human Phenotype Ontology)** (Priority 1) - 2 months
   - Gene-phenotype associations
   - Immediate high value
   - Simple integration (standard OBO format)
   - **Commercial use OK**

2. **AlphaFold** (Priority 2) - 3 months
   - Link to structure IDs (don't download all structures)
   - Implement on-demand structure fetching
   - Focus on human proteome initially

3. **BioGRID** (Priority 3) - 2 months
   - Complement STRING with experimental interactions
   - Add PTM data

**Expected Impact:**
- Phenotype/disease-focused queries enabled
- Clinical genomics applications
- Structure-based drug design queries
- Enhanced interaction networks

---

### Phase 2: RNA & Expression (4-6 months)
**Goal:** Add gene regulation and expression context

4. **RNAcentral** (Priority 4) - 3 months
   - ncRNA integration
   - miRNA-target interactions
   - Fill gap in RNA biology

5. **Bgee** (Priority 5) - 3 months
   - Tissue expression
   - Developmental stages
   - Single-cell data

**Expected Impact:**
- Complete central dogma coverage
- Tissue-specific queries
- Gene regulation insights

---

### Phase 3: Clinical Genomics (Optional, 4-6 months)

6. **ClinVar** - 2 months
   - Variant interpretation
   - Clinical significance

7. **miRBase** - 1 month
   - miRNA complement to RNAcentral
   - Small, focused dataset

8. **PDB** (selective) - 2 months
   - Experimental structures for key proteins
   - Link to structures, don't download all

---

## Dataset Selection Decision Matrix

### Must-Have Criteria (All Required)
- ✅ Open access/free for commercial use
- ✅ API or bulk download available
- ✅ Regular updates maintained
- ✅ Strong cross-references to existing biobtree data
- ✅ Manageable size or selective download possible

### High-Value Indicators (2+ Required)
- 🔗 Fills gap in current biobtree coverage
- 🔗 Enables new query types
- 🔗 High citation/usage in literature
- 🔗 Complements existing datasets
- 🔗 Low implementation effort

### Red Flags (Any Disqualifies)
- ❌ Commercial license required
- ❌ Restrictive redistribution terms
- ❌ No clear update schedule
- ❌ Poor/no documentation
- ❌ Proprietary data format

---

## Summary: Top 5 Recommendations

| Rank | Dataset | Effort | Impact | Timeline | Commercial OK? |
|------|---------|--------|--------|----------|----------------|
| 1 | **HPO** | Low | ⭐⭐⭐⭐⭐ | 2 months | ✅ YES (CC BY 4.0) |
| 2 | **AlphaFold DB** | Medium | ⭐⭐⭐⭐⭐ | 3 months | ✅ YES (CC BY 4.0) |
| 3 | **BioGRID** | Low | ⭐⭐⭐⭐ | 2 months | ✅ YES (MIT License) |
| 4 | **RNAcentral** | Medium | ⭐⭐⭐⭐ | 3 months | ✅ YES (CC BY 4.0) |
| 5 | **Bgee** | Medium-High | ⭐⭐⭐⭐ | 3 months | ✅ YES (CC0) |

**Total Effort:** ~13 months for all 5
**Recommended Approach:** Phase 1 (HPO + AlphaFold + BioGRID) first

✅ **All top 5 recommendations are commercially compatible!**

---

## Killer Query Examples (With New Datasets)

### Drug Discovery & Target Validation
```bash
# Complete drug discovery pipeline with phenotypes
"phenotype:cognitive_impairment >> hpo >> uniprot >> alphafold >> chembl >> clinical_trials"

# Patent landscape with structure & phenotype
"US-patent >> surechembl >> chembl >> uniprot >> alphafold >> hpo"

# Find druggable targets with structural data
"phenotype:tumor >> hpo >> uniprot >> alphafold[alphafold.pLDDT>80] >> chembl"
```

### Systems Biology
```bash
# Gene → Structure → Interactions → Pathways → Phenotype
"HGNC:EGFR >> uniprot >> alphafold >> string >> reactome >> hpo"

# miRNA regulation → protein → phenotype
"miR-21 >> rnacentral >> ensembl >> uniprot >> hpo"

# Tissue-specific phenotype networks
"phenotype:diabetes >> hpo >> ensembl >> bgee:pancreas >> string"
```

### Clinical Genomics
```bash
# Variant → Clinical significance → Gene → Phenotype → Drugs
"rs123456 >> clinvar >> ensembl >> hpo >> uniprot >> chembl"

# Gene expression + phenotype + drugs
"HGNC:TP53 >> bgee:lung >> hpo >> uniprot >> chembl >> clinical_trials"
```

### Comparative Interactomics
```bash
# Compare experimental vs predicted interactions
"P53 >> biogrid[biogrid.experimental==true]"
"P53 >> string[string.score>900]"

# Structurally characterized interactions
"HGNC:BRCA1 >> uniprot >> biogrid >> alphafold"
```

---

## Data Size Summary

### Full Integration (All 5 Top Datasets)
- **Compressed:** ~15-20GB
- **Uncompressed:** ~60-80GB
- **With AlphaFold structures (selective):** +20-50GB

### Storage Strategy Recommendations
1. **Metadata-only for large datasets** (AlphaFold, PubChem)
2. **Link to external APIs** for on-demand data
3. **Selective organism downloads** (human, model organisms)
4. **Compressed storage** for bulk data

---

## Update Frequency Considerations

| Dataset | Release Cycle | Download Time | Processing Time |
|---------|---------------|---------------|-----------------|
| HPO | Monthly | ~15 min | ~30 min |
| AlphaFold | Continuous | N/A (link only) | Minutes |
| BioGRID | Monthly | ~30 min | ~1 hour |
| RNAcentral | Quarterly | ~2 hours | ~4 hours |
| Bgee | Annual | ~3 hours | ~6 hours |

**Recommendation:** Quarterly full rebuilds, monthly incremental updates for BioGRID

---

## References

### Database Issues
- 2025 NAR Database Issue: https://academic.oup.com/nar/issue/53/D1
- 2024 NAR Database Issue: https://academic.oup.com/nar/issue/52/D1

### Knowledge Graph Research (2024)
- Petagraph (2024): Scientific Data, https://www.nature.com/articles/s41597-024-04070-w
- BioKG (2024): Nature Machine Intelligence, https://www.nature.com/articles/s42256-025-01014-w
- TarKG (2024): Bioinformatics, https://academic.oup.com/bioinformatics/article/40/10/btae598/7818343

### Dataset URLs
- **HPO**: https://hpo.jax.org/
- **AlphaFold DB**: https://alphafold.ebi.ac.uk/
- **BioGRID**: https://thebiogrid.org/
- **RNAcentral**: https://rnacentral.org/
- **Bgee**: https://www.bgee.org/
- **ClinVar**: https://www.ncbi.nlm.nih.gov/clinvar/
- **PubChem**: https://pubchem.ncbi.nlm.nih.gov/
- **PDB**: https://www.rcsb.org/
- **ClinGen**: https://www.clinicalgenome.org/
- **DisGeNET** (commercial): https://www.disgenet.org/

---

## Change Log

- **2025-01-06:** Complete rewrite with 2024-2025 research
  - Updated with Reactome ✅, STRING ✅, Clinical Trials ✅, Patents ✅ as integrated
  - **CORRECTED:** Replaced DisGeNET (NC license) with HPO as top priority
  - HPO, AlphaFold DB, RNAcentral, Bgee, BioGRID as top priorities
  - All top 5 recommendations verified for commercial compatibility ✅
  - Simplified format with tables for easier scanning
  - Added commercial license warnings (KEGG, DrugBank, DisGeNET, OMIM)
  - Included 2024 knowledge graph research
  - Added implementation phases and timelines

- **Previous:** Original recommendations (STRING, Reactome, Clinical Trials now integrated)
