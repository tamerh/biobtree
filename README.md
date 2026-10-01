# BioBTree v2

A unified biomedical graph database that integrates 80+ primary data sources — genes, proteins, compounds, diseases, pathways, and clinical data — into a single queryable graph with billions of cross-reference edges. Its native MCP server gives LLMs direct access to structured, authoritative, up-to-date biomedical data.

```
BRCA1 >> ensembl >> uniprot >> pdb[resolution<2.0]
```

One line, three databases: find BRCA1 in Ensembl, map to UniProt proteins, and return high-resolution PDB structures.

**Website:** [sugi.bio/biobtree](https://sugi.bio/biobtree/) — live MCP endpoint, REST API, examples, and full [documentation](docs/index.md).

## Knowledge Graph

BioBTree exports a biolink-typed [KGX](https://github.com/biolink/kgx) knowledge graph. A human-scoped, Neo4j-ready subgraph (~40M nodes / ~132M edges) is published on Zenodo under CC BY-NC-SA 4.0. See **[docs/kg_export](docs/kg_export/index.md)**.

## Publication

**BioBTree v2: Grounding LLM Responses with Large-Scale Structured Biomedical Data**

The preprint — detailing the graph architecture and comparative LLM use cases — is on Zenodo: [https://zenodo.org/records/18962899](https://zenodo.org/records/18962899)

BioBTree v1: [F1000Research](https://f1000research.com/articles/8-145)

## License

**Free for everyone** — use it live at [sugi.bio](https://sugi.bio), or
self-host it under **AGPL-3.0**.

For commercial or closed-source use, a **commercial license** is available —
get in touch at **tamer.gur07@gmail.com**.
