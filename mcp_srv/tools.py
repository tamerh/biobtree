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
        name="biobtree_variant",
        description=TOOL_DESCRIPTIONS["biobtree_variant"],
        inputSchema=INPUT_SCHEMAS["biobtree_variant"]
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
            "name": "biobtree_variant",
            "description": TOOL_DESCRIPTIONS["biobtree_variant"],
            "parameters": INPUT_SCHEMAS["biobtree_variant"]
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
    `truncated`/`returned`/`total` metadata.

    A continuation token from the service describes the UNTRUNCATED page: it
    resumes after the last element the service returned, not after the last
    element kept here. Following it would silently skip the dropped elements,
    so truncation invalidates it (`has_next=false`, `next_token` removed) and
    reports `dropped_inputs` (map/search rows cut from the tail) for the
    caller to re-query in smaller batches.
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

        # The service token points past the whole page, so it cannot express
        # "resume after item n". Drop it rather than let the caller skip data.
        pagination = c.get("pagination")
        if isinstance(pagination, dict) and (
            pagination.get("next_token") or pagination.get("has_next")
        ):
            pagination = dict(pagination)
            pagination["has_next"] = False
            pagination.pop("next_token", None)
            pagination["invalidated_by_truncation"] = True
            c["pagination"] = pagination

        # Tell the caller which inputs to re-query (map) or which rows were cut.
        dropped_inputs = []
        for item in items[n:]:
            if isinstance(item, dict):
                value = item.get("input")
                if isinstance(value, str) and value and value not in dropped_inputs:
                    dropped_inputs.append(value)
        if dropped_inputs:
            c["dropped_inputs"] = dropped_inputs

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
        elif tool_name == "biobtree_variant":
            # Variant pages come from the Sugi Variant reference, not the
            # biobtree API. The helper enforces the ambiguity policy and the
            # atlas-style content budget; this guarantees valid JSON either way.
            from .variant import variant_lookup
            res = await asyncio.to_thread(
                variant_lookup,
                arguments["entities"],
                arguments.get("gene"),
                arguments.get("slug"),
                arguments.get("section"),
                bool(arguments.get("full", False)),
                arguments.get("include", "none"),
            )
            return _truncate_result(res, max_result_length)
        elif tool_name == "biobtree_enrich":
            genes = arguments["genes"]
            if isinstance(genes, str):
                genes = [g.strip() for g in genes.split(",") if g.strip()]
            res = await client.enrich(
                genes,
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
