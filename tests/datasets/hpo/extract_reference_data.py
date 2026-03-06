#!/usr/bin/env python3
"""
Extract HPO reference data from OBO and genes_to_phenotype files.

Downloads HPO files and extracts COMPLETE term data for test IDs.
Preserves ALL fields from OBO format plus gene associations.
"""

import sys
import json
import re
import requests
from pathlib import Path
from typing import Dict, List, Optional, Any
from collections import defaultdict

# HPO file URLs (same as in config)
HPO_OBO_URL = "http://purl.obolibrary.org/obo/hp.obo"
GENES_TO_PHENOTYPE_URL = "https://github.com/obophenotype/human-phenotype-ontology/releases/download/v2025-10-22/genes_to_phenotype.txt"
IDS_FILE = "hpo_ids.txt"
OUTPUT_FILE = "reference_data.json"
HPO_CACHE = "hp.obo"
GENES_CACHE = "genes_to_phenotype.txt"


def download_file(url: str, cache_file: str) -> str:
    """Download file or use cached version"""
    cache_path = Path(cache_file)

    if cache_path.exists():
        print(f"Using cached file: {cache_file}")
        with open(cache_path, 'r', encoding='utf-8') as f:
            return f.read()

    print(f"Downloading from {url}...")
    response = requests.get(url, timeout=300)
    response.raise_for_status()

    content = response.text

    # Cache for future use
    with open(cache_path, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"✓ Downloaded and cached ({len(content)} bytes)")

    return content


def parse_synonym_line(line: str) -> Optional[Dict[str, Any]]:
    """
    Parse synonym line: synonym: "text" SCOPE [refs]
    Returns dict with text, scope, and refs
    """
    line = line.replace('synonym: ', '', 1)
    if not line.startswith('"'):
        return None

    end_quote = line.find('"', 1)
    if end_quote == -1:
        return None

    text = line[1:end_quote]
    remainder = line[end_quote + 1:].strip()

    scope = None
    refs = None

    parts = remainder.split('[', 1)
    if parts[0].strip():
        scope = parts[0].strip()

    if len(parts) > 1:
        refs_end = parts[1].find(']')
        if refs_end != -1:
            refs = parts[1][:refs_end].strip()

    return {
        'text': text,
        'scope': scope,
        'refs': refs
    }


def parse_xref_line(line: str) -> Optional[Dict[str, Any]]:
    """
    Parse xref line: xref: DATABASE:ID {props}
    Returns dict with id and properties
    """
    line = line.replace('xref: ', '', 1)

    brace_idx = line.find('{')
    if brace_idx != -1:
        xref_id = line[:brace_idx].strip()
        props_end = line.find('}', brace_idx)
        props = line[brace_idx + 1:props_end].strip() if props_end != -1 else None
    else:
        space_idx = line.find(' ')
        if space_idx != -1:
            xref_id = line[:space_idx].strip()
        else:
            xref_id = line.strip()
        props = None

    return {
        'id': xref_id,
        'properties': props
    } if xref_id else None


def parse_is_a_line(line: str) -> Optional[Dict[str, Any]]:
    """
    Parse is_a line: is_a: HP:0000001 ! All
    Returns dict with id and comment
    """
    line = line.replace('is_a: ', '', 1)

    comment_idx = line.find('!')
    if comment_idx != -1:
        parent_id = line[:comment_idx].strip()
        comment = line[comment_idx + 1:].strip()
    else:
        parent_id = line.strip()
        comment = None

    return {
        'id': parent_id,
        'comment': comment
    } if parent_id else None


def parse_def_line(line: str) -> Optional[Dict[str, Any]]:
    """
    Parse definition line: def: "text" [refs]
    Returns dict with text and refs
    """
    line = line.replace('def: ', '', 1)
    if not line.startswith('"'):
        return None

    end_quote = line.find('"', 1)
    if end_quote == -1:
        return None

    text = line[1:end_quote]
    remainder = line[end_quote + 1:].strip()

    refs = None
    if remainder.startswith('['):
        refs_end = remainder.find(']')
        if refs_end != -1:
            refs = remainder[1:refs_end].strip()

    return {
        'text': text,
        'refs': refs
    }


