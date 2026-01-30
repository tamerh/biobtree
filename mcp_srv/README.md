# Biobtree MCP Server

MCP (Model Context Protocol) server that exposes biobtree functionality to Claude and other AI assistants.

## Overview

This MCP server provides 4 tools for querying biobtree's integrated biological databases:

| Tool | Purpose |
|------|---------|
| `biobtree_search` | Search for identifiers across 70+ databases |
| `biobtree_map` | Map identifiers through dataset chains |
| `biobtree_entry` | Get full entry details |
| `biobtree_meta` | List available datasets |

## Prerequisites

1. **Biobtree running** - Start biobtree web server:
   ```bash
   cd /path/to/biobtree
   ./biobtree web
   # Server runs at http://localhost:9291
   ```

2. **Python 3.10+** with MCP package installed (in tamer_biobtree conda env)

## Usage

### With Claude Desktop

Add to your Claude Desktop config:

**Linux:** `~/.config/claude/claude_desktop_config.json`
**macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "biobtree": {
      "command": "python",
      "args": ["-m", "mcp_srv.server"],
      "cwd": "/data/bioyoda/biobtreev2"
    }
  }
}
```

Restart Claude Desktop. The biobtree tools will be available.

### Testing

```bash
# Test with MCP inspector
npx @anthropic-ai/mcp-inspector python -m mcp_srv.server
```

## Query Patterns (Tested & Working)

### Basic Mapping

| Use Case | Query |
|----------|-------|
| Gene → Protein | `BRCA1 >>ensembl>>uniprot` |
| Gene → Reviewed Protein | `BRCA1 >>ensembl>>uniprot[uniprot.reviewed==true]` |
| Gene → Human Reviewed Protein | `BRCA1 >>ensembl[ensembl.genome=="homo_sapiens"]>>uniprot[uniprot.reviewed==true]` |
| Protein → Pathways | `P04637 >>uniprot>>reactome` |
| Gene → GO Terms | `TP53 >>ensembl>>uniprot>>go` |
| Gene → Transcripts | `TP53 >>ensembl>>transcript` |
| Gene → ClinVar | `BRCA1 >>ensembl>>clinvar` |

### Protein Structures (AlphaFold)

```
Gene → AlphaFold:
BRCA1 >>ensembl[ensembl.genome=="homo_sapiens"]>>uniprot[uniprot.reviewed==true]>>alphafold

Protein → AlphaFold:
P38398 >>uniprot>>alphafold

AlphaFold Lookup:
AF-P38398-F1 >>alphafold
```

### BioGRID Interactions

```
Gene → BioGRID:
TP53 >>ensembl[ensembl.genome=="homo_sapiens"]>>entrez>>biogrid

Entrez → BioGRID:
7157 >>entrez>>biogrid

BioGRID → UniProt Interactors:
7157 >>entrez>>biogrid>>uniprot
```

### dbSNP Variants

```
Gene → dbSNP:
BRCA1 >>ensembl[ensembl.genome=="homo_sapiens"]>>dbsnp

dbSNP Lookup:
rs1801133 >>dbsnp

dbSNP → ClinVar:
rs1801133 >>dbsnp>>ensembl>>clinvar
```

### Drug Discovery (Full ChEMBL Chain)

```
Gene → ChEMBL Target:
EGFR >>ensembl>>uniprot>>chembl_target_component>>chembl_target

Gene → Drugs (full chain):
JAK2 >>ensembl>>uniprot>>chembl_target_component>>chembl_target>>chembl_assay>>chembl_activity>>chembl_molecule

Phase 3+ Drugs Only:
JAK2 >>...>>chembl_molecule[chembl.molecule.highestDevelopmentPhase>2]
```

### Drug Discovery Workflows (via GenCC)

```
Disease → AlphaFold Structures:
glioblastoma >>mondo>>gencc>>ensembl[ensembl.genome=="homo_sapiens"]>>uniprot[uniprot.reviewed==true]>>alphafold

Disease → BioGRID Interactions:
glioblastoma >>mondo>>gencc>>ensembl[ensembl.genome=="homo_sapiens"]>>entrez>>biogrid

Disease → dbSNP Variants:
breast cancer >>mondo>>gencc>>ensembl[ensembl.genome=="homo_sapiens"]>>dbsnp

Disease → IntAct Interactions:
glioblastoma >>mondo>>gencc>>ensembl[ensembl.genome=="homo_sapiens"]>>uniprot[uniprot.reviewed==true]>>intact
```

### GWAS Genetics

```
Disease → GWAS SNPs:
EFO:0000400 >>efo>>gwas

Disease → GWAS Genes:
MONDO:0005148 >>mondo>>efo>>gwas>>ensembl[ensembl.genome=="homo_sapiens"]
```

### Clinical Variants

```
Gene → ClinVar Variants:
BRCA1 >>ensembl>>clinvar

Disease → ClinVar:
MONDO:0015628 >>mondo>>clinvar

