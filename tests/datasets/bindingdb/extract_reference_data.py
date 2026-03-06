#!/usr/bin/env python3
"""
Extract reference data for BindingDB test entries.

Reads BindingDB TSV file and extracts data for entries listed in bindingdb_ids.txt.
"""

import json
import csv
import zipfile
from pathlib import Path


def extract_reference_data():
    """Extract reference data from BindingDB TSV file"""
    script_dir = Path(__file__).parent
    ids_file = script_dir / "bindingdb_ids.txt"
    output_file = script_dir / "reference_data.json"

    # BindingDB file location - downloaded during test build
    bindingdb_zip = script_dir.parent.parent.parent / "test_data" / "bindingdb" / "BindingDB_All_tsv.zip"
    bindingdb_dir = script_dir.parent.parent.parent / "test_data" / "bindingdb"

    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print("Run: ./biobtree -d bindingdb test")
        print("Then copy test_out/reference/bindingdb_ids.txt here")
        return 1

    # Read test IDs
    with open(ids_file) as f:
        test_ids = set(line.strip() for line in f if line.strip())

    print(f"Found {len(test_ids)} test IDs")

    # Find the TSV file (either in zip or extracted)
    tsv_file = None

    if bindingdb_zip.exists():
        print(f"Reading from ZIP: {bindingdb_zip}")
        with zipfile.ZipFile(bindingdb_zip, 'r') as zf:
            for name in zf.namelist():
                if name.endswith('.tsv') or name.endswith('.txt'):
                    print(f"Extracting {name}...")
                    zf.extract(name, bindingdb_dir)
                    tsv_file = bindingdb_dir / name
                    break

    if not tsv_file or not tsv_file.exists():
        # Try to find any TSV in the directory
        for f in bindingdb_dir.glob("*.tsv"):
            tsv_file = f
            break
        for f in bindingdb_dir.glob("*.txt"):
            tsv_file = f
            break

    if not tsv_file or not tsv_file.exists():
        print(f"Error: BindingDB TSV file not found in {bindingdb_dir}")
        print("The file should be downloaded during test build")
        return 1

    print(f"Reading from: {tsv_file}")

    # Read BindingDB TSV and extract matching entries
    reference_data = []

    with open(tsv_file, 'r', encoding='utf-8', errors='replace') as f:
        reader = csv.DictReader(f, delimiter='\t')

        for row in reader:
            bindingdb_id = row.get('BindingDB MonomerID', '').strip()
            if bindingdb_id in test_ids:
                entry = {
                    'bindingdb_id': bindingdb_id,
                    'ligand_name': row.get('Ligand Name', row.get('BindingDB Ligand Name', '')).strip(),
                    'ligand_smiles': row.get('Ligand SMILES', '').strip(),
                    'ligand_inchi': row.get('Ligand InChI', '').strip(),
                    'ligand_inchi_key': row.get('Ligand InChI Key', '').strip(),
                    'target_name': row.get('Target Name', '').strip(),
                    'target_source_organism': row.get('Target Source Organism According to Curator or DataSource', '').strip(),
                    'ki': row.get('Ki (nM)', '').strip(),
                    'ic50': row.get('IC50 (nM)', '').strip(),
                    'kd': row.get('Kd (nM)', '').strip(),
                    'ec50': row.get('EC50 (nM)', '').strip(),
                    'kon': row.get('kon (M-1-s-1)', '').strip(),
                    'koff': row.get('koff (s-1)', '').strip(),
                    'ph': row.get('pH', '').strip(),
                    'temp_c': row.get('Temp (C)', '').strip(),
                    'doi': row.get('Article DOI', '').strip(),
                    'pmid': row.get('PMID', '').strip(),
                    'patent_number': row.get('Patent Number', '').strip(),
                    'institution': row.get('Institution', '').strip(),
                }

                # Parse UniProt IDs (pipe-separated)
                uniprot_str = row.get('UniProt (SwissProt) Primary ID of Target Chain', '').strip()
                if uniprot_str:
                    entry['uniprot_ids'] = [u.strip() for u in uniprot_str.split('|') if u.strip()]
                else:
                    entry['uniprot_ids'] = []

                # Parse PubChem CIDs
                pubchem_str = row.get('PubChem CID', '').strip()
                if pubchem_str:
                    entry['pubchem_cids'] = [p.strip() for p in pubchem_str.split('|') if p.strip()]
                else:
                    entry['pubchem_cids'] = []

                # Parse ChEMBL IDs
                chembl_str = row.get('ChEMBL ID of Ligand', '').strip()
                if chembl_str:
                    entry['chembl_ids'] = [c.strip() for c in chembl_str.split('|') if c.strip()]
                else:
                    entry['chembl_ids'] = []

                # Parse ChEBI IDs
                chebi_str = row.get('ChEBI ID of Ligand', '').strip()
                if chebi_str:
                    entry['chebi_ids'] = [c.strip() for c in chebi_str.split('|') if c.strip()]
                else:
                    entry['chebi_ids'] = []

                reference_data.append(entry)

    print(f"Extracted {len(reference_data)} entries")

    # Write reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"Saved to {output_file}")
    return 0


if __name__ == "__main__":
    exit(extract_reference_data())