def parse_hpo_obo(obo_content: str, target_ids: set) -> Dict[str, Dict]:
    """
    Parse HPO OBO file and extract COMPLETE data for target IDs.
    Returns dict keyed by ID for easy lookup.
    """
    terms = {}
    current_term = None
    in_term = False

    for line in obo_content.split('\n'):
        line = line.strip()

        if line == '[Term]':
            if current_term and current_term.get('id') in target_ids:
                if not current_term.get('is_obsolete', False):
                    terms[current_term['id']] = current_term

            in_term = True
            current_term = {
                'synonyms': [],
                'xrefs': [],
                'parents': [],
                'subsets': [],
                'comments': [],
                'alt_ids': [],
                'gene_associations': []  # Will be filled later
            }
            continue

        if line.startswith('[') and line != '[Term]':
            in_term = False
            continue

        if not in_term or not line:
            continue

        if line.startswith('id: '):
            current_term['id'] = line.replace('id: ', '')

        elif line.startswith('name: '):
            current_term['name'] = line.replace('name: ', '')

        elif line.startswith('def: '):
            current_term['definition'] = parse_def_line(line)

        elif line.startswith('synonym: '):
            synonym = parse_synonym_line(line)
            if synonym:
                current_term['synonyms'].append(synonym)

        elif line.startswith('is_a: '):
            parent = parse_is_a_line(line)
            if parent:
                current_term['parents'].append(parent)

        elif line.startswith('xref: '):
            xref = parse_xref_line(line)
            if xref:
                current_term['xrefs'].append(xref)

        elif line.startswith('subset: '):
            current_term['subsets'].append(line.replace('subset: ', ''))

        elif line.startswith('comment: '):
            current_term['comments'].append(line.replace('comment: ', ''))

        elif line.startswith('alt_id: '):
            current_term['alt_ids'].append(line.replace('alt_id: ', ''))

        elif line == 'is_obsolete: true':
            current_term['is_obsolete'] = True

        elif line.startswith('namespace: '):
            current_term['namespace'] = line.replace('namespace: ', '')

    # Don't forget last term
    if current_term and current_term.get('id') in target_ids:
        if not current_term.get('is_obsolete', False):
            terms[current_term['id']] = current_term

    return terms


def parse_gene_associations(genes_content: str, terms: Dict[str, Dict]):
    """
    Parse genes_to_phenotype.txt and add gene associations to terms.
    Format: ncbi_gene_id\tgene_symbol\thpo_id\thpo_name\tfrequency\tdisease_id
    """
    lines = genes_content.strip().split('\n')

    # Skip header
    for line in lines[1:]:
        if not line.strip():
            continue

        fields = line.split('\t')
        if len(fields) < 3:
            continue

        ncbi_gene_id = fields[0]
        gene_symbol = fields[1]
        hpo_id = fields[2]

        # Add to term if it's one of our target IDs
        if hpo_id in terms:
            terms[hpo_id]['gene_associations'].append({
                'gene_symbol': gene_symbol,
                'ncbi_gene_id': ncbi_gene_id
            })


def main():
    """Main extraction process"""
    # Read target IDs
    ids_path = Path(IDS_FILE)
    if not ids_path.exists():
        print(f"Error: {IDS_FILE} not found")
        print("Run: cp ../../test_out/reference/hpo_ids.txt .")
        return 1

    with open(ids_path, 'r') as f:
        target_ids = set(line.strip() for line in f if line.strip())

    print(f"Target IDs: {len(target_ids)}")

    # Download/load HPO OBO file
    try:
        obo_content = download_file(HPO_OBO_URL, HPO_CACHE)
    except Exception as e:
        print(f"Error downloading HPO OBO file: {e}")
        return 1

    # Parse OBO file
    print("Parsing HPO OBO file...")
    terms = parse_hpo_obo(obo_content, target_ids)
    print(f"✓ Extracted {len(terms)}/{len(target_ids)} phenotype terms")

    # Download/load genes_to_phenotype file
    try:
        genes_content = download_file(GENES_TO_PHENOTYPE_URL, GENES_CACHE)
    except Exception as e:
        print(f"Warning: Could not download gene associations: {e}")
        genes_content = None

    # Parse gene associations
    if genes_content:
        print("Parsing gene-phenotype associations...")
        parse_gene_associations(genes_content, terms)

        gene_count = sum(len(t['gene_associations']) for t in terms.values())
        print(f"✓ Added {gene_count} gene associations")

    # Validate we found all IDs
    found_ids = set(terms.keys())
    missing = target_ids - found_ids
    if missing:
        print(f"Warning: Missing {len(missing)} IDs:")
        for mid in sorted(list(missing)[:10]):
            print(f"  - {mid}")
        if len(missing) > 10:
            print(f"  ... and {len(missing) - 10} more")

    # Convert to list for JSON output
    terms_list = list(terms.values())

    # Save reference data
    output_path = Path(OUTPUT_FILE)
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(terms_list, f, indent=2, ensure_ascii=False)

    file_size_kb = output_path.stat().st_size / 1024
    print(f"✓ Saved to {OUTPUT_FILE} ({file_size_kb:.1f} KB)")

    # Show sample
    if terms_list:
        sample = terms_list[0]
        print(f"\nSample phenotype term:")
        print(f"  ID: {sample.get('id')}")
        print(f"  Name: {sample.get('name', 'N/A')}")
        if sample.get('definition'):
            def_text = sample['definition'].get('text', '')
            print(f"  Definition: {def_text[:80]}..." if len(def_text) > 80 else f"  Definition: {def_text}")
        print(f"  Synonyms: {len(sample.get('synonyms', []))}")
        print(f"  Parents: {len(sample.get('parents', []))}")
        print(f"  Xrefs: {len(sample.get('xrefs', []))}")
        print(f"  Gene associations: {len(sample.get('gene_associations', []))}")
        if sample.get('gene_associations'):
            first_gene = sample['gene_associations'][0]['gene_symbol']
            print(f"    e.g., {first_gene}")

    print(f"\n✓ Complete reference data extracted")

    return 0


if __name__ == "__main__":
    sys.exit(main())
