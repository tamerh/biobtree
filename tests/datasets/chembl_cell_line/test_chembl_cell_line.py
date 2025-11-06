#!/usr/bin/env python3
"""
ChEMBL Cell Line Test Suite

Tests chembl_cell_line dataset processing using the common test framework.
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


class ChEMBLCellLineTests:
    """ChEMBL Cell Line custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_cell_line_with_description(self):
        """Check cell line has description"""
        # Find a cell line with description
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("cell_description")),
            None
        )
        if not entry:
            return False, "No cell line with description in reference"

        cell_id = entry["cell_chembl_id"]
        description = entry["cell_description"][:50]

        data = self.runner.lookup(cell_id)

        if not data or not data.get("results"):
            return False, f"No results for {cell_id}"

        return True, f"{cell_id} has description: {description}..."

    @test
    def test_cell_line_with_organism(self):
        """Check cell line has source organism"""
        # Find a cell line with organism
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("cell_source_organism")),
            None
        )
        if not entry:
            return False, "No cell line with organism in reference"

        cell_id = entry["cell_chembl_id"]
        organism = entry["cell_source_organism"]

        data = self.runner.lookup(cell_id)

        if not data or not data.get("results"):
            return False, f"No results for {cell_id}"

        return True, f"{cell_id} from organism: {organism}"

    @test
    def test_cell_line_with_taxonomy(self):
        """Check cell line has taxonomy ID"""
        # Find a cell line with tax_id
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("cell_source_tax_id")),
            None
        )
        if not entry:
            return False, "No cell line with taxonomy in reference"

        cell_id = entry["cell_chembl_id"]
        tax_id = entry["cell_source_tax_id"]

        data = self.runner.lookup(cell_id)

        if not data or not data.get("results"):
            return False, f"No results for {cell_id}"

        return True, f"{cell_id} has taxonomy: {tax_id}"

    @test
    def test_cell_line_with_tissue(self):
        """Check cell line has source tissue"""
        # Find a cell line with tissue
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("cell_source_tissue")),
            None
        )
        if not entry:
            return False, "No cell line with tissue in reference"

        cell_id = entry["cell_chembl_id"]
        tissue = entry["cell_source_tissue"]

        data = self.runner.lookup(cell_id)

        if not data or not data.get("results"):
            return False, f"No results for {cell_id}"

        return True, f"{cell_id} from tissue: {tissue}"

    @test
    def test_cell_line_with_cellosaurus_id(self):
        """Check cell line has Cellosaurus ID"""
        # Find a cell line with cellosaurus_id
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("cellosaurus_id")),
            None
        )
        if not entry:
            return False, "No cell line with Cellosaurus ID in reference"

        cell_id = entry["cell_chembl_id"]
        cellosaurus_id = entry["cellosaurus_id"]

        data = self.runner.lookup(cell_id)

        if not data or not data.get("results"):
            return False, f"No results for {cell_id}"

        return True, f"{cell_id} has Cellosaurus ID: {cellosaurus_id}"

    @test
    def test_cell_line_with_name(self):
        """Check cell line has name"""
        # Find a cell line with name
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("cell_name")),
            None
        )
        if not entry:
            return False, "No cell line with name in reference"

        cell_id = entry["cell_chembl_id"]
        name = entry["cell_name"]

        data = self.runner.lookup(cell_id)

        if not data or not data.get("results"):
            return False, f"No results for {cell_id}"

        return True, f"{cell_id} has name: {name}"


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
    custom_tests = ChEMBLCellLineTests(runner)
    for test_method in [
        custom_tests.test_cell_line_with_description,
        custom_tests.test_cell_line_with_organism,
        custom_tests.test_cell_line_with_taxonomy,
        custom_tests.test_cell_line_with_tissue,
        custom_tests.test_cell_line_with_cellosaurus_id,
        custom_tests.test_cell_line_with_name
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
