#!/usr/bin/env python3
"""
Extract MONDO reference data from OBO file.

Downloads MONDO OBO file and extracts COMPLETE term data for test IDs.
Preserves ALL fields from OBO format for comprehensive testing.
"""

import sys
import json
import re
import requests
from pathlib import Path
from typing import Dict, List, Optional, Any
from collections import defaultdict

# MONDO OBO file URL (same as in config)
MONDO_OBO_URL = "https://github.com/monarch-initiative/mondo/releases/download/v2025-10-07/mondo.obo"
IDS_FILE = "mondo_ids.txt"
OUTPUT_FILE = "reference_data.json"
CACHE_FILE = "mondo.obo"  # Cache downloaded file


def download_mondo_obo() -> str:
    """Download MONDO OBO file or use cached version"""
    cache_path = Path(CACHE_FILE)

    if cache_path.exists():
        print(f"Using cached OBO file: {CACHE_FILE}")
        with open(cache_path, 'r', encoding='utf-8') as f:
            return f.read()

    print(f"Downloading MONDO OBO from {MONDO_OBO_URL}...")
    response = requests.get(MONDO_OBO_URL, timeout=300)
    response.raise_for_status()

    content = response.text

    # Cache for future use
    with open(cache_path, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"✓ Downloaded and cached ({len(content)} bytes)")

    return content


def parse_synonym_line(line: str) -> Optional[Dict[str, Any]]:
    """
    Parse synonym line completely: synonym: "text" SCOPE [refs]
    Returns dict with text, scope, and refs
    """
    line = line.replace('synonym: ', '', 1)
    if not line.startswith('"'):
        return None

    # Extract quoted text
    end_quote = line.find('"', 1)
    if end_quote == -1:
        return None

    text = line[1:end_quote]
    remainder = line[end_quote + 1:].strip()

    # Extract scope (EXACT, BROAD, NARROW, RELATED)
    scope = None
    refs = None

    parts = remainder.split('[', 1)
    if parts[0].strip():
        scope = parts[0].strip()

    if len(parts) > 1:
        # Extract references
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
    Parse xref line completely: xref: DATABASE:ID {props}
    Returns dict with id and properties
    """
    line = line.replace('xref: ', '', 1)

    # Find properties if present
    brace_idx = line.find('{')
    if brace_idx != -1:
        xref_id = line[:brace_idx].strip()
        props_end = line.find('}', brace_idx)
        props = line[brace_idx + 1:props_end].strip() if props_end != -1 else None
    else:
        # No properties, find end of ID (space)
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
    Parse is_a line completely: is_a: MONDO:0000001 ! disease or disorder
    Returns dict with id and comment
    """
    line = line.replace('is_a: ', '', 1)

    # Find comment after !
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

    # Extract references
    refs = None
    if remainder.startswith('['):
        refs_end = remainder.find(']')
        if refs_end != -1:
            refs = remainder[1:refs_end].strip()

    return {
        'text': text,
        'refs': refs
    }


def parse_relationship_line(line: str) -> Optional[Dict[str, Any]]:
    """
    Parse relationship line: relationship: disease_has_basis_in_disruption_of MONDO:0000123 ! comment
    Returns dict with type, target, and comment
    """
    line = line.replace('relationship: ', '', 1)

    parts = line.split(None, 1)
    if len(parts) < 2:
        return None

    rel_type = parts[0]
    remainder = parts[1]

    # Find comment after !
    comment_idx = remainder.find('!')
    if comment_idx != -1:
        target = remainder[:comment_idx].strip()
        comment = remainder[comment_idx + 1:].strip()
    else:
        target = remainder.strip()
        comment = None

    return {
        'type': rel_type,
        'target': target,
        'comment': comment
    }


