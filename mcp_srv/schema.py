"""
Biobtree Schema Data

Schema definitions for biobtree_help tool - dataset connections, filters, and query patterns.
"""

# =============================================================================
# Dataset Edges (connections between datasets)
# =============================================================================

SCHEMA_EDGES = {
    "ensembl": ["uniprot", "go", "transcript", "exon", "ortholog", "paralog", "dbsnp", "clinvar", "hgnc", "entrez", "refseq", "bgee", "gwas", "gencc", "biogrid", "string", "antibody", "scxa"],
    "hgnc": ["ensembl", "uniprot", "entrez", "gencc", "pharmgkb_gene", "msigdb"],
    "entrez": ["ensembl", "uniprot", "refseq", "go", "biogrid", "pubchem_activity"],
    "refseq": ["ensembl", "uniprot", "entrez"],
    "transcript": ["ensembl", "exon", "ufeature"],
    "uniprot": ["ensembl", "alphafold", "interpro", "pdb", "ufeature", "intact", "string", "biogrid", "chembl_target_component", "go", "reactome", "rhea", "swisslipids", "bindingdb", "antibody", "pubchem_activity"],
    "alphafold": ["uniprot"],
    "interpro": ["uniprot"],
    "chembl_molecule": ["chembl_activity", "pubchem", "chebi", "drugcentral", "clinical_trials"],
    "chembl_activity": ["chembl_molecule", "chembl_assay"],
    "chembl_assay": ["chembl_activity", "chembl_target", "chembl_document"],
    "chembl_target": ["chembl_assay", "chembl_target_component"],
    "chembl_target_component": ["chembl_target", "uniprot"],
    "pubchem": ["chembl_molecule", "chebi", "hmdb", "pubchem_activity", "pubmed", "patent_compound", "bindingdb", "ctd", "pharmgkb"],
    "pubchem_activity": ["pubchem", "ensembl", "uniprot"],
    "chebi": ["pubchem", "chembl_molecule", "rhea", "intact"],
    "drugcentral": ["chembl_molecule", "uniprot"],
    "swisslipids": ["uniprot", "go", "chebi", "uberon"],
    "lipidmaps": ["chebi", "pubchem"],
    "dbsnp": ["ensembl", "hgnc", "clinvar", "pharmgkb_variant"],
    "clinvar": ["ensembl", "hgnc", "mondo", "hpo", "dbsnp", "orphanet"],
    "alphamissense": ["uniprot", "transcript"],
    "gwas": ["gwas_study", "ensembl", "efo", "dbsnp"],
    "gwas_study": ["gwas", "efo"],
    "mondo": ["gencc", "clinvar", "efo", "mesh", "hpo", "clinical_trials", "antibody", "cellxgene", "orphanet"],
    "gencc": ["mondo", "hpo", "hgnc", "ensembl"],
    "clinical_trials": ["mondo", "chembl_molecule"],
    "pharmgkb": ["hgnc", "dbsnp", "mesh", "pharmgkb_gene", "pharmgkb_variant", "pharmgkb_clinical", "pharmgkb_guideline", "pharmgkb_pathway"],
    "ctd": ["mesh", "entrez", "efo", "pubchem", "taxonomy"],
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
    "hpo": ["clinvar", "gencc", "mondo", "msigdb", "orphanet", "mim", "hmdb"],
    "efo": ["gwas", "mondo", "cellxgene"],
    "uberon": ["bgee", "cellxgene", "swisslipids"],
    "cl": ["bgee", "cellxgene", "scxa"],
    "taxonomy": ["ensembl", "uniprot", "bgee", "biogrid", "ctd"],
    "mesh": ["pharmgkb", "ctd", "pubchem", "mondo"],
    "antibody": ["ensembl", "uniprot", "mondo", "pdb"],
    "msigdb": ["hgnc", "entrez", "go", "hpo"],
    "orphanet": ["hpo", "ensembl", "uniprot", "mondo", "hgnc", "clinvar", "mim", "mesh"],
    "mim": ["clinvar", "hpo", "mondo", "uniprot", "ctd"],
    "hmdb": ["pubchem", "hpo", "chebi", "uniprot"]
}

