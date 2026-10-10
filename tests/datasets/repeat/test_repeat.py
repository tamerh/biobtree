#!/usr/bin/env python3
"""Repeat regions (UCSC rmsk/TRF/segdup) test suite."""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner


class RepeatTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _positions(self):
        ref = self.runner.reference_data
        return ref.get("positions", []) if isinstance(ref, dict) else []

    def test_interval_query(self):
        """A variant position resolves to overlapping repeat region(s)"""
        for pz in self._positions():
            m = requests.get(f"{self.api}/ws/map/", params={"i": f"{pz['chr']}:{pz['pos']}", "m": ">>repeat"}, timeout=15).json()
            ids = [t.get("identifier", "").upper() for r in (m.get("results") or []) for t in r.get("targets", [])]
            if pz["expect_id"].upper() not in ids:
                return False, f"{pz['chr']}:{pz['pos']} did not return {pz['expect_id']} (got {ids[:5]})"
            return True, f"{pz['chr']}:{pz['pos']} -> {len(ids)} repeat(s)"
        return False, "no positions"

    def test_repeat_type(self):
        """Overlapping repeats carry a repeat_type/class"""
        for pz in self._positions():
            m = requests.get(f"{self.api}/ws/map/", params={"i": f"{pz['chr']}:{pz['pos']}", "m": ">>repeat"}, timeout=15).json()
            types = {(t.get("Attributes") or {}).get("Repeat", {}).get("repeat_type") for r in (m.get("results") or []) for t in r.get("targets", [])}
            if pz["expect_type"] not in types:
                return False, f"expected type {pz['expect_type']} not in {types}"
            return True, f"repeat types: {sorted(x for x in types if x)}"
        return False, "no positions"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'), script_dir / "reference_data.json")
    c = RepeatTests(runner)
    for m in [c.test_interval_query, c.test_repeat_type]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
