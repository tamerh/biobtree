#!/usr/bin/env python3
"""
GenCC (Gene Curation Coalition) Test Suite

Tests gene-disease validity curations, classifications, and cross-references.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class GenccTests:
    """GenCC custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_entry_with_gene_symbol(self):
        """Check entry has gene symbol"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_symbol")),
            None
        )
        if not entry:
            return False, "No entry with gene_symbol in reference"

        uuid = entry["uuid"]
        gene_symbol = entry["gene_symbol"]
        data = self.runner.lookup(uuid)

        if not data or not data.get("results"):
            return False, f"No results for {uuid}"

        return True, f"{uuid[:40]}... has gene_symbol: {gene_symbol}"

    @test
    def test_entry_with_disease(self):
        """Check entry has disease information"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("disease_title")),
            None
        )
        if not entry:
            return False, "No entry with disease_title in reference"

        uuid = entry["uuid"]
        disease_title = entry["disease_title"][:50]
        data = self.runner.lookup(uuid)

        if not data or not data.get("results"):
            return False, f"No results for {uuid}"

        return True, f"Entry has disease: {disease_title}..."

    @test
    def test_entry_with_classification(self):
        """Check entry has classification (gene-disease validity)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("classification_title")),
            None
        )
        if not entry:
            return False, "No entry with classification_title in reference"

        uuid = entry["uuid"]
        classification = entry["classification_title"]
        data = self.runner.lookup(uuid)

        if not data or not data.get("results"):
            return False, f"No results for {uuid}"

        return True, f"Entry has classification: {classification}"

    @test
    def test_text_search_by_gene_symbol(self):
        """Test text search by gene symbol"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_symbol") and len(e["gene_symbol"]) >= 2),
            None
        )
        if not entry:
            return False, "No entry with suitable gene_symbol in reference"

        gene_symbol = entry["gene_symbol"]

        # Search by gene symbol
        data = self.runner.lookup(gene_symbol)
        if not data or not data.get("results"):
            return False, f"No results for gene symbol: {gene_symbol}"

        return True, f"Found results for gene symbol '{gene_symbol}'"

    @test
    def test_text_search_by_disease_title(self):
        """Test text search by disease title"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("disease_title") and len(e["disease_title"]) > 10),
            None
        )
        if not entry:
            return False, "No entry with suitable disease_title in reference"

        disease_title = entry["disease_title"]

        # Search by disease title
        data = self.runner.lookup(disease_title)
        if not data or not data.get("results"):
            return False, f"No results for disease: {disease_title}"

        return True, f"Found results for disease '{disease_title[:40]}...'"

    @test
    def test_entry_with_pmids(self):
        """Check entry has PubMed references"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("submitted_as_pmids") and len(e["submitted_as_pmids"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with PMIDs in reference"

        uuid = entry["uuid"]
        pmid_count = len(entry["submitted_as_pmids"])
        data = self.runner.lookup(uuid)

        if not data or not data.get("results"):
            return False, f"No results for {uuid}"

        return True, f"Entry has {pmid_count} PMID(s)"


def main():
    """Main test execution"""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        print("Run: python3 extract_reference_data.py")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom_tests = GenccTests(runner)

    for test_method in [
        custom_tests.test_entry_with_gene_symbol,
        custom_tests.test_entry_with_disease,
        custom_tests.test_entry_with_classification,
        custom_tests.test_text_search_by_gene_symbol,
        custom_tests.test_text_search_by_disease_title,
        custom_tests.test_entry_with_pmids,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
