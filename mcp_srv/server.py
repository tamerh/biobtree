"""
Biobtree MCP Server

Combined FastAPI REST API + MCP over SSE server for biobtree.

Provides:
- REST API endpoints at /api/*
- MCP over SSE endpoint at /mcp for Claude Desktop/CLI
- Health check at /health

Usage:
    # Run server
    python -m mcp_srv.server

    # Or with uvicorn directly
    uvicorn mcp_srv.server:app --host 0.0.0.0 --port 8000

Configuration (environment variables):
    BIOBTREE_URL=http://localhost:9291
    BIOBTREE_PORT=8000
    BIOBTREE_LOG_LEVEL=INFO
"""

import asyncio
import json
import logging
import sys
import time
from contextlib import asynccontextmanager
from logging.handlers import RotatingFileHandler
from typing import Any, Optional

from fastapi import FastAPI, Query, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from sse_starlette.sse import EventSourceResponse
from starlette.middleware.base import BaseHTTPMiddleware

from mcp.server import Server
from mcp.types import Tool, TextContent

from .biobtree_client import BiobtreeClient, BiobtreeError
from .config import config
from .schema import get_schema, SCHEMA_EDGES, SCHEMA_FILTERS, SCHEMA_HIERARCHIES

# =============================================================================
# Logging Setup
# =============================================================================

def setup_logging():
    """Configure logging with file rotation."""
    # Create log directory
    config.log_dir.mkdir(parents=True, exist_ok=True)

    log_format = "%(asctime)s %(levelname)s [%(name)s] %(message)s"

    # Console handler
    console_handler = logging.StreamHandler(sys.stdout)
    console_handler.setFormatter(logging.Formatter(log_format))

    # Main log file (rotating, 10MB max, keep 5 backups)
    file_handler = RotatingFileHandler(
        config.log_file,
        maxBytes=10*1024*1024,
        backupCount=5
    )
    file_handler.setFormatter(logging.Formatter(log_format))

    logging.basicConfig(
        level=getattr(logging, config.log_level),
        handlers=[console_handler, file_handler]
    )

    # Access logger (separate file for request logs)
    access_logger = logging.getLogger("access")
    access_handler = RotatingFileHandler(
        config.access_log_file,
        maxBytes=10*1024*1024,
        backupCount=5
    )
    access_handler.setFormatter(logging.Formatter("%(asctime)s %(message)s"))
    access_logger.addHandler(access_handler)
    access_logger.setLevel(logging.INFO)

setup_logging()
logger = logging.getLogger(__name__)
access_logger = logging.getLogger("access")


def get_client_ip(request: Request) -> str:
    """Extract client IP from request, handling proxies."""
    # Check X-Forwarded-For header (set by nginx/proxies)
    forwarded = request.headers.get("X-Forwarded-For")
    if forwarded:
        return forwarded.split(",")[0].strip()
    # Check X-Real-IP header
    real_ip = request.headers.get("X-Real-IP")
    if real_ip:
        return real_ip
    # Fall back to direct client
    return request.client.host if request.client else "unknown"


# =============================================================================
# Biobtree Client (singleton)
# =============================================================================

client: Optional[BiobtreeClient] = None


async def get_client() -> BiobtreeClient:
    """Get or create biobtree client."""
    global client
    if client is None:
        client = BiobtreeClient()
    return client


# =============================================================================
# MCP Server Setup
# =============================================================================

mcp_server = Server(config.mcp_server_name)


