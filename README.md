# BioBTree v2

A unified biomedical graph database that integrates more than 95 public data sources (genes, proteins, compounds, diseases, pathways, and clinical data) into a single queryable graph of 3.5 billion entries linked by 9.3 billion cross-references. Its built-in MCP server gives LLMs direct access to structured, authoritative, up-to-date biomedical data.

```
BRCA1 >> ensembl >> uniprot >> pdb[resolution<2.0]
```

One line, three databases: find BRCA1 in Ensembl, map to UniProt proteins, and return high-resolution PDB structures.

**Website:** [sugi.bio/biobtree](https://sugi.bio/biobtree/). Live MCP endpoint, REST API, examples, and full [documentation](docs/index.md).

## Knowledge Graph

BioBTree exports a biolink-typed [KGX](https://github.com/biolink/kgx) knowledge graph. A human-scoped, Neo4j-ready subgraph (~47M nodes / ~151M edges) is published on [Zenodo](https://doi.org/10.5281/zenodo.20816741) under CC BY-NC-SA 4.0. See **[docs/kg_export](docs/kg_export/index.md)**.

## Benchmark

LLMs answering GeneTuring, LAB-Bench and variant questions with BioBTree, with web search and without tools: results in **[docs/benchmark](docs/benchmark/index.md)**.

## Publication

**BioBTree: An Integrated Biomedical Graph Database for Researchers and LLM Agents**

Preprint (2026): [https://doi.org/10.5281/zenodo.24144112](https://doi.org/10.5281/zenodo.24144112)

Earlier versions: [preprint](https://zenodo.org/records/18962899) (Zenodo); BioBTree v1: Gür T, [F1000Research 2019](https://doi.org/10.12688/f1000research.17927.1)

## License

**Free for everyone.** Use it live at [sugi.bio](https://sugi.bio), or
self-host it under **AGPL-3.0**.

For commercial or closed-source use, a **commercial license** is available.
Get in touch at **contact@sugi.bio**.
