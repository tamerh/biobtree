#!/usr/bin/env python3
"""
EnsemblPlants Gene Test Suite

Tests ensembl_plants dataset processing using the common test framework.
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


class EnsemblPlantsTests:
    """Ensembl custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_gene_with_name(self):
        """Check gene has display_name"""
        # Find a gene with display_name
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("display_name")),
            None
        )
        if not entry:
            return False, "No gene with display_name in reference"

        gene_id = entry["id"]
        display_name = entry["display_name"][:60]

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id} has name: {display_name}..."

    @test
    def test_gene_with_biotype(self):
        """Check gene has biotype"""
        # Find a gene with biotype
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("biotype")),
            None
        )
        if not entry:
            return False, "No gene with biotype in reference"

        gene_id = entry["id"]
        biotype = entry["biotype"]

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id} has biotype: {biotype}"

    @test
    def test_gene_with_strand(self):
        """Check gene has strand information"""
        # Find a gene with strand
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("strand")),
            None
        )
        if not entry:
            return False, "No gene with strand in reference"

        gene_id = entry["id"]
        strand = entry["strand"]

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id} has strand: {strand}"


def main():
    """Main test execution"""
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
    custom_tests = EnsemblPlantsTests(runner)
    for test_method in [
        custom_tests.test_gene_with_name,
        custom_tests.test_gene_with_biotype,
        custom_tests.test_gene_with_strand
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
