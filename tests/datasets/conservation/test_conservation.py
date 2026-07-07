#!/usr/bin/env python3
"""
Conservation (per-position evolutionary conservation) Test Suite.

Validates biobtree's per-base conservation layer (phyloP470way / GERP++ /
phastCons470way), sourced from the primary UCSC + GERP providers (dbNSFP itself
is CC BY-NC-ND / No-Derivatives so cannot be redistributed as a subset).

KEY SCHEME: genomic position "chr:pos" (GRCh38/hg38), ref/alt-agnostic — this
differs from the variant datasets (alphamissense/spliceai/gnomad_variant/clinvar)
which key "chr:pos:ref:alt". A variant does NOT auto-join to a conservation
record; the intended positional join (variant -> strip ref/alt -> chr:pos ->
conservation lookup) is a merge-review decision, not implemented here.

Data under test is the hand-crafted fixture
(raw_data/conservation/conservation_hg38.tsv.gz), NOT real UCSC/GERP tracks.
"""

import sys
import os
import re
import requests
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


def _attr_rows(data, attr_name):
    """Return result rows carrying the given Attribute block."""
    out = []
    for r in (data or {}).get("results", []):
        attrs = r.get("Attributes") or {}
        if attr_name in attrs:
            out.append(r)
    return out


class ConservationTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _positions(self):
        ref = self.runner.reference_data
        return ref.get("positions", []) if isinstance(ref, dict) else []

    @test
    def test_position_lookup(self):
        """A chr:pos id resolves to a Conservation record with all score fields"""
        for p in self._positions():
            for r in _attr_rows(self.runner.lookup(p["id"]), "Conservation"):
                a = r["Attributes"]["Conservation"]
                if a.get("chromosome") == p["chromosome"] and a.get("position") == p["position"]:
                    for f in ("phylop", "gerp", "phastcons"):
                        if f not in a:
                            return False, f"{p['id']} missing field {f}"
                    return True, (f"{p['id']} -> phylop={a.get('phylop')} "
                                  f"gerp={a.get('gerp')} phastcons={a.get('phastcons')}")
        return False, "No chr:pos id resolved to a Conservation record"

    @test
    def test_key_scheme_chr_pos(self):
        """Entry ID is chr:pos (two fields), NOT chr:pos:ref:alt"""
        pat = re.compile(r"^[0-9XYM]+T?:[0-9]+$")
        for p in self._positions():
            for r in _attr_rows(self.runner.lookup(p["id"]), "Conservation"):
                ident = r.get("identifier", "")
                if ident.count(":") != 1:
                    return False, f"unexpected key '{ident}' (expected chr:pos)"
                if not pat.match(ident):
                    return False, f"key '{ident}' does not match chr:pos pattern"
                return True, f"key scheme OK: '{ident}' is chr:pos (ref/alt-agnostic)"
        return False, "No conservation record found to check key scheme"

    @test
    def test_score_values(self):
        """Stored phyloP/GERP/phastCons values match the fixture (incl. negatives)"""
        for p in self._positions():
            for r in _attr_rows(self.runner.lookup(p["id"]), "Conservation"):
                a = r["Attributes"]["Conservation"]
                if a.get("position") != p["position"]:
                    continue
                for f in ("phylop", "gerp", "phastcons"):
                    got = float(a.get(f, 0.0))
                    want = float(p[f])
                    if abs(got - want) > 0.01:
                        return False, f"{p['id']} {f}: got {got}, want {want}"
                return True, f"{p['id']} scores match fixture"
        return False, "No conservation record matched for value check"

    @test
    def test_cel_filter_high_phylop(self):
        """CEL filter phylop>5.0 keeps conserved positions, drops accelerated ones"""
        ids = ",".join(p["id"] for p in self._positions())
        url = f"{self.runner.api_url}/ws/?i={ids}&d=1&f=conservation.phylop>5.0"
        resp = requests.get(url, timeout=15)
        if resp.status_code != 200:
            return False, f"HTTP {resp.status_code}"
        data = resp.json()
        rows = _attr_rows(data, "Conservation")
        if not rows:
            return False, "filter returned no results"
        for r in rows:
            phylop = float(r["Attributes"]["Conservation"].get("phylop", 0.0))
            if phylop <= 5.0:
                return False, f"filter leaked {r.get('identifier')} phylop={phylop}"
        # 1:69096 (phylop -1.23) must NOT appear.
        idents = {r.get("identifier") for r in rows}
        if "1:69096" in idents:
            return False, "accelerated position 1:69096 was not filtered out"
        return True, f"CEL filter phylop>5.0 kept {len(rows)} conserved positions"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = ConservationTests(runner)
    for m in [
        custom.test_position_lookup,
        custom.test_key_scheme_chr_pos,
        custom.test_score_values,
        custom.test_cel_filter_high_phylop,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
