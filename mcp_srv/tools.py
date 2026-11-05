"""
Biobtree Tool Definitions

Tool definitions for MCP and Chat endpoints.
All definitions (TOOL_DESCRIPTIONS, INPUT_SCHEMAS) come from prompts.py.
This file builds the tool objects and handles execution.
"""

import asyncio
import json
import logging
from typing import Any, Dict, List

from mcp.types import Tool

from .biobtree_client import BiobtreeClient, BiobtreeError
from .prompts import TOOL_DESCRIPTIONS, INPUT_SCHEMAS

logger = logging.getLogger(__name__)


# =============================================================================
# MCP Tool Definitions
# =============================================================================

MCP_TOOLS = [
    Tool(
        name="biobtree_search",
        description=TOOL_DESCRIPTIONS["biobtree_search"],
        inputSchema=INPUT_SCHEMAS["biobtree_search"]
    ),
    Tool(
        name="biobtree_map",
        description=TOOL_DESCRIPTIONS["biobtree_map"],
        inputSchema=INPUT_SCHEMAS["biobtree_map"]
    ),
    Tool(
        name="biobtree_entry",
        description=TOOL_DESCRIPTIONS["biobtree_entry"],
        inputSchema=INPUT_SCHEMAS["biobtree_entry"]
    ),
    Tool(
        name="biobtree_atlas",
        description=TOOL_DESCRIPTIONS["biobtree_atlas"],
        inputSchema=INPUT_SCHEMAS["biobtree_atlas"]
    ),
    Tool(
        name="biobtree_enrich",
        description=TOOL_DESCRIPTIONS["biobtree_enrich"],
        inputSchema=INPUT_SCHEMAS["biobtree_enrich"]
    )
]


# =============================================================================
# OpenAI/Chat Tool Definitions
# =============================================================================

# Tool definitions with prompt caching support for Anthropic models.
# cache_control on the LAST tool caches ALL tool definitions (~2,100 tokens).
# See docs/LLM_CACHING.md for details.
CHAT_TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "biobtree_search",
            "description": TOOL_DESCRIPTIONS["biobtree_search"],
            "parameters": INPUT_SCHEMAS["biobtree_search"]
        }
    },
    {
        "type": "function",
        "function": {
            "name": "biobtree_map",
            "description": TOOL_DESCRIPTIONS["biobtree_map"],
            "parameters": INPUT_SCHEMAS["biobtree_map"]
        }
    },
    {
        "type": "function",
        "function": {
            "name": "biobtree_entry",
            "description": TOOL_DESCRIPTIONS["biobtree_entry"],
            "parameters": INPUT_SCHEMAS["biobtree_entry"]
        }
    },
    {
        "type": "function",
        "function": {
            "name": "biobtree_atlas",
            "description": TOOL_DESCRIPTIONS["biobtree_atlas"],
            "parameters": INPUT_SCHEMAS["biobtree_atlas"]
        }
    },
    {
        "type": "function",
        "function": {
            "name": "biobtree_enrich",
            "description": TOOL_DESCRIPTIONS["biobtree_enrich"],
            "parameters": INPUT_SCHEMAS["biobtree_enrich"]
        },
        # Cache control on last tool caches entire tools array
        "cache_control": {"type": "ephemeral"}
    }
]


# =============================================================================
# Tool Execution
# =============================================================================

def _compact_json(obj: Any) -> str:
    """Compact JSON for tool results (no pretty-print whitespace)."""
    return json.dumps(obj, separators=(",", ":"), ensure_ascii=False)


def _payload_list_key(result: Any) -> Any:
    """Return the key of the primary list payload, or None."""
    if isinstance(result, dict):
        for key in ("data", "mappings", "results", "entries", "targets"):
            if isinstance(result.get(key), list):
                return key
    return None


