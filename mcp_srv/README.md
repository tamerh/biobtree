# Biobtree MCP Server

Combined FastAPI REST API + MCP over SSE server for biobtree biological database queries.

## Architecture

```
                         ┌──────────────────────────────────────────────┐
                         │            mcp_srv :8000                     │
                         │                                              │
  /api/*  ──────────────►│  REST API (api.py)                          │
  (direct queries)       │  - search, map, entry, meta, help            │
                         │                                              │
                         │                                              │
  /chat   ──────────────►│  Chat Endpoint (chat.py)                    │
  (LLM + tools)          │  - Receives question                         │
                         │  - Calls OpenRouter LLM ◄──────────────────┐ │
                         │  - LLM decides to use tools                 │ │
                         │  - Executes biobtree tools ─────┐           │ │
                         │  - Returns tool results to LLM ─┘           │ │
                         │  - LLM generates final answer ──────────────┘ │
                         │                                              │
                         │                                              │    ┌─────────────┐
  /mcp    ──────────────►│  MCP Handlers (mcp_handlers.py)             │───►│  biobtree   │
  (Claude Desktop/CLI)   │  - SSE connection                           │    │  :9291      │
                         │  - JSON-RPC tool calls                       │    └─────────────┘
                         │                                              │
                         │                 │                            │
                         │                 ▼                            │
                         │  Tools (tools.py) ──► biobtree_client.py ───┼───►
                         │  - biobtree_search                           │
                         │  - biobtree_map                              │
                         │  - biobtree_entry                            │
                         │  - biobtree_meta                             │
                         │  - biobtree_help                             │
                         └──────────────────────────────────────────────┘
```

## Chat Endpoint Flow

The `/chat` endpoint provides conversational access to biobtree via LLMs:

```
┌──────────┐     ┌──────────────┐     ┌─────────────┐     ┌──────────┐
│  User    │     │  /chat       │     │  OpenRouter │     │ biobtree │
│          │     │  endpoint    │     │  (LLM)      │     │          │
└────┬─────┘     └──────┬───────┘     └──────┬──────┘     └────┬─────┘
     │                  │                    │                  │
     │  POST question   │                    │                  │
     │─────────────────►│                    │                  │
     │                  │                    │                  │
     │                  │  Send question     │                  │
     │                  │  + tool schemas    │                  │
     │                  │───────────────────►│                  │
     │                  │                    │                  │
     │                  │  Tool call request │                  │
     │                  │◄───────────────────│                  │
     │                  │                    │                  │
     │                  │  Execute query     │                  │
     │                  │─────────────────────────────────────►│
     │                  │                    │                  │
     │                  │  Query results     │                  │
     │                  │◄─────────────────────────────────────│
     │                  │                    │                  │
     │                  │  Tool results      │                  │
     │                  │───────────────────►│                  │
     │                  │                    │                  │
     │                  │    ... (repeat for more tools) ...   │
     │                  │                    │                  │
     │                  │  Final answer      │                  │
     │                  │◄───────────────────│                  │
     │                  │                    │                  │
     │  JSON response   │                    │                  │
     │◄─────────────────│                    │                  │
     │                  │                    │                  │
```

**Key features:**
- LLM automatically decides which biobtree tools to use
- Supports multiple tool calls per question
- Returns both answer and tool call history
- Toggle `with_tools=false` for baseline comparison

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
| `GET /api/search?i=TP53` | Search identifiers |
| `GET /api/map?i=BRCA1&m=>>ensembl>>uniprot` | Map through chains |
| `GET /api/entry?i=P04637&s=uniprot` | Get entry details |
| `GET /api/meta` | List datasets |
| `GET /api/help?topic=edges` | Schema reference |
| `POST /chat` | Chat with LLM + biobtree tools |
| `GET /health` | Health check |
| `POST /mcp` | MCP JSON-RPC endpoint |

## Chat Endpoint

The `/chat` endpoint enables conversational queries using LLMs (via OpenRouter) with automatic biobtree tool calling.

**Request:**
```bash
curl -X POST https://sugi.bio/chat \
  -H "Content-Type: application/json" \
  -d '{
    "question": "What proteins does BRCA1 encode?",
    "model": "anthropic/claude-sonnet-4",
    "with_tools": true
  }'
```

