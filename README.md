# Biobtree


Biobtree is a bioinformatics tool which allows mapping the bioinformatics datasets
via identifiers and special keywors with simple or advance chain query capability.


## Features

* **Datasets** - supports wide datasets such as `Ensembl` `Uniprot` `ChEMBL` `HMDB` `Taxonomy` `GO` `EFO` `HPO` `UBERON` `CL` `HGNC` `ECO` `Uniparc` `Uniref` `RNACentral` `Bgee` `GWAS Catalog` `dbSNP` `IntAct`  with tens of more via cross references
by retrieving latest data from providers

* **MapReduce** - processes small or large datasets based on users selection and build B+ tree based uniform local database via specialized MapReduce based tecnique with efficient storage usage 

* **Query** - Allow simple or advance chain queries between datasets with intiutive syntax which allows writing RDF or graph like queries

* **Genome** - supports querying full Ensembl genomes coordinates with `transcript`, `CDS`, `exon`, `utr` with several attiributes, mapped datasets and identifiers such as `ortholog` ,`paralog` or probe identifers belongs `Affymetrix` or `Illumina`

* **Protein** - Uniprot proteins including protein features with variations and mapped datasets.

* **Chemistry** - `ChEMBL`, `HMDB`, `ChEBI`, `LIPID MAPS`, and `SwissLipids` datasets supported for chemistry, disease, lipid metabolism, and drug releated analaysis. SwissLipids provides 779K+ lipid structures with protein associations, GO annotations, tissue localization, and evidence codes

* **Patents** - `SureChEMBL` patent data with 43M+ patents, 30M+ compounds, and patent-compound mappings for drug discovery and IP analysis

* **Clinical Trials** - `ClinicalTrials.gov` data with trial metadata, conditions, interventions, publications, and automatic drug mapping to ChEMBL molecules

* **Genetic Variants** - `ClinVar` database with curated genetic variant-disease relationships, including variant classifications, clinical significance, review status, HGVS expressions, gene annotations, and phenotype associations

* **Pathways** - `Reactome` pathway database with 23K+ curated pathways across 16 species, including protein/gene/compound participants, pathway hierarchy, GO mappings, disease annotations, and evidence codes (TAS/IEA) for curation quality

* **Non-Coding RNAs** - `RNACentral` database with 49.8M+ unique ncRNA sequences aggregated from 56 expert databases, including rRNA, miRNA, lncRNA, tRNA, and other RNA types with comprehensive metadata

* **Gene Expression** - `Bgee` database with curated gene expression data across 30+ species and 1,000+ anatomical structures. Includes tissue-specific expression patterns, expression quality scores, multi-technology support (Affymetrix, RNA-Seq, scRNA-Seq), observation counts, and cross-references to Ensembl genes and UBERON tissues

* **GWAS Genetics** - `GWAS Catalog` from NHGRI-EBI with 1,000,000+ SNP-trait associations and 182,000+ published studies. Includes variant-level data (genomic positions, genes, p-values, effect sizes) and study-level metadata (publications, sample sizes, platforms). Supports variant-trait discovery, gene-based variant lookup, disease genetics exploration, and links to EFO trait ontology. Future enhancement planned for ancestry-based filtering

* **Genetic Variants** - `dbSNP` (database of Single Nucleotide Polymorphisms) from NCBI with RefSNP IDs (rs numbers), genomic coordinates, allele information, population allele frequencies, gene associations, and clinical significance data. Supports variant lookup, gene-to-SNP mapping, allele frequency analysis, and variant type classification (SNV, insertion, deletion)

* **Protein Interactions** - `IntAct` database from EBI with ~1.8 million experimentally validated protein-protein interactions across ~100,000 unique proteins. Provides detailed experimental evidence including detection methods, interaction types, confidence scores, experimental roles, and direct citations to 23,000+ publications. Supports interaction network analysis, drug target discovery, and pathway exploration with PSI-MI standardized terms

