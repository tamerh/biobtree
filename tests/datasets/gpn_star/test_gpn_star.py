#!/usr/bin/env python3
"""
GPN-Star test suite.

Validates biobtree's GPN-Star layer — allele-specific genomic language-model
score (Benegas et al. 2025), the allele-aware complement to phyloP/phastCons.
More negative = more constrained/deleterious. Raw score, NOT ACMG-calibrated.

KEY SCHEME: "chr:pos:ref:alt" (GRCh38), score-only. Each (ref,alt) at a position
is a distinct key. Reached by DIRECT ENTRY LOOKUP of the variant key (no reverse/
gene links) — same access pattern as cadd/conservation. Own `gpn_star` federation.

Data under test is the hand-crafted fixture
(tests/datasets/gpn_star/gpn_star_fixture.tsv), NOT real GPN-Star data.
"""
import sys, os, re, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner, test


def _attr_rows(data, attr_name):
    return [r for r in (data or {}).get("results", []) if attr_name in (r.get("Attributes") or {})]


class GpnStarTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _variants(self):
        ref = self.runner.reference_data
        return ref.get("variants", []) if isinstance(ref, dict) else []

    def _find(self, vid):
        for r in _attr_rows(self.runner.lookup(vid), "GpnStar"):
            if r.get("identifier") == vid:
                return r["Attributes"]["GpnStar"]
        return None

    @test
    def test_variant_lookup(self):
        """A chr:pos:ref:alt id resolves to a GPN-Star record with the score field"""
        for v in self._variants():
            a = self._find(v["id"])
            if a is not None:
                if "score" not in a:
                    return False, f"{v['id']} missing 'score' field"
                return True, f"{v['id']} -> score={a.get('score')}"
        return False, "No chr:pos:ref:alt id resolved to a GPN-Star record"

    @test
    def test_key_scheme_chr_pos_ref_alt(self):
        """Entry ID is chr:pos:ref:alt"""
        pat = re.compile(r"^[0-9XYM]+:[0-9]+:[ACGT]+:[ACGT]+$")
        for v in self._variants():
            for r in _attr_rows(self.runner.lookup(v["id"]), "GpnStar"):
                ident = r.get("identifier", "")
                if not pat.match(ident):
                    return False, f"key '{ident}' not chr:pos:ref:alt"
                return True, f"key scheme OK: '{ident}'"
        return False, "No GPN-Star record found to check key scheme"

    @test
    def test_score_values(self):
        """Stored scores match the fixture (string-preserved, sign kept)"""
        checked = 0
        for v in self._variants():
            a = self._find(v["id"])
            if a is None:
                continue
            got = a.get("score", "")
            if abs(float(got) - float(v["score"])) > 0.001:
                return False, f"{v['id']} score: got {got}, want {v['score']}"
            checked += 1
        if checked == 0:
            return False, "No GPN-Star record matched for value check"
        return True, f"{checked} score(s) match the fixture (incl. negative sign)"

    @test
    def test_same_position_distinct_alts(self):
        """Two alts at one position are distinct keys with distinct scores"""
        a = self._find("1:69091:A:C")
        b = self._find("1:69091:A:G")
        if a is None or b is None:
            return False, "expected both 1:69091:A:C and 1:69091:A:G to resolve"
        if a.get("score") == b.get("score"):
            return False, "the two alts collapsed to one score"
        return True, f"A:C score={a.get('score')} vs A:G score={b.get('score')} (distinct)"

    @test
    def test_cel_filter_constrained(self):
        """CEL filter double(gpn_star.score) < -5 keeps constrained variants, drops tolerant"""
        ids = ",".join(v["id"] for v in self._variants())
        url = f"{self.runner.api_url}/ws/?i={ids}&d=1&f=double(gpn_star.score)<-5.0"
        resp = requests.get(url, timeout=15)
        if resp.status_code != 200:
            return False, f"HTTP {resp.status_code}: {resp.text[:200]}"
        rows = _attr_rows(resp.json(), "GpnStar")
        if not rows:
            return False, "filter returned no GPN-Star results"
        for r in rows:
            sv = float(r["Attributes"]["GpnStar"].get("score", 0.0))
            if sv >= -5.0:
                return False, f"filter leaked {r.get('identifier')} score={sv}"
        idents = {r.get("identifier") for r in rows}
        if "1:69091:A:C" in idents:
            return False, "tolerant 1:69091:A:C (score -0.52) was not filtered out"
        return True, f"CEL filter kept {len(rows)} constrained variant(s)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')
    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1
    runner = TestRunner(api_url, reference_file)
    custom = GpnStarTests(runner)
    for m in [
        custom.test_variant_lookup,
        custom.test_key_scheme_chr_pos_ref_alt,
        custom.test_score_values,
        custom.test_same_position_distinct_alts,
        custom.test_cel_filter_constrained,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
