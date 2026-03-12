# 🧬 BioBTree v2

****A unified biomedical graph database that integrates 50+ primary data sources — genes, proteins, compounds, diseases, pathways, and clinical data — into a single queryable graph with billions of cross-reference edges. Its native MCP server gives LLMs direct access to structured, authoritative biomedical data, complementing their reasoning with reliable identifiers and up-to-date database content.****

```
BRCA1 >> ensembl >> uniprot >> pdb[resolution<2.0]
```

This finds BRCA1 in Ensembl, maps to UniProt proteins, and returns high-resolution PDB structures — crossing three databases in a single line.

    
## 🔗 Try It

The fastest way to experience BioBTree v2 is through MCP. We recommend **Claude CLI** (tested extensively), though **Codex CLI** and **Gemini CLI** also work:

```json
{
  "mcpServers": {
    "biobtree": {
      "type": "http",
      "url": "https://sugi.bio/biobtree/mcp"
    }
  }
}
```

Once connected, just ask questions in natural language — the AI will query BioBTree automatically:

> 💊 *"What tissues express SCN9A most highly? Are there safety concerns for a Nav1.7 inhibitor?"*

> 🧪 *"How many ClinVar variants does BRCA1 have? How many are pathogenic?"*

> 🎯 *"What are all the protein targets of Alectinib with IC50 values?"*

A REST API is also available for direct programmatic access:

```
https://sugi.bio/biobtree/api/ws/?i=BRCA1
https://sugi.bio/biobtree/api/ws/map/?i=BRCA1&m=>>ensembl>>uniprot>>chembl_target
https://sugi.bio/biobtree/api/ws/entry/?i=P38398&s=uniprot
```

## 📖 Documentation

Query syntax, [integrated databases](docs/datasets/index.md), development, and other details check the : **[docs/](docs/index.md)** or refer to latest preprint. 

For any questions, issues, or collaboration ideas, feel free to [create an issue](https://github.com/tamerh/biobtree/issues) or reach out at **tamer.gur07@gmail.com**.

## 📄 Publication

**BioBTree v2: Grounding LLM Responses with Large-Scale Structured Biomedical Data**

The full preprint manuscript, detailing the graph architecture and comparative LLM use cases is available on Zenodo: [https://zenodo.org/records/18962899](https://zenodo.org/records/18962899)

BioBTree v1: [F1000Research](https://f1000research.com/articles/8-145)

## ⚖️ License

AGPL-v3  

The public MCP server hosted at sugi.bio is for academic and personal use only due to upstream non-commercial data licenses.
