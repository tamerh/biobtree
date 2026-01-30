#!/usr/bin/env python3
"""
BioGRID Test Suite

Tests BioGRID protein-protein and genetic interaction database integration.
Uses declarative tests from test_cases.json and custom Python tests.

BioGRID uses a dual-dataset architecture:
- biogrid: Lightweight summary entries (statistics only)
- biogrid_interaction: Individual interaction records with full details

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
        self.test_biogrid_id = "112315"  # Known test ID for biogrid summary

    # ==========================================================================
    # biogrid dataset tests (summary statistics)
    # ==========================================================================

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
    def test_biogrid_summary_attributes(self):
        """Test BioGRID summary entry has expected statistics."""
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

        # Check required summary fields (no embedded interactions array)
        required_fields = ["biogrid_id", "interaction_count", "unique_partners"]
        missing = [f for f in required_fields if f not in biogrid_attr]

        if missing:
            return False, f"BioGRID entry missing fields: {missing}"

        # Verify interaction_count is positive
        count = biogrid_attr.get("interaction_count", 0)
        if count <= 0:
            return False, f"interaction_count should be positive, got {count}"

        return True, f"BioGRID summary: {count} interactions, {biogrid_attr.get('unique_partners', 0)} unique partners"

    @test
    def test_biogrid_physical_genetic_counts(self):
        """Test BioGRID entry has physical and genetic interaction counts."""
        data = self.runner.lookup(self.test_biogrid_id)

        if not data or not data.get("results"):
            return False, f"No results for BioGRID ID {self.test_biogrid_id}"

        biogrid_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid"),
            None
        )

        if not biogrid_entry:
            return False, f"No biogrid entry found for {self.test_biogrid_id}"

        biogrid_attr = biogrid_entry.get("Attributes", {}).get("Biogrid", {})

        physical = biogrid_attr.get("physical_count", 0)
        genetic = biogrid_attr.get("genetic_count", 0)
        total = biogrid_attr.get("interaction_count", 0)

        # At least one type should be present
        if physical == 0 and genetic == 0:
            return False, "Both physical_count and genetic_count are 0"

        # physical + genetic should roughly equal total (may differ due to other types)
        return True, f"Physical: {physical}, Genetic: {genetic}, Total: {total}"

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
    def test_biogrid_to_interaction_mapping(self):
        """Test mapping from BioGRID summary to biogrid_interaction entries."""
        data = self.runner.lookup(self.test_biogrid_id)

        if not data or not data.get("results"):
            return False, f"No results for BioGRID ID {self.test_biogrid_id}"

        biogrid_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid"),
            None
        )

        if not biogrid_entry:
            return False, f"No biogrid entry found for {self.test_biogrid_id}"

        # Check for biogrid_interaction in entries list
        entries = biogrid_entry.get("entries", [])
        interaction_entries = [
            e for e in entries
            if e.get("dataset_name") == "biogrid_interaction"
        ]

        if not interaction_entries:
            return False, f"No biogrid_interaction mappings found for {self.test_biogrid_id}"

        return True, f"BioGRID -> biogrid_interaction: {len(interaction_entries)} interactions"


class BioGRIDInteractionTests:
    """biogrid_interaction dataset tests (individual interaction records)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner
        self.test_interaction_id = None  # Will be discovered dynamically

    def _find_interaction_id(self):
        """Find a valid interaction ID from biogrid summary."""
        if self.test_interaction_id:
            return self.test_interaction_id

        # Look up a known biogrid entry and get its interactions
        data = self.runner.lookup("112315")  # Known biogrid ID
        if not data or not data.get("results"):
            return None

        biogrid_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid"),
            None
        )

        if not biogrid_entry:
            return None

        # Find biogrid_interaction entries
        entries = biogrid_entry.get("entries", [])
        interaction_entry = next(
            (e for e in entries if e.get("dataset_name") == "biogrid_interaction"),
            None
        )

        if interaction_entry:
            self.test_interaction_id = interaction_entry.get("identifier")

        return self.test_interaction_id

    @test
    def test_interaction_lookup(self):
        """Test looking up a biogrid_interaction entry directly."""
        interaction_id = self._find_interaction_id()
        if not interaction_id:
            return False, "Could not find a valid interaction ID"

        data = self.runner.lookup(interaction_id)
        if not data or not data.get("results"):
            return False, f"No results for interaction ID {interaction_id}"

        # Check for biogrid_interaction entry
        interaction_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid_interaction"),
            None
        )

        if not interaction_entry:
            return False, f"No biogrid_interaction entry found for {interaction_id}"

        return True, f"biogrid_interaction lookup works for {interaction_id}"

    @test
    def test_interaction_has_both_interactors(self):
        """Test biogrid_interaction entry has both interactor IDs."""
        interaction_id = self._find_interaction_id()
        if not interaction_id:
            return False, "Could not find a valid interaction ID"

        data = self.runner.lookup(interaction_id)
        if not data or not data.get("results"):
            return False, f"No results for interaction ID {interaction_id}"

        interaction_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid_interaction"),
            None
        )

        if not interaction_entry:
            return False, f"No biogrid_interaction entry found for {interaction_id}"

        attrs = interaction_entry.get("Attributes", {}).get("BiogridInteraction", {})

        interactor_a = attrs.get("interactor_a_id") or attrs.get("interactor_a_symbol")
        interactor_b = attrs.get("interactor_b_id") or attrs.get("interactor_b_symbol")

        if not interactor_a:
            return False, "Missing interactor_a information"

        if not interactor_b:
            return False, "Missing interactor_b information"

        return True, f"Interaction {interaction_id}: {interactor_a} <-> {interactor_b}"

    @test
    def test_interaction_experimental_system(self):
        """Test biogrid_interaction has experimental_system and type."""
        interaction_id = self._find_interaction_id()
        if not interaction_id:
            return False, "Could not find a valid interaction ID"

        data = self.runner.lookup(interaction_id)
        if not data or not data.get("results"):
            return False, f"No results for interaction ID {interaction_id}"

        interaction_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid_interaction"),
            None
        )

        if not interaction_entry:
            return False, f"No biogrid_interaction entry found for {interaction_id}"

        attrs = interaction_entry.get("Attributes", {}).get("BiogridInteraction", {})

        exp_system = attrs.get("experimental_system")
        exp_type = attrs.get("experimental_system_type")

        if not exp_system:
            return False, "Missing experimental_system"

        if not exp_type:
            return False, "Missing experimental_system_type"

        valid_types = ["physical", "genetic"]
        if exp_type not in valid_types:
            return False, f"Invalid experimental_system_type: {exp_type}"

        return True, f"Experimental system: {exp_system} ({exp_type})"

    @test
    def test_interaction_throughput(self):
        """Test biogrid_interaction has throughput field."""
        interaction_id = self._find_interaction_id()
        if not interaction_id:
            return False, "Could not find a valid interaction ID"

        data = self.runner.lookup(interaction_id)
        if not data or not data.get("results"):
            return False, f"No results for interaction ID {interaction_id}"

        interaction_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid_interaction"),
            None
        )

        if not interaction_entry:
            return False, f"No biogrid_interaction entry found for {interaction_id}"

        attrs = interaction_entry.get("Attributes", {}).get("BiogridInteraction", {})

        throughput = attrs.get("throughput", "")
        valid_values = ["Low Throughput", "High Throughput", "Both", ""]

        if throughput and throughput not in valid_values:
            return False, f"Invalid throughput value: {throughput}"

        return True, f"Throughput: {throughput or '(not specified)'}"

    @test
    def test_interaction_to_pubmed(self):
        """Test biogrid_interaction links to PubMed."""
        interaction_id = self._find_interaction_id()
        if not interaction_id:
            return False, "Could not find a valid interaction ID"

        data = self.runner.lookup(interaction_id)
        if not data or not data.get("results"):
            return False, f"No results for interaction ID {interaction_id}"

        interaction_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid_interaction"),
            None
        )

        if not interaction_entry:
            return False, f"No biogrid_interaction entry found for {interaction_id}"

        # Check for pubmed in entries
        entries = interaction_entry.get("entries", [])
        pubmed_entries = [e for e in entries if e.get("dataset_name") == "pubmed"]

        if not pubmed_entries:
            return False, f"No PubMed cross-references for interaction {interaction_id}"

        return True, f"Interaction {interaction_id} links to {len(pubmed_entries)} PubMed reference(s)"

    @test
    def test_interaction_to_uniprot(self):
        """Test biogrid_interaction links to UniProt proteins."""
        interaction_id = self._find_interaction_id()
        if not interaction_id:
            return False, "Could not find a valid interaction ID"

        data = self.runner.lookup(interaction_id)
        if not data or not data.get("results"):
            return False, f"No results for interaction ID {interaction_id}"

        interaction_entry = next(
            (entry for entry in data.get("results", [])
             if entry.get("dataset_name") == "biogrid_interaction"),
            None
        )

        if not interaction_entry:
            return False, f"No biogrid_interaction entry found for {interaction_id}"

        # Check for uniprot in entries
        entries = interaction_entry.get("entries", [])
        uniprot_entries = [e for e in entries if e.get("dataset_name") == "uniprot"]

        if not uniprot_entries:
            return False, f"No UniProt cross-references for interaction {interaction_id}"

        return True, f"Interaction {interaction_id} links to {len(uniprot_entries)} UniProt protein(s)"


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

    # Add biogrid (summary) custom tests
    biogrid_tests = BioGRIDTests(runner)
    for test_method in [
        biogrid_tests.test_biogrid_basic_lookup,
        biogrid_tests.test_biogrid_summary_attributes,
        biogrid_tests.test_biogrid_physical_genetic_counts,
        biogrid_tests.test_biogrid_to_entrez_mapping,
        biogrid_tests.test_biogrid_to_pubmed_mapping,
        biogrid_tests.test_biogrid_to_interaction_mapping,
    ]:
        runner.add_custom_test(test_method)

    # Add biogrid_interaction custom tests
    interaction_tests = BioGRIDInteractionTests(runner)
    for test_method in [
        interaction_tests.test_interaction_lookup,
        interaction_tests.test_interaction_has_both_interactors,
        interaction_tests.test_interaction_experimental_system,
        interaction_tests.test_interaction_throughput,
        interaction_tests.test_interaction_to_pubmed,
        interaction_tests.test_interaction_to_uniprot,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
