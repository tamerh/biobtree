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
    "mondo": ["gencc", "clinvar", "efo", "mesh", "hpo", "clinical_trials", "antibody", "cellxgene", "cellxgene_celltype", "orphanet"],
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
    "reactome": ["ensembl", "uniprot", "chebi", "go"],
    "rhea": ["chebi", "uniprot", "go"],
    "go": ["ensembl", "uniprot", "reactome", "msigdb", "swisslipids", "bgee", "interpro"],
    "hpo": ["clinvar", "gencc", "mondo", "msigdb", "orphanet", "mim", "hmdb","hgnc"],
    "efo": ["gwas", "mondo", "cellxgene"],
    "uberon": ["bgee", "cellxgene", "cellxgene_celltype", "swisslipids"],
    "cl": ["bgee", "cellxgene", "cellxgene_celltype", "scxa", "scxa_gene_experiment"],
    "taxonomy": ["ensembl", "uniprot", "bgee", "biogrid", "ctd"],
    "mesh": ["pharmgkb", "ctd", "pubchem", "mondo"],
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
    "eco": ["ecoparent", "ecochild"],
    "interpro": ["interproparent", "interprochild"]
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
    "spliceai": {"score": "float 0-1 (delta score, higher=more splice impact)", "effect": "str (acceptor_loss|acceptor_gain|donor_loss|donor_gain)"},
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
    "antibody": {"status": "str (Active|Discontinued)", "antibody_type": "str (therapeutic)", "isotype": "str (G1|G2|G4)"},
    "collectri": {"tf_gene": "str (TF gene symbol)", "target_gene": "str (target gene symbol)", "regulation": "str (Activation|Repression|Unknown)", "confidence": "str (High|Low)"},
    "esm2_similarity": {"top_similarity": "float (0-1, highest cosine similarity)", "avg_similarity": "float (0-1)", "similarity_count": "int"},
    "cellphonedb": {"directionality": "str (Ligand-Receptor|Adhesion-Adhesion|etc)", "classification": "str (signaling pathway)", "receptor_a": "bool", "receptor_b": "bool", "secreted_a": "bool", "secreted_b": "bool", "is_complex_a": "bool", "is_complex_b": "bool", "is_integrin": "bool"},
    "pdb": {"method": "str (X-RAY DIFFRACTION|ELECTRON MICROSCOPY|SOLUTION NMR)", "resolution": "float (Angstroms, lower=better)"},
    "fantom5_promoter": {"tpm_average": "float", "tpm_max": "float", "samples_expressed": "int", "expression_breadth": "str (ubiquitous|broad|tissue_specific)"},
    "fantom5_gene": {"tpm_average": "float", "tpm_max": "float", "expression_breadth": "str"},
    "jaspar": {"collection": "str (CORE|UNVALIDATED)", "type": "str (ChIP-seq|SELEX|PBM)", "tax_group": "str (vertebrates|plants|insects|fungi)"},
    "encode_ccre": {"ccre_class": "str (PLS|pELS|dELS|CA-CTCF|CA-TF|CA|TF)", "chromosome": "str (chr1..chr22,chrX,chrY)", "start": "int", "end": "int"},
    "mirdb": {"target_count": "int", "max_score": "float 50-100", "avg_score": "float"}
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
    "string": "9606.ENSP00000269305",
    "collectri": "MYC:TERT (MYC regulates TERT)",
    "esm2_similarity": "P04637 (TP53 UniProt ID)",
    "cellphonedb": "CPI-SC0A2DB962D (ligand-receptor interaction)",
    "spliceai": "1:803428:TCCAT:T (chr:pos:ref:alt variant ID)",
    "pdb": "4HHB (Hemoglobin structure)",
    "fantom5_promoter": "TP53 (gene symbol)",
    "fantom5_gene": "BRCA1 (gene symbol)",
    "jaspar": "MA0004.1 (TF binding profile)",
    "encode_ccre": "EH38E2776516 (cCRE accession)",
    "mirdb": "hsa-miR-21-5p (human miRNA)"
}

# =============================================================================
# Query Patterns
# =============================================================================

