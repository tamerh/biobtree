#!/usr/bin/env python3
"""
Extract Cellosaurus reference data for tests.

Streams the Cellosaurus flat file (UniProt line format) and samples cell-line
entries with a name, species and disease so the test suite has known-good
CVCL_ accessions/fields to validate against a running biobtree server. CC BY 4.0.
"""

import json
import urllib.request
from pathlib import Path

URL = "https://ftp.expasy.org/databases/cellosaurus/cellosaurus.txt"
N = 30


def main():
    out = Path(__file__).parent / "reference_data.json"
    cells = []
    cur = {}
    with urllib.request.urlopen(URL, timeout=180) as resp:
        for raw in resp:
            line = raw.decode("utf-8", "replace").rstrip("\n")
            if line == "//":
                # keep entries that have an accession, a disease and human species
                if cur.get("ac") and cur.get("disease") and cur.get("human"):
                    cells.append({
                        "accession": cur["ac"],
                        "name": cur.get("name", ""),
                        "disease": cur["disease"],
                    })
                    if len(cells) >= N:
                        break
                cur = {}
                continue
            if len(line) < 5:
                continue
            code, val = line[:2], line[5:].strip()
            if code == "AC" and "ac" not in cur:
                cur["ac"] = val
            elif code == "ID" and "name" not in cur:
                cur["name"] = val
            elif code == "DI" and "disease" not in cur:
                parts = val.split(";")
                if len(parts) >= 3:
                    cur["disease"] = parts[2].strip()
            elif code == "OX" and "NCBI_TaxID=9606" in val:
                cur["human"] = True

    with open(out, "w") as f:
        json.dump({"cells": cells}, f, indent=2)
    print(f"Wrote {len(cells)} cell lines to {out}")


if __name__ == "__main__":
    main()
