a# Biobtree


Biobtree is a bioinformatics tool which allows mapping the bioinformatics datasets
via identifiers and special keywors with simple or advance chain query capability.


## Features

* **Datasets** - supports wide datasets such as `Ensembl` `Uniprot` `ChEMBL` `HMDB` `BindingDB` `CTD` `STRING` `BioGRID` `MSigDB` `Taxonomy` `GO` `EFO` `HPO` `UBERON` `CL` `HGNC` `ECO` `Uniparc` `Uniref` `RNACentral` `Bgee` `GWAS Catalog` `dbSNP` `RefSeq` `IntAct` `GenCC` `AlphaMissense` `ClinVar` `PharmGKB` `CELLxGENE` `SCXA` `Orphanet`  with tens of more via cross references
by retrieving latest data from providers

* **MapReduce** - processes small or large datasets based on users selection and build B+ tree based uniform local database via specialized MapReduce based tecnique with efficient storage usage 

* **Query** - Allow simple or advance chain queries between datasets with intiutive syntax which allows writing RDF or graph like queries

* **Genome** - supports querying full Ensembl genomes coordinates with `transcript`, `CDS`, `exon`, `utr` with several attiributes, mapped datasets and identifiers such as `ortholog` ,`paralog` or probe identifers belongs `Affymetrix` or `Illumina`

* **Protein** - Uniprot proteins including protein features with variations and mapped datasets.

* **Chemistry** - `ChEMBL`, `HMDB`, `ChEBI`, `LIPID MAPS`, and `SwissLipids` datasets supported for chemistry, disease, lipid metabolism, and drug releated analaysis. SwissLipids provides 779K+ lipid structures with protein associations, GO annotations, tissue localization, and evidence codes

* **Binding Affinity** - `BindingDB` database with 2.9M+ measured binding affinities (Ki, IC50, Kd, EC50) between drug-like molecules and protein targets. Provides ligand SMILES, InChI, target names, organism information, experimental conditions (pH, temperature), and literature references. Cross-references to UniProt, PubChem, ChEMBL, and ChEBI for comprehensive drug-target interaction analysis

* **Toxicogenomics** - `CTD` (Comparative Toxicogenomics Database) with 180K+ chemicals, 2.5M+ chemical-gene interactions, and 8.3M+ chemical-disease associations. Provides curated toxicogenomic relationships with organism context, PubMed evidence, inference scores for disease associations, and cross-references to MeSH, Entrez Gene, MONDO, EFO, OMIM, Taxonomy, and PubChem. Supports toxicity profiling, biomarker discovery, and environmental health research

* **Patents** - `SureChEMBL` patent data with 43M+ patents, 30M+ compounds, and patent-compound mappings for drug discovery and IP analysis

* **Clinical Trials** - `ClinicalTrials.gov` data with trial metadata, conditions, interventions, publications, and automatic drug mapping to ChEMBL molecules

* **Genetic Variants** - `ClinVar` database with curated genetic variant-disease relationships, including variant classifications, clinical significance, review status, HGVS expressions, gene annotations, and phenotype associations

* **Pathways** - `Reactome` pathway database with 23K+ curated pathways across 16 species, including protein/gene/compound participants, pathway hierarchy, GO mappings, disease annotations, and evidence codes (TAS/IEA) for curation quality

* **Non-Coding RNAs** - `RNACentral` database with 49.8M+ unique ncRNA sequences aggregated from 56 expert databases, including rRNA, miRNA, lncRNA, tRNA, and other RNA types with comprehensive metadata

* **Gene Expression** - `Bgee` database with curated gene expression data across 30+ species and 1,000+ anatomical structures. Includes tissue-specific expression patterns, expression quality scores, multi-technology support (Affymetrix, RNA-Seq, scRNA-Seq), observation counts, and cross-references to Ensembl genes and UBERON tissues

* **Single-Cell Transcriptomics** - `CELLxGENE` Census from CZ Science with 80M+ cells across 1,800+ datasets. Provides dataset metadata (organism, assay types, cell counts) and aggregated cell type data with tissue distribution and disease associations. Cross-references to Taxonomy, CL (Cell Ontology), UBERON (tissues), EFO (assays), and MONDO (diseases). Supports finding datasets by cell type, exploring cell type tissue distribution, and disease-associated single-cell studies

* **Single-Cell Expression Atlas** - `SCXA` from EMBL-EBI with 380+ single-cell RNA-seq experiments. Three integrated datasets: `scxa` (experiment metadata with technology types, cell counts, ontology annotations), `scxa_expression` (gene-centric summaries with marker counts across experiments), and `scxa_gene_experiment` (cluster-level expression details). Cross-references to Taxonomy, CL, UBERON, and Ensembl marker genes. Supports experiment discovery by cell type/tissue, gene expression profiling across single-cell atlas, and marker gene validation

* **GWAS Genetics** - `GWAS Catalog` from NHGRI-EBI with 1,000,000+ SNP-trait associations and 182,000+ published studies. Includes variant-level data (genomic positions, genes, p-values, effect sizes) and study-level metadata (publications, sample sizes, platforms). Supports variant-trait discovery, gene-based variant lookup, disease genetics exploration, and links to EFO trait ontology. Future enhancement planned for ancestry-based filtering

