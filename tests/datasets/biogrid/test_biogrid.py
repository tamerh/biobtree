#!/usr/bin/env python3
"""
BioGRID Test Suite

Tests BioGRID protein-protein and genetic interaction database integration.
Uses declarative tests from test_cases.json and custom Python tests.

Note: This script is called by the main orchestrator (tests/run_tests.py)
which manages the biobtree web server lifecycle.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path (tests/ directory)
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class BioGRIDTests:
    """BioGRID custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner
        self.test_biogrid_id = "112315"  # Known test ID

    @test
    def test_biogrid_basic_lookup(self):
        """Test basic BioGRID interactor lookup."""
        data = self.runner.lookup(self.test_biogrid_id)

        if not data or not data.get("results"):
            return False, f"No results for BioGRID ID {self.test_biogrid_id}"

        # Check that we got a biogrid entry
        found_biogrid = any(
            entry.get("dataset_name") == "biogrid"
            for entry in data.get("results", [])
        )

        if not found_biogrid:
            return False, f"No biogrid entry found for {self.test_biogrid_id}"

        return True, f"Basic BioGRID lookup works for {self.test_biogrid_id}"

    @test
    def test_biogrid_attributes(self):
        """Test BioGRID entry has expected attributes."""
        data = self.runner.lookup(self.test_biogrid_id)

        if not data or not data.get("results"):
            return False, f"No results for BioGRID ID {self.test_biogrid_id}"

        # Find the biogrid entry
        biogrid_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid"),
            None
        )

        if not biogrid_entry:
            return False, f"No biogrid entry found for {self.test_biogrid_id}"

        # Check for biogrid attribute (under Attributes.Biogrid)
        attributes = biogrid_entry.get("Attributes", {})
        biogrid_attr = attributes.get("Biogrid", {})
        if not biogrid_attr:
            return False, "BioGRID entry missing 'Attributes.Biogrid' attribute"

        # Check required fields
        required_fields = ["biogrid_id", "interaction_count", "unique_partners"]
        missing = [f for f in required_fields if f not in biogrid_attr]

        if missing:
            return False, f"BioGRID entry missing fields: {missing}"

        return True, f"BioGRID attributes present for {self.test_biogrid_id}"

    @test
    def test_biogrid_interactions(self):
        """Test BioGRID entry has interactions list."""
        data = self.runner.lookup(self.test_biogrid_id)

        if not data or not data.get("results"):
            return False, f"No results for BioGRID ID {self.test_biogrid_id}"

        # Find the biogrid entry
        biogrid_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid"),
            None
        )

        if not biogrid_entry:
            return False, f"No biogrid entry found for {self.test_biogrid_id}"

        biogrid_attr = biogrid_entry.get("Attributes", {}).get("Biogrid", {})
        interactions = biogrid_attr.get("interactions", [])

        if not interactions:
            return False, "BioGRID entry has no interactions"

        # Check first interaction has expected fields
        first = interactions[0]
        expected_fields = ["interaction_id", "partner_biogrid_id", "experimental_system"]
        missing = [f for f in expected_fields if f not in first]

        if missing:
            return False, f"Interaction missing fields: {missing}"

        return True, f"BioGRID has {len(interactions)} interactions for {self.test_biogrid_id}"

    @test
    def test_biogrid_throughput_field(self):
        """Test BioGRID interactions have throughput field (TAB3 format)."""
        data = self.runner.lookup(self.test_biogrid_id)

        if not data or not data.get("results"):
            return False, f"No results for BioGRID ID {self.test_biogrid_id}"

        # Find the biogrid entry
        biogrid_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid"),
            None
        )

        if not biogrid_entry:
            return False, f"No biogrid entry found for {self.test_biogrid_id}"

        biogrid_attr = biogrid_entry.get("Attributes", {}).get("Biogrid", {})
        interactions = biogrid_attr.get("interactions", [])

        if not interactions:
            return False, "BioGRID entry has no interactions"

        # Check if any interaction has throughput field
        has_throughput = any(
            inter.get("throughput") for inter in interactions
        )

        if not has_throughput:
            return False, "No interactions have throughput field"

        # Get the throughput value
        throughput = interactions[0].get("throughput", "")
        valid_values = ["Low Throughput", "High Throughput"]

        if throughput and throughput not in valid_values:
            return False, f"Invalid throughput value: {throughput}"

        return True, f"Throughput field present: {throughput}"

    @test
    def test_biogrid_to_entrez_mapping(self):
        """Test mapping from BioGRID to Entrez Gene."""
        has_xref = self.runner.check_xref(self.test_biogrid_id, "entrez")

        if not has_xref:
            return False, f"No Entrez Gene mappings found for {self.test_biogrid_id}"

        return True, f"BioGRID -> Entrez mapping works for {self.test_biogrid_id}"

    @test
    def test_biogrid_to_pubmed_mapping(self):
        """Test mapping from BioGRID to PubMed."""
        has_xref = self.runner.check_xref(self.test_biogrid_id, "pubmed")

        if not has_xref:
            return False, f"No PubMed mappings found for {self.test_biogrid_id}"

        return True, f"BioGRID -> PubMed mapping works for {self.test_biogrid_id}"

    @test
    def test_biogrid_partner_mapping(self):
        """Test mapping from BioGRID to partner BioGRID entries."""
        data = self.runner.lookup(self.test_biogrid_id)

        if not data or not data.get("results"):
            return False, f"No results for BioGRID ID {self.test_biogrid_id}"

        # Find the biogrid entry
        biogrid_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid"),
            None
        )

        if not biogrid_entry:
            return False, f"No biogrid entry found for {self.test_biogrid_id}"

        # Check for biogrid partner in entries list (different ID than self)
        entries = biogrid_entry.get("entries", [])
        has_partner = any(
            entry.get("dataset_name") == "biogrid" and
            entry.get("identifier") != self.test_biogrid_id
            for entry in entries
        )

        if not has_partner:
            return False, f"No BioGRID partner mappings found for {self.test_biogrid_id}"

        return True, f"BioGRID -> BioGRID partner mapping works for {self.test_biogrid_id}"


def main():
    """Main test entry point."""
    # Setup paths
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # API URL (default port used by orchestrator)
    api_url = os.environ.get("BIOBTREE_API_URL", "http://localhost:9292")

    # Create test runner (reference_file may not exist, that's OK)
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = BioGRIDTests(runner)
    for test_method in [
        custom_tests.test_biogrid_basic_lookup,
        custom_tests.test_biogrid_attributes,
        custom_tests.test_biogrid_interactions,
        custom_tests.test_biogrid_throughput_field,
        custom_tests.test_biogrid_to_entrez_mapping,
        custom_tests.test_biogrid_to_pubmed_mapping,
        custom_tests.test_biogrid_partner_mapping,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