* **Taxonomy & Ontologies** - `Taxonomy` `GO` `EFO` `ECO` `HPO` `MONDO` `UBERON` `CL` data with mapping to other datasets and child and parent query capability. CL (Cell Ontology) provides 2,700+ cell type classifications for tissue-specific and cell-specific analysis

* **Your data** - Your custom data can be integrated with or without relation to other datasets

**Note**: Detailed documentation for each dataset can be found at `tests/datasets/*/README.md`

* **Web UI** - Web interface for easy explorations and examples

* **Web Services** - REST or gRPC services

* **R & Python** - [Bioconductor R](https://github.com/tamerh/biobtreeR) and [Python](https://github.com/tamerh/biobtreePy) wrapper packages to use from existing pipelines easier with built-in databases

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

# build with gene expression (requires Ensembl and works well with UBERON)
biobtree -d "ensembl,bgee,uberon" build

# build with GWAS genetics (works well with EFO, HGNC)
biobtree -d "gwas,gwas_study,efo,hgnc" build

# build with genetic variants (works well with HGNC, ClinVar)
biobtree -d "dbsnp,hgnc" build

# build with protein interactions (requires UniProt)
biobtree -d "uniprot,intact" build

# once data is built start web for using ws and ui
biobtree web

# to see all options and datasets use help
biobtree help

```

#### Starting biobtree with built-in databases

```sh
# 4 built-in database provided with commonly studied datasets and organism genomes in order to speed up database build process
# Check following func doc for each database content 
# https://github.com/tamerh/biobtreeR/blob/master/R/buildData.R

biobtree --pre-built 1 install
biobtree web
```
Builting databases updated regularly at least for each Ensembl release and all builtin database files along with configuration files are hosted in spererate github [repository](https://github.com/tamerh/biobtree-conf)

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

# Retrieve entry with filtered mapping entries. Only page parameter is optional
localhost:9292/ws/filter/?i={identifier}&s={dataset}&f={filter_datasets}&p={page}

# Retrieve entry results with page index. All the parameters are mandatory
localhost:9292/ws/page/?i={identifier}&s={dataset}&p={page}&t={total}

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

# IntAct protein interaction queries
biobtree query "P49418"                           # Protein interaction lookup
biobtree query "P49418 >> intact"                 # Get interaction partners
biobtree query "P49418 >> intact >> uniprot"      # Get partner protein details
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

# IntAct filters
biobtree query "P49418 >> intact[intact.interactions[0].confidence_score>0.6]"  # High-confidence interactions
biobtree query "P49418 >> intact[intact.interactions[0].detection_method~\"two hybrid\"]"  # By method
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

<!-- ### Integrating your dataset

User data can be integrated to biobtree. Since biobtree has capability to process large datasets, this feature creates an alternative for  mapping related data to be indexed with biobtree. Data should be gzipped and in an xml format compliant with UniProt xml schema [definition](ftp://ftp.uniprot.org/pub/databases/uniprot/current_release/knowledgebase/complete/uniprot.xsd). Once data has been prepared, file location needs to be configured in biobtree configuration file which is located at `conf/source.dataset.json`. After these configuration dataset used similarly with other dataset like

```sh
biobtree -d "+my_data" start
``` -->

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

**`default.dataset.json`**: Defines datasets derived automatically via cross-references when processing source datasets. These are included by default in all builds.

**`optional.dataset.json`**: Defines derived datasets that are optional. Excluding these reduces the final database size while maintaining core functionality.

**Note**: All three dataset files are merged at runtime. To build with only source and default datasets (excluding optional), use appropriate build flags.

#### Other Configuration Files

**`application.param.json`**: Main configuration file for biobtree application settings including database backend, remote checks, and runtime parameters.

**`medical_term_mappings.json`**: Configuration for medical terminology mappings used in clinical and disease-related datasets.

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

### Building from source

biobtree is written with GO for the data processing and Vue.js for the web application part.

#### Building the biobtree binary

Requirements: Go >= 1.13

```sh
# Using Makefile (recommended)
make build

# Or directly
cd src && go build -o ../biobtree

# See all available commands
make help
```

**Makefile commands:**
- `make build` - Build biobtree binary
- `make run` - Build and run biobtree
- `make proto` - Regenerate protobuf code (only needed when .proto files change)
- `make clean` - Clean build artifacts

#### Building the web application

To build the web application for development in the web directory run

```sh
npm install
npm run serve
```

To build the web package run

```sh
npm run build
```

### Adding New Datasets

To integrate a new dataset into biobtree, the following components must be modified:

#### 1. Configuration Files
- **`conf/source.dataset.json`**: Add basic dataset definition (name, id, path, useLocalFile)
- **`conf/default.dataset.json`**: Add full metadata (url, attrs, hasFilter)
- For hierarchical datasets (ontologies): Also add `datasetnameParent` and `datasetnameChild` definitions

#### 2. Protocol Buffers
- **`pbuf/pbuf.proto`**: Define attribute structure as a protobuf message
- Compile with: `make proto` (generates `pbuf.pb.go`)

#### 3. Data Parser
- **`src/update/datasetname.go`**: Create parser implementing `update()` method
- Key operations:
  - Save entries: `addProp3(id, datasetID, marshaledAttrs)`
  - Text search: `addXref(term, textLinkID, id, datasetName, true)`
  - Cross-references: `addXref(fromID, fromDatasetID, toID, toDatasetName, false)`
  - **Important**: Second parameter must be dataset **ID** (numeric), fourth parameter must be dataset **name** (string)

#### 4. Merge Logic
- **`src/generate/mergeg.go`**: Add dataset to `xref` struct and unmarshal case for your dataset ID
- Without this, attributes will appear empty in responses
- also if dataset does not has any xref it should also add to another place in mergeg.go 

#### 5. Filter Support (Optional)
If `hasFilter="yes"`:
- **`src/service/service.go`**: Add CEL declaration
- **`src/service/mapfilter.go`**: Add filter evaluation case
#### 6. Adding test 
 Each dataset needs to add its tests in tests/datasets/ folder with the same approach and convention with other datasets. Details
 can be seen in tests/README.md 

#### Build Order
```sh
# 1. Compile protobuf
make proto

# 2. Build biobtree
make build

# 3. Build database
./biobtree -d "datasetname" build
```

**Common Pitfalls:**
- Using dataset name instead of ID in `addXref` parameter 2 causes "dataset id to integer conversion error"
- Forgetting `make proto` after changing `.proto` files
- Not adding dataset ID case in `mergeg.go` results in empty attributes
- Not creating bidirectional cross-references

### Database Backend

Biobtree supports both **LMDB** and **MDBX** database backends through a clean abstraction layer.

**Default:** LMDB (proven stability, mature codebase)
**Optional:** MDBX (auto-growing database, easier sizing)
**Performance:** Identical in real-world workloads (extensively tested)

#### Configuration

To switch backends, add to `conf/application.param.json`:

```json
{
  "dbBackend": "lmdb"  // or "mdbx"
}
```

LMDB is used by default if not specified.

#### Why LMDB Default?

Extensive testing (REST, gRPC, NFS, local storage) showed:
- ✅ Identical performance in all scenarios
- ✅ LMDB more mature and proven
- ✅ Both perform excellently for biobtree's workload

Use MDBX if you prefer auto-growing database (no manual size calculation needed).

---

### Use Cases

**Drug Discovery**:
- Find all patents for compounds targeting EGFR
- Identify patent families covering specific drug candidates
- Map patent compounds to clinical trial drugs

**Competitive Intelligence**:
- Track competitor patent filings by assignee
- Monitor technology trends via IPC/CPC codes
- Find patent gaps in therapeutic areas

**Research Integration**:
- Link patents to PubMed citations
- Connect patented compounds to protein targets
- Map patents to genes and pathways


### TODO

- Geographic search by facility location

### Documentation

For detailed information about patent data processing:
- **Patents Module**: `../modules/patents/README.md`
- **Processing Scripts**: `../modules/patents/scripts/`
- **Data Source**: https://chembl.gitbook.io/surechembl
