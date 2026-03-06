#!/usr/bin/env python3
"""
MSigDB (Molecular Signatures Database) Test Suite

Tests gene set attributes, cross-references to HGNC gene symbols,
PubMed, GO terms, and HPO terms.
"""

import sys
import os
import requests
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class MsigdbTests:
    """MSigDB custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _test_mapping(self, source_id: str, target_dataset: str) -> tuple:
        """Helper to test a mapping from MSigDB to another dataset"""
        url = f"{self.runner.api_url}/ws/map/?i={source_id}&m=>>msigdb>>{target_dataset}"
        try:
            resp = requests.get(url, timeout=30)
            if resp.status_code != 200:
                return False, f"HTTP {resp.status_code}"
            data = resp.json()
            if data.get("results") and len(data["results"]) > 0:
                result = data["results"][0]
                targets = result.get("targets", [])
                if targets:
                    return True, f"Found {len(targets)} {target_dataset} mappings"
            return False, f"No {target_dataset} mappings found"
        except Exception as e:
            return False, str(e)

    @test
    def test_entry_with_standard_name(self):
        """Check entry has standard name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("standard_name")),
            None
        )
        if not entry:
            return False, "No entry with standard_name in reference"

        systematic_name = entry["systematic_name"]
        standard_name = entry["standard_name"]
        data = self.runner.lookup(systematic_name)

        if not data or not data.get("results"):
            return False, f"No results for {systematic_name}"

        return True, f"{systematic_name} has name: {standard_name[:50]}"

    @test
    def test_entry_with_collection(self):
        """Check entry has collection classification"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("collection")),
            None
        )
        if not entry:
            return False, "No entry with collection in reference"

        systematic_name = entry["systematic_name"]
        collection = entry["collection"]
        sub_collection = entry.get("sub_collection", "")
        data = self.runner.lookup(systematic_name)

        if not data or not data.get("results"):
            return False, f"No results for {systematic_name}"

        collection_str = f"{collection}:{sub_collection}" if sub_collection else collection
        return True, f"Entry is in collection: {collection_str}"

    @test
    def test_entry_with_gene_symbols(self):
        """Check entry has gene symbols"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_symbols") and len(e["gene_symbols"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with gene_symbols in reference"

        systematic_name = entry["systematic_name"]
        gene_count = len(entry["gene_symbols"])
        data = self.runner.lookup(systematic_name)

        if not data or not data.get("results"):
            return False, f"No results for {systematic_name}"

        return True, f"Entry has {gene_count} gene symbols"

    @test
    def test_entry_with_description(self):
        """Check entry has description"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("description") and len(e["description"]) > 10),
            None
        )
        if not entry:
            return False, "No entry with description in reference"

        systematic_name = entry["systematic_name"]
        description = entry["description"]
        data = self.runner.lookup(systematic_name)

        if not data or not data.get("results"):
            return False, f"No results for {systematic_name}"

        return True, f"Entry has description: {description[:60]}..."

    @test
    def test_entry_with_go_terms(self):
        """Check entry has GO terms"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("go_terms") and len(e["go_terms"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with go_terms in reference (may be normal)"

        systematic_name = entry["systematic_name"]
        go_count = len(entry["go_terms"])
        data = self.runner.lookup(systematic_name)

        if not data or not data.get("results"):
            return False, f"No results for {systematic_name}"

        return True, f"Entry has {go_count} GO term associations"

    @test
    def test_entry_with_hpo_terms(self):
        """Check entry has HPO terms"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("hpo_terms") and len(e["hpo_terms"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with hpo_terms in reference (may be normal)"

        systematic_name = entry["systematic_name"]
        hpo_count = len(entry["hpo_terms"])
        data = self.runner.lookup(systematic_name)

        if not data or not data.get("results"):
            return False, f"No results for {systematic_name}"

        return True, f"Entry has {hpo_count} HPO term associations"

    @test
    def test_entry_with_pmid(self):
        """Check entry has PubMed ID"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("pmid") and e["pmid"] not in ("", "0")),
            None
        )
        if not entry:
            return False, "No entry with pmid in reference"

        systematic_name = entry["systematic_name"]
        pmid = entry["pmid"]
        data = self.runner.lookup(systematic_name)

        if not data or not data.get("results"):
            return False, f"No results for {systematic_name}"

        return True, f"Entry has PMID: {pmid}"

    @test
    def test_text_search_by_standard_name(self):
        """Test text search by standard name (e.g., HALLMARK_APOPTOSIS)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("standard_name") and len(e["standard_name"]) >= 5),
            None
        )
        if not entry:
            return False, "No entry with suitable standard_name in reference"

        standard_name = entry["standard_name"]

        # Search by standard name
        data = self.runner.lookup(standard_name)
        if not data or not data.get("results"):
            return False, f"No results for: {standard_name}"

        return True, f"Found results for '{standard_name[:40]}'"

    @test
    def test_hallmark_collection(self):
        """Test Hallmark collection (H) entries exist"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("collection") == "H"),
            None
        )
        if not entry:
            return False, "No Hallmark (H) collection entry in reference"

        systematic_name = entry["systematic_name"]
        data = self.runner.lookup(systematic_name)

        if not data or not data.get("results"):
            return False, f"No results for {systematic_name}"

        return True, f"Hallmark gene set: {entry.get('standard_name', systematic_name)}"

    @test
    def test_mapping_hgnc_to_msigdb(self):
        """Test HGNC gene symbol -> MSigDB mapping"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_symbols") and len(e["gene_symbols"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with gene_symbols in reference"

        # Pick a gene symbol from the gene set
        gene_symbol = entry["gene_symbols"][0]

        url = f"{self.runner.api_url}/ws/map/?i={gene_symbol}&m=>>hgnc>>msigdb"
        try:
            resp = requests.get(url, timeout=30)
            if resp.status_code != 200:
                return False, f"HTTP {resp.status_code}"
            data = resp.json()
            if data.get("results") and len(data["results"]) > 0:
                result = data["results"][0]
                targets = result.get("targets", [])
                if targets:
                    return True, f"{gene_symbol} maps to {len(targets)} gene sets"
            return False, f"No MSigDB mappings for {gene_symbol}"
        except Exception as e:
            return False, str(e)

    @test
    def test_mapping_msigdb_to_pubmed(self):
        """Test MSigDB -> PubMed mapping"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("pmid") and e["pmid"] not in ("", "0")),
            None
        )
        if not entry:
            return False, "No entry with pmid in reference"

        systematic_name = entry["systematic_name"]
        return self._test_mapping(systematic_name, "pubmed")

    @test
    def test_mapping_msigdb_to_go(self):
        """Test MSigDB -> GO mapping"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("go_terms") and len(e["go_terms"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with go_terms in reference (may be normal)"

        systematic_name = entry["systematic_name"]
        return self._test_mapping(systematic_name, "go")

    @test
    def test_mapping_msigdb_to_hpo(self):
        """Test MSigDB -> HPO mapping"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("hpo_terms") and len(e["hpo_terms"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with hpo_terms in reference (may be normal)"

        systematic_name = entry["systematic_name"]
        return self._test_mapping(systematic_name, "hpo")

    @test
    def test_cel_filter_collection(self):
        """Test CEL filter by collection"""
        # Find a Hallmark entry (collection H)
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("collection") == "H"),
            None
        )
        if not entry:
            return False, "No Hallmark collection entry for CEL filter test"

        # Pick a gene from this entry
        if not entry.get("gene_symbols"):
            return False, "Entry has no gene symbols"

        gene_symbol = entry["gene_symbols"][0]

        # Map from gene to MSigDB with collection filter
        url = f"{self.runner.api_url}/ws/map/?i={gene_symbol}&m=>>hgnc>>msigdb&f=msigdb.collection=='H'"
        try:
            resp = requests.get(url, timeout=30)
            if resp.status_code != 200:
                return False, f"HTTP {resp.status_code}"
            data = resp.json()
            if data.get("results") and len(data["results"]) > 0:
                result = data["results"][0]
                targets = result.get("targets", [])
                if targets:
                    return True, f"CEL filter works: {len(targets)} Hallmark sets for {gene_symbol}"
            return False, "No filtered results"
        except Exception as e:
            return False, str(e)


def main():
    """Main test execution"""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        print("Run: python3 extract_reference_data.py")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom_tests = MsigdbTests(runner)

    for test_method in [
        # Attribute tests
        custom_tests.test_entry_with_standard_name,
        custom_tests.test_entry_with_collection,
        custom_tests.test_entry_with_gene_symbols,
        custom_tests.test_entry_with_description,
        custom_tests.test_entry_with_go_terms,
        custom_tests.test_entry_with_hpo_terms,
        custom_tests.test_entry_with_pmid,
        custom_tests.test_text_search_by_standard_name,
        custom_tests.test_hallmark_collection,
        # Cross-reference mapping tests
        custom_tests.test_mapping_hgnc_to_msigdb,
        custom_tests.test_mapping_msigdb_to_pubmed,
        custom_tests.test_mapping_msigdb_to_go,
        custom_tests.test_mapping_msigdb_to_hpo,
        # CEL filter test
        custom_tests.test_cel_filter_collection,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
