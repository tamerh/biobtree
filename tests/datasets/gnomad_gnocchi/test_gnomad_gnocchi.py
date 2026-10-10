#!/usr/bin/env python3
"""gnomAD non-coding constraint (Gnocchi) test suite."""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner


class GnomadGnocchiTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _windows(self):
        ref = self.runner.reference_data
        return ref.get("windows", []) if isinstance(ref, dict) else []

    def test_window_entry(self):
        """Window resolves with its Z-score preserved"""
        for w in self._windows():
            for r in requests.get(f"{self.api}/ws/", params={"i": w["id"], "s": "gnomad_gnocchi", "d": "0"}, timeout=15).json().get("results", []):
                if r.get("dataset_name") == "gnomad_gnocchi":
                    a = r["Attributes"].get("GnomadGnocchi", {})
                    if a.get("z") != w["z"]:
                        return False, f"{w['id']} z {a.get('z')!r} (expected {w['z']!r})"
                    return True, f"{w['id']} z={a.get('z')}"
        return False, "no gnocchi window resolved"

    def test_interval_query(self):
        """A variant position resolves to its covering constraint window"""
        for w in self._windows():
            m = requests.get(f"{self.api}/ws/map/", params={"i": f"{w['chr']}:{w['pos']}", "m": ">>gnomad_gnocchi"}, timeout=15).json()
            hits = [t.get("identifier", "").upper() for r in (m.get("results") or []) for t in r.get("targets", [])]
            if w["id"].upper() not in hits:
                return False, f"{w['chr']}:{w['pos']} did not cover {w['id']} (got {hits[:5]})"
        return True, f"interval query resolved {len(self._windows())} position(s)"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'), script_dir / "reference_data.json")
    c = GnomadGnocchiTests(runner)
    for m in [c.test_window_entry, c.test_interval_query]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
