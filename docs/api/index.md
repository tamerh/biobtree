# API Reference

## REST API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/search` | GET | Search identifiers |
| `/api/map` | GET | Map through dataset chains |
| `/api/entry` | GET | Get full entry details |
| `/api/meta` | GET | List all datasets |
| `/api/release` | GET | Latest build's release metadata snapshot (`?id=` for a specific one) |

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

### Release metadata snapshot

Every successful generate writes an immutable metadata record beside the databases:

```
<out>/releases/<release_id>.json   # never overwritten
<out>/releases/current.json        # pointer to the newest one
```

`release_id` is `<UTC timestamp>-<federation>-<db_vN>`, e.g.
`20261109T110653Z-main-db_v56`. Each snapshot embeds the exact `/ws/meta`
payload (datasets, `data_db_version`, `data_version`, thresholds, freshness
flags) plus the `federations` map (`main: db_v56`, `dbsnp: db_v4`, …) and the
biobtree binary version — so a paper or benchmark can pin provenance without a
running server. Size is ~165 KB per release.

```
GET /api/release                      # current.json
GET /api/release?id=<release_id>      # one snapshot
```

Backfill an existing tree (e.g. a release that predates the feature):

```bash
cd src
go run ./cmd/releasesnap -out ../out_prod -conf ../ -federation main -db-version db_v56 -binary-version v2.20.6
```

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
