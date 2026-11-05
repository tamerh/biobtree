"""Gene-set over-representation for an arbitrary gene list.

The model no longer has to map every gene to MSigDB and intersect the sets
itself. One `biobtree_enrich` call fetches the sets for all input genes, then
computes a hypergeometric test and BH-FDR over the MSigDB collection.

The MSigDB gene universe (43,633 human symbols) is the background. Set sizes
are restricted to 5-500 so specific terms (e.g. a 10-gene GO set) are kept;
the Atlas interaction-partner enrichment uses the narrower 15-500 range.
"""

import math
import re
from typing import Any, Dict, Iterable, List, Optional, Tuple

# Unique human gene symbols in MSigDB v2026.1.Hs.
MSIGDB_BACKGROUND = 43633

MIN_SET_SIZE = 5
MAX_SET_SIZE = 500

# Number of sets in each collection that pass the 5-500 size filter. Used as
# the BH denominator; for unknown collections we fall back to the number of
# overlapping sets (a less conservative correction). The Atlas interaction-
# partner enrichment uses 15-500; gene-list enrichment keeps smaller sets so
# specific terms (e.g. a 10-gene GO set) are not discarded.
COLLECTION_TOTALS = {
    "C5:GO": 10127,
    "C5:GO:BP": 7276,
    "C5:GO:MF": 1825,
    "C5:GO:CC": 1026,
    "C2:CP:REACTOME": 1818,
    "C2:CGP": 3401,
    "C5:HPO": 5559,
    "H": 50,
}

_TARGET_SPLIT = re.compile(r"(?<!\\)\|")


def _parse_target(target: str) -> Optional[Tuple[str, str, str, int]]:
    """Parse a compact MSigDB map target: id|name|collection|description|size."""
    parts = _TARGET_SPLIT.split(target)
    if len(parts) < 5:
        return None
    set_id = parts[0].strip()
    name = parts[1].strip()
    collection = parts[2].strip()
    try:
        size = int(parts[-1].strip())
    except ValueError:
        return None
    if not set_id or not name:
        return None
    return set_id, name, collection, size


def _hypergeom_sf(k: int, N: int, K: int, n: int) -> float:
    """P(X >= k) for X ~ Hypergeometric(N, K, n)."""
    if k <= 0:
        return 1.0
    upper = min(K, n)
    if k > upper:
        return 0.0

    log_denom = math.lgamma(N + 1) - math.lgamma(n + 1) - math.lgamma(N - n + 1)
    p = 0.0
    for i in range(k, upper + 1):
        log_p = (
            math.lgamma(K + 1)
            - math.lgamma(i + 1)
            - math.lgamma(K - i + 1)
            + math.lgamma(N - K + 1)
            - math.lgamma(n - i + 1)
            - math.lgamma(N - K - n + i + 1)
            - log_denom
        )
        p += math.exp(log_p)
    return min(1.0, p)


def _bh_fdr(pvals: List[float]) -> List[float]:
    """Benjamini-Hochberg adjusted p-values."""
    m = len(pvals)
    if m == 0:
        return []
    order = sorted(range(m), key=lambda i: pvals[i])
    q = [0.0] * m
    prev = 1.0
    for rank in range(m, 0, -1):
        idx = order[rank - 1]
        val = min(prev, pvals[idx] * m / rank)
        q[idx] = val
        prev = val
    return q


def _normalize_collection(collection: str) -> Tuple[str, str]:
    """Return (base_collection, name_prefix) for a requested collection.

    The map response stores all GO sets as collection "C5:GO"; a request for
    "C5:GO:BP" is narrowed with the GOBP_ name prefix.
    """
    collection = (collection or "C5:GO").strip()
    if collection.startswith("C5:GO:"):
        ns = collection.rsplit(":", 1)[-1].upper()
        prefix = {"BP": "GOBP_", "MF": "GOMF_", "CC": "GOCC_"}.get(ns, "")
        return "C5:GO", prefix
    return collection, ""


