#!/bin/bash

# This script builds a compact LOOKUP database for model organism annotations.
# It uses the BUILD command (update + generate in one step) for fast creation.
#
# Datasets included:
# - HGNC: Human gene names and symbols
# - Ensembl: Gene IDs for 16 model organisms
# - ChEMBL: Chemical/drug information (molecule, document, target)
# - MONDO: Disease ontology
# - HPO: Human Phenotype Ontology
# - GO: Gene Ontology
# - UniProt: Protein sequences and annotations
# - Taxonomy: Organism classification
# - MeSH: Medical Subject Headings (NEW)
#
# This creates a fast lookup database suitable for:
# - Gene symbol → Ensembl ID resolution
# - Drug/chemical name → ChEMBL ID lookup
# - Disease term → MONDO/HPO lookup
# - GO term search
# - MeSH medical terminology lookup
#
# BUILD command: Runs update + generate sequentially (simpler than separate steps)

set -e

# Parse arguments
if [[ -z $1 ]]; then
    echo "Output directory parameter is required"
    echo "Usage: $0 <output_dir> [OPTIONS]"
    echo "Example: $0 /localscratch/\$USER/biobtree_lookup"
    echo "Example: $0 ./lookup_db"
    echo "Example: $0 ./lookup_db --maxcpu 8"
    echo ""
    echo "Options:"
    echo "  --maxcpu <N>      Maximum number of CPUs for biobtree (default: 4)"
    echo "  --tax <IDs>       Taxonomy IDs for Ensembl (default: 16 model organisms)"
    exit 1
fi

OUT_DIR=$1
MAXCPU=4  # Default: 4 CPUs

# Model organism taxonomy IDs (16 organisms from AlphaFold)
DEFAULT_TAXIDS="9606,10090,10116,7955,7227,6239,559292,284812,511145,3702,39947,4577,3847,44689,237561,243232"
ENSEMBL_TAXIDS="${DEFAULT_TAXIDS}"

# Parse additional arguments
shift
while [[ $# -gt 0 ]]; do
    case "$1" in
        --maxcpu)
            if [[ -z $2 || $2 =~ ^- ]]; then
                echo "Error: --maxcpu requires a numeric argument"
                exit 1
            fi
            MAXCPU=$2
            shift 2
            ;;
        --tax)
            if [[ -z $2 || $2 =~ ^- ]]; then
                echo "Error: --tax requires taxonomy IDs (comma-separated)"
                exit 1
            fi
            ENSEMBL_TAXIDS=$2
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Valid options: --maxcpu <N>, --tax <IDs>"
            exit 1
            ;;
    esac
done

echo "============================================"
echo "Biobtree Lookup Database Builder"
echo "============================================"
echo "Output directory: $OUT_DIR"
echo "Max CPUs: $MAXCPU"
echo "Build mode: Sequential (update + generate)"
echo ""
echo "Model organisms (16 organisms from AlphaFold):"
echo "  Organism                      Taxonomy ID"
echo "  ────────────────────────────  ────────────"
echo "  Homo sapiens                  9606"
echo "  Mus musculus                  10090"
echo "  Rattus norvegicus             10116"
echo "  Danio rerio                   7955"
echo "  Drosophila melanogaster       7227"
echo "  Caenorhabditis elegans        6239"
echo "  Saccharomyces cerevisiae      559292"
echo "  Schizosaccharomyces pombe     284812"
echo "  Escherichia coli K-12         511145"
echo "  Arabidopsis thaliana          3702"
echo "  Oryza sativa                  39947"
echo "  Zea mays                      4577"
echo "  Glycine max                   3847"
echo "  Dictyostelium discoideum      44689"
echo "  Candida albicans              237561"
echo "  Methanocaldococcus jannaschii 243232"
echo ""
echo "Datasets to build:"
echo "  - hgnc:     Human gene nomenclature"
echo "  - ensembl:  Genes for ${ENSEMBL_TAXIDS}"
echo "  - chembl:   Chemical/drug database (molecule, document, target, activity, assay, cell_line)"
echo "  - mondo:    Disease ontology"
echo "  - hpo:      Human Phenotype Ontology"
echo "  - go:       Gene Ontology"
echo "  - uniprot:  Protein database"
echo "  - taxonomy: Organism classification"
echo "  - mesh:     Medical Subject Headings (NEW - 217K terms)"
echo ""
echo "Build strategy: Single BUILD command (update + generate)"
echo "============================================"
echo ""