Disease → ClinVar Genes:
MONDO:0005148 >>mondo>>clinvar>>ensembl[ensembl.genome=="homo_sapiens"]

ClinVar → HPO Phenotypes:
981341 >>clinvar>>hpo
```

### Protein Interactions

```
Protein → IntAct:
P38398 >>uniprot>>intact

Protein → Interaction Partners:
P38398 >>uniprot>>intact>>uniprot

ChEBI → Protein Targets:
CHEBI:50210 >>chebi>>intact>>uniprot

RNA → Protein Binding:
URS00002D9DEC >>rnacentral>>intact>>uniprot
```

### Antibodies (TheraSAbDab/SAbDab)

```
Antibody Lookup:
BEVACIZUMAB >>antibody

Antibody → Target Genes:
BEVACIZUMAB >>antibody>>ensembl

Gene → Therapeutic Antibodies:
VEGFA >>ensembl>>antibody

PDB → Antibody Data:
7S4S >>pdb>>antibody

Disease → Antibodies:
MONDO:0005015 >>mondo>>antibody
```

### Clinical Trials

```
Trial Lookup:
NCT00720356 >>clinical_trials

Disease Text → Trials:
diabetes >>clinical_trials

Trial → Disease Ontology:
NCT06777108 >>clinical_trials>>mondo

Disease → Clinical Trials:
MONDO:0005044 >>mondo>>clinical_trials

Trial → Gene Targets:
NCT05969704 >>clinical_trials>>mondo>>gencc>>ensembl
```

### PubChem / NCBI

```
PubChem → Bioactivity:
2244 >>pubchem>>pubchem_activity

PubChem → Gene Targets:
2244 >>pubchem>>pubchem_activity>>ensembl

PubChem → HMDB:
2244 >>pubchem>>hmdb

Entrez → Ensembl:
672 >>entrez>>ensembl

Entrez → GO:
BRCA1 >>entrez>>go
```

### Expression

```
Gene → Tissue Expression:
ENSG00000139618 >>ensembl>>bgee

Tissue → Expressed Genes:
UBERON:0000955 >>uberon>>bgee

Tissue → Genes → Functions:
UBERON:0000955 >>uberon>>bgee>>ensembl>>go
```

### Ontology Navigation

```
GO Term → Parents:     GO:0004707 >>go>>goparent
EFO Disease → Children: EFO:0003767 >>efo>>efochild
UBERON → Parents:      UBERON:0000955 >>uberon>>uberonparent
Cell Ontology → Children: CL:0000576 >>cl>>clchild
```

## Filters

Append filters to any dataset:

| Filter | Example |
|--------|---------|
| Reviewed proteins | `>>uniprot[uniprot.reviewed==true]` |
| Human genes | `>>ensembl[ensembl.genome=="homo_sapiens"]` |
| Biological process | `>>go[go.type=="biological_process"]` |
| Phase 3+ drugs | `>>chembl_molecule[chembl.molecule.highestDevelopmentPhase>=3]` |

## Example Results

**Gene to UniProt:**
```
BRCA1 >>ensembl>>uniprot → 29 protein entries
```

**Protein to Reactome:**
```
P04637 >>uniprot>>reactome → 45 pathways
```

**Gene to ClinVar:**
```
BRCA1 >>ensembl>>clinvar → 300+ variants
```

## Available Datasets

**Genes:** ensembl, hgnc, entrez, refseq

**Proteins:** uniprot, uniparc, pdb

**Structures:** alphafold, pdb

**Interactions:** intact, string, biogrid

**Drugs:** chembl_molecule, chembl_target, pubchem, drugcentral, hmdb

**Diseases:** efo, mondo, mesh, hpo, gencc

**Variants:** dbsnp, clinvar, gwas, alphamissense

**Pharmacogenomics:** pharmgkb, pharmgkb_variant, pharmgkb_clinical

**Pathways:** reactome, go, msigdb, rhea

**Expression:** bgee, cellxgene, scxa

**Antibodies:** antibody (TheraSAbDab/SAbDab)

**Clinical Trials:** clinical_trials

**Ontologies:** go, efo, mondo, hpo, uberon, cl, eco, chebi, rnacentral

## Troubleshooting

**"Biobtree not running"**
```bash
./biobtree web
curl http://localhost:9291/ws/meta
```

**"Unknown dataset"**
- Check available datasets with `biobtree_meta`
- Dataset names are lowercase

**"No mappings found"**
- Check identifier spelling
- Try searching first with `biobtree_search`

## Development

```bash
# Quick test
python -c "
import asyncio
from mcp_srv.server import call_tool
import mcp_srv.server as srv
from mcp_srv.biobtree_client import BiobtreeClient

async def test():
    srv.client = BiobtreeClient()
    result = await call_tool('biobtree_map', {'terms': 'BRCA1', 'chain': '>>ensembl>>uniprot'})
    print(result[0].text)
    await srv.client.close()

asyncio.run(test())
"
```
