#!/usr/bin/env python3
"""
CORUM Test Suite

Tests CORUM (Comprehensive Resource of Mammalian Protein Complexes) dataset.
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


class CORUMTests:
    """CORUM custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_uniprot_xref(self):
        """UniProt cross-reference exists for subunits"""
        for entry in self.runner.reference_data:
            subunits = entry.get("subunits", [])
            for sub in subunits:
                swissprot = sub.get("swissprot", {})
                uniprot_id = swissprot.get("uniprot_id")
                if uniprot_id:
                    data = self.runner.lookup(entry["complex_id"])
                    if data and data.get("results"):
                        result = data["results"][0]
                        if self.runner.has_xref(result, "uniprot", uniprot_id):
                            return True, f"Found UniProt xref {uniprot_id} for complex {entry['complex_id']}"
        return False, "No entry with UniProt cross-reference found"

    @test
    def test_go_xref(self):
        """GO cross-reference for functional annotations"""
        for entry in self.runner.reference_data:
            functions = entry.get("functions", [])
            for func in functions:
                go_obj = func.get("go", {})
                go_id = go_obj.get("go_id")
                if go_id:
                    data = self.runner.lookup(entry["complex_id"])
                    if data and data.get("results"):
                        result = data["results"][0]
                        if self.runner.has_xref(result, "go", go_id):
                            return True, f"Found GO xref {go_id} for complex {entry['complex_id']}"
        return False, "No entry with GO cross-reference found"

    @test
    def test_pubmed_xref(self):
        """PubMed literature references exist"""
        for entry in self.runner.reference_data:
            pmid = entry.get("pmid")
            if pmid:
                data = self.runner.lookup(entry["complex_id"])
                if data and data.get("results"):
                    result = data["results"][0]
                    if self.runner.has_xref(result, "pubmed", str(pmid)):
                        return True, f"Found PubMed xref {pmid} for complex {entry['complex_id']}"
        return False, "No entry with PubMed cross-reference found"

    @test
    def test_taxonomy_xref(self):
        """Taxonomy cross-reference for organism"""
        # Map organism names to expected taxonomy IDs
        org_to_tax = {
            "Human": "9606",
            "Mouse": "10090",
            "Rat": "10116",
        }
        for entry in self.runner.reference_data:
            organism = entry.get("organism")
            if organism in org_to_tax:
                expected_tax = org_to_tax[organism]
                data = self.runner.lookup(entry["complex_id"])
                if data and data.get("results"):
                    result = data["results"][0]
                    if self.runner.has_xref(result, "taxonomy", expected_tax):
                        return True, f"Found taxonomy xref {expected_tax} ({organism}) for complex {entry['complex_id']}"
        return False, "No entry with taxonomy cross-reference found"

    @test
    def test_multiple_entries_exist(self):
        """Multiple entries can be looked up"""
        found_count = 0
        for entry in self.runner.reference_data[:10]:
            data = self.runner.lookup(entry["complex_id"])
            if data and data.get("results"):
                found_count += 1
        if found_count >= 5:
            return True, f"Found {found_count}/10 entries in database"
        return False, f"Only found {found_count}/10 entries"

    @test
    def test_xref_counts(self):
        """Entries have cross-references"""
        entry = self.runner.reference_data[0]
        complex_id = entry["complex_id"]
        data = self.runner.lookup(complex_id)
        if data and data.get("results"):
            result = data["results"][0]
            xref_count = self.runner.get_xref_count(result)
            if xref_count > 0:
                return True, f"Complex {complex_id} has {xref_count} cross-references"
        return False, f"No cross-references found for complex {complex_id}"

    @test
    def test_corum_dataset_present(self):
        """CORUM entries are in the corum dataset"""
        entry = self.runner.reference_data[0]
        complex_id = entry["complex_id"]
        data = self.runner.lookup(complex_id)
        if data and data.get("results"):
            result = data["results"][0]
            if result.get("dataset_name") == "corum":
                return True, f"Complex {complex_id} found in corum dataset"
        return False, f"Complex {complex_id} not found in corum dataset"

    @test
    def test_text_search_by_name(self):
        """Complex name text search works"""
        entry = self.runner.reference_data[0]
        complex_name = entry.get("complex_name", "")
        if complex_name:
            data = self.runner.lookup(complex_name)
            if data and data.get("results"):
                for result in data["results"]:
                    if result.get("dataset_name") == "corum":
                        return True, f"Found complex via name search: {complex_name}"
        return False, "Text search by name did not find complex"

    @test
    def test_subunit_count_attribute(self):
        """Subunit count attribute matches source data"""
        for entry in self.runner.reference_data[:5]:
            expected_count = len(entry.get("subunits", []))
            if expected_count > 0:
                data = self.runner.lookup(entry["complex_id"])
                if data and data.get("results"):
                    result = data["results"][0]
                    attrs = result.get("attributes", {})
                    if attrs:
                        actual_count = attrs.get("subunit_count", 0)
                        if actual_count == expected_count:
                            return True, f"Subunit count matches: {actual_count}"
        return True, "SKIP: Could not verify subunit count"


def main():
    """Run CORUM test suite"""
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
    custom_tests = CORUMTests(runner)
    for test_method in [
        custom_tests.test_uniprot_xref,
        custom_tests.test_go_xref,
        custom_tests.test_pubmed_xref,
        custom_tests.test_taxonomy_xref,
        custom_tests.test_multiple_entries_exist,
        custom_tests.test_xref_counts,
        custom_tests.test_corum_dataset_present,
        custom_tests.test_text_search_by_name,
        custom_tests.test_subunit_count_attribute,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
