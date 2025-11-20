#!/usr/bin/env python3
"""
Update integration test queries from old single >> syntax to new two-step >>dataset>>target syntax
"""

import json
import re

def update_query(query, should_pass):
    """
    Update a single >> query to the new syntax based on input identifiers

    Logic:
    1. If query already has 2+ >>, it's fine - no change needed
    2. If query has single >>, determine the lookup dataset from inputs:
       - Gene symbols (BRCA1, TP53, etc.) → lookup in ensembl first
       - Ensembl IDs (ENSG...) → lookup in ensembl
       - UniProt IDs (P04637, etc.) → lookup in uniprot
       - ChEMBL IDs (CHEMBL...) → lookup in chembl_molecule
       - GO terms (GO:...) → lookup in go
       - Other ontology IDs → lookup in respective dataset
    """

    # Count >> in query
    arrow_count = query.count('>>')

    # If already has 2+ steps, no change needed
    if arrow_count >= 2:
        return query

    # Single >> query - need to add lookup dataset
    if arrow_count == 1:
        # Extract target dataset from query (first word after >>)
        match = re.match(r'>>(\w+)', query)
        if not match:
            return query

        target = match.group(1)
        rest_of_query = query[len(f'>>{target}'):]  # Get filters, etc.

        # Determine lookup dataset from sample inputs
        sample = should_pass[0] if should_pass else ""

        # Gene symbols or Ensembl IDs → ensembl
        if sample.startswith('ENSG') or sample.startswith('ENSMUSG') or sample.startswith('ENST') or sample.startswith('ENSP'):
            lookup = 'ensembl'
        elif sample.startswith('P0') or sample.startswith('Q') or len(sample) == 6 or (sample.startswith('A') and len(sample) == 6):
            # UniProt accession pattern
            lookup = 'uniprot'
        elif sample.startswith('CHEMBL'):
            lookup = 'chembl_molecule'
        elif sample.startswith('GO:'):
            lookup = 'go'
        elif sample.startswith('EFO:'):
            lookup = 'efo'
        elif sample.startswith('UBERON:'):
            lookup = 'uberon'
        elif sample.startswith('CL:'):
            lookup = 'cl'
        elif sample.startswith('R-HSA-') or sample.startswith('R-MMU-'):
            lookup = 'reactome'
        elif sample.startswith('RHEA:'):
            lookup = 'rhea'
        elif sample.startswith('CHEBI:'):
            lookup = 'chebi'
        elif sample.startswith('GCST'):
            lookup = 'gwas_study'
        elif sample.startswith('rs'):
            lookup = 'gwas'
        elif sample.startswith('SLM:'):
            lookup = 'swisslipids'
        elif sample.startswith('AF-'):
            lookup = 'alphafold'
        elif sample.isdigit():
            # Numeric ID - could be taxonomy or entrez gene
            if target in ['taxchild', 'taxparent']:
                lookup = 'taxonomy'
            else:
                lookup = 'ensembl'  # Try ensembl for entrez gene IDs
        else:
            # Gene symbol (BRCA1, TP53, etc.) - these are keywords
            # Gene symbols link to ensembl, not directly to uniprot
            # So queries like "TP53 >> uniprot >> go" should become "TP53 >> ensembl >> uniprot >> go"

            if target == 'uniprot':
                # Gene symbols need to go through ensembl first to reach uniprot
                lookup = 'ensembl'
            elif target in ['ensembl', 'transcript', 'exon', 'cds', 'ortholog', 'paralog']:
                lookup = 'ensembl'
            elif target in ['chembl_activity', 'chembl_assay', 'chembl_target', 'chembl_molecule', 'chembl_target_component']:
                lookup = 'chembl_molecule'
            elif target.startswith('go'):
                lookup = 'go'
            elif target.startswith('tax'):
                lookup = 'taxonomy'
            elif target in ['bgee']:
                lookup = 'ensembl'
            elif target in ['rhea']:
                lookup = 'chebi'
            elif target in ['gwas']:
                lookup = 'ensembl'  # Gene symbols to GWAS
            elif target in ['intact']:
                # Gene symbols to interactions need to go through ensembl first
                lookup = 'ensembl'
            else:
                # Default to ensembl for gene-related queries
                lookup = 'ensembl'

        # Build new query
        new_query = f'>>{lookup}>>{target}{rest_of_query}'
        return new_query

    return query

def main():
    input_file = 'integration_tests_expanded.json'
    output_file = 'integration_tests_expanded_updated.json'

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
    data['metadata']['note'] = 'Updated for new >> syntax: first >> is lookup, subsequent >> are mappings. Removed s= parameter.'

    with open(output_file, 'w') as f:
        json.dump(data, f, indent=2)

    print(f"\n✓ Updated {updated_count} queries")
    print(f"✓ Saved to: {output_file}")

if __name__ == '__main__':
    main()
