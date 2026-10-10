#!/usr/bin/env python3
"""SpliceAI (Ensembl MANE source) test suite."""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner


class SpliceaiTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _ref(self, key):
        r = self.runner.reference_data
        return r.get(key, []) if isinstance(r, dict) else []

    def _attr(self, vid):
        for r in requests.get(f"{self.api}/ws/", params={"i": vid, "s": "spliceai", "d": "0"}, timeout=15).json().get("results", []):
            if r.get("dataset_name") == "spliceai":
                return r["Attributes"].get("Spliceai", {})
        return None

    def test_all_alts_present(self):
        """Every alt at a position is kept, with correct effect + delta (fixes #1/#2)"""
        for v in self._ref("present"):
            a = self._attr(v["id"])
            if a is None:
                return False, f"{v['id']} missing (should be present)"
            if a.get("effect") != v["effect"]:
                return False, f"{v['id']} effect {a.get('effect')} != {v['effect']}"
            if a.get("ds_dl") != v["ds_dl"]:
                return False, f"{v['id']} ds_dl {a.get('ds_dl')!r} != {v['ds_dl']!r}"
            if a.get("gene_symbol") != v["gene"]:
                return False, f"{v['id']} gene {a.get('gene_symbol')} != {v['gene']}"
        return True, f"all {len(self._ref('present'))} alts present with deltas"

    def test_below_threshold_filtered(self):
        """Variants with all deltas < 0.2 are not ingested"""
        for vid in self._ref("absent"):
            if self._attr(vid) is not None:
                return False, f"{vid} should have been filtered out"
        return True, "below-threshold variants filtered"

    def test_four_deltas_stored(self):
        """All four DS_* and DP_* fields are present"""
        a = self._attr("8:42305277:T:C")
        if a is None:
            return False, "IKBKB variant missing"
        for k in ("ds_ag", "ds_al", "ds_dg", "ds_dl", "dp_ag", "dp_al", "dp_dg", "dp_dl"):
            if k not in a:
                return False, f"missing delta field {k}"
        return True, f"4 deltas+positions present (DS_DG={a.get('ds_dg')})"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'), script_dir / "reference_data.json")
    c = SpliceaiTests(runner)
    for m in [c.test_all_alts_present, c.test_below_threshold_filtered, c.test_four_deltas_stored]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
