#!/usr/bin/env python3
"""
Wikidata disease->QID test suite.

The `wikidata` dataset links every biobtree disease node (Mondo / Disease
Ontology) to its Wikidata item (QID), from the FULL P5270/P699 mapping —
independent of symptoms (the old symptom-gated coverage was ~15%; Atlas
reported Bardet-Biedl DOID:1935 resolved to nothing). So mondo/doid >> wikidata
should resolve for any disease with a Mondo or DOID mapping on Wikidata.

Data under test is the hand-crafted fixture
(tests/datasets/wikidata/wikidata_diseases_fixture.tsv), NOT live Wikidata.
"""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner, test


def _targets(data, dataset):
    return [t.get("identifier") for r in (data or {}).get("results", [])
            for t in r.get("targets", []) if t.get("dataset_name") == dataset]


class WikidataTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _map(self, i, m):
        return requests.get(f"{self.api}/ws/map/", params={"i": i, "m": m}, timeout=15).json()

    @test
    def test_mondo_to_qid(self):
        """A Mondo disease resolves to its Wikidata QID"""
        q = _targets(self._map("MONDO:0004975", ">>mondo>>wikidata"), "wikidata")
        if "Q11081" not in q:
            return False, f"MONDO:0004975 >> wikidata = {q} (expected Q11081)"
        return True, "MONDO:0004975 -> Q11081"

    @test
    def test_doid_to_qid_atlas_case(self):
        """DOID:1935 (Bardet-Biedl — the Atlas gap case) now resolves to a QID"""
        q = _targets(self._map("DOID:1935", ">>doid>>wikidata"), "wikidata")
        if "Q166228" not in q:
            return False, f"DOID:1935 >> wikidata = {q} (expected Q166228)"
        return True, "DOID:1935 -> Q166228 (previously nothing)"

    @test
    def test_coverage_independent_of_symptoms(self):
        """A disease with NO symptom edge still gets a QID (full mapping)"""
        # Q124407 is linked by Mondo+DOID but has no wikidata_symptom edge in any fixture.
        q = _targets(self._map("MONDO:0005148", ">>mondo>>wikidata"), "wikidata")
        if "Q124407" not in q:
            return False, f"MONDO:0005148 >> wikidata = {q} (expected Q124407)"
        return True, "symptom-independent disease -> Q124407"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'),
                        script_dir / "reference_data.json")
    c = WikidataTests(runner)
    for m in [c.test_mondo_to_qid, c.test_doid_to_qid_atlas_case, c.test_coverage_independent_of_symptoms]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
