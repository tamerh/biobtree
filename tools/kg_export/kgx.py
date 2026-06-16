"""Phase 3: assemble KGX outputs — merge, serialize (JSONL), validate, manifest.

The node/edge builders (nodes, edges, reified, go) each write a partial KGX TSV.
This module merges them into a single nodes.tsv + edges.tsv, emits KGX JSON-Lines,
runs a lightweight structural validation (dangling-edge check), and writes a
manifest with counts.
"""

from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path
from typing import Iterable

NODE_HEADER = "id\tcategory\tname\tequivalent_identifiers\tprovided_by"
EDGE_HEADER = (
    "subject\tpredicate\tobject\tprimary_knowledge_source\t"
    "aggregator_knowledge_source"
)
BIOLINK_VERSION = "4.2.1"  # target Monarch release line; pin as needed


def _read_rows(path: Path):
    with path.open(encoding="utf-8") as fh:
        header = next(fh, "").rstrip("\n")
        for line in fh:
            yield header, line.rstrip("\n")


def merge_nodes(inputs: Iterable[str | Path], out_path: str | Path) -> int:
    """Concatenate node TSVs, de-duplicating by node id (first wins)."""
    seen: set[str] = set()
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    n = 0
    with out_path.open("w", encoding="utf-8") as out:
        out.write(NODE_HEADER + "\n")
        for inp in inputs:
            p = Path(inp)
            if not p.exists():
                continue
            for _, row in _read_rows(p):
                if not row:
                    continue
                node_id = row.split("\t", 1)[0]
                if node_id in seen:
                    continue
                seen.add(node_id)
                out.write(row + "\n")
                n += 1
    return n


def merge_edges(inputs: Iterable[str | Path], out_path: str | Path) -> int:
    """Concatenate edge TSVs (no dedup; builders dedup by construction)."""
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    n = 0
    with out_path.open("w", encoding="utf-8") as out:
        out.write(EDGE_HEADER + "\n")
        for inp in inputs:
            p = Path(inp)
            if not p.exists():
                continue
            for _, row in _read_rows(p):
                if row:
                    out.write(row + "\n")
                    n += 1
    return n


def nodes_to_jsonl(nodes_tsv: str | Path, out_path: str | Path) -> int:
    cols = NODE_HEADER.split("\t")
    n = 0
    with Path(out_path).open("w", encoding="utf-8") as out:
        for _, row in _read_rows(Path(nodes_tsv)):
            if not row:
                continue
            vals = row.split("\t")
            d = dict(zip(cols, vals))
            d["category"] = [d["category"]] if d.get("category") else []
            d["equivalent_identifiers"] = (
                d["equivalent_identifiers"].split("|")
                if d.get("equivalent_identifiers")
                else []
            )
            out.write(json.dumps(d) + "\n")
            n += 1
    return n


def edges_to_jsonl(edges_tsv: str | Path, out_path: str | Path) -> int:
    cols = EDGE_HEADER.split("\t")
    n = 0
    with Path(out_path).open("w", encoding="utf-8") as out:
        for _, row in _read_rows(Path(edges_tsv)):
            if not row:
                continue
            out.write(json.dumps(dict(zip(cols, row.split("\t")))) + "\n")
            n += 1
    return n


def validate(nodes_tsv: str | Path, edges_tsv: str | Path) -> dict:
    """Lightweight structural validation: dangling edges + basic shape checks."""
    node_ids: set[str] = set()
    bad_node_curie = 0
    for _, row in _read_rows(Path(nodes_tsv)):
        if not row:
            continue
        nid = row.split("\t", 1)[0]
        node_ids.add(nid)
        if ":" not in nid:
            bad_node_curie += 1

    edges = 0
    dangling_subject = 0
    dangling_object = 0
    bad_predicate = 0
    for _, row in _read_rows(Path(edges_tsv)):
        if not row:
            continue
        parts = row.split("\t")
        if len(parts) < 3:
            continue
        edges += 1
        subj, pred, obj = parts[0], parts[1], parts[2]
        if subj not in node_ids:
            dangling_subject += 1
        if obj not in node_ids:
            dangling_object += 1
        if not pred.startswith("biolink:"):
            bad_predicate += 1

    return {
        "nodes": len(node_ids),
        "edges": edges,
        "dangling_subject_edges": dangling_subject,
        "dangling_object_edges": dangling_object,
        "bad_node_curie": bad_node_curie,
        "bad_predicate": bad_predicate,
        "ok": dangling_subject == 0
        and dangling_object == 0
        and bad_node_curie == 0
        and bad_predicate == 0,
    }


def manifest(
    nodes_tsv: str | Path,
    edges_tsv: str | Path,
    data_version: str | None = None,
    validation: dict | None = None,
) -> dict:
    by_category: dict[str, int] = defaultdict(int)
    node_count = 0
    for _, row in _read_rows(Path(nodes_tsv)):
        if not row:
            continue
        node_count += 1
        cat = row.split("\t")[1] if "\t" in row else ""
        by_category[cat] += 1

    by_predicate: dict[str, int] = defaultdict(int)
    by_source: dict[str, int] = defaultdict(int)
    edge_count = 0
    for _, row in _read_rows(Path(edges_tsv)):
        if not row:
            continue
        parts = row.split("\t")
        if len(parts) < 4:
            continue
        edge_count += 1
        by_predicate[parts[1]] += 1
        by_source[parts[3]] += 1

    return {
        "name": "biobtree-kg",
        "data_version": data_version,
        "biolink_model_version": BIOLINK_VERSION,
        "generated_by": "tools.kg_export",
        "knowledge_source": "infores:biobtree",
        "node_count": node_count,
        "edge_count": edge_count,
        "node_categories": dict(sorted(by_category.items(), key=lambda kv: -kv[1])),
        "edge_predicates": dict(sorted(by_predicate.items(), key=lambda kv: -kv[1])),
        "edge_sources": dict(sorted(by_source.items(), key=lambda kv: -kv[1])),
        "validation": validation,
    }
