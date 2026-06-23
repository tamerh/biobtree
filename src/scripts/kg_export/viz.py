"""Build an interactive HTML view of a KGX subgraph (pyvis).

Caps the graph to stay responsive in a browser. Colors nodes by biolink category,
labels by name, shows the predicate on edge hover.

    python -m kg_export.viz --nodes nodes.tsv --edges edges.tsv \
        --out kg.html [--seeds HGNC:1100,HGNC:11998,CHEMBL25] [--per-seed 80]
"""

from __future__ import annotations

import argparse
import csv

from pyvis.network import Network

_PALETTE = [
    "#4e79a7", "#f28e2b", "#59a14f", "#e15759", "#76b7b2", "#edc948",
    "#b07aa1", "#ff9da7", "#9c755f", "#bab0ac", "#86bc86", "#d37295",
]


def _load_nodes(path):
    nodes = {}
    with open(path) as f:
        r = csv.DictReader(f, delimiter="\t")
        for row in r:
            nodes[row["id"]] = (row.get("name") or row["id"], row.get("category", ""))
    return nodes


def build(nodes_tsv, edges_tsv, out_html, seeds, per_seed, max_edges):
    nodes = _load_nodes(nodes_tsv)
    seedset = set(seeds)

    # collect edges touching a seed, capped per seed for balance
    per = {s: 0 for s in seedset}
    chosen = []
    with open(edges_tsv) as f:
        r = csv.DictReader(f, delimiter="\t")
        for e in r:
            s, o = e["subject"], e["object"]
            hit = s if s in seedset else (o if o in seedset else None)
            if hit is None or per[hit] >= per_seed:
                continue
            per[hit] += 1
            chosen.append((s, e["predicate"], o))
            if len(chosen) >= max_edges:
                break

    used = {x for s, _, o in chosen for x in (s, o)}
    cats = sorted({nodes.get(n, ("", ""))[1] for n in used})
    color = {c: _PALETTE[i % len(_PALETTE)] for i, c in enumerate(cats)}

    net = Network(height="900px", width="100%", directed=True, bgcolor="#ffffff")
    net.barnes_hut(spring_length=180)
    for n in used:
        name, cat = nodes.get(n, (n, ""))
        net.add_node(
            n, label=name, title=f"{n}\n{cat}", color=color.get(cat, "#bab0ac"),
            size=26 if n in seedset else 12,
        )
    for s, pred, o in chosen:
        if s in used and o in used:
            net.add_edge(s, o, title=pred, arrows="to")
    net.write_html(out_html, notebook=False, open_browser=False)
    legend = ", ".join(f"{c.split(':')[-1]}={color[c]}" for c in cats if c)
    print(f"wrote {out_html}: {len(used)} nodes, {len(chosen)} edges")
    print(f"categories: {legend}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--nodes", required=True)
    ap.add_argument("--edges", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--seeds", default="HGNC:1100,HGNC:11998,CHEMBL25,MONDO:0007254,UniProtKB:P38398")
    ap.add_argument("--per-seed", type=int, default=80)
    ap.add_argument("--max-edges", type=int, default=600)
    a = ap.parse_args()
    build(a.nodes, a.edges, a.out,
          [s.strip() for s in a.seeds.split(",") if s.strip()],
          a.per_seed, a.max_edges)


if __name__ == "__main__":
    main()
