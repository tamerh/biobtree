#!/usr/bin/env python3
"""
Extract reference data for CellPhoneDB test IDs from local CSV files.

Usage: python3 extract_reference_data.py

Reads cellphonedb_ids.txt and extracts matching rows from the CellPhoneDB CSV files.
Saves structured reference data to reference_data.json.
"""

import json
import csv
import os
import sys

# CellPhoneDB data files relative to biobtree root
INTERACTION_FILE = "cache/cellphonedb/interaction_table.csv"
MULTIDATA_FILE = "cache/cellphonedb/multidata_table.csv"
GENE_FILE = "cache/cellphonedb/gene_table.csv"
COMPLEX_FILE = "cache/cellphonedb/complex_composition_table.csv"


def load_test_ids(filename="cellphonedb_ids.txt"):
    """Load test IDs from file."""
    if not os.path.exists(filename):
        print(f"Error: {filename} not found")
        sys.exit(1)

    with open(filename, 'r') as f:
        return set(line.strip() for line in f if line.strip())


def find_file(data_file):
    """Find file relative to script or biobtree root."""
    if os.path.exists(data_file):
        return data_file

    # Try relative to biobtree root (3 levels up from this script)
    filepath = os.path.join(os.path.dirname(__file__), "..", "..", "..", data_file)
    if os.path.exists(filepath):
        return filepath

    return None


def load_multidata():
    """Load multidata_table.csv into a lookup dictionary."""
    filepath = find_file(MULTIDATA_FILE)
    if not filepath:
        print(f"Warning: {MULTIDATA_FILE} not found")
        return {}

    multidata = {}
    with open(filepath, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            multidata_id = row.get('id_multidata', '')
            if multidata_id:
                multidata[multidata_id] = row
    return multidata


def load_genes():
    """Load gene_table.csv into a lookup dictionary by uniprot."""
    filepath = find_file(GENE_FILE)
    if not filepath:
        print(f"Warning: {GENE_FILE} not found")
        return {}

    genes = {}
    with open(filepath, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            uniprot = row.get('uniprot', '')
            if uniprot:
                genes[uniprot] = row
    return genes


def load_complex_components():
    """Load complex_composition_table.csv into a lookup dictionary."""
    filepath = find_file(COMPLEX_FILE)
    if not filepath:
        print(f"Warning: {COMPLEX_FILE} not found")
        return {}

    complexes = {}
    with open(filepath, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            complex_id = row.get('complex_multidata_id', '')
            protein_id = row.get('protein_multidata_id', '')
            if complex_id and protein_id:
                if complex_id not in complexes:
                    complexes[complex_id] = []
                complexes[complex_id].append(protein_id)
    return complexes


def get_genes_for_partner(partner_id, is_complex, multidata, genes, complexes):
    """Get gene symbols for a partner (protein or complex)."""
    gene_list = []

    if is_complex and partner_id in complexes:
        # Complex: get genes from all component proteins
        for protein_id in complexes[partner_id]:
            if protein_id in multidata:
                protein_data = multidata[protein_id]
                uniprot = protein_data.get('protein_name', '')
                if uniprot in genes:
                    gene_name = genes[uniprot].get('hgnc_symbol', '')
                    if gene_name:
                        gene_list.append(gene_name)
    else:
        # Simple protein
        if partner_id in multidata:
            protein_data = multidata[partner_id]
            uniprot = protein_data.get('protein_name', '')
            if uniprot in genes:
                gene_name = genes[uniprot].get('hgnc_symbol', '')
                if gene_name:
                    gene_list.append(gene_name)

    return gene_list


def parse_interaction_file(test_ids, multidata, genes, complexes):
    """Parse interaction_table.csv and extract matching entries."""
    filepath = find_file(INTERACTION_FILE)
    if not filepath:
        print(f"Error: {INTERACTION_FILE} not found")
        return {}

    results = {}
    print(f"Processing {INTERACTION_FILE}...")

    with open(filepath, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            interaction_id = row.get('id_cp_interaction', '')
            if interaction_id not in test_ids:
                continue

            # Get partner multidata IDs
            partner_a_id = row.get('multidata_1_id', '')
            partner_b_id = row.get('multidata_2_id', '')

            # Get partner names from multidata
            partner_a_name = multidata.get(partner_a_id, {}).get('name', '')
            partner_b_name = multidata.get(partner_b_id, {}).get('name', '')

            # Determine if partners are complexes
            is_complex_a = multidata.get(partner_a_id, {}).get('is_complex', '') == 'True'
            is_complex_b = multidata.get(partner_b_id, {}).get('is_complex', '') == 'True'

            # Get receptor/secreted/integrin status
            receptor_a = multidata.get(partner_a_id, {}).get('receptor', '') == 'True'
            receptor_b = multidata.get(partner_b_id, {}).get('receptor', '') == 'True'
            secreted_a = multidata.get(partner_a_id, {}).get('secreted_highlight', '') == 'True'
            secreted_b = multidata.get(partner_b_id, {}).get('secreted_highlight', '') == 'True'
            is_integrin = row.get('is_integrin_interaction', '') == 'True'

            # Get genes for each partner
            genes_a = get_genes_for_partner(partner_a_id, is_complex_a, multidata, genes, complexes)
            genes_b = get_genes_for_partner(partner_b_id, is_complex_b, multidata, genes, complexes)

            entry = {
                "id": interaction_id,
                "partner_a": partner_a_name,
                "partner_b": partner_b_name,
                "directionality": row.get('directionality', ''),
                "classification": row.get('classification', ''),
                "source": row.get('source', ''),
                "is_ppi": row.get('is_ppi', '') == 'True',
                "is_integrin": is_integrin,
                "is_complex_a": is_complex_a,
                "is_complex_b": is_complex_b,
                "receptor_a": receptor_a,
                "receptor_b": receptor_b,
                "secreted_a": secreted_a,
                "secreted_b": secreted_b,
                "genes_a": genes_a,
                "genes_b": genes_b,
            }

            results[interaction_id] = entry

    return results


def main():
    print("Extracting CellPhoneDB reference data...")

    # Load test IDs
    test_ids = load_test_ids()
    print(f"Loaded {len(test_ids)} test IDs")

    # Load lookup tables
    print("Loading multidata table...")
    multidata = load_multidata()
    print(f"  Loaded {len(multidata)} multidata entries")

    print("Loading gene table...")
    genes = load_genes()
    print(f"  Loaded {len(genes)} gene entries")

    print("Loading complex composition table...")
    complexes = load_complex_components()
    print(f"  Loaded {len(complexes)} complex entries")

    # Parse interaction file
    entries = parse_interaction_file(test_ids, multidata, genes, complexes)
    print(f"Found {len(entries)} matching entries")

    # Convert to list format for reference_data.json
    reference_data = list(entries.values())

    # Save to file
    output_file = "reference_data.json"
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(reference_data, f, indent=2, ensure_ascii=False)

    print(f"Saved {len(reference_data)} entries to {output_file}")

    # Print some examples
    if reference_data:
        print("\nSample entry:")
        sample = reference_data[0]
        print(f"  ID: {sample['id']}")
        print(f"  {sample['partner_a']} <-> {sample['partner_b']}")
        print(f"  Directionality: {sample['directionality']}")
        print(f"  Classification: {sample['classification']}")
        if sample['genes_a']:
            print(f"  Genes A: {', '.join(sample['genes_a'][:3])}")
        if sample['genes_b']:
            print(f"  Genes B: {', '.join(sample['genes_b'][:3])}")


if __name__ == "__main__":
    main()
