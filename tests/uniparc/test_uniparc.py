#!/usr/bin/env python3
"""
UniParc Test Suite

Tests UniParc dataset processing using the common test framework.
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


class UniParcTests:
    """UniParc custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_entry_with_many_xrefs(self):
        """Check UniParc entry has cross-references"""
        # Find an entry with many cross-references
        entry = next(
            (e for e in self.runner.reference_data
             if len(e.get("uniParcCrossReferences", [])) > 10),
            None
        )
        if not entry:
            return False, "No entry with cross-references in reference"

        upi_id = entry["uniParcId"]
        xref_count = len(entry.get("uniParcCrossReferences", []))

        data = self.runner.lookup(upi_id)

        if not data or not data.get("results"):
            return False, f"No results for {upi_id}"

        return True, f"{upi_id} has {xref_count} cross-references"

    @test
    def test_entry_with_sequence(self):
        """Check UniParc entry has sequence information"""
        # UniParc entries should have sequence data
        entry = self.runner.reference_data[0] if self.runner.reference_data else None

        if not entry:
            return False, "No entries in reference data"

        upi_id = entry["uniParcId"]

        # Check if sequence exists in reference
        has_sequence = "sequence" in entry and entry["sequence"].get("length")

        if not has_sequence:
            return False, f"{upi_id} has no sequence in reference data"

        data = self.runner.lookup(upi_id)

        if not data or not data.get("results"):
            return False, f"No results for {upi_id}"

        seq_length = entry["sequence"]["length"]
        return True, f"{upi_id} has sequence (length: {seq_length})"


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
    custom_tests = UniParcTests(runner)
    for test_method in [custom_tests.test_entry_with_many_xrefs,
                       custom_tests.test_entry_with_sequence]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
