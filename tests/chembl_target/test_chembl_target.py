#!/usr/bin/env python3
"""
ChEMBL Target Test Suite

Tests chembl_target dataset processing using the common test framework.
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


class ChEMBLTargetTests:
    """ChEMBL Target custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_target_with_name(self):
        """Check target has preferred name"""
        # Find a target with pref_name
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("pref_name")),
            None
        )
        if not entry:
            return False, "No target with pref_name in reference"

        target_id = entry["target_chembl_id"]
        pref_name = entry["pref_name"][:60]

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} has name: {pref_name}..."

    @test
    def test_target_with_type(self):
        """Check target has type"""
        # Find a target with type
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_type")),
            None
        )
        if not entry:
            return False, "No target with type in reference"

        target_id = entry["target_chembl_id"]
        target_type = entry["target_type"]

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} has type: {target_type}"

    @test
    def test_target_with_organism(self):
        """Check target has organism"""
        # Find a target with organism
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("organism")),
            None
        )
        if not entry:
            return False, "No target with organism in reference"

        target_id = entry["target_chembl_id"]
        organism = entry["organism"]

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} for organism: {organism}"

    @test
    def test_target_with_components(self):
        """Check target has target components"""
        # Find a target with components
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_components") and len(e["target_components"]) > 0),
            None
        )
        if not entry:
            return False, "No target with components in reference"

        target_id = entry["target_chembl_id"]
        num_components = len(entry["target_components"])

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} has {num_components} component(s)"

    @test
    def test_single_protein_target(self):
        """Check single protein target"""
        # Find a single protein target
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_type") == "SINGLE PROTEIN"),
            None
        )
        if not entry:
            return False, "No SINGLE PROTEIN target in reference"

        target_id = entry["target_chembl_id"]

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} is SINGLE PROTEIN type"

    @test
    def test_target_with_taxonomy(self):
        """Check target has taxonomy ID"""
        # Find a target with tax_id
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("tax_id")),
            None
        )
        if not entry:
            return False, "No target with tax_id in reference"

        target_id = entry["target_chembl_id"]
        tax_id = entry["tax_id"]

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} has taxonomy: {tax_id}"

    @test
    def test_component_with_accession(self):
        """Check target component has UniProt accession"""
        # Find a target with component that has accession
        entry = None
        component = None
        for e in self.runner.reference_data:
            if e.get("target_components"):
                for comp in e["target_components"]:
                    if comp.get("accession"):
                        entry = e
                        component = comp
                        break
                if entry:
                    break

        if not entry:
            return False, "No component with accession in reference"

        target_id = entry["target_chembl_id"]
        accession = component["accession"]

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} component has accession: {accession}"

    @test
    def test_component_with_description(self):
        """Check target component has description"""
        # Find a target with component that has description
        entry = None
        component = None
        for e in self.runner.reference_data:
            if e.get("target_components"):
                for comp in e["target_components"]:
                    if comp.get("component_description"):
                        entry = e
                        component = comp
                        break
                if entry:
                    break

        if not entry:
            return False, "No component with description in reference"

        target_id = entry["target_chembl_id"]
        description = component["component_description"][:50]

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} component: {description}..."

    @test
    def test_component_with_synonyms(self):
        """Check target component has synonyms"""
        # Find a target with component that has synonyms
        entry = None
        component = None
        for e in self.runner.reference_data:
            if e.get("target_components"):
                for comp in e["target_components"]:
                    if comp.get("target_component_synonyms") and len(comp["target_component_synonyms"]) > 0:
                        entry = e
                        component = comp
                        break
                if entry:
                    break

        if not entry:
            return False, "No component with synonyms in reference"

        target_id = entry["target_chembl_id"]
        num_synonyms = len(component["target_component_synonyms"])

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} component has {num_synonyms} synonym(s)"

    @test
    def test_component_with_xrefs(self):
        """Check target component has cross-references"""
        # Find a target with component that has xrefs
        entry = None
        component = None
        for e in self.runner.reference_data:
            if e.get("target_components"):
                for comp in e["target_components"]:
                    if comp.get("target_component_xrefs") and len(comp["target_component_xrefs"]) > 0:
                        entry = e
                        component = comp
                        break
                if entry:
                    break

        if not entry:
            return False, "No component with xrefs in reference"

        target_id = entry["target_chembl_id"]
        num_xrefs = len(component["target_component_xrefs"])

        data = self.runner.lookup(target_id)

        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} component has {num_xrefs} xref(s)"


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
    custom_tests = ChEMBLTargetTests(runner)
    for test_method in [
        custom_tests.test_target_with_name,
        custom_tests.test_target_with_type,
        custom_tests.test_target_with_organism,
        custom_tests.test_target_with_components,
        custom_tests.test_single_protein_target,
        custom_tests.test_target_with_taxonomy,
        custom_tests.test_component_with_accession,
        custom_tests.test_component_with_description,
        custom_tests.test_component_with_synonyms,
        custom_tests.test_component_with_xrefs
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
