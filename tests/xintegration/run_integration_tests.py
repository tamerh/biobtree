#!/usr/bin/env python3
"""
Simple Cross-Integration Test Runner

Executes integration tests from integration_tests.json

Usage:
    python3 run_integration_tests.py [--server URL] [--verbose]
"""

import json
import requests
import sys
from datetime import datetime
from pathlib import Path
import argparse
from urllib.parse import urlencode


class Colors:
    GREEN = '\033[92m'
    RED = '\033[91m'
    YELLOW = '\033[93m'
    BLUE = '\033[94m'
    BOLD = '\033[1m'
    END = '\033[0m'


class IntegrationTestRunner:
    def __init__(self, test_file: str, server_url: str = None, verbose: bool = False, category: str = None):
        self.test_file = Path(test_file)
        self.tests = self.load_tests()
        self.server = server_url or self.tests['metadata']['server']
        self.verbose = verbose
        self.category = category
        self.results = []
        self.start_time = datetime.now()

    def load_tests(self):
        with open(self.test_file, 'r') as f:
            return json.load(f)

    def get_categories(self):
        """Get available test categories"""
        return self.tests.get('test_categories', {})

    def run_all(self):
        """Execute all test cases"""
        tests_to_run = self.tests['tests']

        # Filter by category if specified
        if self.category:
            tests_to_run = [t for t in tests_to_run if t.get('category') == self.category]
            if not tests_to_run:
                print(f"{Colors.RED}No tests found for category: {self.category}{Colors.END}")
                print(f"Available categories: {', '.join(self.get_categories().keys())}")
                return

        total_identifiers = sum(
            len(test['should_pass']) + len(test['should_fail'])
            for test in tests_to_run
        )

        category_msg = f" (category: {self.category})" if self.category else ""
        print(f"\n{Colors.BOLD}Running {total_identifiers} integration tests{category_msg}...{Colors.END}\n")

        for test in tests_to_run:
            self.run_test(test)

    def run_test(self, test):
        """Execute a test case"""
        print(f"{Colors.BLUE}[{test['name']}]{Colors.END} {test['query']}")
        if self.verbose:
            print(f"  Why: {test['why']}")

        # Test identifiers that should pass
        for identifier in test['should_pass']:
            result = self.run_query(test, identifier, expected_pass=True)
            self.results.append(result)
            self.print_result(result)

        # Test identifiers that should fail
        for identifier in test['should_fail']:
            result = self.run_query(test, identifier, expected_pass=False)
            self.results.append(result)
            self.print_result(result)

    def run_query(self, test, identifier, expected_pass):
        """Execute a single query"""
        # Choose endpoint based on query type
        if test['query'] == '':
            # Empty query = lookup using search endpoint
            params = {'i': identifier}
            url = f"{self.server}/ws/"
        else:
            # Non-empty query = mapping endpoint
            params = {'i': identifier, 'm': test['query']}
            url = f"{self.server}/ws/map/"

        try:
            response = requests.get(url, params=params, timeout=30)
            response.raise_for_status()
            data = response.json()

            # Store full URL for debugging
            full_url = response.url

            has_results = 'results' in data and len(data.get('results', [])) > 0

            # Test passes if: (expected to pass AND has results) OR (expected to fail AND no results)
            passed = (expected_pass == has_results)

            return {
                'test_name': test['name'],
                'query': test['query'],
                'identifier': identifier,
                'expected_pass': expected_pass,
                'has_results': has_results,
                'passed': passed,
                'url': full_url,
                'response': data,
                'why': test['why']
            }

        except Exception as e:
            return {
                'test_name': test['name'],
                'query': test['query'],
                'identifier': identifier,
                'expected_pass': expected_pass,
                'passed': False,
                'error': str(e),
                'url': f"{url}?{urlencode(params)}",
                'why': test['why']
            }

    def print_result(self, result):
        """Print test result"""
        if result['passed']:
            status = f"{Colors.GREEN}✓{Colors.END}"
            expected = "should pass" if result['expected_pass'] else "should fail"
            print(f"  {status} {result['identifier']} ({expected})")
        else:
            status = f"{Colors.RED}✗{Colors.END}"
            if 'error' in result:
                reason = f"Error: {result['error']}"
            elif result['expected_pass'] and not result.get('has_results'):
                reason = "Expected results but got none"
            elif not result['expected_pass'] and result.get('has_results'):
                reason = "Expected no results but got data"
            else:
                reason = "Unknown failure"

            print(f"  {status} {result['identifier']} - {reason}")
            if self.verbose:
                print(f"      URL: {result['url']}")

    def generate_report(self, report_dir: str = 'reports'):
        """Generate markdown report"""
        report_path = Path(report_dir) / f"{self.start_time.strftime('%Y-%m-%d_%H%M')}_analysis.md"
        report_path.parent.mkdir(exist_ok=True, parents=True)

        with open(report_path, 'w') as f:
            f.write(self.format_report())

        return report_path

    def format_report(self):
        """Format results as markdown"""
        total = len(self.results)
        passed = sum(1 for r in self.results if r['passed'])
        failed = total - passed
        pass_rate = (passed / total * 100) if total > 0 else 0

        report = f"""# Integration Test Analysis

**Date**: {self.start_time.strftime('%Y-%m-%d %H:%M:%S')}
**Server**: {self.server}
**Tests**: {total}
**Passed**: {passed} ({pass_rate:.1f}%)
**Failed**: {failed} ({100-pass_rate:.1f}%)

---

## ✅ Passing Tests

"""
        # Group by test name
        by_test = {}
        for r in self.results:
            name = r['test_name']
            if name not in by_test:
                by_test[name] = {'passed': [], 'failed': []}

            if r['passed']:
                by_test[name]['passed'].append(r)
            else:
                by_test[name]['failed'].append(r)

        for test_name, results in by_test.items():
            if results['passed']:
                first = results['passed'][0]
                report += f"### {test_name}\n"
                report += f"**Why**: {first['why']}\n"
                report += f"**Query**: `{first['query']}`\n\n"
                for r in results['passed']:
                    expected = "pass" if r['expected_pass'] else "fail (correctly)"
                    report += f"- ✓ `{r['identifier']}` (expected to {expected})\n"
                report += "\n"

        report += "\n---\n\n## ❌ Failing Tests\n\n"

        failures_found = False
        for test_name, results in by_test.items():
            if results['failed']:
                failures_found = True
                first = results['failed'][0]
                report += f"### {test_name}\n"
                report += f"**Why**: {first['why']}\n"
                report += f"**Query**: `{first['query']}`\n\n"

                for r in results['failed']:
                    expected = "pass" if r['expected_pass'] else "fail"
                    if 'error' in r:
                        reason = f"Error: {r['error']}"
                    elif r['expected_pass'] and not r.get('has_results'):
                        reason = "Expected results but got none"
                    elif not r['expected_pass'] and r.get('has_results'):
                        reason = "Expected no results but got data"
                    else:
                        reason = "Unknown failure"

                    report += f"- ✗ `{r['identifier']}` (expected to {expected})\n"
                    report += f"  - **Failure**: {reason}\n"
                    report += f"  - **URL**: {r['url']}\n\n"

        if not failures_found:
            report += "_No failures! All tests passed._\n\n"

        report += f"""
---

## 📊 Summary

**Total Queries**: {total}
**Success Rate**: {pass_rate:.1f}%
**Execution Time**: {(datetime.now() - self.start_time).total_seconds():.2f}s

---

## 📝 Test Coverage

"""
        for test in self.tests['tests']:
            test_results = [r for r in self.results if r['test_name'] == test['name']]
            test_passed = sum(1 for r in test_results if r['passed'])
            test_total = len(test_results)
            report += f"- **{test['name']}**: {test_passed}/{test_total} passed\n"

        return report

    def print_summary(self):
        """Print summary to console"""
        total = len(self.results)
        passed = sum(1 for r in self.results if r['passed'])
        failed = total - passed

        print(f"\n{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
        print(f"{Colors.BOLD}  Test Summary{Colors.END}")
        print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
        print(f"Total:    {total}")
        print(f"{Colors.GREEN}Passed:{Colors.END}   {passed}")
        print(f"{Colors.RED}Failed:{Colors.END}   {failed}")
        print(f"Rate:     {(passed/total*100):.1f}%")
        print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}\n")


