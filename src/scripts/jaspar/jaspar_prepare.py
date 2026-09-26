#!/usr/bin/env python3
"""Rebuild JASPAR metadata TSVs from the JASPAR REST API.

The legacy metadata tables (mencius.uio.no/.../ultimate_metadata_table_*.tsv)
went offline when JASPAR migrated to jaspar.elixir.no, which serves PFM matrix
files but no aggregated metadata table. This script reconstructs the 15-column
TSV that src/update/jaspar.go expects, from the public API, for the CORE and
UNVALIDATED collections (all versions, so version-pinned ids like MA0004.1 stay
resolvable).

Output: raw_data/jaspar/jaspar_metadata_{CORE,UNVALIDATED}.tsv
Columns (tab-sep, one header line then data):
  collection tax_group matrix_id base_id version name class family
  uniprot_ids validation comment source type tax_id species
"""
import json, os, sys, time, urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed

BASE = "https://jaspar.elixir.no/api/v1/matrix/"
OUTDIR = os.path.join(os.path.dirname(__file__), "..", "..", "..", "raw_data", "jaspar")
HEADER = "\t".join(["collection", "tax_group", "matrix_id", "base_id", "version",
                    "name", "class", "family", "uniprot_ids", "validation",
                    "comment", "source", "type", "tax_id", "species"])


def get(url, tries=4):
    for i in range(tries):
        try:
            req = urllib.request.Request(url, headers={"Accept": "application/json"})
            with urllib.request.urlopen(req, timeout=30) as r:
                return json.load(r)
        except Exception as e:
            if i == tries - 1:
                sys.stderr.write(f"GET failed {url}: {e}\n")
            time.sleep(1.5 * (i + 1))
    return None


def list_ids(collection):
    ids, url = [], f"{BASE}?collection={collection}&page_size=1000"
    while url:
        d = get(url)
        if not d:
            break
        ids += [r["matrix_id"] for r in d.get("results", [])]
        url = d.get("next")
    return ids


def clean(x):
    return str(x).replace("\t", " ").replace("\n", " ").replace("\r", " ")


def detail_row(mid, collection):
    d = get(f"{BASE}{mid}/")
    if not d:
        return None
    sp = d.get("species") or []
    tax_id = str(sp[0]["tax_id"]) if sp and sp[0].get("tax_id") is not None else ""
    species = "::".join(clean(s.get("name", "")) for s in sp)
    pubmed = (d.get("pubmed_ids") or d.get("medline") or [""])
    pubmed = pubmed[0] if pubmed else ""
    row = [d.get("collection", collection) or collection,
           d.get("tax_group", "") or "",
           d.get("matrix_id", "") or "",
           d.get("base_id", "") or "",
           str(d.get("version", "")),
           d.get("name", "") or "",
           ",".join(d.get("class") or []),
           ",".join(d.get("family") or []),
           "::".join(d.get("uniprot_ids") or []),
           str(pubmed or ""),
           d.get("description", "") or "",
           "",  # source (not in API; legacy column kept blank)
           d.get("type", "") or "",
           tax_id,
           species]
    return "\t".join(clean(c) for c in row)


def main():
    os.makedirs(OUTDIR, exist_ok=True)
    total = 0
    for coll, fn in [("CORE", "jaspar_metadata_CORE.tsv"),
                     ("UNVALIDATED", "jaspar_metadata_UNVALIDATED.tsv")]:
        ids = list_ids(coll)
        print(f"{coll}: {len(ids)} matrices listed", flush=True)
        rows = []
        with ThreadPoolExecutor(max_workers=8) as ex:
            futs = {ex.submit(detail_row, m, coll): m for m in ids}
            done = 0
            for f in as_completed(futs):
                r = f.result()
                if r:
                    rows.append(r)
                done += 1
                if done % 500 == 0:
                    print(f"  {coll}: {done}/{len(ids)} fetched", flush=True)
        path = os.path.join(OUTDIR, fn)
        with open(path, "w") as out:
            out.write(HEADER + "\n")
            out.write("\n".join(rows) + "\n")
        print(f"{coll}: wrote {len(rows)} rows -> {path}", flush=True)
        total += len(rows)
    print(f"DONE: {total} total rows", flush=True)
    if total < 3000:
        sys.stderr.write(f"WARNING: only {total} rows (expected ~5900)\n")
        sys.exit(1)


if __name__ == "__main__":
    main()
