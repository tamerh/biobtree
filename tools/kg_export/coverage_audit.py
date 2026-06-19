"""Coverage drift detector: which BioBTree datasets the KG covers, and which new
ones need a decision.

Classifies every source1/source2 dataset as COVERED (a node/edge rule or a
runtime builder), SKIP (listed in mappings/coverage_skip.yaml with a reason), or
UNEXPLAINED (neither — a NEW gap that needs either coverage or a skip entry).

Exits non-zero if any UNEXPLAINED datasets exist, so it doubles as a CI / post-
build drift check: run it after BioBTree changes; new datasets surface here.

    python -m tools.kg_export.coverage_audit [--conf <dir>] [--quiet]

Other conf files (xref1/xref2) are identifier/derived and out of scope.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import yaml

from .attributes import load_config as load_attr_config
from .categories import CategoryMap
from .predicates import PredicateMap

# datasets typed at runtime by a dedicated builder (no categories.yaml entry)
RUNTIME_BUILDERS = {"go", "refseq", "dbsnp", "mesh"}


def _core_datasets(conf_dir: Path) -> set[str]:
    out: set[str] = set()
    for fn in ("source1.dataset.json", "source2.dataset.json"):
        p = conf_dir / fn
        if p.exists():
            out |= set(json.loads(p.read_text()))
    return out


def covered_datasets(cats: CategoryMap, preds: PredicateMap,
                     attr_datasets: set[str] | None = None) -> set[str]:
    """Every dataset the exporter emits as a node, edge endpoint, or node attribute."""
    cov: set[str] = set(cats.datasets()) | set(RUNTIME_BUILDERS) | set(attr_datasets or ())
    for key in preds.pairs():
        s, o = key.split(">")
        cov |= {s, o}
    for ds in preds.reified_datasets():
        r = preds.reified_rule(ds)
        cov.add(ds)
        for v in (r.subject, r.object, r.partner, r.via):
            if v:
                cov.add(v)
        cov |= set(r.extra_objects or [])
        cov |= set(r.extra_subjects or [])
        if r.qualifiers:
            cov |= set(r.qualifiers.values())
    return cov


def classify(conf_dir: Path, cats: CategoryMap, preds: PredicateMap,
             skip: dict[str, str], attr_datasets: set[str] | None = None) -> dict:
    core = _core_datasets(conf_dir)
    cov = covered_datasets(cats, preds, attr_datasets)

    def is_covered(ds: str) -> bool:
        if ds in cov:
            return True
        if ds.endswith("parent") or ds.endswith("child"):
            base = ds[:-6] if ds.endswith("parent") else ds[:-5]
            # taxonomy uses tax{parent,child}; otherwise <name>{parent,child}
            return base in cats.datasets() or base in RUNTIME_BUILDERS or base == "tax"
        return False

    covered, skipped, unexplained = [], [], []
    for ds in sorted(core):
        if is_covered(ds):
            covered.append(ds)
        elif ds in skip:
            skipped.append(ds)
        else:
            unexplained.append(ds)
    return {"core": core, "covered": covered, "skipped": skipped,
            "unexplained": unexplained}


def main(argv=None) -> int:
    repo = Path(__file__).resolve().parents[2]
    ap = argparse.ArgumentParser()
    ap.add_argument("--conf", default=str(repo / "conf"),
                    help="conf dir to audit (point at the live BioBTree conf to detect drift)")
    ap.add_argument("--categories", default=str(repo / "mappings" / "categories.yaml"))
    ap.add_argument("--predicates", default=str(repo / "mappings" / "predicates.yaml"))
    ap.add_argument("--skip", default=str(repo / "mappings" / "coverage_skip.yaml"))
    ap.add_argument("--attributes", default=str(repo / "mappings" / "attributes.yaml"))
    ap.add_argument("--quiet", action="store_true", help="only print the summary + gaps")
    a = ap.parse_args(argv)

    cats = CategoryMap.load(a.categories)
    preds = PredicateMap.load(a.predicates)
    skip = (yaml.safe_load(Path(a.skip).read_text()) or {}).get("skip", {})
    attr_datasets = set(load_attr_config(a.attributes))
    r = classify(Path(a.conf), cats, preds, skip, attr_datasets)

    n = len(r["core"])
    print(f"BioBTree source1+source2 datasets: {n}")
    print(f"  covered:     {len(r['covered'])}")
    print(f"  skipped:     {len(r['skipped'])} (intentional, see coverage_skip.yaml)")
    print(f"  UNEXPLAINED: {len(r['unexplained'])}")
    if not a.quiet and r["unexplained"]:
        print("\nUNEXPLAINED (new gaps -> add coverage or a coverage_skip.yaml entry):")
        for ds in r["unexplained"]:
            print(f"  - {ds}")
    if r["unexplained"]:
        print("\nDRIFT: unexplained datasets present. Decide coverage or add to "
              "coverage_skip.yaml with a reason.", file=sys.stderr)
        return 1
    print("\nOK: every source1/source2 dataset is covered or explicitly skipped.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
