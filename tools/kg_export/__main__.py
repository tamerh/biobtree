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
import json
import sys
import time
from pathlib import Path

from .categories import CategoryMap
from .datasets import DatasetRegistry
from . import kgx
from .edges import build_edges, load_id_map
from .go import build_go
from .nodes import build_nodes
from .predicates import PredicateMap
from .reified import build_reified_edges


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


def _cmd_reified(args: argparse.Namespace) -> int:
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
    stats = build_reified_edges(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        predicates=predicates,
        out_path=args.out,
        id_map=id_map,
        stats_path=args.stats,
        datasets=datasets,
    )
    dt = time.time() - t0
    print(f"reified edges.tsv written: {args.out}", file=sys.stderr)
    print(
        f"  datasets={stats.datasets_processed} groups={stats.groups:,} "
        f"lines={stats.lines:,} id_map={len(id_map):,}",
        file=sys.stderr,
    )
    print(
        f"  edges_written={stats.edges_written:,} self_loops={stats.self_loops:,} "
        f"oversized_groups={stats.oversized_groups:,}",
        file=sys.stderr,
    )
    print(f"  by_dataset={dict(stats.by_dataset)}", file=sys.stderr)
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_go(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    id_map = load_id_map(args.id_map)
    sources = tuple(d.strip() for d in args.sources.split(",") if d.strip())
    t0 = time.time()
    stats = build_go(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        nodes_out=args.nodes_out,
        edges_out=args.edges_out,
        id_map=id_map,
        stats_path=args.stats,
        annotation_sources=sources,
    )
    dt = time.time() - t0
    print(f"GO nodes: {args.nodes_out}  edges: {args.edges_out}", file=sys.stderr)
    print(
        f"  terms={stats.terms:,} by_aspect={dict(stats.terms_by_aspect)}",
        file=sys.stderr,
    )
    print(
        f"  nodes={stats.nodes_written:,} edges={stats.edges_written:,} "
        f"missing_aspect={stats.edges_missing_aspect:,}",
        file=sys.stderr,
    )
    print(f"  by_predicate={dict(stats.by_predicate)}", file=sys.stderr)
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_assemble(args: argparse.Namespace) -> int:
    out_dir = Path(args.out_dir)
    node_inputs = [s.strip() for s in args.nodes.split(",") if s.strip()]
    edge_inputs = [s.strip() for s in args.edges.split(",") if s.strip()]
    nodes_tsv = out_dir / "nodes.tsv"
    edges_tsv = out_dir / "edges.tsv"

    t0 = time.time()
    n_nodes = kgx.merge_nodes(node_inputs, nodes_tsv)
    edge_dedup = kgx.merge_edges(edge_inputs, edges_tsv)
    n_edges = edge_dedup["written"]
    stub_info = None
    if args.stub_nodes:
        categories = CategoryMap.load(args.categories)
        stub_info = kgx.add_stub_nodes(nodes_tsv, edges_tsv, categories)
        n_nodes += stub_info["stubs_added"]
    kgx.nodes_to_jsonl(nodes_tsv, out_dir / "nodes.jsonl")
    kgx.edges_to_jsonl(edges_tsv, out_dir / "edges.jsonl")
    report = kgx.validate(nodes_tsv, edges_tsv)
    mani = kgx.manifest(nodes_tsv, edges_tsv, args.data_version, report)
    mani["edge_dedup"] = edge_dedup
    if stub_info is not None:
        mani["stub_nodes"] = stub_info
    # publish gate: stamp the dump so an invalid graph can't be mistaken for a
    # release, and exit non-zero.
    mani["status"] = "VALID" if report["ok"] else "INVALID"
    (out_dir / "manifest.json").write_text(json.dumps(mani, indent=2))
    dt = time.time() - t0

    print(f"assembled KGX dump in {out_dir}  status={mani['status']}", file=sys.stderr)
    print(
        f"  nodes={n_nodes:,} edges={n_edges:,} "
        f"(deduped {edge_dedup['removed']:,} of {edge_dedup['input']:,})",
        file=sys.stderr,
    )
    if stub_info is not None:
        print(
            f"  stub_nodes={stub_info['stubs_added']:,} "
            f"untyped_endpoints={stub_info['untyped_endpoints']:,} "
            f"{stub_info['by_category']}",
            file=sys.stderr,
        )
    print(f"  validation={report}", file=sys.stderr)
    print(f"  node_categories={mani['node_categories']}", file=sys.stderr)
    if not report["ok"]:
        print(
            "  WARNING: validation FAILED — not a publishable release. "
            "Re-run nodes with full dataset coverage.",
            file=sys.stderr,
        )
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0 if report["ok"] else 2


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

    r = sub.add_parser("reified", help="build reified KGX edges.tsv")
    r.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    r.add_argument("--conf", default="conf", help="dataset config dir")
    r.add_argument("--categories", default="mappings/categories.yaml")
    r.add_argument("--predicates", default="mappings/predicates.yaml")
    r.add_argument("--out", required=True, help="output edges.tsv path")
    r.add_argument("--id-map", default=None, help="Phase 1 member->canonical map")
    r.add_argument("--stats", default=None, help="output edge-stats JSON path")
    r.add_argument("--datasets", default=None, help="comma list to restrict")
    r.set_defaults(func=_cmd_reified)

    g = sub.add_parser("go", help="build GO nodes + annotation edges")
    g.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    g.add_argument("--conf", default="conf", help="dataset config dir")
    g.add_argument("--categories", default="mappings/categories.yaml")
    g.add_argument("--nodes-out", required=True, help="output GO nodes.tsv")
    g.add_argument("--edges-out", required=True, help="output GO edges.tsv")
    g.add_argument("--id-map", default=None, help="Phase 1 member->canonical map")
    g.add_argument("--stats", default=None, help="output stats JSON path")
    g.add_argument("--sources", default="uniprot,ensembl", help="annotation subjects")
    g.set_defaults(func=_cmd_go)

    a = sub.add_parser("assemble", help="merge -> JSONL -> validate -> manifest")
    a.add_argument("--nodes", required=True, help="comma list of node TSVs")
    a.add_argument("--edges", required=True, help="comma list of edge TSVs")
    a.add_argument("--out-dir", required=True, help="output dir for the KGX dump")
    a.add_argument("--data-version", default=None, help="biobtree data release tag")
    a.add_argument("--categories", default="mappings/categories.yaml")
    a.add_argument("--stub-nodes", action="store_true",
                   help="emit minimal nodes for edge endpoints lacking one")
    a.set_defaults(func=_cmd_assemble)

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