# Tool definitions for MCP
TOOLS = [
    Tool(
        name="biobtree_search",
        description="""Search biobtree for biological identifiers.

Finds entries matching the given terms across 70+ integrated databases.
Returns compact results with dataset, ID, and cross-reference counts.

PARAMETERS:
- terms: Comma-separated identifiers (required)
- dataset: Filter to specific dataset (optional)
- mode: "lite" (compact) or "full" (detailed) - default "lite"

EXAMPLES:
- Search gene: terms="TP53"
- Search protein: terms="P04637"
- Search multiple: terms="BRCA1,BRCA2,TP53"
- Search in dataset: terms="TP53", dataset="ensembl"
- Search drug: terms="aspirin"
- Search disease: terms="breast cancer"
- Search variant: terms="rs1799853"

DATASETS (common):
- Genes: ensembl, hgnc, entrez
- Proteins: uniprot, uniparc, uniref
- Drugs: chembl, pubchem, drugcentral
- Diseases: efo, mondo, mesh
- Variants: dbsnp, clinvar, gwas
- Pathways: reactome, go
- Expression: bgee, cellxgene
- Pharmacogenomics: pharmgkb""",
        inputSchema={
            "type": "object",
            "properties": {
                "terms": {
                    "type": "string",
                    "description": "Comma-separated identifiers to search"
                },
                "dataset": {
                    "type": "string",
                    "description": "Filter to specific dataset (optional)"
                },
                "mode": {
                    "type": "string",
                    "enum": ["lite", "full"],
                    "default": "lite",
                    "description": "Response mode"
                },
                "page": {
                    "type": "string",
                    "description": "Pagination token (next_token from previous response)"
                }
            },
            "required": ["terms"]
        }
    ),
    Tool(
        name="biobtree_map",
        description="""Map identifiers through biobtree dataset chains.

The core tool for cross-database queries. Maps identifiers from one database
to another through intermediate datasets using chain syntax.

CHAIN SYNTAX:
>> dataset1[filter] >> dataset2[filter] >> ...

First >> is lookup, subsequent >> are cross-reference mappings.

PARAMETERS:
- terms: Comma-separated identifiers (required)
- chain: Mapping chain like ">>ensembl>>uniprot" (required)
- mode: "lite" or "full" - default "lite"

IMPORTANT: Use biobtree_help tool to get:
- Valid dataset connections (edges)
- Available filters per dataset
- Common query patterns

QUICK EXAMPLES:
- Gene to protein: terms="BRCA1", chain=">>ensembl>>uniprot"
- Gene to drugs: terms="EGFR", chain=">>ensembl>>uniprot>>chembl_target_component>>chembl_target>>chembl_assay>>chembl_activity>>chembl_molecule"
- Disease to genes: terms="diabetes", chain=">>mondo>>gencc>>ensembl"
- SNP to disease: terms="rs1799853", chain=">>dbsnp>>clinvar>>mondo"
- Ontology parents: terms="GO:0006915", chain=">>go>>goparent"

COMMON FILTERS:
- >>ensembl[ensembl.genome=="homo_sapiens"]
- >>uniprot[uniprot.reviewed==true]
- >>chembl_molecule[chembl.molecule.highestDevelopmentPhase>2]""",
        inputSchema={
            "type": "object",
            "properties": {
                "terms": {
                    "type": "string",
                    "description": "Comma-separated identifiers to map"
                },
                "chain": {
                    "type": "string",
                    "description": "Mapping chain (e.g., '>> ensembl >> uniprot')"
                },
                "mode": {
                    "type": "string",
                    "enum": ["lite", "full"],
                    "default": "lite",
                    "description": "Response mode"
                },
                "page": {
                    "type": "string",
                    "description": "Pagination token (next_token from previous response)"
                }
            },
            "required": ["terms", "chain"]
        }
    ),
    Tool(
        name="biobtree_entry",
        description="""Get full entry details from biobtree.

Retrieves complete information for a specific identifier in a dataset,
including all attributes and cross-references.

PARAMETERS:
- identifier: The ID to look up (required)
- dataset: The dataset containing the entry (required)

EXAMPLES:
- Protein details: identifier="P04637", dataset="uniprot"
- Gene details: identifier="ENSG00000141510", dataset="ensembl"
- Drug details: identifier="CHEMBL25", dataset="chembl"
- Disease details: identifier="EFO:0000305", dataset="efo"
- Variant details: identifier="rs1799853", dataset="dbsnp"
- Pathway details: identifier="R-HSA-109582", dataset="reactome"

USE CASES:
- Get protein function, sequence features, disease associations
- Get drug mechanism, targets, clinical phase
- Get gene location, transcripts, orthologs
- Get variant allele frequencies, clinical significance
- Get pathway participants, hierarchy""",
        inputSchema={
            "type": "object",
            "properties": {
                "identifier": {
                    "type": "string",
                    "description": "The identifier to look up"
                },
                "dataset": {
                    "type": "string",
                    "description": "The dataset containing the entry"
                }
            },
            "required": ["identifier", "dataset"]
        }
    ),
    Tool(
        name="biobtree_meta",
        description="""Get biobtree metadata and available datasets.

Returns information about all integrated datasets including names,
entry counts, and relationships. Useful for discovering what data
is available and understanding the data model.

NO PARAMETERS REQUIRED.

RETURNS:
- List of all datasets with IDs and names
- Entry counts per dataset
- Cross-reference relationships

USE THIS TO:
- See all available datasets before querying
- Check if a specific database is integrated
- Understand data coverage""",
        inputSchema={
            "type": "object",
            "properties": {}
        }
    ),
    Tool(
        name="biobtree_help",
        description="""Get biobtree schema reference - dataset connections, filters, and query patterns.

Call this tool when you need to:
- Know which datasets connect to which (EDGES)
- Find available filters for a dataset
- See example query patterns
- Understand ontology hierarchies
- IMPORTANT: Learn filter syntax rules (use "filter_syntax" topic) - float values need .0 suffix!
- IMPORTANT: Learn disease ontology mapping strategies (use "disease_ontology" topic)

Returns a compact JSON schema with all dataset relationships and queryable attributes.

PARAMETERS:
- topic: Optional filter - "edges", "filters", "hierarchies", "patterns", "examples", "filter_syntax", "disease_ontology", or "all" (default)
  - "filter_syntax": CRITICAL - explains .0 suffix for floats, no scientific notation, case-sensitive strings
  - "disease_ontology": CRITICAL - explains which ontology each database uses and how to use bridges/parent terms when direct mapping fails""",
        inputSchema={
            "type": "object",
            "properties": {
                "topic": {
                    "type": "string",
                    "enum": ["edges", "filters", "hierarchies", "patterns", "examples", "filter_syntax", "disease_ontology", "all"],
                    "default": "all",
                    "description": "Which section of the schema to return"
                }
            }
        }
    )
]


