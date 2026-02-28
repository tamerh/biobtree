# Biobtree

**Unified access to 70+ biological databases through intuitive chain queries.**

Biobtree aggregates data from major bioinformatics resources (UniProt, Ensembl, ChEMBL, PubChem, ClinVar, GWAS, and 60+ more) into a local, queryable database with cross-reference mapping.

## Key Features

- **70+ Databases** - Genes, proteins, drugs, diseases, pathways, variants, expression data
- **Chain Queries** - Intuitive `>>` syntax: `BRCA1 >> ensembl >> uniprot >> chembl`
- **Filters** - CEL-based filtering: `[reviewed==true]`, `[resolution<2.0]`
- **LLM Integration** - MCP server for Claude Desktop/CLI with natural language queries
- **Fast Local Access** - MapReduce processing with B+ tree indexing

## Quick Start

```bash
# Setup environment and build
conda env create -f conf/conda.yaml
conda activate biobtree
make build

# Build all datasets (production)
./bb.sh                      # Updates all datasets, runs in background
./bb.sh --status             # Check progress
./bb.sh --generate           # Build database after updates
./bb.sh --activate           # Activate new database version
./bb.sh --web                # Start web server (localhost:9292)
```

## Query Examples

```bash
# Gene to drug targets
curl "localhost:9292/ws/map/?i=BRCA1&m=>>ensembl>>uniprot>>chembl_target>>chembl_molecule&mode=lite"

# Protein interactions
curl "localhost:9292/ws/map/?i=P04637&m=>>uniprot>>string&mode=lite"

# Disease variants (with filter)
curl "localhost:9292/ws/map/?i=BRCA1&m=>>clinvar[germline_classification==\"Pathogenic\"]&mode=lite"

# Drug binding affinity
curl "localhost:9292/ws/map/?i=aspirin&m=>>bindingdb>>uniprot&mode=lite"
```

## Web API

```
GET /ws/?i={terms}&mode={full|lite}           # Search
GET /ws/map/?i={terms}&m={chain}&mode=lite    # Map through datasets
GET /ws/entry/?i={id}&s={dataset}             # Get full entry
GET /ws/meta                                   # List datasets
```

## Documentation

Full documentation: **[docs/](docs/index.md)**

| Topic | Link |
|-------|------|
| Getting Started | [docs/getting-started/](docs/getting-started/) |
| Query Syntax | [docs/api/query-syntax.md](docs/api/query-syntax.md) |
| All Datasets | [docs/datasets/](docs/datasets/index.md) |
| MCP Server | [docs/mcp-server/](docs/mcp-server/) |
| Development | [docs/development/](docs/development/) |

## Wrappers

- **R**: [biobtreeR](https://github.com/tamerh/biobtreeR)
- **Python**: [biobtreePy](https://github.com/tamerh/biobtreePy)

## Publication

[F1000Research Article](https://f1000research.com/articles/8-145)

## License

Apache 2.0