# =============================================================================
# Ontology Hierarchies
# =============================================================================

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

# =============================================================================
# Dataset Filters (CEL expressions)
# =============================================================================

SCHEMA_FILTERS = {
    "ensembl": {"genome": "str (homo_sapiens|mus_musculus)", "biotype": "str", "name": "str", "start": "int", "end": "int"},
    "uniprot": {"reviewed": "bool (true=Swiss-Prot)"},
    "alphafold": {"global_metric": "float", "mean_pae": "float"},
    "interpro": {"type": "str (Domain|Family|Repeat)"},
    "ufeature": {"type": "str (modified residue|disulfide bond|signal peptide|DNA-binding region|lipid moiety-binding region)"},
    "chembl_molecule": {"highestDevelopmentPhase": "int 0-4", "type": "str", "weight": "float"},
    "chembl_activity": {"standardType": "str (IC50|Ki|Kd)", "pChembl": "float"},
    "chembl_target": {"type": "str (SINGLE PROTEIN|PROTEIN COMPLEX)"},
    "pubchem": {
        "is_fda_approved": "bool (FDA approval status)",
        "compound_type": "str (drug|bioactive|literature|patent|biologic)",
        "molecular_weight": "float",
        "xlogp": "float (lipophilicity)",
        "hydrogen_bond_donors": "int",
        "hydrogen_bond_acceptors": "int",
        "tpsa": "float (topological polar surface area)",
        "rotatable_bonds": "int",
        "pharmacological_actions": "list (drug class, e.g., ACE Inhibitors)",
        "unii": "str (FDA UNII identifier)"
    },
    "dbsnp": {"allele_frequency": "float", "clinical_significance": "str", "is_common": "bool"},
    "clinvar": {"germline_classification": "str (Pathogenic|Benign)", "review_status": "str"},
    "alphamissense": {"am_class": "str (likely_pathogenic|ambiguous|likely_benign)", "am_pathogenicity": "float 0-1"},
    "gwas": {"p_value": "float", "pvalue_mlog": "float"},
    "gencc": {"classification_title": "str (Definitive|Strong|Moderate|Limited)", "moi_title": "str"},
    "pharmgkb_clinical": {"level_of_evidence": "str (1A|1B|2A|2B|3|4)"},
    "pharmgkb_guideline": {"source": "str (CPIC|DPWG)"},
    "clinical_trials": {"phase": "str (PHASE1|PHASE2|PHASE3|PHASE4)", "overall_status": "str (RECRUITING|COMPLETED|TERMINATED)"},
    "bindingdb": {"ki": "str", "ic50": "str"},
    "intact": {"confidence_score": "float", "detection_method": "str"},
    "string": {"interactions[].score": "int 0-1000", "interactions[].has_experimental": "bool"},
    "biogrid": {"interaction_count": "int"},
    "bgee": {"max_expression_score": "float (use .0 suffix)", "average_expression_score": "float", "gold_quality_count": "int", "expression_breadth": "str (ubiquitous|broad|moderate|narrow|specific)"},
    "reactome": {"is_disease_pathway": "bool"},
    "go": {"type": "str (biological_process|molecular_function|cellular_component)"},
    "msigdb": {"collection": "str (H|C1-C8)", "gene_count": "int"},
    "antibody": {"status": "str (Active|Discontinued)", "antibody_type": "str (therapeutic)", "isotype": "str (G1|G2|G4)"}
}

# =============================================================================
# Example Identifiers
# =============================================================================

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

# =============================================================================
# Query Patterns
# =============================================================================

