#!/usr/bin/env python3
"""
DrugCentral extras prep.

DrugCentral's on-label indications and WHO ATC classification are NOT in the
static TSV/CSV downloads — they live only in the full PostgreSQL dump (~1 GB
gzipped). Rather than stream that whole dump at every build, this one-off prep
extracts the three small tables biobtree needs and writes them to
raw_data/drugcentral/, which the drugcentral parser reads locally:

  - atc.tsv                : ATC code -> level-1..4 class names
  - struct2atc.tsv         : struct_id -> ATC code
  - omop_indications.tsv   : struct_id -> on-label indication disease name
                             (omop_relationship rows where relationship_name='indication')

Re-run only when bumping the DrugCentral release. License: CC BY-SA 4.0.

Usage: python3 src/scripts/drugcentral/drugcentral_prepare.py
"""
import gzip
import io
import os
import re
import sys
import urllib.request

DUMP_URL = "https://unmtid-dbs.net/download/DrugCentral/20200918/drugcentral-pgdump_20200918.sql.gz"
OUT_DIR = "raw_data/drugcentral"

COPY_RE = re.compile(r'^COPY (?:public\.)?"?(\w+)"?\s*\(')
WANT = {"atc", "struct2atc", "omop_relationship"}


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    req = urllib.request.Request(DUMP_URL, headers={"User-Agent": "curl/8"})
    print(f"Streaming DrugCentral pg_dump: {DUMP_URL}", flush=True)
    resp = urllib.request.urlopen(req, timeout=180)
    gz = gzip.GzipFile(fileobj=resp)
    reader = io.TextIOWrapper(gz, encoding="utf-8", errors="replace")

    files = {
        "atc": open(os.path.join(OUT_DIR, "atc.tsv"), "w", encoding="utf-8"),
        "struct2atc": open(os.path.join(OUT_DIR, "struct2atc.tsv"), "w", encoding="utf-8"),
        "omop_relationship": open(os.path.join(OUT_DIR, "omop_indications.tsv"), "w", encoding="utf-8"),
    }
    counts = {k: 0 for k in files}
    seen = set()
    cur = None
    for line in reader:
        if cur is None:
            m = COPY_RE.match(line)
            if m and m.group(1) in WANT and m.group(1) not in seen:
                cur = m.group(1)
            continue
        if line.rstrip("\n") == r"\.":
            seen.add(cur)
            cur = None
            if WANT <= seen:
                break
            continue
        if cur == "omop_relationship":
            # keep only on-label indications (field 3 = relationship_name)
            f = line.rstrip("\n").split("\t")
            if len(f) < 5 or f[3] != "indication":
                continue
        files[cur].write(line)
        counts[cur] += 1

    for fh in files.values():
        fh.close()
    print(f"wrote atc={counts['atc']} struct2atc={counts['struct2atc']} "
          f"omop_indications={counts['omop_relationship']} rows to {OUT_DIR}/", flush=True)
    if not (WANT <= seen):
        print(f"WARNING: not all tables found (missing {WANT - seen})", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
