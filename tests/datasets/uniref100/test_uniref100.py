#!/usr/bin/env python3
"""
UniRef100 Test Suite

Tests UniRef100 dataset processing using the common test framework.
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


class UniRef100Tests:
    """UniRef100 custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_entry_has_name(self):
        """Check UniRef100 entry has cluster name"""
        entry = self.runner.reference_data[0] if self.runner.reference_data else None

        if not entry:
            return False, "No entries in reference data"

        uniref_id = entry["id"]
        name = entry.get("name", "")

        data = self.runner.lookup(uniref_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniref_id}"

        return True, f"{uniref_id} has name: {name[:50]}..."

    @test
    def test_representative_member_present(self):
        """Check entry has representative member with organism info"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("representativeMember", {}).get("organismName")),
            None
        )
        if not entry:
            return False, "No entry with representative member in reference"

        uniref_id = entry["id"]
        organism = entry["representativeMember"]["organismName"]

        data = self.runner.lookup(uniref_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniref_id}"

        return True, f"{uniref_id} has representative member from: {organism[:50]}..."

    @test
    def test_seed_protein_present(self):
        """Check entry has seed protein ID"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("seedId")),
            None
        )
        if not entry:
            return False, "No entry with seed protein in reference"

        uniref_id = entry["id"]
        seed_id = entry["seedId"]

        data = self.runner.lookup(uniref_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniref_id}"

        return True, f"{uniref_id} has seed protein: {seed_id}"


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
    custom_tests = UniRef100Tests(runner)
    for test_method in [custom_tests.test_entry_has_name,
                       custom_tests.test_representative_member_present,
                       custom_tests.test_seed_protein_present]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
