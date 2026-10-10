#!/usr/bin/env python3
"""
GTEx (v10 cis-QTL) test suite.

Variant-keyed (chr:pos:ref:alt) regulatory evidence: one entry per variant holds
all its significant cis-QTL associations across tissues, each tagged eqtl/sqtl.
Each distinct affected gene also gets a variant->ensembl xref.

Data under test is the hand-crafted fixture (tests/datasets/gtex/gtex_fixture.tsv),
NOT real GTEx data.
"""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner, test


def _rows(data, attr_name):
    return [r for r in (data or {}).get("results", []) if attr_name in (r.get("Attributes") or {})]


class GtexTests:
    def __init__(self, runner):
        self.runner = runner

    def _variants(self):
        ref = self.runner.reference_data
        return ref.get("variants", []) if isinstance(ref, dict) else []

    def _attr(self, vid):
        for r in _rows(self.runner.lookup(vid), "Gtex"):
            if r.get("identifier") == vid:
                return r["Attributes"]["Gtex"]
        return None

    @test
    def test_variant_lookup(self):
        """A chr:pos:ref:alt id resolves to a GTEx record with associations"""
        for v in self._variants():
            a = self._attr(v["id"])
            if a is not None:
                assoc = a.get("associations") or []
                if not assoc:
                    return False, f"{v['id']} has no associations"
                return True, f"{v['id']} -> {len(assoc)} association(s)"
        return False, "No variant resolved to a GTEx record"

    @test
    def test_full_per_tissue_aggregation(self):
        """A multi-association variant keeps every tissue + both QTL types"""
        v = self._variants()[0]  # 1:100000:A:G -> 3 associations
        a = self._attr(v["id"])
        if a is None:
            return False, f"{v['id']} not found"
        assoc = a.get("associations") or []
        if len(assoc) != v["n_assoc"]:
            return False, f"expected {v['n_assoc']} associations, got {len(assoc)}"
        tissues = {s.get("tissue") for s in assoc}
        types = {s.get("qtl_type") for s in assoc}
        for t in v["tissues"]:
            if t not in tissues:
                return False, f"tissue {t} missing from {tissues}"
        for q in v["qtl_types"]:
            if q not in types:
                return False, f"qtl_type {q} missing from {types}"
        return True, f"{v['id']} -> {len(assoc)} assoc, tissues={sorted(tissues)}, types={sorted(types)}"

    @test
    def test_sqtl_phenotype_and_slope(self):
        """sQTL associations carry a phenotype_id; slope sign is preserved"""
        a = self._attr("7:300000:G:A")
        if a is None:
            return False, "7:300000:G:A not found"
        s = (a.get("associations") or [{}])[0]
        if s.get("qtl_type") != "sqtl" or not s.get("phenotype_id"):
            return False, f"expected sqtl with phenotype_id, got {s}"
        # the aggregating variant has a negative sQTL slope — check sign preserved
        a2 = self._attr("1:100000:A:G")
        neg = [x for x in (a2.get("associations") or []) if (x.get("slope") or "").startswith("-")]
        if not neg:
            return False, "negative slope not preserved on 1:100000:A:G"
        return True, f"sqtl phenotype={s.get('phenotype_id')[:24]}..., negative slope preserved"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'),
                        script_dir / "reference_data.json")
    c = GtexTests(runner)
    for m in [c.test_variant_lookup, c.test_full_per_tissue_aggregation, c.test_sqtl_phenotype_and_slope]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
