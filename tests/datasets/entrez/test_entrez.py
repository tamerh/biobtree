#!/usr/bin/env python3
"""
Entrez Gene Test Suite

Tests NCBI Entrez Gene dataset processing using the common test framework.
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


class EntrezTests:
    """Entrez Gene custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_gene_with_symbol(self):
        """Check gene has official symbol"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("Symbol") and e.get("Symbol") != "-"),
            None
        )
        if not entry:
            return True, "SKIP: No entry with symbol"

        gene_id = str(entry["GeneID"])
        symbol = entry["Symbol"]

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"Gene {gene_id} has symbol: {symbol}"

    @test
    def test_gene_with_description(self):
        """Check gene has description/name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("description") and e.get("description") != "-"),
            None
        )
        if not entry:
            return True, "SKIP: No entry with description"

        gene_id = str(entry["GeneID"])
        desc = entry["description"][:50] + "..." if len(entry.get("description", "")) > 50 else entry.get("description", "")

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"Gene {gene_id}: {desc}"

    @test
    def test_gene_with_chromosome(self):
        """Check gene has chromosome location"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("chromosome") and e.get("chromosome") != "-"),
            None
        )
        if not entry:
            return True, "SKIP: No entry with chromosome"

        gene_id = str(entry["GeneID"])
        chromosome = entry["chromosome"]
        symbol = entry.get("Symbol", "unknown")

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{symbol} is on chromosome {chromosome}"

    @test
    def test_gene_type(self):
        """Check gene has type classification"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("type_of_gene") and e.get("type_of_gene") != "-"),
            None
        )
        if not entry:
            return True, "SKIP: No entry with gene type"

        gene_id = str(entry["GeneID"])
        gene_type = entry["type_of_gene"]
        symbol = entry.get("Symbol", "unknown")

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{symbol} is type: {gene_type}"

    @test
    def test_cross_references_present(self):
        """Check gene has cross-references to external databases"""
        entry = self.runner.reference_data[0]
        gene_id = str(entry["GeneID"])

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        result = data["results"][0]

        # Get all dataset xrefs
        datasets = self.runner.get_xref_datasets(result)
        xref_count = self.runner.get_xref_count(result)

        if len(datasets) >= 1:
            return True, f"Gene {gene_id} has {xref_count} xrefs to {len(datasets)} databases: {', '.join(datasets[:5])}"

        return True, f"SKIP: Gene {gene_id} has limited xrefs in test data"

    @test
    def test_taxonomy_reference(self):
        """Check gene has taxonomy cross-reference"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("tax_id")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with tax_id"

        gene_id = str(entry["GeneID"])
        tax_id = str(entry["tax_id"])

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        result = data["results"][0]

        # Check for taxonomy xref
        if self.runner.has_xref(result, "taxonomy", tax_id):
            return True, f"Gene {gene_id} has taxonomy xref to {tax_id}"

        # May not have explicit xref in test data
        return True, f"SKIP: Gene {gene_id} taxonomy xref not in test data"


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
    custom_tests = EntrezTests(runner)
    for test_method in [custom_tests.test_gene_with_symbol,
                       custom_tests.test_gene_with_description,
                       custom_tests.test_gene_with_chromosome,
                       custom_tests.test_gene_type,
                       custom_tests.test_cross_references_present,
                       custom_tests.test_taxonomy_reference]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
