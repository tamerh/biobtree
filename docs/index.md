# Biobtree Documentation

Welcome to the Biobtree documentation. Biobtree provides unified access to 70+ biological databases through intuitive chain queries.

## Quick Navigation

| Section | Description |
|---------|-------------|
| [Getting Started](getting-started/) | Installation, quickstart, configuration |
| [Concepts](concepts/) | Architecture, data model, query model |
| [API Reference](api/) | REST API, query syntax, filters |
| [MCP Server](mcp-server/) | LLM integration, Claude Desktop setup |
| [Datasets](datasets/index.md) | All 70+ supported databases |
| [Development](development/) | Contributing, adding datasets, testing |
| [Internals](internals/) | Technical deep-dives (k-way merge, bucket system) |

---

## Getting Started

### Installation

Download the latest release for your platform:
- [GitHub Releases](https://github.com/tamerh/biobtree/releases/latest)

Or use wrapper packages:
- **R**: [biobtreeR](https://github.com/tamerh/biobtreeR)
- **Python**: [biobtreePy](https://github.com/tamerh/biobtreePy)

### Quickstart

```bash
# Build all datasets (production - runs in background)
./bb.sh                      # Update all datasets
./bb.sh --status             # Check progress
./bb.sh --generate           # Build database
./bb.sh --activate           # Activate new version
./bb.sh --web                # Start web server (localhost:9292)

# Query via API
curl "localhost:9292/ws/map/?i=BRCA1&m=>>ensembl>>uniprot&mode=lite"
```

### Build Management

```bash
# Update specific datasets
./bb.sh --only uniprot,chembl      # Update specific datasets
./bb.sh --from pubchem             # Resume from dataset
./bb.sh --check                    # Check for source changes

# Database versions
./bb.sh --db-versions              # Show versions
./bb.sh --activate                 # Activate latest
./bb.sh --cleanup                  # Remove old versions
```

---

## Core Concepts

### Chain Query Syntax

Use `>>` to traverse datasets:

```
identifier >> dataset1 >> dataset2 >> dataset3
```

**Examples:**
```bash
# Gene symbol → Ensembl → UniProt → Drug targets
curl "localhost:9292/ws/map/?i=TP53&m=>>ensembl>>uniprot>>chembl_target&mode=lite"

# Protein → Pathways
curl "localhost:9292/ws/map/?i=P04637&m=>>reactome&mode=lite"

# Disease → Genes
curl "localhost:9292/ws/map/?i=breast%20cancer&m=>>mondo>>gencc>>hgnc&mode=lite"
```

### Filters

Apply CEL-based filters at any step:

```bash
# Reviewed proteins only
curl "localhost:9292/ws/map/?i=TP53&m=>>uniprot[reviewed==true]&mode=lite"

# High-resolution structures
curl "localhost:9292/ws/map/?i=P04637&m=>>pdb[resolution<2.0]&mode=lite"

# Pathogenic variants
curl "localhost:9292/ws/map/?i=BRCA1&m=>>alphamissense[am_class==\"likely_pathogenic\"]&mode=lite"
```

### Response Modes

- **full** (default): Complete data with all attributes
- **lite**: Compact IDs-only format (~50x smaller, optimized for AI agents)

```bash
curl "localhost:9292/ws/map/?i=TP53&m=>>ensembl>>uniprot&mode=lite"
```

---

## Dataset Categories

Biobtree integrates 70+ databases across these categories:

| Category | Examples |
|----------|----------|
| [Genomics](datasets/index.md#genomics) | Ensembl, HGNC, Entrez, RefSeq, dbSNP |
| [Proteins](datasets/index.md#proteins) | UniProt, AlphaFold, PDB, InterPro |
| [Chemistry](datasets/index.md#chemistry) | ChEMBL, PubChem, ChEBI, HMDB |
| [Pathways](datasets/index.md#pathways) | Reactome, STRING, IntAct, SIGNOR |
| [Disease](datasets/index.md#disease) | ClinVar, MONDO, HPO, Orphanet, GWAS |
| [Ontologies](datasets/index.md#ontologies) | GO, EFO, UBERON, Cell Ontology |
| [Expression](datasets/index.md#expression) | Bgee, CELLxGENE, FANTOM5, SCXA |

See [Datasets Index](datasets/index.md) for the complete list.

---

## Web API

```ruby
# Search
GET /ws/?i={terms}&s={dataset}&mode={full|lite}

# Map through datasets
GET /ws/map/?i={terms}&m={chain}&mode={full|lite}

# Get entry details
GET /ws/entry/?i={identifier}&s={dataset}

# List all datasets
GET /ws/meta
```

See [API Reference](api/) for full documentation.

---

## MCP Server (LLM Integration)

Biobtree includes an MCP server for Claude Desktop/CLI integration:

```bash
cd mcp_srv
python -m mcp_srv --mode http
```

Tools available:
- `biobtree_search` - Search 70+ databases
- `biobtree_map` - Map through dataset chains
- `biobtree_entry` - Get full entry details
- `biobtree_meta` - List available datasets

See [MCP Server Documentation](mcp-server/) for setup instructions.

---

## Resources

- **Publication**: [F1000Research Article](https://f1000research.com/articles/8-145)
- **GitHub**: [tamerh/biobtree](https://github.com/tamerh/biobtree)
- **Issues**: [Report bugs or request features](https://github.com/tamerh/biobtree/issues)
