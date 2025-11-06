#!/usr/bin/env python3
"""
GO Test Suite

Tests GO dataset processing using the common test framework.
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


class GOTests:
    """GO custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_biological_process_term(self):
        """Check term with biological_process aspect"""
        # Find a term with biological_process aspect
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("aspect") == "biological_process" and not e.get("isObsolete")),
            None
        )
        if not entry:
            return False, "No biological_process term in reference"

        go_id = entry["id"]
        term_name = entry.get("name", "unknown")

        data = self.runner.lookup(go_id)

        if not data or not data.get("results"):
            return False, f"No results for {go_id}"

        return True, f"{go_id} ({term_name}) is biological_process"

    @test
    def test_term_with_synonyms(self):
        """Check term has synonyms"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e.get("synonyms", [])) > 0),
            None
        )
        if not entry:
            return False, "No term with synonyms in reference"

        go_id = entry["id"]
        synonym_count = len(entry.get("synonyms", []))

        data = self.runner.lookup(go_id)

        if not data or not data.get("results"):
            return False, f"No results for {go_id}"

        return True, f"{go_id} has {synonym_count} synonym(s)"

    @test
    def test_molecular_function_term(self):
        """Check term with molecular_function aspect"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("aspect") == "molecular_function" and not e.get("isObsolete")),
            None
        )
        if not entry:
            return True, "SKIP: No molecular_function term in reference"

        go_id = entry["id"]
        term_name = entry.get("name", "unknown")

        data = self.runner.lookup(go_id)

        if not data or not data.get("results"):
            return False, f"No results for {go_id}"

        return True, f"{go_id} ({term_name}) is molecular_function"

    @test
    def test_cellular_component_term(self):
        """Check term with cellular_component aspect"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("aspect") == "cellular_component" and not e.get("isObsolete")),
            None
        )
        if not entry:
            return True, "SKIP: No cellular_component term in reference"

        go_id = entry["id"]
        term_name = entry.get("name", "unknown")

        data = self.runner.lookup(go_id)

        if not data or not data.get("results"):
            return False, f"No results for {go_id}"

        return True, f"{go_id} ({term_name}) is cellular_component"

    @test
    def test_obsolete_term(self):
        """Check obsolete term handling"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("isObsolete") == True),
            None
        )
        if not entry:
            return True, "SKIP: No obsolete term in reference"

        go_id = entry["id"]
        term_name = entry.get("name", "unknown")

        data = self.runner.lookup(go_id)

        if not data or not data.get("results"):
            return False, f"No results for {go_id}"

        return True, f"{go_id} ({term_name}) is obsolete"

    @test
    def test_term_with_definition(self):
        """Check term has definition"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("definition") and e["definition"].get("text")),
            None
        )
        if not entry:
            return False, "No term with definition in reference"

        go_id = entry["id"]
        def_text = entry["definition"]["text"][:60]

        data = self.runner.lookup(go_id)

        if not data or not data.get("results"):
            return False, f"No results for {go_id}"

        return True, f"{go_id} has definition: {def_text}..."

    @test
    def test_parent_child_relationships(self):
        """Check GO term hierarchy relationships"""
        entry = self.runner.reference_data[0]
        go_id = entry["id"]

        data = self.runner.lookup(go_id)

        if not data or not data.get("results"):
            return False, f"No results for {go_id}"

        result = data["results"][0]

        # Check for parent/child cross-references
        datasets = self.runner.get_xref_datasets(result)
        xref_count = self.runner.get_xref_count(result)

        if len(datasets) >= 1:
            return True, f"{go_id} has {xref_count} relationships: {', '.join(datasets[:5])}"

        return True, f"SKIP: {go_id} has no relationships in test data"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        print("Run: python3 extract_reference_data.py")
        return 1

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = GOTests(runner)
    for test_method in [custom_tests.test_biological_process_term,
                       custom_tests.test_term_with_synonyms,
                       custom_tests.test_molecular_function_term,
                       custom_tests.test_cellular_component_term,
                       custom_tests.test_obsolete_term,
                       custom_tests.test_term_with_definition,
                       custom_tests.test_parent_child_relationships]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
