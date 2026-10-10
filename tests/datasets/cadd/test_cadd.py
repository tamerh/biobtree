#!/usr/bin/env python3
"""
CADD (Combined Annotation Dependent Depletion) Test Suite.

Validates biobtree's CADD deleteriousness layer. PHRED-scaled C-score (higher =
more deleterious; ~20 ≈ top 1% of variants, a common pathogenicity threshold).

KEY SCHEME: "chr:pos:ref:alt" (GRCh38). Minimal phred-only storage (raw_score
intentionally dropped). Each (ref,alt) at a position is a distinct key. Reached
by DIRECT ENTRY LOOKUP of the variant key — CADD has no reverse/gene links, the
same access pattern as the conservation federation.

Data under test is the hand-crafted fixture (tests/datasets/cadd/cadd_fixture.tsv),
NOT real CADD data.
"""

import sys
import os
import re
import requests
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


def _attr_rows(data, attr_name):
    out = []
    for r in (data or {}).get("results", []):
        attrs = r.get("Attributes") or {}
        if attr_name in attrs:
            out.append(r)
    return out


class CaddTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _variants(self):
        ref = self.runner.reference_data
        return ref.get("variants", []) if isinstance(ref, dict) else []

    def _find(self, vid):
        for r in _attr_rows(self.runner.lookup(vid), "Cadd"):
            if r.get("identifier") == vid:
                return r["Attributes"]["Cadd"]
        return None

    @test
    def test_variant_lookup(self):
        """A chr:pos:ref:alt id resolves to a CADD record with the phred field"""
        for v in self._variants():
            a = self._find(v["id"])
            if a is not None:
                if "phred" not in a:
                    return False, f"{v['id']} missing 'phred' field"
                return True, f"{v['id']} -> phred={a.get('phred')}"
        return False, "No chr:pos:ref:alt id resolved to a CADD record"

    @test
    def test_key_scheme_chr_pos_ref_alt(self):
        """Entry ID is chr:pos:ref:alt (three colon-separated fields)"""
        pat = re.compile(r"^[0-9XYM]+:[0-9]+:[ACGT]+:[ACGT]+$")
        for v in self._variants():
            for r in _attr_rows(self.runner.lookup(v["id"]), "Cadd"):
                ident = r.get("identifier", "")
                if ident.count(":") != 3:
                    return False, f"unexpected key '{ident}' (expected chr:pos:ref:alt)"
                if not pat.match(ident):
                    return False, f"key '{ident}' does not match chr:pos:ref:alt pattern"
                return True, f"key scheme OK: '{ident}' is chr:pos:ref:alt"
        return False, "No CADD record found to check key scheme"

    @test
    def test_phred_values(self):
        """Stored PHRED scores match the fixture (string-preserved, incl. decimals)"""
        checked = 0
        for v in self._variants():
            a = self._find(v["id"])
            if a is None:
                continue
            got = a.get("phred", "")
            # compare numerically to be robust to trailing-zero formatting
            if abs(float(got) - float(v["phred"])) > 0.001:
                return False, f"{v['id']} phred: got {got}, want {v['phred']}"
            checked += 1
        if checked == 0:
            return False, "No CADD record matched for value check"
        return True, f"{checked} PHRED score(s) match the fixture"

    @test
    def test_same_position_distinct_alts(self):
        """Two alts at one position are distinct keys with distinct scores"""
        ac = self._find("1:69091:A:C")
        ag = self._find("1:69091:A:G")
        if ac is None or ag is None:
            return False, "expected both 1:69091:A:C and 1:69091:A:G to resolve"
        if ac.get("phred") == ag.get("phred"):
            return False, "the two alts collapsed to one score"
        return True, f"A:C phred={ac.get('phred')} vs A:G phred={ag.get('phred')} (distinct)"

    @test
    def test_cel_filter_deleterious(self):
        """CEL filter cadd.phred>20 keeps deleterious variants, drops benign"""
        ids = ",".join(v["id"] for v in self._variants())
        url = f"{self.runner.api_url}/ws/?i={ids}&d=1&f=double(cadd.phred)>20.0"
        resp = requests.get(url, timeout=15)
        if resp.status_code != 200:
            return False, f"HTTP {resp.status_code}: {resp.text[:200]}"
        rows = _attr_rows(resp.json(), "Cadd")
        if not rows:
            return False, "filter returned no CADD results"
        for r in rows:
            pv = float(r["Attributes"]["Cadd"].get("phred", 0.0))
            if pv <= 20.0:
                return False, f"filter leaked {r.get('identifier')} phred={pv}"
        idents = {r.get("identifier") for r in rows}
        if "2:179431991:G:T" in idents:
            return False, "benign 2:179431991:G:T (phred 1.2) was not filtered out"
        return True, f"CEL filter cadd.phred>20 kept {len(rows)} deleterious variant(s)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file if test_cases_file.exists() else None)
    custom = CaddTests(runner)
    for m in [
        custom.test_variant_lookup,
        custom.test_key_scheme_chr_pos_ref_alt,
        custom.test_phred_values,
        custom.test_same_position_distinct_alts,
        custom.test_cel_filter_deleterious,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
