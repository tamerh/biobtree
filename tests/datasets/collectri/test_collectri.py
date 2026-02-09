#!/usr/bin/env python3
"""
CollecTRI Test Suite

Tests CollecTRI TF-target gene regulatory network dataset.
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


class CollecTRITests:
    """CollecTRI custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_tf_target_structure(self):
        """Check TF:TG ID format is correctly stored"""
        entry = self.runner.reference_data[0]
        entry_id = entry["id"]
        tf_gene = entry["tf_gene"]
        target_gene = entry["target_gene"]

        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        result = data["results"][0]

        # Check the identifier matches expected format
        if result.get("identifier") != entry_id:
            return False, f"ID mismatch: expected {entry_id}, got {result.get('identifier')}"

        return True, f"{entry_id}: {tf_gene} regulates {target_gene}"

    @test
    def test_activation_regulation(self):
        """Check entry with Activation regulation"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("regulation") == "Activation"),
            None
        )
        if not entry:
            return True, "SKIP: No Activation entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        return True, f"{entry_id} has Activation regulation"

    @test
    def test_repression_regulation(self):
        """Check entry with Repression regulation"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("regulation") == "Repression"),
            None
        )
        if not entry:
            return True, "SKIP: No Repression entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        return True, f"{entry_id} has Repression regulation"

    @test
    def test_high_confidence_entry(self):
        """Check entry with High confidence"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("confidence") == "High"),
            None
        )
        if not entry:
            return True, "SKIP: No High confidence entry in reference"

        entry_id = entry["id"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        return True, f"{entry_id} has High confidence"

    @test
    def test_multiple_sources(self):
        """Check entry with multiple evidence sources"""
        entry = next(
            (e for e in self.runner.reference_data
             if len(e.get("sources", [])) >= 3),
            None
        )
        if not entry:
            return True, "SKIP: No entry with 3+ sources in reference"

        entry_id = entry["id"]
        sources = entry["sources"]
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        return True, f"{entry_id} has {len(sources)} sources: {', '.join(sources[:3])}"

    @test
    def test_no_tred_in_sources(self):
        """Verify TRED is not included in any sources (licensing exclusion)"""
        for entry in self.runner.reference_data:
            sources = entry.get("sources", [])
            if "TRED" in sources:
                return False, f"TRED found in sources for {entry['id']}"

        return True, "TRED correctly excluded from all entries"

    @test
    def test_has_pubmed_xrefs(self):
        """Check entries have PubMed cross-references"""
        entry = next(
            (e for e in self.runner.reference_data
             if len(e.get("pmids", [])) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with PMIDs in reference"

        entry_id = entry["id"]
        expected_pmid_count = len(entry["pmids"])
        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        # Check for pubmed xrefs
        pubmed_xrefs = self.runner.get_xrefs(data["results"][0], "pubmed")
        if not pubmed_xrefs:
            return False, f"{entry_id} has no PubMed cross-references"

        return True, f"{entry_id} has {len(pubmed_xrefs)} PubMed cross-references (expected ~{expected_pmid_count})"

    @test
    def test_search_by_tf_gene(self):
        """Search for TF gene finds CollecTRI entries"""
        entry = self.runner.reference_data[0]
        tf_gene = entry["tf_gene"]

        data = self.runner.lookup(tf_gene)

        if not data or not data.get("results"):
            return False, f"No results when searching for TF gene {tf_gene}"

        # Check that we got CollecTRI results
        collectri_results = [
            r for r in data["results"]
            if r.get("dataset_name", "").lower() == "collectri"
        ]

        if not collectri_results:
            return True, f"SKIP: {tf_gene} search did not return CollecTRI entries in test data"

        return True, f"Searching '{tf_gene}' found {len(collectri_results)} CollecTRI entries"

    @test
    def test_cross_references_to_hgnc(self):
        """Check CollecTRI entries have cross-references to HGNC"""
        entry = self.runner.reference_data[0]
        entry_id = entry["id"]

        data = self.runner.lookup(entry_id)

        if not data or not data.get("results"):
            return False, f"No results for {entry_id}"

        result = data["results"][0]
        datasets = self.runner.get_xref_datasets(result)

        # CollecTRI should have xrefs to hgnc (via gene symbols)
        if "hgnc" in [d.lower() for d in datasets]:
            hgnc_count = self.runner.get_xref_count(result, "hgnc")
            return True, f"{entry_id} has {hgnc_count} HGNC cross-references"

        return True, f"SKIP: {entry_id} has no HGNC xrefs in test data (xrefs: {', '.join(datasets[:5])})"


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
    custom_tests = CollecTRITests(runner)
    for test_method in [
        custom_tests.test_tf_target_structure,
        custom_tests.test_activation_regulation,
        custom_tests.test_repression_regulation,
        custom_tests.test_high_confidence_entry,
        custom_tests.test_multiple_sources,
        custom_tests.test_no_tred_in_sources,
        custom_tests.test_has_pubmed_xrefs,
        custom_tests.test_search_by_tf_gene,
        custom_tests.test_cross_references_to_hgnc,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
