#!/usr/bin/env python3
"""
gnomAD v4 per-variant Test Suite.

Validates biobtree's per-variant population-frequency layer (AF / grpmax / FAF /
per-ancestry AF), keyed chr:pos:ref:alt (GRCh38) — same scheme as alphamissense
/ spliceai, and DISTINCT from the gene-level gnomad_constraint (id 800).

Data under test is the hand-crafted VCF fixture
(tests/datasets/gnomad_variant/gnomad_variant_fixture.vcf), NOT real gnomAD data.
"""

import sys
import os
import requests
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


def _rows(data, attr_name):
    out = []
    for r in (data or {}).get("results", []):
        if attr_name in (r.get("Attributes") or {}):
            out.append(r)
    return out


class GnomadVariantTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner
        self.api = runner.api_url

    def _variants(self):
        ref = self.runner.reference_data
        return ref.get("variants", []) if isinstance(ref, dict) else []

    def _get(self, params):
        return requests.get(f"{self.api}/ws/", params=params, timeout=15).json()

    @test
    def test_variant_lookup(self):
        """chr:pos:ref:alt resolves to a GnomadVariant record with AF fields"""
        for v in self._variants():
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                a = r["Attributes"]["GnomadVariant"]
                for f in ("af", "af_grpmax", "grpmax_ancestry"):
                    if f not in a:
                        return False, f"{v['id']} missing field {f}"
                return True, (f"{v['id']} -> af={a.get('af')} grpmax={a.get('af_grpmax')} "
                              f"({a.get('grpmax_ancestry')})")
        return False, "No chr:pos:ref:alt id resolved to a GnomadVariant record"

    @test
    def test_key_scheme_chr_pos_ref_alt(self):
        """Entry ID is chr:pos:ref:alt (3 colons), NOT chr:pos"""
        for v in self._variants():
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                ident = r.get("identifier", "")
                if ident.count(":") != 3:
                    return False, f"unexpected key '{ident}' (expected chr:pos:ref:alt)"
                return True, f"key scheme OK: '{ident}' is chr:pos:ref:alt"
        return False, "No GnomadVariant record found to check key scheme"

    @test
    def test_af_values(self):
        """Stored AF / grpmax match the fixture"""
        for v in self._variants():
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                a = r["Attributes"]["GnomadVariant"]
                if abs(float(a.get("af", -1)) - float(v["af"])) > 1e-9:
                    return False, f"{v['id']} af: got {a.get('af')}, want {v['af']}"
                if a.get("grpmax_ancestry") != v["grpmax_ancestry"]:
                    return False, f"{v['id']} grpmax_ancestry: got {a.get('grpmax_ancestry')}, want {v['grpmax_ancestry']}"
                return True, f"{v['id']} af/grpmax match fixture"
        return False, "No GnomadVariant record matched for value check"

    @test
    def test_dbsnp_rsid_join(self):
        """rsID reaches the variant frequency via the dbsnp hub (rs2691305 -> 1:69094:G:A)"""
        data = requests.get(f"{self.api}/ws/map/",
                            params={"i": "rs2691305", "m": ">>dbsnp>>gnomad_variant"},
                            timeout=15).json()
        idents = {t.get("identifier")
                  for res in data.get("results", []) for t in res.get("targets", [])}
        if "1:69094:G:A" in idents:
            return True, "rs2691305 >>dbsnp>>gnomad_variant -> 1:69094:G:A"
        return False, f"rsID->dbsnp->gnomad_variant join failed; got {sorted(idents)[:5]}"

    @test
    def test_cel_filter_rare(self):
        """CEL filter af<0.001 keeps rare variants and drops the common one (1:69094 af=0.152)"""
        ids = ",".join(v["id"] for v in self._variants())
        data = self._get({"i": ids, "d": "1", "f": "gnomad_variant.af < 0.001"})
        rows = _rows(data, "GnomadVariant")
        if not rows:
            return False, "filter returned no results"
        idents = {r.get("identifier") for r in rows}
        if "1:69094:G:A" in idents:
            return False, "common variant 1:69094:G:A (af=0.152) leaked past af<0.001 filter"
        for r in rows:
            if float(r["Attributes"]["GnomadVariant"].get("af", 1)) >= 0.001:
                return False, f"filter leaked {r.get('identifier')}"
        return True, f"CEL filter af<0.001 kept {len(rows)} rare variants"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = GnomadVariantTests(runner)
    for m in [
        custom.test_variant_lookup,
        custom.test_key_scheme_chr_pos_ref_alt,
        custom.test_af_values,
        custom.test_dbsnp_rsid_join,
        custom.test_cel_filter_rare,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