@mcp_server.list_tools()
async def list_tools() -> list[Tool]:
    """Return list of available tools."""
    return TOOLS


@mcp_server.call_tool()
async def call_tool(name: str, arguments: dict[str, Any]) -> list[TextContent]:
    """Handle MCP tool calls."""
    biobtree = await get_client()

    try:
        if name == "biobtree_search":
            result = await biobtree.search(
                terms=arguments["terms"],
                dataset=arguments.get("dataset"),
                mode=arguments.get("mode", "lite"),
                page=arguments.get("page")
            )

        elif name == "biobtree_map":
            result = await biobtree.map(
                terms=arguments["terms"],
                chain=arguments["chain"],
                mode=arguments.get("mode", "lite"),
                page=arguments.get("page")
            )

        elif name == "biobtree_entry":
            result = await biobtree.entry(
                identifier=arguments["identifier"],
                dataset=arguments["dataset"]
            )

        elif name == "biobtree_meta":
            result = await biobtree.meta()

        elif name == "biobtree_help":
            topic = arguments.get("topic", "all")
            result = get_schema(topic)

        else:
            return [TextContent(
                type="text",
                text=json.dumps({"error": f"Unknown tool: {name}"})
            )]

        return [TextContent(
            type="text",
            text=json.dumps(result, indent=2)
        )]

    except BiobtreeError as e:
        logger.error(f"Tool {name} failed: {e}")
        return [TextContent(
            type="text",
            text=json.dumps({"error": str(e)})
        )]
    except Exception as e:
        logger.exception(f"Tool {name} unexpected error: {e}")
        return [TextContent(
            type="text",
            text=json.dumps({"error": f"Internal error: {str(e)}"})
        )]