def enrich_from_mappings(
    mappings: Iterable[Dict[str, Any]],
    collection: str = "C5:GO",
    limit: int = 20,
    background: int = MSIGDB_BACKGROUND,
) -> Dict[str, Any]:
    """Compute over-representation from biobtree map `mappings`."""
    base_collection, name_prefix = _normalize_collection(collection)

    sets: Dict[str, Dict[str, Any]] = {}
    mapped_genes = set()

    for mapping in mappings or []:
        # `source` is the canonical matched entry (e.g. HGNC:11998|...), while
        # `input` is the user's term. Use the source so TP53 and HGNC:11998
        # count as one gene.
        source = (mapping.get("source") or "").split("|", 1)[0].strip()
        gene = source or (mapping.get("input") or "").strip()
        if not gene:
            continue
        for target in mapping.get("targets") or []:
            parsed = _parse_target(target)
            if not parsed:
                continue
            set_id, name, coll, size = parsed
            if base_collection and not coll.startswith(base_collection):
                continue
            if name_prefix and not name.startswith(name_prefix):
                continue
            if size < MIN_SET_SIZE or size > MAX_SET_SIZE:
                continue
            entry = sets.get(set_id)
            if entry is None:
                entry = {
                    "id": set_id,
                    "name": name,
                    "collection": coll,
                    "set_size": size,
                    "genes": set(),
                }
                sets[set_id] = entry
            entry["genes"].add(gene)
            mapped_genes.add(gene)

    n = len(mapped_genes)
    if n == 0 or not sets:
        return {
            "query": {
                "genes": sorted(mapped_genes),
                "mapped_genes": n,
                "collection": collection,
                "background": background,
            },
            "results": [],
            "tested_sets": 0,
        }

    results: List[Dict[str, Any]] = []
    for entry in sets.values():
        k = len(entry["genes"])
        if k == 0:
            continue
        K = entry["set_size"]
        p = _hypergeom_sf(k, background, K, n)
        fold = (k / n) / (K / background) if K and n else 0.0
        results.append(
            {
                "id": entry["id"],
                "name": entry["name"],
                "collection": entry["collection"],
                "overlap": k,
                "set_size": K,
                "fold": fold,
                "p": p,
            }
        )

    # BH denominator: the number of sets in the collection that pass the size
    # filter, not just the ones hit by this query.
    m = COLLECTION_TOTALS.get(collection, len(results))
    m = max(m, len(results))
    qvals = _bh_fdr([r["p"] for r in results])
    # _bh_fdr used len(results) as m; rescale to the full collection size.
    if m > len(results) and results:
        qvals = [min(1.0, q * m / len(results)) for q in qvals]
    for r, q in zip(results, qvals):
        r["fdr"] = q

    # Rank by fold (specificity), matching the Atlas enrichment; p/FDR are
    # reported so the caller can apply its own significance cutoff.
    results.sort(key=lambda r: (-r["fold"], r["fdr"], -r["overlap"]))
    return {
        "query": {
            "genes": sorted(mapped_genes),
            "mapped_genes": n,
            "collection": collection,
            "background": background,
        },
        "results": results[: max(1, min(int(limit), 100))],
        "tested_sets": len(results),
    }


async def enrich_genes(
    client: Any,
    genes: Any,
    collection: str = "C5:GO",
    limit: int = 20,
) -> Dict[str, Any]:
    """Fetch MSigDB sets for `genes` and compute over-representation."""
    if isinstance(genes, str):
        gene_list = [g.strip() for g in genes.split(",") if g.strip()]
    else:
        gene_list = [str(g).strip() for g in (genes or []) if str(g).strip()]

    if not gene_list:
        return {
            "query": {"genes": [], "mapped_genes": 0, "collection": collection},
            "results": [],
            "tested_sets": 0,
            "message": "No genes supplied.",
        }

    terms = ",".join(gene_list)
    mappings: List[Dict[str, Any]] = []
    not_found: set = set()
    unmapped: set = set()
    page = None
    pages = 0

    while True:
        res = await client.map(terms=terms, chain=">>hgnc>>msigdb", page=page)
        mappings.extend(res.get("mappings") or [])
        # Only the first page's not_found/unmapped is authoritative; later
        # pagination pages repeat the input terms there even after mapping.
        if pages == 0:
            not_found.update(res.get("not_found") or [])
            unmapped.update(res.get("unmapped") or [])
        page = (res.get("pagination") or {}).get("next_token")
        pages += 1
        if not page or pages >= 100:
            break

    out = enrich_from_mappings(mappings, collection=collection, limit=limit)
    out["query"]["inputs"] = gene_list
    out["pages_fetched"] = pages
    if not_found:
        out["not_found"] = sorted(not_found)
    if unmapped:
        out["unmapped"] = sorted(unmapped)
    return out
