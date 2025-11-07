# Model Organisms Dataset

## Overview

This document describes the model organisms subset of Biobtree, which provides comprehensive biological data for 16 commonly studied organisms. This dataset is optimized for research applications requiring high-quality, well-annotated data across multiple species.

## Dataset Selection

The 16 model organisms are based on [AlphaFold's model organism proteomes](https://alphafold.ebi.ac.uk/download#proteomes-section), representing key species across major biological domains:

### Organism List

| Organism | Common Name | Domain | Ensembl Tax ID | STRING Tax ID |
|----------|-------------|--------|----------------|---------------|
| *Homo sapiens* | Human | Eukaryota | 9606 | 9606 |
| *Mus musculus* | Mouse | Eukaryota | 10090 | 10090 |
| *Rattus norvegicus* | Rat | Eukaryota | 10116 | 10116 |
| *Danio rerio* | Zebrafish | Eukaryota | 7955 | 7955 |
| *Drosophila melanogaster* | Fruit fly | Eukaryota | 7227 | 7227 |
| *Caenorhabditis elegans* | Nematode | Eukaryota | 6239 | 6239 |
| *Saccharomyces cerevisiae* S288C | Budding yeast | Eukaryota | 559292 | 4932 ⚠️ |
| *Schizosaccharomyces pombe* 972h- | Fission yeast | Eukaryota | 284812 | 284812 |
| *Escherichia coli* K-12 MG1655 | E. coli | Bacteria | 511145 | 511145 |
| *Arabidopsis thaliana* | Thale cress | Eukaryota | 3702 | 3702 |
| *Oryza sativa* | Rice | Eukaryota | 39947 | 39947 |
| *Zea mays* | Maize | Eukaryota | 4577 | 4577 |
| *Glycine max* | Soybean | Eukaryota | 3847 | 3847 |
| *Dictyostelium discoideum* | Slime mold | Eukaryota | 44689 | 44689 |
| *Candida albicans* | Candida | Eukaryota | 237561 | 237561 |
| *Methanocaldococcus jannaschii* | Methanogen | Archaea | 243232 | 243232 |

⚠️ **Important Note**: *Saccharomyces cerevisiae* uses different taxonomy IDs in Ensembl vs STRING databases:
- **Ensembl**: Uses strain-specific ID 559292 (S. cerevisiae S288C)
- **STRING**: Uses species-level ID 4932 (S. cerevisiae species)

This difference reflects that Ensembl hosts multiple yeast strain genomes, while STRING aggregates data at the species level.

## Dataset Coverage

The model organisms dataset includes comprehensive biological information from multiple integrated databases:

### Core Biological Data

**Proteins & Genes:**
- **UniProt**: Comprehensive protein sequences and functional annotations
- **Ensembl**: Genome assemblies, gene models, and genomic features for all 16 organisms
- **HGNC**: Human gene nomenclature (Homo sapiens only)

**Protein Structure:**
- **AlphaFold**: AI-predicted protein structures for all organisms
- Coverage: 550,122 structure predictions across the 16 species

**Ontologies & Classifications:**
- **GO (Gene Ontology)**: Functional annotations for biological processes, molecular functions, and cellular components
- **ECO (Evidence & Conclusion Ontology)**: Evidence codes for annotations
- **InterPro**: Protein family classifications and domain annotations

**Small Molecules & Metabolites:**
- **ChEBI**: Chemical entities of biological interest
- **HMDB**: Human metabolome database (Homo sapiens relevant)
- **ChEMBL**: Bioactive molecules and drug-like compounds

**Disease & Phenotype:**
- **EFO (Experimental Factor Ontology)**: Experimental factors and disease classifications
- **Mondo**: Disease ontology with cross-references
- **HPO (Human Phenotype Ontology)**: Human phenotype terms (Homo sapiens relevant)

**Pathways & Interactions:**
- **Reactome**: Curated biological pathways (23,157 pathways)
- **STRING**: Protein-protein interaction networks
  - Coverage: 197,394 proteins with 11,863,908 interactions across 16 organisms
  - Includes physical and functional associations

**Biomedical Resources:**
- **RNACentral**: Non-coding RNA sequences (40,712,941 sequences)
- **Clinical Trials**: Clinical trial information (547,532 trials)
- **Patent**: Patent data for biological innovations (665,100 records)

**Taxonomy:**
- **NCBI Taxonomy**: Taxonomic classifications and relationships for all organisms

## Processing Approach

The model organisms dataset is generated using a two-phase processing pipeline optimized for cluster environments:

### Phase 1: UPDATE (Parallel Processing)

Downloads and processes data from all source databases. Split into three parallel jobs:

1. **Core Part 1** (8 datasets):
   - uniprot, go, eco, hgnc, taxonomy, interpro, hmdb, chembl

2. **Core Part 2** (9 datasets, with taxonomy filtering):
   - efo, mondo, hpo, alphafold, rnacentral, reactome, clinical_trials, patent, string
   - STRING data filtered to only include the 16 model organisms

3. **Ensembl** (filtered genomes):
   - Ensembl genome assemblies filtered to only include the 16 model organisms

**Resource Requirements:**
- CPU: 8 cores per job (CPU-intensive)
- Memory: 32GB per job
- Runtime: Several hours to days depending on dataset

### Phase 2: GENERATE (Sequential Processing)

Consolidates all index files from Phase 1 and generates the final Biobtree database.

**Resource Requirements:**
- CPU: 3-4 cores (not CPU-intensive, mostly I/O)
- Memory: 64GB+ (RAM-intensive)
- Runtime: Several hours
- Execution: Runs locally with nohup (not submitted to cluster)

## Usage

### Running the Pipeline

```bash
# Submit all UPDATE jobs (core1 + core2 + ensembl)
./scripts/data/model_organisms_sge.sh /path/to/output_dir

# Or run individual jobs for testing/recovery
./scripts/data/model_organisms_sge.sh /path/to/output_dir --core1-only
./scripts/data/model_organisms_sge.sh /path/to/output_dir --core2-only
./scripts/data/model_organisms_sge.sh /path/to/output_dir --ensembl-only

# After all UPDATE jobs complete, run GENERATE phase
./scripts/data/model_organisms_sge.sh /path/to/output_dir --generate-only
```

### Monitoring Progress

```bash
# Check job status
qstat -u $(whoami)

# Monitor individual job logs
tail -f logs/core_part1.log
tail -f logs/core_part2.log
tail -f logs/ensembl_model.log

# Monitor GENERATE phase
tail -f logs/generate_model.log
```

### Starting Web Services

After GENERATE completes:

```bash
# Start Biobtree web service
nohup ./biobtree --out-dir /path/to/output_dir web > logs/web_model.log 2>&1 &

# Access at http://localhost:9292
```

## Database Size

**Estimated Storage Requirements:**

- UPDATE phase output: ~500GB (index files in subdirectories)
- GENERATE phase output: ~200GB (final database)
- Total peak usage: ~700GB (both index files and database during generation)

**Space Optimization:**

After successful database generation, you can delete index files:
```bash
rm -rf /path/to/output_dir/index
```

This reduces storage to ~200GB for the final database only.

## Data Quality & Versioning

- All source databases are downloaded from official repositories
- Ensembl: Latest release version
- STRING: v12.0
- AlphaFold: Latest structures from EBI
- Data freshness depends on when UPDATE phase is run

## Use Cases

This model organisms dataset is ideal for:

1. **Comparative genomics** across multiple species
2. **Protein structure analysis** with AlphaFold predictions
3. **Pathway analysis** across organisms using Reactome
4. **Protein interaction networks** from STRING
5. **Cross-species orthology** studies
6. **Drug target identification** across model organisms
7. **Multi-omics integration** with comprehensive annotations

## Citation

If you use this model organisms dataset, please cite:

- Biobtree: [Citation details]
- AlphaFold: Jumper et al., Nature (2021)
- STRING: Szklarczyk et al., Nucleic Acids Research (2023)
- Ensembl: Cunningham et al., Nucleic Acids Research (2022)
- And individual database citations as appropriate

## Support

For issues or questions:
- GitHub: https://github.com/tamerh/biobtree
- Documentation: See README.md in the biobtreev2 directory
