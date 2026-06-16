"""Meta-graph (schema view): biolink category --predicate--> category, derived
from the mapping tables (categories.yaml + predicates.yaml + the GO rules). Shows
the big-picture shape of the KG -- what node types exist and how they connect --
independent of any instance data.

    python -m tools.kg_export.metaschema --out kg_meta.html [--print]
"""

from __future__ import annotations

import argparse
from collections import defaultdict

from .categories import CategoryMap
from .predicates import PredicateMap

# GO annotation sources (go.py): subject dataset -> category, and the 3 aspects.
_GO_SOURCES = [("uniprot", "biolink:Protein"), ("ensembl", "biolink:Gene")]
_GO_EDGES = [
    ("biolink:enables", "biolink:MolecularActivity"),
    ("biolink:actively_involved_in", "biolink:BiologicalProcess"),
    ("biolink:located_in", "biolink:CellularComponent"),
]

_PALETTE = [
    "#4e79a7", "#f28e2b", "#59a14f", "#e15759", "#76b7b2", "#edc948",
    "#b07aa1", "#ff9da7", "#9c755f", "#bab0ac", "#86bc86", "#d37295",
    "#7f7f7f", "#17becf", "#bcbd22", "#aec7e8", "#ffbb78", "#98df8a",
]


def schema_triples(cats: CategoryMap, preds: PredicateMap):
    """(subject_category, predicate, object_category) -> set of contributing datasets."""
    triples: dict[tuple, set] = defaultdict(set)

    def add(sc, p, oc, ds):
        if sc and oc:
            triples[(sc, p, oc)].add(ds)

    for key in preds.pairs():
        s, o = key.split(">")
        r = preds.rule_for(s, o)
        if r.is_skip:
            continue
        if r.flip:
            s, o = o, s
        add(cats.category_for(s), r.predicate, cats.category_for(o), f"{key}")

    for ds in preds.reified_datasets():
        r = preds.reified_rule(ds)
        if r.kind in ("pairwise", "star"):
            c = cats.category_for(r.partner)
            add(c, r.predicate, c, ds)
        else:  # bipartite (object resolved via `via`/symbol is still rule.object)
            add(cats.category_for(r.subject), r.predicate, cats.category_for(r.object), ds)

    for src_ds, sc in _GO_SOURCES:
        for p, oc in _GO_EDGES:
            add(sc, p, oc, f"go({src_ds})")
    return triples


def render_html(triples, out_html):
    from pyvis.network import Network
    cats = sorted({c for (s, _, o) in triples for c in (s, o)})
    color = {c: _PALETTE[i % len(_PALETTE)] for i, c in enumerate(cats)}
    net = Network(height="900px", width="100%", directed=True, bgcolor="#ffffff")
    net.barnes_hut(spring_length=240)
    for c in cats:
        net.add_node(c, label=c.split(":")[1], title=c, color=color[c], size=24)
    for (s, p, o), ds in sorted(triples.items()):
        net.add_edge(s, o, label=p.split(":")[1], title=f"{p}  ({len(ds)} datasets: {', '.join(sorted(ds))})", arrows="to")
    net.write_html(out_html, notebook=False, open_browser=False)


def print_summary(triples):
    by_subj = defaultdict(list)
    for (s, p, o), ds in triples.items():
        by_subj[s].append((p, o, ds))
    cats = sorted({c for (s, _, o) in triples for c in (s, o)})
    print(f"NODE TYPES ({len(cats)}): " + ", ".join(c.split(':')[1] for c in cats))
    print(f"SCHEMA EDGES ({len(triples)} category->predicate->category):\n")
    for s in sorted(by_subj):
        for p, o, ds in sorted(by_subj[s]):
            print(f"  {s.split(':')[1]:>22} --{p.split(':')[1]}--> {o.split(':')[1]}  [{len(ds)}]")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--categories", default="mappings/categories.yaml")
    ap.add_argument("--predicates", default="mappings/predicates.yaml")
    ap.add_argument("--out", default=None, help="output HTML path")
    ap.add_argument("--print", action="store_true", dest="show")
    a = ap.parse_args()
    cats = CategoryMap.load(a.categories)
    preds = PredicateMap.load(a.predicates)
    triples = schema_triples(cats, preds)
    if a.show or not a.out:
        print_summary(triples)
    if a.out:
        render_html(triples, a.out)
        print(f"\nwrote {a.out}: {len({c for (s,_,o) in triples for c in (s,o)})} node types, {len(triples)} schema edges")


if __name__ == "__main__":
    main()