SCHEMA_PATTERNS = """# Human genes: use >>hgnc>>ensembl instead of >>ensembl[genome filter]
# Pagination: to scan all results, use p=<next_token> until has_next==false

# ===== ChEMBL DATA MODEL =====
#
# Target vs Molecule vs UniProt:
#   - TARGET = the biological entity (protein) that a drug ACTS ON
#   - MOLECULE = the chemical compound (drug) that does the targeting
#   - UNIPROT = the protein sequence/annotation
#
# Relationships:
#   molecule >> target    = "this drug acts on these targets"
#   target >> uniprot     = "this target IS this protein"
#   molecule >> target >> uniprot = "what proteins does this drug affect?"
#   uniprot >> chembl_target >> chembl_molecule = "what drugs target this protein?"

# ===== DRUG DISCOVERY (use BOTH ChEMBL AND PubChem for comprehensive results) =====

# Gene -> Drugs via ChEMBL (semantic path: protein -> target -> molecules)
<gene> >> ensembl >> uniprot >> chembl_target                # Protein to drug targets
<gene> >> ensembl >> uniprot >> chembl_target >> chembl_molecule  # Protein to drugs
<gene> >> ensembl >> uniprot >> chembl_target >> chembl_molecule[chembl.molecule.highestDevelopmentPhase>2]  # Approved drugs

# Gene -> Drugs via PubChem (broader coverage, FDA approval, bioactivity)
<gene> >> ensembl >> uniprot >> pubchem_activity >> pubchem
<gene> >> ensembl >> uniprot >> pubchem_activity >> pubchem[pubchem.is_fda_approved==true]  # FDA approved only

# ChEMBL Molecule -> Gene targets (semantic path: drug -> target -> protein)
<chembl_id> >> chembl_molecule >> chembl_target >> uniprot   # Drug to protein targets
<chembl_id> >> chembl_molecule >> chembl_target >> uniprot >> ensembl  # Drug to genes

# Compound -> Gene/Protein targets via PubChem
<compound> >> pubchem >> pubchem_activity >> ensembl
<compound> >> pubchem >> pubchem_activity >> uniprot

# Compound -> Cross-database links via PubChem
<compound> >> pubchem >> chebi           # ChEBI metabolite ontology
<compound> >> pubchem >> hmdb            # HMDB metabolite data
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

# ===== METABOLITES (ChEBI, HMDB) =====

# ===== COMPOUNDS/METABOLITES (PubChem is the central hub) =====
# IMPORTANT: PubChem is the hub for all compound cross-references
# Always route through PubChem for: chebi, hmdb, chembl_molecule, bindingdb, ctd, pharmgkb

# PubChem connections:
<compound> >> pubchem >> chebi           # PubChem to ChEBI metabolite ontology
<compound> >> pubchem >> hmdb            # PubChem to Human Metabolome DB
<compound> >> pubchem >> chembl_molecule # PubChem to ChEMBL drugs
<compound> >> pubchem >> bindingdb       # PubChem to binding affinity data
<compound> >> pubchem >> ctd             # PubChem to toxicogenomics
<compound> >> pubchem >> pharmgkb        # PubChem to pharmacogenomics
<compound> >> pubchem >> pubmed          # PubChem to literature

# ChEBI (go via PubChem for cross-database)
<metabolite> >> chebi >> pubchem         # ChEBI to PubChem (then to other DBs)
<metabolite> >> chebi >> rhea            # ChEBI to biochemical reactions

# HMDB for human metabolome
<metabolite> >> hmdb >> pubchem          # HMDB to PubChem
<metabolite> >> hmdb >> chebi            # HMDB to ChEBI

# Metabolite -> Protein/Enzyme connections
<chebi_id> >> chebi >> rhea >> uniprot   # Metabolite to enzymes via reactions

# ===== ONTOLOGY EXPANSION (CRITICAL for drug/disease queries) =====
# When querying GO terms or diseases, ALSO query child terms for broader coverage.
# Proteins are often annotated with regulatory terms ("regulation of X") rather than
# the direct process term ("X"), so expanding to children captures more results.

# Step 1: Get child terms
<go_term> >> go >> gochild              # Get child GO terms
<disease> >> mondo >> mondochild        # Get more specific disease subtypes
<phenotype> >> hpo >> hpochild          # Get more specific phenotypes

# Step 2: Query children for drugs
# First get children, then query relevant child terms for broader drug coverage
<go_term> >> go >> ensembl >> uniprot >> chembl_target >> chembl_molecule  # Direct query
# Better: First expand with >>gochild, then query each relevant child term

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
# NOTE: GenCC only covers ~35K Mendelian/genetic disease-gene curations. Non-genetic diseases
# (paraneoplastic syndromes, infections, injuries) will return 0 results - this is expected.
<disease> >> mondo >> gencc >> ensembl      # causative genes (genetic diseases only)
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

# Cell type -> Marker genes with expression
<cell_type> >> cl >> scxa_gene_experiment  # CL:0000788 -> 75 marker genes with TPM, logFC
<gene> >> ensembl >> scxa_expression >> scxa_gene_experiment  # CD19 -> per-cell-type expression

# ===== FANTOM5 CAGE EXPRESSION (promoters, enhancers, TSS) =====
<gene> >> fantom5_promoter             # CAGE peaks/promoters for gene
<gene> >> fantom5_gene                 # Gene-level CAGE expression
<tissue> >> uberon >> fantom5_promoter # Promoters active in tissue
<cell_type> >> cl >> fantom5_promoter  # Promoters active in cell type

# ===== INTERACTIONS =====

# Gene -> Interactions
<gene> >> ensembl >> uniprot >> intact
<gene> >> ensembl >> entrez >> biogrid

# ===== TRANSCRIPTIONAL REGULATION (CollecTRI) =====

# Gene -> TF-target regulatory interactions
<gene> >> hgnc >> collectri                            # Find all TF-target pairs involving gene
<gene> >> hgnc >> collectri[collectri.regulation=="Activation"]   # Activating TFs only
<gene> >> hgnc >> collectri[collectri.regulation=="Repression"]   # Repressing TFs only
<gene> >> hgnc >> collectri[collectri.confidence=="High"]         # High confidence interactions

# ===== CELL-CELL COMMUNICATION (CellPhoneDB) =====

<gene> >> hgnc >> cellphonedb                          # Gene to ligand-receptor interactions
<protein> >> uniprot >> cellphonedb                    # Protein to cell communication

# ===== PROTEIN STRUCTURES (PDB) =====

<pdb_id> >> pdb                                        # Structure lookup (e.g., 4HHB)
<protein> >> uniprot >> pdb                            # Protein to 3D structures
<gene> >> ensembl >> uniprot >> pdb                    # Gene to structures
<protein> >> uniprot >> pdb[pdb.method=="X-RAY DIFFRACTION"]  # X-ray only
<protein> >> uniprot >> pdb[pdb.resolution<2.0]        # High-resolution (<2A)
<pdb_id> >> pdb >> uniprot                             # Structure to proteins
<pdb_id> >> pdb >> go                                  # Structure to GO terms

# ===== GENE RELATIONSHIPS =====

# Gene -> Paralogs (genes with similar function from same family)
<gene> >> ensembl >> paralog        # Find genes with shared evolutionary origin
<gene> >> hgnc >> ensembl >> paralog  # From gene symbol to paralogs

# Gene -> Orthologs (same gene in other species)
<gene> >> ensembl >> ortholog       # Cross-species gene mappings

# ===== ONTOLOGY =====

# Ontology navigation
<term> >> go >> goparent
<term> >> go >> gochild
<term> >> mondo >> mondoparent      # Broader disease terms (important when specific term has no data)
<term> >> mondo >> mondochild       # More specific disease subtypes
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
<trial_id> >> clinical_trials >> mondo  # diseases in trial (106 for NCT00000466)

# ===== TF BINDING PROFILES (JASPAR) =====

<gene> >> ensembl >> uniprot >> jaspar                 # Gene to TF binding motifs
<matrix_id> >> jaspar >> uniprot                       # Binding profile to protein
<gene> >> jaspar[jaspar.collection=="CORE"]            # CORE collection only
<gene> >> jaspar[jaspar.type=="ChIP-seq"]              # ChIP-seq derived profiles

# ===== ENCODE cCRE (Regulatory Elements) =====

<ccre_id> >> encode_ccre                                    # cCRE lookup (e.g., EH38E2776516)
PLS >> encode_ccre                                          # Promoter-like sequences
pELS >> encode_ccre                                         # Proximal enhancer-like sequences
dELS >> encode_ccre                                         # Distal enhancer-like sequences
<ccre_id> >> encode_ccre[encode_ccre.ccre_class=="PLS"]     # Filter by classification
<ccre_id> >> encode_ccre[encode_ccre.chromosome=="chr1"]    # Filter by chromosome"""

