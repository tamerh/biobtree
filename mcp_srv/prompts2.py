"""
Biobtree Prompt Configuration v2 - MINIMAL & UNIFIED

All guidance lives in TOOL_DESCRIPTIONS. No separate schema hints.
Simple architecture: tool descriptions + minimal system prompt.

DESIGN PRINCIPLES:
1. Everything the model needs is in the tool it's using
2. No redundancy - each piece of info appears once
3. Compact but complete
"""

# =============================================================================
# EDGES - Full dataset connections
# =============================================================================

EDGES = """
EDGES (what connects to what):
ensembl: uniprot, go, transcript, exon, ortholog, paralog, hgnc, entrez, refseq, bgee, gwas, gencc, antibody, scxa
hgnc: ensembl, uniprot, entrez, gencc, pharmgkb_gene, msigdb, clinvar, mim, refseq, alphafold, collectri, gwas, dbsnp, hpo, cellphonedb
entrez: ensembl, uniprot, refseq, go, biogrid, pubchem_activity
refseq: ensembl, entrez, taxonomy, ccds, uniprot, mirdb
mirdb: refseq
transcript: ensembl, exon, ufeature
uniprot: ensembl, alphafold, interpro, pdb, ufeature, intact, string, biogrid, biogrid_interaction, chembl_target, go, reactome, rhea, swisslipids, bindingdb, antibody, pubchem_activity, cellphonedb, jaspar, signor
alphafold: uniprot
interpro: uniprot, go, interproparent, interprochild
chembl_molecule: chembl_activity, chembl_target, pubchem, chebi, clinical_trials
chembl_activity: chembl_molecule, chembl_assay, bao
chembl_assay: chembl_activity, chembl_target, chembl_document, bao
chembl_target: chembl_assay, uniprot, chembl_molecule
pubchem: chembl_molecule, chebi, hmdb, pubchem_activity, pubmed, patent_compound, bindingdb, ctd, pharmgkb
pubchem_activity: pubchem, ensembl, uniprot
chebi: pubchem, rhea, intact
swisslipids: uniprot, go, chebi, uberon, cl
lipidmaps: chebi, pubchem
dbsnp: hgnc, clinvar, pharmgkb_variant, alphamissense, spliceai
clinvar: hgnc, mondo, hpo, dbsnp, orphanet
alphamissense: uniprot, transcript
gwas: gwas_study, efo, dbsnp, hgnc
gwas_study: gwas, efo
mondo: gencc, clinvar, efo, mesh, hpo, clinical_trials, antibody, cellxgene, cellxgene_celltype, orphanet, mondoparent, mondochild
gencc: mondo, hpo, hgnc, ensembl
clinical_trials: mondo, chembl_molecule
pharmgkb: hgnc, dbsnp, mesh, pharmgkb_gene, pharmgkb_variant, pharmgkb_clinical, pharmgkb_guideline, pharmgkb_pathway
pharmgkb_variant: pharmgkb_clinical, hgnc, mesh, dbsnp
pharmgkb_gene: hgnc, entrez, ensembl, pharmgkb
pharmgkb_clinical: dbsnp, hgnc, mesh, pharmgkb_variant
pharmgkb_guideline: hgnc, pharmgkb
pharmgkb_pathway: hgnc, pharmgkb
ctd: mesh, entrez, efo, pubchem, taxonomy
intact: uniprot, chebi, rnacentral
string: uniprot
biogrid: entrez, uniprot, refseq, taxonomy
bgee: ensembl, uberon, cl, taxonomy
cellxgene: cl, uberon, mondo, efo, taxonomy
cellxgene_celltype: cl, uberon, mondo
scxa: cl, uberon, taxonomy, ensembl, scxa_gene_experiment
scxa_expression: ensembl, scxa, scxa_gene_experiment
scxa_gene_experiment: ensembl, scxa, scxa_expression, cl
rnacentral: uniprot, ensembl, intact
reactome: ensembl, uniprot, chebi, go, reactomeparent, reactomechild
rhea: chebi, uniprot, go
go: ensembl, uniprot, reactome, msigdb, swisslipids, bgee, interpro, goparent, gochild
hpo: clinvar, gencc, mondo, msigdb, orphanet, mim, hmdb, hgnc, hpoparent, hpochild
efo: gwas, mondo, cellxgene, efoparent, efochild
uberon: bgee, cellxgene, cellxgene_celltype, swisslipids, uberonparent, uberonchild
cl: bgee, cellxgene, cellxgene_celltype, scxa, scxa_gene_experiment, clparent, clchild
taxonomy: ensembl, uniprot, bgee, biogrid, ctd, taxparent, taxchild
mesh: pharmgkb, ctd, pubchem, mondo, meshparent, meshchild
eco: ecoparent, ecochild
antibody: ensembl, uniprot, mondo, pdb
msigdb: hgnc, entrez, go, hpo
orphanet: hpo, uniprot, mondo, hgnc, clinvar, mim, mesh
mim: clinvar, hpo, mondo, uniprot, ctd
hmdb: pubchem, hpo, chebi, uniprot
collectri: hgnc
esm2_similarity: uniprot
cellphonedb: uniprot, ensembl, hgnc, pubmed
spliceai: hgnc
pdb: uniprot, go, interpro, pfam, taxonomy, pubmed
fantom5_promoter: ensembl, hgnc, entrez, uniprot, uberon, cl
fantom5_enhancer: ensembl, uberon, cl
fantom5_gene: ensembl, hgnc, entrez
jaspar: uniprot, pubmed, taxonomy
encode_ccre: taxonomy
bao: chembl_activity, chembl_assay, baoparent, baochild
"""


