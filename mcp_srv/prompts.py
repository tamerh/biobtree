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
chembl_molecule: mesh, chembl_activity, chembl_target, pubchem, chebi, clinical_trials
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
string: uniprot, string_interaction
string_interaction: string, uniprot
biogrid: entrez, uniprot, refseq, taxonomy
bgee: ensembl, uberon, cl, taxonomy
cellxgene: cl, uberon, mondo, efo, taxonomy
cellxgene_celltype: cl, uberon, mondo
scxa: cl, uberon, taxonomy, ensembl, scxa_gene_experiment
scxa_expression: ensembl, scxa, scxa_gene_experiment
scxa_gene_experiment: ensembl, scxa, scxa_expression, cl
rnacentral: uniprot, ensembl, intact, hgnc, refseq, ena
reactome: ensembl, uniprot, chebi, go, reactomeparent, reactomechild
rhea: chebi, uniprot, go
go: ensembl, uniprot, reactome, msigdb, swisslipids, bgee, interpro, goparent, gochild
hpo: clinvar, gencc, mondo, msigdb, orphanet, mim, hmdb, hgnc, hpoparent, hpochild
efo: gwas, mondo, cellxgene, efoparent, efochild
uberon: bgee, cellxgene, cellxgene_celltype, swisslipids, uberonparent, uberonchild
cl: bgee, cellxgene, cellxgene_celltype, scxa, scxa_gene_experiment, clparent, clchild
taxonomy: ensembl, uniprot, bgee, biogrid, ctd, taxparent, taxchild
mesh: pharmgkb, ctd, pubchem, mondo, chembl_molecule, meshparent, meshchild
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
FILTER SYNTAX: >>dataset[field operator value]

OPERATORS:
  ==       equals           >>dataset[field=="value"]
  !=       not equals       >>dataset[field!="value"]
  >        greater than     >>dataset[field>value]
  <        less than        >>dataset[field<value]
  >=       greater or equal >>dataset[field>=value]
  <=       less or equal    >>dataset[field<=value]
  contains string match     >>dataset[field.contains("value")]

LOGICAL OPERATORS:
  &&       AND              >>dataset[field1>5 && field2<10]
  ||       OR               >>dataset[field=="A" || field=="B"]
  !        NOT              >>dataset[!field] or >>dataset[!(field=="value")]

TYPE RULES:
  - FLOAT: use decimal point (70.0 not 70)
  - INT: no decimal (2 not 2.0)
  - STRING: quote values ("Pathogenic", "PHASE3")
  - BOOL: true/false (no quotes)

EXAMPLES:
  >>chembl_molecule[highestDevelopmentPhase==4]  # approved drugs
  >>chembl_molecule[highestDevelopmentPhase>=3]  # Phase 3+
  >>clinical_trials[phase=="PHASE3"]
  >>go[type=="biological_process"]
  >>clinvar[germline_classification=="Pathogenic"]
  >>reactome[name.contains("signaling")]
"""


# =============================================================================
# TOOL DESCRIPTIONS - Each tool as separate variable
# =============================================================================

DESC_SEARCH = """Search 70+ biological databases.

SYNTAX: biobtree_search(terms="entity")

BEFORE SEARCHING - Use your training knowledge to plan:
1. What type of entity is this? (disease, process, drug, gene, protein)
2. What is the query asking for? (drugs, genes, function, etc.)
3. What equivalent terms might give better results?
   (e.g., "temperature homeostasis" is a process → related condition is "fever")
4. Choose best entry point for query type (disease terms for drug queries)

WORKFLOW:
1. Search WITHOUT dataset filter first (discover where entity exists)
2. Use IDs from results with biobtree_map

QUERY PATTERNS (choose based on question):

"DRUG FOR DISEASE/CONDITION X":
- Prefer disease terms (mesh/mondo/efo) over GO terms for drug queries
- If search only returns GO term, search for the related CONDITION instead
  (e.g., "temperature homeostasis" → search "fever" instead)
- Search disease → mondo → clinical_trials → chembl_molecule
- OR search drug class directly (e.g., "antipyretic", "NSAID", "antibiotic")
- Verify mechanism for top 2-3 drugs only (don't enumerate all proteins!)

"DRUG TARGETS" (use BOTH paths for complete picture):
- chembl: >>chembl_molecule>>chembl_target>>uniprot (mechanism-level)
- pubchem: >>pubchem>>pubchem_activity>>uniprot (protein-level, often 50+ targets)
- Filter approved: >>chembl_molecule[highestDevelopmentPhase==4]

"DISEASE GENES":
- Search disease → mondo/hpo → gencc/clinvar/orphanet → hgnc

"PROTEIN FUNCTION":
- Search protein → uniprot → go/reactome

"MECHANISM QUERIES" (drug-disease):
- Use biobtree_entry to see what's connected (xrefs)
- Check EDGES to see where each xref leads
- Follow connections relevant to your question
- Build chain: Drug → Target → [connections] → Disease

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

SOME DRUG EXPLORATION PATHS:
- >>chembl_molecule>>chembl_target>>uniprot (drug targets)
- >>pubchem>>pubchem_activity>>uniprot (bioactivity)
- >>ensembl>>reactome>>chebi (pathway chemicals - when no direct targets)
- Discover more via entry xrefs + EDGES

WARNING - GO terms with high xref_count (>100):
- Don't map GO → proteins → drugs (too many results)
- Instead: search drug class for condition → verify targets this GO term

DISEASE GENE PATTERNS:
- >>mondo>>gencc>>hgnc (curated)
- >>mondo>>clinvar>>hgnc (variant-based)

DISEASE → DRUG PATTERNS:
- >>mesh>>chembl_molecule (MeSH disease/condition → drugs with indications)
- >>mondo>>clinical_trials>>chembl_molecule (disease → trial drugs)

DISCOVERY APPROACH:
- Use biobtree_entry to see xrefs (what's connected)
- Use EDGES above to see where each dataset leads
- Build chains based on what connections exist for YOUR entity

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
- **DISCOVER CONNECTIONS**: xrefs show what datasets link to this entry

WORKFLOW: Get entry → see xrefs → check EDGES for where they lead → follow relevant paths

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
