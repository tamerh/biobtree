# MCP Server

The MCP (Model Context Protocol) server enables LLM integration with biobtree, allowing natural language queries through Claude Desktop, Claude CLI, or any MCP-compatible client.

## Quick Start

### Start the Server

```bash
cd mcp_srv
python -m mcp_srv --mode http
# Server starts at http://localhost:8000
```

### Claude Desktop Integration

Add to Claude Desktop settings:

```json
{
  "mcpServers": {
    "biobtree": {
      "url": "http://localhost:8000/mcp"
    }
  }
}
```

## Available Tools

| Tool | Description |
|------|-------------|
| `biobtree_search` | Search 70+ databases for identifiers |
| `biobtree_map` | Map identifiers through dataset chains |
| `biobtree_entry` | Get full details for an entry |
| `biobtree_atlas` | Curated [Sugi Atlas](https://sugi.bio/atlas) summaries for genes, diseases, and drugs |

## Atlas grounding (`biobtree_atlas`)

For questions about a **gene, disease, or drug** (what it is, its biology, disease/drug/clinical context), `biobtree_atlas` returns curated Sugi Atlas summaries to ground the answer. Call it first for those; fall back to `search`/`map`/`entry` for exact ID mappings, cross-references, or anything not covered.

```text
biobtree_atlas(entities=["TP53", "imatinib", "breast carcinoma"])
```

- Pass the entity name(s) from the question. Covered entities return content + a citable `canonical_url`; uncovered ones are listed in `not_covered` (no dead links).
- Default returns a compact **digest** (Summary + Identifiers) plus the page's `sections` list. Narrow with `section="Disease & clinical"` (a name from `sections`) for one zone, or `full=true` for the whole page.
- Large sections across several entities are trimmed to fit, with a breadcrumb to fetch the full block one entity at a time.

Resolution uses the Atlas name→slug manifest (cached at startup; restart to refresh). Pages are fetched live from `sugi.bio/atlas/<type>/<slug>/index.md`, so coverage always reflects the latest Atlas.

## Example Queries

Once connected, ask Claude:

- "What is known about TP53?" (curated Atlas summary)
- "Compare imatinib and dasatinib — targets and indications"
- "What proteins does BRCA1 encode?"
- "Find drugs that target TP53"
- "What pathways involve P04637?"
- "Show me pathogenic variants in BRCA2"

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /mcp` | MCP over SSE (for Claude Desktop) |
| `POST /mcp` | MCP JSON-RPC |
| `GET /api/search` | Direct search API |
| `GET /api/map` | Direct mapping API |
| `POST /chat` | Chat with tool calling |
| `GET /health` | Health check |

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `BIOBTREE_URL` | `http://localhost:9292` | Backend URL |
| `BIOBTREE_PORT` | `8000` | Server port |
| `OPENROUTER_API_KEY` | (required for /chat) | OpenRouter API key |

## See Also

- [mcp_srv/README.md](../../mcp_srv/README.md) - Full technical documentation
- [Tools Reference](tools.md) - Detailed tool schemas
