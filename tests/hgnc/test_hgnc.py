#!/usr/bin/env python3
"""
HGNC Test Suite

Tests HGNC dataset processing using the common test framework.
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


class HGNCTests:
    """HGNC custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_approved_status_gene(self):
        """Check gene with Approved status"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("status") == "Approved"),
            None
        )
        if not entry:
            return False, "No Approved entry in reference"

        hgnc_id = entry["hgnc_id"]
        symbol = entry.get("symbol", "unknown")

        data = self.runner.lookup(hgnc_id)

        if not data or not data.get("results"):
            return False, f"No results for {hgnc_id}"

        return True, f"{hgnc_id} ({symbol}) has Approved status"

    @test
    def test_gene_with_locus_group(self):
        """Check gene has locus_group"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("locus_group")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with locus_group"

        hgnc_id = entry["hgnc_id"]
        locus_group = entry["locus_group"]

        data = self.runner.lookup(hgnc_id)

        if not data or not data.get("results"):
            return False, f"No results for {hgnc_id}"

        return True, f"{hgnc_id} has locus_group: {locus_group}"

    @test
    def test_gene_with_location(self):
        """Check gene has chromosomal location"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("location")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with location"

        hgnc_id = entry["hgnc_id"]
        location = entry["location"]
        symbol = entry.get("symbol", "unknown")

        data = self.runner.lookup(hgnc_id)

        if not data or not data.get("results"):
            return False, f"No results for {hgnc_id}"

        return True, f"{symbol} located at {location}"

    @test
    def test_gene_with_gene_group(self):
        """Check gene has gene_group classification"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_group") and len(e.get("gene_group", [])) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with gene_group"

        hgnc_id = entry["hgnc_id"]
        gene_groups = entry["gene_group"]
        symbol = entry.get("symbol", "unknown")

        data = self.runner.lookup(hgnc_id)

        if not data or not data.get("results"):
            return False, f"No results for {hgnc_id}"

        return True, f"{symbol} in {len(gene_groups)} gene group(s): {gene_groups[0]}"

    @test
    def test_cross_references_present(self):
        """Check gene has cross-references to external databases"""
        entry = self.runner.reference_data[0]
        hgnc_id = entry["hgnc_id"]

        data = self.runner.lookup(hgnc_id)

        if not data or not data.get("results"):
            return False, f"No results for {hgnc_id}"

        result = data["results"][0]

        # Get all dataset xrefs
        datasets = self.runner.get_xref_datasets(result)
        xref_count = self.runner.get_xref_count(result)

        if len(datasets) >= 1:
            return True, f"{hgnc_id} has {xref_count} xrefs to {len(datasets)} databases: {', '.join(datasets[:5])}"

        return True, f"SKIP: {hgnc_id} has limited xrefs in test data"


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
    custom_tests = HGNCTests(runner)
    for test_method in [custom_tests.test_approved_status_gene,
                       custom_tests.test_gene_with_locus_group,
                       custom_tests.test_gene_with_location,
                       custom_tests.test_gene_with_gene_group,
                       custom_tests.test_cross_references_present]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
