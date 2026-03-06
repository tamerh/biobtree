#!/usr/bin/env python3
"""
MONDO (Monarch Disease Ontology) Test Suite

Tests disease terms, hierarchies, and cross-references.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class MondoTests:
    """MONDO custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_disease_with_name(self):
        """Check disease has name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name")),
            None
        )
        if not entry:
            return False, "No disease with name in reference"

        disease_id = entry["id"]
        disease_name = entry["name"][:60]
        data = self.runner.lookup(disease_id)

        if not data or not data.get("results"):
            return False, f"No results for {disease_id}"

        return True, f"{disease_id} has name: {disease_name}..."

    @test
    def test_disease_with_synonyms(self):
        """Check disease has synonyms"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e["synonyms"]) > 0),
            None
        )
        if not entry:
            return False, "No disease with synonyms in reference"

        disease_id = entry["id"]
        syn_count = len(entry["synonyms"])
        synonyms = entry.get("synonyms", [])
        data = self.runner.lookup(disease_id)

        if not data or not data.get("results"):
            return False, f"No results for {disease_id}"

        # Show first synonym as example
        first_synonym = synonyms[0].get('text', '')[:40] if synonyms else ""
        return True, f"{disease_id} has {syn_count} synonym(s) (e.g., '{first_synonym}...')"

    @test
    def test_disease_with_parents(self):
        """Check disease hierarchy (parent relationships)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("parents") and len(e["parents"]) > 0),
            None
        )
        if not entry:
            return False, "No disease with parents in reference"

        disease_id = entry["id"]
        parent_count = len(entry["parents"])
        parent_id = entry["parents"][0].get('id', '')

        data = self.runner.lookup(disease_id)
        if not data or not data.get("results"):
            return False, f"No results for {disease_id}"

        return True, f"{disease_id} has {parent_count} parent(s) (e.g., {parent_id})"

    @test
    def test_disease_with_xrefs(self):
        """Check disease has cross-references"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("xrefs") and len(e["xrefs"]) > 0),
            None
        )
        if not entry:
            return False, "No disease with xrefs in reference"

        disease_id = entry["id"]
        xref_count = len(entry["xrefs"])

        data = self.runner.lookup(disease_id)
        if not data or not data.get("results"):
            return False, f"No results for {disease_id}"

        return True, f"{disease_id} has {xref_count} xrefs in reference"

    @test
    def test_text_search_by_disease_name(self):
        """Test text search by disease name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name") and len(e["name"]) > 5),
            None
        )
        if not entry:
            return False, "No disease with suitable name in reference"

        disease_name = entry["name"]
        disease_id = entry["id"]

        # Search by name
        data = self.runner.lookup(disease_name)
        if not data or not data.get("results"):
            return False, f"No results for name: {disease_name}"

        return True, f"Found results for '{disease_name[:40]}...'"

    @test
    def test_text_search_by_synonym(self):
        """Test text search by disease synonym"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e["synonyms"]) > 0
             and len(e["synonyms"][0].get('text', '')) > 5),
            None
        )
        if not entry:
            return False, "No disease with suitable synonym in reference"

        synonym = entry["synonyms"][0].get('text', '')
        disease_id = entry["id"]

        # Search by synonym
        data = self.runner.lookup(synonym)
        if not data or not data.get("results"):
            return False, f"No results for synonym: {synonym}"

        return True, f"Found results for synonym '{synonym[:40]}...'"


def main():
    """Main test execution"""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        print("Run: python3 extract_reference_data.py")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom_tests = MondoTests(runner)

    for test_method in [
        custom_tests.test_disease_with_name,
        custom_tests.test_disease_with_synonyms,
        custom_tests.test_disease_with_parents,
        custom_tests.test_disease_with_xrefs,
        custom_tests.test_text_search_by_disease_name,
        custom_tests.test_text_search_by_synonym,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