def parse_mondo_obo(obo_content: str, target_ids: set) -> List[Dict]:
    """
    Parse MONDO OBO file and extract COMPLETE data for target IDs.
    Preserves ALL fields, not just selected ones.
    """
    terms = []
    current_term = None
    in_term = False

    for line in obo_content.split('\n'):
        line = line.strip()

        # Start of new term
        if line == '[Term]':
            # Save previous term if it matches target IDs
            if current_term and current_term.get('id') in target_ids:
                if not current_term.get('is_obsolete', False):
                    terms.append(current_term)

            # Start new term - use defaultdict for arrays
            in_term = True
            current_term = {
                'synonyms': [],
                'xrefs': [],
                'parents': [],
                'relationships': [],
                'subsets': [],
                'property_values': [],
                'comments': [],
                'alt_ids': []
            }
            continue

        # End of term section
        if line.startswith('[') and line != '[Term]':
            in_term = False
            continue

        if not in_term or not line:
            continue

        # Parse ALL term fields comprehensively
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

        elif line.startswith('relationship: '):
            rel = parse_relationship_line(line)
            if rel:
                current_term['relationships'].append(rel)

        elif line.startswith('subset: '):
            current_term['subsets'].append(line.replace('subset: ', ''))

        elif line.startswith('property_value: '):
            current_term['property_values'].append(line.replace('property_value: ', ''))

        elif line.startswith('comment: '):
            current_term['comments'].append(line.replace('comment: ', ''))

        elif line.startswith('alt_id: '):
            current_term['alt_ids'].append(line.replace('alt_id: ', ''))

        elif line == 'is_obsolete: true':
            current_term['is_obsolete'] = True

        elif line.startswith('created_by: '):
            current_term['created_by'] = line.replace('created_by: ', '')

        elif line.startswith('creation_date: '):
            current_term['creation_date'] = line.replace('creation_date: ', '')

        elif line.startswith('namespace: '):
            current_term['namespace'] = line.replace('namespace: ', '')

        # Catch any other fields we might have missed
        elif ': ' in line and not line.startswith('!'):
            field_name, field_value = line.split(': ', 1)
            # Store in "other_fields" dict for truly unknown fields
            if 'other_fields' not in current_term:
                current_term['other_fields'] = {}
            if field_name not in current_term['other_fields']:
                current_term['other_fields'][field_name] = []
            current_term['other_fields'][field_name].append(field_value)

    # Don't forget last term
    if current_term and current_term.get('id') in target_ids:
        if not current_term.get('is_obsolete', False):
            terms.append(current_term)

    return terms


def main():
    """Main extraction process"""
    # Read target IDs
    ids_path = Path(IDS_FILE)
    if not ids_path.exists():
        print(f"Error: {IDS_FILE} not found")
        print("Run: cp ../../test_out/reference/mondo_ids.txt .")
        return 1

    with open(ids_path, 'r') as f:
        target_ids = set(line.strip() for line in f if line.strip())

    print(f"Target IDs: {len(target_ids)}")

    # Download/load OBO file
    try:
        obo_content = download_mondo_obo()
    except Exception as e:
        print(f"Error downloading OBO file: {e}")
        return 1

    # Parse and extract COMPLETE data
    print("Parsing OBO file (extracting ALL fields)...")
    terms = parse_mondo_obo(obo_content, target_ids)

    print(f"✓ Extracted {len(terms)}/{len(target_ids)} terms")

    # Validate we found all IDs
    found_ids = {term['id'] for term in terms}
    missing = target_ids - found_ids
    if missing:
        print(f"Warning: Missing {len(missing)} IDs:")
        for mid in sorted(list(missing)[:10]):
            print(f"  - {mid}")
        if len(missing) > 10:
            print(f"  ... and {len(missing) - 10} more")

    # Save reference data
    output_path = Path(OUTPUT_FILE)
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(terms, f, indent=2, ensure_ascii=False)

    file_size_kb = output_path.stat().st_size / 1024
    print(f"✓ Saved to {OUTPUT_FILE} ({file_size_kb:.1f} KB)")

    # Show sample with ALL fields
    if terms:
        sample = terms[0]
        print(f"\nSample term (showing all extracted fields):")
        print(f"  ID: {sample.get('id')}")
        print(f"  Name: {sample.get('name', 'N/A')}")
        if sample.get('definition'):
            def_text = sample['definition'].get('text', '')
            print(f"  Definition: {def_text[:80]}..." if len(def_text) > 80 else f"  Definition: {def_text}")
        print(f"  Synonyms: {len(sample.get('synonyms', []))} (with scope & refs)")
        print(f"  Parents: {len(sample.get('parents', []))} (with comments)")
        print(f"  Xrefs: {len(sample.get('xrefs', []))} (with properties)")
        print(f"  Relationships: {len(sample.get('relationships', []))}")
        print(f"  Subsets: {len(sample.get('subsets', []))}")
        print(f"  Property values: {len(sample.get('property_values', []))}")
        print(f"  Comments: {len(sample.get('comments', []))}")
        print(f"  Alt IDs: {len(sample.get('alt_ids', []))}")
        if sample.get('namespace'):
            print(f"  Namespace: {sample.get('namespace')}")
        if sample.get('created_by'):
            print(f"  Created by: {sample.get('created_by')}")
        if sample.get('creation_date'):
            print(f"  Creation date: {sample.get('creation_date')}")

    print(f"\n✓ Complete reference data extracted with ALL available fields")

    return 0


if __name__ == "__main__":
    sys.exit(main())
