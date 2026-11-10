#!/usr/bin/env python3
"""Upload the published subgraph KGX export to Zenodo as a DRAFT deposition.

Creates a draft, uploads the dump files via the bucket API (handles multi-GB files),
sets the metadata + CC BY-NC-SA 4.0 license, and STOPS -- it never publishes. You
review the draft on zenodo.org and click Publish yourself (publishing is permanent).

    ZENODO_TOKEN=xxxx python -m kg_export.zenodo_upload [--sandbox]
        [--dump out/kg/full/sub/dump] [--creator "Surname, Given"]

Token: zenodo.org -> Applications -> Personal access tokens (scopes deposit:write,
deposit:actions). Use --sandbox (sandbox.zenodo.org) for a throwaway dry run first.
Requires: requests.
"""
import argparse
import json
import os
import sys
from pathlib import Path

import requests

# Files that make up the published bundle (in upload order). A generated README is
# added too. JSONL is included so the bundle is Neo4j-ready out of the box.
DUMP_FILES = ["manifest.json", "nodes.tsv.gz", "edges.tsv.gz",
              "nodes.jsonl.gz", "edges.jsonl.gz"]

DESCRIPTION = """\
<p>A biolink-typed <a href="https://github.com/biolink/kgx">KGX</a> knowledge graph
exported from <a href="https://github.com/tamerh/biobtree">BioBTree</a>, which
integrates 80+ primary biomedical databases. This is the <b>human-scoped published
subgraph</b>: a practical, Neo4j-ready projection of the full graph
(~{nodes} nodes / ~{edges} edges).</p>
<p>Nodes are biolink categories (genes, transcripts, exons, proteins, variants,
diseases, phenotypes, pathways, GO functions, anatomy, drugs/compounds, ontologies);
edges are typed biolink predicates carrying evidence and provenance. Standard KGX
files: <code>nodes.tsv.gz</code> / <code>edges.tsv.gz</code> (+ JSON Lines) and a
<code>manifest.json</code>.</p>
<p><b>License &amp; exclusions.</b> Released under CC BY-NC-SA 4.0. To keep the bundle
redistributable, sources without compatible redistribution terms are excluded:
OMIM full data (only bare identifiers appear as cross-references), CTD, PanelApp,
miRDB, and the SpliceAI prediction layer (CC BY-NC). AlphaMissense is CC BY 4.0
and is excluded only because the variant-prediction layer is disabled
(<code>WITH_PREDICTIONS=0</code>). DrugBank and HMDB (CC BY-NC), ChEMBL/PharmGKB/
DrugCentral (CC BY-SA) and WikiPathways (CC0), ChIP-Atlas and GOA (CC BY 4.0),
gnomAD-SV (CC0) and dbVar (public domain) are included, which is why the bundle is
CC BY-NC-SA. MSigDB: gene-set membership only.</p>
"""


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--sandbox", action="store_true", help="use sandbox.zenodo.org")
    ap.add_argument("--dump", default="out/kg/full/sub/dump", help="subgraph dump dir")
    ap.add_argument("--new-version-of", default="", metavar="RECORD_ID",
                    help="create a NEW VERSION of an existing published record "
                         "(concept DOI stays the same) instead of a fresh deposition")
    ap.add_argument("--creator", default=os.environ.get("ZENODO_CREATOR", "Gür, Tamer"))
    ap.add_argument("--affiliation", default=os.environ.get("ZENODO_AFFILIATION", ""))
    ap.add_argument("--version", default=os.environ.get("ZENODO_VERSION", "v1"))
    ap.add_argument("--preprint-doi", default=os.environ.get("ZENODO_PREPRINT_DOI", "10.5281/zenodo.18962899"),
                    help="BioBTree v2 preprint DOI to link as isSupplementTo (empty to skip)")
    args = ap.parse_args()

    token = os.environ.get("ZENODO_TOKEN")
    if not token:
        print("ERROR: set ZENODO_TOKEN (zenodo.org -> Personal access tokens).", file=sys.stderr)
        return 2
    base = "https://sandbox.zenodo.org/api" if args.sandbox else "https://zenodo.org/api"
    site = "https://sandbox.zenodo.org" if args.sandbox else "https://zenodo.org"
    # Bearer header, NOT ?access_token=: requests puts the full URL in HTTPError
    # messages, so a query-param token ends up printed in logs on any 4xx/5xx.
    s = requests.Session()
    s.headers.update({"Authorization": f"Bearer {token}"})
    dump = Path(args.dump)

    manifest = json.loads((dump / "manifest.json").read_text())
    nodes_h = f"{manifest.get('node_count', 0)/1e6:.0f}M"
    edges_h = f"{manifest.get('edge_count', 0)/1e6:.0f}M"

    # 1. create the draft: either a fresh deposition, or a new version of an
    #    existing published record (same concept DOI, version bumped).
    if args.new_version_of:
        print(f"Creating new version of record {args.new_version_of} on {site} ...")
        r = s.post(f"{base}/deposit/depositions/{args.new_version_of}/actions/newversion")
        if r.status_code >= 400:
            print(f"ERROR: new-version request failed: HTTP {r.status_code} {r.reason}", file=sys.stderr)
            print(f"  response: {r.text[:400]}", file=sys.stderr)
            return 2
        latest = (r.json().get("links") or {}).get("latest_draft")
        if not latest:
            print("ERROR: no latest_draft link in the new-version response", file=sys.stderr)
            return 2
        dep = s.get(latest).json()
        dep_id, bucket = dep["id"], dep["links"]["bucket"]
        print(f"  version draft id={dep_id}")
        # The new-version draft inherits the previous files; remove them so the
        # bundle is exactly the files below (Zenodo rejects duplicate names).
        existing = s.get(f"{base}/deposit/depositions/{dep_id}/files").json()
        if isinstance(existing, list):
            for f in existing:
                d = s.delete(f"{base}/deposit/depositions/{dep_id}/files/{f['id']}")
                if d.status_code >= 400:
                    print(f"  warning: could not remove {f.get('filename')}: HTTP {d.status_code}", file=sys.stderr)
            print(f"  removed {len(existing)} inherited file(s)")
    else:
        print(f"Creating draft on {site} ...")
        r = s.post(f"{base}/deposit/depositions", json={})
        if r.status_code >= 400:
            print(f"ERROR: draft creation failed: HTTP {r.status_code} {r.reason}", file=sys.stderr)
            print(f"  response: {r.text[:400]}", file=sys.stderr)
            print("  check the token scopes (deposit:write, deposit:actions) and that it "
                  "matches the endpoint (--sandbox token for sandbox.zenodo.org).", file=sys.stderr)
            return 2
        dep = r.json()
        dep_id, bucket = dep["id"], dep["links"]["bucket"]
        print(f"  draft id={dep_id}")

    # generated README accompanying the bundle
    readme = (dump / "README.md")
    readme.write_text(
        f"# BioBTree Knowledge Graph — Human Subgraph (KGX)\n\n"
        f"{manifest.get('node_count', 0):,} nodes / {manifest.get('edge_count', 0):,} edges. "
        f"License: CC BY-NC-SA 4.0.\n\nGenerated by BioBTree's KG export "
        f"(https://github.com/tamerh/biobtree). Excluded for redistributability: "
        f"OMIM full data, CTD, PanelApp, miRDB, SpliceAI. AlphaMissense is CC BY 4.0 "
        f"and is omitted only because the prediction layer is off. See manifest.json.\n")

    # 2. upload files via the bucket (streamed PUT; handles multi-GB)
    for name in ["README.md"] + DUMP_FILES:
        p = dump / name
        if not p.exists():
            print(f"  skip missing {name}")
            continue
        size_mb = p.stat().st_size / 1e6
        print(f"  uploading {name} ({size_mb:,.0f} MB) ...", flush=True)
        with p.open("rb") as fh:
            up = s.put(f"{bucket}/{name}", data=fh)
        up.raise_for_status()

    # 3. metadata (draft only -- not published)
    meta = {"metadata": {
        "title": "BioBTree Knowledge Graph — Human Subgraph (KGX)",
        "upload_type": "dataset",
        "description": DESCRIPTION.format(nodes=nodes_h, edges=edges_h),
        "creators": [{"name": args.creator, **({"affiliation": args.affiliation} if args.affiliation else {})}],
        "license": "cc-by-nc-sa-4.0",
        "version": args.version,
        "keywords": ["knowledge graph", "biolink model", "KGX", "biomedical",
                     "Neo4j", "BioBTree", "cross-references", "data integration"],
        "related_identifiers": [
            {"relation": "isCompiledBy", "identifier": "https://github.com/tamerh/biobtree",
             "scheme": "url"},
        ] + ([{"relation": "isSupplementTo", "identifier": args.preprint_doi, "scheme": "doi"}]
             if args.preprint_doi else []),
    }}
    r = s.put(f"{base}/deposit/depositions/{dep_id}", json=meta)
    if r.status_code >= 400:
        print(f"ERROR: metadata update failed: HTTP {r.status_code} {r.reason}", file=sys.stderr)
        print(f"  response: {r.text[:400]}", file=sys.stderr)
        return 2

    print("\nDRAFT READY (not published). Review + publish here:")
    print(f"  {site}/uploads/{dep_id}   (or {site}/deposit/{dep_id})")
    print("Publishing is permanent — do it from the web UI after you've checked it.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
