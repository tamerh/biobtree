#!/usr/bin/env python3
"""
HMDB Test Suite

Tests HMDB dataset processing using the common test framework.
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


class HMDBTests:
    """HMDB custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_chemical_formula(self):
        """Check metabolite with chemical formula."""
        # Find an entry with chemical formula
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("chemical_formula")),
            None
        )

        if not entry:
            return True, "No entries with chemical formula"

        hmdb_id = entry["accession"]
        data = self.runner.lookup(hmdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {hmdb_id}"

        return True, f"{hmdb_id} has chemical formula"

    @test
    def test_smiles_notation(self):
        """Check metabolite with SMILES notation."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("smiles")),
            None
        )

        if not entry:
            return True, "No entries with SMILES"

        hmdb_id = entry["accession"]
        data = self.runner.lookup(hmdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {hmdb_id}"

        return True, f"{hmdb_id} has SMILES notation"

    @test
    def test_molecular_weight(self):
        """Check metabolite with molecular weight."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("average_molecular_weight")),
            None
        )

        if not entry:
            return True, "No entries with molecular weight"

        hmdb_id = entry["accession"]
        data = self.runner.lookup(hmdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {hmdb_id}"

        return True, f"{hmdb_id} has molecular weight"

    @test
    def test_disease_associations(self):
        """Check metabolite with disease associations."""
        # Find entry with diseases
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("diseases") and e["diseases"].get("disease")),
            None
        )

        if not entry:
            return True, "No entries with disease associations"

        hmdb_id = entry["accession"]
        data = self.runner.lookup(hmdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {hmdb_id}"

        return True, f"{hmdb_id} has disease associations"

    @test
    def test_pathway_information(self):
        """Check metabolite with pathway information."""
        # Find entry with pathways
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("biological_properties") and
                e["biological_properties"].get("pathways") and
                e["biological_properties"]["pathways"].get("pathway")),
            None
        )

        if not entry:
            return True, "No entries with pathway information"

        hmdb_id = entry["accession"]
        data = self.runner.lookup(hmdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {hmdb_id}"

        return True, f"{hmdb_id} has pathway information"

    @test
    def test_biospecimen_locations(self):
        """Check metabolite with biospecimen locations."""
        # Find entry with biospecimen locations
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("biological_properties") and
                e["biological_properties"].get("biospecimen_locations") and
                e["biological_properties"]["biospecimen_locations"].get("biospecimen")),
            None
        )

        if not entry:
            return True, "No entries with biospecimen locations"

        hmdb_id = entry["accession"]
        data = self.runner.lookup(hmdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {hmdb_id}"

        return True, f"{hmdb_id} has biospecimen locations"

    @test
    def test_kegg_cross_reference(self):
        """Check metabolite with KEGG cross-reference."""
        # Find entry with KEGG ID
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("kegg_id")),
            None
        )

        if not entry:
            return True, "No entries with KEGG cross-reference"

        hmdb_id = entry["accession"]

        # Check if KEGG xref exists
        has_kegg = self.runner.check_xref(hmdb_id, "kegg")

        if not has_kegg:
            return False, f"KEGG cross-reference not found for {hmdb_id}"

        return True, f"{hmdb_id} has KEGG cross-reference"

    @test
    def test_inchi_key_search(self):
        """Check searchability by InChI Key."""
        # Find entry with InChI Key
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("inchikey")),
            None
        )

        if not entry:
            return True, "No entries with InChI Key"

        inchi_key = entry["inchikey"]

        # Search by InChI Key
        data = self.runner.lookup(inchi_key)

        if not data or not data.get("results"):
            return False, f"InChI Key search failed: {inchi_key}"

        return True, f"InChI Key search successful: {inchi_key[:20]}..."


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
    custom_tests = HMDBTests(runner)
    for test_method in [
        custom_tests.test_chemical_formula,
        custom_tests.test_smiles_notation,
        custom_tests.test_molecular_weight,
        custom_tests.test_disease_associations,
        custom_tests.test_pathway_information,
        custom_tests.test_biospecimen_locations,
        custom_tests.test_kegg_cross_reference,
        custom_tests.test_inchi_key_search,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
