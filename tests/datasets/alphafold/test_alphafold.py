#!/usr/bin/env python3
"""
AlphaFold Protein Structure Predictions Test Suite

Tests AlphaFold dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

Note: AlphaFold data is stored as attributes on UniProt entries, so tests
query UniProt IDs and check for AlphaFold structure prediction attributes.

This script is called by the main orchestrator (tests/run_tests.py)
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


class AlphaFoldTests:
    """AlphaFold custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_pldddt_fractions_sum(self):
        """Check that pLDDT confidence fractions sum to approximately 1.0"""
        invalid_count = 0
        checked_count = 0

        for test_id in self.runner.test_ids[:10]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    alphafold_attr = result.get("Attributes", {}).get("Alphafold", {})
                    if alphafold_attr:
                        checked_count += 1
                        fractions_sum = (
                            alphafold_attr.get("fraction_pldddt_very_high", 0) +
                            alphafold_attr.get("fraction_pldddt_confident", 0) +
                            alphafold_attr.get("fraction_pldddt_low", 0) +
                            alphafold_attr.get("fraction_pldddt_very_low", 0)
                        )
                        # Allow small floating-point error
                        if abs(fractions_sum - 1.0) > 0.01:
                            invalid_count += 1

        if checked_count == 0:
            return False, "No AlphaFold attributes found"

        if invalid_count > 0:
            return False, f"Found {invalid_count}/{checked_count} entries with invalid fraction sums"

        return True, f"All {checked_count} entries have valid fraction sums (≈1.0)"

    @test
    def test_global_metric_range(self):
        """Check that global metric (average pLDDT) is in valid range 0-100"""
        out_of_range_count = 0
        checked_count = 0

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    alphafold_attr = result.get("Attributes", {}).get("Alphafold", {})
                    if alphafold_attr and "global_metric" in alphafold_attr:
                        checked_count += 1
                        global_metric = alphafold_attr["global_metric"]
                        if not (0 <= global_metric <= 100):
                            out_of_range_count += 1

        if checked_count == 0:
            return False, "No AlphaFold global metrics found"

        if out_of_range_count > 0:
            return False, f"Found {out_of_range_count}/{checked_count} entries with invalid global_metric"

        return True, f"All {checked_count} entries have valid global_metric (0-100)"

    @test
    def test_model_id_format(self):
        """Check that model entity IDs follow expected format: AF-{UniProtID}-F{N}"""
        invalid_format_count = 0
        checked_count = 0

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    alphafold_attr = result.get("Attributes", {}).get("Alphafold", {})
                    if alphafold_attr and "model_entity_id" in alphafold_attr:
                        checked_count += 1
                        model_id = alphafold_attr["model_entity_id"]
                        # Expected format: AF-{UniProtID}-F{fragment_number}
                        if not (model_id.startswith("AF-") and "-F" in model_id):
                            invalid_format_count += 1

        if checked_count == 0:
            return False, "No AlphaFold model IDs found"

        if invalid_format_count > 0:
            return False, f"Found {invalid_format_count}/{checked_count} entries with invalid model_id format"

        return True, f"All {checked_count} model IDs follow expected format"

    @test
    def test_alphafold_model_id_keyword_lookup(self):
        """Check that AlphaFold model IDs work as keywords to find entries"""
        # Find an AlphaFold entry and extract its model ID
        model_id = None
        original_uniprot_id = None

        for test_id in self.runner.test_ids[:10]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    alphafold_attr = result.get("Attributes", {}).get("Alphafold", {})
                    if alphafold_attr and alphafold_attr.get("model_entity_id"):
                        model_id = alphafold_attr["model_entity_id"]
                        original_uniprot_id = test_id
                        break
            if model_id:
                break

        if not model_id:
            return False, "No AlphaFold model IDs found in test data"

        # Query by model ID (should resolve via keyword)
        model_data = self.runner.lookup(model_id)

        if not model_data or not model_data.get("results"):
            return False, f"AlphaFold model ID {model_id} did not resolve to any entry"

        # Check if it resolves to AlphaFold dataset with proper attributes
        for result in model_data["results"]:
            if result.get("dataset") == 30:  # 30 is AlphaFold dataset ID
                # Validate attributes using helper
                valid, msg = self.runner.validate_attributes(result, "Alphafold", ["model_entity_id"])
                if not valid:
                    return False, f"Model ID {model_id} resolved but {msg}"

                return True, f"Model ID {model_id} successfully resolves to AlphaFold entry with valid attributes"

        return False, f"Model ID {model_id} did not resolve to AlphaFold dataset"

    @test
    def test_uniprot_alphafold_cooccurrence(self):
        """Check that querying UniProt ID returns both UniProt and AlphaFold datasets"""
        checked_count = 0
        missing_both = 0

        for test_id in self.runner.test_ids[:10]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                checked_count += 1

                # Check if both AlphaFold (30) and UniProt (1) are present
                datasets = {result.get("dataset") for result in data["results"]}

                if 30 not in datasets or 1 not in datasets:
                    missing_both += 1

        if checked_count == 0:
            return False, "No results found to check"

        if missing_both > 0:
            return False, f"Found {missing_both}/{checked_count} queries missing both datasets"

        return True, f"All {checked_count} UniProt IDs return both AlphaFold and UniProt datasets"

    @test
    def test_model_id_resolves_to_uniprot(self):
        """Check that AlphaFold model IDs resolve to correct UniProt entries"""
        # Query a UniProt ID to get its model ID
        uniprot_id = self.runner.test_ids[0] if self.runner.test_ids else None

        if not uniprot_id:
            return False, "No test IDs available"

        # Get AlphaFold data
        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No data found for UniProt ID {uniprot_id}"

        # Find AlphaFold model ID
        model_id = None
        for result in data["results"]:
            alphafold_attr = result.get("Attributes", {}).get("Alphafold", {})
            if alphafold_attr:
                model_id = alphafold_attr.get("model_entity_id")
                break

        if not model_id:
            return False, f"No AlphaFold model ID found for {uniprot_id}"

        # Query by model ID - should resolve to AlphaFold entry for same UniProt ID
        model_data = self.runner.lookup(model_id)

        if not model_data or not model_data.get("results"):
            return False, f"Model ID {model_id} did not resolve to any entries"

        # Check if AlphaFold result has correct UniProt ID
        for result in model_data["results"]:
            if result.get("dataset") == 30:  # AlphaFold dataset
                if result.get("identifier") == uniprot_id:
                    return True, f"Model ID {model_id} correctly resolves to UniProt {uniprot_id}"

        return False, f"Model {model_id} does not resolve to correct UniProt ID {uniprot_id}"

    @test
    def test_high_confidence_structures(self):
        """Check distribution of high-confidence structures (global_metric > 70)"""
        high_confidence_count = 0
        checked_count = 0

        for test_id in self.runner.test_ids:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    alphafold_attr = result.get("Attributes", {}).get("Alphafold", {})
                    if alphafold_attr and "global_metric" in alphafold_attr:
                        checked_count += 1
                        if alphafold_attr["global_metric"] > 70:
                            high_confidence_count += 1

        if checked_count == 0:
            return False, "No AlphaFold entries found"

        percentage = (high_confidence_count / checked_count) * 100

        # Most Swiss-Prot entries should have reasonably high confidence
        if percentage < 50:
            return False, f"Only {high_confidence_count}/{checked_count} ({percentage:.1f}%) have high confidence"

        return True, f"{high_confidence_count}/{checked_count} ({percentage:.1f}%) entries have high confidence (>70 pLDDT)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Use generated test IDs from test_out directory (created by test mode build)
    test_ids_file = script_dir / "../../test_out/reference/alphafold_ids.txt"
    if not test_ids_file.exists():
        test_ids_file = Path("test_out/reference/alphafold_ids.txt")

    # Fallback to static test IDs
    if not test_ids_file.exists():
        test_ids_file = script_dir / "alphafold_ids.txt"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not test_ids_file.exists():
        print(f"Error: {test_ids_file} not found")
        print("Run: ./biobtree -d alphafold test")
        return 1

    # Create test runner
    runner = TestRunner(api_url, reference_file if reference_file.exists() else None, test_cases_file)

    # Load test IDs
    with open(test_ids_file, 'r') as f:
        runner.test_ids = [line.strip() for line in f if line.strip()]

    # Add custom tests
    custom_tests = AlphaFoldTests(runner)
    for test_method in [
        custom_tests.test_pldddt_fractions_sum,
        custom_tests.test_global_metric_range,
        custom_tests.test_model_id_format,
        custom_tests.test_alphafold_model_id_keyword_lookup,
        custom_tests.test_uniprot_alphafold_cooccurrence,
        custom_tests.test_high_confidence_structures
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
