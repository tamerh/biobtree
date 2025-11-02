#!/usr/bin/env python3
"""
Patent Test Suite

Tests Patent dataset processing using the common test framework.
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


class PatentTests:
    """Patent custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_patent_with_title(self):
        """Check patent with title."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("title")),
            None
        )

        if not entry:
            return True, "No entries with title"

        patent_id = entry["patent_number"]
        data = self.runner.lookup(patent_id)

        if not data or not data.get("results"):
            return False, f"No results for {patent_id}"

        return True, f"{patent_id} has title: {entry['title'][:50]}..."

    @test
    def test_patent_with_country_date(self):
        """Check patent with country and publication date."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("country") and e.get("publication_date")),
            None
        )

        if not entry:
            return True, "No entries with country and date"

        patent_id = entry["patent_number"]
        data = self.runner.lookup(patent_id)

        if not data or not data.get("results"):
            return False, f"No results for {patent_id}"

        return True, f"{patent_id} has country={entry['country']}, date={entry['publication_date']}"

    @test
    def test_patent_family(self):
        """Check patent with family ID cross-reference."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("family_id") and str(e["family_id"]) != "0"),
            None
        )

        if not entry:
            return True, "No entries with family ID"

        patent_id = entry["patent_number"]
        family_id = str(entry["family_id"])

        # Check if patent family xref exists
        has_family = self.runner.check_xref(patent_id, "patent_family")

        if not has_family:
            return False, f"Patent family cross-reference not found for {patent_id}"

        return True, f"{patent_id} has patent family {family_id}"

    @test
    def test_cpc_codes(self):
        """Check patent with CPC classification codes."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("cpc") and e["cpc"] not in ("", "[]")),
            None
        )

        if not entry:
            return True, "No entries with CPC codes"

        patent_id = entry["patent_number"]

        # Check if CPC xref exists
        has_cpc = self.runner.check_xref(patent_id, "cpc")

        if not has_cpc:
            return False, f"CPC cross-reference not found for {patent_id}"

        return True, f"{patent_id} has CPC codes"

    @test
    def test_ipc_codes(self):
        """Check patent with IPC classification codes."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("ipc") and e["ipc"] not in ("", "[]")),
            None
        )

        if not entry:
            return True, "No entries with IPC codes"

        patent_id = entry["patent_number"]

        # Check if IPC xref exists
        has_ipc = self.runner.check_xref(patent_id, "ipc")

        if not has_ipc:
            return False, f"IPC cross-reference not found for {patent_id}"

        return True, f"{patent_id} has IPC codes"

    @test
    def test_assignees(self):
        """Check patent with assignee cross-references."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("asignee") and e["asignee"] not in ("", "[]")),
            None
        )

        if not entry:
            return True, "No entries with assignees"

        patent_id = entry["patent_number"]

        # Check if assignee xref exists
        has_assignee = self.runner.check_xref(patent_id, "assignee")

        if not has_assignee:
            return False, f"Assignee cross-reference not found for {patent_id}"

        return True, f"{patent_id} has assignees"

    @test
    def test_patent_compounds(self):
        """Check patent with compound cross-references."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("compound_ids") and len(e["compound_ids"]) > 0),
            None
        )

        if not entry:
            return True, "No entries with compounds"

        patent_id = entry["patent_number"]
        compound_count = entry.get("compound_count", 0)

        # Check if patent_compound xref exists
        has_compound = self.runner.check_xref(patent_id, "patent_compound")

        if not has_compound:
            return False, f"Patent-compound cross-reference not found for {patent_id}"

        return True, f"{patent_id} has {compound_count} compounds"

    # Note: InChI Key and SMILES searchability tests are omitted here
    # because they require ChEMBL dataset to be present. The patent dataset
    # creates links from InChI Keys and SMILES to patent compounds, but
    # searchability of these identifiers comes from ChEMBL integration.


def main():
    """Main test entry point."""
    # Setup paths
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # API URL (default port used by orchestrator)
    api_url = os.environ.get("BIOBTREE_API_URL", "http://localhost:9292")

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = PatentTests(runner)
    for test_method in [
        custom_tests.test_patent_with_title,
        custom_tests.test_patent_with_country_date,
        custom_tests.test_patent_family,
        custom_tests.test_cpc_codes,
        custom_tests.test_ipc_codes,
        custom_tests.test_assignees,
        custom_tests.test_patent_compounds,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
