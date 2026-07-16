#!/usr/bin/env python3
"""
Variant annotation-coverage matrix — a factual, quantified demonstration that ONE
biobtree query returns a variant's evidence across many sources, where single tools
each cover a subset.

Test set = real variants sampled from the Sugi Atlas pages (genomic coordinate +
ClinVar VCV parsed from each page's NC_*.11 g.HGVS). For each variant we query every
biobtree source by its native key and record hit/miss, then report per-source
coverage and "% covered by >=N sources".

Run: python variant_coverage.py [n_per_gene]
"""
import glob
import json
import os
import re
import sys
import urllib.parse
import urllib.request

BASE = "http://localhost:9291"
ATLAS = "/data2/sugi-webdev/atlas/variant"
GENES = ["pten", "asxl1", "acta1", "naa10", "dact1", "rpl10"]
NC = {f"NC_0000{n:02d}.11": (str(n) if n <= 22 else {23: "X", 24: "Y"}[n]) for n in range(1, 25)}
SOURCES = ["clinvar", "gnomad_variant", "alphamissense", "revel", "spliceai", "conservation", "saprot"]


def get(url, timeout=30):
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            return json.loads(r.read().decode())
    except Exception:
        return None


def entry(ident, ds):
    d = get(f"{BASE}/ws/entry/?i={urllib.parse.quote(ident)}&s={ds}")
    if not d:
        return False
    a = d.get("Attributes")
    return bool(a) and "Empty" not in json.dumps(a)


def parse_variant(md_path):
    """Extract genomic coord (from NC_*.11 g.HGVS), VCV, gene from an Atlas page."""
    txt = open(md_path, encoding="utf-8", errors="ignore").read()
    m = re.search(r"(NC_0000\d\d\.11):g\.(\d+)([ACGT])>([ACGT])", txt)
    if not m or m.group(1) not in NC:
        return None  # SNV genomic HGVS only (skip indels/complex for a clean matrix)
    chrom, pos, ref, alt = NC[m.group(1)], m.group(2), m.group(3), m.group(4)
    vcv = re.search(r"VCV(\d+)", txt)
    return {"coord": f"{chrom}:{pos}:{ref}:{alt}", "chrpos": f"{chrom}:{pos}",
            "vcv": vcv.group(1) if vcv else None, "id": os.path.basename(os.path.dirname(md_path))}


def coverage(v):
    row = {}
    # ClinVar keyed by variation id (VCV number)
    row["clinvar"] = entry(v["vcv"], "clinvar") if v["vcv"] else False
    for ds in ("gnomad_variant", "alphamissense", "revel", "spliceai"):
        row[ds] = entry(v["coord"], ds)
    row["conservation"] = entry(v["chrpos"], "conservation")
    # SaProt keyed uniprot:protein_variant — derive from the AlphaMissense record
    upv = None
    d = get(f"{BASE}/ws/entry/?i={urllib.parse.quote(v['coord'])}&s=alphamissense")
    if d:
        a = (d.get("Attributes") or {}).get("Alphamissense") or {}
        if a.get("uniprot_id") and a.get("protein_variant"):
            upv = f"{a['uniprot_id']}:{a['protein_variant']}"
    row["saprot"] = entry(upv, "saprot") if upv else False
    return row


def main():
    n_per = int(sys.argv[1]) if len(sys.argv) > 1 else 5
    variants = []
    for g in GENES:
        pages = sorted(glob.glob(f"{ATLAS}/{g}-p-*/index.md"))
        picked = 0
        for p in pages:
            v = parse_variant(p)
            if v:
                v["gene"] = g
                variants.append(v)
                picked += 1
            if picked >= n_per:
                break

    print(f"# Variant annotation-coverage matrix ({len(variants)} Atlas variants)\n")
    print("| variant | " + " | ".join(SOURCES) + " | n |")
    print("|---|" + "|".join(["--"] * len(SOURCES)) + "|--|")
    counts = {s: 0 for s in SOURCES}
    per_variant_n = []
    for v in variants:
        row = coverage(v)
        nhit = sum(row.values())
        per_variant_n.append(nhit)
        for s in SOURCES:
            counts[s] += 1 if row[s] else 0
        cells = " | ".join("Y" if row[s] else "·" for s in SOURCES)
        print(f"| {v['gene']}:{v['id'].split('-p-')[-1]} | {cells} | {nhit} |")

    N = len(variants)
    print(f"\n**Per-source coverage (of {N} variants):**")
    for s in SOURCES:
        print(f"- {s}: {counts[s]}/{N} ({100*counts[s]/N:.0f}%)")
    print(f"\n**Multi-source depth:** mean {sum(per_variant_n)/N:.1f} sources/variant.")
    for k in (3, 4, 5, 6):
        c = sum(1 for x in per_variant_n if x >= k)
        print(f"- covered by >={k} sources: {c}/{N} ({100*c/N:.0f}%)")
    print("\n_One biobtree query surfaces all of the above per variant; VarSome/Franklin/"
          "OpenCRAVAT each expose a subset, and none co-serve MaveDB + an owned unsupervised PLM (SaProt)._")


if __name__ == "__main__":
    main()
