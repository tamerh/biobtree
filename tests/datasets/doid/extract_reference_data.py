#!/usr/bin/env python3
"""
Extract DOID reference data from the OWL file for the test IDs.

DOID rides biobtree's generic OWL ontology parser, the same as CL/Uberon/GO,
so reference data is extracted directly from the OWL file used to build the
test database.
"""

import urllib.request
import xml.etree.ElementTree as ET
import json
from pathlib import Path
import sys


def extract_reference_data():
    """Extract reference data for test IDs from the DOID OWL file"""

    script_dir = Path(__file__).parent
    ids_file = script_dir / "doid_ids.txt"
    output_file = script_dir / "reference_data.json"

    doid_url = "http://purl.obolibrary.org/obo/doid.owl"

    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print("Run test build first: ./biobtree -d 'doid' test")
        return 1

    with open(ids_file) as f:
        test_ids = set(line.strip() for line in f if line.strip())

    print(f"Loaded {len(test_ids)} test DOID IDs")
    print(f"Downloading DOID OWL file from {doid_url}...")

    try:
        with urllib.request.urlopen(doid_url) as response:
            owl_data = response.read()
    except Exception as e:
        print(f"Error downloading DOID OWL: {e}")
        return 1

    print("Parsing OWL/XML...")

    root = ET.fromstring(owl_data)

    ns = {
        'owl': 'http://www.w3.org/2002/07/owl#',
        'rdf': 'http://www.w3.org/1999/02/22-rdf-syntax-ns#',
        'rdfs': 'http://www.w3.org/2000/01/rdf-schema#',
        'obo': 'http://purl.obolibrary.org/obo/',
        'oboInOwl': 'http://www.geneontology.org/formats/oboInOwl#'
    }

    reference_data = []

    for class_elem in root.findall('.//owl:Class', ns):
        about = class_elem.get('{http://www.w3.org/1999/02/22-rdf-syntax-ns#}about', '')
        if not about:
            continue

        doid_id = about.split('/')[-1].replace('_', ':')
        if doid_id not in test_ids:
            continue

        label = class_elem.find('.//rdfs:label', ns)
        name = label.text if label is not None else ""

        synonyms = []
        for syn in class_elem.findall('.//oboInOwl:hasExactSynonym', ns):
            if syn.text:
                synonyms.append(syn.text)
        for syn in class_elem.findall('.//oboInOwl:hasRelatedSynonym', ns):
            if syn.text:
                synonyms.append(syn.text)

        parents = []
        for subclass in class_elem.findall('.//rdfs:subClassOf', ns):
            parent_about = subclass.get('{http://www.w3.org/1999/02/22-rdf-syntax-ns#}resource', '')
            if parent_about and '/DOID_' in parent_about:
                parent_id = parent_about.split('/')[-1].replace('_', ':')
                parents.append(parent_id)

        reference_data.append({
            "id": doid_id,
            "name": name,
            "synonyms": synonyms,
            "parents": parents
        })

        print(f"  Extracted {doid_id}: {name[:60]}...")

    reference_data.sort(key=lambda x: x["id"])

    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"\n✓ Extracted data for {len(reference_data)} DOID terms")
    print(f"✓ Saved to {output_file}")

    if reference_data:
        total_synonyms = sum(len(e["synonyms"]) for e in reference_data)
        total_parents = sum(len(e["parents"]) for e in reference_data)
        print(f"\nSummary:")
        print(f"  Total synonyms: {total_synonyms}")
        print(f"  Total parent relationships: {total_parents}")

    return 0


if __name__ == "__main__":
    sys.exit(extract_reference_data())
