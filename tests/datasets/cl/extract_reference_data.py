#!/usr/bin/env python3
"""
Extract CL reference data from OWL file for test IDs.

CL doesn't have a public REST API, so we extract data directly from the
OWL file that was used to build the test database.
"""

import urllib.request
import xml.etree.ElementTree as ET
import json
from pathlib import Path
import sys

def extract_reference_data():
    """Extract reference data for test IDs from CL OWL file"""

    script_dir = Path(__file__).parent
    ids_file = script_dir / "cl_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Download CL OWL file
    cl_url = "http://purl.obolibrary.org/obo/cl.owl"

    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print("Run test build first: ./biobtree -d 'cl' test")
        return 1

    # Load test IDs
    with open(ids_file) as f:
        test_ids = set(line.strip() for line in f if line.strip())

    print(f"Loaded {len(test_ids)} test CL IDs")
    print(f"Downloading CL OWL file from {cl_url}...")

    try:
        with urllib.request.urlopen(cl_url) as response:
            owl_data = response.read()
    except Exception as e:
        print(f"Error downloading CL OWL: {e}")
        return 1

    print("Parsing OWL/XML...")

    # Parse OWL/XML
    root = ET.fromstring(owl_data)

    # Namespaces
    ns = {
        'owl': 'http://www.w3.org/2002/07/owl#',
        'rdf': 'http://www.w3.org/1999/02/22-rdf-syntax-ns#',
        'rdfs': 'http://www.w3.org/2000/01/rdf-schema#',
        'obo': 'http://purl.obolibrary.org/obo/',
        'oboInOwl': 'http://www.geneontology.org/formats/oboInOwl#'
    }

    reference_data = []

    # Extract data for each test ID
    for class_elem in root.findall('.//owl:Class', ns):
        about = class_elem.get('{http://www.w3.org/1999/02/22-rdf-syntax-ns#}about', '')

        if not about:
            continue

        # Extract CL ID
        cl_id = about.split('/')[-1].replace('_', ':')

        if cl_id not in test_ids:
            continue

        # Extract name
        label = class_elem.find('.//rdfs:label', ns)
        name = label.text if label is not None else ""

        # Extract synonyms
        synonyms = []
        for syn in class_elem.findall('.//oboInOwl:hasExactSynonym', ns):
            if syn.text:
                synonyms.append(syn.text)
        for syn in class_elem.findall('.//oboInOwl:hasRelatedSynonym', ns):
            if syn.text:
                synonyms.append(syn.text)

        # Extract parents (is_a relationships)
        parents = []
        for subclass in class_elem.findall('.//rdfs:subClassOf', ns):
            parent_about = subclass.get('{http://www.w3.org/1999/02/22-rdf-syntax-ns#}resource', '')
            if parent_about and '/CL_' in parent_about:
                parent_id = parent_about.split('/')[-1].replace('_', ':')
                parents.append(parent_id)

        reference_data.append({
            "id": cl_id,
            "name": name,
            "synonyms": synonyms,
            "parents": parents
        })

        print(f"  Extracted {cl_id}: {name[:60]}...")

    # Sort by ID
    reference_data.sort(key=lambda x: x["id"])

    # Save reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"\n✓ Extracted data for {len(reference_data)} CL terms")
    print(f"✓ Saved to {output_file}")

    # Print summary
    if reference_data:
        total_synonyms = sum(len(e["synonyms"]) for e in reference_data)
        total_parents = sum(len(e["parents"]) for e in reference_data)
        print(f"\nSummary:")
        print(f"  Total synonyms: {total_synonyms}")
        print(f"  Total parent relationships: {total_parents}")
        print(f"  Avg synonyms per term: {total_synonyms / len(reference_data):.1f}")
        print(f"\nExample term:")
        example = reference_data[0]
        print(f"  {example['id']}: {example['name']}")
        if example['synonyms']:
            print(f"  Synonyms: {', '.join(example['synonyms'][:3])}")
        if example['parents']:
            print(f"  Parents: {', '.join(example['parents'][:3])}")

    return 0

if __name__ == "__main__":
    sys.exit(extract_reference_data())
