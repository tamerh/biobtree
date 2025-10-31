#!/usr/bin/env python3
"""
ECO Test Suite

Tests ECO (Evidence & Conclusion Ontology) dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

Note: This script is called by the main orchestrator (tests/run_tests.py)
which manages the biobtree web server lifecycle.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent))

from common import TestRunner, test

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class ECOTests:
    """ECO custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_evidence_term_with_name(self):
        """Check ECO term has evidence name"""
        # Find a term with a descriptive name (not just "evidence")
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name") and e["name"] != "evidence"),
            None
        )
        if not entry:
            return False, "No ECO term with descriptive name in reference"

        eco_id = entry["id"]
        term_name = entry.get("name", "unknown")

        data = self.runner.lookup(eco_id)

        if not data or not data.get("results"):
            return False, f"No results for {eco_id}"

        return True, f"{eco_id} has evidence name: {term_name[:60]}..."

    @test
    def test_term_with_synonyms(self):
        """Check ECO term has synonyms"""
        # Find a term with synonyms
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e.get("synonyms", [])) > 0),
            None
        )
        if not entry:
            return False, "No ECO term with synonyms in reference"

        eco_id = entry["id"]
        synonym_count = len(entry.get("synonyms", []))
        synonyms = entry.get("synonyms", [])

        data = self.runner.lookup(eco_id)

        if not data or not data.get("results"):
            return False, f"No results for {eco_id}"

        # Show first synonym as example
        first_synonym = synonyms[0] if synonyms else ""
        return True, f"{eco_id} has {synonym_count} synonym(s) (e.g., '{first_synonym}')"


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
    custom_tests = ECOTests(runner)
    for test_method in [custom_tests.test_evidence_term_with_name,
                       custom_tests.test_term_with_synonyms]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
