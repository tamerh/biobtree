# Development Guide

## Environment Setup

### Using Conda (Recommended)

```bash
# Create environment with all dependencies
conda env create -f conf/conda.yaml
conda activate biobtree

# Build
make build
```

The conda environment (`conf/conda.yaml`) includes:
- **Go 1.20+** - Backend compiler
- **Python 3.12** - MCP server, scripts
- **Build tools** - make, gcc, protobuf
- **MCP dependencies** - fastapi, httpx, mcp
- **Data tools** - pandas, pyarrow, cellxgene-census
- **Utilities** - jq, curl, tabix

### Manual Setup

Requirements:
- Go 1.23+
- Python 3.8+ (for MCP server)
- make, gcc

### Build

```bash
# Using Makefile
make build

# Or directly
cd src && go build -o ../biobtree
```

## Development vs Production

| Use Case | Tool | Purpose |
|----------|------|---------|
| **Production** | `bb.sh` | Full data processing with logging, versioning |
| **Development** | `biobtree` | Testing, debugging, limited data builds |

### Development Commands (biobtree)

```bash
# Build test database (limited entries per dataset)
./biobtree -d "uniprot,ensembl" test

# Update specific datasets (development)
./biobtree -d "uniprot" update

# Generate database
./biobtree generate

# Start web server
./biobtree web

# Query (server must be running)
curl "localhost:9292/ws/map/?i=BRCA1&m=>>ensembl&mode=lite"
```

### Run Tests

```bash
# Single dataset
python3 tests/run_tests.py uniprot

# All datasets
python3 tests/run_tests.py all
```

## Project Structure

```
biobtreev2/
├── src/                      # Go backend source
│   ├── biobtree.go           # Main CLI
│   ├── update/               # Dataset parsers
│   ├── generate/             # Database generation
│   ├── service/              # Web service
│   ├── query/                # Query parser
│   └── pbuf/                 # Protocol buffers
├── mcp_srv/                  # Python MCP server
├── conf/                     # Configuration files
├── tests/                    # Integration tests
└── docs/                     # Documentation
```

## Adding a New Dataset

1. **Create parser**: `src/update/<dataset>.go`
2. **Add config**: `conf/source.dataset.json`
3. **Create tests**: `tests/datasets/<dataset>/`
4. **Add docs**: `docs/datasets/<dataset>.md`

See [Adding Datasets](adding-datasets.md) for detailed guide.

## Testing Philosophy

- Integration tests over unit tests
- Real database builds with limited data
- Real web server queries
- Reference data from official APIs

See [Testing Guide](testing.md) for details.

## Code Style

- Go: Standard `gofmt`
- Python: Black formatter
- Comments only where logic isn't self-evident

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Submit pull request

## See Also

- [Technical Internals](../internals/) - Deep architecture docs
- [tests/README.md](../../tests/README.md) - Testing documentation