# Create output and logs directories
mkdir -p ${OUT_DIR}
mkdir -p logs

# Datasets to include in lookup database
LOOKUP_DATASETS="hgnc,ensembl,chembl,mondo,hpo,go,uniprot,taxonomy,mesh"

# Build parameters
BB_PARAMS="--maxcpu ${MAXCPU}"

# Log file
LOG_FILE="logs/lookup_build.log"

echo "Starting build at: $(date)"
echo ""
echo "Command:"
echo "./biobtree ${BB_PARAMS} --tax ${ENSEMBL_TAXIDS} -d \"${LOOKUP_DATASETS}\" --out-dir \"${OUT_DIR}\" build"
echo ""
echo "Progress will be logged to: ${LOG_FILE}"
echo ""

# Run the build command
./biobtree ${BB_PARAMS} --tax ${ENSEMBL_TAXIDS} -d "${LOOKUP_DATASETS}" --out-dir "${OUT_DIR}" build > ${LOG_FILE} 2>&1

# Check if successful
if [ $? -eq 0 ]; then
    echo ""
    echo "============================================"
    echo "Build Complete!"
    echo "============================================"
    echo ""
    echo "Database location: ${OUT_DIR}/db/"
    echo "Log file: ${LOG_FILE}"
    echo ""
    echo "Database statistics:"
    echo "  - HGNC: Human gene symbols and names"
    echo "  - Ensembl: Genes for 16 model organisms"
    echo "  - ChEMBL: Drugs, molecules, targets, activities, assays, cell lines"
    echo "  - MONDO: Disease ontology terms"
    echo "  - HPO: Phenotype terms with gene associations"
    echo "  - GO: Gene Ontology (BP, MF, CC)"
    echo "  - UniProt: Protein sequences (Swiss-Prot + TrEMBL)"
    echo "  - Taxonomy: NCBI Taxonomy"
    echo "  - MeSH: 30K+ descriptors + 186K+ supplementary concepts"
    echo ""
    echo "To start the web server:"
    echo "  ./biobtree --out-dir \"${OUT_DIR}\" web"
    echo ""
    echo "Example queries:"
    echo "  http://localhost:8888/ws/entry/?i=BRCA1          # Gene lookup"
    echo "  http://localhost:8888/ws/entry/?i=CHEMBL25      # Drug lookup"
    echo "  http://localhost:8888/ws/entry/?i=MONDO:0005015 # Disease lookup"
    echo "  http://localhost:8888/ws/entry/?i=HP:0001250    # Phenotype lookup"
    echo "  http://localhost:8888/ws/entry/?i=aspirin       # MeSH drug search"
    echo ""
    echo "Database size:"
    du -sh ${OUT_DIR}/db/ 2>/dev/null || echo "  (run 'du -sh ${OUT_DIR}/db/' to check)"
    echo ""
    echo "Completed at: $(date)"
    echo "============================================"
else
    echo ""
    echo "============================================"
    echo "Build Failed!"
    echo "============================================"
    echo ""
    echo "Check log file for details: ${LOG_FILE}"
    echo ""
    echo "Common issues:"
    echo "  - Network connectivity problems"
    echo "  - Insufficient disk space"
    echo "  - FTP download failures"
    echo ""
    echo "Try running again or check the log file"
    echo "============================================"
    exit 1
fi
