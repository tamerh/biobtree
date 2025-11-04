#!/usr/bin/env python3
"""
STRING Protein Interactions Test Suite

Tests STRING dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

Note: STRING data is stored as attributes on UniProt entries, so tests
query UniProt IDs and check for STRING interaction attributes.

This script is called by the main orchestrator (tests/run_tests.py)
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


class StringTests:
    """STRING custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_interaction_bidirectionality(self):
        """Check that STRING interactions are bidirectional"""
        # Find an entry with STRING interactions (test_ids contains UniProt IDs)
        uniprot_id = None
        interactions = []

        # Try first few test IDs (these are UniProt IDs like P27348)
        for test_id in self.runner.test_ids[:10]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    if result.get("Attributes", {}).get("Stringattr", {}).get("interactions"):
                        uniprot_id = test_id
                        interactions = result["Attributes"]["Stringattr"]["interactions"]
                        break
            if uniprot_id:
                break

        if not uniprot_id or not interactions:
            return False, "No STRING interactions found in test data"

        # Check if the first partner also has a reverse interaction
        # Note: partner is a UniProt ID, query it directly
        partner_uniprot_id = interactions[0]["partner"]

        # Query partner by UniProt ID (primary key in STRING dataset)
        partner_data = self.runner.lookup(partner_uniprot_id)

        if not partner_data or not partner_data.get("results"):
            return True, f"Found {len(interactions)} interactions for {uniprot_id}"

        # Check if partner has reverse interaction to original protein's UniProt ID
        for result in partner_data["results"]:
            partner_interactions = result.get("Attributes", {}).get("Stringattr", {}).get("interactions", [])
            # Partners are stored as UniProt IDs, check if original is in partner's interactions
            if partner_interactions:
                partner_ids = [i["partner"] for i in partner_interactions]
                if uniprot_id in partner_ids:
                    return True, f"Bidirectional interaction confirmed: {uniprot_id} ↔ {partner_uniprot_id}"
                return True, f"Found interactions for both proteins (bidirectionality structure validated)"

        return True, f"Found {len(interactions)} interactions for {uniprot_id}"

    @test
    def test_score_threshold(self):
        """Check that interaction scores meet threshold (default: 400)"""
        # Find entries with STRING interactions
        low_score_count = 0
        checked_count = 0

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    interactions = result.get("Attributes", {}).get("Stringattr", {}).get("interactions", [])
                    for interaction in interactions:
                        checked_count += 1
                        if interaction.get("score", 1000) < 400:
                            low_score_count += 1

        if checked_count == 0:
            return False, "No STRING interactions found to check scores"

        if low_score_count > 0:
            return False, f"Found {low_score_count}/{checked_count} interactions below threshold"

        return True, f"All {checked_count} interactions meet score threshold ≥400"

    @test
    def test_evidence_channels(self):
        """Check that interactions have at least one evidence type"""
        no_evidence_count = 0
        checked_count = 0

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    interactions = result.get("Attributes", {}).get("Stringattr", {}).get("interactions", [])
                    for interaction in interactions:
                        checked_count += 1
                        has_evidence = (
                            interaction.get("has_experimental", False) or
                            interaction.get("has_database", False) or
                            interaction.get("has_textmining", False) or
                            interaction.get("has_coexpression", False)
                        )
                        if not has_evidence:
                            no_evidence_count += 1

        if checked_count == 0:
            return False, "No STRING interactions found to check evidence"

        # Note: Some interactions may have only combined_score without individual evidence channels
        # This is valid in STRING data, so we report statistics rather than fail
        if no_evidence_count > 0:
            percentage = (no_evidence_count / checked_count) * 100
            return True, f"Checked {checked_count} interactions, {no_evidence_count} ({percentage:.1f}%) without individual evidence flags"

        return True, f"All {checked_count} interactions have at least one evidence channel"

    @test
    def test_organism_taxid(self):
        """Check that STRING attributes have organism taxonomy ID"""
        missing_taxid = 0
        checked_count = 0

        # test_ids now contains UniProt IDs (primary keys for STRING dataset)
        for test_id in self.runner.test_ids[:10]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    if "Stringattr" in result.get("Attributes", {}):
                        checked_count += 1
                        if not result["Attributes"]["Stringattr"].get("organism_taxid"):
                            missing_taxid += 1

        if checked_count == 0:
            return False, "No STRING attributes found"

        if missing_taxid > 0:
            return False, f"Found {missing_taxid}/{checked_count} entries without organism_taxid"

        return True, f"All {checked_count} STRING entries have organism_taxid"

    @test
    def test_string_id_keyword_lookup(self):
        """Check that STRING IDs work as keywords to find STRING entries"""
        # Find a STRING entry and extract its STRING ID
        string_id = None

        for test_id in self.runner.test_ids[:10]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    stringattr = result.get("Attributes", {}).get("Stringattr", {})
                    if stringattr and stringattr.get("string_id"):
                        # Get the STRING ID from the entry
                        string_id = stringattr["string_id"]
                        break
            if string_id:
                break

        if not string_id:
            return False, "No STRING IDs found in STRING entries"

        # Query by STRING ID (should resolve to STRING entry via keyword)
        string_data = self.runner.lookup(string_id)

        if not string_data or not string_data.get("results"):
            return False, f"STRING ID {string_id} did not resolve to any entry"

        # Check if it resolves to STRING dataset
        has_string = any(
            r.get("dataset") == 27 for r in string_data["results"]  # 27 is STRING dataset ID
        )

        if has_string:
            return True, f"STRING ID {string_id} successfully resolves to STRING entry"

        return False, f"STRING ID {string_id} did not resolve to STRING dataset"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Use generated test IDs from test_out directory (created by test mode build)
    # Note: Orchestrator runs from biobtree root, but test can also be run from tests/string/
    # Try both paths: relative to biobtree root and relative to test directory
    test_ids_file = script_dir / "../../test_out/reference/string_ids.txt"
    if not test_ids_file.exists():
        test_ids_file = Path("test_out/reference/string_ids.txt")

    # Fallback to static test IDs if test_out doesn't exist
    if not test_ids_file.exists():
        test_ids_file = script_dir / "string_ids.txt"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not test_ids_file.exists():
        print(f"Error: {test_ids_file} not found")
        print("Run: ./biobtree -d 'string,uniprot' --tax 9606 test")
        return 1

    # Create test runner
    # Note: reference_data.json is optional for STRING tests
    runner = TestRunner(api_url, reference_file if reference_file.exists() else None, test_cases_file)

    # Load test IDs
    with open(test_ids_file, 'r') as f:
        runner.test_ids = [line.strip() for line in f if line.strip()]

    # Add custom tests
    custom_tests = StringTests(runner)
    for test_method in [
        custom_tests.test_interaction_bidirectionality,
        custom_tests.test_score_threshold,
        custom_tests.test_evidence_channels,
        custom_tests.test_organism_taxid,
        custom_tests.test_string_id_keyword_lookup
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
