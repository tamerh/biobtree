# Biobtree


Biobtree is a bioinformatics tool which allows mapping the bioinformatics datasets
via identifiers and special keywors with simple or advance chain query capability.


## Features

* **Datasets** - supports wide datasets such as `Ensembl` `Uniprot` `ChEMBL` `HMDB` `Taxonomy` `GO` `EFO` `HPO` `HGNC` `ECO` `Uniparc` `Uniref` `RNACentral`  with tens of more via cross references
by retrieving latest data from providers

* **MapReduce** - processes small or large datasets based on users selection and build B+ tree based uniform local database via specialized MapReduce based tecnique with efficient storage usage 

* **Query** - Allow simple or advance chain queries between datasets with intiutive syntax which allows writing RDF or graph like queries

* **Genome** - supports querying full Ensembl genomes coordinates with `transcript`, `CDS`, `exon`, `utr` with several attiributes, mapped datasets and identifiers such as `ortholog` ,`paralog` or probe identifers belongs `Affymetrix` or `Illumina`

* **Protein** - Uniprot proteins including protein features with variations and mapped datasets.

* **Chemistry** - `ChEMBL`, `HMDB`, and `ChEBI` datasets supported for chemistry, disease and drug releated analaysis

* **Patents** - `SureChEMBL` patent data with 43M+ patents, 30M+ compounds, and patent-compound mappings for drug discovery and IP analysis

* **Clinical Trials** - `ClinicalTrials.gov` data with trial metadata, conditions, interventions, publications, and automatic drug mapping to ChEMBL molecules

* **Pathways** - `Reactome` pathway database with 23K+ curated pathways across 16 species, including protein/gene/compound participants, pathway hierarchy, GO mappings, disease annotations, and evidence codes (TAS/IEA) for curation quality

* **Non-Coding RNAs** - `RNACentral` database with 49.8M+ unique ncRNA sequences aggregated from 56 expert databases, including rRNA, miRNA, lncRNA, tRNA, and other RNA types with comprehensive metadata

* **Taxonomy & Ontologies** - `Taxonomy` `GO` `EFO` `ECO` `HPO` `MONDO` data with mapping to other datasets and child and parent query capability

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

# build with clinical trials (requires ChEMBL for drug mapping)
biobtree -d "chembl,clinical_trials" build

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
localhost:8888/ws/meta

# Search 
# i is the only mandatory parameter
localhost:8888/ws/?i={terms}&s={dataset}&p={page}&f={filter}

# Mapping 
# i and m are mandatory parameters
localhost:8888/ws/map/?i={terms}&m={mapfilter_query}&s={dataset}&p={page}

# Retrieve dataset entry. Both paramters are mandatory
localhost:8888/ws/entry/?i={identifier}&s={dataset}

# Retrieve entry with filtered mapping entries. Only page parameter is optional
localhost:8888/ws/filter/?i={identifier}&s={dataset}&f={filter_datasets}&p={page}

# Retrieve entry results with page index. All the parameters are mandatory 
localhost:8888/ws/page/?i={identifier}&s={dataset}&p={page}&t={total}

```

### Query Syntax

Biobtree supports intuitive query syntax for mapping identifiers across datasets.

#### CLI Query Command
```bash
# Query from command line (always returns detailed, pretty-printed JSON)
biobtree query "<identifiers> >> <dataset> >> <dataset>"

# Specify database location
biobtree --out-dir <path> query "<query>"
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
```

**Old Syntax (Still Supported)**:
```ruby
# Web API with old function-style syntax
localhost:8888/ws/map/?i=P27348&m=map(uniprot).filter(uniprot.reviewed==true).map(hgnc)
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
├── application.param.json      # Application settings
├── default.dataset.json        # Default datasets
├── optional.dataset.json       # Optional datasets
├── source.dataset.json         # Data source configurations
└── ensembl/                    # Ensembl genome metadata
    ├── ensembl.paths.json
    ├── ensembl_bacteria.paths.json
    ├── ensembl_fungi.paths.json
    ├── ensembl_metazoa.paths.json
    ├── ensembl_plants.paths.json
    └── ensembl_protists.paths.json
