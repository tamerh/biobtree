#!/usr/bin/env python3
"""
BRENDA (Comprehensive Enzyme Information System) Test Suite

Tests all 3 BRENDA datasets:
- brenda: EC enzyme entries with summary attributes
- brenda_kinetics: Detailed kinetic measurements (Km, kcat) per EC+substrate
- brenda_inhibitor: Inhibitor data (Ki, IC50) per EC+inhibitor
"""

import sys
import os
import requests
import urllib.parse
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test, discover_tests


class BrendaTests:
    """BRENDA custom tests for all 3 datasets"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _test_mapping(self, source_id: str, source_dataset: str, target_dataset: str, filter_expr: str = None) -> tuple:
        """Helper to test a mapping between datasets"""
        if filter_expr:
            encoded_filter = urllib.parse.quote(f"[{filter_expr}]")
            url = f"{self.runner.api_url}/ws/map/?i={source_id}&m=>>{source_dataset}>>{target_dataset}{encoded_filter}"
        else:
            url = f"{self.runner.api_url}/ws/map/?i={source_id}&m=>>{source_dataset}>>{target_dataset}"
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

    def _entry_dataset(self, identifier: str, dataset: str) -> dict:
        """Helper to get entry details from specific dataset"""
        encoded_id = urllib.parse.quote(identifier)
        url = f"{self.runner.api_url}/ws/entry/?i={encoded_id}&s={dataset}"
        try:
            resp = requests.get(url, timeout=30)
            if resp.status_code == 200:
                data = resp.json()
                if data and len(data) > 0:
                    return data[0]
        except:
            pass
        return {}

    def _text_search(self, query: str, dataset: str = None) -> list:
        """Helper to perform text search"""
        encoded_query = urllib.parse.quote(query)
        if dataset:
            url = f"{self.runner.api_url}/ws/?i={encoded_query}&d={dataset}"
        else:
            url = f"{self.runner.api_url}/ws/?i={encoded_query}"
        try:
            resp = requests.get(url, timeout=30)
            if resp.status_code == 200:
                data = resp.json()
                return data.get("results", [])
        except:
            pass
        return []

    # =========================================================================
    # BRENDA Main Tests
    # =========================================================================

    @test
    def test_brenda_recommended_name(self):
        """Check brenda entry has recommended_name attribute"""
        entry = self._entry_dataset("1.1.1.1", "brenda")
        if not entry:
            return False, "No entry found for 1.1.1.1"

        attr = entry.get("Attributes", {}).get("Brenda", {})
        name = attr.get("recommended_name", "")
        if not name:
            return False, "No recommended_name attribute"

        return True, f"EC 1.1.1.1 has recommended_name: {name}"

    @test
    def test_brenda_systematic_name(self):
        """Check brenda entry has systematic_name attribute"""
        entry = self._entry_dataset("1.1.1.1", "brenda")
        if not entry:
            return False, "No entry found for 1.1.1.1"

        attr = entry.get("Attributes", {}).get("Brenda", {})
        name = attr.get("systematic_name", "")
        if not name:
            return False, "No systematic_name attribute"

        return True, f"EC 1.1.1.1 has systematic_name: {name[:50]}..."

    @test
    def test_brenda_organism_count(self):
        """Check brenda entry has organism_count > 0"""
        entry = self._entry_dataset("1.1.1.1", "brenda")
        if not entry:
            return False, "No entry found for 1.1.1.1"

        attr = entry.get("Attributes", {}).get("Brenda", {})
        count = attr.get("organism_count", 0)
        if count <= 0:
            return False, "organism_count is 0 or missing"

        return True, f"EC 1.1.1.1 has {count} organisms"

    @test
    def test_brenda_substrate_count(self):
        """Check brenda entry has substrate_count > 0"""
        entry = self._entry_dataset("1.1.1.1", "brenda")
        if not entry:
            return False, "No entry found for 1.1.1.1"

        attr = entry.get("Attributes", {}).get("Brenda", {})
        count = attr.get("substrate_count", 0)
        if count <= 0:
            return False, "substrate_count is 0 or missing"

        return True, f"EC 1.1.1.1 has {count} substrates"

    @test
    def test_brenda_inhibitor_count(self):
        """Check brenda entry has inhibitor_count > 0"""
        entry = self._entry_dataset("1.1.1.1", "brenda")
        if not entry:
            return False, "No entry found for 1.1.1.1"

        attr = entry.get("Attributes", {}).get("Brenda", {})
        count = attr.get("inhibitor_count", 0)
        if count <= 0:
            return False, "inhibitor_count is 0 or missing"

        return True, f"EC 1.1.1.1 has {count} inhibitors"

    @test
    def test_brenda_no_pubmed_ids_attr(self):
        """Verify pubmed_ids removed from attributes (now xrefs only)"""
        entry = self._entry_dataset("1.1.1.1", "brenda")
        if not entry:
            return False, "No entry found for 1.1.1.1"

        attr = entry.get("Attributes", {}).get("Brenda", {})
        if "pubmed_ids" in attr:
            return False, "pubmed_ids should not be in attributes"

        return True, "pubmed_ids correctly stored as xrefs only"

    @test
    def test_brenda_no_top_substrates_attr(self):
        """Verify top_substrates removed (use brenda_kinetics instead)"""
        entry = self._entry_dataset("1.1.1.1", "brenda")
        if not entry:
            return False, "No entry found for 1.1.1.1"

        attr = entry.get("Attributes", {}).get("Brenda", {})
        if "top_substrates" in attr:
            return False, "top_substrates should not be in attributes"

        return True, "top_substrates correctly moved to brenda_kinetics"

    @test
    def test_brenda_no_top_inhibitors_attr(self):
        """Verify top_inhibitors removed (use brenda_inhibitor instead)"""
        entry = self._entry_dataset("1.1.1.1", "brenda")
        if not entry:
            return False, "No entry found for 1.1.1.1"

        attr = entry.get("Attributes", {}).get("Brenda", {})
        if "top_inhibitors" in attr:
            return False, "top_inhibitors should not be in attributes"

        return True, "top_inhibitors correctly moved to brenda_inhibitor"

    # =========================================================================
    # BRENDA Kinetics Tests
    # =========================================================================

    @test
    def test_brenda_kinetics_km_values(self):
        """Check brenda_kinetics entry has km_values"""
        entry = self._entry_dataset("1.1.1.1|ethanol", "brenda_kinetics")
        if not entry:
            return False, "No kinetics entry for 1.1.1.1|ethanol"

        attr = entry.get("Attributes", {}).get("BrendaKinetics", {})
        km_count = attr.get("km_count", 0)
        if km_count <= 0:
            return False, "No Km values found"

        min_km = attr.get("min_km", 0)
        max_km = attr.get("max_km", 0)
        return True, f"Found {km_count} Km values, range: {min_km}-{max_km} mM"

    @test
    def test_brenda_kinetics_substrate_type(self):
        """Check brenda_kinetics has substrate_type attribute"""
        entry = self._entry_dataset("1.1.1.1|ethanol", "brenda_kinetics")
        if not entry:
            return False, "No kinetics entry for 1.1.1.1|ethanol"

        attr = entry.get("Attributes", {}).get("BrendaKinetics", {})
        substrate = attr.get("substrate", "")
        if not substrate:
            return False, "No substrate attribute"

        return True, f"Substrate: {substrate}"

    @test
    def test_brenda_kinetics_xref_to_brenda(self):
        """Check brenda_kinetics has xref back to brenda"""
        entry = self._entry_dataset("1.1.1.1|ethanol", "brenda_kinetics")
        if not entry:
            return False, "No kinetics entry for 1.1.1.1|ethanol"

        entries = entry.get("entries", [])
        brenda_xrefs = [e for e in entries if e.get("dataset_name") == "brenda"]
        if not brenda_xrefs:
            return False, "No xref to brenda dataset"

        return True, f"Has xref to brenda: {brenda_xrefs[0].get('identifier')}"

    # =========================================================================
    # BRENDA Inhibitor Tests
    # =========================================================================

    @test
    def test_brenda_inhibitor_ki_values(self):
        """Check brenda_inhibitor entry with Ki values"""
        entry = self._entry_dataset("1.1.1.1|ethanol", "brenda_inhibitor")
        if not entry:
            return False, "No inhibitor entry for 1.1.1.1|ethanol"

        attr = entry.get("Attributes", {}).get("BrendaInhibitor", {})
        ki_count = attr.get("ki_count", 0)

        if ki_count > 0:
            min_ki = attr.get("min_ki", 0)
            max_ki = attr.get("max_ki", 0)
            return True, f"Found {ki_count} Ki values, range: {min_ki}-{max_ki} mM"
        else:
            # Check for inhibition_data instead
            inh_count = attr.get("inhibition_count", 0)
            if inh_count > 0:
                return True, f"Found {inh_count} inhibition data entries (no Ki values)"
            return False, "No Ki values or inhibition data found"

    @test
    def test_brenda_inhibitor_xref_to_brenda(self):
        """Check brenda_inhibitor has xref back to brenda"""
        entry = self._entry_dataset("1.1.1.1|ethanol", "brenda_inhibitor")
        if not entry:
            return False, "No inhibitor entry for 1.1.1.1|ethanol"

        entries = entry.get("entries", [])
        brenda_xrefs = [e for e in entries if e.get("dataset_name") == "brenda"]
        if not brenda_xrefs:
            return False, "No xref to brenda dataset"

        return True, f"Has xref to brenda: {brenda_xrefs[0].get('identifier')}"

    # =========================================================================
    # Text Search Tests
    # =========================================================================

    @test
    def test_text_search_recommended_name(self):
        """Test text search by recommended enzyme name"""
        results = self._text_search("alcohol dehydrogenase", "brenda")
        if not results:
            return False, "No results for 'alcohol dehydrogenase'"

        ec_ids = [r.get("identifier") for r in results]
        if "1.1.1.1" in ec_ids:
            return True, f"Found {len(results)} results including 1.1.1.1"
        return True, f"Found {len(results)} results: {ec_ids[:3]}"

    @test
    def test_text_search_synonym(self):
        """Test text search by enzyme synonym"""
        results = self._text_search("ethanol dehydrogenase", "brenda")
        if not results:
            return False, "No results for 'ethanol dehydrogenase'"

        return True, f"Found {len(results)} results for synonym search"

    @test
    def test_text_search_substrate(self):
        """Test text search for substrate in brenda_kinetics"""
        results = self._text_search("ethanol", "brenda_kinetics")
        if not results:
            return False, "No results for 'ethanol' in brenda_kinetics"

        return True, f"Found {len(results)} kinetic entries for 'ethanol'"

    @test
    def test_text_search_inhibitor(self):
        """Test text search for inhibitor in brenda_inhibitor"""
        results = self._text_search("pyrazole", "brenda_inhibitor")
        if not results:
            # Try another common inhibitor
            results = self._text_search("iodoacetamide", "brenda_inhibitor")
            if not results:
                return False, "No results for inhibitor search"

        return True, f"Found {len(results)} inhibitor entries"

    # =========================================================================
    # Cross-reference Tests
    # =========================================================================

    @test
    def test_brenda_to_pubmed_xref(self):
        """Test brenda to pubmed cross-references"""
        success, msg = self._test_mapping("1.1.1.1", "brenda", "pubmed")
        return success, msg

    @test
    def test_brenda_to_kinetics_xref(self):
        """Test brenda to brenda_kinetics cross-references"""
        success, msg = self._test_mapping("1.1.1.1", "brenda", "brenda_kinetics")
        return success, msg

    @test
    def test_brenda_to_inhibitor_xref(self):
        """Test brenda to brenda_inhibitor cross-references"""
        success, msg = self._test_mapping("1.1.1.1", "brenda", "brenda_inhibitor")
        return success, msg

    # =========================================================================
    # Filter Tests
    # =========================================================================

    @test
    def test_filter_kinetics_by_km_count(self):
        """Test filtering brenda_kinetics by km_count"""
        success, msg = self._test_mapping(
            "1.1.1.1", "brenda", "brenda_kinetics",
            "brenda_kinetics.km_count>10"
        )
        return success, msg

    @test
    def test_filter_inhibitor_by_ki_count(self):
        """Test filtering brenda_inhibitor by ki_count"""
        success, msg = self._test_mapping(
            "1.1.1.1", "brenda", "brenda_inhibitor",
            "brenda_inhibitor.ki_count>0"
        )
        return success, msg


def main():
    """Run BRENDA tests"""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment or command line
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Allow command-line override
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--api-url', default=api_url)
    args = parser.parse_args()
    api_url = args.api_url

    # Check prerequisites
    if not reference_file.exists():
        print(f"Warning: {reference_file} not found - using empty reference data")
        reference_file = None

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests from BrendaTests class
    custom_tests = BrendaTests(runner)
    for test_method in discover_tests(custom_tests):
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
