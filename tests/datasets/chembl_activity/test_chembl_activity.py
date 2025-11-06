#!/usr/bin/env python3
"""
ChEMBL Activity Test Suite

Tests chembl_activity dataset processing using the common test framework.
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


class ChEMBLActivityTests:
    """ChEMBL Activity custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_activity_with_assay(self):
        """Check activity has assay reference"""
        # Find an activity with assay
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("assay_chembl_id")),
            None
        )
        if not entry:
            return False, "No activity with assay in reference"

        activity_id = entry["activity_chembl_id"]
        assay_id = entry["assay_chembl_id"]

        data = self.runner.lookup(activity_id)

        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"Activity {activity_id} linked to assay {assay_id}"

    @test
    def test_activity_with_molecule(self):
        """Check activity has molecule reference"""
        # Find an activity with molecule
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("molecule_chembl_id")),
            None
        )
        if not entry:
            return False, "No activity with molecule in reference"

        activity_id = entry["activity_chembl_id"]
        molecule_id = entry["molecule_chembl_id"]

        data = self.runner.lookup(activity_id)

        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"Activity {activity_id} linked to molecule {molecule_id}"

    @test
    def test_activity_with_target(self):
        """Check activity has target reference"""
        # Find an activity with target
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_chembl_id")),
            None
        )
        if not entry:
            return False, "No activity with target in reference"

        activity_id = entry["activity_chembl_id"]
        target_id = entry["target_chembl_id"]

        data = self.runner.lookup(activity_id)

        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"Activity {activity_id} linked to target {target_id}"

    @test
    def test_activity_with_measurement(self):
        """Check activity has measurement value"""
        # Find an activity with standard value
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("standard_value") and e.get("standard_type")),
            None
        )
        if not entry:
            return False, "No activity with measurement in reference"

        activity_id = entry["activity_chembl_id"]
        std_type = entry["standard_type"]
        std_value = entry["standard_value"]
        units = entry.get("standard_units", "")

        data = self.runner.lookup(activity_id)

        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"Activity {activity_id}: {std_type} = {std_value} {units}"

    @test
    def test_activity_with_document(self):
        """Check activity has document reference"""
        # Find an activity with document
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("document_chembl_id")),
            None
        )
        if not entry:
            return False, "No activity with document in reference"

        activity_id = entry["activity_chembl_id"]
        doc_id = entry["document_chembl_id"]

        data = self.runner.lookup(activity_id)

        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"Activity {activity_id} linked to document {doc_id}"

    @test
    def test_activity_with_organism(self):
        """Check activity has target organism"""
        # Find an activity with target organism
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_organism")),
            None
        )
        if not entry:
            return False, "No activity with target organism in reference"

        activity_id = entry["activity_chembl_id"]
        organism = entry["target_organism"]

        data = self.runner.lookup(activity_id)

        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"Activity {activity_id} for organism: {organism}"


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
    custom_tests = ChEMBLActivityTests(runner)
    for test_method in [
        custom_tests.test_activity_with_assay,
        custom_tests.test_activity_with_molecule,
        custom_tests.test_activity_with_target,
        custom_tests.test_activity_with_measurement,
        custom_tests.test_activity_with_document,
        custom_tests.test_activity_with_organism
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
