#!/usr/bin/env python3
"""
Test Runner Framework

Main framework for running declarative tests and custom test functions.
Supports both JSON-defined tests and Python test functions.
"""

import json
from pathlib import Path
from typing import Dict, List, Callable, Tuple, Optional
from .test_types import create_test
from .query_helpers import QueryHelper


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

    def load_reference_data(self):
        """Load reference data from JSON file"""
        with open(self.reference_file) as f:
            data = json.load(f)
            self.reference_data = data.get("entries", [])
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
