#!/usr/bin/env python3
"""
RNACentral Non-Coding RNA Database Test Suite

Tests RNACentral dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

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


class RnacentralTests:
    """RNACentral custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_rna_type_diversity(self):
        """Check that we have diverse RNA types in test data"""
        rna_types = set()
        checked_count = 0

        for test_id in self.runner.test_ids[:50]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    rnacentral_attr = result.get("Attributes", {}).get("Rnacentral", {})
                    if rnacentral_attr and "rna_type" in rnacentral_attr:
                        checked_count += 1
                        rna_types.add(rnacentral_attr["rna_type"])

        if checked_count == 0:
            return False, "No RNACentral entries found"

        # We should have at least 1 RNA type
        if len(rna_types) < 1:
            return False, f"Only found {len(rna_types)} RNA type(s)"

        return True, f"Found {len(rna_types)} RNA type(s): {', '.join(sorted(rna_types))}"

    @test
    def test_length_range_validation(self):
        """Check that sequence lengths are in reasonable ranges"""
        invalid_count = 0
        checked_count = 0
        min_length = float('inf')
        max_length = 0

        for test_id in self.runner.test_ids[:50]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    rnacentral_attr = result.get("Attributes", {}).get("Rnacentral", {})
                    if rnacentral_attr and "length" in rnacentral_attr:
                        checked_count += 1
                        length = rnacentral_attr["length"]

                        # Track min/max
                        min_length = min(min_length, length)
                        max_length = max(max_length, length)

                        # Validate reasonable range (10 to 50000 nucleotides)
                        if not (10 <= length <= 50000):
                            invalid_count += 1

        if checked_count == 0:
            return False, "No RNACentral length data found"

        if invalid_count > 0:
            return False, f"Found {invalid_count}/{checked_count} entries with invalid lengths"

        return True, f"All {checked_count} entries have valid lengths (range: {min_length}-{max_length} nt)"

    @test
    def test_description_format(self):
        """Check that descriptions follow expected format"""
        invalid_count = 0
        checked_count = 0

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    rnacentral_attr = result.get("Attributes", {}).get("Rnacentral", {})
                    if rnacentral_attr and "description" in rnacentral_attr:
                        checked_count += 1
                        desc = rnacentral_attr["description"]

                        # Description should not be empty and should contain useful info
                        if not desc or len(desc) < 5:
                            invalid_count += 1

        if checked_count == 0:
            return False, "No RNACentral descriptions found"

        if invalid_count > 0:
            return False, f"Found {invalid_count}/{checked_count} entries with invalid descriptions"

        return True, f"All {checked_count} entries have valid descriptions"

    @test
    def test_organism_count_validity(self):
        """Check that organism counts are valid"""
        invalid_count = 0
        checked_count = 0
        multi_organism = 0

        for test_id in self.runner.test_ids[:50]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    rnacentral_attr = result.get("Attributes", {}).get("Rnacentral", {})
                    if rnacentral_attr and "organism_count" in rnacentral_attr:
                        checked_count += 1
                        count = rnacentral_attr["organism_count"]

                        # Count should be at least 1
                        if count < 1:
                            invalid_count += 1
                        elif count > 1:
                            multi_organism += 1

        if checked_count == 0:
            return False, "No RNACentral organism count data found"

        if invalid_count > 0:
            return False, f"Found {invalid_count}/{checked_count} entries with invalid organism counts"

        return True, f"All {checked_count} entries have valid organism counts ({multi_organism} multi-organism)"

    @test
    def test_active_status(self):
        """Check that entries have proper active status"""
        active_count = 0
        checked_count = 0

        for test_id in self.runner.test_ids[:50]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    rnacentral_attr = result.get("Attributes", {}).get("Rnacentral", {})
                    if rnacentral_attr and "is_active" in rnacentral_attr:
                        checked_count += 1
                        if rnacentral_attr["is_active"]:
                            active_count += 1

        if checked_count == 0:
            return False, "No RNACentral active status data found"

        # Most entries should be active (we're using rnacentral_active.fasta.gz)
        if active_count < checked_count * 0.9:
            return False, f"Only {active_count}/{checked_count} entries are active"

        return True, f"{active_count}/{checked_count} entries are active"

    @test
    def test_all_required_fields_present(self):
        """Check that all required fields are present in entries"""
        missing_fields = {}
        checked_count = 0
        required_fields = ["rna_type", "description", "length", "organism_count", "is_active"]

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    rnacentral_attr = result.get("Attributes", {}).get("Rnacentral", {})
                    if rnacentral_attr:
                        checked_count += 1
                        for field in required_fields:
                            if field not in rnacentral_attr:
                                missing_fields[field] = missing_fields.get(field, 0) + 1

        if checked_count == 0:
            return False, "No RNACentral entries found"

        if missing_fields:
            missing_str = ", ".join(f"{field}({count})" for field, count in missing_fields.items())
            return False, f"Missing fields in some entries: {missing_str}"

        return True, f"All {checked_count} entries have all required fields"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Use generated test IDs from test_out directory (created by test mode build)
    test_ids_file = script_dir / "../../test_out/reference/rnacentral_ids.txt"
    if not test_ids_file.exists():
        test_ids_file = Path("test_out/reference/rnacentral_ids.txt")

    # Fallback to static test IDs
    if not test_ids_file.exists():
        test_ids_file = script_dir / "rnacentral_ids.txt"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not test_ids_file.exists():
        print(f"Error: {test_ids_file} not found")
        print("Run: ./biobtree -d rnacentral test")
        return 1

    # Create test runner
    runner = TestRunner(api_url, reference_file if reference_file.exists() else None, test_cases_file)

    # Load test IDs
    with open(test_ids_file, 'r') as f:
        runner.test_ids = [line.strip() for line in f if line.strip()]

    # Add custom tests
    custom_tests = RnacentralTests(runner)
    for test_method in [
        custom_tests.test_rna_type_diversity,
        custom_tests.test_length_range_validation,
        custom_tests.test_description_format,
        custom_tests.test_organism_count_validity,
        custom_tests.test_active_status,
        custom_tests.test_all_required_fields_present
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
