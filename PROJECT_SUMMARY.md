# BiobtreeV2 - Project Summary

## Quick Overview

**BiobtreeV2** is a bioinformatics identifier mapping tool integrated as a submodule of the BioYoda AI-powered biomedical search system. It provides unified access to 20+ major biological databases through a graph-based identifier mapping approach.

**Purpose in BioYoda:** Make the RAG system "identifier-aware" by enabling seamless translation between different biological identifier systems (genes, proteins, compounds, diseases).

---

## Project Context

### Parent Project: BioYoda
- Location: `/data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/`
- Type: RAG (Retrieval-Augmented Generation) system
- Components: PubMed abstracts, Clinical Trials, Qdrant vector DB, FastAPI
- Goal: AI-powered biomedical literature search and analysis

### BiobtreeV2 Submodule
- Location: `/data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/external/biobtreev2/`
- Type: Fork of public biobtree project
- Language: Go (>=1.20 required)
- Build System: Go modules, custom scripts
- Status: **Actively being modernized and integrated**

---

## Current State (October 2025)

### ✅ Completed Work

1. **FTP to HTTPS Migration** (Critical Fix)
   - Fixed data source access for HGNC, ChEMBL, HMDB, Ensembl
   - All downloads now work with modern HTTPS-only repositories
   - Backward compatible with FTP

2. **Data Format Updates**
   - ChEMBL v36: Fixed float parsing for `highestDevelopmentPhase`
   - HMDB: Added annotation stripper for numeric fields
   - Pattern matching for versioned filenames

3. **Ensembl Metadata**
   - Enabled automatic version checking
   - Regenerating paths for Ensembl v115 (from v53)

4. **Cluster Integration**
   - Created SGE (qsub) versions of build scripts
   - Target queue: `scc`
   - Runtime: 7 days, Memory: 16GB, CPUs: 8

### 📋 Key Files & Locations

**Configuration:**
- `conf/source.dataset.json` - Dataset definitions and URLs
- `conf/application.param.json` - Application settings
- `config/tamer_biobtree.yaml` - Conda environment (created)

**Core Update Code:**
- `update/commons.go` - Common download/FTP utilities (HTTPS support added)
- `update/hgnc.go` - HGNC dataset handler
- `update/chembl.go` - ChEMBL dataset handler (pattern matching added)
- `update/hmdb.go` - HMDB dataset handler (annotation parsing added)
- `update/ensembl.go` - Ensembl dataset handler
- `update/ensembl_meta.go` - Ensembl metadata management

**Cluster Scripts:**
- `scripts/data/all_sge.sh` - SGE cluster submission script (NEW)
- `scripts/data/common_sge.sh` - SGE helper functions (NEW)
- `scripts/data/all.sh` - Original LSF/bsub version
- `scripts/data/common.sh` - Original helper functions

**Documentation:**
- `CHANGES.md` - Detailed changelog of all modifications
- `PROJECT_SUMMARY.md` - This file
- `biobtree_integration.md` - 5-phase integration plan with BioYoda

---

## Datasets Supported

### Currently Tested & Working
✅ HGNC (genes)
✅ ChEMBL (chemical compounds)
✅ HMDB (metabolites)
✅ Ensembl (genomic data)
✅ Taxonomy

### Standard Datasets (Expected to Work)
- UniProt (proteins)
- GO (Gene Ontology)
- ECO, Chebi, Interpro
- EFO (disease ontology)
- Literature mappings
- UniRef50/90/100, UniParc

### Large Datasets (Untested)
- Ensembl Bacteria (largest)
- UniProt Unreviewed
- UniParc

---

## Build & Test Commands

### Environment Setup
```bash
# Create conda environment
conda env create -f config/tamer_biobtree.yaml
conda activate biobtree

# Build biobtree
cd external/biobtreev2
go build
```

### Testing Individual Datasets
```bash
# Test specific dataset
./biobtree -d "hgnc" build
./biobtree -d "chembl_molecule" build
./biobtree -d "hmdb" build
./biobtree -d "ensembl" --tax 9606 build  # Human only

# Test multiple datasets
./biobtree -d "uniprot,hgnc,taxonomy,go" build
```

### Full Build (Cluster)
```bash
# SGE cluster submission
./scripts/data/all_sge.sh scc /path/to/output/dir
```

---

## Integration Plan with BioYoda

Detailed in `biobtree_integration.md` - 5 phases:

### Phase 1: Foundation (Weeks 1-2)
- Build biobtree with core datasets
- Set up API access
- Create basic identifier mapping service

### Phase 2: Identifier Extraction (Weeks 3-4)
- Extract identifiers from PubMed abstracts
- Extract identifiers from clinical trial data
- Store mappings in database