# =============================================================================
# FILTERS - Full filter syntax
# =============================================================================

FILTERS = """
FILTER SYNTAX: >>dataset[dataset.field operator value]

OPERATORS:
  ==       equals           >>dataset[dataset.field=="value"]
  !=       not equals       >>dataset[dataset.field!="value"]
  >        greater than     >>dataset[dataset.field>value]
  <        less than        >>dataset[dataset.field<value]
  >=       greater or equal >>dataset[dataset.field>=value]
  <=       less or equal    >>dataset[dataset.field<=value]
  contains string match     >>dataset[dataset.field.contains("value")]

TYPE RULES:
  - FLOAT: use decimal point (70.0 not 70)
  - INT: no decimal (2 not 2.0)
  - STRING: quote values ("Pathogenic", "PHASE3")
  - BOOL: true/false (no quotes)

EXAMPLES:
  >>chembl_molecule[chembl_molecule.highestDevelopmentPhase==4]  # approved drugs
  >>chembl_molecule[chembl_molecule.highestDevelopmentPhase>=3]  # Phase 3+
  >>clinical_trials[clinical_trials.phase=="PHASE3"]
  >>go[go.type=="biological_process"]
  >>clinvar[clinvar.clinicalSignificance=="Pathogenic"]
  >>reactome[reactome.name.contains("signaling")]
"""


# =============================================================================
# TOOL DESCRIPTIONS - Each tool as separate variable
# =============================================================================

DESC_SEARCH = """Search 70+ biological databases.

SYNTAX: biobtree_search(terms="entity")

WORKFLOW:
1. Search WITHOUT dataset filter first (discover where entity exists)
2. Use IDs from results with biobtree_map

QUERY PATTERNS (choose based on question):

"DRUG FOR DISEASE X":
- Search disease → mondo → clinical_trials → chembl_molecule
- OR search drug class directly (e.g., "antipyretic", "NSAID", "antibiotic")
- Verify mechanism for top 2-3 drugs only (don't enumerate all proteins!)

"DRUG TARGETS" (use BOTH paths for complete picture):
- chembl: >>chembl_molecule>>chembl_target>>uniprot (mechanism-level)
- pubchem: >>pubchem>>pubchem_activity>>uniprot (protein-level, often 50+ targets)
- Filter approved: >>chembl_molecule[chembl_molecule.highestDevelopmentPhase==4]

"DISEASE GENES":
- Search disease → mondo/hpo → gencc/clinvar/orphanet → hgnc

"PROTEIN FUNCTION":
- Search protein → uniprot → go/reactome

RETURNS: id | dataset | name | xref_count"""


