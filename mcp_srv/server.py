"""
Biobtree MCP Server

MCP server exposing biobtree functionality to Claude and other MCP clients.
Provides 4 base tools for searching, mapping, and exploring biological data.

Usage:
    python -m mcp_srv.server

Or add to Claude Desktop config:
    {
        "mcpServers": {
            "biobtree": {
                "command": "python",
                "args": ["-m", "mcp_srv.server"],
                "cwd": "/data/bioyoda/biobtreev2"
            }
        }
    }
"""

import asyncio
import json
import logging
from typing import Any

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import Tool, TextContent

from .biobtree_client import BiobtreeClient

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# =============================================================================
# Schema Data for biobtree_help tool
# =============================================================================

SCHEMA_EDGES = {
    "ensembl": ["uniprot", "go", "transcript", "exon", "ortholog", "paralog", "dbsnp", "clinvar", "hgnc", "entrez", "refseq", "bgee", "gwas", "gencc", "biogrid", "string", "antibody"],
    "hgnc": ["ensembl", "uniprot", "entrez", "gencc", "pharmgkb_gene", "msigdb"],
    "entrez": ["ensembl", "uniprot", "refseq", "go", "biogrid", "pubchem_activity"],
    "refseq": ["ensembl", "uniprot", "entrez"],
    "transcript": ["ensembl", "exon", "ufeature"],
    "uniprot": ["ensembl", "alphafold", "interpro", "pdb", "ufeature", "intact", "string", "biogrid", "chembl_target_component", "go", "reactome", "rhea", "swisslipids", "bindingdb", "antibody"],
    "alphafold": ["uniprot"],
    "interpro": ["uniprot"],
    "chembl_molecule": ["chembl_activity", "pubchem", "chebi", "drugcentral", "clinical_trials"],
    "chembl_activity": ["chembl_molecule", "chembl_assay"],
    "chembl_assay": ["chembl_activity", "chembl_target", "chembl_document"],
    "chembl_target": ["chembl_assay", "chembl_target_component"],
    "chembl_target_component": ["chembl_target", "uniprot"],
    "pubchem": ["chembl_molecule", "chebi", "hmdb", "pubchem_activity"],
    "pubchem_activity": ["pubchem", "ensembl", "uniprot"],
    "chebi": ["pubchem", "chembl_molecule", "rhea", "intact"],
    "drugcentral": ["chembl_molecule", "uniprot"],
    "swisslipids": ["uniprot", "go", "chebi", "uberon"],
    "lipidmaps": ["chebi", "pubchem"],
    "dbsnp": ["ensembl", "hgnc", "clinvar", "pharmgkb_variant"],
    "clinvar": ["ensembl", "hgnc", "mondo", "hpo", "dbsnp"],
    "alphamissense": ["uniprot", "transcript"],
    "gwas": ["gwas_study", "ensembl", "efo", "dbsnp"],
    "gwas_study": ["gwas", "efo"],
    "mondo": ["gencc", "clinvar", "efo", "clinical_trials", "antibody", "cellxgene", "ctd"],
    "gencc": ["mondo", "hpo", "hgnc", "ensembl"],
    "clinical_trials": ["mondo", "chembl_molecule"],
    "pharmgkb": ["hgnc", "dbsnp", "mesh", "pharmgkb_gene", "pharmgkb_variant", "pharmgkb_clinical", "pharmgkb_guideline", "pharmgkb_pathway"],
    "ctd": ["mesh", "entrez", "mondo", "efo", "pubchem", "taxonomy"],
    "intact": ["uniprot", "chebi", "rnacentral"],
    "string": ["uniprot", "ensembl"],
    "biogrid": ["entrez", "uniprot", "refseq", "taxonomy"],
    "bgee": ["ensembl", "uberon", "cl", "taxonomy"],
    "cellxgene": ["cl", "uberon", "mondo", "efo", "taxonomy"],
    "scxa": ["cl", "uberon", "taxonomy", "ensembl"],
    "rnacentral": ["uniprot", "ensembl", "intact"],
    "reactome": ["ensembl", "uniprot", "chebi", "go"],
    "rhea": ["chebi", "uniprot", "go"],
    "go": ["ensembl", "uniprot", "reactome", "msigdb", "swisslipids", "bgee"],
    "hpo": ["clinvar", "gencc", "msigdb"],
    "efo": ["gwas", "mondo", "cellxgene"],
    "uberon": ["bgee", "cellxgene", "swisslipids"],
    "cl": ["bgee", "cellxgene", "scxa"],
    "taxonomy": ["ensembl", "uniprot", "bgee", "biogrid", "ctd"],
    "mesh": ["pharmgkb", "ctd", "pubchem"],
    "antibody": ["ensembl", "uniprot", "mondo", "pdb"],
    "msigdb": ["hgnc", "entrez", "go", "hpo"]
}

