#!/usr/bin/env python3
"""
Update integration test queries from old syntax to new syntax

Key insight: In the OLD system, gene symbols would automatically search everywhere.
In the NEW system, we need to explicitly specify where to look first.

For gene symbols (TP53, BRCA1, etc.), they are keywords that link to ENSEMBL, not UniProt.
So:
- OLD: i=TP53&m=>>uniprot (searched everywhere, found TP53 keyword → ensembl → uniprot)
- NEW: i=TP53&m=>>ensembl>>uniprot (explicit: lookup in ensembl first)

- OLD: i=TP53&m=>>uniprot>>go
- NEW: i=TP53&m=>>ensembl>>uniprot>>go
"""

import json
import re

def determine_lookup_dataset(sample_input):
    """Determine the lookup dataset based on the input identifier format"""
    if not sample_input:
        return 'ensembl'  # Default to ensembl

    # Ensembl IDs
    if sample_input.startswith(('ENSG', 'ENSMUSG', 'ENST', 'ENSP', 'ENSDARG')):
        return 'ensembl'

    # UniProt accessions
    if (sample_input.startswith(('P0', 'Q', 'A0')) and len(sample_input) in [6, 10]) or \
       (sample_input.startswith(('P', 'Q', 'O')) and sample_input[1].isdigit() and len(sample_input) in [6, 10]):
        return 'uniprot'

    # ChEMBL IDs
    if sample_input.startswith('CHEMBL'):
        return 'chembl_molecule'

    # Ontology IDs
    if sample_input.startswith('GO:'):
        return 'go'
    if sample_input.startswith('EFO:'):
        return 'efo'
    if sample_input.startswith('UBERON:'):
        return 'uberon'
    if sample_input.startswith('CL:'):
        return 'cl'
    if sample_input.startswith(('R-HSA-', 'R-MMU-')):
        return 'reactome'
    if sample_input.startswith('RHEA:'):
        return 'rhea'
    if sample_input.startswith('CHEBI:'):
        return 'chebi'
    if sample_input.startswith('GCST'):
        return 'gwas_study'
    if sample_input.startswith('rs') and sample_input[2:].isdigit():
        return 'gwas'
    if sample_input.startswith('SLM:'):
        return 'swisslipids'
    if sample_input.startswith('AF-'):
        return 'alphafold'

    # Numeric IDs
    if sample_input.isdigit():
        return 'taxonomy'  # or could be entrez gene, but default to taxonomy

    # Gene symbols (BRCA1, TP53, etc.) - these are keywords that link to ensembl
    # This is the most common case
    return 'ensembl'

def update_query(query, should_pass):
    """
    Update query to new syntax

    Strategy:
    1. Parse the existing query chain
    2. Determine what the first step should be based on inputs
    3. Rebuild the query with explicit lookup
    """

    if not query.startswith('>>'):
        return query

    # Get sample input to determine lookup dataset
    sample = should_pass[0] if should_pass else ""
    lookup = determine_lookup_dataset(sample)

    # Parse the query
    # Remove leading >>
    query_without_prefix = query[2:]

    # Split by >> to get the chain
    parts = query_without_prefix.split('>>')

    if not parts:
        return query

    # Check if first part is already the correct lookup dataset
    first_dataset_match = re.match(r'^(\w+)', parts[0])
    if not first_dataset_match:
        return query

    first_dataset = first_dataset_match.group(1)

    # If first dataset matches lookup, query is already correct
    if first_dataset == lookup:
        return query

    # Otherwise, prepend the lookup dataset
    new_query = f'>>{lookup}>>' + '>>'.join(parts)
    return new_query

def main():
    input_file = 'integration_tests_expanded.json'
    output_file = 'integration_tests_expanded_v2.json'

    with open(input_file, 'r') as f:
        data = json.load(f)

    updated_count = 0

    for test in data['tests']:
        old_query = test['query']
        new_query = update_query(old_query, test.get('should_pass', []))

        if old_query != new_query:
            test['query'] = new_query
            print(f"Updated: {old_query:60s} → {new_query}")
            updated_count += 1

    # Update metadata
    data['metadata']['version'] = '4.0'
    data['metadata']['last_updated'] = '2025-11-20'
    data['metadata']['note'] = 'Updated for new >> syntax: first >> is lookup, subsequent >> are mappings. Gene symbols start with >>ensembl.'

    with open(output_file, 'w') as f:
        json.dump(data, f, indent=2)

    print(f"\n✓ Updated {updated_count} queries")
    print(f"✓ Saved to: {output_file}")

if __name__ == '__main__':
    main()
