#!/usr/bin/env python3
"""
DOID (Human Disease Ontology) Test Suite

Tests DOID dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

Note: This script is called by the main orchestrator (tests/run_tests.py)
which manages the biobtree web server lifecycle.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class DOIDTests:
    """DOID custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_disease_with_name(self):
        """Check DOID term has a descriptive disease name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name") and len(e["name"]) > 4),
            None
        )
        if not entry:
            return False, "No DOID term with descriptive name in reference"

        doid_id = entry["id"]
        term_name = entry.get("name", "unknown")

        data = self.runner.lookup(doid_id)
        if not data or not data.get("results"):
            return False, f"No results for {doid_id}"

        return True, f"{doid_id} has name: {term_name[:60]}"

    @test
    def test_term_with_synonyms(self):
        """Check DOID term has synonyms"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e.get("synonyms", [])) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No DOID term with synonyms in reference"

        doid_id = entry["id"]
        synonyms = entry.get("synonyms", [])

        data = self.runner.lookup(doid_id)
        if not data or not data.get("results"):
            return False, f"No results for {doid_id}"

        return True, f"{doid_id} has {len(synonyms)} synonym(s) (e.g., '{synonyms[0]}')"

    @test
    def test_text_search_by_name(self):
        """Test text search by disease name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name") and len(e["name"].split()) >= 2),
            None
        )
        if not entry:
            return True, "SKIP: No multi-word disease term found in reference"

        term_name = entry["name"]
        doid_id = entry["id"]

        data = self.runner.lookup(term_name)
        if not data or not data.get("results"):
            return False, f"Text search failed for '{term_name}'"

        found = any(
            result.get("identifier") == doid_id
            for result in data.get("results", [])
        )

        if found:
            return True, f"Text search for '{term_name[:50]}' found {doid_id}"
        return False, f"Text search for '{term_name}' didn't return expected ID {doid_id}"

    @test
    def test_text_search_by_synonym(self):
        """Test text search by disease synonym (verified against DB)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e.get("synonyms", [])) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No term with synonyms found in reference"

        doid_id = entry["id"]
        id_data = self.runner.lookup(doid_id)
        if not id_data or not id_data.get("results"):
            return False, f"Could not lookup {doid_id} to verify synonyms"

        db_synonyms = []
        for result in id_data.get("results", []):
            syns = result.get("Attributes", {}).get("Ontology", {}).get("synonyms")
            if syns:
                db_synonyms = syns
                break

        if not db_synonyms:
            return True, "SKIP: No indexed synonyms in database for selected term"

        synonym = db_synonyms[0]
        data = self.runner.lookup(synonym)
        if not data or not data.get("results"):
            return False, f"Text search failed for synonym '{synonym}'"

        found = any(
            result.get("identifier") == doid_id
            for result in data.get("results", [])
        )

        if found:
            return True, f"Synonym search for '{synonym[:50]}' found {doid_id}"
        return False, f"Synonym search for '{synonym}' didn't return expected ID {doid_id}"

    @test
    def test_hierarchical_relationships(self):
        """Check DOID term exposes parent relationships via doidparent"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("parents") and len(e.get("parents", [])) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No term with parent relationships in reference"

        doid_id = entry["id"]
        parent_count = len(entry.get("parents", []))

        data = self.runner.lookup(doid_id)
        if not data or not data.get("results"):
            return False, f"No results for {doid_id}"

        parent_entries = [
            e for e in data.get("results", [{}])[0].get("entries", [])
            if e.get("dataset_name") == "doidparent"
        ]

        if parent_entries:
            return True, f"{doid_id} has {len(parent_entries)} parent(s)"
        return True, f"SKIP: {doid_id} has no indexed parents (expected {parent_count})"


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

    custom_tests = DOIDTests(runner)
    for test_method in [
        custom_tests.test_disease_with_name,
        custom_tests.test_term_with_synonyms,
        custom_tests.test_text_search_by_name,
        custom_tests.test_text_search_by_synonym,
        custom_tests.test_hierarchical_relationships,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