# =============================================================================
# Text Search Support
# =============================================================================

SCHEMA_TEXT_SEARCH = """Datasets supporting partial text search:
- mondo, hpo, efo, orphanet: disease/phenotype names ("alzheimer", "breast cancer", "marfan syndrome")
- chembl_molecule, pubchem, pharmgkb, bindingdb, hmdb: drug/compound/metabolite names ("warfarin", "aspirin", "glucose")
- clinical_trials: conditions, interventions
- antibody: antibody names ("bevacizumab")
- fantom5_promoter, fantom5_gene, fantom5_enhancer: gene symbols ("TP53", "BRCA1")
- encode_ccre: cCRE IDs or classifications ("EH38E2776516", "PLS", "pELS", "dELS", "CA-CTCF")

NOTE: For drug discovery, query BOTH ChEMBL and PubChem for comprehensive coverage:
- ChEMBL: curated medicinal chemistry, clinical phases, assay protocols
- PubChem: broader compounds, FDA approval, bioactivity screens, patents, metabolites"""

# =============================================================================
# Filter Syntax Rules
# =============================================================================

SCHEMA_FILTER_SYNTAX = """
CRITICAL FILTER SYNTAX RULES:

1. INTEGER vs FLOAT FIELDS:
   - INT (no .0 suffix): highestDevelopmentPhase, hydrogen_bond_donors/acceptors, rotatable_bonds, gene_count, start, end
   - FLOAT (use .0 suffix): molecular_weight, xlogp, tpsa, pChembl, global_metric, mean_pae, am_pathogenicity

   Example: [highestDevelopmentPhase>1] not [highestDevelopmentPhase>1.0]

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
# Lite Mode Response Format
# =============================================================================

SCHEMA_RESPONSE_FORMAT = """
# ===== LITE MODE RESPONSE FORMAT =====
#
# Lite mode returns compact, token-efficient responses with pipe-delimited data.
# Parse by splitting on "|" using the schema as column headers.

