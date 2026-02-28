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
| `biobtree_meta` | List available datasets |

## Example Queries

Once connected, ask Claude:

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
| `BIOBTREE_URL` | `http://localhost:9291` | Backend URL |
| `BIOBTREE_PORT` | `8000` | Server port |
| `OPENROUTER_API_KEY` | (required for /chat) | OpenRouter API key |

## See Also

- [mcp_srv/README.md](../../mcp_srv/README.md) - Full technical documentation
- [Tools Reference](tools.md) - Detailed tool schemas
