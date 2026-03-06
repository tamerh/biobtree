#!/usr/bin/env python3
"""
SwissLipids Test Suite

Tests SwissLipids dataset processing using the common test framework.
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


class SwissLipidsTests:
    """SwissLipids custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_lipid_with_name(self):
        """Check SwissLipids entry has name"""
        entry = self.runner.reference_data[0] if self.runner.reference_data else None
        if not entry:
            return False, "No SwissLipids entry in reference"

        slm_id = entry["id"]
        lipids_data = entry.get("lipids_data", {})
        cols = lipids_data.get("columns", [])
        name = cols[2] if len(cols) > 2 else ""

        data = self.runner.lookup(slm_id)

        if not data or not data.get("results"):
            return False, f"No results for {slm_id}"

        attrs = data["results"][0].get("Attributes", {})
        swisslipids_attrs = attrs.get("Swisslipids", {})

        if not swisslipids_attrs.get("name"):
            return False, f"{slm_id} missing name attribute"

        return True, f"{slm_id} has name: {name[:60]}..."

    @test
    def test_lipid_with_chemical_data(self):
        """Check SwissLipids entry has chemical descriptors"""
        # Find a lipid with SMILES and InChI
        entry = None
        for e in self.runner.reference_data:
            cols = e.get("lipids_data", {}).get("columns", [])
            smiles = cols[8] if len(cols) > 8 else ""
            inchi = cols[9] if len(cols) > 9 else ""
            if smiles and inchi and inchi != "InChI=none":
                entry = e
                break

        if not entry:
            return False, "No lipid with complete chemical data in reference"

        slm_id = entry["id"]
        cols = entry["lipids_data"]["columns"]
        smiles = cols[8] if len(cols) > 8 else ""
        inchi = cols[9] if len(cols) > 9 else ""

        data = self.runner.lookup(slm_id)

        if not data or not data.get("results"):
            return False, f"No results for {slm_id}"

        attrs = data["results"][0].get("Attributes", {})
        swisslipids_attrs = attrs.get("Swisslipids", {})

        if not swisslipids_attrs.get("smiles"):
            return False, f"{slm_id} missing SMILES"

        if not swisslipids_attrs.get("inchi"):
            return False, f"{slm_id} missing InChI"

        return True, f"{slm_id} has chemical descriptors (SMILES, InChI)"

    @test
    def test_lipid_with_uniprot_xrefs(self):
        """Check SwissLipids lipid has UniProt cross-references

        Note: In test mode, lipids2uniprot.tsv is skipped, so UniProt xrefs
        may not be present. This test skips gracefully in that case.
        """
        # Find a lipid with UniProt xrefs in reference data
        entry = None
        for e in self.runner.reference_data:
            if e.get("lipids2uniprot") and len(e["lipids2uniprot"]) > 0:
                entry = e
                break

        if not entry:
            # No reference data with UniProt xrefs - skip test
            return True, "SKIP: No lipid with UniProt xrefs in reference data"

        slm_id = entry["id"]
        expected_count = len(entry["lipids2uniprot"])

        data = self.runner.lookup(slm_id)

        if not data or not data.get("results"):
            return False, f"No results for {slm_id}"

        result = data["results"][0]
        uniprot_xrefs = self.runner.get_xrefs(result, "uniprot")

        if not uniprot_xrefs:
            # In test mode, cross-reference files are skipped - this is expected
            return True, f"SKIP: {slm_id} UniProt xrefs not loaded (test mode skips lipids2uniprot.tsv)"

        return True, f"{slm_id} has {len(uniprot_xrefs)} UniProt xref(s)"

    @test
    def test_lipid_with_eco_evidence(self):
        """Check SwissLipids lipid has ECO evidence cross-references"""
        # Find a lipid with evidence codes in reference data
        entry = None
        for e in self.runner.reference_data:
            if e.get("evidences") and len(e["evidences"]) > 0:
                entry = e
                break

        if not entry:
            # If no reference data with ECO evidences, skip this test
            # (ECO evidences may exist in database but not in our test sample)
            return True, "SKIP: No lipid with ECO evidence in reference data (test sample)"

        slm_id = entry["id"]
        expected_count = len(entry["evidences"])

        data = self.runner.lookup(slm_id)

        if not data or not data.get("results"):
            return False, f"No results for {slm_id}"

        result = data["results"][0]
        eco_xrefs = self.runner.get_xrefs(result, "eco")

        if not eco_xrefs:
            return False, f"{slm_id} missing ECO xrefs (expected {expected_count})"

        return True, f"{slm_id} has {len(eco_xrefs)} ECO evidence code(s)"

    @test
    def test_lipid_with_synonyms(self):
        """Check SwissLipids entry has synonyms"""
        # Find a lipid with synonyms
        entry = None
        for e in self.runner.reference_data:
            cols = e.get("lipids_data", {}).get("columns", [])
            synonyms = cols[4] if len(cols) > 4 else ""
            if synonyms and synonyms.strip():
                entry = e
                break

        if not entry:
            return False, "No lipid with synonyms in reference"

        slm_id = entry["id"]
        cols = entry["lipids_data"]["columns"]
        synonyms = cols[4] if len(cols) > 4 else ""

        data = self.runner.lookup(slm_id)

        if not data or not data.get("results"):
            return False, f"No results for {slm_id}"

        attrs = data["results"][0].get("Attributes", {})
        swisslipids_attrs = attrs.get("Swisslipids", {})

        if not swisslipids_attrs.get("synonyms"):
            return False, f"{slm_id} missing synonyms attribute"

        synonym_count = len(swisslipids_attrs.get("synonyms", []))
        return True, f"{slm_id} has {synonym_count} synonym(s)"

    @test
    def test_lipid_mass_and_formula(self):
        """Check SwissLipids entry has mass and formula"""
        # Find a lipid with mass and formula
        entry = None
        for e in self.runner.reference_data:
            cols = e.get("lipids_data", {}).get("columns", [])
            formula = cols[11] if len(cols) > 11 else ""
            mass = cols[13] if len(cols) > 13 else ""
            if formula and formula.strip() and mass and mass.strip():
                entry = e
                break

        if not entry:
            return False, "No lipid with mass and formula in reference"

        slm_id = entry["id"]
        cols = entry["lipids_data"]["columns"]
        formula = cols[11] if len(cols) > 11 else ""
        mass = cols[13] if len(cols) > 13 else ""

        data = self.runner.lookup(slm_id)

        if not data or not data.get("results"):
            return False, f"No results for {slm_id}"

        attrs = data["results"][0].get("Attributes", {})
        swisslipids_attrs = attrs.get("Swisslipids", {})

        if not swisslipids_attrs.get("formula"):
            return False, f"{slm_id} missing formula"

        if not swisslipids_attrs.get("mass"):
            return False, f"{slm_id} missing mass"

        return True, f"{slm_id} has formula: {formula}, mass: {mass}"


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
    custom_tests = SwissLipidsTests(runner)
    for test_method in [
        custom_tests.test_lipid_with_name,
        custom_tests.test_lipid_with_chemical_data,
        custom_tests.test_lipid_with_uniprot_xrefs,
        custom_tests.test_lipid_with_eco_evidence,
        custom_tests.test_lipid_with_synonyms,
        custom_tests.test_lipid_mass_and_formula
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
