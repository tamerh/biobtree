# API Reference

## REST API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/ws/` | GET | Search identifiers |
| `/ws/map/` | GET | Map through dataset chains |
| `/ws/entry/` | GET | Get full entry details |
| `/ws/meta` | GET | List all datasets |

### Search

```
GET /ws/?i={terms}&s={dataset}&mode={full|lite}&p={page}
```

**Parameters:**
- `i` (required): Comma-separated identifiers
- `s` (optional): Filter to specific dataset
- `mode` (optional): `full` (default) or `lite`
- `p` (optional): Pagination token

**Example:**
```bash
curl "http://localhost:9292/ws/?i=P04637&mode=lite"
```

### Mapping

```
GET /ws/map/?i={terms}&m={chain}&mode={full|lite}&p={page}
```

**Parameters:**
- `i` (required): Comma-separated identifiers
- `m` (required): Mapping chain (e.g., `>>ensembl>>uniprot`)
- `mode` (optional): `full` (default) or `lite`
- `p` (optional): Pagination token

**Example:**
```bash
curl "http://localhost:9292/ws/map/?i=TP53&m=>>ensembl>>uniprot&mode=lite"
```

### Entry Details

```
GET /ws/entry/?i={identifier}&s={dataset}
```

**Parameters:**
- `i` (required): Single identifier
- `s` (required): Dataset name

**Example:**
```bash
curl "http://localhost:9292/ws/entry/?i=P04637&s=uniprot"
```

### Metadata

```
GET /ws/meta
```

Returns list of all available datasets.

## Response Modes

- **full**: Complete data with all attributes
- **lite**: Compact IDs-only (~50x smaller, for AI agents)

## See Also

- [Query Syntax](query-syntax.md)
- [Filter Reference](filter-reference.md)
- [Edge Reference](edge-reference.md)
