#!/usr/bin/env python3
"""
check_versions.py — detect new upstream releases for VERSION/DATE-PINNED datasets.

Why this exists
---------------
`biobtree check --all` compares each source against a stored baseline (etag / date /
version / mtime). That works for datasets whose "new data" is a change to a STABLE
url. It is BLIND to datasets whose download url hard-codes a version or month:

    string     .../protein.links.detailed.v12.0/     <- a v12.5 at a new url is invisible
    bindingdb  BindingDB_All_202606_tsv.zip          <- last month's file is DELETED -> 404

For those, `check --all` reports "NO CHANGE" (frozen url unchanged) or the download
simply 404s. This script closes that gap: for each pinned dataset it discovers the
LATEST upstream version and compares it to what conf is pinned to.

Usage
-----
    python3 src/scripts/version_check/check_versions.py            # report
    python3 src/scripts/version_check/check_versions.py --fix      # + bump conf for auto-detected

Exit code is non-zero if any auto-checkable dataset is BEHIND (handy for CI/cron).
"""
import json
import os
import re
import sys
import urllib.request
from datetime import date, timedelta

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
MANIFEST = os.path.join(HERE, "version_manifest.json")
TIMEOUT = 25


def http_ok(url):
    """True if the url exists (200). Try HEAD, fall back to a 1-byte ranged GET."""
    for method in ("HEAD", "GET"):
        try:
            req = urllib.request.Request(url, method=method)
            req.add_header("User-Agent", "biobtree-version-check/1.0")
            if method == "GET":
                req.add_header("Range", "bytes=0-0")
            with urllib.request.urlopen(req, timeout=TIMEOUT) as r:
                if r.status in (200, 206):
                    return True
        except Exception:
            continue
    return False


def parse_current(conf, entry):
    """Extract our pinned version from the configured conf field via current_regex."""
    val = str(conf.get(entry["conf_field"], ""))
    m = re.search(entry["current_regex"], val)
    return m.group(1) if m else None


def latest_monthly(entry):
    """Newest YYYYMM (as string) for which the templated url exists; probe now+1 .. now-7."""
    today = date.today().replace(day=1)
    months = []
    d = today.replace(day=1)
    # start one month ahead (in case a release just dropped), walk back 8 months
    ahead = (d.month % 12) + 1
    ahead_year = d.year + (1 if d.month == 12 else 0)
    months.append(f"{ahead_year}{ahead:02d}")
    for i in range(0, 8):
        yy = d.year
        mm = d.month - i
        while mm <= 0:
            mm += 12
            yy -= 1
        months.append(f"{yy}{mm:02d}")
    seen = []
    for ym in sorted(set(months), reverse=True):
        url = entry["url_template"].replace("{YYYYMM}", ym)
        if http_ok(url):
            return ym
        seen.append(ym)
    return None


def latest_dir(entry):
    """Highest candidate version for which the templated url exists."""
    def vkey(v):
        return tuple(int(x) for x in v.split("."))
    for v in sorted(entry.get("candidates", []), key=vkey, reverse=True):
        url = entry["url_template"].replace("{V}", v)
        if http_ok(url):
            return v
    return None


def cmp_version(a, b):
    """-1/0/1 comparing dotted-or-numeric version strings a vs b (None-safe)."""
    if a is None or b is None:
        return 0
    def key(v):
        parts = re.split(r"[._]", str(v))
        return tuple(int(p) if p.isdigit() else p for p in parts)
    ka, kb = key(a), key(b)
    return (ka > kb) - (ka < kb)


def main():
    fix = "--fix" in sys.argv
    man = json.load(open(MANIFEST))
    conf_path = os.path.join(REPO, man["conf_file"])
    conf = json.load(open(conf_path))
    conf_text = open(conf_path).read()

    rows, behind, fixes = [], 0, {}
    for name, entry in man["datasets"].items():
        c = conf.get(name, {})
        current = parse_current(c, entry)
        method = entry["method"]
        latest, status = None, ""
        if method == "monthly_probe":
            latest = latest_monthly(entry)
            status = "BEHIND" if cmp_version(latest, current) > 0 else ("CURRENT" if latest else "PROBE-FAIL")
        elif method == "dir_probe":
            latest = latest_dir(entry)
            status = "BEHIND" if cmp_version(latest, current) > 0 else ("CURRENT" if latest else "PROBE-FAIL")
        else:  # manual
            status = "MANUAL"
        if status == "BEHIND":
            behind += 1
            if current and latest:
                fixes[name] = (current, latest)
        rows.append((name, method, current or "?", latest or "-", status, entry.get("reference", "")))

    w = max(len(r[0]) for r in rows)
    print(f"\n{'DATASET':<{w}}  {'METHOD':<13} {'PINNED':<10} {'LATEST':<10} STATUS")
    print("-" * (w + 46))
    icon = {"BEHIND": "⬆", "CURRENT": "✓", "MANUAL": "·", "PROBE-FAIL": "?"}
    for name, method, cur, lat, status, ref in rows:
        print(f"{name:<{w}}  {method:<13} {cur:<10} {lat:<10} {icon.get(status,'')} {status}")
        if status == "MANUAL":
            print(f"{'':<{w}}     check: {ref}")
        elif status == "BEHIND":
            print(f"{'':<{w}}     NEW RELEASE — update conf (ref: {ref})")

    print(f"\nAuto-checked: {sum(1 for r in rows if r[1] in ('monthly_probe','dir_probe'))} | "
          f"BEHIND: {behind} | MANUAL (verify by hand): {sum(1 for r in rows if r[4]=='MANUAL')}")

    if fix and fixes:
        for name, (cur, lat) in fixes.items():
            conf_text = conf_text.replace(cur, lat)
            print(f"  --fix: {name} {cur} -> {lat} (all conf occurrences)")
        with open(conf_path, "w") as f:
            f.write(conf_text)
        print(f"  wrote {conf_path} — review the diff before committing")
    elif fix:
        print("  --fix: nothing to bump (no auto-detected BEHIND)")

    return 1 if behind else 0


if __name__ == "__main__":
    sys.exit(main())
