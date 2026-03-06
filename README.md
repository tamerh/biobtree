# 🧬 BioBTree v2

**A unified biomedical database that connects 50+ primary data sources and makes them queryable by both researchers and AI.**

BioBTree v2 integrates genes, proteins, chemical compounds, diseases, pathways, variants, expression data, and more into a single graph with billions of cross-reference edges. Instead of navigating dozens of databases with different interfaces and identifiers, you write one query that traverses them all:

```
BRCA1 >> ensembl >> uniprot >> pdb[resolution<2.0]
```

This finds BRCA1 in Ensembl, maps to UniProt proteins, and returns high-resolution PDB structures — crossing three databases in a single line.

## 🔗 Try It

The fastest way to experience BioBTree v2 is through an AI assistant with MCP (Model Context Protocol). We recommend **Claude CLI** (tested extensively), though **Codex CLI** and **Gemini CLI** also work:

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

## 🤝 Collaboration

We are looking for an academic lab — preferably in Germany — to collaborate on expanding BioBTree v2. A preprint is available; we would submit to a peer-reviewed journal together with collaborating partners. If you're interested, please reach out: **tamer.gur07@gmail.com**

## 📖 Documentation

Query syntax, [integrated databases](docs/datasets/index.md) (50+), MCP server setup, and self-hosting: **[docs/](docs/index.md)**

## 📄 Publication

**BioBTree v2: Grounding LLM Responses with Large-Scale Structured Biomedical Data**
Preprint: [link forthcoming]
BioBTree v1: [F1000Research](https://f1000research.com/articles/8-145)

## ⚖️ License

GPL-v3
