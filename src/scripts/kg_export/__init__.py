"""BioBTree → Knowledge Graph (KGX/biolink) exporter.

Standalone batch tool that turns BioBTree's post-build sorted index files
(``<dataset>_sorted.<chunk>.index.gz``) into a biolink-typed, normalized
knowledge graph. It does not touch the Go core, the query service, or the MCP
server at runtime.

See ``docs/kg_export/plan.md`` for the design.
"""

__all__ = ["datasets", "categories"]
