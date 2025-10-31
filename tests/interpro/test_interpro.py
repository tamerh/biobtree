#!/usr/bin/env python3
"""
InterPro Test Suite

Tests InterPro dataset processing using the common test framework.
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


class InterProTests:
    """InterPro custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_entry_with_type(self):
        """Check InterPro entry has entry type"""
        # Find an entry with a type defined
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("metadata", {}).get("type") and e["metadata"]["type"] != ""),
            None
        )
        if not entry:
            return False, "No InterPro entry with type in reference"

        interpro_id = entry["id"]
        entry_type = entry.get("metadata", {}).get("type", "unknown")

        data = self.runner.lookup(interpro_id)

        if not data or not data.get("results"):
            return False, f"No results for {interpro_id}"

        return True, f"{interpro_id} has type: {entry_type}"

    @test
    def test_entry_with_protein_count(self):
        """Check InterPro entry has protein count"""
        # Find an entry with protein count
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("metadata", {}).get("counters", {}).get("proteins", 0) > 0),
            None
        )
        if not entry:
            return False, "No InterPro entry with protein_count in reference"

        interpro_id = entry["id"]
        protein_count = entry.get("metadata", {}).get("counters", {}).get("proteins", 0)

        data = self.runner.lookup(interpro_id)

        if not data or not data.get("results"):
            return False, f"No results for {interpro_id}"

        return True, f"{interpro_id} has {protein_count} proteins"

    @test
    def test_entry_with_member_databases(self):
        """Check InterPro entry has member database cross-references"""
        # Find an entry with member databases
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("metadata", {}).get("member_databases")
             and len(e.get("metadata", {}).get("member_databases", {})) > 0),
            None
        )
        if not entry:
            return False, "No InterPro entry with member_databases in reference"

        interpro_id = entry["id"]
        member_dbs = list(entry.get("metadata", {}).get("member_databases", {}).keys())
        db_count = len(member_dbs)

        data = self.runner.lookup(interpro_id)

        if not data or not data.get("results"):
            return False, f"No results for {interpro_id}"

        return True, f"{interpro_id} has {db_count} member database(s): {', '.join(member_dbs[:3])}"


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
    custom_tests = InterProTests(runner)
    for test_method in [custom_tests.test_entry_with_type,
                       custom_tests.test_entry_with_protein_count,
                       custom_tests.test_entry_with_member_databases]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