```

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

#### 5. Filter Support (Optional)
If `hasFilter="yes"`:
- **`src/service/service.go`**: Add CEL declaration
- **`src/service/mapfilter.go`**: Add filter evaluation case

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

#### Documentation & Benchmarks

Complete documentation and benchmark tools:

📁 **`examples/mdbx_benchmarks/`**

- `MDBX_INTEGRATION.md` - Complete technical guide
- `MDBX_INTEGRATION_SUMMARY.md` - Executive summary
- `QUICK_START.md` - Benchmark quick start
- `README.md` - Benchmark scripts documentation

To run performance comparisons:

```bash
cd examples/mdbx_benchmarks
./benchmark_lmdb_vs_mdbx.sh      # REST API benchmark
./benchmark_grpc.sh              # gRPC benchmark
```

See `examples/mdbx_benchmarks/QUICK_START.md` for detailed instructions.

---

## Patents Data Integration (SureChEMBL)

### Overview

Biobtree integrates patent data from SureChEMBL (EMBL-EBI) to enable cross-referencing between:
- **Patents** ↔ **Chemical Compounds** ↔ **ChEMBL Targets**
- **Patents** ↔ **Patent Families**
- **Patents** ↔ **IPC/CPC Classification Codes**

**Data Source**: SureChEMBL 2.0 (43M+ patents, 30M+ compounds)
**Update Frequency**: Bi-weekly releases

### Data Preparation

Patent data is processed by the BioYoda patents module and converted to JSON format for biobtree ingestion:

```bash
# 1. Download and process SureChEMBL data (from BioYoda root)
./bioyoda.sh run patents --cluster

# 2. Convert parquet to JSON for biobtree
python modules/patents/scripts/convert_to_biobtree_json.py \
  --input raw_data/patents/surechembl/2025-10-01 \
  --output data/processed/patents/biobtree \
  --verbose
```

**Output Files**:
```
data/processed/patents/biobtree/
├── patents.json          # 43M patent records
├── compounds.json        # 30M chemical compounds
├── mapping.json          # 1.5B patent-compound mappings
└── conversion_summary.json
```

### Biobtree Configuration

Add patents to your biobtree build in `conf/source.dataset.json`:

```json
{
  "patents": {
    "name": "SureChEMBL Patents",
    "version": "2025-10-01",
    "sourceType": "json",
    "sourcePath": "../data/processed/patents/biobtree/patents.json",
    "updateFrequency": "biweekly"
  },
  "surechembl_compounds": {
    "name": "SureChEMBL Compounds",
    "version": "2025-10-01",
    "sourceType": "json",
    "sourcePath": "../data/processed/patents/biobtree/compounds.json"
  },
  "patent_compound_map": {
    "name": "Patent-Compound Mappings",
    "version": "2025-10-01",
    "sourceType": "json",
    "sourcePath": "../data/processed/patents/biobtree/mapping.json"
  }
}
```

### Building with Patent Data

```bash
# Build biobtree with patents + chemistry datasets
cd biobtreev2
./biobtree -d "patents,surechembl_compounds,chembl,uniprot,hgnc" build

# Start web services
./biobtree web
```

### Query Examples

#### Find all patents for a compound
```bash
# Query by ChEMBL ID
biobtree query "CHEMBL203 >> surechembl >> patent"

# Result: All patents containing aspirin
```

#### Find compounds in a patent
```bash
# Query by patent number
biobtree query "US-20110053848-A1 >> patent >> surechembl"

# Result: All compounds extracted from this patent
```

#### Find biological targets for patented compounds
```bash
# Patent → Compounds → ChEMBL Targets → Proteins
biobtree query "US-20110053848-A1 >> surechembl >> chembl >> uniprot"

# Result: All protein targets for compounds in this patent
```

#### Find patents in a family
```bash
# Query by family ID
biobtree query "family:12345678 >> patent"

