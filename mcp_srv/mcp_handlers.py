"""
Biobtree MCP Handlers

MCP protocol handlers for SSE and JSON-RPC endpoints.
"""

import asyncio
import json
import logging
from typing import Optional, List

from fastapi import APIRouter, Request
from fastapi.responses import JSONResponse
from sse_starlette.sse import EventSourceResponse
from mcp.server import Server
from mcp.types import TextContent

from .biobtree_client import BiobtreeClient
from .config import config
from .tools import MCP_TOOLS, execute_tool

logger = logging.getLogger(__name__)
router = APIRouter(tags=["mcp"])

# MCP Server instance
mcp_server = Server(config.mcp_server_name)

# Singleton client
_client: Optional[BiobtreeClient] = None


async def get_client() -> BiobtreeClient:
    """Get or create biobtree client."""
    global _client
    if _client is None:
        _client = BiobtreeClient()
    return _client


# =============================================================================
# MCP Server Tool Handlers
# =============================================================================

@mcp_server.list_tools()
async def list_tools():
    """Return list of available tools."""
    return MCP_TOOLS


@mcp_server.call_tool()
async def call_tool(name: str, arguments: dict) -> List[TextContent]:
    """Handle tool calls from MCP clients."""
    logger.info(f"MCP tool call: {name} with args: {arguments}")

    biobtree = await get_client()
    result = await execute_tool(name, arguments, biobtree)

    return [TextContent(type="text", text=result)]


# =============================================================================
# HTTP Endpoints for MCP
# =============================================================================

@router.get("/mcp")
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


@router.post("/mcp")
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
                for tool in MCP_TOOLS
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
# Stdio Server Runner
# =============================================================================

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
