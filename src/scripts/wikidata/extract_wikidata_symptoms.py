#!/usr/bin/env python3
"""
Extract disease -> symptom/sign statements from Wikidata (property P780,
"symptoms and signs") and emit a TSV for biobtree's wikidata_symptom dataset.

Why this dataset exists
-----------------------
The authoritative *structured* biomedical disease->phenotype source (HPO's
phenotype.hpoa, already ingested by biobtree) is curated through OMIM / Orphanet
/ DECIPHER and is therefore rare/Mendelian-disease skewed: common diseases
(type 2 diabetes, asthma, migraine, hypertension) carry few or no HPO
annotations. SNOMED CT does cover common-disease clinical findings but is
license-restricted and cannot be redistributed. Wikidata P780 is CC0 and,
being editor-curated from general medical knowledge, fills exactly the
common-disease symptom gap the structured ontologies leave empty. It is
general-knowledge data, not clinical-grade curation, and is labelled as such.

Scope / gating
--------------
Only statements whose disease subject carries a Mondo (P5270) or DOID (P699)
mapping are kept, so every emitted row anchors to a disease node that already
exists in biobtree. Unmapped P780 subjects are overwhelmingly chemical /
toxicology items (poisons, drug toxicity) and are dropped as noise.

Output (TSV, tab-separated, one row per disease-symptom edge)
-------------------------------------------------------------
    symptom_qid   symptom_label   mondo   doid   disease_qid

`mondo` is normalised from Wikidata's `MONDO_xxxxxxx` to biobtree's
`MONDO:xxxxxxx`; `doid` is already `DOID:xxxxx`. Either (not both) may be empty.
`disease_qid` is the disease's own Wikidata item (e.g. Q11081 = Alzheimer's),
exposed so a disease term can link directly to its Wikidata page.
Rows are de-duplicated on (symptom_qid, mondo, doid, disease_qid).
"""

import argparse
import csv
import json
import os
import sys
import time
import urllib.parse
import urllib.request

WDQS = "https://query.wikidata.org/sparql"
PAGE = 5000
UA = "biobtree-wikidata-symptom/1.0 (https://biobtree.org; mailto:biobtree@biobtree.org)"


def _page(offset):
    # ?d is the disease's Wikidata item (e.g. Q11081 = Alzheimer's); exposed so a
    # disease term can link straight to its Wikidata page.
    q = f"""SELECT ?d ?s ?sLabel ?mondo ?doid WHERE {{
      ?d wdt:P780 ?s .
      ?s rdfs:label ?sLabel . FILTER(LANG(?sLabel)="en")
      OPTIONAL {{ ?d wdt:P5270 ?mondo }}
      OPTIONAL {{ ?d wdt:P699 ?doid }}
      FILTER( BOUND(?mondo) || BOUND(?doid) )
    }} LIMIT {PAGE} OFFSET {offset}"""
    url = WDQS + "?" + urllib.parse.urlencode({"query": q, "format": "json"})
    req = urllib.request.Request(
        url, headers={"Accept": "application/sparql-results+json", "User-Agent": UA}
    )
    last = None
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                return json.load(r)["results"]["bindings"]
        except Exception as e:  # transient WDQS throttling / timeouts
            last = e
            time.sleep(3 * (attempt + 1))
    raise RuntimeError(f"WDQS query failed after retries: {last}")


def _local(value):
    """Last path segment of a Wikidata entity/URI value."""
    return value.rsplit("/", 1)[-1]


def extract(test_mode=False):
    rows = []
    offset = 0
    while True:
        batch = _page(offset)
        rows.extend(batch)
        if len(batch) < PAGE:
            break
        offset += PAGE
        time.sleep(1)
        if test_mode and offset >= PAGE:  # one page is plenty for a smoke run
            break

    seen = set()
    out = []
    for r in rows:
        disease = _local(r["d"]["value"])  # disease Wikidata QID, e.g. Q11081
        sym = _local(r["s"]["value"])
        label = r["sLabel"]["value"]
        mondo = r.get("mondo", {}).get("value", "")
        if mondo:
            mondo = _local(mondo).replace("MONDO_", "MONDO:")
        doid = r.get("doid", {}).get("value", "")  # P699 is already "DOID:xxxxx"
        key = (sym, mondo, doid, disease)
        if key in seen:
            continue
        seen.add(key)
        out.append((sym, label, mondo, doid, disease))
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--output", default="raw_data/wikidata/wikidata_symptoms.tsv",
                    help="output TSV path (default: raw_data/wikidata/wikidata_symptoms.tsv)")
    ap.add_argument("--test-mode", action="store_true", help="fetch a single page only (smoke run)")
    args = ap.parse_args()

    out = extract(test_mode=args.test_mode)
    os.makedirs(os.path.dirname(os.path.abspath(args.output)), exist_ok=True)
    with open(args.output, "w", newline="") as f:
        w = csv.writer(f, delimiter="\t")
        w.writerow(["symptom_qid", "symptom_label", "mondo", "doid", "disease_qid"])
        w.writerows(out)

    diseases = {(m, d) for _, _, m, d, _ in out}
    symptoms = {s for s, _, _, _, _ in out}
    print(f"wikidata_symptom: wrote {len(out)} disease-symptom edges "
          f"({len(diseases)} diseases, {len(symptoms)} symptom terms) to {args.output}",
          file=sys.stderr)


if __name__ == "__main__":
    main()