def _truncate_result(result: Any, max_len: int) -> str:
    """Serialize a tool result, truncating at an element boundary if needed.

    A model cannot parse a JSON string cut mid-token, so when the payload is
    too large we keep as many complete elements as fit and add
    `truncated`/`returned`/`total` metadata. The rest of the response
    (stats, pagination, context) is preserved so the caller can continue.
    """
    text = _compact_json(result)
    if len(text) <= max_len:
        return text

    if isinstance(result, list):
        container = {"data": result}
        key = "data"
    elif isinstance(result, dict):
        key = _payload_list_key(result)
        if key is None:
            return _compact_json({
                "truncated": True,
                "returned": 0,
                "total": 0,
                "error": "result too large to return",
                "preview": text[:max(0, max_len - 200)],
            })
        container = result
    else:
        return _compact_json({
            "truncated": True,
            "returned": 0,
            "total": 0,
            "error": "result too large to return",
            "preview": text[:max(0, max_len - 200)],
        })

    items = container[key]
    total = len(items)

    def candidate(n: int) -> dict:
        c = dict(container)
        c[key] = items[:n]
        c["truncated"] = True
        c["returned"] = n
        c["total"] = total
        return c

    # Largest prefix that still fits (including the truncation metadata), so
    # the cut lands between elements and the result stays valid JSON.
    lo, hi, best = 0, total, 0
    while lo <= hi:
        mid = (lo + hi) // 2
        if len(_compact_json(candidate(mid))) <= max_len:
            best = mid
            lo = mid + 1
        else:
            hi = mid - 1

    out = _compact_json(candidate(best))
    if len(out) <= max_len:
        return out

    # Even the metadata is too large; return a valid minimal envelope.
    return _compact_json({
        "truncated": True,
        "returned": 0,
        "total": total,
        "error": "result metadata too large",
        "data": [],
    })


async def execute_tool(
    tool_name: str,
    arguments: Dict[str, Any],
    client: BiobtreeClient,
    max_result_length: int = 50000
) -> str:
    """
    Execute a biobtree tool and return result as string.

    Args:
        tool_name: Name of the tool to execute
        arguments: Tool arguments
        client: BiobtreeClient instance
        max_result_length: Maximum length of result string (truncates if exceeded)

    Returns:
        JSON string with tool result
    """
    try:
        if tool_name == "biobtree_search":
            result = await client.search(
                terms=arguments["terms"],
                dataset=arguments.get("dataset"),
                page=arguments.get("page")
            )
        elif tool_name == "biobtree_map":
            result = await client.map(
                terms=arguments["terms"],
                chain=arguments["chain"],
                page=arguments.get("page")
            )
        elif tool_name == "biobtree_entry":
            result = await client.entry(
                identifier=arguments["identifier"],
                dataset=arguments["dataset"]
            )
        elif tool_name == "biobtree_atlas":
            # Atlas pages come from sugi.bio (not the biobtree API). The Atlas
            # helper already trims section content; this still guarantees the
            # final JSON stays valid if the envelope exceeds the limit.
            from .atlas import atlas_lookup
            res = await asyncio.to_thread(
                atlas_lookup,
                arguments["entities"],
                arguments.get("section"),
                bool(arguments.get("full", False)),
            )
            return _truncate_result(res, max_result_length)
        elif tool_name == "biobtree_enrich":
            from .enrich import enrich_genes
            res = await enrich_genes(
                client,
                arguments["genes"],
                arguments.get("collection", "C5:GO"),
                int(arguments.get("limit", 20)),
            )
            return _truncate_result(res, max_result_length)
        else:
            return _compact_json({"error": f"Unknown tool: {tool_name}"})

        # Compact JSON keeps token cost down; truncate only at element boundaries.
        return _truncate_result(result, max_result_length)

    except BiobtreeError as e:
        return _compact_json({"error": str(e)})
    except Exception as e:
        logger.exception(f"Tool execution error: {e}")
        return _compact_json({"error": str(e)})
