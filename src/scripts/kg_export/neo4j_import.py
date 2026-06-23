"""KGX dump -> Neo4j bulk-import CSVs (for `neo4j-admin database import`, Neo4j 5).

Bulk import is the right tool at subgraph scale (~10M nodes / ~50M edges) -- LOAD CSV
would crawl. Nodes come from nodes.jsonl (it carries the merged attributes + synonyms);
edges from edges.tsv (raw columns incl. has_evidence / qualifiers).

Conventions:
  - node label  = biolink category minus `biolink:` (multi-label via ';').
  - rel :TYPE   = predicate minus `biolink:`.
  - array props (equivalent_identifiers, synonym, has_evidence, list attributes) use
    ';' as the array delimiter (pass --array-delimiter ';' to neo4j-admin).
  - attribute columns are typed by inspecting values: list -> :string[], all-numeric
    -> :double, else string. So `WHERE n.gnomad_pli < 0.1` and `n.entrez_type='...'`
    both work in Cypher.
"""

from __future__ import annotations

import argparse
import csv
import gzip
import json
import sys
from pathlib import Path

CORE = {"id", "category", "name", "equivalent_identifiers", "provided_by"}


def _open(p):
    p = str(p)
    return gzip.open(p, "rt") if p.endswith(".gz") else open(p, encoding="utf-8")


def _short(s: str) -> str:
    return s.split(":", 1)[1] if s.startswith("biolink:") else s


def _clean(x) -> str:
    return str(x).replace(";", " ").replace("\n", " ").replace("\r", " ")


def _is_num(x) -> bool:
    if isinstance(x, bool):
        return False
    if isinstance(x, (int, float)):
        return True
    try:
        float(x)
        return True
    except (ValueError, TypeError):
        return False


def convert_nodes(jsonl, out_csv) -> int:
    # pass 1: collect attribute keys + infer type (array / numeric / string)
    is_arr: dict[str, bool] = {}
    is_num: dict[str, bool] = {}
    with _open(jsonl) as fh:
        for line in fh:
            if not line.strip():
                continue
            d = json.loads(line)
            for k, v in d.items():
                if k in CORE:
                    continue
                if isinstance(v, list):
                    is_arr[k] = True
                    is_num.setdefault(k, False)
                else:
                    is_arr.setdefault(k, False)
                    is_num[k] = is_num.get(k, True) and _is_num(v)
    keys = sorted(is_arr)

    def header_for(k):
        if is_arr[k]:
            return f"{k}:string[]"
        if is_num.get(k):
            return f"{k}:double"
        return k

    n = 0
    with _open(jsonl) as fh, open(out_csv, "w", newline="", encoding="utf-8") as out:
        w = csv.writer(out)
        w.writerow(["id:ID", "name", "equivalent_identifiers:string[]"]
                   + [header_for(k) for k in keys] + [":LABEL"])
        for line in fh:
            if not line.strip():
                continue
            d = json.loads(line)
            row = [d.get("id", ""), _clean(d.get("name", "")),
                   ";".join(_clean(x) for x in (d.get("equivalent_identifiers") or []))]
            for k in keys:
                v = d.get(k)
                if v is None:
                    row.append("")
                elif is_arr[k]:
                    row.append(";".join(_clean(x) for x in v))
                else:
                    row.append(_clean(v))
            cats = d.get("category") or []
            if isinstance(cats, str):
                cats = [cats]
            row.append(";".join(_short(c) for c in cats) or "NamedThing")
            w.writerow(row)
            n += 1
    return n


def convert_edges(edges_tsv, out_csv) -> int:
    # edges.tsv columns: id, subject, predicate, object, primary, agg, kl, at, has_evidence, qualifiers
    n = 0
    with _open(edges_tsv) as fh, open(out_csv, "w", newline="", encoding="utf-8") as out:
        w = csv.writer(out)
        w.writerow([":START_ID", ":END_ID", ":TYPE", "primary_knowledge_source",
                    "knowledge_level", "agent_type", "has_evidence:string[]", "qualifiers"])
        next(fh, "")  # header
        for line in fh:
            p = line.rstrip("\n").split("\t")
            if len(p) < 4:
                continue
            ev = ";".join(x for x in (p[8].split("|") if len(p) > 8 and p[8] else []))
            w.writerow([p[1], p[3], _short(p[2]), p[4] if len(p) > 4 else "",
                        p[6] if len(p) > 6 else "", p[7] if len(p) > 7 else "",
                        ev, p[9] if len(p) > 9 else ""])
            n += 1
    return n


def main(argv=None) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--nodes", required=True, help="nodes.jsonl[.gz]")
    ap.add_argument("--edges", required=True, help="edges.tsv[.gz]")
    ap.add_argument("--out-dir", required=True, help="dir for neo4j CSVs (the /import mount)")
    a = ap.parse_args(argv)
    out = Path(a.out_dir)
    out.mkdir(parents=True, exist_ok=True)
    nn = convert_nodes(a.nodes, out / "neo4j_nodes.csv")
    ne = convert_edges(a.edges, out / "neo4j_edges.csv")
    print(f"nodes={nn:,} -> {out/'neo4j_nodes.csv'}", file=sys.stderr)
    print(f"edges={ne:,} -> {out/'neo4j_edges.csv'}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