def main():
    parser = argparse.ArgumentParser(description='Run biobtree integration tests')
    parser.add_argument('test_file', nargs='?', help='Test file (default: integration_tests.json)', default='integration_tests.json')
    parser.add_argument('--server', help='Server URL', default=None)
    parser.add_argument('--verbose', '-v', help='Verbose output', action='store_true')
    parser.add_argument('--no-report', help='Skip report generation', action='store_true')
    parser.add_argument('--category', '-c', help='Run only tests in specified category', default=None)
    parser.add_argument('--list-categories', help='List available test categories', action='store_true')

    args = parser.parse_args()

    script_dir = Path(__file__).parent
    test_file = script_dir / args.test_file

    # Handle --list-categories
    if args.list_categories:
        with open(test_file, 'r') as f:
            tests = json.load(f)
        categories = tests.get('test_categories', {})
        print(f"\n{Colors.BOLD}Available Test Categories:{Colors.END}\n")
        for cat, desc in categories.items():
            # Count tests in this category
            count = sum(1 for t in tests['tests'] if t.get('category') == cat)
            print(f"  {Colors.BLUE}{cat}{Colors.END}: {desc} ({count} tests)")
        print()
        return

    runner = IntegrationTestRunner(test_file, args.server, args.verbose, args.category)

    try:
        runner.run_all()
    except KeyboardInterrupt:
        print(f"\n{Colors.YELLOW}⚠{Colors.END} Tests interrupted")
        sys.exit(1)

    runner.print_summary()

    if not args.no_report:
        report_path = runner.generate_report()
        print(f"{Colors.GREEN}✓{Colors.END} Report: {report_path}")

    sys.exit(0 if all(r['passed'] for r in runner.results) else 1)


if __name__ == '__main__':
    main()
