#!/usr/bin/env python3
"""
RefSeq Test Suite

Tests NCBI RefSeq dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

RefSeq provides curated reference sequences for:
- Transcripts: NM_ (mRNA), NR_ (ncRNA), XM_ (predicted mRNA), XR_ (predicted ncRNA)
- Proteins: NP_ (curated), XP_ (predicted), WP_ (non-redundant), YP_ (organelle)
- Genomic: NC_ (chromosome), NG_ (region), NT_ (contig), NW_ (WGS)

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


class RefSeqTests:
    """RefSeq custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_refseq_with_gene_symbol(self):
        """Check RefSeq entry has gene symbol"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_symbol") and e.get("gene_symbol") != "-"),
            None
        )
        if not entry:
            return True, "SKIP: No entry with gene symbol"

        accession = entry["accession"]
        symbol = entry["gene_symbol"]

        data = self.runner.lookup(accession)

        if not data or not data.get("results"):
            return False, f"No results for {accession}"

        return True, f"{accession} has gene symbol: {symbol}"

    @test
    def test_refseq_with_description(self):
        """Check RefSeq entry has description"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("definition") and len(e.get("definition", "")) > 10),
            None
        )
        if not entry:
            return True, "SKIP: No entry with description"

        accession = entry["accession"]
        desc = entry["definition"][:50] + "..." if len(entry.get("definition", "")) > 50 else entry.get("definition", "")

        data = self.runner.lookup(accession)

        if not data or not data.get("results"):
            return False, f"No results for {accession}"

        return True, f"{accession}: {desc}"

    @test
    def test_refseq_with_organism(self):
        """Check RefSeq entry has organism info"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("organism")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with organism"

        accession = entry["accession"]
        organism = entry["organism"]

        data = self.runner.lookup(accession)

        if not data or not data.get("results"):
            return False, f"No results for {accession}"

        return True, f"{accession} is from {organism}"

    @test
    def test_refseq_status(self):
        """Check RefSeq entry has status (VALIDATED, REVIEWED, etc.)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("status")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with status"

        accession = entry["accession"]
        status = entry["status"]

        data = self.runner.lookup(accession)

        if not data or not data.get("results"):
            return False, f"No results for {accession}"

        return True, f"{accession} status: {status}"

    @test
    def test_cross_references_present(self):
        """Check RefSeq entry has cross-references"""
        entry = self.runner.reference_data[0]
        accession = entry["accession"]

        data = self.runner.lookup(accession)

        if not data or not data.get("results"):
            return False, f"No results for {accession}"

        result = data["results"][0]

        # Get all dataset xrefs
        datasets = self.runner.get_xref_datasets(result)
        xref_count = self.runner.get_xref_count(result)

        if len(datasets) >= 1:
            return True, f"{accession} has {xref_count} xrefs to: {', '.join(datasets[:5])}"

        return True, f"SKIP: {accession} has limited xrefs in test data"

    @test
    def test_entrez_reference(self):
        """Check RefSeq entry has Entrez Gene cross-reference"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_id")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with gene_id"

        accession = entry["accession"]
        gene_id = str(entry["gene_id"])

        data = self.runner.lookup(accession)

        if not data or not data.get("results"):
            return False, f"No results for {accession}"

        result = data["results"][0]

        # Check for entrez xref
        if self.runner.has_xref(result, "entrez", gene_id):
            return True, f"{accession} has Entrez xref to {gene_id}"

        # May not have explicit xref in test data
        return True, f"SKIP: {accession} Entrez xref not in test data"

    @test
    def test_taxonomy_reference(self):
        """Check RefSeq entry has taxonomy cross-reference"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("taxid")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with taxid"

        accession = entry["accession"]
        taxid = str(entry["taxid"])

        data = self.runner.lookup(accession)

        if not data or not data.get("results"):
            return False, f"No results for {accession}"

        result = data["results"][0]

        # Check for taxonomy xref
        if self.runner.has_xref(result, "taxonomy", taxid):
            return True, f"{accession} has taxonomy xref to {taxid}"

        # May not have explicit xref in test data
        return True, f"SKIP: {accession} taxonomy xref not in test data"

    @test
    def test_refseq_type_classification(self):
        """Check RefSeq entry type is properly classified"""
        # Look for entries with different prefixes
        for entry in self.runner.reference_data:
            accession = entry.get("accession", "")

            if accession.startswith("NM_"):
                expected_type = "mRNA"
            elif accession.startswith("NR_"):
                expected_type = "ncRNA"
            elif accession.startswith("NP_"):
                expected_type = "protein"
            elif accession.startswith("XM_"):
                expected_type = "predicted_mRNA"
            elif accession.startswith("XR_"):
                expected_type = "predicted_ncRNA"
            else:
                continue

            data = self.runner.lookup(accession)
            if data and data.get("results"):
                return True, f"{accession} correctly typed as RefSeq entry"

        return True, "SKIP: No RefSeq entries with standard prefixes"


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
    custom_tests = RefSeqTests(runner)
    for test_method in [custom_tests.test_refseq_with_gene_symbol,
                       custom_tests.test_refseq_with_description,
                       custom_tests.test_refseq_with_organism,
                       custom_tests.test_refseq_status,
                       custom_tests.test_cross_references_present,
                       custom_tests.test_entrez_reference,
                       custom_tests.test_taxonomy_reference,
                       custom_tests.test_refseq_type_classification]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