SCHEMA_PATTERNS = """# ===== DRUG DISCOVERY (use BOTH ChEMBL AND PubChem for comprehensive results) =====

# Gene -> Drugs via ChEMBL (medicinal chemistry focus, clinical phases)
<gene> >> ensembl >> uniprot >> chembl_target_component >> chembl_target >> chembl_assay >> chembl_activity >> chembl_molecule

# Gene -> Drugs via PubChem (broader coverage, FDA approval, bioactivity)
<gene> >> ensembl >> uniprot >> pubchem_activity >> pubchem
<gene> >> ensembl >> uniprot >> pubchem_activity >> pubchem[pubchem.is_fda_approved==true]  # FDA approved only

# Gene -> Approved drugs only (ChEMBL)
<gene> >> ensembl >> uniprot >> chembl_target_component >> chembl_target >> chembl_assay >> chembl_activity >> chembl_molecule[chembl.molecule.highestDevelopmentPhase>2]

# Compound -> Gene/Protein targets via PubChem
<compound> >> pubchem >> pubchem_activity >> ensembl
<compound> >> pubchem >> pubchem_activity >> uniprot

# Compound -> Cross-database links via PubChem
<compound> >> pubchem >> hmdb            # metabolite data
<compound> >> pubchem >> chembl_molecule # ChEMBL cross-ref
<compound> >> pubchem >> pubmed          # literature references (63k+ for aspirin)
<compound> >> pubchem >> patent_compound # patent information
<compound> >> pubchem >> bindingdb       # binding affinity data
<compound> >> pubchem >> ctd             # toxicogenomics (CTD disease/gene links)
<compound> >> pubchem >> pharmgkb        # pharmacogenomics annotations

# Disease -> Compounds via CTD (Comparative Toxicogenomics Database)
# NOTE: Use MeSH bridge for reliable disease->CTD mapping
<disease> >> mondo >> mesh >> ctd >> pubchem
<mesh_id> >> mesh >> ctd >> pubchem  # Direct MeSH to CTD (5000+ compounds for breast cancer)

# NOTE: ChEMBL vs PubChem strengths:
# - ChEMBL: curated medicinal chemistry, clinical development phases, assay details
# - PubChem: broader coverage, FDA approval, bioactivity screening, literature, patents
# - PubChem embedded attributes (in full mode): mesh_terms, pharmacological_actions,
#   compound_type (drug/bioactive/patent), unii (FDA), has_literature, has_patents,
#   molecular properties (xlogp, tpsa, rotatable_bonds, hydrogen_bond_donors/acceptors)

# ===== VARIANT ANALYSIS =====

# Gene -> Pathogenic variants
<gene> >> ensembl >> clinvar[clinvar.germline_classification=="Pathogenic"]
<gene> >> ensembl >> uniprot >> alphamissense[alphamissense.am_class=="likely_pathogenic"]

# SNP -> Clinical significance
<rsid> >> dbsnp >> clinvar >> mondo
<rsid> >> pharmgkb_variant >> pharmgkb_clinical

# ===== DISEASE RESOURCES =====

# Disease -> Structures
<disease> >> mondo >> gencc >> ensembl[ensembl.genome=="homo_sapiens"] >> uniprot[uniprot.reviewed==true] >> alphafold

# Disease -> All resources
<disease> >> mondo >> gencc >> ensembl      # causative genes
<disease> >> mondo >> clinvar >> dbsnp      # pathogenic variants
<disease> >> mondo >> clinical_trials       # active trials
<disease> >> mondo >> antibody              # therapeutic antibodies
<disease> >> mondo >> mesh >> ctd >> pubchem  # associated compounds via CTD
<disease> >> mondo >> cellxgene             # single-cell RNA-seq datasets

# Rare diseases via Orphanet (phenotype frequencies, gene associations)
<disease> >> mondo >> orphanet >> ensembl   # rare disease genes via Orphanet
<phenotype> >> hpo >> orphanet             # rare diseases with phenotype (with frequency evidence)
<phenotype> >> hpo >> mim                  # OMIM Mendelian diseases for phenotype

# ===== SINGLE-CELL / EXPRESSION =====

# Disease/Tissue/CellType -> Single-cell datasets
<disease> >> mondo >> cellxgene        # scRNA-seq datasets for disease (9 for diabetes)
<cell_type> >> cl >> cellxgene         # datasets with cell type (75+ for neurons)
<tissue> >> uberon >> cellxgene        # datasets from tissue (13 for pancreas)
<gene> >> ensembl >> scxa              # Single Cell Expression Atlas

# Tissue -> Expression
<tissue> >> uberon >> bgee >> ensembl  # genes expressed in tissue

# ===== INTERACTIONS =====

# Gene -> Interactions
<gene> >> ensembl >> uniprot >> intact
<gene> >> ensembl >> entrez >> biogrid

# ===== ONTOLOGY =====

# Ontology navigation
<term> >> go >> goparent
<term> >> go >> gochild
<term> >> mondo >> mondoparent
<term> >> mondo >> mondochild
<mesh_id> >> mesh >> meshparent     # MeSH hierarchy (D001943 -> 2 parents)
<mesh_id> >> mesh >> meshchild      # MeSH subtypes (D001943 -> 8 children)

# ===== PATHWAYS =====

# Pathway -> Genes/Proteins
<pathway> >> reactome >> ensembl    # genes in pathway (41 for R-HSA-5693567)
<pathway> >> reactome >> reactomechild  # sub-pathways
<protein> >> uniprot >> reactome    # protein's pathways (46 for TP53)

# ===== CLINICAL TRIALS =====

# Disease <-> Clinical Trials (bidirectional)
<disease> >> mondo >> clinical_trials   # trials for disease (12k+ for diabetes)
<trial_id> >> clinical_trials >> mondo  # diseases in trial (106 for NCT00000466)"""

