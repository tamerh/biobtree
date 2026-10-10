#!/usr/bin/env python3
"""Pangolin splice-score test suite."""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner


class PangolinTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _ref(self, k):
        r = self.runner.reference_data
        return r.get(k, []) if isinstance(r, dict) else []

    def _attr(self, vid):
        for r in requests.get(f"{self.api}/ws/", params={"i": vid, "s": "pangolin", "d": "0"}, timeout=15).json().get("results", []):
            if r.get("dataset_name") == "pangolin":
                return r["Attributes"].get("Pangolin", {})
        return None

    def test_present(self):
        """Gain/loss variants kept with correct effect + gene"""
        for v in self._ref("present"):
            a = self._attr(v["id"])
            if a is None:
                return False, f"{v['id']} missing"
            if a.get("effect") != v["effect"]:
                return False, f"{v['id']} effect {a.get('effect')} != {v['effect']}"
            if a.get("gene") != v["gene"]:
                return False, f"{v['id']} gene {a.get('gene')} != {v['gene']}"
        return True, f"{len(self._ref('present'))} variants present (gain+loss)"

    def test_filtered(self):
        """Variants with largest delta < 0.2 are dropped"""
        for vid in self._ref("absent"):
            if self._attr(vid) is not None:
                return False, f"{vid} should be filtered"
        return True, "below-threshold filtered"


def main():
    sd = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'), sd / "reference_data.json")
    c = PangolinTests(runner)
    for m in [c.test_present, c.test_filtered]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