* **Genetic Variants** - `dbSNP` (database of Single Nucleotide Polymorphisms) from NCBI with RefSNP IDs (rs numbers), genomic coordinates, allele information, population allele frequencies, gene associations, and clinical significance data. Supports variant lookup, gene-to-SNP mapping, allele frequency analysis, and variant type classification (SNV, insertion, deletion)

* **Reference Sequences** - `RefSeq` from NCBI with curated reference sequences for genomes, transcripts, and proteins. Provides genomic coordinates, gene annotations, protein sequences, and cross-references to UniProt, Entrez Gene, and other databases. Use `--genome-taxids` to filter to specific organisms for model organism databases

* **Missense Variant Pathogenicity** - `AlphaMissense` from DeepMind with ~71M missense variant pathogenicity predictions. Provides pathogenicity scores (0-1), classifications (likely_benign/ambiguous/likely_pathogenic), and cross-references to UniProt proteins and Ensembl transcripts. `alphamissense_transcript` dataset provides transcript-level mean pathogenicity scores for mutation tolerance assessment

* **Protein Interactions** - `IntAct` database from EBI with ~1.8 million experimentally validated protein-protein interactions across ~100,000 unique proteins. Provides detailed experimental evidence including detection methods, interaction types, confidence scores, experimental roles, and direct citations to 23,000+ publications. Supports interaction network analysis, drug target discovery, and pathway exploration with PSI-MI standardized terms

* **Protein Networks** - `STRING` database with predicted and known protein-protein interactions across thousands of organisms. Provides combined interaction scores with evidence breakdown (experimental, database, textmining, coexpression), protein annotations, and size information. Enables functional association networks, protein complex analysis, and pathway enrichment studies

* **Genetic Interactions** - `BioGRID` database with 2.8M+ curated protein-protein and genetic interactions from biomedical literature. Provides physical and genetic interaction types, experimental detection methods, publication references, and organism context. Cross-references to Entrez Gene, UniProt, RefSeq, PubMed, and Taxonomy. Supports interaction network analysis, genetic pathway discovery, and protein complex identification

* **Gene Sets** - `MSigDB` (Molecular Signatures Database) with 35K+ annotated gene sets for GSEA analysis. Includes Hallmark (H), positional (C1), curated (C2), regulatory (C3), computational (C4), ontology (C5), oncogenic (C6), immunologic (C7), and cell type (C8) collections. Provides gene symbols, descriptions, PubMed references, GO/HPO term associations, and collection metadata. Cross-references to HGNC gene symbols, Entrez Gene IDs, PubMed, GO, and HPO. Supports gene set enrichment analysis, pathway discovery, and functional annotation

* **Gene-Disease Validity** - `GenCC` (Gene Curation Coalition) database with 35,000+ standardized gene-disease validity curations from multiple authoritative sources including ClinGen, Ambry, Genomics England, and Orphanet. Provides classification levels (Definitive, Strong, Moderate, Limited, Supportive), mode of inheritance (autosomal dominant/recessive, X-linked), submitter information, and PubMed citations. Supports clinical variant interpretation, diagnostic panel design, and gene-disease relationship exploration with cross-references to MONDO, HPO, and PubMed

* **Pharmacogenomics** - `PharmGKB` database with 6 integrated datasets for precision medicine: chemicals/drugs with FDA drug labels, pharmacogenes with VIP flags and CPIC guidelines, clinical variant annotations with evidence levels (1A-4), variant annotations with HGVS nomenclature, dosing guidelines from CPIC/DPWG/CPNDS/RNPGx, and pharmacokinetic/pharmacodynamic pathways. Provides gene-drug relationships, clinical evidence levels, dosing recommendations, and pathway diagrams. Cross-references to HGNC, PubChem, dbSNP, MeSH, and Ensembl. Supports pharmacogenomic dosing decisions, drug-gene interaction analysis, and clinical annotation lookup

* **Rare Diseases** - `Orphanet` database with 11,500+ rare disease entries from Orphadata (CC BY 4.0). Provides disease names, synonyms, definitions, and disorder types. Includes HPO phenotype associations with frequency data (Obligate, Very frequent, Frequent, Occasional, Very rare) and gene associations with Ensembl/HGNC identifiers. Cross-references to OMIM, MONDO, MeSH, HPO, Ensembl, and HGNC. Supports rare disease diagnosis, phenotype-driven gene discovery, and disease-gene network analysis. Embedded phenotype data enables filtering by frequency (e.g., find phenotypes occurring in >80% of patients)

* **Taxonomy & Ontologies** - `Taxonomy` `GO` `EFO` `ECO` `HPO` `MONDO` `UBERON` `CL` `OBA` `PATO` `OBI` `XCO` data with mapping to other datasets and child and parent query capability. CL (Cell Ontology) provides 2,700+ cell type classifications for tissue-specific and cell-specific analysis. OBA (Ontology of Biological Attributes) covers biological traits. PATO (Phenotype And Trait Ontology) describes phenotypic qualities. OBI (Ontology for Biomedical Investigations) covers study designs and assays. XCO (Experimental Conditions Ontology) describes experimental conditions