# Result: All patents in this patent family (US, EP, WO, etc.)
```

#### Find patents by technology classification
```bash
# Query by IPC code
biobtree query "ipc:A61K31 >> patent"

# Result: All patents in pharmaceutical preparations category
```

#### Find genes targeted by patented compounds
```bash
# Patent → Compounds → ChEMBL → Proteins → Genes
biobtree query "US-20110053848-A1 >> surechembl >> chembl >> uniprot >> hgnc"

# Result: Gene symbols for all targets
```

### Cross-Reference Mappings

Patents enable the following identifier mappings:

```
PATENT_NUMBER ↔ SURECHEMBL_COMPOUND_ID
SURECHEMBL_COMPOUND_ID ↔ CHEMBL_ID
PATENT_NUMBER ↔ FAMILY_ID
PATENT_NUMBER ↔ IPC_CODE
PATENT_NUMBER ↔ CPC_CODE
PATENT_NUMBER ↔ ASSIGNEE
INCHI_KEY ↔ SURECHEMBL_COMPOUND_ID
```

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

### Data Schema

**Patents JSON**:
```json
{
  "patents": [
    {
      "id": "internal_id",
      "patent_number": "US-20110053848-A1",
      "country": "US",
      "publication_date": "2011-03-03",
      "family_id": "12345",
      "title": "EGFR inhibitors...",
      "ipc": ["A61K31/517"],
      "cpc": ["A61K31/517"],
      "asignee": ["AstraZeneca"]
    }
  ]
}
```

**Compounds JSON**:
```json
{
  "compounds": [
    {
      "id": "SCHEMBL123",
      "smiles": "CC(C)Cc1ccc(cc1)[C@@H](C)C(=O)O",
      "inchi": "InChI=1S/C13H18O2/...",
      "inchi_key": "HEFNNWSXXWATRW-JTQLQIEISA-N",
      "mol_weight": 206.28
    }
  ]
}
```

**Mappings JSON**:
```json
{
  "mappings": [
    {
      "patent_id": "internal_id",
      "compound_id": "SCHEMBL123",
      "field_id": 2
    }
  ]
}
```

**Field IDs**:
- 1 = Description
- 2 = Claims
- 3 = Abstract
- 4 = Title
- 5 = Image
- 6 = MOL attachment

## Clinical Trials Integration

Clinical trials data from ClinicalTrials.gov is integrated with smart drug-to-ChEMBL mapping capabilities.

### Features

- **Trial Metadata**: NCT ID, title, phase, status, study type
- **Conditions**: Disease and medical conditions associated with trials
- **Interventions**: Drug names, dosages, descriptions with automatic normalization
- **Publications**: Cross-references to PubMed articles (PMIDs)
- **Smart Drug Mapping**: Automatic mapping of intervention names to ChEMBL molecules
  - Multi-attempt lookup (full name, base name, split combinations)
  - Handles drug combinations (e.g., "Edaravone Dexborneol" → 2 ChEMBL IDs)
  - Chemical suffix removal (e.g., "Medroxyprogesterone 17-Acetate" → "Medroxyprogesterone")
  - Case-insensitive, formulation-agnostic

### Cross-References Created

- **NCT ↔ ChEMBL**: Clinical trials linked to drug molecules
- **NCT ↔ PMID**: Clinical trials linked to publications
- **Text Search**: Search trials by intervention name, phase, or status

### Configuration

To enable ChEMBL drug mapping, configure lookup database path in `conf/application.param.json`:

```json
{
  "lookupDbDir": "/path/to/biobtree/out/db",
  "lookupAliasDbDir": "/path/to/biobtree/out/aliasdb"
}
```

### TODO

- Disease ontology mapping (conditions → DisGeNET/UMLS)
- Sponsor normalization for patent linkage
- Geographic search by facility location

### Documentation

For detailed information about patent data processing:
- **Patents Module**: `../modules/patents/README.md`
- **Processing Scripts**: `../modules/patents/scripts/`
- **Data Source**: https://chembl.gitbook.io/surechembl
