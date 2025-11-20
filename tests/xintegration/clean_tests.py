#!/usr/bin/env python3
"""
Remove lookup-only queries from integration tests
These are queries with single >> that don't actually test mapping functionality
"""

import json

def main():
    input_file = 'integration_tests_expanded_v2.json'
    output_file = 'integration_tests_expanded_v3.json'

    with open(input_file, 'r') as f:
        data = json.load(f)

    original_count = len(data['tests'])

    # Filter out lookup-only queries (single >>)
    # But keep legitimate parent/child mappings
    filtered_tests = []
    removed = []

    for test in data['tests']:
        query = test['query']
        arrow_count = query.count('>>')

        # Remove single >> queries (lookup only)
        if arrow_count == 1:
            removed.append({
                'name': test['name'],
                'query': query,
                'reason': 'Lookup only (single >>)'
            })
            continue

        # Keep all multi-step queries
        filtered_tests.append(test)

    data['tests'] = filtered_tests

    # Update metadata
    data['metadata']['version'] = '4.1'
    data['metadata']['last_updated'] = '2025-11-20'
    data['metadata']['note'] = 'Updated for new >> syntax. Removed lookup-only queries (single >>). Only testing actual mapping functionality.'

    with open(output_file, 'w') as f:
        json.dump(data, f, indent=2)

    print(f"Removed {len(removed)} lookup-only queries:\n")
    for item in removed:
        print(f"  - {item['name']}: {item['query']}")

    print(f"\n✓ Original: {original_count} tests")
    print(f"✓ Cleaned:  {len(filtered_tests)} tests")
    print(f"✓ Removed:  {len(removed)} tests")
    print(f"✓ Saved to: {output_file}")

if __name__ == '__main__':
    main()
