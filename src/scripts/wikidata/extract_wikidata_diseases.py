#!/usr/bin/env python3
"""
Extract the FULL disease -> Wikidata-item (QID) mapping: every Wikidata item
that carries a Mondo ID (P5270) or Disease Ontology ID (P699), independent of
whether it has symptom (P780) statements.

Why this exists
---------------
The disease->QID links used to be a byproduct of the `wikidata_symptom` extract
(P780-gated), so only diseases with curated symptoms got a QID — ~15% coverage
(Atlas feedback: Bardet-Biedl DOID:1935 resolved to nothing). This extract is
symptom-independent: it pulls every P5270/P699 item, giving near-complete
Mondo/DOID -> QID coverage for the `wikidata` dataset.

Output (TSV, tab-separated, one row per (item, xref) pair)
----------------------------------------------------------
    qid   xref

`qid` is the Wikidata item (e.g. Q11081). `xref` is a biobtree disease id:
either `MONDO:xxxxxxx` (normalised from Wikidata's `MONDO_xxxxxxx`) or
`DOID:xxxxx`. An item with both a Mondo and a DOID emits two rows. Rows are
de-duplicated.
"""
import argparse
import json
import os
import sys
import time
import urllib.parse
import urllib.request

WDQS = "https://query.wikidata.org/sparql"
PAGE = 10000
UA = "biobtree-wikidata-diseases/1.0 (https://biobtree.org; mailto:biobtree@biobtree.org)"

def _mondo(v):
    # Wikidata stores "MONDO_0007739" (or rarely "MONDO:0007739"/"0007739").
    return "MONDO:" + v.replace("MONDO_", "").replace("MONDO:", "")


def _doid(v):
    return v if v.upper().startswith("DOID:") else "DOID:" + v


# (property, how to normalise its value into a biobtree disease id)
PROPS = [("P5270", _mondo), ("P699", _doid)]


def _query(prop, offset):
    q = f"""SELECT ?item ?v WHERE {{
      ?item wdt:{prop} ?v .
    }} LIMIT {PAGE} OFFSET {offset}"""
    url = WDQS + "?" + urllib.parse.urlencode({"query": q, "format": "json"})
    req = urllib.request.Request(url, headers={"User-Agent": UA, "Accept": "application/sparql-results+json"})
    for attempt in range(5):
        try:
            with urllib.request.urlopen(req, timeout=180) as r:
                return json.load(r)["results"]["bindings"]
        except Exception as e:
            if attempt == 4:
                raise
            print(f"  retry {prop}@{offset} ({e})", file=sys.stderr, flush=True)
            time.sleep(5 * (attempt + 1))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--output", default="raw_data/wikidata/wikidata_diseases.tsv")
    args = ap.parse_args()
    os.makedirs(os.path.dirname(args.output) or ".", exist_ok=True)

    seen = set()
    n = 0
    with open(args.output + ".part", "w") as out:
        for prop, norm in PROPS:
            offset = 0
            while True:
                rows = _query(prop, offset)
                if not rows:
                    break
                for b in rows:
                    qid = b["item"]["value"].rsplit("/", 1)[-1]  # .../Q11081 -> Q11081
                    if not qid.startswith("Q"):
                        continue
                    xref = norm(b["v"]["value"])
                    key = (qid, xref)
                    if key in seen:
                        continue
                    seen.add(key)
                    out.write(f"{qid}\t{xref}\n")
                    n += 1
                print(f"  {prop} offset={offset}: {len(rows)} rows (total {n})", flush=True)
                offset += PAGE
    os.replace(args.output + ".part", args.output)
    print(f"wrote {n} disease->QID rows to {args.output}", flush=True)


if __name__ == "__main__":
    sys.exit(main())