SCHEMA_HIERARCHIES = {
    "go": ["goparent", "gochild"],
    "mondo": ["mondoparent", "mondochild"],
    "hpo": ["hpoparent", "hpochild"],
    "efo": ["efoparent", "efochild"],
    "uberon": ["uberonparent", "uberonchild"],
    "cl": ["clparent", "clchild"],
    "taxonomy": ["taxparent", "taxchild"],
    "reactome": ["reactomeparent", "reactomechild"],
    "mesh": ["meshparent", "meshchild"],
    "eco": ["ecoparent", "ecochild"]
}

SCHEMA_FILTERS = {
    "ensembl": {"genome": "str (homo_sapiens|mus_musculus)", "biotype": "str", "name": "str", "start": "int", "end": "int"},
    "uniprot": {"reviewed": "bool (true=Swiss-Prot)"},
    "alphafold": {"global_metric": "float", "mean_pae": "float"},
    "interpro": {"type": "str (Domain|Family|Repeat)"},
    "ufeature": {"type": "str (Mutagenesis|Variant|Domain)"},
    "chembl_molecule": {"highestDevelopmentPhase": "int 0-4", "type": "str", "weight": "float"},
    "chembl_activity": {"standardType": "str (IC50|Ki|Kd)", "pChembl": "float"},
    "chembl_target": {"type": "str (SINGLE PROTEIN|PROTEIN COMPLEX)"},
    "pubchem": {"is_fda_approved": "bool", "molecular_weight": "float"},
    "dbsnp": {"allele_frequency": "float", "clinical_significance": "str", "is_common": "bool"},
    "clinvar": {"germline_classification": "str (Pathogenic|Benign)", "review_status": "str"},
    "alphamissense": {"am_class": "str (likely_pathogenic|ambiguous|likely_benign)", "am_pathogenicity": "float 0-1"},
    "gwas": {"p_value": "float", "pvalue_mlog": "float"},
    "gencc": {"classification_title": "str (Definitive|Strong|Moderate|Limited)", "moi_title": "str"},
    "pharmgkb_clinical": {"level_of_evidence": "str (1A|1B|2A|2B|3|4)"},
    "pharmgkb_guideline": {"source": "str (CPIC|DPWG)"},
    "clinical_trials": {"phase": "str", "overall_status": "str"},
    "bindingdb": {"ki": "str", "ic50": "str"},
    "intact": {"confidence_score": "float", "detection_method": "str"},
    "string": {"interactions[].score": "int 0-1000", "interactions[].has_experimental": "bool"},
    "biogrid": {"interaction_count": "int"},
    "bgee": {"expression_score": "float", "call_quality": "str (gold quality)"},
    "reactome": {"is_disease_pathway": "bool"},
    "go": {"type": "str (biological_process|molecular_function|cellular_component)"},
    "msigdb": {"collection": "str (H|C1-C8)", "gene_count": "int"},
    "antibody": {"clinical_stage": "str", "antibody_type": "str"}
}

SCHEMA_EXAMPLES = {
    "ensembl": "ENSG00000141510 (TP53)",
    "uniprot": "P04637 (p53)",
    "chembl_molecule": "CHEMBL25 (aspirin)",
    "chembl_target": "CHEMBL203 (EGFR)",
    "pubchem": "2244",
    "dbsnp": "rs1799853",
    "clinvar": "100177",
    "mondo": "MONDO:0005148 (diabetes)",
    "hpo": "HP:0001250",
    "go": "GO:0006915 (apoptosis)",
    "efo": "EFO:0000400",
    "uberon": "UBERON:0000955 (brain)",
    "cl": "CL:0000540 (neuron)",
    "taxonomy": "9606 (human)",
    "reactome": "R-HSA-109582",
    "gwas_study": "GCST010481",
    "clinical_trials": "NCT00720356",
    "antibody": "BEVACIZUMAB",
    "string": "9606.ENSP00000269305"
}

SCHEMA_PATTERNS = """# Gene → Drugs (full chain)
<gene> >> ensembl >> uniprot >> chembl_target_component >> chembl_target >> chembl_assay >> chembl_activity >> chembl_molecule

# Gene → Approved drugs only
<gene> >> ensembl >> uniprot >> chembl_target_component >> chembl_target >> chembl_assay >> chembl_activity >> chembl_molecule[chembl.molecule.highestDevelopmentPhase>2]

# Disease → Structures
<disease> >> mondo >> gencc >> ensembl[ensembl.genome=="homo_sapiens"] >> uniprot[uniprot.reviewed==true] >> alphafold

# Gene → Pathogenic variants
<gene> >> ensembl >> clinvar[clinvar.germline_classification=="Pathogenic"]
<gene> >> ensembl >> uniprot >> alphamissense[alphamissense.am_class=="likely_pathogenic"]

# Gene → Interactions
<gene> >> ensembl >> uniprot >> intact
<gene> >> ensembl >> entrez >> biogrid

# SNP → Clinical significance
<rsid> >> dbsnp >> clinvar >> mondo
<rsid> >> pharmgkb_variant >> pharmgkb_clinical

# Ontology navigation
<term> >> go >> goparent
<term> >> mondo >> mondochild

# Disease → All resources
<disease> >> mondo >> gencc >> ensembl      # causative genes
<disease> >> mondo >> clinvar >> dbsnp      # pathogenic variants
<disease> >> mondo >> clinical_trials       # active trials
<disease> >> mondo >> antibody              # therapeutic antibodies"""

