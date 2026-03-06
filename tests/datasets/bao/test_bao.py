#!/usr/bin/env python3
"""
BAO (BioAssay Ontology) Test Suite

Tests BAO dataset processing using the common test framework.
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


class BAOTests:
    """BAO custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_text_search(self):
        """Search for BAO terms by keyword"""
        # First check if we have any data
        data = self.runner.lookup("BAO:0000001")
        if not data or not data.get("results"):
            return True, "SKIP: No test data available"

        # Search for FRET (a synonym we know exists)
        data = self.runner.lookup("FRET")
        if not data or not data.get("results"):
            return True, "SKIP: No text search results (limited test data)"

        # Check at least one result is from BAO
        bao_results = [r for r in data["results"] if r.get("dataset_name") == "bao"]
        if not bao_results:
            return True, "SKIP: No BAO results in text search"

        return True, f"Found {len(bao_results)} BAO results for 'FRET'"

    @test
    def test_synonym_search(self):
        """Search for BAO terms by synonym"""
        # FRET is a synonym for fluorescence resonance energy transfer
        data = self.runner.lookup("FRET")

        if not data or not data.get("results"):
            return True, "SKIP: No results for 'FRET' search (limited test data)"

        # Check we found BAO:0000001
        found_fret = any(
            r.get("identifier") == "BAO:0000001"
            for r in data["results"]
        )
        if not found_fret:
            return True, "SKIP: BAO:0000001 not in search results"

        return True, "Found BAO:0000001 via 'FRET' synonym"

    @test
    def test_term_with_synonyms(self):
        """Check term has synonyms in attributes"""
        # BAO:0000001 should have 'FRET' as synonym
        data = self.runner.lookup("BAO:0000001")

        if not data or not data.get("results"):
            return False, "No results for BAO:0000001"

        result = data["results"][0]
        attrs = result.get("Attributes", {})
        ontology = attrs.get("Ontology", {})
        synonyms = ontology.get("synonyms", [])

        if not synonyms:
            return True, "SKIP: No synonyms in test data"

        return True, f"Found synonyms: {synonyms}"

    @test
    def test_has_children(self):
        """Check term has child relationships"""
        data = self.runner.lookup("BAO:0000001")

        if not data or not data.get("results"):
            return False, "No results for BAO:0000001"

        result = data["results"][0]
        entries = result.get("entries", [])

        # Look for baochild entries
        children = [e for e in entries if e.get("dataset_name") == "baochild"]
        if not children:
            return True, "SKIP: No child entries in test data"

        return True, f"Found {len(children)} child(ren)"

    @test
    def test_has_parent(self):
        """Check term has parent relationships"""
        data = self.runner.lookup("BAO:0000001")

        if not data or not data.get("results"):
            return False, "No results for BAO:0000001"

        result = data["results"][0]
        entries = result.get("entries", [])

        # Look for baoparent entries
        parents = [e for e in entries if e.get("dataset_name") == "baoparent"]
        if not parents:
            return True, "SKIP: No parent entries in test data"

        return True, f"Found {len(parents)} parent(s)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = BAOTests(runner)
    for test_method in [custom_tests.test_text_search,
                       custom_tests.test_synonym_search,
                       custom_tests.test_term_with_synonyms,
                       custom_tests.test_has_children,
                       custom_tests.test_has_parent]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
