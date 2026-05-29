#!/usr/bin/env python3
"""
Extract intOGen reference data for tests.

Samples driver genes from the intOGen Compendium of Cancer Genes (CC0) so the
test suite has known-good symbols/roles to validate against a running biobtree
server. No registration required.

Role is the majority vote of the per-cohort ROLE calls, mirroring the parser's
consensusRole() so the reference matches what biobtree stores.
"""

import csv
import io
import json
import urllib.request
import zipfile
from collections import Counter, defaultdict
from pathlib import Path

DRIVERS_ZIP = "https://www.intogen.org/download?file=IntOGen-Drivers-20240920.zip"
COMPENDIUM = "Compendium_Cancer_Genes.tsv"
N_GENES = 40


def fetch_zip_member(url, member_suffix):
    with urllib.request.urlopen(url, timeout=180) as resp:
        raw = resp.read()
    zf = zipfile.ZipFile(io.BytesIO(raw))
    name = next(n for n in zf.namelist() if n.endswith(member_suffix))
    return zf.read(name).decode("utf-8", errors="replace")


def main():
    out = Path(__file__).parent / "reference_data.json"
    rows = list(csv.DictReader(io.StringIO(fetch_zip_member(DRIVERS_ZIP, COMPENDIUM)), delimiter="\t"))

    role_counts = defaultdict(Counter)
    cancers = defaultdict(set)
    order = []
    seen = set()
    for r in rows:
        sym = r.get("SYMBOL", "").strip()
        if not sym:
            continue
        if sym not in seen:
            seen.add(sym)
            order.append(sym)
        if r.get("ROLE"):
            role_counts[sym][r["ROLE"]] += 1
        if r.get("CANCER_TYPE"):
            cancers[sym].add(r["CANCER_TYPE"])

    genes = []
    for sym in order[:N_GENES]:
        role = role_counts[sym].most_common(1)[0][0] if role_counts[sym] else "ambiguous"
        genes.append({
            "symbol": sym,
            "role": role,
            "cancer_types": sorted(cancers[sym]),
        })

    with open(out, "w") as f:
        json.dump({"genes": genes}, f, indent=2)
    print(f"Wrote {len(genes)} driver genes to {out}")


if __name__ == "__main__":
    main()
