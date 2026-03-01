#!/usr/bin/env python3
"""
Cross-Integration Test Runner

Executes integration tests from integration_tests.json

Usage:
    python3 run_integration_tests.py [--server URL] [--verbose] [--category CAT]
"""

import json
import requests
import sys
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
    def __init__(self, test_file: str, server_url: str = None, verbose: bool = False, category: str = None, use_mcp: bool = False):
        self.test_file = Path(test_file)
        self.tests = self.load_tests()
        self.server = server_url or self.tests['metadata']['server']
        self.verbose = verbose
        self.category = category
        self.use_mcp = use_mcp  # Use MCP server API endpoints instead of biobtree direct
        self.results = []

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

        # Count tests - validation tests count as 1 per validation check
        total_identifiers = 0
        for test in tests_to_run:
            # Skip comment entries
            if 'name' not in test:
                continue
            if test.get('type') == 'validation':
                total_identifiers += len(test.get('validations', []))
            else:
                total_identifiers += len(test.get('should_pass', [])) + len(test.get('should_fail', []))

        category_msg = f" (category: {self.category})" if self.category else ""
        print(f"\n{Colors.BOLD}Running {total_identifiers} integration tests{category_msg}...{Colors.END}\n")

        for test in tests_to_run:
            # Skip comment entries (no 'name' field)
            if 'name' not in test:
                continue
            if test.get('type') == 'validation':
                self.run_validation_test(test)
            else:
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

    def run_validation_test(self, test):
        """Execute a validation test - checks data attributes, not just mapping existence"""
        import time

        print(f"{Colors.BLUE}[{test['name']}]{Colors.END} {test.get('query', '')} (validation)")
        if self.verbose:
            print(f"  Why: {test['why']}")

        identifier = test['identifier']
        query = test.get('query', '')
        dataset_id = test.get('dataset_id')
        filter_dataset = test.get('filter_dataset')  # Dataset name to filter results

        # Check if test needs xref_count validation (requires entry endpoint for detailed xrefs)
        needs_entry_endpoint = any('xref_count' in v for v in test.get('validations', []))

        # Fetch data
        try:
            if self.use_mcp:
                # MCP server API endpoints (same params as biobtree)
                if query == '':
                    params = {'i': identifier}
                    if filter_dataset:
                        params['s'] = filter_dataset
                    url = f"{self.server}/api/search"
                else:
                    params = {'i': identifier, 'm': query}
                    url = f"{self.server}/api/map"
            else:
                # Biobtree direct endpoints
                if needs_entry_endpoint and query == '':
                    # Use entry endpoint for xref_count validations (returns detailed xrefs)
                    params = {'i': identifier}
                    if filter_dataset:
                        params['s'] = filter_dataset
                    url = f"{self.server}/ws/entry/"
                elif query == '':
                    params = {'i': identifier}
                    if filter_dataset:
                        params['s'] = filter_dataset
                    url = f"{self.server}/ws/"
                else:
                    params = {'i': identifier, 'm': query}
                    # Use lite mode for mapping_total validations (returns stats.total)
                    needs_mapping_total = any('mapping_total' in v for v in test.get('validations', []))
                    if needs_mapping_total:
                        params['mode'] = 'lite'
                    url = f"{self.server}/ws/map/"

            start_time = time.time()
            response = requests.get(url, params=params, timeout=60)
            elapsed_ms = (time.time() - start_time) * 1000

            response.raise_for_status()
            data = response.json()
            full_url = response.url
        except Exception as e:
            # All validations fail if we can't fetch data
            for validation in test.get('validations', []):
                result = {
                    'test_name': test['name'],
                    'query': query,
                    'identifier': identifier,
                    'expected_pass': True,
                    'passed': False,
                    'error': str(e),
                    'url': f"{url}?{urlencode(params)}",
                    'why': test['why'],
                    'validation_desc': validation.get('description', 'Unknown'),
                    'response_time_ms': 0
                }
                self.results.append(result)
                self.print_validation_result(result)
            return

        # Find the entry to validate
        entry = None
        expected_identifier = test.get('expected_identifier')

        # Entry endpoint returns data directly, not wrapped in results
        if needs_entry_endpoint and 'identifier' in data and 'xrefs' in data:
            entry = data
        elif data.get('results'):
            if expected_identifier:
                # Find by expected identifier (exact match)
                for r in data['results']:
                    if r.get('identifier') == expected_identifier:
                        entry = r
                        break
                    # Also check source.identifier for mapping results
                    if r.get('source', {}).get('identifier') == expected_identifier:
                        entry = r
                        break
            elif dataset_id:
                # Find by dataset ID
                for r in data['results']:
                    if r.get('dataset') == dataset_id:
                        entry = r
                        break
            else:
                # Use first result's source for mapping results
                entry = data['results'][0]

        # Run each validation
        for validation in test.get('validations', []):
            result = self.run_single_validation(test, entry, data, validation, full_url, elapsed_ms)
            self.results.append(result)
            self.print_validation_result(result)

    def run_single_validation(self, test, entry, data, validation, url, elapsed_ms=0):
        """Run a single validation check"""
        path = validation.get('path', '')
        desc = validation.get('description', path)

        result_base = {
            'test_name': test['name'],
            'query': test.get('query', ''),
            'identifier': test['identifier'],
            'expected_pass': True,
            'url': url,
            'why': test['why'],
            'validation_desc': desc,
            'response_time_ms': elapsed_ms
        }

        # Navigate path to get value
        try:
            value = self.navigate_path(entry, data, path, validation)
        except Exception as e:
            return {**result_base, 'passed': False, 'error': f"Path error: {e}"}

        # Check validation conditions
        # NOTE: xref_count and mapping_total must be checked BEFORE generic min/max
        # because they also use min/max keys but handle them specially
        if 'xref_count' in validation:
            # Check xref count for a specific dataset (e.g., "pdb" from xrefs.data: ["pdb|33", ...])
            # Used to detect xref inflation bugs where counts are ~2x actual unique entries
            dataset_name = validation['xref_count']
            xref_count = self.get_xref_count(entry, dataset_name)
            if xref_count is None:
                return {**result_base, 'passed': False, 'error': f"Xref dataset '{dataset_name}' not found"}
            if 'min' in validation and 'max' in validation:
                passed = validation['min'] <= xref_count <= validation['max']
                if not passed:
                    return {**result_base, 'passed': False, 'error': f"Xref count for '{dataset_name}' expected {validation['min']}-{validation['max']}, got {xref_count}"}
            elif 'expected' in validation:
                passed = xref_count == validation['expected']
                if not passed:
                    return {**result_base, 'passed': False, 'error': f"Xref count for '{dataset_name}' expected {validation['expected']}, got {xref_count}"}
            else:
                passed = xref_count > 0
                if not passed:
                    return {**result_base, 'passed': False, 'error': f"Xref count for '{dataset_name}' is 0"}
            return {**result_base, 'passed': True, 'has_results': True}
        elif 'mapping_total' in validation:
            # Check mapping stats.total matches expected range (validates no inflation)
            stats_total = data.get('stats', {}).get('total', 0)
            if 'min' in validation and 'max' in validation:
                passed = validation['min'] <= stats_total <= validation['max']
                if not passed:
                    return {**result_base, 'passed': False, 'error': f"Mapping total expected {validation['min']}-{validation['max']}, got {stats_total}"}
            elif 'expected' in validation:
                passed = stats_total == validation['expected']
                if not passed:
                    return {**result_base, 'passed': False, 'error': f"Mapping total expected {validation['expected']}, got {stats_total}"}
            else:
                passed = stats_total > 0
                if not passed:
                    return {**result_base, 'passed': False, 'error': f"Mapping total is 0"}
            return {**result_base, 'passed': True, 'has_results': True}
        elif 'expected' in validation:
            passed = value == validation['expected']
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Expected '{validation['expected']}', got '{value}'"}
        elif 'min' in validation and 'max' in validation:
            passed = validation['min'] <= (value or 0) <= validation['max']
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Expected {validation['min']}-{validation['max']}, got {value}"}
        elif 'min' in validation:
            passed = (value or 0) >= validation['min']
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Expected >= {validation['min']}, got {value}"}
        elif 'min_length' in validation:
            passed = value and len(str(value)) >= validation['min_length']
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Expected length >= {validation['min_length']}, got {len(str(value)) if value else 0}"}
        elif 'starts_with' in validation:
            passed = value and str(value).startswith(validation['starts_with'])
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Expected to start with '{validation['starts_with']}', got '{value}'"}
        elif 'contains' in validation:
            # Check if value contains the specified substring
            # Used for compact format validation (e.g., first km_value should contain "Homo sapiens")
            passed = value and validation['contains'] in str(value)
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Expected to contain '{validation['contains']}', got '{value[:100] if value else None}'"}
        elif 'contains_identifier' in validation:
            # Check if any target has the specified identifier
            passed = self.check_contains_identifier(data, validation['contains_identifier'])
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Target '{validation['contains_identifier']}' not found"}
        elif 'has_results' in validation:
            passed = bool(data.get('results')) == validation['has_results']
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Expected has_results={validation['has_results']}"}
        elif 'first_n_start_with' in validation:
            # Check that first N targets all start with the specified prefix
            # Used for sorting validation (e.g., human genes should appear before other species)
            prefix = validation['first_n_start_with']
            n = validation.get('n', 10)
            targets = self.get_first_n_targets(data, n)
            non_matching = [t for t in targets if not t.startswith(prefix)]
            passed = len(non_matching) == 0
            if not passed:
                return {**result_base, 'passed': False, 'error': f"First {n} targets should start with '{prefix}', found non-matching: {non_matching[:3]}"}
        elif 'first_n_not_start_with' in validation:
            # Check that first N targets do NOT start with the specified prefix
            # Used for negative sorting validation (e.g., no cattle genes in first N)
            prefix = validation['first_n_not_start_with']
            n = validation.get('n', 10)
            targets = self.get_first_n_targets(data, n)
            matching = [t for t in targets if t.startswith(prefix)]
            passed = len(matching) == 0
            if not passed:
                return {**result_base, 'passed': False, 'error': f"First {n} targets should NOT start with '{prefix}', found: {matching[:3]}"}
        elif 'descending_scores' in validation:
            # Check that scores are in descending order (for expression score sorting)
            score_path = validation['descending_scores']
            n = validation.get('n', 10)
            scores = self.get_first_n_scores(data, score_path, n)
            passed = all(scores[i] >= scores[i+1] for i in range(len(scores)-1)) if len(scores) > 1 else True
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Scores not in descending order: {scores[:5]}"}
        else:
            passed = value is not None
            if not passed:
                return {**result_base, 'passed': False, 'error': f"Value not found at path '{path}'"}

        return {**result_base, 'passed': True, 'has_results': True}

    def navigate_path(self, entry, data, path, validation):
        """Navigate a dot-separated path to get a value"""
        if not path:
            return entry

        # Special handling for "targets" - searches across all results
        if path == 'targets':
            return data.get('results', [])

        # Special handling for paths starting with "source."
        if path.startswith('source.'):
            if not entry:
                return None
            path = path[7:]  # Remove "source."
            obj = entry.get('source', {})
        # Special handling for paths starting with "entry." - use entry directly
        elif path.startswith('entry.'):
            if not entry:
                return None
            path = path[6:]  # Remove "entry."
            obj = entry
        elif entry:
            obj = entry.get('Attributes', entry)
        else:
            return None

        # Navigate path
        parts = path.split('.')
        for part in parts:
            # Handle array indexing like "km_values[0]"
            if '[' in part and ']' in part:
                key = part[:part.index('[')]
                idx_str = part[part.index('[')+1:part.index(']')]
                try:
                    idx = int(idx_str)
                except ValueError:
                    return None
                if isinstance(obj, dict):
                    obj = obj.get(key)
                    if isinstance(obj, list) and 0 <= idx < len(obj):
                        obj = obj[idx]
                    else:
                        return None
                else:
                    return None
            elif isinstance(obj, dict):
                obj = obj.get(part)
            else:
                return None
            if obj is None:
                return None

        return obj

    def check_contains_identifier(self, data, target_id):
        """Check if any result's targets contain the specified identifier"""
        for result in data.get('results', []):
            for target in result.get('targets', []):
                if target.get('identifier') == target_id:
                    return True
        return False

    def get_first_n_targets(self, data, n):
        """Get first N target identifiers from results (for sorting validation)"""
        targets = []
        for result in data.get('results', []):
            for target in result.get('targets', []):
                targets.append(target.get('identifier', ''))
                if len(targets) >= n:
                    return targets
        return targets

    def get_xref_count(self, entry, dataset_name):
        """Get xref count for a specific dataset from entry's xrefs.data array.

        xrefs.data format: ["pdb|33", "reactome|27", ...]
        Returns the count for the specified dataset, or None if not found.
        """
        if not entry:
            return None
        xrefs = entry.get('xrefs', {})
        data = xrefs.get('data', [])
        for item in data:
            if '|' in item:
                ds, count = item.split('|', 1)
                if ds == dataset_name:
                    try:
                        return int(count)
                    except ValueError:
                        return None
        return None

    def get_first_n_scores(self, data, score_path, n):
        """Get first N scores from results for descending order validation"""
        scores = []
        for result in data.get('results', []):
            for target in result.get('targets', []):
                # Navigate the score path (e.g., "Attributes.BgeeEvidence.expression_score")
                value = target
                for part in score_path.split('.'):
                    if isinstance(value, dict):
                        value = value.get(part)
                    else:
                        value = None
                        break
                if value is not None:
                    scores.append(float(value))
                if len(scores) >= n:
                    return scores
        return scores

    def print_validation_result(self, result):
        """Print validation test result"""
        desc = result.get('validation_desc', result['identifier'])
        if result['passed']:
            status = f"{Colors.GREEN}✓{Colors.END}"
            print(f"  {status} {desc}")
        else:
            status = f"{Colors.RED}✗{Colors.END}"
            error = result.get('error', 'Unknown failure')
            print(f"  {status} {desc} - {error}")
            if self.verbose:
                print(f"      URL: {result['url']}")

    def run_query(self, test, identifier, expected_pass):
        """Execute a single query"""
        import time

        # Choose endpoint based on query type and server mode
        filter_dataset = test.get('filter_dataset')
        if self.use_mcp:
            # MCP server API endpoints (same params as biobtree)
            if test['query'] == '':
                params = {'i': identifier}
                if filter_dataset:
                    params['s'] = filter_dataset
                url = f"{self.server}/api/search"
            else:
                params = {'i': identifier, 'm': test['query']}
                url = f"{self.server}/api/map"
        else:
            # Biobtree direct endpoints
            if test['query'] == '':
                params = {'i': identifier}
                if filter_dataset:
                    params['s'] = filter_dataset
                url = f"{self.server}/ws/"
            else:
                params = {'i': identifier, 'm': test['query']}
                url = f"{self.server}/ws/map/"

        try:
            start_time = time.time()
            response = requests.get(url, params=params, timeout=30)
            elapsed_ms = (time.time() - start_time) * 1000

            response.raise_for_status()
            data = response.json()

            # Store full URL for debugging
            full_url = response.url

            # Check for results (full mode) or mappings (lite mode)
            has_results = (
                ('results' in data and len(data.get('results', [])) > 0) or
                ('mappings' in data and len(data.get('mappings', [])) > 0)
            )

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
                'why': test['why'],
                'response_time_ms': elapsed_ms
            }

        except Exception as e:
            # If we expected this to fail and got an HTTP error, that's a pass
            # (e.g., invalid dataset should return 400 error)
            passed = not expected_pass
            return {
                'test_name': test['name'],
                'query': test['query'],
                'identifier': identifier,
                'expected_pass': expected_pass,
                'passed': passed,
                'error': str(e) if not passed else None,
                'expected_error': str(e) if passed else None,
                'url': f"{url}?{urlencode(params)}",
                'why': test['why'],
                'response_time_ms': 0
            }

    def print_result(self, result):
        """Print test result"""
        elapsed = result.get('response_time_ms', 0)
        time_str = self.format_time(elapsed)

        if result['passed']:
            status = f"{Colors.GREEN}✓{Colors.END}"
            expected = "should pass" if result['expected_pass'] else "should fail"
            print(f"  {status} {result['identifier']} ({expected}) {time_str}")
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

            print(f"  {status} {result['identifier']} - {reason} {time_str}")
            if self.verbose:
                print(f"      URL: {result['url']}")

    def format_time(self, elapsed_ms):
        """Format response time with color coding"""
        if elapsed_ms == 0:
            return ""
        elif elapsed_ms < 100:
            return f"{Colors.GREEN}[{elapsed_ms:.0f}ms]{Colors.END}"
        elif elapsed_ms < 500:
            return f"{Colors.YELLOW}[{elapsed_ms:.0f}ms]{Colors.END}"
        elif elapsed_ms < 2000:
            return f"{Colors.YELLOW}[{elapsed_ms/1000:.1f}s]{Colors.END}"
        else:
            return f"{Colors.RED}[{elapsed_ms/1000:.1f}s]{Colors.END}"

    def print_summary(self):
        """Print summary to console"""
        total = len(self.results)
        passed = sum(1 for r in self.results if r['passed'])
        failed = total - passed

        # Timing stats
        times = [r.get('response_time_ms', 0) for r in self.results if r.get('response_time_ms', 0) > 0]
        avg_time = sum(times) / len(times) if times else 0
        max_time = max(times) if times else 0
        total_time = sum(times)

        print(f"\n{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
        print(f"{Colors.BOLD}  Test Summary{Colors.END}")
        print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
        print(f"Total:    {total}")
        print(f"{Colors.GREEN}Passed:{Colors.END}   {passed}")
        print(f"{Colors.RED}Failed:{Colors.END}   {failed}")
        print(f"Rate:     {(passed/total*100):.1f}%")
        print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
        print(f"{Colors.BOLD}  Response Times{Colors.END}")
        print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
        print(f"Total:    {total_time/1000:.1f}s")
        print(f"Average:  {avg_time:.0f}ms")
        print(f"Max:      {max_time:.0f}ms")

        # Show slow queries (>100ms)
        slow_queries = sorted(
            [r for r in self.results if r.get('response_time_ms', 0) > 100],
            key=lambda x: x.get('response_time_ms', 0),
            reverse=True
        )

        if slow_queries:
            print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
            print(f"{Colors.YELLOW}  Slow Queries (>100ms){Colors.END}")
            print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
            for r in slow_queries[:15]:  # Top 15 slowest
                elapsed = r.get('response_time_ms', 0)
                identifier = r.get('identifier', r.get('validation_desc', '?'))
                query = r.get('query', '') or '(lookup)'
                if elapsed >= 1000:
                    print(f"  {Colors.RED}{elapsed/1000:.1f}s{Colors.END}  {identifier} | {query}")
                elif elapsed >= 500:
                    print(f"  {Colors.YELLOW}{elapsed:.0f}ms{Colors.END} {identifier} | {query}")
                else:
                    print(f"  {elapsed:.0f}ms {identifier} | {query}")

        # Show failed tests
        failed_tests = [r for r in self.results if not r['passed']]
        if failed_tests:
            print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
            print(f"{Colors.RED}  Failed Tests ({len(failed_tests)}){Colors.END}")
            print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}")
            for r in failed_tests:
                identifier = r.get('identifier', r.get('validation_desc', '?'))
                query = r.get('query', '') or '(lookup)'
                error = r.get('error', '')
                if not error:
                    if r.get('expected_pass') and not r.get('has_results'):
                        error = "Expected results but got none"
                    elif not r.get('expected_pass') and r.get('has_results'):
                        error = "Expected no results but got data"
                print(f"  {Colors.RED}✗{Colors.END} {identifier} | {query}")
                if error:
                    print(f"    {error}")

        print(f"{Colors.BOLD}═══════════════════════════════════════{Colors.END}\n")


def main():
    parser = argparse.ArgumentParser(description='Run biobtree integration tests')
    parser.add_argument('test_file', nargs='?', help='Test file (default: integration_tests.json)', default='integration_tests.json')
    parser.add_argument('--server', help='Server URL', default=None)
    parser.add_argument('--verbose', '-v', help='Verbose output', action='store_true')
    parser.add_argument('--category', '-c', help='Run only tests in specified category', default=None)
    parser.add_argument('--list-categories', help='List available test categories', action='store_true')
    parser.add_argument('--mcp', help='Use MCP server API endpoints instead of biobtree direct', action='store_true')

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

    runner = IntegrationTestRunner(test_file, args.server, args.verbose, args.category, args.mcp)

    try:
        runner.run_all()
    except KeyboardInterrupt:
        print(f"\n{Colors.YELLOW}⚠{Colors.END} Tests interrupted")
        sys.exit(1)

    runner.print_summary()

    sys.exit(0 if all(r['passed'] for r in runner.results) else 1)


if __name__ == '__main__':
    main()
