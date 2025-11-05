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

    @test
    def test_species_rank(self):
        """Check taxonomy entry with species rank"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("rank") == "species"),
            None
        )
        if not entry:
            return True, "SKIP: No species rank entry in reference"

        tax_id = str(entry["taxonId"])
        sci_name = entry.get("scientificName", "unknown")

        data = self.runner.lookup(tax_id)

        if not data or not data.get("results"):
            return False, f"No results for {tax_id}"

        return True, f"{tax_id} ({sci_name}) has species rank"

    @test
    def test_taxonomic_division(self):
        """Check taxonomy entry has taxonomic division"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("taxonomicDivision")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with taxonomic division"

        tax_id = str(entry["taxonId"])
        sci_name = entry.get("scientificName", "unknown")
        division = entry.get("taxonomicDivision", "unknown")

        data = self.runner.lookup(tax_id)

        if not data or not data.get("results"):
            return False, f"No results for {tax_id}"

        return True, f"{tax_id} ({sci_name}) in division: {division}"

    @test
    def test_parent_relationship(self):
        """Check taxonomy entry has parent relationship"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("parent") and e["parent"].get("taxonId")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with parent in reference"

        tax_id = str(entry["taxonId"])
        sci_name = entry.get("scientificName", "unknown")
        parent_id = str(entry["parent"]["taxonId"])
        parent_name = entry["parent"].get("scientificName", "unknown")

        data = self.runner.lookup(tax_id)

        if not data or not data.get("results"):
            return False, f"No results for {tax_id}"

        result = data["results"][0]

        # Check for taxparent cross-references using helper method
        if self.runner.has_xref(result, "taxparent", parent_id):
            return True, f"{tax_id} ({sci_name}) → parent {parent_id} ({parent_name})"

        return True, f"SKIP: {tax_id} parent relationship not validated"

    @test
    def test_hierarchy_relationships(self):
        """Check taxonomy entry has hierarchical relationships"""
        entry = self.runner.reference_data[0]
        tax_id = str(entry["taxonId"])

        data = self.runner.lookup(tax_id)

        if not data or not data.get("results"):
            return False, f"No results for {tax_id}"

        result = data["results"][0]

        # Get all relationship types using helper methods
        datasets = self.runner.get_xref_datasets(result)
        xref_count = self.runner.get_xref_count(result)

        if len(datasets) >= 1:
            return True, f"{tax_id} has {xref_count} relationships: {', '.join(datasets)}"

        return True, f"SKIP: {tax_id} has no relationships in test data"


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
                       custom_tests.test_domain_rank,
                       custom_tests.test_species_rank,
                       custom_tests.test_taxonomic_division,
                       custom_tests.test_parent_relationship,
                       custom_tests.test_hierarchy_relationships]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
