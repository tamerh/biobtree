#!/usr/bin/env python3
"""
Test Runner Framework

Main framework for running declarative tests and custom test functions.
Supports both JSON-defined tests and Python test functions.
"""

import json
from pathlib import Path
from typing import Dict, List, Callable, Tuple, Optional

# Handle both relative and absolute imports (for direct execution and subprocess)
try:
    from .test_types import create_test
    from .query_helpers import QueryHelper
except ImportError:
    from test_types import create_test
    from query_helpers import QueryHelper


def load_dataset_mappings():
    """
    Load dataset name-to-ID mappings from configuration files.

    Dynamically loads from:
    - conf/source.dataset.json
    - conf/default.dataset.json
    - conf/optional.dataset.json

    Returns:
        tuple: (name_to_id dict, id_to_name dict, aliases_to_id dict)

    Example:
        name_to_id["uniprot"] -> 1
        id_to_name[1] -> "uniprot"
        aliases_to_id["uniprotkb"] -> 1
    """
    conf_dir = Path(__file__).parent.parent.parent / "conf"

    name_to_id = {}
    id_to_name = {}
    aliases_to_id = {}

    config_files = [
        conf_dir / "source.dataset.json",
        conf_dir / "default.dataset.json",
        conf_dir / "optional.dataset.json"
    ]

    for config_file in config_files:
        if not config_file.exists():
            continue

        try:
            with open(config_file) as f:
                datasets = json.load(f)

            for dataset_name, config in datasets.items():
                dataset_id = int(config.get("id", 0))

                if dataset_id == 0:
                    continue

                # Store primary name (lowercase for case-insensitive lookup)
                name_to_id[dataset_name.lower()] = dataset_id
                id_to_name[dataset_id] = dataset_name.lower()

                # Store aliases (case-insensitive)
                if "aliases" in config:
                    aliases = config["aliases"].split(",")
                    for alias in aliases:
                        alias = alias.strip().lower()
                        if alias:
                            aliases_to_id[alias] = dataset_id

        except Exception as e:
            print(f"Warning: Failed to load {config_file.name}: {e}")
            continue

    return name_to_id, id_to_name, aliases_to_id


class Colors:
    GREEN = '\033[0;32m'
    RED = '\033[0;31m'
    YELLOW = '\033[1;33m'
    BLUE = '\033[0;34m'
    NC = '\033[0m'


