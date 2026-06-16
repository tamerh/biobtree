"""CURIE rendering for BioBTree raw identifiers.

Raw ids come in two shapes (verified against real index files):
  * already prefixed: ``HGNC:100``, ``MONDO:0000001``, ``CHEBI:100``, ``HP:...``
  * bare:            ``1`` (entrez), ``CHEMBL1``, ``ENSG...``, ``CVCL_0001``

Rule: if the raw id already starts with ``<prefix>:`` (case-insensitive), keep it
as-is; otherwise prepend ``<prefix>:``.
"""

from __future__ import annotations


def to_curie(prefix: str, local_id: str) -> str:
    """Render a biolink CURIE from a dataset prefix and a raw BioBTree id.

    - already prefixed in this namespace (``HGNC:100``)  -> normalize casing
    - carries a *foreign* prefix (``CHEBI:1`` under HMDB) -> keep as-is, never
      double-prefix (avoids invalid ``HMDB:CHEBI:1``)
    - bare (``41``)                                       -> prepend the prefix
    """
    local_id = local_id.strip()
    if not prefix:
        return local_id
    if local_id.lower().startswith(prefix.lower() + ":"):
        return prefix + ":" + local_id.split(":", 1)[1]
    if ":" in local_id:
        # Already a CURIE in some other namespace; do not double-prefix.
        return local_id
    return f"{prefix}:{local_id}"


def looks_foreign_curie(prefix: str, local_id: str) -> bool:
    """True if the id carries a ':' but NOT our prefix (worth logging)."""
    return ":" in local_id and not local_id.lower().startswith(prefix.lower() + ":")