DESC_MAP_BASE = """Map identifiers between databases.

SYNTAX: biobtree_map(terms="ID", chain=">>source>>target")
- Chain MUST start with ">>"
- Source MUST match input ID type

ID TYPE → SOURCE:
- ENSG* → >>ensembl
- P*/Q*/O* → >>uniprot
- CHEMBL* → >>chembl_molecule
- GO:* → >>go
- MONDO:* → >>mondo
- HP:* → >>hpo
- HGNC:* or gene symbols → >>hgnc

DRUG TARGET PATTERNS (use BOTH for complete picture):
- >>chembl_molecule>>chembl_target>>uniprot (mechanism-level)
- >>pubchem>>pubchem_activity>>uniprot (protein-level, often 50+ targets)

DISEASE GENE PATTERNS:
- >>mondo>>gencc>>hgnc (curated)
- >>mondo>>clinvar>>hgnc (variant-based)

RETURNS: mapped identifiers with dataset and name
"""

# Concatenate: description + edges + filters
DESC_MAP = DESC_MAP_BASE + EDGES + FILTERS


DESC_ENTRY = """Get full details for one identifier.

SYNTAX: biobtree_entry(identifier="ID", dataset="dataset_name")

USE FOR:
- See all attributes of an entry
- Discover filterable fields
- Get detailed info (sequences, scores, descriptions)

RETURNS: All attributes + xref counts to connected datasets"""


DESC_META = """Get list of all available datasets.

SYNTAX: biobtree_meta()

RETURNS: Dataset names, entry counts, relationships"""


# =============================================================================
# TOOL_DESCRIPTIONS dict - references the variables above
# =============================================================================

TOOL_DESCRIPTIONS = {
    "biobtree_search": DESC_SEARCH,
    "biobtree_map": DESC_MAP,
    "biobtree_entry": DESC_ENTRY,
    "biobtree_meta": DESC_META,
}


# =============================================================================
# SYSTEM PROMPT - Minimal
# =============================================================================

SYSTEM_PROMPT = """You are a bioinformatics assistant with access to biobtree, a database integrating 70+ biological data sources.

Use the tools to search, map, and retrieve biological data. Tool descriptions contain all the guidance you need."""


# =============================================================================
# INPUT SCHEMAS - For tool parameters
# =============================================================================

INPUT_SCHEMAS = {
    "biobtree_search": {
        "type": "object",
        "properties": {
            "terms": {"type": "string", "description": "Comma-separated identifiers to search"},
            "dataset": {"type": "string", "description": "Filter to specific dataset (omit for discovery)"},
            "mode": {"type": "string", "enum": ["lite", "full"], "default": "lite"},
            "page": {"type": "string", "description": "Pagination token"}
        },
        "required": ["terms"]
    },
    "biobtree_map": {
        "type": "object",
        "properties": {
            "terms": {"type": "string", "description": "Comma-separated identifiers to map"},
            "chain": {"type": "string", "description": "Mapping chain (e.g., >>ensembl>>uniprot)"},
            "mode": {"type": "string", "enum": ["lite", "full"], "default": "lite"},
            "page": {"type": "string", "description": "Pagination token"}
        },
        "required": ["terms", "chain"]
    },
    "biobtree_entry": {
        "type": "object",
        "properties": {
            "identifier": {"type": "string", "description": "The identifier to look up"},
            "dataset": {"type": "string", "description": "The dataset containing the entry"}
        },
        "required": ["identifier", "dataset"]
    },
    "biobtree_meta": {
        "type": "object",
        "properties": {}
    }
}


# =============================================================================
# SCHEMA API - For REST API /api/help endpoint (not LLM tool)
# =============================================================================

def get_schema(topic: str = "all") -> dict:
    """Called by /api/help endpoint for human users."""
    if topic == "edges":
        return {"edges": EDGES}
    elif topic == "filters":
        return {"filters": FILTERS}
    else:
        return {"edges": EDGES, "filters": FILTERS}
