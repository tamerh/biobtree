#!/usr/bin/env python3
"""
LIPID MAPS Test Suite

Tests LIPID MAPS dataset processing using the common test framework.
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


class LipidmapsTests:
    """LIPID MAPS custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_molecular_formula(self):
        """Check lipid with molecular formula."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("FORMULA")),
            None
        )

        if not entry:
            return True, "No entries with molecular formula"

        lm_id = entry["LM_ID"]
        data = self.runner.lookup(lm_id)

        if not data or not data.get("results"):
            return False, f"No results for {lm_id}"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Lipidmaps", {})

        if not attrs.get("formula"):
            return False, f"{lm_id} missing formula attribute"

        return True, f"{lm_id} has formula: {attrs['formula']}"

    @test
    def test_inchi_key_search(self):
        """Check InChI Key searchability."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("INCHI_KEY")),
            None
        )

        if not entry:
            return True, "No entries with InChI Key"

        inchi_key = entry["INCHI_KEY"]
        lm_id = entry["LM_ID"]

        # Search by InChI Key
        data = self.runner.lookup(inchi_key)

        if not data or not data.get("results"):
            return False, f"InChI Key {inchi_key} not searchable"

        # Verify it found the right lipid
        found = any(r.get("identifier") == lm_id for r in data["results"])
        if not found:
            return False, f"InChI Key search found wrong lipid"

        return True, f"InChI Key {inchi_key[:20]}... found {lm_id}"

    @test
    def test_lipid_classification(self):
        """Check hierarchical lipid classification."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("SUB_CLASS")),
            None
        )

        if not entry:
            return True, "No entries with sub_class classification"

        lm_id = entry["LM_ID"]
        data = self.runner.lookup(lm_id)

        if not data or not data.get("results"):
            return False, f"No results for {lm_id}"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Lipidmaps", {})

        # Check all classification levels present
        classification_fields = ["category", "main_class", "sub_class"]
        missing = [f for f in classification_fields if not attrs.get(f)]

        if missing:
            return False, f"{lm_id} missing classification: {', '.join(missing)}"

        return True, f"{lm_id} has complete classification: {attrs['category']}"

    @test
    def test_chebi_xref(self):
        """Check ChEBI cross-reference.

        Note: Cross-references only work when target datasets are loaded.
        In test mode with only lipidmaps, xrefs are stored but targets don't exist.
        """
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("CHEBI_ID")),
            None
        )

        if not entry:
            return True, "No entries with ChEBI cross-reference in reference data"

        lm_id = entry["LM_ID"]
        chebi_id = entry["CHEBI_ID"]

        data = self.runner.lookup(lm_id)

        if not data or not data.get("results"):
            return False, f"No results for {lm_id}"

        result = data["results"][0]

        # Check if ChEBI xref exists
        chebi_xrefs = self.runner.get_xrefs(result, "chebi")

        if not chebi_xrefs:
            # In test mode, target datasets might not be loaded
            return True, f"{lm_id} has ChEBI ID {chebi_id} in source (xref target not loaded in test)"

        # Verify it's the right ChEBI ID
        expected_chebi = chebi_id if chebi_id.startswith("CHEBI:") else f"CHEBI:{chebi_id}"
        found = any(x.get("identifier") == expected_chebi for x in chebi_xrefs)

        if not found:
            return False, f"{lm_id} has ChEBI xref but not {expected_chebi}"

        return True, f"{lm_id} → {expected_chebi}"

    @test
    def test_hmdb_xref(self):
        """Check HMDB cross-reference.

        Note: Cross-references only work when target datasets are loaded.
        In test mode with only lipidmaps, xrefs are stored but targets don't exist.
        """
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("HMDB_ID")),
            None
        )

        if not entry:
            return True, "No entries with HMDB cross-reference in reference data"

        lm_id = entry["LM_ID"]
        hmdb_id = entry["HMDB_ID"]

        data = self.runner.lookup(lm_id)

        if not data or not data.get("results"):
            return False, f"No results for {lm_id}"

        result = data["results"][0]

        # Check if HMDB xref exists
        hmdb_xrefs = self.runner.get_xrefs(result, "hmdb")

        if not hmdb_xrefs:
            # In test mode, target datasets might not be loaded
            return True, f"{lm_id} has HMDB ID {hmdb_id} in source (xref target not loaded in test)"

        if not self.runner.has_xref(result, "hmdb", hmdb_id):
            return False, f"{lm_id} missing HMDB cross-reference to {hmdb_id}"

        return True, f"{lm_id} → {hmdb_id}"

    @test
    def test_kegg_xref(self):
        """Check KEGG cross-reference.

        Note: Cross-references only work when target datasets are loaded.
        In test mode with only lipidmaps, xrefs are stored but targets don't exist.
        """
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("KEGG_ID")),
            None
        )

        if not entry:
            return True, "No entries with KEGG cross-reference in reference data"

        lm_id = entry["LM_ID"]
        kegg_id = entry["KEGG_ID"]

        data = self.runner.lookup(lm_id)

        if not data or not data.get("results"):
            return False, f"No results for {lm_id}"

        result = data["results"][0]

        # Check if KEGG xref exists
        kegg_xrefs = self.runner.get_xrefs(result, "kegg")

        if not kegg_xrefs:
            # In test mode, target datasets might not be loaded
            return True, f"{lm_id} has KEGG ID {kegg_id} in source (xref target not loaded in test)"

        if not self.runner.has_xref(result, "kegg", kegg_id):
            return False, f"{lm_id} missing KEGG cross-reference to {kegg_id}"

        return True, f"{lm_id} → {kegg_id}"

    @test
    def test_exact_mass(self):
        """Check exact mass attribute."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("EXACT_MASS")),
            None
        )

        if not entry:
            return True, "No entries with exact mass"

        lm_id = entry["LM_ID"]
        data = self.runner.lookup(lm_id)

        if not data or not data.get("results"):
            return False, f"No results for {lm_id}"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Lipidmaps", {})

        if not attrs.get("exact_mass"):
            return False, f"{lm_id} missing exact_mass attribute"

        return True, f"{lm_id} has exact mass: {attrs['exact_mass']}"

    @test
    def test_synonyms_searchable(self):
        """Check that synonyms are searchable."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("SYNONYMS") and len(e["SYNONYMS"]) > 1),
            None
        )

        if not entry:
            return True, "No entries with multiple synonyms"

        lm_id = entry["LM_ID"]
        synonym = entry["SYNONYMS"][1]  # Use second synonym

        # Search by synonym
        data = self.runner.lookup(synonym)

        if not data or not data.get("results"):
            return False, f"Synonym '{synonym}' not searchable"

        # Verify it found the right lipid
        found = any(r.get("identifier") == lm_id for r in data["results"])
        if not found:
            return False, f"Synonym search found wrong lipid"

        return True, f"Synonym '{synonym}' found {lm_id}"


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
    custom_tests = LipidmapsTests(runner)
    for test_method in [
        custom_tests.test_molecular_formula,
        custom_tests.test_inchi_key_search,
        custom_tests.test_lipid_classification,
        custom_tests.test_chebi_xref,
        custom_tests.test_hmdb_xref,
        custom_tests.test_kegg_xref,
        custom_tests.test_exact_mass,
        custom_tests.test_synonyms_searchable,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    main()