# =============================================================================
# FastAPI Application
# =============================================================================

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan handler."""
    global client

    logger.info(f"Starting Biobtree MCP Server on port {config.port}")
    logger.info(f"Logs: {config.log_dir}")

    # Initialize client
    client = BiobtreeClient()

    # Check biobtree connection
    if await client.health_check():
        logger.info(f"Connected to biobtree at {config.biobtree_url}")
    else:
        logger.warning(f"Biobtree not running at {config.biobtree_url} - API calls will fail until started")

    yield

    # Cleanup
    if client:
        await client.close()
    logger.info("Server shutdown complete")


app = FastAPI(
    title="Biobtree API",
    description="REST API and MCP server for biobtree biological database queries",
    version="1.0.0",
    lifespan=lifespan
)

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# =============================================================================
# Request Logging Middleware
# =============================================================================

class RequestLoggingMiddleware(BaseHTTPMiddleware):
    """Middleware to log all incoming requests with IP and timing."""

    async def dispatch(self, request: Request, call_next):
        start_time = time.perf_counter()
        client_ip = get_client_ip(request)

        # Process request
        response = await call_next(request)

        # Calculate duration
        duration_ms = (time.perf_counter() - start_time) * 1000

        # Build log message
        method = request.method
        path = request.url.path
        query = str(request.url.query) if request.url.query else ""
        status = response.status_code

        # Format: IP METHOD /path?query STATUS TIMEms
        if query:
            log_msg = f"{client_ip} {method} {path}?{query} {status} {duration_ms:.1f}ms"
        else:
            log_msg = f"{client_ip} {method} {path} {status} {duration_ms:.1f}ms"

        # Log to access log
        if config.log_requests:
            access_logger.info(log_msg)

        return response


# Add request logging middleware
app.add_middleware(RequestLoggingMiddleware)


# =============================================================================
# Health Check
# =============================================================================

@app.get("/health")
async def health():
    """Health check endpoint."""
    biobtree = await get_client()
    biobtree_ok = await biobtree.health_check()

    return {
        "status": "healthy" if biobtree_ok else "degraded",
        "biobtree": "connected" if biobtree_ok else "disconnected",
        "biobtree_url": config.biobtree_url
    }


# =============================================================================
# REST API Endpoints
# =============================================================================

@app.get("/api/search")
async def api_search(
    i: str = Query(..., description="Comma-separated identifiers to search"),
    s: Optional[str] = Query(None, description="Filter to specific dataset"),
    mode: Optional[str] = Query(None, description="Response mode: lite or full"),
    p: Optional[str] = Query(None, description="Pagination token")
):
    """
    Search for biological identifiers.

    Finds entries matching the given terms across 70+ integrated databases.
    """
    try:
        biobtree = await get_client()
        result = await biobtree.search(terms=i, dataset=s, mode=mode, page=p)
        return result
    except BiobtreeError as e:
        return JSONResponse(status_code=503, content={"error": str(e)})


@app.get("/api/map")
async def api_map(
    i: str = Query(..., description="Comma-separated identifiers to map"),
    m: str = Query(..., description="Mapping chain (e.g., '>>ensembl>>uniprot')"),
    mode: Optional[str] = Query(None, description="Response mode: lite or full"),
    p: Optional[str] = Query(None, description="Pagination token")
):
    """
    Map identifiers through dataset chains.

    The core endpoint for cross-database queries.
    """
    try:
        biobtree = await get_client()
        result = await biobtree.map(terms=i, chain=m, mode=mode, page=p)
        return result
    except BiobtreeError as e:
        return JSONResponse(status_code=503, content={"error": str(e)})


@app.get("/api/entry")
async def api_entry(
    i: str = Query(..., description="The identifier to look up"),
    s: str = Query(..., description="The dataset containing the entry")
):
    """
    Get full entry details.

    Retrieves complete information for a specific identifier in a dataset.
    """
    try:
        biobtree = await get_client()
        result = await biobtree.entry(identifier=i, dataset=s)
        return result
    except BiobtreeError as e:
        return JSONResponse(status_code=503, content={"error": str(e)})


@app.get("/api/meta")
async def api_meta():
    """
    Get metadata about available datasets.

    Returns information about all integrated datasets.
    """
    try:
        biobtree = await get_client()
        result = await biobtree.meta()
        return result
    except BiobtreeError as e:
        return JSONResponse(status_code=503, content={"error": str(e)})


@app.get("/api/help")
async def api_help(
    topic: str = Query("all", description="Topic: edges, filters, hierarchies, patterns, examples, filter_syntax, disease_ontology, or all")
):
    """
    Get biobtree schema reference.

    Returns dataset connections, filters, and query patterns.
    """
    return get_schema(topic)


# =============================================================================
# MCP over SSE Endpoint
# =============================================================================

@app.get("/mcp")
async def mcp_sse(request: Request):
    """
    MCP over SSE endpoint for Claude Desktop/CLI.

    Configure Claude Desktop with:
    {
        "mcpServers": {
            "biobtree": {
                "url": "https://sugi.bio/mcp"
            }
        }
    }
    """
    async def event_generator():
        """Generate SSE events for MCP protocol."""
        # Send initial connection event
        yield {
            "event": "open",
            "data": json.dumps({
                "protocolVersion": "2024-11-05",
                "serverInfo": {
                    "name": config.mcp_server_name,
                    "version": "1.0.0"
                },
                "capabilities": {
                    "tools": {}
                }
            })
        }

        # Keep connection alive and handle messages
        try:
            while True:
                if await request.is_disconnected():
                    break

                # Send keepalive ping every 30 seconds
                yield {"event": "ping", "data": ""}
                await asyncio.sleep(30)

        except asyncio.CancelledError:
            pass

    return EventSourceResponse(event_generator())


@app.post("/mcp")
async def mcp_message(request: Request):
    """
    Handle MCP JSON-RPC messages.

    This endpoint receives tool calls and returns results.
    """
    try:
        body = await request.json()
        method = body.get("method")
        params = body.get("params", {})
        request_id = body.get("id")

        if method == "tools/list":
            # Return list of tools
            tools_list = [
                {
                    "name": tool.name,
                    "description": tool.description,
                    "inputSchema": tool.inputSchema
                }
                for tool in TOOLS
            ]
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {"tools": tools_list}
            }

        elif method == "tools/call":
            # Execute tool
            tool_name = params.get("name")
            arguments = params.get("arguments", {})

            result = await call_tool(tool_name, arguments)

            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "content": [{"type": "text", "text": result[0].text}]
                }
            }

        elif method == "initialize":
            # Handle initialization
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "protocolVersion": "2024-11-05",
                    "serverInfo": {
                        "name": config.mcp_server_name,
                        "version": "1.0.0"
                    },
                    "capabilities": {
                        "tools": {}
                    }
                }
            }

        else:
            return {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {
                    "code": -32601,
                    "message": f"Method not found: {method}"
                }
            }

    except Exception as e:
        logger.exception(f"MCP message error: {e}")
        return {
            "jsonrpc": "2.0",
            "id": body.get("id") if 'body' in dir() else None,
            "error": {
                "code": -32603,
                "message": str(e)
            }
        }


# =============================================================================
# Main Entry Point
# =============================================================================

def run_http_server():
    """Run the HTTP server with uvicorn (for remote access)."""
    import uvicorn
    uvicorn.run(
        "mcp_srv.server:app",
        host=config.host,
        port=config.port,
        log_level=config.log_level.lower()
    )


async def run_stdio_server():
    """Run the MCP server with stdio transport (for local Claude CLI)."""
    from mcp.server.stdio import stdio_server

    logger.info("Starting MCP server in stdio mode")

    async with stdio_server() as (read_stream, write_stream):
        await mcp_server.run(
            read_stream,
            write_stream,
            mcp_server.create_initialization_options()
        )


def main():
    """Main entry point with mode selection."""
    import argparse

    parser = argparse.ArgumentParser(description="Biobtree MCP Server")
    parser.add_argument(
        "--mode",
        choices=["http", "stdio"],
        default="stdio",
        help="Server mode: 'stdio' for local Claude CLI (default), 'http' for remote access"
    )
    args = parser.parse_args()

    if args.mode == "http":
        run_http_server()
    else:
        # stdio mode for local MCP
        import asyncio
        asyncio.run(run_stdio_server())


if __name__ == "__main__":
    main()