# =============================================================================
# Text Search Support
# =============================================================================

SCHEMA_TEXT_SEARCH = """Datasets supporting partial text search:
- mondo, hpo, efo, orphanet: disease/phenotype names ("alzheimer", "breast cancer", "marfan syndrome")
- chembl_molecule, pubchem, pharmgkb, bindingdb, hmdb: drug/compound/metabolite names ("warfarin", "aspirin", "glucose")
- clinical_trials: conditions, interventions
- antibody: antibody names ("bevacizumab")

NOTE: For drug discovery, query BOTH ChEMBL and PubChem for comprehensive coverage:
- ChEMBL: curated medicinal chemistry, clinical phases, assay protocols
- PubChem: broader compounds, FDA approval, bioactivity screens, patents, metabolites"""

# =============================================================================
# Filter Syntax Rules
# =============================================================================

SCHEMA_FILTER_SYNTAX = """
CRITICAL FILTER SYNTAX RULES:

1. FLOAT COMPARISONS NEED .0 SUFFIX:
   - WRONG: >>pubchem[pubchem.molecular_weight<500]
   - RIGHT: >>pubchem[pubchem.molecular_weight<500.0]

   Affected fields: molecular_weight, xlogp, tpsa, pvalue_mlog, global_metric,
                    mean_pae, am_pathogenicity, expression_score, confidence_score

2. NO SCIENTIFIC NOTATION:
   - WRONG: >>gwas[gwas.p_value<5e-8]
   - RIGHT: >>gwas[gwas.p_value<0.00000005]

3. STRING VALUES ARE CASE-SENSITIVE:
   - Use exact values: "Pathogenic" not "pathogenic"
   - Phase values: "PHASE1", "PHASE2", "PHASE3", "PHASE4" (uppercase)
   - Status values: "RECRUITING", "COMPLETED" (uppercase)

4. EXAMPLES WITH CORRECT SYNTAX:
   # PubChem Lipinski filters
   >>pubchem[pubchem.molecular_weight<500.0]
   >>pubchem[pubchem.xlogp<5.0]
   >>pubchem[pubchem.tpsa<140.0]

   # AlphaFold confidence
   >>alphafold[alphafold.global_metric>70.0]
   >>alphafold[alphafold.mean_pae<25.0]

   # GWAS significance
   >>gwas[gwas.pvalue_mlog>8.0]
   >>gwas[gwas.p_value<0.00000005]

   # Bgee expression
   >>bgee[bgee.max_expression_score>90.0]
   >>bgee[bgee.gold_quality_count>10]

   # Clinical trials
   >>clinical_trials[clinical_trials.phase=="PHASE3"]
   >>clinical_trials[clinical_trials.overall_status=="RECRUITING"]
"""

