#!/usr/bin/env python3
"""
ChEMBL Document Test Suite

Tests chembl_document dataset processing using the common test framework.
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


class ChEMBLDocumentTests:
    """ChEMBL Document custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_publication_document(self):
        """Check document with PUBLICATION type"""
        # Find a PUBLICATION type document
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("doc_type") == "PUBLICATION"),
            None
        )
        if not entry:
            return False, "No PUBLICATION document in reference"

        doc_id = entry["document_chembl_id"]
        title = entry.get("title", "")[:60]

        data = self.runner.lookup(doc_id)

        if not data or not data.get("results"):
            return False, f"No results for {doc_id}"

        return True, f"{doc_id} is PUBLICATION: {title}..."

    @test
    def test_document_with_journal(self):
        """Check document has journal information"""
        # Find a document with journal
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("journal") and e["journal"] != ""),
            None
        )
        if not entry:
            return False, "No document with journal in reference"

        doc_id = entry["document_chembl_id"]
        journal = entry.get("journal", "unknown")

        data = self.runner.lookup(doc_id)

        if not data or not data.get("results"):
            return False, f"No results for {doc_id}"

        return True, f"{doc_id} published in: {journal}"

    @test
    def test_document_with_pubmed_id(self):
        """Check document has PubMed ID cross-reference"""
        # Find a document with PubMed ID
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("pubmed_id") and e["pubmed_id"] is not None),
            None
        )
        if not entry:
            return False, "No document with PubMed ID in reference"

        doc_id = entry["document_chembl_id"]
        pmid = entry["pubmed_id"]

        data = self.runner.lookup(doc_id)

        if not data or not data.get("results"):
            return False, f"No results for {doc_id}"

        return True, f"{doc_id} has PubMed ID: {pmid}"

    @test
    def test_document_with_doi(self):
        """Check document has DOI cross-reference"""
        # Find a document with DOI
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("doi") and e["doi"] is not None),
            None
        )
        if not entry:
            return False, "No document with DOI in reference"

        doc_id = entry["document_chembl_id"]
        doi = entry["doi"]

        data = self.runner.lookup(doc_id)

        if not data or not data.get("results"):
            return False, f"No results for {doc_id}"

        return True, f"{doc_id} has DOI: {doi}"

    @test
    def test_document_with_year(self):
        """Check document has publication year"""
        # Find a document with year
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("year") and e["year"] is not None),
            None
        )
        if not entry:
            return False, "No document with year in reference"

        doc_id = entry["document_chembl_id"]
        year = entry["year"]

        data = self.runner.lookup(doc_id)

        if not data or not data.get("results"):
            return False, f"No results for {doc_id}"

        return True, f"{doc_id} published in {year}"


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
    custom_tests = ChEMBLDocumentTests(runner)
    for test_method in [
        custom_tests.test_publication_document,
        custom_tests.test_document_with_journal,
        custom_tests.test_document_with_pubmed_id,
        custom_tests.test_document_with_doi,
        custom_tests.test_document_with_year
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
