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
        description="""Search biobtree for biological identifiers.

Finds entries matching the given terms across 70+ integrated databases.
Returns compact results with dataset, ID, and cross-reference counts.

PARAMETERS:
- terms: Comma-separated identifiers (required)
- dataset: Filter to specific dataset (optional)
- mode: "lite" (compact) or "full" (detailed) - default "lite"

SEARCH STRATEGY:
- Search WITHOUT dataset filter first to discover all database entries
- This reveals available dataset entry points for your query
- Then explore relevant paths from different sources for comprehensive results
- If general search returns too many results, use focused dataset filter:
  - Drugs: dataset="chembl_molecule,pubchem,drugcentral"
  - Genes: dataset="ensembl,hgnc,uniprot"
  - Diseases: dataset="mondo,efo,mesh"

MODE SELECTION:
- lite (default): For discovery and navigation. Returns IDs, names, xref counts.
  Use for: finding connections, ID mapping, graph traversal.
- full: For detailed attributes. Returns all fields including scores, values.
  Use when you need: expression levels, clinical significance, binding affinities,
  pathogenicity scores, development phases, confidence scores.

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
- Metabolites: chebi, hmdb
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
                    "description": "Filter to specific dataset(s), comma-separated (e.g., 'ensembl,hgnc,uniprot')"
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

MODE SELECTION:
- lite (default): For graph traversal. Returns IDs and key attributes only.
  Use for: discovering paths, finding related entities, building networks.
- full: For detailed data. Returns all attributes including numeric values.
  Use when you need: Ki/IC50 values (bindingdb), TPM expression (bgee, cellxgene),
  p-values (gwas), pathogenicity scores (alphamissense), review status (clinvar).

IMPORTANT: Use biobtree_help tool to get:
- Valid dataset connections (edges)
- Available filters per dataset
- Common query patterns

QUICK EXAMPLES:
- Gene to protein: terms="BRCA1", chain=">>ensembl>>uniprot"
- Gene to drugs: terms="EGFR", chain=">>ensembl>>uniprot>>chembl_target>>chembl_molecule"
- Drug to gene: terms="CHEMBL25", chain=">>chembl_molecule>>chembl_target>>uniprot>>hgnc"
- Drug to metabolite: terms="CHEMBL25", chain=">>chembl_molecule>>chebi"
- PubChem to ChEBI: terms="5793", chain=">>pubchem>>chebi"
- Disease to genes: terms="diabetes", chain=">>mondo>>gencc>>ensembl"
- Disease to drugs: terms="breast cancer", chain=">>mondo>>mesh>>ctd>>pubchem"
- SNP to disease: terms="rs1799853", chain=">>dbsnp>>clinvar>>mondo"
- Ontology parents: terms="GO:0006915", chain=">>go>>goparent"
- Ontology children: terms="GO:0006954", chain=">>go>>gochild"
- Disease hierarchy: terms="MONDO:0005148", chain=">>mondo>>mondoparent"
- Gene paralogs: terms="ENSG00000141510", chain=">>ensembl>>paralog"
- Protein to pathways: terms="P04637", chain=">>uniprot>>reactome"
- Pathway to genes: terms="R-HSA-109582", chain=">>reactome>>ensembl"
- Metabolite info: terms="glucose", chain=">>chebi>>pubchem"

ONTOLOGY EXPANSION (IMPORTANT for drug/disease queries):
When querying biological processes (GO) or diseases (MONDO), ALSO query child terms
for broader coverage. Proteins may be annotated with regulatory terms (e.g.,
"regulation of X") rather than the direct process term ("X").
- First get children: chain=">>go>>gochild" or ">>mondo>>mondochild"
- Then query relevant child terms for drugs/genes

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
including ALL attributes and cross-references. Always returns full details.

WHEN TO USE:
- After search/map found an ID, use entry to get complete attributes
- When you need specific values not in lite mode (scores, sequences, coordinates)
- For detailed analysis of a single entity

PARAMETERS:
- identifier: The ID to look up (required)
- dataset: The dataset containing the entry (required)

EXAMPLES:
- Protein details: identifier="P04637", dataset="uniprot"
- Gene details: identifier="ENSG00000141510", dataset="ensembl"
- Drug details: identifier="CHEMBL25", dataset="chembl_molecule"
- Disease details: identifier="MONDO:0005148", dataset="mondo"
- Variant details: identifier="rs1799853", dataset="dbsnp"
- TF regulation: identifier="NR3C1:PTHLH", dataset="collectri"

RETURNS (examples):
- pubchem: pharmacological_actions (drug mechanism e.g. "Protein Synthesis Inhibitors")
- clinvar: germline_classification, review_status, conditions
- pharmgkb: level_of_evidence, clinical_annotations, guidelines
- alphamissense: am_pathogenicity score, am_class
- drugcentral: mechanism of action, target info, action_type
- bgee: expression_score, anatomical_entity, developmental_stage

DRUG DATA TIP:
- For drugs, check BOTH chembl_molecule AND pubchem entries
- chembl_molecule: target data, clinical development phase, assay details
- pubchem: pharmacological_actions (mechanism), FDA status, literature
- They complement each other - neither has complete information alone""",
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
- topic: Optional filter - "edges", "filters", "hierarchies", "patterns", "examples", "filter_syntax", "disease_ontology", "response_format", or "all" (default)
  - "filter_syntax": CRITICAL - explains .0 suffix for floats, no scientific notation, case-sensitive strings
  - "disease_ontology": CRITICAL - explains which ontology each database uses and how to use bridges/parent terms when direct mapping fails
  - "response_format": explains lite mode response structure (pipe-delimited data, schemas, pagination)""",
        inputSchema={
            "type": "object",
            "properties": {
                "topic": {
                    "type": "string",
                    "enum": ["edges", "filters", "hierarchies", "patterns", "examples", "filter_syntax", "disease_ontology", "response_format", "all"],
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
            "description": "Search biobtree for biological identifiers. Finds entries matching the given terms across 70+ integrated databases including genes (ensembl, hgnc), proteins (uniprot), drugs (chembl, drugcentral), diseases (mondo, efo), variants (dbsnp, clinvar), and more.",
            "parameters": {
                "type": "object",
                "properties": {
                    "terms": {
                        "type": "string",
                        "description": "Comma-separated identifiers to search (e.g., 'TP53', 'BRCA1,BRCA2', 'aspirin')"
                    },
                    "dataset": {
                        "type": "string",
                        "description": "Optional: Filter to dataset(s), comma-separated (e.g., 'ensembl,hgnc,uniprot' for genes, 'chembl_molecule,pubchem,drugcentral' for drugs)"
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
            "description": "Map identifiers through biobtree dataset chains. The core tool for cross-database queries. Maps identifiers from one database to another through intermediate datasets.",
            "parameters": {
                "type": "object",
                "properties": {
                    "terms": {
                        "type": "string",
                        "description": "Comma-separated identifiers to map (e.g., 'BRCA1', 'TP53,EGFR')"
                    },
                    "chain": {
                        "type": "string",
                        "description": "Mapping chain (e.g., '>>ensembl>>uniprot' for gene to protein, '>>chembl_molecule>>chembl_target>>uniprot>>hgnc' for drug to gene)"
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
            "description": "Get full entry details from biobtree. Retrieves complete information for a specific identifier in a dataset, including all attributes and cross-references.",
            "parameters": {
                "type": "object",
                "properties": {
                    "identifier": {
                        "type": "string",
                        "description": "The ID to look up (e.g., 'P04637', 'ENSG00000141510')"
                    },
                    "dataset": {
                        "type": "string",
                        "description": "The dataset containing the entry (e.g., 'uniprot', 'ensembl')"
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
            "description": "Get biobtree schema reference - dataset connections, filters, and query patterns. Use this to understand which datasets connect to which and how to build mapping chains.",
            "parameters": {
                "type": "object",
                "properties": {
                    "topic": {
                        "type": "string",
                        "enum": ["edges", "filters", "hierarchies", "patterns", "examples", "all"],
                        "description": "Which section of the schema to return"
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
