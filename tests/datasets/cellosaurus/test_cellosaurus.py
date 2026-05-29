#!/usr/bin/env python3
"""
Cellosaurus Test Suite

Validates the cell-line entity: entry lookup, name text search, species
(taxonomy) edges, and disease (orphanet/mondo) edges. Cellosaurus adds cell
lines as a connected hub linking to genes, diseases, species and literature.
"""

import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class CellosaurusTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _cells(self):
        ref = self.runner.reference_data
        return ref.get("cells", []) if isinstance(ref, dict) else []

    def _entry(self, accession):
        data = self.runner.lookup(accession)
        if not data:
            return None
        for r in data.get("results", []):
            if r.get("dataset_name") == "cellosaurus":
                return r
        return None

    @test
    def test_entry_exists(self):
        """A cell line is retrievable by CVCL_ accession"""
        cells = self._cells()
        if not cells:
            return False, "No cells in reference data"
        ac = cells[0]["accession"]
        if self._entry(ac) is None:
            return False, f"No cellosaurus entry for {ac}"
        return True, f"Cell line {cells[0]['name']} ({ac}) found"

    @test
    def test_name_text_search(self):
        """Cell line name resolves via text search"""
        cells = self._cells()
        name = next((c["name"] for c in cells if len(c.get("name", "")) >= 3), None)
        if not name:
            return False, "No suitable cell line name in reference"
        data = self.runner.lookup(name)
        if not data or not data.get("results"):
            return False, f"No results for name {name}"
        return True, f"Text search found '{name}'"

    @test
    def test_maps_to_taxonomy(self):
        """Cell line maps to its species (taxonomy) — present on every entry"""
        cells = self._cells()
        if not cells:
            return False, "No cells in reference data"
        ac = cells[0]["accession"]
        e = self._entry(ac)
        if e is None or not self.runner.has_xref(e, "taxonomy"):
            return False, f"{ac} has no taxonomy xref"
        return True, f"{ac} -> taxonomy OK"

    @test
    def test_maps_to_disease(self):
        """Cell line maps to a disease (orphanet and/or mondo)"""
        cells = self._cells()
        if not cells:
            return False, "No cells in reference data"
        # Try a few — disease mapping depends on ORDO/MONDO resolution
        for c in cells[:10]:
            e = self._entry(c["accession"])
            if e and (self.runner.has_xref(e, "orphanet") or self.runner.has_xref(e, "mondo")):
                return True, f"{c['accession']} -> disease OK"
        return False, "No sampled cell line mapped to orphanet/mondo"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        print("Run: python3 extract_reference_data.py")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = CellosaurusTests(runner)

    for m in [
        custom.test_entry_exists,
        custom.test_name_text_search,
        custom.test_maps_to_taxonomy,
        custom.test_maps_to_disease,
    ]:
        runner.add_custom_test(m)

    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
