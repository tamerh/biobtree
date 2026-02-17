"""
Biobtree Schema Data

Schema definitions for biobtree_help tool - dataset connections, filters, and query patterns.
"""

# =============================================================================
# Dataset Edges (connections between datasets)
# =============================================================================

SCHEMA_EDGES = {
    "ensembl": ["uniprot", "go", "transcript", "exon", "ortholog", "paralog", "hgnc", "entrez", "refseq", "bgee", "gwas", "gencc", "antibody", "scxa"],
    "hgnc": ["ensembl", "uniprot", "entrez", "gencc", "pharmgkb_gene", "msigdb", "clinvar", "mim", "refseq", "alphafold", "collectri", "gwas", "dbsnp", "hpo", "cellphonedb"],
    "entrez": ["ensembl", "uniprot", "refseq", "go", "biogrid", "pubchem_activity"],
    "refseq": ["ensembl", "entrez", "taxonomy", "ccds", "uniprot", "mirdb"],
    "mirdb": ["refseq"],
    "transcript": ["ensembl", "exon", "ufeature"],
    "uniprot": ["ensembl", "alphafold", "interpro", "pdb", "ufeature", "intact", "string", "biogrid", "chembl_target", "go", "reactome", "rhea", "swisslipids", "bindingdb", "antibody", "pubchem_activity", "cellphonedb", "jaspar"],
    "alphafold": ["uniprot"],
    "interpro": ["uniprot", "go", "interproparent", "interprochild"],
    "chembl_molecule": ["chembl_activity", "chembl_target", "pubchem", "chebi", "drugcentral", "clinical_trials"],
    "chembl_activity": ["chembl_molecule", "chembl_assay"],
    "chembl_assay": ["chembl_activity", "chembl_target", "chembl_document"],
    "chembl_target": ["chembl_assay", "uniprot", "chembl_molecule"],
    "pubchem": ["chembl_molecule", "chebi", "hmdb", "pubchem_activity", "pubmed", "patent_compound", "bindingdb", "ctd", "pharmgkb"],
    "pubchem_activity": ["pubchem", "ensembl", "uniprot"],
    "chebi": ["pubchem", "rhea", "intact"],
    "drugcentral": ["chembl_molecule", "uniprot"],
    "swisslipids": ["uniprot", "go", "chebi", "uberon", "cl"],
    "lipidmaps": ["chebi", "pubchem"],
    "dbsnp": ["hgnc", "clinvar", "pharmgkb_variant", "alphamissense", "spliceai"],
    "clinvar": ["hgnc", "mondo", "hpo", "dbsnp", "orphanet"],
    "alphamissense": ["uniprot", "transcript"],
    "gwas": ["gwas_study", "efo", "dbsnp", "hgnc"],
    "gwas_study": ["gwas", "efo"],
    "mondo": ["gencc", "clinvar", "efo", "mesh", "hpo", "clinical_trials", "antibody", "cellxgene", "cellxgene_celltype", "orphanet", "mondoparent", "mondochild"],
    "gencc": ["mondo", "hpo", "hgnc", "ensembl"],
    "clinical_trials": ["mondo", "chembl_molecule"],
    "pharmgkb": ["hgnc", "dbsnp", "mesh", "pharmgkb_gene", "pharmgkb_variant", "pharmgkb_clinical", "pharmgkb_guideline", "pharmgkb_pathway"],
    "pharmgkb_variant": ["pharmgkb_clinical", "hgnc", "mesh", "dbsnp"],
    "pharmgkb_gene": ["hgnc", "entrez", "ensembl", "pharmgkb"],
    "pharmgkb_clinical": ["dbsnp", "hgnc", "mesh", "pharmgkb_variant"],
    "pharmgkb_guideline": ["hgnc", "pharmgkb"],
    "pharmgkb_pathway": ["hgnc", "pharmgkb"],
    "ctd": ["mesh", "entrez", "efo", "pubchem", "taxonomy"],
    "intact": ["uniprot", "chebi", "rnacentral"],
    "string": ["uniprot"],
    "biogrid": ["entrez", "uniprot", "refseq", "taxonomy"],
    "bgee": ["ensembl", "uberon", "cl", "taxonomy"],
    "cellxgene": ["cl", "uberon", "mondo", "efo", "taxonomy"],
    "cellxgene_celltype": ["cl", "uberon", "mondo"],
    "scxa": ["cl", "uberon", "taxonomy", "ensembl", "scxa_gene_experiment"],
    "scxa_expression": ["ensembl", "scxa", "scxa_gene_experiment"],
    "scxa_gene_experiment": ["ensembl", "scxa", "scxa_expression", "cl"],
    "rnacentral": ["uniprot", "ensembl", "intact"],
    "reactome": ["ensembl", "uniprot", "chebi", "go", "reactomeparent", "reactomechild"],
    "rhea": ["chebi", "uniprot", "go"],
    "go": ["ensembl", "uniprot", "reactome", "msigdb", "swisslipids", "bgee", "interpro", "goparent", "gochild"],
    "hpo": ["clinvar", "gencc", "mondo", "msigdb", "orphanet", "mim", "hmdb", "hgnc", "hpoparent", "hpochild"],
    "efo": ["gwas", "mondo", "cellxgene", "efoparent", "efochild"],
    "uberon": ["bgee", "cellxgene", "cellxgene_celltype", "swisslipids", "uberonparent", "uberonchild"],
    "cl": ["bgee", "cellxgene", "cellxgene_celltype", "scxa", "scxa_gene_experiment", "clparent", "clchild"],
    "taxonomy": ["ensembl", "uniprot", "bgee", "biogrid", "ctd", "taxparent", "taxchild"],
    "mesh": ["pharmgkb", "ctd", "pubchem", "mondo", "meshparent", "meshchild"],
    "eco": ["ecoparent", "ecochild"],
    "antibody": ["ensembl", "uniprot", "mondo", "pdb"],
    "msigdb": ["hgnc", "entrez", "go", "hpo"],
    "orphanet": ["hpo", "uniprot", "mondo", "hgnc", "clinvar", "mim", "mesh"],
    "mim": ["clinvar", "hpo", "mondo", "uniprot", "ctd"],
    "hmdb": ["pubchem", "hpo", "chebi", "uniprot"],
    "collectri": ["hgnc"],
    "esm2_similarity": ["uniprot"],
    "cellphonedb": ["uniprot", "ensembl", "hgnc", "pubmed"],
    "spliceai": ["hgnc"],
    "pdb": ["uniprot", "go", "interpro", "pfam", "taxonomy", "pubmed"],
    "fantom5_promoter": ["ensembl", "hgnc", "entrez", "uniprot", "uberon", "cl"],
    "fantom5_enhancer": ["ensembl", "uberon", "cl"],
    "fantom5_gene": ["ensembl", "hgnc", "entrez"],
    "jaspar": ["uniprot", "pubmed", "taxonomy"],
    "encode_ccre": ["taxonomy"]
}

