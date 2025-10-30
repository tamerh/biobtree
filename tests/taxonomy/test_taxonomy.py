#!/usr/bin/env python3
"""
Taxonomy Test Suite

Tests Taxonomy dataset processing using the common test framework.
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


class TaxonomyTests:
    """Taxonomy custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_entry_with_common_name(self):
        """Check taxonomy entry has common name"""
        # Find an entry with common name
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("commonName")),
            None
        )
        if not entry:
            return False, "No entry with common name in reference"

        tax_id = str(entry["taxonId"])
        sci_name = entry.get("scientificName", "unknown")
        common_name = entry.get("commonName", "unknown")

        data = self.runner.lookup(tax_id)

        if not data or not data.get("results"):
            return False, f"No results for {tax_id}"

        return True, f"{tax_id} ({sci_name}) has common name: {common_name}"

    @test
    def test_domain_rank(self):
        """Check taxonomy entry with domain rank"""
        # Find an entry with domain rank
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("rank") == "domain"),
            None
        )
        if not entry:
            return False, "No domain rank entry in reference"

        tax_id = str(entry["taxonId"])
        sci_name = entry.get("scientificName", "unknown")

        data = self.runner.lookup(tax_id)

        if not data or not data.get("results"):
            return False, f"No results for {tax_id}"

        return True, f"{tax_id} ({sci_name}) has domain rank"


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
    custom_tests = TaxonomyTests(runner)
    for test_method in [custom_tests.test_entry_with_common_name,
                       custom_tests.test_domain_rank]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