# =============================================================================
# Disease Ontology Mapping Strategy
# =============================================================================

SCHEMA_DISEASE_ONTOLOGY = """
# ===== DISEASE ONTOLOGY MAPPING STRATEGY =====
#
# IMPORTANT: Different databases annotate diseases using DIFFERENT ontologies.
# When a specific disease term doesn't map, try these strategies:
#
# 1. ONTOLOGY USAGE BY DATABASE:
#    | Database        | Primary Ontology | Notes                              |
#    |-----------------|------------------|-------------------------------------|
#    | GWAS Catalog    | EFO              | Experimental Factor Ontology        |
#    | CTD             | MeSH             | Medical Subject Headings            |
#    | CellXGene       | MONDO            | Monarch Disease Ontology            |
#    | ClinVar         | MONDO, HPO       | Also uses OMIM for Mendelian        |
#    | Clinical Trials | MONDO            | Mapped from MeSH/ICD                |
#    | GenCC           | MONDO            | Gene-disease curations              |
#    | Orphanet        | Orphanet IDs     | Rare diseases (with HPO phenotypes) |
#    | MIM/OMIM        | MIM numbers      | Mendelian diseases                  |
#    | Bgee            | UBERON, CL       | Tissues and cell types              |
#
# 2. WHEN DIRECT MAPPING FAILS - Use ontology BRIDGES:
#
#    # MONDO -> CTD (CTD uses MeSH, not MONDO directly)
#    <disease> >> mondo >> mesh >> ctd >> pubchem   # Use MeSH bridge
#
#    # MONDO -> GWAS (GWAS uses EFO)
#    <disease> >> mondo >> efo >> gwas              # Map to EFO first
#    <efo_id> >> efo >> gwas                        # Or use EFO ID directly
#
# 3. WHEN SPECIFIC TERM FAILS - Try PARENT terms:
#
#    # Example: MONDO:0005148 (type 2 diabetes) -> EFO fails
#    # Because EFO:0001360 (type II diabetes) is OBSOLETE in EFO
#    # Solution: Use parent term MONDO:0005015 (diabetes mellitus)
#
#    <disease> >> mondo >> mondoparent >> efo      # Try parent if specific fails
#    <disease> >> mondo >> mondoparent >> mesh     # Or for MeSH bridge
#
#    # Check parent/child relationships:
#    <disease> >> mondo >> mondoparent             # Get broader disease terms
#    <disease> >> mondo >> mondochild              # Get more specific subtypes
#    <disease> >> efo >> efoparent                 # EFO hierarchy
#    <mesh_id> >> mesh >> meshparent               # MeSH tree navigation
#
# 4. WORKING CROSS-ONTOLOGY MAPPINGS:
#
#    | Path                    | Example Working                           |
#    |-------------------------|-------------------------------------------|
#    | mondo >> efo            | MONDO:0005015 -> EFO:0000400 (diabetes)   |
#    | efo >> mondo            | EFO:0000400 -> MONDO:0005015              |
#    | mondo >> mesh           | MONDO:0005148 -> D003924 (type 2 diabetes)|
#    | mesh >> ctd             | D003924 -> 5000+ compounds                |
#    | hpo >> mondo            | Some work, depends on xref in source data |
#    | hpo >> clinvar          | HP:0001250 -> 150+ variants (seizures)    |
#
# 5. PHENOTYPE vs DISEASE distinction:
#
#    # HPO = Phenotypes (symptoms, features)
#    # MONDO = Diseases (diagnoses)
#    # For gene discovery from phenotypes, use ClinVar path:
#    <phenotype> >> hpo >> clinvar >> ensembl      # Genes with variants causing phenotype
#
#    # NOT >> hpo >> gencc (GenCC only has 8 HPO terms - inheritance modes, not phenotypes)
#
# 6. PRACTICAL EXAMPLES:
#
#    # Type 2 Diabetes drug discovery - use MeSH bridge
#    MONDO:0005148 >> mondo >> mesh >> ctd >> pubchem
#    # Result: 150+ compounds from toxicogenomics literature
#
#    # Diabetes GWAS - use parent term or direct EFO
#    EFO:0000400 >> efo >> gwas                    # Direct EFO works
#    MONDO:0005015 >> mondo >> efo >> gwas         # Parent MONDO works
#
#    # Breast cancer - multiple paths for comprehensive results
#    MONDO:0007254 >> mondo >> cellxgene           # 38 scRNA-seq datasets
#    D001943 >> mesh >> ctd >> pubchem             # CTD compounds via MeSH
#    MONDO:0007254 >> mondo >> gencc >> ensembl    # Causative genes
"""

