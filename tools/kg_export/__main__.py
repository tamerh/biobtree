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
from .attributes import build_attributes, load_attributes, merge_attr_dict, load_config as load_attr_config
from .dbsnp import build_dbsnp
from .mesh import build_mesh
from .nodes import build_nodes
from .ontology import build_ontology
from .predicates import PredicateMap
from .nodeattrs import build_node_attributes, load_config as load_nodeattr_config
from .refseq import build_refseq
from .reified import build_reified_edges
from .structure import build_structure
from .showcase import build_showcase, load_config as load_showcase_config
from .subgraph import build_subgraph, check_completeness, load_config as load_subgraph_config


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


def _cmd_dbsnp(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    id_map = load_id_map(args.id_map)
    t0 = time.time()
    stats = build_dbsnp(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        nodes_out=args.nodes_out,
        edges_out=args.edges_out,
        id_map=id_map,
        stats_path=args.stats,
        max_variants=args.max_variants,
    )
    dt = time.time() - t0
    print(f"dbSNP nodes: {args.nodes_out}  edges: {args.edges_out}", file=sys.stderr)
    print(
        f"  variants={stats.variants:,} nodes={stats.nodes_written:,} "
        f"edges={stats.edges_written:,}",
        file=sys.stderr,
    )
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_mesh(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    t0 = time.time()
    stats = build_mesh(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        nodes_out=args.nodes_out,
        edges_out=args.edges_out,
        stats_path=args.stats,
    )
    dt = time.time() - t0
    print(f"MeSH disease nodes: {args.nodes_out}  edges: {args.edges_out}", file=sys.stderr)
    print(
        f"  descriptors={stats.descriptors:,} disease_nodes={stats.disease_nodes:,} "
        f"close_match={stats.close_match_edges:,} elapsed={dt:.1f}s",
        file=sys.stderr,
    )
    return 0


def _cmd_refseq(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    id_map = load_id_map(args.id_map)
    t0 = time.time()
    stats = build_refseq(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        nodes_out=args.nodes_out,
        edges_out=args.edges_out,
        id_map=id_map,
        stats_path=args.stats,
    )
    dt = time.time() - t0
    print(f"RefSeq nodes: {args.nodes_out}  edges: {args.edges_out}", file=sys.stderr)
    print(
        f"  accessions={stats.accessions:,} nodes={stats.nodes_written:,} "
        f"edges={stats.edges_written:,} untyped={stats.untyped:,}",
        file=sys.stderr,
    )
    print(f"  by_category={dict(stats.by_category)}", file=sys.stderr)
    print(f"  by_predicate={dict(stats.by_predicate)}", file=sys.stderr)
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_ontology(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    t0 = time.time()
    stats = build_ontology(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        edges_out=args.out,
        stats_path=args.stats,
    )
    dt = time.time() - t0
    print(f"ontology edges.tsv written: {args.out}", file=sys.stderr)
    print(
        f"  ontologies={stats.ontologies} subclass_of={stats.subclass_edges:,} "
        f"close_match={stats.close_match_edges:,} self_loops={stats.self_loops:,}",
        file=sys.stderr,
    )
    print(f"  by_ontology={dict(stats.by_ontology)}", file=sys.stderr)
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_attributes(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    config = load_attr_config(args.config)
    id_map = load_id_map(args.id_map)
    t0 = time.time()
    stats = build_attributes(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        config=config,
        out_path=args.out,
        id_map=id_map,
        stats_path=args.stats,
    )
    dt = time.time() - t0
    print(f"node-attributes table written: {args.out}", file=sys.stderr)
    print(
        f"  datasets={stats.datasets_processed} rows={stats.rows_written:,} "
        f"fields={stats.fields_extracted:,}",
        file=sys.stderr,
    )
    print(f"  by_dataset={dict(stats.by_dataset)}", file=sys.stderr)
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_structure(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    id_map = load_id_map(args.id_map)
    t0 = time.time()
    stats = build_structure(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        edges_out=args.edges_out,
        attrs_out=args.attrs_out,
        id_map=id_map,
        stats_path=args.stats,
    )
    dt = time.time() - t0
    print(f"structure edges: {args.edges_out}  attrs: {args.attrs_out}", file=sys.stderr)
    print(
        f"  cds_translates_to={stats.cds_translates_to:,} "
        f"feature_has_part={stats.feature_haspart:,} "
        f"(with_evidence={stats.feature_with_evidence:,}) attr_rows={stats.attr_rows:,}",
        file=sys.stderr,
    )
    print(f"  by_predicate={dict(stats.by_predicate)} elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_nodeattrs(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    config = load_nodeattr_config(args.config)
    id_map = load_id_map(args.id_map)
    t0 = time.time()
    stats = build_node_attributes(
        index_dir=args.index_dir,
        registry=registry,
        categories=categories,
        config=config,
        out_path=args.out,
        id_map=id_map,
        stats_path=args.stats,
    )
    dt = time.time() - t0
    print(f"node-attribute table written: {args.out}", file=sys.stderr)
    print(
        f"  datasets={stats.datasets_processed} rows={stats.rows_written:,} "
        f"fields={stats.fields_extracted:,}",
        file=sys.stderr,
    )
    print(f"  by_dataset={dict(stats.by_dataset)}", file=sys.stderr)
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_showcase(args: argparse.Namespace) -> int:
    registry = DatasetRegistry.load(args.conf)
    categories = CategoryMap.load(args.categories)
    config = load_showcase_config(args.config)
    id_map = load_id_map(args.id_map)
    t0 = time.time()
    stats = build_showcase(
        index_dir=args.index_dir, registry=registry, categories=categories,
        config=config, id_map=id_map, out_nodes=args.out_nodes, out_edges=args.out_edges,
        gene_filter_out=args.gene_filter_out, stats_path=args.stats,
    )
    dt = time.time() - t0
    print(f"showcase nodes: {args.out_nodes}  edges: {args.out_edges}", file=sys.stderr)
    print(f"  genes={stats.genes_resolved} proteins={stats.proteins} "
          f"compounds={stats.compounds_resolved} bioactivity_edges={stats.bioactivity_edges:,}",
          file=sys.stderr)
    print(f"  entrez gene-filter for dbSNP: {args.gene_filter_out}", file=sys.stderr)
    print(f"  by_source={dict(stats.by_source)} elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_subgraph(args: argparse.Namespace) -> int:
    config = load_subgraph_config(args.config)
    t0 = time.time()
    stats = build_subgraph(
        nodes_tsv=args.nodes, edges_tsv=args.edges, config=config,
        out_nodes=args.out_nodes, out_edges=args.out_edges, stats_path=args.stats,
        workers=args.workers,
    )
    dt = time.time() - t0
    print(f"subgraph nodes: {args.out_nodes}  edges: {args.out_edges}", file=sys.stderr)
    print(
        f"  spine={stats.spine_nodes:,} (human_anchor={stats.human_nodes:,}) "
        f"nodes_out={stats.nodes_out:,} edges {stats.edges_out:,}/{stats.edges_in:,} "
        f"(capped {stats.capped_dropped:,}, unanchored {stats.unanchored_dropped:,})",
        file=sys.stderr,
    )
    if args.full_manifest:
        mani = json.loads(Path(args.full_manifest).read_text())
        comp = check_completeness(stats, mani, config.get("expected_missing"))
        print(f"  completeness={comp}", file=sys.stderr)
    print(f"  elapsed={dt:.1f}s", file=sys.stderr)
    return 0


def _cmd_assemble(args: argparse.Namespace) -> int:
    out_dir = Path(args.out_dir)
    node_inputs = [s.strip() for s in args.nodes.split(",") if s.strip()]
    edge_inputs = [s.strip() for s in args.edges.split(",") if s.strip()]
    ext = ".gz" if args.gzip else ""
    nodes_tsv = out_dir / ("nodes.tsv" + ext)
    edges_tsv = out_dir / ("edges.tsv" + ext)

    t0 = time.time()
    n_nodes = kgx.merge_nodes(node_inputs, nodes_tsv)
    edge_dedup = kgx.merge_edges(edge_inputs, edges_tsv)
    n_edges = edge_dedup["written"]
    stub_info = None
    if args.stub_nodes:
        categories = CategoryMap.load(args.categories)
        stub_info = kgx.add_stub_nodes(nodes_tsv, edges_tsv, categories)
        n_nodes += stub_info["stubs_added"]
    # node-attribute join: concatenate the attr tables and sort by id, then let
    # nodes_to_jsonl do a memory-flat sorted merge-join. (Loading them into a dict
    # OOMs at full scale -- node_entry_attrs alone is ~138M rows.)
    attr_path = None
    if args.node_attributes:
        tbls = [s.strip() for s in args.node_attributes.split(",") if s.strip()]
        cat = out_dir / "node_attrs.concat.tmp"
        with open(cat, "wt") as o:
            for tbl in tbls:
                p = Path(tbl)
                if not p.exists():
                    continue
                with kgx.xopen(p, "rt") as fh:
                    for line in fh:
                        if "\t" in line:
                            o.write(line if line.endswith("\n") else line + "\n")
        attr_path = out_dir / "node_attrs.sorted.tmp"
        kgx._sort_file(cat, attr_path, "-k1,1", tmp_dir=out_dir)
        cat.unlink(missing_ok=True)
    kgx.nodes_to_jsonl(nodes_tsv, out_dir / ("nodes.jsonl" + ext),
                       attr_path=attr_path, merge_fn=merge_attr_dict, tmp_dir=out_dir)
    if attr_path is not None:
        Path(attr_path).unlink(missing_ok=True)
    kgx.edges_to_jsonl(edges_tsv, out_dir / ("edges.jsonl" + ext))
    if args.validate_mode == "streaming":
        # billion-scale: shape checks streamed; dangling/dup from construction stats
        report = kgx.validate_streaming(
            nodes_tsv, edges_tsv,
            removed_edges=edge_dedup["removed"],
            stub_untyped=stub_info["untyped_endpoints"] if stub_info else 0,
        )
        if stub_info is None:
            print("  WARNING: streaming validate without --stub-nodes can't confirm "
                  "dangling edges; use --stub-nodes or --validate-mode full.", file=sys.stderr)
    else:
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

    rs = sub.add_parser("refseq", help="build RefSeq nodes + edges (type-split)")
    rs.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    rs.add_argument("--conf", default="conf", help="dataset config dir")
    rs.add_argument("--categories", default="mappings/categories.yaml")
    rs.add_argument("--nodes-out", required=True, help="output RefSeq nodes.tsv")
    rs.add_argument("--edges-out", required=True, help="output RefSeq edges.tsv")
    rs.add_argument("--id-map", default=None, help="Phase 1 member->canonical map")
    rs.add_argument("--stats", default=None, help="output stats JSON path")
    rs.set_defaults(func=_cmd_refseq)

    db = sub.add_parser("dbsnp", help="build dbSNP variant nodes + edges (OPT-IN; federation dir)")
    db.add_argument("--index-dir", required=True, help="dbSNP federation index dir (out/dbsnp/index)")
    db.add_argument("--conf", default="conf", help="dataset config dir")
    db.add_argument("--categories", default="mappings/categories.yaml")
    db.add_argument("--nodes-out", required=True, help="output dbSNP nodes.tsv")
    db.add_argument("--edges-out", required=True, help="output dbSNP edges.tsv")
    db.add_argument("--id-map", default=None, help="Phase 1 member->canonical map")
    db.add_argument("--stats", default=None, help="output stats JSON path")
    db.add_argument("--max-variants", type=int, default=None, help="cap variants (debug)")
    db.set_defaults(func=_cmd_dbsnp)

    me = sub.add_parser("mesh", help="build MeSH disease-subset nodes + mondo close_match")
    me.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    me.add_argument("--conf", default="conf", help="dataset config dir")
    me.add_argument("--categories", default="mappings/categories.yaml")
    me.add_argument("--nodes-out", required=True, help="output MeSH disease nodes.tsv")
    me.add_argument("--edges-out", required=True, help="output MeSH close_match edges.tsv")
    me.add_argument("--stats", default=None, help="output stats JSON path")
    me.set_defaults(func=_cmd_mesh)

    on = sub.add_parser("ontology", help="build subclass_of + cross-ont close_match edges")
    on.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    on.add_argument("--conf", default="conf", help="dataset config dir")
    on.add_argument("--categories", default="mappings/categories.yaml")
    on.add_argument("--out", required=True, help="output ontology edges.tsv")
    on.add_argument("--stats", default=None, help="output stats JSON path")
    on.set_defaults(func=_cmd_ontology)

    st = sub.add_parser("structure", help="build sub-gene/protein structure edges + attrs (exon/cds/ufeature)")
    st.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    st.add_argument("--conf", default="conf", help="dataset config dir")
    st.add_argument("--categories", default="mappings/categories.yaml")
    st.add_argument("--edges-out", required=True, help="output structure edges.tsv")
    st.add_argument("--attrs-out", required=True, help="output node-attribute table (coords/feature type)")
    st.add_argument("--id-map", default=None, help="Phase 1 member->canonical map")
    st.add_argument("--stats", default=None, help="output stats JSON path")
    st.set_defaults(func=_cmd_structure)

    na = sub.add_parser("nodeattrs", help="build general node-attribute table (entry attrs -> node props)")
    na.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    na.add_argument("--conf", default="conf", help="dataset config dir")
    na.add_argument("--categories", default="mappings/categories.yaml")
    na.add_argument("--config", default="mappings/node_attributes.yaml", help="node-attributes config")
    na.add_argument("--out", required=True, help="output node-attribute table path")
    na.add_argument("--id-map", default=None, help="Phase 1 member->canonical map")
    na.add_argument("--stats", default=None, help="output stats JSON path")
    na.set_defaults(func=_cmd_nodeattrs)

    at = sub.add_parser("attributes", help="build numeric/value NODE attribute table")
    at.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    at.add_argument("--conf", default="conf", help="dataset config dir")
    at.add_argument("--categories", default="mappings/categories.yaml")
    at.add_argument("--config", default="mappings/attributes.yaml", help="node-attributes config")
    at.add_argument("--out", required=True, help="output node-attribute table path")
    at.add_argument("--id-map", default=None, help="Phase 1 member->canonical map")
    at.add_argument("--stats", default=None, help="output stats JSON path")
    at.set_defaults(func=_cmd_attributes)

    sh = sub.add_parser("showcase", help="dbSNP+bioactivity showcase for the curated famous genes/compounds")
    sh.add_argument("--index-dir", required=True, help="dir with *_sorted.*.index.gz")
    sh.add_argument("--conf", default="conf", help="dataset config dir")
    sh.add_argument("--categories", default="mappings/categories.yaml")
    sh.add_argument("--config", default="mappings/showcase.yaml", help="showcase gene/compound lists")
    sh.add_argument("--id-map", default=None, help="Phase 1 member->canonical map")
    sh.add_argument("--out-nodes", required=True, help="output showcase nodes.tsv[.gz]")
    sh.add_argument("--out-edges", required=True, help="output showcase edges.tsv[.gz] (bioactivity)")
    sh.add_argument("--gene-filter-out", required=True,
                    help="output entrez gene-id list -> feed to dbsnp extract.py --genes")
    sh.add_argument("--stats", default=None, help="output stats JSON path")
    sh.set_defaults(func=_cmd_showcase)

    sg = sub.add_parser("subgraph", help="induced human-scoped + capped projection of the full dump")
    sg.add_argument("--nodes", required=True, help="full dump nodes.tsv[.gz]")
    sg.add_argument("--edges", required=True, help="full dump edges.tsv[.gz]")
    sg.add_argument("--config", default="mappings/subgraph.yaml")
    sg.add_argument("--out-nodes", required=True, help="output subgraph nodes.tsv[.gz]")
    sg.add_argument("--out-edges", required=True, help="output subgraph edges.tsv[.gz]")
    sg.add_argument("--full-manifest", default=None, help="full dump manifest.json for completeness check")
    sg.add_argument("--stats", default=None, help="output stats JSON path")
    sg.add_argument("--workers", type=int, default=8, help="parallel workers (zcat | N); 1 = serial")
    sg.set_defaults(func=_cmd_subgraph)

    a = sub.add_parser("assemble", help="merge -> JSONL -> validate -> manifest")
    a.add_argument("--nodes", required=True, help="comma list of node TSVs")
    a.add_argument("--edges", required=True, help="comma list of edge TSVs")
    a.add_argument("--out-dir", required=True, help="output dir for the KGX dump")
    a.add_argument("--data-version", default=None, help="biobtree data release tag")
    a.add_argument("--categories", default="mappings/categories.yaml")
    a.add_argument("--stub-nodes", action="store_true",
                   help="emit minimal nodes for edge endpoints lacking one")
    a.add_argument("--gzip", action="store_true", help="gzip the final TSV/JSONL")
    a.add_argument("--validate-mode", choices=("full", "streaming"), default="full",
                   help="full: exact in-memory validate (subgraph/small). streaming: "
                        "billion-scale; shape checks + dangling/dup from construction")
    a.add_argument("--node-attributes", default=None,
                   help="node-attribute table(s) (comma list; from `attributes`/`structure`) "
                        "to merge into nodes.jsonl")
    a.set_defaults(func=_cmd_assemble)

    args = parser.parse_args(argv)
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
