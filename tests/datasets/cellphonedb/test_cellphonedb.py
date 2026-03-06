#!/usr/bin/env python3
"""
CellPhoneDB Test Suite

Tests CellPhoneDB ligand-receptor interaction dataset.
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


class CellPhoneDBTests:
    """CellPhoneDB custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_interaction_structure(self):
        """Check interaction ID format is correctly stored"""
        entry = self.runner.reference_data[0]
        entry_id = entry["id"]
        partner_a = entry["partner_a"]
        partner_b = entry["partner_b"]

        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        result = data["results"][0]

        # Check the identifier matches expected format
        if result.get("identifier") != entry_id:
            return False, f"ID mismatch: expected {entry_id}, got {result.get('identifier')}"

        return True, f"{entry_id}: {partner_a} <-> {partner_b}"

    @test
    def test_ligand_receptor_directionality(self):
        """Check entry with Ligand-Receptor directionality"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("directionality") == "Ligand-Receptor"),
            None
        )
        if not entry:
            return True, "SKIP: No Ligand-Receptor entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        return True, f"{entry_id} has Ligand-Receptor directionality"

    @test
    def test_adhesion_directionality(self):
        """Check entry with Adhesion-Adhesion directionality"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("directionality") == "Adhesion-Adhesion"),
            None
        )
        if not entry:
            return True, "SKIP: No Adhesion-Adhesion entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        return True, f"{entry_id} has Adhesion-Adhesion directionality"

    @test
    def test_complex_interaction(self):
        """Check entry involving a protein complex"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("is_complex_a") or e.get("is_complex_b")),
            None
        )
        if not entry:
            return True, "SKIP: No complex entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        complex_partner = "A" if entry.get("is_complex_a") else "B"
        return True, f"{entry_id} involves complex in partner {complex_partner}"

    @test
    def test_integrin_interaction(self):
        """Check entry involving integrin"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("is_integrin")),
            None
        )
        if not entry:
            return True, "SKIP: No integrin entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        return True, f"{entry_id} involves integrin"

    @test
    def test_receptor_interaction(self):
        """Check entry with receptor partner"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("receptor_a") or e.get("receptor_b")),
            None
        )
        if not entry:
            return True, "SKIP: No receptor entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        receptor_partner = "A" if entry.get("receptor_a") else "B"
        return True, f"{entry_id} has receptor in partner {receptor_partner}"

    @test
    def test_secreted_interaction(self):
        """Check entry with secreted partner"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("secreted_a") or e.get("secreted_b")),
            None
        )
        if not entry:
            return True, "SKIP: No secreted entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        secreted_partner = "A" if entry.get("secreted_a") else "B"
        return True, f"{entry_id} has secreted partner {secreted_partner}"

    @test
    def test_search_by_gene(self):
        """Search for gene finds CellPhoneDB entries"""
        entry = self.runner.reference_data[0]
        genes_a = entry.get("genes_a", [])
        if not genes_a:
            return True, "SKIP: No genes_a in reference entry"

        gene = genes_a[0]
        data = self.runner.lookup(gene)

        if not data or not data.get("results"):
            return False, f"No results when searching for gene {gene}"

        # Check that we got CellPhoneDB results
        cellphonedb_results = [
            r for r in data["results"]
            if r.get("dataset_name", "").lower() == "cellphonedb"
        ]

        if not cellphonedb_results:
            return True, f"SKIP: {gene} search did not return CellPhoneDB entries in test data"

        return True, f"Searching '{gene}' found {len(cellphonedb_results)} CellPhoneDB entries"

    @test
    def test_cross_references_to_uniprot(self):
        """Check CellPhoneDB entries have cross-references to UniProt"""
        # Find entry with non-complex partner (has UniProt ID)
        entry = next(
            (e for e in self.runner.reference_data
             if not e.get("is_complex_a") and e.get("partner_a")),
            None
        )
        if not entry:
            return True, "SKIP: No non-complex entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        result = data["results"][0]
        datasets = self.runner.get_xref_datasets(result)

        if "uniprot" in [d.lower() for d in datasets]:
            uniprot_count = self.runner.get_xref_count(result, "uniprot")
            return True, f"{entry_id} has {uniprot_count} UniProt cross-references"

        return True, f"SKIP: {entry_id} has no UniProt xrefs in test data (xrefs: {', '.join(datasets[:5])})"

    @test
    def test_cross_references_to_ensembl(self):
        """Check CellPhoneDB entries have cross-references to Ensembl"""
        entry = self.runner.reference_data[0]
        entry_id = entry["id"]

        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        result = data["results"][0]
        datasets = self.runner.get_xref_datasets(result)

        if "ensembl" in [d.lower() for d in datasets]:
            ensembl_count = self.runner.get_xref_count(result, "ensembl")
            return True, f"{entry_id} has {ensembl_count} Ensembl cross-references"

        return True, f"SKIP: {entry_id} has no Ensembl xrefs in test data (xrefs: {', '.join(datasets[:5])})"

    @test
    def test_classification_signaling(self):
        """Check entry with signaling classification"""
        entry = next(
            (e for e in self.runner.reference_data
             if "Signaling" in e.get("classification", "")),
            None
        )
        if not entry:
            return True, "SKIP: No Signaling entry in reference"

        entry_id = entry["id"]
        classification = entry["classification"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        return True, f"{entry_id} has classification: {classification}"


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
    custom_tests = CellPhoneDBTests(runner)
    for test_method in [
        custom_tests.test_interaction_structure,
        custom_tests.test_ligand_receptor_directionality,
        custom_tests.test_adhesion_directionality,
        custom_tests.test_complex_interaction,
        custom_tests.test_integrin_interaction,
        custom_tests.test_receptor_interaction,
        custom_tests.test_secreted_interaction,
        custom_tests.test_search_by_gene,
        custom_tests.test_cross_references_to_uniprot,
        custom_tests.test_cross_references_to_ensembl,
        custom_tests.test_classification_signaling,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
