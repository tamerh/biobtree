# Getting Started

## Installation

Download the latest release for your platform:
- [GitHub Releases](https://github.com/tamerh/biobtree/releases/latest)

Extract and run from the extracted directory.

### Environment Setup (conda)

For development or running data processing scripts, use the provided conda environment:

```bash
# Create environment
conda env create -f conf/conda.yaml

# Activate
conda activate biobtree
```

This includes:
- Go 1.20+ compiler
- Python 3.12 with MCP server dependencies
- Build tools (make, gcc)
- Data processing tools (pandas, pyarrow)
- CELLxGENE Census for single-cell data

### Alternative: Wrapper Packages

- **R**: [biobtreeR](https://github.com/tamerh/biobtreeR)
- **Python**: [biobtreePy](https://github.com/tamerh/biobtreePy)

## Production Build (bb.sh)

Use `bb.sh` for production data processing. It handles all datasets with proper logging, background execution, and database versioning.

### 1. Build All Datasets

```bash
./bb.sh                      # Update all datasets (runs in background)
./bb.sh --status             # Check progress
```

Each dataset has its own log in `logs/<dataset>.log`.

### 2. Generate Database

After updates complete:

```bash
./bb.sh --generate           # Build LMDB database
./bb.sh --activate           # Activate new version
```

### 3. Start Web Server

```bash
./bb.sh --web                # Start server (localhost:9292)
```

### 4. Query

**Web API:**
```bash
# Search
curl "localhost:9292/ws/?i=BRCA1&mode=lite"

# Map through datasets
curl "localhost:9292/ws/map/?i=BRCA1&m=>>ensembl>>uniprot&mode=lite"

# Get entry details
curl "localhost:9292/ws/entry/?i=P04637&s=uniprot"
```

**MCP Server (for LLM integration):**
```bash
cd mcp_srv && python -m mcp_srv --mode http
# Then use Claude Desktop or API at localhost:8000
```

## Build Management

```bash
# Update specific datasets only
./bb.sh --only uniprot,chembl,hgnc

# Resume from a specific dataset
./bb.sh --from pubchem

# Check for source changes without updating
./bb.sh --check

# Force rebuild even if unchanged
./bb.sh --force --only uniprot
```

## Database Versioning

```bash
./bb.sh --db-versions        # Show all versions
./bb.sh --activate           # Activate latest version
./bb.sh --activate 2         # Activate specific version
./bb.sh --cleanup            # Remove old versions (keep last 2)
```

## Development Build

For development/testing, use `biobtree` directly with limited data:

```bash
# Build test database (limited entries)
biobtree -d "uniprot,ensembl" test

# Run specific dataset tests
python3 tests/run_tests.py uniprot
```

See [Development Guide](../development/) for more details.

## Next Steps

- [Query Syntax](../api/query-syntax.md) - Learn chain queries and filters
- [Datasets](../datasets/index.md) - Browse all 70+ supported databases
- [Configuration](configuration.md) - Customize settings
