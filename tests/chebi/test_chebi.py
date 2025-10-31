#!/usr/bin/env python3
"""
ChEBI Dataset Tests

Note: ChEBI is a cross-reference-only dataset in biobtree.
Unlike other datasets (GO, UniProt, etc.) which store primary searchable entries,
ChEBI only stores cross-references FROM ChEBI IDs TO other databases.

Structure:
- ChEBI parser reads database_accession.tsv
- Format: CHEBI:X -> target_database:Y
- These are stored as inverse xrefs ON the target entries
- ChEBI IDs themselves are NOT directly searchable

This means ChEBI tests focus on:
1. Verifying ChEBI IDs were processed
2. Checking that cross-references exist (if target entries are in test DB)
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


class ChEBITests:
    """ChEBI custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_chebi_ids_logged(self):
        """Verify ChEBI IDs were logged during database build"""
        script_dir = Path(__file__).parent
        id_file = script_dir / "chebi_ids.txt"

        if not id_file.exists():
            return False, f"ChEBI IDs file not found: {id_file}"

        with open(id_file, 'r') as f:
            ids = [line.strip() for line in f if line.strip()]

        if len(ids) < 100:
            return False, f"Expected 100 ChEBI IDs, found {len(ids)}"

        # Verify format
        sample_id = ids[0]
        if not sample_id.startswith("CHEBI:"):
            return False, f"Invalid ChEBI ID format: {sample_id}"

        return True, f"Logged {len(ids)} ChEBI IDs (sample: {sample_id})"

    @test
    def test_chebi_cross_reference_structure(self):
        """Document ChEBI's special cross-reference-only structure"""
        return (
            True,
            "ChEBI stores cross-references FROM ChEBI IDs TO other databases. "
            "ChEBI IDs are not directly searchable in biobtree. "
            "Cross-references are stored on target database entries (UniProt, GO, etc.)."
        )

    @test
    def test_chebi_id_not_searchable(self):
        """Verify that ChEBI IDs are not directly searchable (expected behavior)"""
        # Get a ChEBI ID from our test set
        script_dir = Path(__file__).parent
        id_file = script_dir / "chebi_ids.txt"
        with open(id_file, 'r') as f:
            chebi_id = f.readline().strip()

        # Try to search for it - this SHOULD fail
        data = self.runner.query.lookup(chebi_id)

        if data and data.get("results"):
            return (
                False,
                f"Unexpected: {chebi_id} is directly searchable. "
                "ChEBI should only store cross-references, not primary entries."
            )

        return (
            True,
            f"{chebi_id} is not directly searchable (expected for cross-reference-only dataset)"
        )

    @test
    def test_sample_entries_for_chebi_xrefs(self):
        """
        Check if any entries in other databases have ChEBI cross-references

        Note: This may not find any xrefs in the test database depending on
        which entries were selected for testing. This is informational only.
        """
        # Try a few entries from other datasets that might have ChEBI xrefs
        test_candidates = [
            "GO:0000001",
            "GO:0000002",
            "P0C9F0",  # UniProt
            "HGNC:5",
        ]

        found_chebi_xrefs = []

        for entry_id in test_candidates:
            # Query with x=true to get cross-references
            try:
                response = requests.get(
                    f"{self.runner.api_url}/ws/search",
                    params={"i": entry_id, "x": "true"},
                    timeout=5
                )
                if response.status_code != 200:
                    continue

                data = response.json()
                if not data or not data.get("results"):
                    continue

                result = data["results"][0]
                xrefs = result.get("Xrefs", {})

                if "chebi" in xrefs:
                    chebi_xref_count = len(xrefs["chebi"])
                    found_chebi_xrefs.append(f"{entry_id} ({chebi_xref_count} ChEBI xrefs)")
            except Exception:
                continue

        if found_chebi_xrefs:
            return (
                True,
                f"Found ChEBI cross-references in {len(found_chebi_xrefs)} entries: {', '.join(found_chebi_xrefs)}"
            )
        else:
            return (
                True,
                "No ChEBI cross-references found in sample entries (may be expected in small test DB)"
            )


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
    custom_tests = ChEBITests(runner)
    for test_method in [
        custom_tests.test_chebi_ids_logged,
        custom_tests.test_chebi_cross_reference_structure,
        custom_tests.test_chebi_id_not_searchable,
        custom_tests.test_sample_entries_for_chebi_xrefs
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