# =============================================================================
# Pagination Info
# =============================================================================

SCHEMA_PAGINATION = {
    "description": "Results are automatically paginated (~150 results per page)",
    "response_fields": {
        "has_next": "boolean indicating more results available",
        "next_token": "token to pass for next page of results"
    },
    "usage": "When has_next is true, make another request with page=next_token to get more results"
}


def get_schema(topic: str = "all") -> dict:
    """
    Get schema information for a specific topic.

    Args:
        topic: One of "edges", "filters", "hierarchies", "patterns",
               "examples", "filter_syntax", "disease_ontology", or "all"

    Returns:
        Schema dictionary for the requested topic
    """
    if topic == "edges":
        return {"edges": SCHEMA_EDGES}
    elif topic == "filters":
        return {"filters": SCHEMA_FILTERS}
    elif topic == "hierarchies":
        return {"hierarchies": SCHEMA_HIERARCHIES, "note": "Use dataset>>parent or dataset>>child for navigation"}
    elif topic == "patterns":
        return {"patterns": SCHEMA_PATTERNS, "text_search": SCHEMA_TEXT_SEARCH, "tip": "For cross-ontology mapping issues (MONDO/EFO/MeSH), use topic='disease_ontology'"}
    elif topic == "examples":
        return {"examples": SCHEMA_EXAMPLES}
    elif topic == "filter_syntax":
        return {"filter_syntax": SCHEMA_FILTER_SYNTAX, "note": "CRITICAL: Float comparisons need .0 suffix (e.g., >90.0 not >90). No scientific notation."}
    elif topic == "disease_ontology":
        return {"disease_ontology_mapping": SCHEMA_DISEASE_ONTOLOGY, "note": "CRITICAL: Different databases use different ontologies. Use bridges and parent terms when direct mapping fails."}
    else:  # "all"
        return {
            "query_syntax": "<terms> >> <dataset>[<filter>] >> <dataset>[<filter>] >> ...",
            "edges": SCHEMA_EDGES,
            "hierarchies": SCHEMA_HIERARCHIES,
            "filters": SCHEMA_FILTERS,
            "examples": SCHEMA_EXAMPLES,
            "patterns": SCHEMA_PATTERNS,
            "text_search": SCHEMA_TEXT_SEARCH,
            "pagination": SCHEMA_PAGINATION,
            "additional_topics": ["disease_ontology - use when cross-ontology mapping fails (MONDO/EFO/MeSH bridges)"]
        }
