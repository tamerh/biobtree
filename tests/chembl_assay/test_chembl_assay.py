#!/usr/bin/env python3
"""
ChEMBL Assay Test Suite

Tests chembl_assay dataset processing using the common test framework.
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


class ChEMBLAssayTests:
    """ChEMBL Assay custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_assay_with_description(self):
        """Check assay has description"""
        # Find an assay with description
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("description")),
            None
        )
        if not entry:
            return False, "No assay with description in reference"

        assay_id = entry["assay_chembl_id"]
        desc = entry["description"][:60]

        data = self.runner.lookup(assay_id)

        if not data or not data.get("results"):
            return False, f"No results for {assay_id}"

        return True, f"{assay_id} has description: {desc}..."

    @test
    def test_assay_with_type(self):
        """Check assay has type"""
        # Find an assay with type
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("assay_type")),
            None
        )
        if not entry:
            return False, "No assay with type in reference"

        assay_id = entry["assay_chembl_id"]
        assay_type = entry["assay_type"]

        data = self.runner.lookup(assay_id)

        if not data or not data.get("results"):
            return False, f"No results for {assay_id}"

        return True, f"{assay_id} has type: {assay_type}"

    @test
    def test_assay_with_organism(self):
        """Check assay has organism"""
        # Find an assay with organism
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("assay_organism")),
            None
        )
        if not entry:
            return False, "No assay with organism in reference"

        assay_id = entry["assay_chembl_id"]
        organism = entry["assay_organism"]

        data = self.runner.lookup(assay_id)

        if not data or not data.get("results"):
            return False, f"No results for {assay_id}"

        return True, f"{assay_id} for organism: {organism}"

    @test
    def test_assay_with_target(self):
        """Check assay has target"""
        # Find an assay with target
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_chembl_id")),
            None
        )
        if not entry:
            return False, "No assay with target in reference"

        assay_id = entry["assay_chembl_id"]
        target_id = entry["target_chembl_id"]

        data = self.runner.lookup(assay_id)

        if not data or not data.get("results"):
            return False, f"No results for {assay_id}"

        return True, f"{assay_id} has target: {target_id}"

    @test
    def test_assay_with_document(self):
        """Check assay has document reference"""
        # Find an assay with document
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("document_chembl_id")),
            None
        )
        if not entry:
            return False, "No assay with document in reference"

        assay_id = entry["assay_chembl_id"]
        doc_id = entry["document_chembl_id"]

        data = self.runner.lookup(assay_id)

        if not data or not data.get("results"):
            return False, f"No results for {assay_id}"

        return True, f"{assay_id} linked to document: {doc_id}"

    @test
    def test_assay_with_tissue(self):
        """Check assay has tissue"""
        # Find an assay with tissue
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("assay_tissue")),
            None
        )
        if not entry:
            return False, "No assay with tissue in reference"

        assay_id = entry["assay_chembl_id"]
        tissue = entry["assay_tissue"]

        data = self.runner.lookup(assay_id)

        if not data or not data.get("results"):
            return False, f"No results for {assay_id}"

        return True, f"{assay_id} has tissue: {tissue}"


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
    custom_tests = ChEMBLAssayTests(runner)
    for test_method in [
        custom_tests.test_assay_with_description,
        custom_tests.test_assay_with_type,
        custom_tests.test_assay_with_organism,
        custom_tests.test_assay_with_target,
        custom_tests.test_assay_with_document,
        custom_tests.test_assay_with_tissue
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
