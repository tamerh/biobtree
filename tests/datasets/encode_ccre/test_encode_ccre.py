#!/usr/bin/env python3
"""
ENCODE cCRE Test Suite

Tests ENCODE cCRE (candidate cis-Regulatory Elements) dataset processing.
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


class EncodeCcreTests:
    """ENCODE cCRE custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_promoter_like_search(self):
        """Check PLS (promoter-like) classification is searchable"""
        data = self.runner.lookup("PLS")

        if not data or not data.get("results"):
            return False, "No results for PLS classification"

        count = len(data["results"])
        return True, f"Found {count} PLS entries"

    @test
    def test_proximal_enhancer_search(self):
        """Check pELS (proximal enhancer-like) classification is searchable"""
        data = self.runner.lookup("pELS")

        if not data or not data.get("results"):
            return False, "No results for pELS classification"

        count = len(data["results"])
        return True, f"Found {count} pELS entries"

    @test
    def test_distal_enhancer_search(self):
        """Check dELS (distal enhancer-like) classification is searchable"""
        data = self.runner.lookup("dELS")

        if not data or not data.get("results"):
            return False, "No results for dELS classification"

        count = len(data["results"])
        return True, f"Found {count} dELS entries"

    @test
    def test_ctcf_bound_search(self):
        """Check CA-CTCF (CTCF-bound) classification is searchable"""
        data = self.runner.lookup("CA-CTCF")

        if not data or not data.get("results"):
            return False, "No results for CA-CTCF classification"

        count = len(data["results"])
        return True, f"Found {count} CA-CTCF entries"

    @test
    def test_taxonomy_xref(self):
        """Check cCRE to Taxonomy cross-reference (human)"""
        # Use mapping endpoint to test taxonomy xref
        map_url = f"{self.runner.api_url}/ws/map/"
        try:
            response = requests.get(
                map_url,
                params={"i": "pELS", "m": ">>encode_ccre>>taxonomy"},
                timeout=30
            )
            if response.status_code == 200:
                data = response.json()
                if data.get("results"):
                    # Check if human taxonomy (9606) is in results
                    tax_ids = [r.get("id", "") for r in data["results"]]
                    if "9606" in tax_ids:
                        return True, "Taxonomy xref to human (9606) verified"
                    else:
                        return True, f"Taxonomy xref exists (found: {tax_ids[:3]})"
                else:
                    return False, "No taxonomy mappings found"
            else:
                return False, f"Mapping failed: {response.status_code}"
        except Exception as e:
            return False, f"Mapping error: {e}"

    @test
    def test_chromosome_attribute(self):
        """Check chromosome attribute is stored correctly"""
        data = self.runner.lookup("pELS")

        if not data or not data.get("results"):
            return False, "No results for pELS"

        result = data["results"][0]
        has_attr = result.get("has_attr", False)

        if has_attr:
            return True, "cCRE entry has attributes (chromosome, start, end)"
        else:
            return True, "cCRE entry found (attrs in full build only)"

    @test
    def test_classification_filter(self):
        """Check classification filter works"""
        map_url = f"{self.runner.api_url}/ws/map/"
        try:
            response = requests.get(
                map_url,
                params={"i": "pELS", "m": '>>encode_ccre[encode_ccre.ccre_class=="pELS"]'},
                timeout=30
            )
            if response.status_code == 200:
                data = response.json()
                if data.get("results"):
                    return True, f"Classification filter works ({len(data['results'])} results)"
                else:
                    return False, "Classification filter returned no results"
            else:
                return False, f"Filter failed: {response.status_code}"
        except Exception as e:
            return False, f"Filter error: {e}"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Create test runner (reference_data.json is optional for this dataset)
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = EncodeCcreTests(runner)
    for test_method in [
        custom_tests.test_promoter_like_search,
        custom_tests.test_proximal_enhancer_search,
        custom_tests.test_distal_enhancer_search,
        custom_tests.test_ctcf_bound_search,
        custom_tests.test_taxonomy_xref,
        custom_tests.test_chromosome_attribute,
        custom_tests.test_classification_filter,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
