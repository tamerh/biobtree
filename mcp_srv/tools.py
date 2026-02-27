"""
Biobtree Tool Definitions

Tool definitions for MCP and Chat endpoints.
All definitions (TOOL_DESCRIPTIONS, INPUT_SCHEMAS) come from prompts.py.
This file builds the tool objects and handles execution.
"""

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
        name="biobtree_meta",
        description=TOOL_DESCRIPTIONS["biobtree_meta"],
        inputSchema=INPUT_SCHEMAS["biobtree_meta"]
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
            "name": "biobtree_meta",
            "description": TOOL_DESCRIPTIONS["biobtree_meta"],
            "parameters": INPUT_SCHEMAS["biobtree_meta"]
        },
        # Cache control on last tool caches entire tools array
        "cache_control": {"type": "ephemeral"}
    }
]


# =============================================================================
# Tool Execution
# =============================================================================

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
        # Default to lite mode to save tokens
        default_mode = "lite"

        if tool_name == "biobtree_search":
            result = await client.search(
                terms=arguments["terms"],
                dataset=arguments.get("dataset"),
                mode=arguments.get("mode", default_mode),
                page=arguments.get("page")
            )
        elif tool_name == "biobtree_map":
            result = await client.map(
                terms=arguments["terms"],
                chain=arguments["chain"],
                mode=arguments.get("mode", default_mode),
                page=arguments.get("page")
            )
        elif tool_name == "biobtree_entry":
            result = await client.entry(
                identifier=arguments["identifier"],
                dataset=arguments["dataset"]
            )
        elif tool_name == "biobtree_meta":
            result = await client.meta()
        else:
            return json.dumps({"error": f"Unknown tool: {tool_name}"})

        # Format result
        result_str = json.dumps(result, indent=2)

        # Truncate large results to avoid token limits
        if len(result_str) > max_result_length:
            result_str = result_str[:max_result_length] + "\n... [truncated]"

        return result_str

    except BiobtreeError as e:
        return json.dumps({"error": str(e)})
    except Exception as e:
        logger.exception(f"Tool execution error: {e}")
        return json.dumps({"error": str(e)})
