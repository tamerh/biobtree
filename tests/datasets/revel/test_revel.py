#!/usr/bin/env python3
"""
REVEL (ensemble missense pathogenicity) Test Suite.

Validates biobtree's REVEL predictor layer. Score is 0-1 (higher = more
pathogenic); ClinGen SVI / Pejaver 2022 provide calibrated PP3/BP4 thresholds.

KEY SCHEME: "chr:pos:ref:alt" (GRCh38) — co-locates with AlphaMissense/SpliceAI.
Multi-transcript rows for the same variant collapse to one entry with the MAX
REVEL and all transcript ids; rows with a blank grch38_pos are skipped.

Data under test is the hand-crafted fixture (tests/datasets/revel/revel_fixture.csv),
NOT real REVEL data.
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


class RevelTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _variants(self):
        ref = self.runner.reference_data
        return ref.get("variants", []) if isinstance(ref, dict) else []

    def _find(self, vid):
        for r in _attr_rows(self.runner.lookup(vid), "Revel"):
            if r.get("identifier") == vid:
                return r["Attributes"]["Revel"]
        return None

    @test
    def test_variant_lookup(self):
        """A chr:pos:ref:alt id resolves to a Revel record with the score field"""
        for v in self._variants():
            a = self._find(v["id"])
            if a is not None:
                if "revel" not in a:
                    return False, f"{v['id']} missing 'revel' field"
                return True, f"{v['id']} -> revel={a.get('revel')} aa={a.get('aaref')}>{a.get('aaalt')}"
        return False, "No chr:pos:ref:alt id resolved to a Revel record"

    @test
    def test_key_scheme_chr_pos_ref_alt(self):
        """Entry ID is chr:pos:ref:alt (three colon-separated fields)"""
        pat = re.compile(r"^[0-9XYM]+:[0-9]+:[ACGT]+:[ACGT]+$")
        for v in self._variants():
            for r in _attr_rows(self.runner.lookup(v["id"]), "Revel"):
                ident = r.get("identifier", "")
                if ident.count(":") != 3:
                    return False, f"unexpected key '{ident}' (expected chr:pos:ref:alt)"
                if not pat.match(ident):
                    return False, f"key '{ident}' does not match chr:pos:ref:alt pattern"
                return True, f"key scheme OK: '{ident}' is chr:pos:ref:alt"
        return False, "No revel record found to check key scheme"

    @test
    def test_score_values(self):
        """Stored REVEL scores match the fixture"""
        for v in self._variants():
            a = self._find(v["id"])
            if a is None:
                continue
            got = float(a.get("revel", -1))
            want = float(v["revel"])
            if abs(got - want) > 0.01:
                return False, f"{v['id']} revel: got {got}, want {want}"
            return True, f"{v['id']} revel {got} matches fixture"
        return False, "No revel record matched for value check"

    @test
    def test_multitranscript_dedup(self):
        """Multi-transcript variant collapses to MAX REVEL + all transcript ids"""
        a = self._find("10:87864533:G:A")
        if a is None:
            return False, "dedup variant 10:87864533:G:A not found"
        got = float(a.get("revel", -1))
        if abs(got - 0.940) > 0.01:
            return False, f"expected max REVEL 0.940 (not 0.932), got {got}"
        tids = a.get("transcript_ids") or []
        if len(tids) != 2:
            return False, f"expected 2 transcript ids, got {len(tids)}: {tids}"
        return True, f"dedup OK: revel={got} (max), transcripts={tids}"

    @test
    def test_transcript_split(self):
        """';'-separated Ensembl_transcriptid field is split into individual ids"""
        a = self._find("1:35142:G:A")
        if a is None:
            return False, "1:35142:G:A not found"
        tids = a.get("transcript_ids") or []
        if set(tids) != {"ENST00000417324", "ENST00000622660"}:
            return False, f"expected 2 split transcript ids, got {tids}"
        return True, f"';'-split OK: {tids}"

    @test
    def test_cel_filter_high_revel(self):
        """CEL filter revel>0.5 keeps pathogenic-leaning variants, drops benign"""
        ids = ",".join(v["id"] for v in self._variants())
        url = f"{self.runner.api_url}/ws/?i={ids}&d=1&f=revel.revel>0.5"
        resp = requests.get(url, timeout=15)
        if resp.status_code != 200:
            return False, f"HTTP {resp.status_code}"
        rows = _attr_rows(resp.json(), "Revel")
        if not rows:
            return False, "filter returned no results"
        for r in rows:
            rv = float(r["Attributes"]["Revel"].get("revel", 0.0))
            if rv <= 0.5:
                return False, f"filter leaked {r.get('identifier')} revel={rv}"
        idents = {r.get("identifier") for r in rows}
        if "1:35142:G:A" in idents:
            return False, "benign 1:35142:G:A (revel 0.027) was not filtered out"
        return True, f"CEL filter revel>0.5 kept {len(rows)} variant(s)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = RevelTests(runner)
    for m in [
        custom.test_variant_lookup,
        custom.test_key_scheme_chr_pos_ref_alt,
        custom.test_score_values,
        custom.test_multitranscript_dedup,
        custom.test_transcript_split,
        custom.test_cel_filter_high_revel,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