class TestRunner:
    """Main test runner that executes both declarative and custom tests"""

    def __init__(self, api_url: str, reference_file: Path, test_cases_file: Optional[Path] = None):
        self.api_url = api_url.rstrip('/')
        self.reference_file = reference_file
        self.test_cases_file = test_cases_file
        self.reference_data = None
        self.test_cases = None
        self.custom_tests = []
        self.results = {
            "passed": 0,
            "failed": 0,
            "total": 0
        }
        self.failed_tests = []

        # Query helper for easy queries
        self.query = QueryHelper(api_url)

        # Load dataset mappings dynamically from config files
        self._name_to_id, self._id_to_name, self._aliases_to_id = load_dataset_mappings()

    def load_reference_data(self):
        """Load reference data from JSON file"""
        with open(self.reference_file) as f:
            data = json.load(f)
            # Support both formats: dict with "entries" key or plain list
            if isinstance(data, list):
                self.reference_data = data
            elif isinstance(data, dict):
                self.reference_data = data.get("entries", [])
            else:
                self.reference_data = []
            print(f"Loaded {len(self.reference_data)} reference entries")

    def load_test_cases(self):
        """Load declarative test cases from JSON file"""
        if not self.test_cases_file or not self.test_cases_file.exists():
            print("No test_cases.json file found - skipping declarative tests")
            self.test_cases = {"common_tests": [], "custom_tests": []}
            return

        with open(self.test_cases_file) as f:
            self.test_cases = json.load(f)
            common_count = len(self.test_cases.get("common_tests", []))
            custom_count = len(self.test_cases.get("custom_tests", []))
            print(f"Loaded {common_count} common tests and {custom_count} custom declarative tests")

    def add_custom_test(self, test_func: Callable):
        """
        Add a custom Python test function

        Test function should return (success: bool, message: str)
        """
        self.custom_tests.append(test_func)

    def run_test(self, name: str, test_func: Callable) -> bool:
        """
        Run a single test function

        Args:
            name: Test name
            test_func: Function that returns (success, message)

        Returns:
            True if test passed, False otherwise
        """
        self.results["total"] += 1
        print(f"\n{Colors.BLUE}Test {self.results['total']}:{Colors.NC} {name}")

        try:
            success, message = test_func()

            if success:
                print(f"  {Colors.GREEN}✓ PASS{Colors.NC}: {message}")
                self.results["passed"] += 1
                return True
            else:
                print(f"  {Colors.RED}✗ FAIL{Colors.NC}: {message}")
                self.results["failed"] += 1
                self.failed_tests.append((name, message))
                return False

        except Exception as e:
            print(f"  {Colors.RED}✗ ERROR{Colors.NC}: {e}")
            self.results["failed"] += 1
            self.failed_tests.append((name, str(e)))
            return False

    def run_declarative_tests(self):
        """Run all declarative tests from JSON"""
        if not self.test_cases:
            return

        # Run common tests
        for test_config in self.test_cases.get("common_tests", []):
            test = create_test(test_config)
            if test:
                self.run_test(
                    test.name,
                    lambda t=test: t.execute(self.api_url, self.reference_data)
                )

        # Run custom declarative tests
        for test_config in self.test_cases.get("custom_tests", []):
            test = create_test(test_config)
            if test:
                self.run_test(
                    test.name,
                    lambda t=test: t.execute(self.api_url, self.reference_data)
                )

    def run_custom_tests(self):
        """Run all custom Python test functions"""
        for test_func in self.custom_tests:
            test_name = test_func.__doc__ or test_func.__name__
            self.run_test(test_name, test_func)

    def run_all_tests(self):
        """Run all tests (declarative + custom)"""
        print("═" * 60)
        print("  Test Suite")
        print("═" * 60)
        print()

        # Load data
        self.load_reference_data()
        if self.test_cases_file:
            self.load_test_cases()
        print()

        # Run declarative tests
        if self.test_cases:
            self.run_declarative_tests()

        # Run custom tests
        if self.custom_tests:
            self.run_custom_tests()

    def print_summary(self) -> int:
        """
        Print test summary

        Returns:
            Exit code (0 for success, 1 for failure)
        """
        print()
        print("═" * 60)
        print("  TEST SUMMARY")
        print("═" * 60)
        print(f"Total:  {self.results['total']}")
        print(f"{Colors.GREEN}Passed: {self.results['passed']}{Colors.NC}")
        print(f"{Colors.RED}Failed: {self.results['failed']}{Colors.NC}")

        if self.failed_tests:
            print()
            print("Failed Tests:")
            for name, message in self.failed_tests:
                print(f"  - {name}: {message}")

        print("═" * 60)

        if self.results['failed'] == 0:
            print(f"{Colors.GREEN}✓ ALL TESTS PASSED{Colors.NC}")
            return 0
        else:
            print(f"{Colors.RED}✗ {self.results['failed']} TEST(S) FAILED{Colors.NC}")
            return 1

    # Convenience methods for easy querying
    def lookup(self, identifier: str):
        """Quick lookup helper"""
        return self.query.lookup(identifier)

    def lookup_symbol(self, symbol: str):
        """Quick symbol lookup helper"""
        return self.query.lookup_symbol(symbol)

    def check_xref(self, identifier: str, xref_type: str = None):
        """Quick xref check helper"""
        return self.query.check_xref(identifier, xref_type)

    def has_results(self, identifier: str) -> bool:
        """Quick check if identifier has results"""
        return self.query.has_results(identifier)

    # Cross-reference helper methods
    def get_xrefs(self, result, dataset_name=None):
        """
        Get cross-references from a result, optionally filtered by dataset name.

        Args:
            result: API result dict
            dataset_name: Optional dataset name to filter by (e.g., "taxonomy", "ensembl")
                         Case-insensitive, also accepts aliases (e.g., "uniprotkb")

        Returns:
            List of xref entries

        Example:
            taxonomy_xrefs = runner.get_xrefs(result, "taxonomy")
            ensembl_xrefs = runner.get_xrefs(result, "Ensembl")
            all_xrefs = runner.get_xrefs(result)
        """
        entries = result.get("entries", [])

        if dataset_name:
            dataset_name_lower = dataset_name.lower()

            # Try direct name lookup first
            dataset_id = self._name_to_id.get(dataset_name_lower)

            # If not found, try aliases
            if dataset_id is None:
                dataset_id = self._aliases_to_id.get(dataset_name_lower)

            if dataset_id is None:
                print(f"Warning: Unknown dataset '{dataset_name}'")
                return []

            return [x for x in entries if x.get("dataset") == dataset_id]

        return entries

    def has_xref(self, result, dataset_name, identifier=None):
        """
        Check if result has cross-reference to a dataset.

        Args:
            result: API result dict
            dataset_name: Dataset name (e.g., "taxonomy", "ensembl")
                         Case-insensitive, also accepts aliases
            identifier: Optional specific identifier to check for

        Returns:
            bool: True if xref exists

        Examples:
            # Check if any taxonomy xref exists
            if runner.has_xref(result, "taxonomy"):
                ...

            # Check for specific taxonomy ID
            if runner.has_xref(result, "taxonomy", "9606"):
                ...
        """
        xrefs = self.get_xrefs(result, dataset_name)

        if identifier:
            return any(x.get("identifier") == str(identifier) for x in xrefs)

        return len(xrefs) > 0

    def get_xref_count(self, result, dataset_name=None):
        """
        Get count of cross-references, optionally filtered by dataset.

        Args:
            result: API result dict
            dataset_name: Optional dataset name to filter by
                         Case-insensitive, also accepts aliases

        Returns:
            int: Number of xrefs

        Examples:
            total_xrefs = runner.get_xref_count(result)
            taxonomy_count = runner.get_xref_count(result, "taxonomy")
        """
        xrefs = self.get_xrefs(result, dataset_name)
        return len(xrefs)

    def get_xref_datasets(self, result):
        """
        Get list of unique dataset names that have xrefs.

        Args:
            result: API result dict

        Returns:
            list: Dataset names that have cross-references

        Example:
            datasets = runner.get_xref_datasets(result)
            # ["taxonomy", "ensembl", "go", "embl"]
        """
        entries = result.get("entries", [])
        dataset_ids = set(x.get("dataset") for x in entries if x.get("dataset"))

        # Reverse lookup to get names
        dataset_names = []
        for dataset_id in dataset_ids:
            name = self._id_to_name.get(dataset_id, f"unknown_{dataset_id}")
            dataset_names.append(name)

        return sorted(dataset_names)


def test(func):
    """
    Decorator for marking functions as tests

    Usage:
        @test
        def my_test_function(self):
            # test logic
            return True, "Test passed"
    """
    func._is_test = True
    return func


def discover_tests(test_instance) -> List[Callable]:
    """
    Discover all methods marked with @test decorator

    Args:
        test_instance: Instance to search for test methods

    Returns:
        List of test methods
    """
    tests = []
    for attr_name in dir(test_instance):
        attr = getattr(test_instance, attr_name)
        if callable(attr) and hasattr(attr, '_is_test') and attr._is_test:
            tests.append(attr)
    return tests