### Phase 3: RAG Enhancement (Weeks 5-6)
- Enhance search with identifier expansion
- Add cross-dataset linking
- Integrate with existing Qdrant collections

### Phase 4: Use Cases (Weeks 7-8)
- Gene-centric literature search
- Drug discovery RAG queries
- Cross-species analysis

### Phase 5: Production (Weeks 9-10)
- Performance optimization
- Monitoring and logging
- Documentation and deployment

---

## Known Issues & Limitations

### ⚠️ Current Issues
1. **Ensembl Metadata Generation**: Uses FTP directory listings (may need HTTPS alternative)
2. **Large Memory Requirements**: Generate phase needs substantial RAM
3. **Long Build Times**: Full dataset processing takes several days
4. **Debug Logs**: Temporary debug output in commons.go (can be removed)

### 🔄 Pending Tasks
- [ ] Test remaining datasets (UniProt, GO, etc.)
- [ ] Verify full build on SGE cluster
- [ ] Remove debug logging from production code
- [ ] Test Ensembl metadata regeneration end-to-end
- [ ] Benchmark performance with BioYoda integration

---

## Development Environment

### Conda Environment: `biobtree`
```yaml
name: biobtree
dependencies:
  - go >=1.20
  - make
  - gcc_linux-64
  - gxx_linux-64
  - git
  - nodejs >=18
  - curl
  - jq
```

### SGE Cluster Settings
- Queue: `scc`
- CPUs: 8 per job
- Memory: 16GB per job
- Runtime: 7 days (604800 seconds)
- Parallel execution with sequential Ensembl processing

---

## Quick Reference for Next Session

### Starting a Development Session
```bash
# 1. Navigate to project
cd /data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/external/biobtreev2

# 2. Activate environment
conda activate biobtree

# 3. Check status
git status
./biobtree --help

# 4. Test a small dataset
./biobtree -d "taxonomy" build
```

### Common Development Tasks

**Adding a new dataset fix:**
1. Check `conf/source.dataset.json` for dataset configuration
2. Find handler in `update/<dataset>.go`
3. Test with `./biobtree -d "<dataset>" build`
4. Update `CHANGES.md` with modifications

**Testing HTTPS changes:**
1. Check `update/commons.go` for HTTPS logic
2. Add debug logging if needed
3. Rebuild: `go build`
4. Test dataset download

**Modifying cluster scripts:**
1. Edit `scripts/data/all_sge.sh` for job parameters
2. Edit `scripts/data/common_sge.sh` for helper functions
3. Test with dry run before full submission

---

## Key Concepts

### Biobtree Architecture
1. **Update Phase**: Download and parse individual datasets → create indexes
2. **Generate Phase**: Merge all indexes → create unified LMDB database
3. **Query Phase**: Serve API/web interface for identifier mapping

### Identifier Mapping Strategy
- Graph-based approach: treats identifiers as nodes, relationships as edges
- Supports transitive mapping (e.g., Gene → Protein → Compound)
- Bidirectional mapping (forward and reverse lookups)

### Data Storage
- LMDB (Lightning Memory-Mapped Database) for fast key-value access
- Index files: temporary, can be deleted after generate phase
- DB folder: final database, needs backup

---

## Next Steps (Suggested)

### Immediate Priority
1. Test full dataset build on SGE cluster
2. Complete Phase 1 of integration plan
3. Set up biobtree API service

### Short-term Goals
1. Extract identifiers from existing BioYoda data
2. Create mapping service endpoint
3. Integrate with RAG search flow

### Long-term Vision
1. Real-time identifier expansion in search queries
2. Cross-species literature correlation
3. Drug-target-disease knowledge graph queries
4. Automated identifier updates with data refreshes

---

## Resources

### Documentation
- Original biobtree: https://github.com/tamerh/biobtree
- Ensembl API: https://rest.ensembl.org
- ChEMBL RDF: https://ftp.ebi.ac.uk/pub/databases/chembl/ChEMBL-RDF/

### Logs & Debugging
- Build logs: `*.log` files in current directory
- Cluster logs: Written by SGE to job-specific files
- Test with single datasets before full runs

### Contact & Collaboration
- Project owner: Tamer (original biobtree author)
- Integration context: BioYoda RAG system
- Development machine: SCC cluster with SGE scheduler

---

## Version History

- **v2.0-bioyoda-dev** (October 2025): FTP→HTTPS migration, SGE support, BioYoda integration
- **v1.x**: Original public biobtree release

---

*Last Updated: October 14, 2025*
*Next Review: After Phase 1 completion*