# ----- SEARCH RESPONSE -----
{
  "context": {"query": "TP53,BRCA1", "dataset_filter": ""},
  "stats": {"total": 10},
  "pagination": {"has_next": true, "next_token": "0,0,-1,10"},
  "schema": "id|dataset|name|xref_count",
  "data": [
    "HGNC:11998|hgnc|tumor protein p53|24",
    "ENSG00000141510|ensembl|TP53|337",
    "P04637|uniprot|Cellular tumor antigen p53|5674"
  ]
}

# Parsing search data:
# - Split each row by "|" -> [id, dataset, name, xref_count]
# - xref_count = number of cross-references (higher = more connected)

# ----- MAP RESPONSE (grouped by input) -----
{
  "context": {
    "query": ">>hgnc>>ensembl",
    "source_dataset": "hgnc",
    "target_dataset": "ensembl"
  },
  "stats": {"total": 3, "mapped": 3},
  "pagination": {"has_next": false},
  "schema": "id|name|biotype|genome",
  "mappings": [
    {
      "input": "TP53",
      "source": "HGNC:11998|tumor protein p53",
      "targets": ["ENSG00000141510|TP53|protein_coding|homo_sapiens"]
    },
    {
      "input": "BRCA1",
      "source": "HGNC:1100|BRCA1 DNA repair associated",
      "targets": ["ENSG00000012048|BRCA1|protein_coding|homo_sapiens"]
    }
  ]
}

# Parsing map data:
# - Each mapping has: input (original term), source (resolved ID|name), targets (list)
# - Split source by "|" -> [source_id, source_name]
# - Split each target by "|" using schema columns -> [id, name, biotype, genome]
# - Schema varies by target dataset (ensembl has biotype/genome, uniprot has reviewed, etc.)

# ----- COMMON SCHEMAS BY DATASET -----
# ensembl: id|name|biotype|genome
# uniprot: id|reviewed (true = Swiss-Prot curated)
# chembl_molecule: id|name|type|highestDevelopmentPhase
# go: id|type|name
# clinvar: id|name|type|germline_classification
# dbsnp: id|chromosome|position|ref_allele|alt_allele
"""

# =============================================================================
# Pagination Info
# =============================================================================

SCHEMA_PAGINATION = {
    "description": "Results are paginated by total targets across all inputs",
    "limits": {
        "lite_mode": "~150 targets per page (optimized for LLM token efficiency)",
        "full_mode": "~30 targets per page (includes all attributes)"
    },
    "response_fields": {
        "has_next": "boolean - true if more results available",
        "next_token": "string - pass as 'page' parameter for next page"
    },
    "usage": "When has_next is true, call again with page=next_token to continue",
    "note": "For map queries with multiple inputs, pagination groups complete mappings (all targets for an input stay together)"
}


def get_schema(topic: str = "all") -> dict:
    """
    Get schema information for a specific topic.

    Args:
        topic: One of "edges", "filters", "hierarchies", "patterns",
               "examples", "filter_syntax", "disease_ontology",
               "response_format", or "all"

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
    elif topic == "response_format":
        return {"response_format": SCHEMA_RESPONSE_FORMAT, "pagination": SCHEMA_PAGINATION, "note": "Lite mode returns compact pipe-delimited data. Parse by splitting on '|' using schema as headers."}
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
            "additional_topics": ["disease_ontology - cross-ontology mapping (MONDO/EFO/MeSH bridges)", "response_format - lite mode response structure and parsing"]
        }