**Response:**
```json
{
  "answer": "BRCA1 encodes the protein P38398 (UniProt)...",
  "model": "anthropic/claude-sonnet-4",
  "tools_used": true,
  "tool_calls": [
    {"tool": "biobtree_search", "arguments": {"terms": "BRCA1"}, "result_length": 15016},
    {"tool": "biobtree_map", "arguments": {"terms": "BRCA1", "chain": ">>ensembl>>uniprot"}, "result_length": 15016}
  ],
  "iterations": 3
}
```

**Parameters:**
| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `question` | string | required | User question |
| `model` | string | claude-sonnet-4 | OpenRouter model ID |
| `with_tools` | bool | true | Enable biobtree tools |
| `system_prompt` | string | (default) | Custom system prompt |

**Supported models (via OpenRouter):**
| Model | ID | Speed | Cost |
|-------|-----|-------|------|
| Claude Haiku | `anthropic/claude-3-haiku` | Fast | $ |
| Claude Sonnet | `anthropic/claude-sonnet-4` | Medium | $$ |
| GPT-4o mini | `openai/gpt-4o-mini` | Fast | $ |
| GPT-4o | `openai/gpt-4o` | Medium | $$$ |
| Llama 3.3 70B | `meta-llama/llama-3.3-70b-instruct` | Fast | $ |

More models at [openrouter.ai/models](https://openrouter.ai/models)

## Benchmarking

Compare LLM answers with and without biobtree tools:

```bash
# Run benchmark with default models and questions
python -m mcp_srv.tests.benchmark_chat

# Quick test with Haiku
python -m mcp_srv.tests.benchmark_chat \
  --models "anthropic/claude-3-haiku" \
  --questions gene_protein_simple protein_function

# Compare multiple models
python -m mcp_srv.tests.benchmark_chat \
  --models "anthropic/claude-3-haiku" "anthropic/claude-sonnet-4" "openai/gpt-4o-mini"

# Save results to JSON
python -m mcp_srv.tests.benchmark_chat --output results.json

# Skip baseline tests (faster)
python -m mcp_srv.tests.benchmark_chat --no-baseline
```

**Benchmark questions:**
| ID | Category | Question |
|----|----------|----------|
| gene_protein_simple | gene_to_protein | What protein does the TP53 gene encode? |
| gene_protein_brca1 | gene_to_protein | What proteins does BRCA1 encode? |
| drug_target | drug_to_target | What are the protein targets of imatinib? |
| gene_disease | gene_to_disease | What diseases are associated with EGFR? |
| variant_clinical | variant_to_disease | Clinical significance of rs1799853? |
| protein_function | protein_info | Function of UniProt P04637? |
| gene_drugs | gene_to_drug | What drugs target EGFR? |
| pathway_genes | pathway_to_gene | Genes in apoptosis pathway? |

## Running

```bash
# Stdio mode - for local Claude CLI (default)
python -m mcp_srv

# HTTP mode - for remote access
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
| `OPENROUTER_API_KEY` | - | OpenRouter API key (required for /chat) |
| `BIOBTREE_DEFAULT_MODEL` | `anthropic/claude-sonnet-4` | Default LLM model |
| `BIOBTREE_CHAT_MAX_ITERATIONS` | `10` | Max tool call iterations |

## Logging

Logs are written to `logs/` directory:

- `mcp_server.log` - Server logs (rotating, 10MB max, 5 backups)
- `access.log` - Request logs with IP addresses

Access log format:
```
2026-02-02 08:02:40 127.0.0.1 GET /api/search?i=BRCA1 200 5.6ms
2026-02-02 08:02:40 10.0.0.5 POST /chat 200 3420.1ms
```

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

```
mcp_srv/
├── server.py           # FastAPI app, middleware, health endpoint
├── api.py              # REST API endpoints (/api/*)
├── chat.py             # Chat endpoint with LLM tool loop
├── mcp_handlers.py     # MCP SSE and JSON-RPC handlers
├── tools.py            # Tool definitions and execution
├── biobtree_client.py  # Async HTTP client for biobtree
├── config.py           # Environment-based configuration
├── schema.py           # Dataset edges, filters, query patterns
└── tests/
    └── benchmark_chat.py  # Chat endpoint benchmark script
```
