# Biobtree MCP Server

Combined FastAPI REST API + MCP over SSE server for biobtree biological database queries.

## Architecture

```
                    ┌─────────────────────────────────┐
   sugi.bio/api/*   │                                 │
   ─────────────────►  FastAPI REST API               │
                    │  (search, map, entry, meta)     │
                    │                                 │
   sugi.bio/mcp     │                                 │    ┌─────────────┐
   ─────────────────►  MCP over SSE                   │────► biobtree    │
   (Claude Desktop/ │  (JSON-RPC tools)               │    │ :9291       │
    Claude CLI)     │                                 │    └─────────────┘
                    │        mcp_srv :8000            │
                    └─────────────────────────────────┘
```

## Tools (MCP)

| Tool | Purpose |
|------|---------|
| `biobtree_search` | Search for identifiers across 70+ databases |
| `biobtree_map` | Map identifiers through dataset chains |
| `biobtree_entry` | Get full entry details |
| `biobtree_meta` | List available datasets |
| `biobtree_help` | Get schema reference (edges, filters, patterns) |

## REST API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/search?terms=TP53` | Search identifiers |
| `GET /api/map?i=BRCA1&m=>>ensembl>>uniprot` | Map through chains |
| `GET /api/entry?i=P04637&s=uniprot` | Get entry details |
| `GET /api/meta` | List datasets |
| `GET /api/help?topic=edges` | Schema reference |
| `GET /health` | Health check |
| `POST /mcp` | MCP JSON-RPC endpoint |

## Running

```bash
# Stdio mode - for local Claude CLI (default)
python -m mcp_srv

# HTTP mode - for remote access (sugi.bio/mcp)
python -m mcp_srv --mode http

# Or with uvicorn directly
uvicorn mcp_srv.server:app --host 0.0.0.0 --port 8000
```

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `BIOBTREE_URL` | `http://localhost:9291` | Biobtree backend URL |
| `BIOBTREE_PORT` | `8000` | Server port |
| `BIOBTREE_LOG_LEVEL` | `INFO` | Logging level |
| `BIOBTREE_LOG_DIR` | `../logs` | Log directory |
| `BIOBTREE_LOG_REQUESTS` | `true` | Log requests with IP |

## Logging

Logs are written to `logs/` directory (relative to biobtreev2):

- `mcp_server.log` - Server logs (rotating, 10MB max, 5 backups)
- `access.log` - Request logs with IP addresses

Access log format:
```
2026-02-02 08:02:40 127.0.0.1 GET /api/search?terms=BRCA1 200 5.6ms
2026-02-02 08:02:40 10.0.0.5 POST /mcp 200 1.4ms
```

Note: When behind nginx, uses `X-Forwarded-For` or `X-Real-IP` headers for client IP.

## Claude Desktop/CLI Configuration

Add to Claude Desktop settings:

```json
{
  "mcpServers": {
    "biobtree": {
      "url": "https://sugi.bio/mcp"
    }
  }
}
```

## Files

- `server.py` - FastAPI app + MCP handlers
- `schema.py` - Dataset edges, filters, query patterns
- `config.py` - Environment-based configuration
- `biobtree_client.py` - Async HTTP client for biobtree