SCHEMA_TEXT_SEARCH = """Datasets supporting partial text search:
- mondo, hpo, efo: disease/phenotype names ("alzheimer", "breast cancer")
- chembl_molecule, pharmgkb, bindingdb: drug names ("warfarin", "aspirin")
- clinical_trials: conditions, interventions
- antibody: antibody names ("bevacizumab")"""

SCHEMA_PAGINATION = {
    "description": "Results are automatically paginated (~150 results per page)",
    "response_fields": {
        "has_next": "boolean indicating more results available",
        "next_token": "token to pass for next page of results"
    },
    "usage": "When has_next is true, make another request with page=next_token to get more results"
}

# Initialize MCP server
server = Server("biobtree")

# Biobtree client (initialized on startup)
client: BiobtreeClient = None


# =============================================================================
# Tool Definitions
# =============================================================================

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

Returns a compact JSON schema with all dataset relationships and queryable attributes.

PARAMETERS:
- topic: Optional filter - "edges", "filters", "hierarchies", "patterns", "examples", or "all" (default)""",
        inputSchema={
            "type": "object",
            "properties": {
                "topic": {
                    "type": "string",
                    "enum": ["edges", "filters", "hierarchies", "patterns", "examples", "all"],
                    "default": "all",
                    "description": "Which section of the schema to return"
                }
            }
        }
    )
]


# =============================================================================
# Tool Handlers
# =============================================================================

@server.list_tools()
async def list_tools() -> list[Tool]:
    """Return list of available tools."""
    return TOOLS


@server.call_tool()
async def call_tool(name: str, arguments: dict[str, Any]) -> list[TextContent]:
    """Handle tool calls."""
    global client

    if client is None:
        client = BiobtreeClient()

    try:
        if name == "biobtree_search":
            result = await client.search(
                terms=arguments["terms"],
                dataset=arguments.get("dataset"),
                mode=arguments.get("mode", "lite"),
                page=arguments.get("page")
            )

        elif name == "biobtree_map":
            result = await client.map(
                terms=arguments["terms"],
                chain=arguments["chain"],
                mode=arguments.get("mode", "lite"),
                page=arguments.get("page")
            )

        elif name == "biobtree_entry":
            result = await client.entry(
                identifier=arguments["identifier"],
                dataset=arguments["dataset"]
            )

        elif name == "biobtree_meta":
            result = await client.meta()

        elif name == "biobtree_help":
            topic = arguments.get("topic", "all")

            if topic == "edges":
                result = {"edges": SCHEMA_EDGES}
            elif topic == "filters":
                result = {"filters": SCHEMA_FILTERS}
            elif topic == "hierarchies":
                result = {"hierarchies": SCHEMA_HIERARCHIES, "note": "Use dataset>>parent or dataset>>child for navigation"}
            elif topic == "patterns":
                result = {"patterns": SCHEMA_PATTERNS, "text_search": SCHEMA_TEXT_SEARCH}
            elif topic == "examples":
                result = {"examples": SCHEMA_EXAMPLES}
            else:  # "all"
                result = {
                    "query_syntax": "<terms> >> <dataset>[<filter>] >> <dataset>[<filter>] >> ...",
                    "edges": SCHEMA_EDGES,
                    "hierarchies": SCHEMA_HIERARCHIES,
                    "filters": SCHEMA_FILTERS,
                    "examples": SCHEMA_EXAMPLES,
                    "patterns": SCHEMA_PATTERNS,
                    "text_search": SCHEMA_TEXT_SEARCH,
                    "pagination": SCHEMA_PAGINATION
                }

        else:
            return [TextContent(
                type="text",
                text=json.dumps({"error": f"Unknown tool: {name}"})
            )]

        return [TextContent(
            type="text",
            text=json.dumps(result, indent=2)
        )]

    except Exception as e:
        logger.error(f"Tool {name} failed: {e}")
        return [TextContent(
            type="text",
            text=json.dumps({"error": str(e)})
        )]


# =============================================================================
# Server Lifecycle
# =============================================================================

async def main():
    """Run the MCP server."""
    global client

    logger.info("Starting Biobtree MCP Server")

    # Initialize client
    client = BiobtreeClient()

    # Check biobtree connection
    if await client.health_check():
        logger.info("Connected to biobtree at http://localhost:9291")
    else:
        logger.warning("Biobtree not running at http://localhost:9291 - tools will fail until started")

    # Run server
    async with stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream,
            write_stream,
            server.create_initialization_options()
        )


if __name__ == "__main__":
    asyncio.run(main())
