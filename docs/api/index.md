# API Reference

## REST API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/search` | GET | Search identifiers |
| `/api/map` | GET | Map through dataset chains |
| `/api/entry` | GET | Get full entry details |
| `/api/meta` | GET | List all datasets |

### Search

```
GET /api/search?i={terms}&s={dataset}&p={page}
```

**Parameters:**
- `i` (required): Comma-separated identifiers
- `s` (optional): Filter to specific dataset
- `p` (optional): Pagination token

**Example:**
```bash
curl "http://localhost:8000/api/search?i=BRCA1"
```

### Mapping

```
GET /api/map?i={terms}&m={chain}&p={page}
```

**Parameters:**
- `i` (required): Comma-separated identifiers
- `m` (required): Mapping chain (e.g., `>>ensembl>>uniprot`)
- `p` (optional): Pagination token

**Example:**
```bash
curl "http://localhost:8000/api/map?i=TP53&m=>>ensembl>>uniprot"
```

### Entry Details

```
GET /api/entry?i={identifier}&s={dataset}
```

**Parameters:**
- `i` (required): Single identifier
- `s` (required): Dataset name

**Example:**
```bash
curl "http://localhost:8000/api/entry?i=P04637&s=uniprot"
```

### Metadata

```
GET /api/meta
```

Returns list of all available datasets.

### Help/Schema

```
GET /api/help?topic={topic}
```

**Parameters:**
- `topic` (optional): `edges`, `filters`, `hierarchies`, `patterns`, `examples`, or `all` (default)

Returns biobtree schema reference including dataset connections and query patterns.

## Public API

The public API is available at:

```
https://sugi.bio/biobtree/api/
```

**Examples:**
```bash
# Search
curl "https://sugi.bio/biobtree/api/search?i=SCN9A"

# Map gene to proteins
curl "https://sugi.bio/biobtree/api/map?i=BRCA1&m=>>ensembl>>uniprot"

# Get entry details
curl "https://sugi.bio/biobtree/api/entry?i=HGNC:10597&s=hgnc"
```

## See Also

- [Query Syntax](query-syntax.md)
- [Filter Reference](filter-reference.md)
- [Edge Reference](edge-reference.md)
