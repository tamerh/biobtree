#!/usr/bin/env python3
"""
ChEMBL Molecule Test Suite

Tests chembl_molecule dataset processing using the common test framework.
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


class ChEMBLMoleculeTests:
    """ChEMBL Molecule custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_molecule_with_smiles(self):
        """Check molecule has SMILES structure"""
        # Find a molecule with SMILES
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("molecule_structures", {}).get("canonical_smiles")),
            None
        )
        if not entry:
            return False, "No molecule with SMILES in reference"

        mol_id = entry["molecule_chembl_id"]
        smiles = entry["molecule_structures"]["canonical_smiles"][:30]

        data = self.runner.lookup(mol_id)

        if not data or not data.get("results"):
            return False, f"No results for {mol_id}"

        return True, f"{mol_id} has SMILES: {smiles}..."

    @test
    def test_molecule_with_inchi(self):
        """Check molecule has InChI identifier"""
        # Find a molecule with InChI
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("molecule_structures", {}).get("standard_inchi")),
            None
        )
        if not entry:
            return False, "No molecule with InChI in reference"

        mol_id = entry["molecule_chembl_id"]
        inchi_key = entry["molecule_structures"].get("standard_inchi_key", "unknown")

        data = self.runner.lookup(mol_id)

        if not data or not data.get("results"):
            return False, f"No results for {mol_id}"

        return True, f"{mol_id} has InChI Key: {inchi_key}"

    @test
    def test_molecule_with_properties(self):
        """Check molecule has molecular properties"""
        # Find a molecule with properties
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("molecule_properties")),
            None
        )
        if not entry:
            return False, "No molecule with properties in reference"

        mol_id = entry["molecule_chembl_id"]
        props = entry["molecule_properties"]
        mw = props.get("full_mwt", "unknown")

        data = self.runner.lookup(mol_id)

        if not data or not data.get("results"):
            return False, f"No results for {mol_id}"

        return True, f"{mol_id} has MW: {mw}"

    @test
    def test_molecule_with_formula(self):
        """Check molecule has molecular formula"""
        # Find a molecule with formula
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("molecule_properties", {}).get("full_molformula")),
            None
        )
        if not entry:
            return False, "No molecule with formula in reference"

        mol_id = entry["molecule_chembl_id"]
        formula = entry["molecule_properties"]["full_molformula"]

        data = self.runner.lookup(mol_id)

        if not data or not data.get("results"):
            return False, f"No results for {mol_id}"

        return True, f"{mol_id} has formula: {formula}"

    @test
    def test_small_molecule_type(self):
        """Check small molecule type"""
        # Find a small molecule
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("molecule_type") == "Small molecule"),
            None
        )
        if not entry:
            return False, "No small molecule in reference"

        mol_id = entry["molecule_chembl_id"]
        mol_type = entry["molecule_type"]

        data = self.runner.lookup(mol_id)

        if not data or not data.get("results"):
            return False, f"No results for {mol_id}"

        return True, f"{mol_id} is type: {mol_type}"


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
    custom_tests = ChEMBLMoleculeTests(runner)
    for test_method in [
        custom_tests.test_molecule_with_smiles,
        custom_tests.test_molecule_with_inchi,
        custom_tests.test_molecule_with_properties,
        custom_tests.test_molecule_with_formula,
        custom_tests.test_small_molecule_type
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