# Note: Ontology hierarchies (goparent/gochild, mondoparent/mondochild, etc.)
# are now included directly in SCHEMA_EDGES for each ontology dataset.

# =============================================================================
# Filter Syntax (lean version - model discovers attributes from entry data)
# =============================================================================

SCHEMA_FILTERS = """
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
  - NO scientific notation (use 0.00000005 not 5e-8)

DISCOVER ATTRIBUTES:
  Use biobtree_entry to see available fields for any dataset.
  Filter on any attribute you see in the entry response.

EXAMPLES:
  >>uniprot[uniprot.reviewed==true]                           # Swiss-Prot curated only
  >>chembl_molecule[chembl_molecule.highestDevelopmentPhase>=3]  # Phase 3+ drugs
  >>clinical_trials[clinical_trials.phase=="PHASE3"]          # Phase 3 trials
  >>go[go.type=="biological_process"]                         # BP terms only
  >>reactome[reactome.name.contains("signaling")]             # Name contains "signaling"
"""



# =============================================================================
# Hints (general guidance for effective queries)
# =============================================================================

SCHEMA_HINTS = """
# QUERY WORKFLOW

1. DISCOVER: biobtree_search(terms="entity") with NO dataset filter
2. NAVIGATE: biobtree_map from discovered IDs
3. DETAILS: biobtree_entry when needed

# DRUG TARGET DISCOVERY (try ALL three sources)

drugcentral: Best for mechanism of action, FDA-approved drug targets
  <drug> >> drugcentral >> uniprot

pubchem: Broadest coverage, bioactivity data, 60+ targets per compound
  <compound> >> pubchem >> pubchem_activity >> uniprot

chembl_target: Medicinal chemistry, requires activity/assay chain
  <chembl_id> >> chembl_molecule >> chembl_target >> uniprot
  NOTE: If chembl_molecule>>chembl_target returns 0, use drugcentral or pubchem instead

# FINDING SUBSTRATES/EFFECTORS (what the target acts on)

When the question asks what is AFFECTED by the drug's mechanism:
1. Get target's GO terms: >>uniprot>>go
2. Read GO term names - they contain substrate/product clues
3. Search for the metabolite mentioned in GO term name (in chebi or pubchem)

Example workflow:
  - Find drug target: saxagliptin >> drugcentral >> uniprot >> P27487 (DPP4)
  - Get GO terms: P27487 >> uniprot >> go >> "glucagon processing"
  - Extract clue from GO name: "glucagon"
  - Search metabolite: "glucagon-like peptide 1" >> find in pubchem/chembl
  - Answer: GLP-1 (the substrate DPP4 acts on)

Example GO term clues:
  - "cAMP/PKA signal transduction" -> search cAMP -> CHEBI:17489
  - "glucagon processing" -> search GLP-1 -> pubchem/chembl
  - "chloride channel" -> search chloride -> CHEBI:17996

# DISEASE-GENE (try multiple sources)

gencc: Curated disease-gene validity (Mendelian diseases)
clinvar: Variant-disease associations
orphanet: Rare disease genes
hpo: Phenotype to gene via >>hpo>>hgnc

# KEY PATTERNS

Drug mechanism: Search drug -> get drugcentral/pubchem ID -> map to uniprot -> check protein function
Disease genes: Search disease -> get mondo/hpo ID -> map via gencc/clinvar/orphanet to genes
Expression: gene >> ensembl >> bgee (tissue) or >> scxa (single-cell)

# FALLBACKS

- Zero results? Try alternative database from same category
- Disease not found? Try >>mondoparent for broader term
"""




def get_schema(topic: str = "all") -> dict:
    """
    Get schema information for a specific topic.

    Args:
        topic: One of "edges", "filters", or "all"

    Returns:
        Schema dictionary for the requested topic
    """
    if topic == "edges":
        # Bundle hints with edges - model needs to know both what connects AND what each database contains
        return {
            "edges": SCHEMA_EDGES,
            "hints": SCHEMA_HINTS,
            "note": "Use edges to build chains: >>dataset1>>dataset2. Ontology hierarchies use >>parent/>>child suffix (e.g., >>goparent, >>mondochild)."
        }
    elif topic == "filters" or topic == "filter_syntax":
        return {"filter_syntax": SCHEMA_FILTERS}
    elif topic == "datasets" or topic == "hierarchies":
        # Deprecated: use edges to see all datasets and their connections
        return {"note": "Use topic='edges' to see all datasets and their connections. Use biobtree_entry to discover dataset attributes."}
    else:  # "all"
        return {
            "syntax": ">>dataset1[filter]>>dataset2[filter]",
            "edges": SCHEMA_EDGES,
            "filters": SCHEMA_FILTERS,
            "hints": SCHEMA_HINTS
        }
