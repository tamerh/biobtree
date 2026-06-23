"""Seed gap-discovery: for a set of seed entities, ask the BioBTree service which
datasets actually connect to them, then classify each as COVERED (our KG maps it)
or a GAP (present in BioBTree, missing from the KG). Drives the iterate-to-
completeness loop for a representative subgraph.

    python -m kg_export.seed_gaps --symbols BRCA1,TP53,... [--url ...]
"""

from __future__ import annotations

from pathlib import Path as _P
_MAP = _P(__file__).resolve().parent / "mappings"
import argparse
import json
import urllib.parse
import urllib.request
from collections import defaultdict

from .categories import CategoryMap
from .predicates import PredicateMap


def _get(url):
    with urllib.request.urlopen(url, timeout=30) as r:
        return json.load(r)


def resolve_hgnc(base, symbol):
    url = f"{base}/ws/search/?i={urllib.parse.quote(symbol)}&s=hgnc&mode=lite"
    for row in _get(url).get("data", []):
        # lite search: id|dataset|name|xref_count
        parts = row.split("|")
        if len(parts) >= 2 and parts[1] == "hgnc":
            return parts[0]
    return None


def entry_xref_counts(base, ident, dataset):
    url = f"{base}/ws/entry/?i={urllib.parse.quote(ident)}&s={dataset}"
    d = _get(url)
    return {x.split("|")[0]: int(x.split("|")[1])
            for x in d.get("xrefs", {}).get("data", [])}, d.get("Attributes")


def covered_datasets(categories: CategoryMap, predicates: PredicateMap) -> set:
    cov = set(categories.datasets())
    for key in predicates.pairs():
        s, t = key.split(">")
        cov.add(s); cov.add(t)
    for ds in predicates.reified_datasets():
        r = predicates.reified_rule(ds)
        cov.add(ds)
        for role in (r.partner, r.subject, r.object):
            if role:
                cov.add(role)
    return cov


# non-entity targets that are intentionally not KG nodes (ids/citations/etc.)
_NON_ENTITY = {
    "pubmed", "doi", "ena", "refseq", "ccds", "vega", "proteomes", "patent",
    "patent_compound", "pubchem_assay", "chembl_assay", "chembl_document",
    "textsearch", "entry", "literature_mappings", "medgen", "umls", "mesh",
    "ncit", "icd9", "icd10cm", "icd11", "sctid", "omim", "neighborentrez",
    "orthologentrez", "relatedentrez",
}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--symbols", required=True, help="comma gene symbols")
    ap.add_argument("--url", default="http://localhost:9291")
    ap.add_argument("--conf", default="conf")
    ap.add_argument("--categories", default=str(_MAP / "categories.yaml"))
    ap.add_argument("--predicates", default=str(_MAP / "predicates.yaml"))
    a = ap.parse_args()

    cats = CategoryMap.load(a.categories)
    preds = PredicateMap.load(a.predicates)
    cov = covered_datasets(cats, preds)

    symbols = [s.strip() for s in a.symbols.split(",") if s.strip()]
    total = defaultdict(int)        # dataset -> summed xref count
    seen_in = defaultdict(int)      # dataset -> # seeds connecting to it
    resolved = []
    for sym in symbols:
        hid = resolve_hgnc(a.url, sym)
        if not hid:
            print(f"  ! could not resolve {sym}")
            continue
        resolved.append((sym, hid))
        counts, _ = entry_xref_counts(a.url, hid, "hgnc")
        for ds, n in counts.items():
            total[ds] += n
            seen_in[ds] += 1

    print(f"\nseeds resolved: {len(resolved)}/{len(symbols)}")
    rows = sorted(total.items(), key=lambda kv: -kv[1])
    covered, gaps, non_entity = [], [], []
    for ds, n in rows:
        if ds in _NON_ENTITY:
            non_entity.append((ds, n, seen_in[ds]))
        elif ds in cov:
            covered.append((ds, n, seen_in[ds]))
        else:
            gaps.append((ds, n, seen_in[ds]))

    def show(title, items):
        print(f"\n=== {title} ({len(items)}) ===")
        for ds, n, s in items:
            print(f"  {ds:24} edges={n:<9} seeds={s}")

    show("GAPS — connected to seeds but NOT in the KG", gaps)
    show("COVERED — datasets the KG already maps", covered)
    show("NON-ENTITY targets (ids/citations, intentionally excluded)", non_entity)


if __name__ == "__main__":
    main()
