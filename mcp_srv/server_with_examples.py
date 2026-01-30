"""
Biobtree MCP Server - VERSION WITH FULL EXAMPLES

This is the backup version with extensive examples in biobtree_map description.
Use this if the minimal version (server.py) doesn't work well.

Usage:
    python -m mcp_srv.server_with_examples
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
>> dataset1 >> dataset2 >> dataset3

First >> is lookup (finds ID in dataset), subsequent >> are cross-reference mappings.

PARAMETERS:
- terms: Comma-separated identifiers (required)
- chain: Mapping chain like ">>ensembl>>uniprot" (required)
- mode: "lite" or "full" - default "lite"

BASIC EXAMPLES (TESTED & WORKING):
- Gene to protein: terms="BRCA1", chain=">>ensembl>>uniprot"
- Gene to reviewed protein: terms="BRCA1", chain=">>ensembl>>uniprot[uniprot.reviewed==true]"
- Gene to human reviewed protein: terms="BRCA1", chain=">>ensembl[ensembl.genome==\"homo_sapiens\"]>>uniprot[uniprot.reviewed==true]"
- Protein to pathways: terms="P04637", chain=">>uniprot>>reactome"
- Gene to GO biological process: terms="TP53", chain=">>ensembl>>uniprot>>go[go.type==\"biological_process\"]"
- Gene to transcripts: terms="TP53", chain=">>ensembl>>transcript"
- Gene to orthologs: terms="BRCA1", chain=">>ensembl>>ortholog"
- Human to mouse orthologs: terms="TP53", chain=">>ensembl[ensembl.genome==\"homo_sapiens\"]>>ortholog[ensembl.genome==\"mus_musculus\"]"

PROTEIN STRUCTURES (ALPHAFOLD):
- Gene to AlphaFold: terms="BRCA1", chain=">>ensembl[ensembl.genome==\"homo_sapiens\"]>>uniprot[uniprot.reviewed==true]>>alphafold"
- Protein to AlphaFold: terms="P38398", chain=">>uniprot>>alphafold"
- AlphaFold lookup: terms="AF-P38398-F1", chain=">>alphafold"

BIOGRID INTERACTIONS:
- Gene to BioGRID: terms="TP53", chain=">>ensembl[ensembl.genome==\"homo_sapiens\"]>>entrez>>biogrid"
- Entrez to BioGRID: terms="7157", chain=">>entrez>>biogrid"
- BioGRID to UniProt: terms="7157", chain=">>entrez>>biogrid>>uniprot"
- UniProt to BioGRID: terms="P04637", chain=">>uniprot>>biogrid"
- BioGRID lookup: terms="107140", chain=">>biogrid"

DBSNP VARIANTS:
- Gene to dbSNP: terms="BRCA1", chain=">>ensembl[ensembl.genome==\"homo_sapiens\"]>>dbsnp"
- Ensembl to dbSNP: terms="ENSG00000012048", chain=">>ensembl>>dbsnp"
- dbSNP lookup: terms="rs1801133", chain=">>dbsnp"
- dbSNP to ClinVar: terms="rs1801133", chain=">>dbsnp>>ensembl>>clinvar"

DRUG DISCOVERY (FULL CHEMBL CHAIN):
- Gene to ChEMBL targets:
  terms="EGFR", chain=">>ensembl>>uniprot>>chembl_target_component>>chembl_target"

- Gene to drugs (full chain):
  terms="JAK2", chain=">>ensembl>>uniprot>>chembl_target_component>>chembl_target>>chembl_assay>>chembl_activity>>chembl_molecule"

- Advanced phase drugs only:
  terms="JAK2", chain=">>ensembl>>uniprot>>chembl_target_component>>chembl_target>>chembl_assay>>chembl_activity>>chembl_molecule[chembl.molecule.highestDevelopmentPhase>2]"

DRUG DISCOVERY WORKFLOWS (via GenCC):
- Disease to AlphaFold structures:
  terms="glioblastoma", chain=">>mondo>>gencc>>ensembl[ensembl.genome==\"homo_sapiens\"]>>uniprot[uniprot.reviewed==true]>>alphafold"

- Disease to BioGRID interactions:
  terms="glioblastoma", chain=">>mondo>>gencc>>ensembl[ensembl.genome==\"homo_sapiens\"]>>entrez>>biogrid"

- Disease to dbSNP variants:
  terms="breast cancer", chain=">>mondo>>gencc>>ensembl[ensembl.genome==\"homo_sapiens\"]>>dbsnp"

- Disease to IntAct interactions:
  terms="glioblastoma", chain=">>mondo>>gencc>>ensembl[ensembl.genome==\"homo_sapiens\"]>>uniprot[uniprot.reviewed==true]>>intact"

GWAS GENETICS:
- Disease to GWAS SNPs via EFO: terms="EFO:0000400", chain=">>efo>>gwas"
- Disease to GWAS genes: terms="MONDO:0005148", chain=">>mondo>>efo>>gwas>>ensembl[ensembl.genome==\"homo_sapiens\"]"
- GWAS study to SNPs: terms="GCST010481", chain=">>gwas_study>>gwas"

CLINICAL VARIANTS:
- Gene to ClinVar variants: terms="BRCA1", chain=">>ensembl>>clinvar"
- Disease to ClinVar variants: terms="MONDO:0015628", chain=">>mondo>>clinvar"
- Disease to ClinVar genes: terms="MONDO:0005148", chain=">>mondo>>clinvar>>ensembl[ensembl.genome==\"homo_sapiens\"]"
- ClinVar to disease ontology: terms="100177", chain=">>clinvar>>mondo"
- ClinVar to HPO phenotypes: terms="981341", chain=">>clinvar>>hpo"

VARIANT ANALYSIS:
- SNP lookup: terms="rs1799853", chain=">>dbsnp"
- SNP to pharmacogenomics: terms="rs1799853", chain=">>pharmgkb_variant>>pharmgkb_clinical"

PROTEIN INTERACTIONS:
- Protein to IntAct interactions: terms="P38398", chain=">>uniprot>>intact"
- Protein to interaction partners: terms="P38398", chain=">>uniprot>>intact>>uniprot"
- Gene to STRING network: terms="BRCA1", chain=">>ensembl>>uniprot>>string"
- ChEBI to protein targets: terms="CHEBI:50210", chain=">>chebi>>intact>>uniprot"
- RNA to protein binding: terms="URS00002D9DEC", chain=">>rnacentral>>intact>>uniprot"

EXPRESSION:
- Gene to tissue expression: terms="ENSG00000139618", chain=">>ensembl>>bgee"
- Gene to tissues where expressed: terms="ENSG00000139618", chain=">>ensembl>>bgee>>uberon"
- Tissue to expressed genes: terms="UBERON:0000955", chain=">>uberon>>bgee"
- Tissue to genes to functions: terms="UBERON:0000955", chain=">>uberon>>bgee>>ensembl>>go"

PATHWAYS:
- Protein to Reactome: terms="P00533", chain=">>uniprot>>reactome"
- Reactome pathway parents: terms="R-HSA-75205", chain=">>reactome>>reactomeparent"
- Reactome to proteins: terms="R-HSA-109582", chain=">>reactome>>ensembl>>uniprot"
- ChEBI to Rhea reactions: terms="CHEBI:15377", chain=">>chebi>>rhea"

ANTIBODIES:
- Antibody lookup: terms="BEVACIZUMAB", chain=">>antibody"
- Antibody to target genes: terms="BEVACIZUMAB", chain=">>antibody>>ensembl"
- Gene to antibodies: terms="VEGFA", chain=">>ensembl>>antibody"
- PDB to antibodies: terms="7S4S", chain=">>pdb>>antibody"
- Antibody to diseases: terms="BEVACIZUMAB", chain=">>antibody>>mondo"
- Disease to antibodies: terms="MONDO:0005015", chain=">>mondo>>antibody"

CLINICAL TRIALS:
- Trial lookup: terms="NCT00720356", chain=">>clinical_trials"
- Disease text to trials: terms="diabetes", chain=">>clinical_trials"
- Trial to disease ontology: terms="NCT06777108", chain=">>clinical_trials>>mondo"
- Disease to trials: terms="MONDO:0005044", chain=">>mondo>>clinical_trials"
- Trial to gene targets: terms="NCT05969704", chain=">>clinical_trials>>mondo>>gencc>>ensembl"

PUBCHEM / NCBI:
- PubChem to bioactivity: terms="2244", chain=">>pubchem>>pubchem_activity"
- PubChem to gene targets: terms="2244", chain=">>pubchem>>pubchem_activity>>ensembl"
- PubChem to HMDB: terms="2244", chain=">>pubchem>>hmdb"
- PubChem to ChEMBL: terms="2244", chain=">>pubchem>>chembl_molecule"
- Entrez to Ensembl: terms="672", chain=">>entrez>>ensembl"
- Entrez to GO: terms="BRCA1", chain=">>entrez>>go"
- Entrez to RefSeq: terms="BRCA1", chain=">>entrez>>refseq"

ONTOLOGY NAVIGATION:
- GO term parents: terms="GO:0004707", chain=">>go>>goparent"
- EFO disease children: terms="EFO:0003767", chain=">>efo>>efochild"
- UBERON anatomy parents: terms="UBERON:0000955", chain=">>uberon>>uberonparent"
- Cell Ontology children: terms="CL:0000576", chain=">>cl>>clchild"

FILTERS (append to any dataset):
- Reviewed proteins: >>uniprot[uniprot.reviewed==true]
- Human genes only: >>ensembl[ensembl.genome=="homo_sapiens"]
- Biological process GO: >>go[go.type=="biological_process"]
- Phase 3+ drugs: >>chembl_molecule[chembl.molecule.highestDevelopmentPhase>=3]
- Pathogenic variants: >>clinvar[clinvar.clinical_significance=="Pathogenic"]""",
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
                mode=arguments.get("mode", "lite")
            )

        elif name == "biobtree_map":
            result = await client.map(
                terms=arguments["terms"],
                chain=arguments["chain"],
                mode=arguments.get("mode", "lite")
            )

        elif name == "biobtree_entry":
            result = await client.entry(
                identifier=arguments["identifier"],
                dataset=arguments["dataset"]
            )

        elif name == "biobtree_meta":
            result = await client.meta()

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

    logger.info("Starting Biobtree MCP Server (with examples)")

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
