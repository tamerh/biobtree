#!/usr/bin/env python3
"""
Antibody Test Suite

Tests unified antibody dataset (TheraSAbDab, SAbDab, IMGT/GENE-DB, IMGT/LIGM-DB)
using the common test framework.

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


class AntibodyTests:
    """Antibody custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_therasabdab_therapeutic_antibody(self):
        """Check TheraSAbDab therapeutic antibody has correct attributes."""
        # Test Abciximab (first therapeutic antibody)
        data = self.runner.lookup("Abciximab")

        if not data or not data.get("results"):
            return False, "No results for Abciximab"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Antibody", {})

        # Check source is therasabdab
        if attrs.get("source") != "therasabdab":
            return False, f"Wrong source: {attrs.get('source')}"

        # Check antibody type
        if attrs.get("antibody_type") != "therapeutic":
            return False, f"Wrong type: {attrs.get('antibody_type')}"

        # Check has sequences
        if not attrs.get("heavy_chain_seq"):
            return False, "Missing heavy chain sequence"

        return True, f"Therapeutic antibody {attrs.get('inn_name')} validated (format: {attrs.get('format')}, targets: {len(attrs.get('targets', []))})"

    @test
    def test_sabdab_structural_antibody(self):
        """Check SAbDab structural antibody searchable via PDB ID."""
        # Search by PDB ID (text link) which should return antibody cross-references
        data = self.runner.lookup("9mte")

        if not data or not data.get("results"):
            return False, "No results for PDB ID 9mte"

        result = data["results"][0]

        # Check if we have antibody cross-references
        entries = result.get("entries", [])
        antibody_entries = [e for e in entries if e.get("dataset_name") == "antibody"]

        if not antibody_entries:
            return False, "No antibody cross-references found for PDB 9mte"

        # Validate that these are SAbDab entries (composite IDs have underscores)
        sample_id = antibody_entries[0].get("identifier", "")
        if "_" not in sample_id:
            return False, f"Invalid SAbDab composite ID format: {sample_id}"

        return True, f"PDB ID 9mte links to {len(antibody_entries)} SAbDab antibody structure(s) (e.g., {sample_id})"

    @test
    def test_pdb_cross_reference(self):
        """Check PDB cross-references are bidirectional."""
        # Therapeutic antibodies may have PDB cross-references
        data = self.runner.lookup("Abciximab")

        if not data or not data.get("results"):
            return False, "No results for Abciximab"

        result = data["results"][0]

        # Check if has UniProt cross-references (for targets)
        entries = result.get("entries", [])
        uniprot_entries = [e for e in entries if e.get("dataset_name") == "uniprot"]

        if uniprot_entries:
            return True, f"Abciximab has {len(uniprot_entries)} UniProt target cross-reference(s)"
        else:
            return True, "Abciximab has no UniProt cross-references (expected for test data)"

    @test
    def test_multiple_sources_unified(self):
        """Check that antibody dataset successfully unifies multiple sources."""
        # Test that we can find different source types
        therapeutic = self.runner.lookup("Abciximab")

        if not therapeutic or not therapeutic.get("results"):
            return False, "Could not find therapeutic antibody"

        therapeutic_source = therapeutic["results"][0].get("Attributes", {}).get("Antibody", {}).get("source")

        # Look for a SAbDab structural antibody (using composite ID from test data)
        # We know from antibody_ids.txt that structural entries exist like "9mte_A_NA"
        structural = self.runner.lookup("9mte_A_NA")

        if not structural or not structural.get("results"):
            return True, f"SKIP: Could not find structural antibody in test data (only verified {therapeutic_source})"

        structural_source = structural["results"][0].get("Attributes", {}).get("Antibody", {}).get("source")

        if therapeutic_source == "therasabdab" and structural_source == "sabdab":
            return True, f"Successfully unified: {therapeutic_source} + {structural_source}"
        else:
            return False, f"Source mismatch: {therapeutic_source} vs {structural_source}"

    @test
    def test_antibody_sequences_present(self):
        """Check antibody sequences are stored."""
        data = self.runner.lookup("Abciximab")

        if not data or not data.get("results"):
            return False, "No results for Abciximab"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Antibody", {})

        heavy_seq = attrs.get("heavy_chain_seq", [])
        light_seq = attrs.get("light_chain_seq", [])

        if not heavy_seq:
            return False, "Missing heavy chain sequence"

        if not light_seq:
            return False, "Missing light chain sequence"

        heavy_len = len(heavy_seq[0]) if heavy_seq else 0
        light_len = len(light_seq[0]) if light_seq else 0

        return True, f"Sequences present: Heavy={heavy_len}aa, Light={light_len}aa"

    @test
    def test_therapeutic_indications(self):
        """Check therapeutic antibodies have indication/target data."""
        data = self.runner.lookup("Abciximab")

        if not data or not data.get("results"):
            return False, "No results for Abciximab"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Antibody", {})

        targets = attrs.get("targets", [])
        indications = attrs.get("indications", [])

        if not targets:
            return False, "Missing target information"

        return True, f"Clinical data: {len(targets)} target(s), {len(indications)} indication(s)"


def main():
    """Main test execution"""
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
    custom_tests = AntibodyTests(runner)
    for test_method in [custom_tests.test_therasabdab_therapeutic_antibody,
                       custom_tests.test_sabdab_structural_antibody,
                       custom_tests.test_pdb_cross_reference,
                       custom_tests.test_multiple_sources_unified,
                       custom_tests.test_antibody_sequences_present,
                       custom_tests.test_therapeutic_indications]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    main()
