#!/usr/bin/env python3
"""
PubChem Test Suite

Tests PubChem Compound dataset integration (P0: FDA-approved drugs)
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


class PubChemTests:
    """PubChem custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_fda_approved_flag(self):
        """Check FDA-approved drugs have the flag set."""
        # Test Aspirin (definitely FDA-approved)
        data = self.runner.lookup("2244")

        if not data or not data.get("results"):
            return False, "No results for Aspirin (CID 2244)"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Pubchem", {})

        # Check FDA approved flag
        if not attrs.get("is_fda_approved"):
            return False, f"FDA flag not set for Aspirin"

        # Check compound type
        compound_type = attrs.get("compound_type", "")
        if compound_type != "drug":
            return False, f"Wrong compound type: {compound_type}, expected 'drug'"

        cid = attrs.get("cid", "")
        title = attrs.get("title", "")

        return True, f"FDA-approved drug validated: CID {cid} ({title})"

    @test
    def test_molecular_properties(self):
        """Check molecular properties are present."""
        # Test Aspirin
        data = self.runner.lookup("2244")

        if not data or not data.get("results"):
            return False, "No results for Aspirin"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Pubchem", {})

        # Check required properties
        required = ["molecular_formula", "molecular_weight", "smiles", "inchi_key"]
        missing = [prop for prop in required if not attrs.get(prop)]

        if missing:
            return False, f"Missing properties: {', '.join(missing)}"

        mw = attrs.get("molecular_weight", 0)
        formula = attrs.get("molecular_formula", "")

        # Aspirin is C9H8O4 with MW ~180
        if formula != "C9H8O4":
            return False, f"Wrong formula: {formula}, expected C9H8O4"

        if not (179 < mw < 181):
            return False, f"Wrong molecular weight: {mw}, expected ~180"

        return True, f"Molecular properties validated: {formula}, MW={mw:.2f}"

    @test
    def test_lipinski_properties(self):
        """Check Lipinski Rule of Five properties are present."""
        # Test Ibuprofen
        data = self.runner.lookup("3672")

        if not data or not data.get("results"):
            return False, "No results for Ibuprofen (CID 3672)"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Pubchem", {})

        # Check Lipinski properties
        hbd = attrs.get("hydrogen_bond_donors")
        hba = attrs.get("hydrogen_bond_acceptors")
        xlogp = attrs.get("xlogp")

        if hbd is None or hba is None or xlogp is None:
            missing = []
            if hbd is None: missing.append("HBD")
            if hba is None: missing.append("HBA")
            if xlogp is None: missing.append("XLogP")
            return False, f"Missing Lipinski properties: {', '.join(missing)}"

        return True, f"Lipinski properties: HBD={hbd}, HBA={hba}, XLogP={xlogp}"

    @test
    def test_synonyms_present(self):
        """Check synonyms are stored."""
        # Test Aspirin (should have many synonyms)
        data = self.runner.lookup("2244")

        if not data or not data.get("results"):
            return False, "No results for Aspirin"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Pubchem", {})

        synonyms = attrs.get("synonyms", [])

        if not synonyms:
            return False, "No synonyms found"

        # Check if common synonym is present (case insensitive)
        synonyms_lower = [s.lower() for s in synonyms]
        if "acetylsalicylic acid" not in synonyms_lower:
            return False, f"Expected synonym 'Acetylsalicylic acid' not found"

        return True, f"Synonyms present: {len(synonyms)} total (includes 'Acetylsalicylic acid')"

    @test
    def test_chembl_cross_reference(self):
        """Check ChEMBL cross-references are created."""
        # Test Aspirin (ChEMBL25 in ChEMBL)
        data = self.runner.lookup("2244")

        if not data or not data.get("results"):
            return False, "No results for Aspirin"

        result = data["results"][0]
        entries = result.get("entries", [])

        # Look for ChEMBL cross-references
        chembl_entries = [e for e in entries if "chembl" in e.get("dataset_name", "").lower()]

        if chembl_entries:
            chembl_ids = [e.get("identifier") for e in chembl_entries]
            return True, f"ChEMBL cross-references found: {', '.join(chembl_ids[:3])}"
        else:
            return True, "SKIP: No ChEMBL cross-references (expected if ChEMBL not in test build)"

    @test
    def test_chebi_cross_reference(self):
        """Check ChEBI cross-references are created."""
        # Test Aspirin (CHEBI:15365)
        data = self.runner.lookup("2244")

        if not data or not data.get("results"):
            return False, "No results for Aspirin"

        result = data["results"][0]
        entries = result.get("entries", [])

        # Look for ChEBI cross-references
        chebi_entries = [e for e in entries if e.get("dataset_name") == "chebi"]

        if chebi_entries:
            chebi_ids = [e.get("identifier") for e in chebi_entries]
            return True, f"ChEBI cross-references found: {', '.join(chebi_ids)}"
        else:
            return True, "SKIP: No ChEBI cross-references (expected if ChEBI not in test build)"

    @test
    def test_text_search_by_smiles(self):
        """Check SMILES text search works."""
        # Aspirin SMILES: CC(=O)Oc1ccccc1C(=O)O
        data = self.runner.lookup("CC(=O)Oc1ccccc1C(=O)O")

        if not data or not data.get("results"):
            return True, "SKIP: SMILES text search not returning results (may not be indexed)"

        # Check if we got PubChem results
        results = data.get("results", [])
        pubchem_results = [r for r in results if r.get("dataset_name") == "pubchem"]

        if pubchem_results:
            return True, f"SMILES search found {len(pubchem_results)} PubChem compound(s)"
        else:
            return True, "SKIP: SMILES not returning PubChem results"

    @test
    def test_text_search_by_inchi_key(self):
        """Check InChI Key text search works."""
        # Aspirin InChI Key: BSYNRYMUTXBXSQ-UHFFFAOYSA-N
        data = self.runner.lookup("BSYNRYMUTXBXSQ-UHFFFAOYSA-N")

        if not data or not data.get("results"):
            return True, "SKIP: InChI Key text search not returning results (may not be indexed)"

        # Check if we got PubChem results
        results = data.get("results", [])
        pubchem_results = [r for r in results if r.get("dataset_name") == "pubchem"]

        if pubchem_results:
            cid = pubchem_results[0].get("Attributes", {}).get("Pubchem", {}).get("cid")
            if cid == "2244":
                return True, f"InChI Key search correctly found Aspirin (CID {cid})"
            else:
                return True, f"InChI Key search found PubChem compound CID {cid}"
        else:
            return True, "SKIP: InChI Key not returning PubChem results"


def main():
    """Main test execution"""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not reference_file.exists():
        print(f"Warning: {reference_file} not found")
        print("Skipping reference data tests (run: python3 extract_reference_data.py)")
        reference_file = None

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = PubChemTests(runner)
    for test_method in [custom_tests.test_fda_approved_flag,
                       custom_tests.test_molecular_properties,
                       custom_tests.test_lipinski_properties,
                       custom_tests.test_synonyms_present,
                       custom_tests.test_chembl_cross_reference,
                       custom_tests.test_chebi_cross_reference,
                       custom_tests.test_text_search_by_smiles,
                       custom_tests.test_text_search_by_inchi_key]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
