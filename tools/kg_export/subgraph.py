"""Seed-driven published subgraph: an induced, trimmed projection of the full dump.

A *working snapshot* of the full graph -- every node category and edge predicate
present, just smaller: the taxon-dependent spine (genes/proteins/transcripts/...) is
scoped to one organism, and the big relational datasets are capped per node. It runs
as a post-filter over the assembled full `nodes.tsv` + `edges.tsv`; node attributes /
synonyms come back for free by re-running `assemble` on the filtered TSVs with the
same `--node-attributes` tables (they only attach to nodes that survive).

Passes (all streaming; in-memory state is just the kept-node sets + cap counters,
bounded by the *small* output, not the billion-row input):

 1. human set      -- subjects of `in_taxon -> <taxon>` edges (+ HGNC genes).
 2. base spine     -- every FULL-category node, plus SCOPED-category nodes that are
                      human. (Unknown categories are kept, to be safe.)
 3. edges          -- keep an edge anchored on the spine, up to a per-(subject,
                      predicate) cap for big sources (uncapped for `full_sources`/
                      cap 0); each kept edge pulls BOTH endpoints into the node set
                      (so capped compounds / variants / cross-species genes appear).
 4. nodes          -- emit the spine + pulled-in endpoints.

Config: mappings/subgraph.yaml. Completeness is asserted against the full manifest
(every category/predicate that exists upstream must survive the trim).
"""

from __future__ import annotations

import json
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

import yaml

from . import kgx


def load_config(path: str | Path) -> dict:
    return yaml.safe_load(Path(path).read_text()) or {}


@dataclass
class SubgraphStats:
    human_nodes: int = 0
    spine_nodes: int = 0
    nodes_out: int = 0
    edges_in: int = 0
    edges_out: int = 0
    capped_dropped: int = 0
    unanchored_dropped: int = 0
    by_predicate: dict = field(default_factory=lambda: defaultdict(int))
    by_category: dict = field(default_factory=lambda: defaultdict(int))

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_predicate"] = dict(self.by_predicate)
        d["by_category"] = dict(self.by_category)
        return d


def _source_name(primary: str) -> str:
    return primary.split(":", 1)[1] if primary.startswith("infores:") else primary


def build_subgraph(
    nodes_tsv: str | Path,
    edges_tsv: str | Path,
    config: dict,
    out_nodes: str | Path,
    out_edges: str | Path,
    stats_path: str | Path | None = None,
) -> SubgraphStats:
    nodes_tsv, edges_tsv = Path(nodes_tsv), Path(edges_tsv)
    out_nodes, out_edges = Path(out_nodes), Path(out_edges)
    out_nodes.parent.mkdir(parents=True, exist_ok=True)
    stats = SubgraphStats()

    taxon = config.get("taxon", "NCBITaxon:9606")
    full_cats = set(config.get("full_categories") or [])
    scoped_cats = set(config.get("scoped_categories") or [])
    full_sources = set(config.get("full_sources") or [])
    caps = dict(config.get("caps") or {})
    default_cap = config.get("default_cap", 100)

    def node_cols(row: str):
        p = row.split("\t")
        return p[0], (p[1] if len(p) > 1 else "")

    # --- pass 1: human entity nodes (in_taxon -> taxon; + HGNC genes) -----------
    human: set[str] = set()
    for _, row in kgx._read_rows(edges_tsv):
        if not row:
            continue
        p = row.split("\t")
        if len(p) >= 4 and p[2] == "biolink:in_taxon" and p[3] == taxon:
            human.add(p[1])
    stats.human_nodes = len(human)

    # --- pass 2: base spine -----------------------------------------------------
    spine: set[str] = set()
    for _, row in kgx._read_rows(nodes_tsv):
        if not row:
            continue
        nid, cat = node_cols(row)
        if cat in full_cats or not cat:
            spine.add(nid)
        elif cat in scoped_cats:
            if nid in human or nid.startswith("HGNC:"):
                spine.add(nid)
        else:  # unknown category -> keep (be safe; flagged by completeness check)
            spine.add(nid)
    stats.spine_nodes = len(spine)

    # --- pass 3: edges (anchored on spine, per-(subject,predicate) capped) -------
    keep_nodes: set[str] = set(spine)
    cap_count: dict = defaultdict(int)
    with kgx.xopen(out_edges, "wt") as eout:
        eout.write(kgx.EDGE_HEADER + "\n")
        for _, row in kgx._read_rows(edges_tsv):
            if not row:
                continue
            p = row.split("\t")
            if len(p) < 5:
                continue
            stats.edges_in += 1
            subj, pred, obj, primary = p[1], p[2], p[3], p[4]
            if subj not in spine and obj not in spine:
                stats.unanchored_dropped += 1
                continue
            src = _source_name(primary)
            cap = 0 if src in full_sources else caps.get(src, default_cap)
            if cap:  # cap > 0 -> bounded; 0 -> keep all
                key = (subj, pred)
                if cap_count[key] >= cap:
                    stats.capped_dropped += 1
                    continue
                cap_count[key] += 1
            eout.write(row + "\n")
            stats.edges_out += 1
            stats.by_predicate[pred] += 1
            keep_nodes.add(subj)
            keep_nodes.add(obj)

    # --- pass 4: emit the kept nodes -------------------------------------------
    with kgx.xopen(out_nodes, "wt") as nout:
        nout.write(kgx.NODE_HEADER + "\n")
        for _, row in kgx._read_rows(nodes_tsv):
            if not row:
                continue
            nid, cat = node_cols(row)
            if nid in keep_nodes:
                nout.write(row + "\n")
                stats.nodes_out += 1
                stats.by_category[cat] += 1

    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats


def check_completeness(stats: SubgraphStats, full_manifest: dict) -> dict:
    """Every category/predicate in the full graph must survive the trim, else the
    snapshot isn't faithful (a small dataset was accidentally dropped)."""
    full_cats = set((full_manifest.get("node_categories") or {}).keys())
    full_preds = set((full_manifest.get("edge_predicates") or {}).keys())
    missing_cats = sorted(full_cats - set(stats.by_category))
    missing_preds = sorted(full_preds - set(stats.by_predicate))
    return {
        "ok": not missing_cats and not missing_preds,
        "missing_categories": missing_cats,
        "missing_predicates": missing_preds,
    }
