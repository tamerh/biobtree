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
| **RNACentral** | Non-coding RNA | ✅ Integrated | Medium | 49.8M+ ncRNA sequences from 56 databases |
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

### 2. AlphaFold Protein Structure Database - 📋 PLANNED (Swiss-Prot Subset)

**Priority:** ⭐⭐⭐⭐

| Attribute | Details |
|-----------|---------|
| **License** | ✅ Open Access (CC-BY-4.0) - commercial use OK |
| **Size** | Swiss-Prot: 550K structures (28.5GB) / Full: 214M+ |
| **Update Frequency** | Monthly (version 6 current) |
| **FTP** | https://ftp.ebi.ac.uk/pub/databases/alphafold/ |
| **Format** | PDB/mmCIF with pLDDT in B-factor column |

**Integration Strategy:**
Start with **Swiss-Prot subset only** (550,122 reviewed proteins), expand later if needed

**Data to Store (Metadata Only):**
```
- AlphaFold ID (AF-Q9Y6K9-F1)
- Global pLDDT score (82.0)
- Fraction very high confidence (0.623)
- Fraction confident (0.136)
- Fraction low (0.055)
- Fraction very low (0.186)
- Model version + date
```

**Data Source:**
- File: `swissprot_pdb_v6.tar` (28.5GB)
- Contains PDB.gz files with pLDDT scores in B-factor column (positions 60-66)
- Stream TAR, extract scores, calculate metrics on-the-fly
- No structure files stored (link to AlphaFold DB instead)

**Cross-References:**
- ✅ UniProt IDs → Direct bidirectional mapping
- Separate "alphafold" dataset (like STRING model)

**Value Proposition:**
- **Confidence-based filtering** - Find proteins with high-quality models
- **Drug discovery** - Prioritize targets with good structures
- **Research gap analysis** - Identify proteins needing experimental structures
- **Query capability** - Filter by structure quality (not just availability)
- **Marketing value** - AlphaFold extremely popular in research community

**Example Queries:**
```bash
# High confidence drug targets
biobtree query "chembl >> target >> uniprot >> alphafold [globalPLDDT > 80]"

# Disease genes without good structures (research gaps)
biobtree query "mondo >> hpo >> hgnc >> uniprot >> alphafold [fractionVeryLow > 0.3]"

# Pathway proteins with structural coverage
biobtree query "reactome:pathway >> uniprot >> alphafold [fractionVeryHigh > 0.7]"

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

### 4. RNAcentral - Non-coding RNA Database - ✅ INTEGRATED (2025)

**Status:** ✅ **Integrated and tested** (49.8M+ ncRNA sequences, RNA type annotations, metadata-only storage)

| Attribute | Details |
|-----------|---------|
| **License** | ✅ Open Access (CC-BY-4.0) |
| **Size** | 8.4GB compressed (active FASTA) |
| **Update Frequency** | Quarterly (Release 25) |
| **API** | REST API available |
| **Download** | FASTA streaming from EMBL-EBI FTP |

**Integrated Data:**
- 49.8M+ unique ncRNA sequences (Release 25)
- 56 expert databases aggregated
- RNA types: rRNA, miRNA, lncRNA, tRNA, snoRNA, etc.
- Metadata-only storage (sequences via API)
- Organism distribution tracking
- Active/obsolete status

**Attributes Stored:**
- RNA type classification
- Sequence length
- Description
- Organism count
- Source databases
- Active status
- MD5 checksum

**Cross-References:**
- ✅ RNACentral ID keyword lookup
- Future: Ensembl, RefSeq, miRBase (via id_mapping.tsv)

**Value Delivered:**
- Fills ncRNA gap in biobtree
- Gene regulation studies
- RNA annotation for genome assemblies
- Completes the "central dogma" (DNA→RNA→Protein)

**Example Queries:**
```bash
# Basic RNA lookup
biobtree query "URS000149A9AF >> rnacentral"

# Find by RNA type (future with filters)
biobtree query "gene >> ensembl >> rnacentral[rnacentral.rna_type=='miRNA']"
```

**Implementation Details:** Streaming FASTA parser, intelligent RNA type detection, 100 test entries with 15 tests (9 declarative + 6 custom)

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

4. **RNAcentral** (Priority 4) - ✅ **COMPLETED** (2025)
   - ✅ ncRNA integration (49.8M sequences)
   - ✅ RNA type classification
   - ✅ Metadata-only storage
   - Future: miRNA-target interactions via id_mapping

5. **Bgee** (Priority 5) - 3 months
   - Tissue expression
   - Developmental stages
   - Single-cell data

**Expected Impact:**
- ✅ Central dogma coverage complete (DNA→RNA→Protein)
- Tissue-specific queries (pending Bgee)
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

| Rank | Dataset | Effort | Impact | Status | Commercial OK? |
|------|---------|--------|--------|--------|----------------|
| 1 | **HPO** | Low | ⭐⭐⭐⭐⭐ | ✅ **Integrated** | ✅ YES (CC BY 4.0) |
| 2 | **AlphaFold DB** | Medium | ⭐⭐⭐⭐⭐ | ✅ **Integrated** | ✅ YES (CC BY 4.0) |
| 3 | **BioGRID** | Low | ⭐⭐⭐⭐ | 📋 Planned | ✅ YES (MIT License) |
| 4 | **RNAcentral** | Medium | ⭐⭐⭐⭐ | ✅ **Integrated** | ✅ YES (CC BY 4.0) |
| 5 | **Bgee** | Medium-High | ⭐⭐⭐⭐ | 📋 Planned | ✅ YES (CC0) |

**Progress:** 3/5 completed (HPO, AlphaFold, RNAcentral)
**Remaining Effort:** ~5 months for BioGRID + Bgee

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

- **2025-11-06:** RNAcentral integration complete
  - ✅ RNAcentral integrated with 49.8M+ ncRNA sequences
  - Streaming FASTA parser with intelligent RNA type detection
  - Metadata-only storage (rna_type, length, description, organism_count, databases, is_active)
  - 15 comprehensive tests (9 declarative + 6 custom)
  - Progress: 3/5 top priorities completed (HPO ✅, AlphaFold ✅, RNAcentral ✅)
  - Completes central dogma coverage: DNA (Ensembl) → RNA (RNACentral) → Protein (UniProt)

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