* **Your data** - Your custom data can be integrated with or without relation to other datasets

**Note**: Detailed documentation for each dataset can be found at `tests/datasets/*/README.md`

* **Web UI** - Web interface for easy explorations and examples

* **Web Services** - REST or gRPC services

h
### Usage

First install [latest](https://github.com/tamerh/biobtree/releases/latest) biobtree executable available for Windows, Mac or Linux. Then extract the downloaded file to a new folder and open a terminal in this new folder directory and starts the biobtree. Alternatively R and Python based [biobtreeR](https://github.com/tamerh/biobtreeR) and [biobtreePy](https://github.com/tamerh/biobtreePy) wrapper packages can be used instead of using the executable directly for eaiser integration.

#### Starting biobtree with target datasets or genomes
```sh

# build ensembl genomes by tax id with uniprot&taxonomy datasets
biobtree  --tax 595,984254 -d "uniprot,taxonomy" build 

# build datasets only
biobtree -d "uniprot,taxonomy,hgnc" build
biobtree -d "hgnc,chembl,hmdb" build

# build with lipid datasets (SwissLipids + LIPID MAPS with protein associations)
biobtree -d "lipidmaps,swisslipids,uniprot,go,eco" build

# build with clinical trials (requires ChEMBL for drug mapping)
biobtree -d "chembl,clinical_trials" build

# build with genetic variants (works well with HGNC, MONDO, HPO)
biobtree -d "hgnc,clinvar,mondo,hpo" build

# build with binding affinity data (works well with UniProt, PubChem, ChEMBL)
biobtree -d "bindingdb,uniprot,pubchem,chembl" build

# build with toxicogenomics data (works well with MeSH, Entrez, MONDO, EFO)
biobtree -d "ctd,mesh,entrez,mondo,efo" build

# build with gene-disease validity (works well with MONDO, HPO)
biobtree -d "gencc,mondo,hpo" build

# build with gene sets for GSEA (works well with HGNC, Entrez, GO, HPO)
biobtree -d "msigdb,hgnc,entrez,go,hpo" build

# build with gene expression (requires Ensembl and works well with UBERON)
biobtree -d "ensembl,bgee,uberon" build

# build with single-cell transcriptomics (works well with CL, UBERON, MONDO, EFO)
biobtree -d "cellxgene,cl,uberon,mondo,efo" build

# build with Single Cell Expression Atlas (works well with CL, UBERON, Taxonomy, Ensembl)
biobtree -d "scxa,scxa_expression,cl,uberon,taxonomy" build

# build with GWAS genetics (works well with EFO, HGNC)
biobtree -d "gwas,gwas_study,efo,hgnc" build

# build with genetic variants (works well with HGNC, ClinVar)
biobtree -d "dbsnp,hgnc" build

# build with missense variant pathogenicity (works well with UniProt, Ensembl)
biobtree -d "alphamissense,alphamissense_transcript,uniprot" build

# build with pharmacogenomics (works well with HGNC, dbSNP, MeSH)
biobtree -d "pharmgkb,hgnc,dbsnp,mesh" build

# build with rare diseases (works well with HPO, MONDO, Ensembl, HGNC)
biobtree -d "orphanet,hpo,mondo,ensembl,hgnc" build

# build with protein interactions (requires UniProt)
biobtree -d "uniprot,intact" build

# build with STRING protein networks (use with Ensembl genomes for best results)
biobtree --tax 9606 -d "string" build

# build with RefSeq reference sequences (filter to specific organisms)
biobtree --genome-taxids 9606 -d "refseq,uniprot,entrez" build  # Human only
biobtree --genome-taxids 9606,10090 -d "refseq" build  # Human + Mouse

# build all ontologies at once (GO, ECO, EFO, UBERON, CL, MONDO, HPO, OBA, PATO, OBI, XCO)
biobtree -d "ontology" build

# build with specific ontologies
biobtree -d "oba,pato,obi,xco" build

# once data is built start web for using ws and ui
biobtree web

# to see all options and datasets use help
biobtree help

```

### Web service endpoints
```ruby
# Meta
# datasets meta informations
localhost:9292/ws/meta

# Search
# i is mandatory, mode defaults to "full"
localhost:9292/ws/?i={terms}&s={dataset}&p={page}&f={filter}&mode={full|lite}

# Mapping
# i and m are mandatory, mode defaults to "full"
localhost:9292/ws/map/?i={terms}&m={mapfilter_query}&p={page}&mode={full|lite}

# Retrieve dataset entry. Both parameters are mandatory
localhost:9292/ws/entry/?i={identifier}&s={dataset}

```

#### API Response Modes

The Web API supports two response modes optimized for different use cases:

**Full Mode (default)** - Complete response with all attributes:
```ruby
# Returns full data with attributes, entries, and enhanced metadata
localhost:9292/ws/?i=P04637&mode=full
localhost:9292/ws/map/?i=P04637&m=>>uniprot>>hgnc&mode=full

# Response includes:
# - results: Full Xref objects with all attributes
# - query: Echo of the original query (terms, dataset_filter, raw)
# - stats: Result statistics (total_results, returned, by_dataset)
# - pagination: Structured pagination info (page, has_next, next_token)
```

**Lite Mode** - Compact response optimized for AI agents and bulk operations:
```ruby
# Returns compact IDs-only format (~50x smaller payload)
localhost:9292/ws/?i=P04637&mode=lite
localhost:9292/ws/map/?i=P04637&m=>>uniprot>>hgnc&mode=lite

# Search response includes:
# - mode: "lite"
# - query: Echo of the original query
# - results: Compact entries with {d: dataset, id: identifier, has_attr: bool, xref_count: int}
# - stats: {total_results, returned, by_dataset}
# - pagination: {page, has_next, next_token}

# Mapping response includes:
# - mode: "lite"
# - query: {terms, chain, raw}
# - mappings: Array with {input, source, targets[], error} per input term
# - stats: {total_terms, mapped, failed, total_targets}
# - pagination: {page, has_next, next_token}
```

**Key Features:**
- **Query Echo**: Both modes return the original query for debugging and logging
- **Statistics**: Success/failure counts, results per dataset
- **Structured Pagination**: `has_next` boolean and `next_token` for easy iteration
- **Error Tracking** (lite mode): Failed terms include `error` field with reason
- **Attribute Flag** (lite mode): `has_attr` indicates if entry has indexed attributes
- **Sorting** (lite mode): Results sorted by `has_attr` (entries with attributes first)

#### Pagination

Both response modes support pagination for handling large result sets. Lite mode returns more results per page due to its smaller payload size.

**Page Size Limits (configurable in `conf/application.param.json`):**
| Mode | Search Results | Mapping Results |
|------|----------------|-----------------|
| Full | 10 per page (`maxSearchResult`) | 75 per page (`maxMappingResult`) |
| Lite | 50 per page (`maxSearchResultLite`) | 150 per page (`maxMappingResultLite`) |

**How Pagination Works:**

1. **First Request** - No pagination parameter needed:
```bash
curl "http://localhost:9292/ws/map/?i=TP53,BRCA1,KCND2&m=>>ensembl>>uniprot&mode=lite"
```

Response includes pagination info:
```json
{
  "pagination": {
    "page": 1,
    "has_next": true,
    "next_token": "2,0,-1,3[]-1"
  }
}
```

2. **Subsequent Pages** - Use `p=` parameter with the `next_token` value:
```bash
curl "http://localhost:9292/ws/map/?i=TP53,BRCA1,KCND2&m=>>ensembl>>uniprot&mode=lite&p=2,0,-1,3[]-1"
```

Response for page 2:
```json
{
  "pagination": {
    "page": 2,
    "has_next": false
  }
}
```

3. **Continue until `has_next` is false** - When `has_next` is `false`, you've retrieved all results.

**Pagination Fields:**
- `page`: Current page number (1-indexed)
- `has_next`: Boolean indicating if more pages exist
- `next_token`: Opaque token to pass as `p=` parameter for next page (only present when `has_next` is true)

**Important Notes:**
- The `next_token` is an opaque string - don't modify or parse it
- Failed/not-found terms are only reported on page 1 (not repeated on subsequent pages)
- Statistics (`stats`) reflect only the current page's results
- Always check `has_next` before requesting another page

**Example - Iterating Through All Pages (Python):**
```python
import requests

base_url = "http://localhost:9292/ws/map/"
params = {"i": "TP53,BRCA1,KCND2", "m": ">>ensembl>>uniprot", "mode": "lite"}

all_results = []
while True:
    response = requests.get(base_url, params=params).json()
    all_results.extend(response.get("mappings", []))

    pagination = response.get("pagination", {})
    if not pagination.get("has_next"):
        break
    params["p"] = pagination["next_token"]

print(f"Total mappings: {len(all_results)}")
```

**Example - Lite Mode Search Response:**
```json
{
  "mode": "lite",
  "query": {"terms": ["P04637"], "raw": "P04637"},
  "results": [
    {"d": "uniprot", "id": "P04637", "has_attr": true, "xref_count": 21}
  ],
  "stats": {"total_results": 1, "returned": 1, "by_dataset": {"uniprot": 1}},
  "pagination": {"page": 1}
}
```

**Example - Lite Mode Mapping Response with Errors:**
```json
{
  "mode": "lite",
  "query": {"terms": ["P04637", "INVALID"], "chain": ">>uniprot>>hgnc", "raw": "P04637,INVALID >>uniprot>>hgnc"},
  "mappings": [
    {"input": "P04637", "source": {"d": "uniprot", "id": "P04637", "has_attr": true},
     "targets": [{"d": "hgnc", "id": "HGNC:11998", "has_attr": true}]},
    {"input": "INVALID", "error": "No mapping found"}
  ],
  "stats": {"total_terms": 2, "mapped": 1, "failed": 1, "total_targets": 1},
  "pagination": {"page": 1}
}
```

**Backward Compatibility**: The `d=1` parameter still works and is equivalent to `mode=full`.

### Query Syntax

Biobtree supports intuitive query syntax for mapping identifiers across datasets.

#### CLI Query Command
```bash
# Query from command line (returns pretty-printed JSON)
biobtree query "<identifiers> >> <dataset> >> <dataset>"

# Specify database location
biobtree --out-dir <path> query "<query>"

# Response mode options (default: full)
biobtree query -m full "P04637"           # Full mode with all attributes
biobtree query -m lite "P04637"           # Lite mode - compact IDs only
biobtree query --mode lite "P04637 >> hgnc"  # Lite mode for mappings

# Filter by dataset
biobtree query -s uniprot "P04637"        # Filter results to uniprot dataset
```

#### Basic Mapping Syntax
```bash
# Simple lookup (no mapping)
biobtree query "P27348"

# Map through single dataset
biobtree query "P27348 >> hgnc"

# Map through multiple datasets (multi-hop)
biobtree query "P27348 >> hgnc >> chembl"
biobtree query "ENSG00000134308 >> uniprot >> hgnc"

# Multiple identifiers
biobtree query "P27348,Q04917 >> hgnc"
biobtree query "cas9 >> uniprot >> hgnc"

# Lipid metabolism queries
biobtree query "SLM:000094711"                    # SwissLipids lipid lookup
biobtree query "SLM:000094711 >> uniprot"         # Find proteins associated with lipid
biobtree query "P00533 >> swisslipids"            # Find lipids associated with protein
biobtree query "GO:0008203 >> swisslipids"        # Find lipids in GO biological process

# Gene expression queries
biobtree query "ENSG00000139618"                  # Gene expression profile
biobtree query "ENSG00000139618 >> bgee"          # Gene to expression data
biobtree query "UBERON:0000955 >> bgee"           # Find genes expressed in brain
biobtree query "CL:0000576 >> bgee"               # Find genes expressed in monocytes
biobtree query "ENSG00000139618 >> bgee >> uberon" # Gene to tissues where expressed
biobtree query "ENSG00000139618 >> bgee >> cl"    # Gene to cell types where expressed

# Single-cell transcriptomics queries (CELLxGENE)
biobtree query "CL_0000540"                       # Cell type lookup (neuron)
biobtree query "CL:0000540 >> cellxgene"          # Find datasets containing neurons
biobtree query "CL_0000540 >> cellxgene_celltype" # Cell type tissue distribution
biobtree query "UBERON:0000955 >> cellxgene"      # Find brain single-cell datasets
biobtree query "MONDO:0007254 >> cellxgene"       # Find breast cancer scRNA-seq datasets

# Single Cell Expression Atlas queries (SCXA)
biobtree query "E-MTAB-6386"                      # Experiment metadata lookup
biobtree query "E-MTAB-6386 >> scxa"              # Experiment details with cell types
biobtree query "ENSG00000139618 >> scxa_expression"  # Gene expression summary
biobtree query "ENSG00000139618 >> scxa_expression >> scxa"  # Gene to experiments
biobtree query "CL:0000084 >> scxa"               # Find experiments with T cells
biobtree query "UBERON:0002048 >> scxa"           # Find lung single-cell experiments

# GWAS genetics queries
biobtree query "rs12451471"                       # SNP variant lookup with traits
biobtree query "rs12451471 >> gwas_study"         # SNP to GWAS studies
biobtree query "BRCA1 >> gwas"                    # Find SNPs in BRCA1 gene
biobtree query "Type 2 diabetes >> gwas"          # Find SNPs for disease
biobtree query "EFO:0000400 >> gwas"              # EFO trait to SNPs
biobtree query "GCST010481 >> gwas"               # Study to SNP associations

# dbSNP genetic variant queries
biobtree query "rs200676709"                      # SNP lookup with genomic position
biobtree query "rs200676709 >> hgnc"              # SNP to associated gene
biobtree query "BRCA1 >> dbsnp"                   # Find SNPs in gene

# AlphaMissense pathogenicity queries
biobtree query "1:69094:G:T"                      # Variant pathogenicity lookup
biobtree query "1:69094:G:T >> uniprot"           # Variant to affected protein
biobtree query "1:69094:G:T >> transcript"        # Variant to affected transcripts
biobtree query "P04637 >> alphamissense"          # Find variants for protein
biobtree query "ENST00000269305.9"                # Transcript mean pathogenicity

# RefSeq reference sequence queries
biobtree query "NM_007294"                        # RefSeq transcript lookup
biobtree query "NM_007294 >> uniprot"             # RefSeq transcript to UniProt
biobtree query "NP_005219 >> entrez"              # RefSeq protein to Entrez Gene
biobtree query "NC_000017.11"                     # RefSeq chromosome lookup

# IntAct protein interaction queries
biobtree query "P49418"                           # Protein interaction lookup
biobtree query "P49418 >> intact"                 # Get interaction partners
biobtree query "P49418 >> intact >> uniprot"      # Get partner protein details

# STRING protein network queries
biobtree query "9606.ENSP00000269305"             # STRING protein lookup
biobtree query "9606.ENSP00000269305 >> string"   # Get interaction partners with scores
biobtree query "BRCA1 >> ensembl >> string"       # Gene to STRING network

# BioGRID genetic interaction queries
biobtree query "112315"                           # BioGRID interactor lookup
biobtree query "112315 >> biogrid"                # Get interaction partners
biobtree query "112315 >> biogrid >> uniprot"     # Interactor to UniProt
biobtree query "P45985 >> biogrid"                # UniProt to BioGRID interactions
biobtree query "6416 >> entrez >> biogrid"        # Entrez Gene to BioGRID

# MSigDB gene set queries
biobtree query "M5890"                            # Gene set lookup (systematic name)
biobtree query "HALLMARK_APOPTOSIS"               # Gene set lookup (standard name)
biobtree query "BRCA1 >> hgnc >> msigdb"          # Find gene sets containing BRCA1
biobtree query "672 >> entrez >> msigdb"          # Find gene sets by Entrez Gene ID
biobtree query "M5890 >> msigdb >> pubmed"        # Gene set to publications
biobtree query "M5890 >> msigdb >> go"            # Gene set to GO terms
biobtree query "GO:0006915 >> go >> msigdb"       # Find gene sets linked to GO term

# GenCC gene-disease validity queries
biobtree query "BRCA1 >> gencc"                   # Find gene-disease curations for BRCA1
biobtree query "Fanconi anemia >> gencc"          # Find curations by disease name
biobtree query "BRCA1 >> gencc >> mondo"          # Gene to disease ontology terms
biobtree query "BRCA1 >> gencc >> hpo"            # Gene to inheritance patterns

# BindingDB binding affinity queries
biobtree query "50000308"                         # BindingDB entry lookup
biobtree query "50000308 >> uniprot"              # Find target proteins for compound
biobtree query "P00533 >> bindingdb"              # Find binding data for protein (EGFR)
biobtree query "aspirin >> bindingdb"             # Search by ligand name
biobtree query "50000308 >> bindingdb >> pubchem" # Compound to PubChem CID
biobtree query "50000308 >> bindingdb >> chembl"  # Compound to ChEMBL ID

# CTD toxicogenomics queries
biobtree query "D000082"                          # CTD chemical lookup (Acetaminophen)
biobtree query "D000082 >> ctd >> entrez"         # Chemical to interacting genes
biobtree query "D000082 >> ctd >> mesh"           # Chemical to MeSH disease terms
biobtree query "D000082 >> ctd >> mondo"          # Chemical to MONDO disease ontology
biobtree query "Acetaminophen >> ctd"             # Search by chemical name
biobtree query "D000082 >> ctd >> taxonomy"       # Chemical to organism context
biobtree query "D000082 >> ctd >> pubchem"        # Chemical to PubChem compound

# PharmGKB pharmacogenomics queries
biobtree query "warfarin"                         # Drug lookup with dosing guidelines
biobtree query "warfarin >> pharmgkb >> hgnc"     # Drug to related genes
biobtree query "warfarin >> pharmgkb >> pharmgkb_guideline"  # Drug to CPIC/DPWG guidelines
biobtree query "warfarin >> pharmgkb >> pharmgkb_pathway"    # Drug to PK/PD pathways
biobtree query "CYP2C9 >> hgnc >> pharmgkb_guideline"        # Gene to dosing guidelines
biobtree query "CYP2C9 >> hgnc >> pharmgkb_clinical"         # Gene to clinical annotations
biobtree query "rs1799853 >> pharmgkb_variant"               # Variant annotation lookup
biobtree query "rs1799853 >> pharmgkb_clinical"              # Variant clinical evidence
biobtree query "CYP2C9 >> pharmgkb_gene"                     # Pharmacogene details (VIP, CPIC)
```

#### Filter Syntax
Use `[]` brackets to filter results at any step:

```bash
# Filter on boolean field
biobtree query "ENSG00000134308 >> uniprot[uniprot.reviewed==true] >> hgnc"

# Filter on string field
biobtree query "P27348 >> go[go.type==\"biological_process\"]"

# Filter before mapping
biobtree query "9606 >> [ensembl.genome==\"homo_sapiens\"] >> transcript"

# Multiple filters in chain
biobtree query "cas9 >> uniprot[uniprot.reviewed==true] >> hgnc[hgnc.status==\"Approved\"]"

# Complex filter expressions (CEL syntax)
biobtree query "P27348 >> ensembl[ensembl.overlaps(114129278,114129328)]"
biobtree query "hgnc >> chembl[chembl.molecule.highestDevelopmentPhase>2]"

# Gene expression filters
biobtree query "UBERON:0000955 >> bgee[bgee.expression_score>90]"  # High expression in brain
biobtree query "ENSG00000139618 >> bgee[bgee.call_quality==\"gold quality\"]"  # Gold quality only

# GWAS filters
biobtree query "Type 2 diabetes >> gwas[gwas.p_value<0.00000005]"  # Genome-wide significant SNPs
biobtree query "rs12451471 >> gwas[gwas.chr_id==\"11\"]"  # Filter by chromosome
biobtree query "BRCA1 >> gwas[gwas.pvalue_mlog>7.3]"  # -log10(p) > 7.3 (p < 5e-8)

# dbSNP filters
biobtree query "BRCA1 >> dbsnp[dbsnp.allele_frequency>0.01]"  # Common variants (MAF > 1%)
biobtree query "rs200676709 >> dbsnp[dbsnp.clinical_significance!=\"\"]"  # ClinVar annotated

# AlphaMissense filters
biobtree query "P04637 >> alphamissense[alphamissense.am_class==\"likely_pathogenic\"]"  # Pathogenic variants only
biobtree query "P04637 >> alphamissense[alphamissense.am_pathogenicity>=0.9]"  # High pathogenicity score

# IntAct filters
biobtree query "P49418 >> intact[intact.interactions[0].confidence_score>0.6]"  # High-confidence interactions
biobtree query "P49418 >> intact[intact.interactions[0].detection_method~\"two hybrid\"]"  # By method

# STRING filters
biobtree query "9606.ENSP00000269305 >> string[string.interactions[0].score>700]"  # High-confidence interactions (>0.7)
biobtree query "9606.ENSP00000269305 >> string[string.interactions[0].has_experimental==true]"  # Experimentally validated

# GenCC filters
biobtree query "BRCA1 >> gencc[gencc.classification_title==\"Definitive\"]"  # Only definitive classifications
biobtree query "BRCA1 >> gencc[gencc.moi_title==\"Autosomal dominant\"]"     # Filter by inheritance mode

# MSigDB filters
biobtree query "BRCA1 >> hgnc >> msigdb[msigdb.collection==\"H\"]"           # Only Hallmark gene sets
biobtree query "BRCA1 >> hgnc >> msigdb[msigdb.gene_count>100]"              # Large gene sets only
biobtree query "TP53 >> hgnc >> msigdb[msigdb.collection==\"C2\"]"           # Curated gene sets only
biobtree query "M5890 >> msigdb[msigdb.pmid!=\"\"]"                          # Gene sets with publications

# BindingDB filters
biobtree query "P00533 >> bindingdb[bindingdb.ki!=\"\"]"                     # Only entries with Ki values
biobtree query "P00533 >> bindingdb[bindingdb.ic50!=\"\"]"                   # Only entries with IC50 values
biobtree query "aspirin >> bindingdb[bindingdb.target_source_organism==\"Homo sapiens\"]"  # Human targets only

# CTD toxicogenomics filters
biobtree query "D000082 >> ctd[ctd.gene_interaction_count>10]"              # Chemicals with many gene interactions
biobtree query "D000082 >> ctd[ctd.disease_association_count>5]"            # Chemicals with many disease links
biobtree query "Acetaminophen >> ctd[ctd.chemical_name~\"Acetaminophen\"]"  # Filter by chemical name pattern

# PharmGKB pharmacogenomics filters
biobtree query "warfarin >> pharmgkb >> pharmgkb_guideline[pharmgkb_guideline.source==\"CPIC\"]"  # CPIC guidelines only
biobtree query "warfarin >> pharmgkb >> pharmgkb_pathway[pharmgkb_pathway.is_pharmacokinetic==true]"  # PK pathways only
biobtree query "CYP2C9 >> hgnc >> pharmgkb_clinical[pharmgkb_clinical.level_of_evidence==\"1A\"]"  # Highest evidence
biobtree query "CYP2C9 >> pharmgkb_gene[pharmgkb_gene.is_vip==true]"        # VIP genes only
```

#### Migration Guide - Breaking Change in Mapping Queries

**IMPORTANT**: The `s=` (source dataset) parameter has been removed from mapping queries. The first `>>` in the mapping chain now acts as a **lookup operation** instead of a cross-reference mapping.

**Old Syntax (Deprecated)**:
```ruby
# Web API - old approach with s= parameter
localhost:9292/ws/map/?i=TP53&s=ensembl&m=>>uniprot

# CLI - old approach (no longer works)
# The s= parameter is no longer available
```

**New Syntax (Required)**:
```ruby
# Web API - first >> is lookup, subsequent >> are mappings
localhost:9292/ws/map/?i=TP53&m=>>ensembl>>uniprot

# CLI - first >> is lookup, subsequent >> are mappings
biobtree query "TP53 >> ensembl >> uniprot"

# Wildcard lookup - search everywhere first
localhost:9292/ws/map/?i=TP53&m=>>*>>uniprot
biobtree query "TP53 >> * >> uniprot"
```

**Key Changes**:
- ✅ **First `>>dataset`**: Always performs a **lookup** operation (finds identifiers in that dataset)
- ✅ **Subsequent `>>`**: Perform **cross-reference mappings** (follow xrefs between datasets)
- ❌ **Removed**: The `s=` parameter is no longer supported
- ⚠️ **Required**: All mapping queries must have at least 2 steps (lookup + target), or use wildcard `>>*`

**Examples**:
```ruby
# Old: i=TP53&s=hgnc&m=>>uniprot
# New: i=TP53&m=>>hgnc>>uniprot

# Old: i=P27348&s=uniprot&m=>>ensembl>>paralog
# New: i=P27348&m=>>uniprot>>ensembl>>paralog

# Old: i=TP53&m=>>uniprot (with implicit search everywhere)
# New: i=TP53&m=>>*>>uniprot (explicit wildcard search)
```

**Function-Style Syntax (Still Supported for Backward Compatibility)**:
```ruby
# Web API with old function-style syntax
localhost:9292/ws/map/?i=P27348&m=map(uniprot).filter(uniprot.reviewed==true).map(hgnc)
```


### Publication
https://f1000research.com/articles/8-145

### Configuration

Biobtree uses configuration files located in the `conf/` directory:

```
conf/
├── application.param.json      # Application settings and parameters
├── source.dataset.json         # Primary datasets to process
├── default.dataset.json        # Derived datasets (via xref, included by default)
├── optional.dataset.json       # Derived datasets (optional, excluded to reduce data size)
├── medical_term_mappings.json  # Medical terminology mappings
├── aliases.json                # Predefined ID sets for batch queries
└── ensembl/                    # Ensembl genome metadata
    ├── ensembl.paths.json
    ├── ensembl_bacteria.paths.json
    ├── ensembl_fungi.paths.json
    ├── ensembl_metazoa.paths.json
    ├── ensembl_plants.paths.json
    └── ensembl_protists.paths.json
```

#### Dataset Configuration Files

**`source.dataset.json`**: Defines primary datasets that biobtree processes. These datasets can create cross-references (xrefs) with each other and serve as the foundation for data integration.

Dataset paths use **full URLs** for clarity and simplicity:
```json
{
  "taxonomy": {
    "path": "ftp://ftp.ebi.ac.uk/pub/databases/uniprot/current_release/knowledgebase/taxonomy/taxonomy.xml.gz"
  }
}
```

This allows easy identification of data sources and simplifies maintenance. The actual source URL used during build is recorded in `dataset_state.json` for tracking.

**`default.dataset.json`**: Defines datasets derived automatically via cross-references when processing source datasets. These are included by default in all builds.

**`optional.dataset.json`**: Defines derived datasets that are optional. Excluding these reduces the final database size while maintaining core functionality.

**Note**: All three dataset files are merged at runtime. To build with only source and default datasets (excluding optional), use appropriate build flags.

#### Other Configuration Files

**`application.param.json`**: Main configuration file for biobtree application settings including database backend, remote checks, and runtime parameters.

**`medical_term_mappings.json`**: Configuration for medical terminology mappings used in clinical and disease-related datasets.

**`aliases.json`**: Define named aliases for batch queries. Use `alias:name` in queries to expand to predefined ID sets. Supports inline IDs or external file references for large sets:
```json
{
  "my_genes": {"ids": ["ENSG00000139618", "ENSG00000141510"]},
  "large_set": {"file": "gene_list.txt"}
}
```
Query with: `localhost:9292/ws/search?i=alias:my_genes`

**`ensembl/*.paths.json`**: Path configurations for different Ensembl genome divisions (main, bacteria, fungi, metazoa, plants, protists).

#### Disabling Remote Configuration Checks

By default, biobtree checks for new configuration releases from GitHub. To disable this (useful for offline environments or when using custom configurations):

Add to `conf/application.param.json`:

```json
{
  "disableRemoteConfigCheck": "y",
  "disableEnsemblReleaseCheck": "y"
}
```

**Settings:**
- `disableRemoteConfigCheck`: Disables GitHub configuration version checks and automatic downloads
- `disableEnsemblReleaseCheck`: Disables Ensembl release version checks and metadata updates

**Behavior:**
- When disabled, biobtree uses only local configuration files
- First-time installations will still download configs if directories don't exist
- Useful for air-gapped systems, custom deployments, or development environments

### Incremental Updates

Biobtree supports incremental updates with crash-safe state tracking. The system uses a three-phase status model stored in `dataset_state.json`:

| Status | Meaning | On Next Run |
|--------|---------|-------------|
| `processing` | Dataset being processed (bucket files being written) | Rebuild (was interrupted) |
| `processed` | Processing complete, awaiting merge | Merge only (skip parsing) |
| `merged` | Fully complete, data in final index | Skip (unless source changed) |

**State Transitions:**
```
(new/source changed) → processing → processed → merged
                           ↑                        │
                           └──── (source changes) ──┘
```

**Crash Recovery:**
- Crash during parsing → status stays `processing` → next run rebuilds from scratch
- Crash after parsing but before merge → status is `processed` → next run merges existing bucket files
- Crash after merge → status is `merged` → next run skips entirely

**Commands:**
```sh
# Normal update (checks source changes, uses incremental)
./biobtree -d "uniprot,taxonomy" update

# Force rebuild (ignores state, rebuilds everything)
./biobtree -d "uniprot,taxonomy" --force update

# Check what would be updated without running
./biobtree -d "uniprot,taxonomy" check
```

**State File Location:** `{out-dir}/index/dataset_state.json`

### Building from source

biobtree is written with GO for the data processing and Vue.js for the web application part.

#### Building the biobtree binary

Requirements: Go >= 1.13

```sh
# Using Makefile (recommended)
make build

# See all available commands
make help
```

**Makefile commands:**
- `make build` - Build biobtree binary
- `make run` - Build and run biobtree
- `make proto` - Regenerate protobuf code (only needed when .proto files change)
- `make clean` - Clean build artifacts


