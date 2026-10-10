#!/usr/bin/env python3
"""gnomAD regional missense constraint (gnomad_rmc) test suite."""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner


class GnomadRmcTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _regions(self):
        ref = self.runner.reference_data
        return ref.get("regions", []) if isinstance(ref, dict) else []

    def _get(self, params):
        return requests.get(f"{self.api}/ws/", params=params, timeout=15).json()

    def test_region_entry(self):
        """Region resolves with transcript + preserved oe (incl. oe=0)"""
        for rg in self._regions():
            for r in self._get({"i": rg["id"], "s": "gnomad_rmc", "d": "0"}).get("results", []):
                if r.get("dataset_name") == "gnomad_rmc":
                    a = r["Attributes"].get("GnomadRmc", {})
                    if a.get("transcript") != rg["transcript"]:
                        return False, f"{rg['id']} transcript {a.get('transcript')}"
                    if a.get("oe") != rg["oe"]:
                        return False, f"{rg['id']} oe {a.get('oe')!r} (expected {rg['oe']!r} preserved)"
                    return True, f"{rg['id']} oe={a.get('oe')} constrained={a.get('constrained')}"
        return False, "no gnomad_rmc region resolved"

    def test_interval_query(self):
        """A variant position resolves to the covering constrained region"""
        for rg in self._regions():
            m = requests.get(f"{self.api}/ws/map/", params={"i": f"{rg['chr']}:{rg['pos']}", "m": ">>gnomad_rmc"}, timeout=15).json()
            hits = [t.get("identifier") for r in (m.get("results") or []) for t in r.get("targets", [])]
            if rg["id"] not in hits:
                return False, f"{rg['chr']}:{rg['pos']} did not cover {rg['id']} (got {hits[:5]})"
        return True, f"interval query resolved {len(self._regions())} position(s)"

    def test_transcript_edge(self):
        """Region links to its Ensembl transcript"""
        for rg in self._regions():
            for r in self._get({"i": rg["id"], "s": "gnomad_rmc", "d": "1"}).get("results", []):
                if r.get("dataset_name") == "gnomad_rmc":
                    tx = {e.get("identifier") for e in r.get("entries", []) if e.get("dataset_name") == "transcript"}
                    if rg["transcript"] not in tx:
                        return False, f"{rg['id']} missing transcript edge {rg['transcript']}"
                    return True, "transcript edge present"
        return False, "no region resolved"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'), script_dir / "reference_data.json")
    c = GnomadRmcTests(runner)
    for m in [c.test_region_entry, c.test_interval_query, c.test_transcript_edge]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
