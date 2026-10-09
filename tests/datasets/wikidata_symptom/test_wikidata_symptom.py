#!/usr/bin/env python3
"""
Wikidata Symptom (P780) Test Suite.

Validates biobtree's common-disease symptom layer: Wikidata P780 ("symptoms and
signs") statements gated to Mondo (P5270) / DOID (P699)-mapped diseases. Each
symptom term is a wikidata_symptom entry (OntologyAttr, type="symptom", keyed by
Wikidata QID); each statement adds a disease->symptom edge onto mondo/doid.

Data under test is the committed fixture
(tests/datasets/wikidata_symptom/wikidata_symptoms_fixture.tsv) - a small real
subset (migraine / asthma / type 2 diabetes), NOT a full Wikidata extract.

Unit mode builds only wikidata_symptom, so the mondo/doid disease nodes are not
present; these tests therefore check the symptom entries and the edges written
FROM each symptom to its disease. The disease->symptom direction (and the CEL
filter) are covered by the cross-dataset integration tests against the full DB.
"""

import sys
import os
import requests
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner


class WikidataSymptomTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner
        self.api = runner.api_url

    def _symptoms(self):
        ref = self.runner.reference_data
        return ref.get("symptoms", []) if isinstance(ref, dict) else []

    def _get(self, params):
        return requests.get(f"{self.api}/ws/", params=params, timeout=15).json()

    def _entry(self, qid):
        for r in self._get({"i": qid, "d": "1"}).get("results", []):
            if r.get("dataset_name") == "wikidata_symptom" and r.get("identifier") == qid:
                return r
        return None

    def test_symptom_entry_exists(self):
        """Every fixture symptom QID resolves to a wikidata_symptom entry"""
        missing = [s["id"] for s in self._symptoms() if self._entry(s["id"]) is None]
        if missing:
            return False, f"symptom QIDs not found: {missing}"
        return True, f"all {len(self._symptoms())} symptom entries resolved"

    def test_symptom_attr_type_and_name(self):
        """Entries carry OntologyAttr with type='symptom' and the fixture label"""
        for s in self._symptoms():
            e = self._entry(s["id"])
            if e is None:
                return False, f"{s['id']} not found"
            a = (e.get("Attributes") or {}).get("Ontology") or {}
            if a.get("type") != "symptom":
                return False, f"{s['id']} type={a.get('type')} (expected 'symptom')"
            if a.get("name") != s["name"]:
                return False, f"{s['id']} name={a.get('name')!r} (expected {s['name']!r})"
        return True, f"type + name correct for {len(self._symptoms())} symptoms"

    def test_symptom_to_disease_edges(self):
        """Each symptom links back to its mondo and/or doid disease node"""
        for s in self._symptoms():
            e = self._entry(s["id"])
            targets = {t.get("identifier") for t in (e.get("entries") or [])}
            for key in ("mondo", "doid"):
                want = s.get(key)
                if want and want not in targets:
                    return False, f"{s['id']} missing {key} edge {want} (have {sorted(targets)})"
        return True, f"disease edges present for {len(self._symptoms())} symptoms"

    def test_disease_wikidata_link(self):
        """Each disease's own Wikidata item links to its mondo/doid term (wikidata namespace)"""
        ref = self.runner.reference_data
        diseases = ref.get("disease_wikidata", []) if isinstance(ref, dict) else []
        for d in diseases:
            qid = d["disease_qid"]
            # the wikidata disease item resolves and links to the disease term(s)
            targets = set()
            for r in self._get({"i": qid, "d": "1"}).get("results", []):
                if r.get("dataset_name") == "wikidata" and r.get("identifier") == qid:
                    targets = {t.get("identifier") for t in (r.get("entries") or [])}
            for key in ("mondo", "doid"):
                want = d.get(key)
                if want and want not in targets:
                    return False, f"{qid} missing {key} link {want} (have {sorted(targets)})"
        return True, f"disease->wikidata links present for {len(diseases)} diseases"

    def test_text_search_by_symptom_name(self):
        """A symptom is findable by its label via text search"""
        for s in self._symptoms():
            ids = [r.get("identifier") for r in self._get({"i": s["name"], "d": "0"}).get("results", [])]
            if s["id"] not in ids:
                return False, f"'{s['name']}' did not return {s['id']} (got {ids[:5]})"
        return True, f"text search resolved all {len(self._symptoms())} symptom names"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file)
    custom = WikidataSymptomTests(runner)
    for m in [
        custom.test_symptom_entry_exists,
        custom.test_symptom_attr_type_and_name,
        custom.test_symptom_to_disease_edges,
        custom.test_disease_wikidata_link,
        custom.test_text_search_by_symptom_name,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
