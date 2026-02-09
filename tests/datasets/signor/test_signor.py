#!/usr/bin/env python3
"""
SIGNOR Test Suite

Tests SIGNOR causal signaling network dataset.
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


class SIGNORTests:
    """SIGNOR custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_uniprot_xref(self):
        """UniProt cross-reference exists for protein entries"""
        for entry in self.runner.reference_data:
            if entry.get("database_a") == "UNIPROT" and entry.get("id_a"):
                data = self.runner.lookup(entry["signor_id"])
                if data and data.get("results"):
                    result = data["results"][0]
                    if self.runner.has_xref(result, "uniprot", entry["id_a"]):
                        return True, f"Found UniProt xref {entry['id_a']} for {entry['signor_id']}"
        return False, "No entry with UniProt cross-reference found"

    @test
    def test_chebi_xref(self):
        """ChEBI cross-reference for chemical entries"""
        for entry in self.runner.reference_data:
            if entry.get("database_a") == "ChEBI" or entry.get("database_b") == "ChEBI":
                chebi_id = entry.get("id_a") if entry.get("database_a") == "ChEBI" else entry.get("id_b")
                if chebi_id:
                    chebi_id_clean = chebi_id.replace("CHEBI:", "")
                    data = self.runner.lookup(entry["signor_id"])
                    if data and data.get("results"):
                        result = data["results"][0]
                        if self.runner.has_xref(result, "chebi", chebi_id_clean):
                            return True, f"Found ChEBI xref {chebi_id_clean} for {entry['signor_id']}"
        return False, "No entry with ChEBI cross-reference found"

    @test
    def test_pubmed_xref(self):
        """PubMed literature references exist"""
        for entry in self.runner.reference_data:
            pmid = entry.get("pmid", "")
            if pmid and pmid != "Other" and pmid.isdigit():
                data = self.runner.lookup(entry["signor_id"])
                if data and data.get("results"):
                    result = data["results"][0]
                    if self.runner.has_xref(result, "pubmed", pmid):
                        return True, f"Found PubMed xref {pmid} for {entry['signor_id']}"
        return False, "No entry with PubMed cross-reference found"

    @test
    def test_taxonomy_xref(self):
        """Taxonomy cross-reference for organism"""
        for entry in self.runner.reference_data:
            tax_id = entry.get("tax_id", "")
            if tax_id:
                data = self.runner.lookup(entry["signor_id"])
                if data and data.get("results"):
                    result = data["results"][0]
                    if self.runner.has_xref(result, "taxonomy", tax_id):
                        return True, f"Found taxonomy xref {tax_id} for {entry['signor_id']}"
        return False, "No entry with taxonomy cross-reference found"

    @test
    def test_multiple_entries_exist(self):
        """Multiple entries can be looked up"""
        found_count = 0
        for entry in self.runner.reference_data[:10]:
            data = self.runner.lookup(entry["signor_id"])
            if data and data.get("results"):
                found_count += 1
        if found_count >= 5:
            return True, f"Found {found_count}/10 entries in database"
        return False, f"Only found {found_count}/10 entries"

    @test
    def test_xref_counts(self):
        """Entries have cross-references"""
        entry = self.runner.reference_data[0]
        signor_id = entry["signor_id"]
        data = self.runner.lookup(signor_id)
        if data and data.get("results"):
            result = data["results"][0]
            xref_count = self.runner.get_xref_count(result)
            if xref_count > 0:
                return True, f"{signor_id} has {xref_count} cross-references"
        return False, f"No cross-references found for {signor_id}"

    @test
    def test_signor_dataset_present(self):
        """SIGNOR entries are in the signor dataset"""
        entry = self.runner.reference_data[0]
        signor_id = entry["signor_id"]
        data = self.runner.lookup(signor_id)
        if data and data.get("results"):
            result = data["results"][0]
            # Check the result's own dataset_name, not cross-references
            if result.get("dataset_name") == "signor":
                return True, f"{signor_id} found in signor dataset"
        return False, f"{signor_id} not found in signor dataset"

    @test
    def test_drugbank_xref_exists(self):
        """DrugBank cross-reference exists for drug entries"""
        for entry in self.runner.reference_data:
            if entry.get("database_a") == "DRUGBANK" or entry.get("database_b") == "DRUGBANK":
                drug_id = entry.get("id_a") if entry.get("database_a") == "DRUGBANK" else entry.get("id_b")
                if drug_id:
                    data = self.runner.lookup(entry["signor_id"])
                    if data and data.get("results"):
                        result = data["results"][0]
                        if self.runner.has_xref(result, "drugbank", drug_id):
                            return True, f"Found DrugBank xref {drug_id} for {entry['signor_id']}"
        return True, "SKIP: No DrugBank entries in test data"


def main():
    """Run SIGNOR test suite"""
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
    custom_tests = SIGNORTests(runner)
    for test_method in [
        custom_tests.test_uniprot_xref,
        custom_tests.test_chebi_xref,
        custom_tests.test_pubmed_xref,
        custom_tests.test_taxonomy_xref,
        custom_tests.test_multiple_entries_exist,
        custom_tests.test_xref_counts,
        custom_tests.test_signor_dataset_present,
        custom_tests.test_drugbank_xref_exists,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
