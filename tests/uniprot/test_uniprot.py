#!/usr/bin/env python3
"""
UniProt Test Suite

Tests UniProt dataset processing using the common test framework.
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


class UniProtTests:
    """UniProt custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_reviewed_entry(self):
        """Check reviewed (Swiss-Prot) entry"""
        # Find a reviewed entry
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("entryType") == "UniProtKB reviewed (Swiss-Prot)"),
            None
        )
        if not entry:
            return False, "No reviewed entry in reference"

        uniprot_id = entry["primaryAccession"]
        organism = entry.get("organism", {}).get("scientificName", "unknown")

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        return True, f"{uniprot_id} ({organism}) is reviewed"

    @test
    def test_protein_name_present(self):
        """Check protein has recommended name"""
        # Find entry with protein description
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("proteinDescription", {}).get("recommendedName")),
            None
        )
        if not entry:
            return False, "No entry with protein name in reference"

        uniprot_id = entry["primaryAccession"]
        protein_name = entry["proteinDescription"]["recommendedName"]["fullName"]["value"]

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        return True, f"{uniprot_id} has protein name: {protein_name[:50]}..."


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
    custom_tests = UniProtTests(runner)
    for test_method in [custom_tests.test_reviewed_entry,
                       custom_tests.test_protein_name_present]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
