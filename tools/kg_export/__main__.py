"""kg_export CLI.

Usage:
    python -m tools.kg_export nodes \\
        --index-dir /data/biobtree/out_prod/main/index \\
        --conf conf --categories mappings/categories.yaml \\
        --out out/kg/nodes.tsv --stats out/kg/nodes.stats.json \\
        [--datasets hgnc,ensembl,entrez] [--max-lines N]
"""

from __future__ import annotations

import argparse
import sys
import time
from pathlib import Path

from .categories import CategoryMap
from .datasets import DatasetRegistry
from .edges import build_edges, load_id_map
from .nodes import build_nodes
from .predicates import PredicateMap


def _cmd_nodes(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    datasets = (
        [d.strip() for d in args.datasets.split(",") if d.strip()]
        if args.datasets
        else None
    )
    t0 = time.time()
    stats = build_nodes(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        out_path=args.out,
        stats_path=args.stats,
        id_map_path=args.id_map,
        datasets=datasets,
        max_lines=args.max_lines,
    )
    dt = time.time() - t0
    print(f"nodes.tsv written: {args.out}", file=sys.stderr)
    print(
        f"  files={stats.files_scanned} lines={stats.lines:,} "
        f"edges={stats.edge_lines:,} props={stats.property_lines:,}",
        file=sys.stderr,
    )
    print(
        f"  node_candidates={stats.node_candidates:,} "
        f"nodes_written={stats.nodes_written:,} merges={stats.merges:,} "
        f"multi_clusters={stats.multi_member_clusters:,} "
        f"names={stats.names_found:,}",
        file=sys.stderr,
    )
    print(f"  by_category={dict(stats.by_category)}", file=sys.stderr)
    if stats.mixed_category_clusters:
        print(
            f"  WARNING mixed_category_clusters={stats.mixed_category_clusters}",
            file=sys.stderr,
        )
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_edges(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    predicates = PredicateMap.load(args.predicates)
    id_map = load_id_map(args.id_map)
    datasets = (
        [d.strip() for d in args.datasets.split(",") if d.strip()]
        if args.datasets
        else None
    )
    t0 = time.time()
    stats = build_edges(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        predicates=predicates,
        out_path=args.out,
        id_map=id_map,
        stats_path=args.stats,
        datasets=datasets,
        max_lines=args.max_lines,
    )
    dt = time.time() - t0
    print(f"edges.tsv written: {args.out}", file=sys.stderr)
    print(
        f"  files={stats.files_scanned} lines={stats.lines:,} "
        f"id_map={len(id_map):,}",
        file=sys.stderr,
    )
    print(
        f"  edges_written={stats.edges_written:,} skipped={stats.skipped:,} "
        f"unmapped={stats.unmapped:,} dropped_not_node={stats.dropped_not_node:,}",
        file=sys.stderr,
    )
    print(f"  by_predicate={dict(stats.by_predicate)}", file=sys.stderr)
    if stats.unmapped_pairs:
        top = sorted(stats.unmapped_pairs.items(), key=lambda kv: -kv[1])[:8]
        print(f"  top unmapped pairs={top}", file=sys.stderr)
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="kg_export")
    sub = parser.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("nodes", help="build KGX nodes.tsv")
    p.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    p.add_argument("--conf", default="conf", help="dataset config dir")
    p.add_argument(
        "--categories", default="mappings/categories.yaml", help="category map"
    )
    p.add_argument("--out", required=True, help="output nodes.tsv path")
    p.add_argument("--stats", default=None, help="output merge-stats JSON path")
    p.add_argument("--id-map", default=None, help="output member->canonical map")
    p.add_argument("--datasets", default=None, help="comma list to restrict files")
    p.add_argument("--max-lines", type=int, default=None, help="cap lines (debug)")
    p.set_defaults(func=_cmd_nodes)

    e = sub.add_parser("edges", help="build KGX edges.tsv")
    e.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    e.add_argument("--conf", default="conf", help="dataset config dir")
    e.add_argument("--categories", default="mappings/categories.yaml")
    e.add_argument("--predicates", default="mappings/predicates.yaml")
    e.add_argument("--out", required=True, help="output edges.tsv path")
    e.add_argument("--id-map", default=None, help="Phase 1 member->canonical map")
    e.add_argument("--stats", default=None, help="output edge-stats JSON path")
    e.add_argument("--datasets", default=None, help="comma list to restrict files")
    e.add_argument("--max-lines", type=int, default=None, help="cap lines (debug)")
    e.set_defaults(func=_cmd_edges)

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
