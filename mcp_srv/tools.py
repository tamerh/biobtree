"""
Biobtree Tool Definitions

Shared tool definitions for MCP and Chat endpoints.
"""

import json
import logging
from typing import Any, Dict, List

from mcp.types import Tool

from .biobtree_client import BiobtreeClient, BiobtreeError
from .schema import get_schema

logger = logging.getLogger(__name__)


# =============================================================================
# MCP Tool Definitions
# =============================================================================

MCP_TOOLS = [
    Tool(
        name="biobtree_search",
        description="""Search for biological identifiers across 70+ integrated databases.

WORKFLOW:
1. Search WITHOUT dataset filter to discover which databases have your entity
2. For DRUG TARGETS, use these paths (try ALL until one works):
   - drugcentral ID >> drugcentral >> uniprot (mechanism of action)
   - pubchem ID >> pubchem >> pubchem_activity >> uniprot (bioactivity targets)
   - chembl ID >> chembl_molecule >> chembl_target >> uniprot
3. For DISEASE GENES: mondo/hpo ID >> gencc/clinvar/orphanet >> hgnc

RETURNS: id | dataset | name | xref_count""",
        inputSchema={
            "type": "object",
            "properties": {
                "terms": {
                    "type": "string",
                    "description": "Comma-separated identifiers to search"
                },
                "dataset": {
                    "type": "string",
                    "description": "DO NOT USE for initial search. Only use after discovery to narrow results."
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
        description="""Map identifiers between databases using chain syntax.

CRITICAL: Chain MUST start with ">>" (double angle brackets).
WRONG: >drugcentral>>uniprot
RIGHT: >>drugcentral>>uniprot

DRUG TARGET PATTERNS:
- >>drugcentral>>uniprot (FDA drug targets with mechanism)
- >>pubchem>>pubchem_activity>>uniprot (bioactivity targets)
- >>chembl_molecule>>chembl_target>>uniprot (medicinal chemistry)

OTHER PATTERNS:
- >>ensembl>>uniprot (gene to protein)
- >>mondo>>gencc>>hgnc (disease to genes)

SYNTAX: >>dataset1>>dataset2 (always starts with >>)""",
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
        description="""Get full details for a specific identifier in a dataset.

SYNTAX: identifier="<id>", dataset="<dataset>"

RETURNS: Attributes + xref counts (summary of connected datasets).

To get actual cross-references, use biobtree_map (e.g., >>pubchem>>chembl_molecule).

Use biobtree_help to see what attributes each dataset provides.""",
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
            "properties": {},
        }
    ),
    Tool(
        name="biobtree_help",
        description="""Get the biobtree schema - dataset connections, filters, and descriptions.

CALL THIS FIRST to understand:
- EDGES: which datasets connect to which (required for building chains)
- FILTERS: filter syntax and operators

TOPICS: "edges", "filters", "all" (default)""",
        inputSchema={
            "type": "object",
            "properties": {
                "topic": {
                    "type": "string",
                    "enum": ["edges", "filters", "all"],
                    "default": "all",
                    "description": "Which section of the schema to return"
                }
            }
        }
    )
]


# =============================================================================
# OpenAI/Chat Tool Definitions
# =============================================================================

CHAT_TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "biobtree_search",
            "description": "Search 70+ databases. For DRUG TARGETS: use drugcentral>>uniprot or pubchem>>pubchem_activity>>uniprot. For DISEASE GENES: use gencc/clinvar/orphanet.",
            "parameters": {
                "type": "object",
                "properties": {
                    "terms": {
                        "type": "string",
                        "description": "Comma-separated identifiers to search"
                    },
                    "dataset": {
                        "type": "string",
                        "description": "Optional filter. Omit for discovery."
                    }
                },
                "required": ["terms"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "biobtree_map",
            "description": "Map IDs between databases. Chain MUST start with '>>'. DRUG TARGETS: >>drugcentral>>uniprot or >>pubchem>>pubchem_activity>>uniprot. DISEASE GENES: >>mondo>>gencc>>hgnc.",
            "parameters": {
                "type": "object",
                "properties": {
                    "terms": {
                        "type": "string",
                        "description": "Comma-separated identifiers to map"
                    },
                    "chain": {
                        "type": "string",
                        "description": "Chain MUST start with '>>'. Example: '>>drugcentral>>uniprot' (NOT '>drugcentral>>uniprot')"
                    }
                },
                "required": ["terms", "chain"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "biobtree_entry",
            "description": "Get attributes + xref counts for an identifier. Use biobtree_map for actual cross-references.",
            "parameters": {
                "type": "object",
                "properties": {
                    "identifier": {
                        "type": "string",
                        "description": "The ID to look up"
                    },
                    "dataset": {
                        "type": "string",
                        "description": "The dataset containing the entry"
                    }
                },
                "required": ["identifier", "dataset"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "biobtree_help",
            "description": "Get biobtree schema - dataset connections (edges) and filter syntax. CALL THIS FIRST to understand what connects to what.",
            "parameters": {
                "type": "object",
                "properties": {
                    "topic": {
                        "type": "string",
                        "enum": ["edges", "filters", "all"],
                        "description": "Which section to return"
                    }
                },
                "required": []
            }
        }
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
        # Default to lite mode for chat to save tokens (23x smaller responses)
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
        elif tool_name == "biobtree_help":
            result = get_schema(arguments.get("topic", "all"))
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
