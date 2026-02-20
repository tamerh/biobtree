#!/usr/bin/env python3
"""
ChEMBL Reference Data Extractor

Extracts reference data directly from ChEMBL SQLite database for testing.
This replaces the old API-based extraction and works with the new SQLite architecture.

Usage:
    python3 extract_reference_data.py [--db /path/to/chembl.db] [--from-test-build]

After running test build, use --from-test-build to extract data for actual test IDs:
    ./biobtree -d "chembl_target,chembl_molecule,..." test
    python3 extract_reference_data.py --from-test-build
"""

import json
import sqlite3
import argparse
from pathlib import Path


def load_test_ids(id_file):
    """Load IDs from test build reference file."""
    ids = []
    with open(id_file, 'r') as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#'):
                ids.append(line)
    return ids


def extract_targets(cursor, ids=None, limit=20):
    """Extract target reference data."""
    if ids:
        placeholders = ','.join('?' * len(ids))
        cursor.execute(f"""
            SELECT
                td.chembl_id,
                td.pref_name,
                td.target_type,
                td.organism,
                td.tax_id
            FROM target_dictionary td
            WHERE td.chembl_id IN ({placeholders})
        """, ids)
    else:
        cursor.execute("""
            SELECT
                td.chembl_id,
                td.pref_name,
                td.target_type,
                td.organism,
                td.tax_id
            FROM target_dictionary td
            WHERE td.chembl_id IS NOT NULL
            LIMIT ?
        """, (limit,))

    targets = []
    for row in cursor.fetchall():
        target = {
            "target_chembl_id": row[0],
            "pref_name": row[1],
            "target_type": row[2],
            "organism": row[3],
            "tax_id": row[4]
        }

        # Get UniProt accessions
        cursor.execute("""
            SELECT cs.accession
            FROM target_components tc
            JOIN component_sequences cs ON tc.component_id = cs.component_id
            WHERE tc.tid = (SELECT tid FROM target_dictionary WHERE chembl_id = ?)
        """, (row[0],))
        target["uniprot_ids"] = [r[0] for r in cursor.fetchall() if r[0]]

        targets.append(target)

    return targets


def extract_molecules(cursor, ids=None, limit=20):
    """Extract molecule reference data."""
    if ids:
        placeholders = ','.join('?' * len(ids))
        cursor.execute(f"""
            SELECT
                md.chembl_id,
                md.pref_name,
                md.molecule_type,
                md.max_phase
            FROM molecule_dictionary md
            WHERE md.chembl_id IN ({placeholders})
        """, ids)
    else:
        cursor.execute("""
            SELECT
                md.chembl_id,
                md.pref_name,
                md.molecule_type,
                md.max_phase
            FROM molecule_dictionary md
            WHERE md.chembl_id IS NOT NULL
            LIMIT ?
        """, (limit,))

    molecules = []
    for row in cursor.fetchall():
        mol_id = row[0]
        molecule = {
            "molecule_chembl_id": mol_id,
            "pref_name": row[1],
            "molecule_type": row[2],
            "max_phase": row[3]
        }

        # Get synonyms
        cursor.execute("""
            SELECT syn_type, synonyms
            FROM molecule_synonyms
            WHERE molregno = (SELECT molregno FROM molecule_dictionary WHERE chembl_id = ?)
        """, (mol_id,))
        molecule["synonyms"] = [{"type": r[0], "name": r[1]} for r in cursor.fetchall() if r[1]]

        molecules.append(molecule)

    return molecules


def extract_activities(cursor, ids=None, limit=20):
    """Extract activity reference data."""
    if ids:
        # Convert CHEMBL_ACT_123 to 123
        activity_ids = [int(id.replace('CHEMBL_ACT_', '')) for id in ids if id.startswith('CHEMBL_ACT_')]
        if not activity_ids:
            return []
        placeholders = ','.join('?' * len(activity_ids))
        cursor.execute(f"""
            SELECT
                a.activity_id,
                md.chembl_id as molecule_chembl_id,
                ass.chembl_id as assay_chembl_id,
                td.chembl_id as target_chembl_id,
                doc.chembl_id as document_chembl_id,
                a.standard_type,
                a.standard_value,
                a.standard_units,
                td.organism as target_organism
            FROM activities a
            JOIN molecule_dictionary md ON a.molregno = md.molregno
            JOIN assays ass ON a.assay_id = ass.assay_id
            LEFT JOIN target_dictionary td ON ass.tid = td.tid
            LEFT JOIN docs doc ON a.doc_id = doc.doc_id
            WHERE a.activity_id IN ({placeholders})
        """, activity_ids)
    else:
        cursor.execute("""
            SELECT
                a.activity_id,
                md.chembl_id as molecule_chembl_id,
                ass.chembl_id as assay_chembl_id,
                td.chembl_id as target_chembl_id,
                doc.chembl_id as document_chembl_id,
                a.standard_type,
                a.standard_value,
                a.standard_units,
                td.organism as target_organism
            FROM activities a
            JOIN molecule_dictionary md ON a.molregno = md.molregno
            JOIN assays ass ON a.assay_id = ass.assay_id
            LEFT JOIN target_dictionary td ON ass.tid = td.tid
            LEFT JOIN docs doc ON a.doc_id = doc.doc_id
            WHERE md.chembl_id IS NOT NULL
            LIMIT ?
        """, (limit,))

    activities = []
    for row in cursor.fetchall():
        activity = {
            "activity_chembl_id": f"CHEMBL_ACT_{row[0]}",
            "molecule_chembl_id": row[1],
            "assay_chembl_id": row[2],
            "target_chembl_id": row[3],
            "document_chembl_id": row[4],
            "standard_type": row[5],
            "standard_value": row[6],
            "standard_units": row[7],
            "target_organism": row[8]
        }
        activities.append(activity)

    return activities


def extract_assays(cursor, ids=None, limit=20):
    """Extract assay reference data."""
    if ids:
        placeholders = ','.join('?' * len(ids))
        cursor.execute(f"""
            SELECT
                ass.chembl_id,
                ass.description,
                ass.assay_type,
                ass.assay_organism,
                ass.assay_tissue,
                td.chembl_id as target_chembl_id,
                doc.chembl_id as document_chembl_id
            FROM assays ass
            LEFT JOIN target_dictionary td ON ass.tid = td.tid
            LEFT JOIN docs doc ON ass.doc_id = doc.doc_id
            WHERE ass.chembl_id IN ({placeholders})
        """, ids)
    else:
        cursor.execute("""
            SELECT
                ass.chembl_id,
                ass.description,
                ass.assay_type,
                ass.assay_organism,
                ass.assay_tissue,
                td.chembl_id as target_chembl_id,
                doc.chembl_id as document_chembl_id
            FROM assays ass
            LEFT JOIN target_dictionary td ON ass.tid = td.tid
            LEFT JOIN docs doc ON ass.doc_id = doc.doc_id
            WHERE ass.chembl_id IS NOT NULL
            LIMIT ?
        """, (limit,))

    assays = []
    for row in cursor.fetchall():
        assay = {
            "assay_chembl_id": row[0],
            "description": row[1],
            "assay_type": row[2],
            "assay_organism": row[3],
            "assay_tissue": row[4],
            "target_chembl_id": row[5],
            "document_chembl_id": row[6]
        }
        assays.append(assay)

    return assays


def extract_documents(cursor, ids=None, limit=20):
    """Extract document reference data."""
    if ids:
        placeholders = ','.join('?' * len(ids))
        cursor.execute(f"""
            SELECT
                chembl_id,
                title,
                doc_type,
                journal,
                year,
                pubmed_id,
                doi
            FROM docs
            WHERE chembl_id IN ({placeholders})
        """, ids)
    else:
        cursor.execute("""
            SELECT
                chembl_id,
                title,
                doc_type,
                journal,
                year,
                pubmed_id,
                doi
            FROM docs
            WHERE chembl_id IS NOT NULL
            LIMIT ?
        """, (limit,))

    documents = []
    for row in cursor.fetchall():
        doc = {
            "document_chembl_id": row[0],
            "title": row[1],
            "doc_type": row[2],
            "journal": row[3],
            "year": row[4],
            "pubmed_id": row[5],
            "doi": row[6]
        }
        documents.append(doc)

    return documents


def extract_cell_lines(cursor, ids=None, limit=20):
    """Extract cell line reference data."""
    if ids:
        placeholders = ','.join('?' * len(ids))
        cursor.execute(f"""
            SELECT
                chembl_id,
                cell_name,
                cell_description,
                cell_source_organism,
                cell_source_tax_id,
                cell_source_tissue,
                cellosaurus_id
            FROM cell_dictionary
            WHERE chembl_id IN ({placeholders})
        """, ids)
    else:
        cursor.execute("""
            SELECT
                chembl_id,
                cell_name,
                cell_description,
                cell_source_organism,
                cell_source_tax_id,
                cell_source_tissue,
                cellosaurus_id
            FROM cell_dictionary
            WHERE chembl_id IS NOT NULL
            LIMIT ?
        """, (limit,))

    cell_lines = []
    for row in cursor.fetchall():
        cell = {
            "cell_chembl_id": row[0],
            "cell_name": row[1],
            "cell_description": row[2],
            "cell_source_organism": row[3],
            "cell_source_tax_id": row[4],
            "cell_source_tissue": row[5],
            "cellosaurus_id": row[6]
        }
        cell_lines.append(cell)

    return cell_lines


def main():
    parser = argparse.ArgumentParser(description='Extract ChEMBL reference data')
    parser.add_argument('--db', default='raw_data/chembl/chembl_36/chembl_36_sqlite/chembl_36.db',
                        help='Path to ChEMBL SQLite database')
    parser.add_argument('--limit', type=int, default=20,
                        help='Number of entries per entity type')
    parser.add_argument('--from-test-build', action='store_true',
                        help='Use IDs from test_out/reference/ (run after test build)')
    args = parser.parse_args()

    # Connect to database
    db_path = Path(args.db)
    if not db_path.exists():
        print(f"Error: Database not found at {db_path}")
        return 1

    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()

    print(f"Extracting reference data from {db_path}...")

    # Load IDs from test build if requested
    if args.from_test_build:
        project_root = Path(__file__).parent.parent.parent.parent
        ref_dir = project_root / "test_out" / "reference"

        if not ref_dir.exists():
            print(f"Error: Test reference directory not found at {ref_dir}")
            print("Run test build first: ./biobtree -d 'chembl_target,chembl_molecule,...' test")
            return 1

        print(f"Loading IDs from {ref_dir}...")

        target_ids = load_test_ids(ref_dir / "chembl_target_ids.txt") if (ref_dir / "chembl_target_ids.txt").exists() else None
        molecule_ids = load_test_ids(ref_dir / "chembl_molecule_ids.txt") if (ref_dir / "chembl_molecule_ids.txt").exists() else None
        activity_ids = load_test_ids(ref_dir / "chembl_activity_ids.txt") if (ref_dir / "chembl_activity_ids.txt").exists() else None
        assay_ids = load_test_ids(ref_dir / "chembl_assay_ids.txt") if (ref_dir / "chembl_assay_ids.txt").exists() else None
        document_ids = load_test_ids(ref_dir / "chembl_document_ids.txt") if (ref_dir / "chembl_document_ids.txt").exists() else None
        cell_line_ids = load_test_ids(ref_dir / "chembl_cell_line_ids.txt") if (ref_dir / "chembl_cell_line_ids.txt").exists() else None

        # Extract all entity types using test build IDs
        reference_data = {
            "targets": extract_targets(cursor, ids=target_ids),
            "molecules": extract_molecules(cursor, ids=molecule_ids),
            "activities": extract_activities(cursor, ids=activity_ids),
            "assays": extract_assays(cursor, ids=assay_ids),
            "documents": extract_documents(cursor, ids=document_ids),
            "cell_lines": extract_cell_lines(cursor, ids=cell_line_ids)
        }
    else:
        # Extract all entity types (first N from database)
        reference_data = {
            "targets": extract_targets(cursor, limit=args.limit),
            "molecules": extract_molecules(cursor, limit=args.limit),
            "activities": extract_activities(cursor, limit=args.limit),
            "assays": extract_assays(cursor, limit=args.limit),
            "documents": extract_documents(cursor, limit=args.limit),
            "cell_lines": extract_cell_lines(cursor, limit=args.limit)
        }

    conn.close()

    # Save to file
    output_file = Path(__file__).parent / "reference_data.json"
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    # Print summary
    print(f"\nExtracted reference data:")
    for entity_type, entries in reference_data.items():
        print(f"  {entity_type}: {len(entries)} entries")

    print(f"\nSaved to {output_file}")

    # Also save IDs
    ids_file = Path(__file__).parent / "chembl_ids.txt"
    with open(ids_file, 'w') as f:
        f.write("# ChEMBL Test IDs\n")
        f.write("# Generated by extract_reference_data.py\n\n")

        f.write("## Targets\n")
        for t in reference_data["targets"]:
            f.write(f"{t['target_chembl_id']}\n")

        f.write("\n## Molecules\n")
        for m in reference_data["molecules"]:
            f.write(f"{m['molecule_chembl_id']}\n")

        f.write("\n## Activities\n")
        for a in reference_data["activities"]:
            f.write(f"{a['activity_chembl_id']}\n")

        f.write("\n## Assays\n")
        for a in reference_data["assays"]:
            f.write(f"{a['assay_chembl_id']}\n")

        f.write("\n## Documents\n")
        for d in reference_data["documents"]:
            f.write(f"{d['document_chembl_id']}\n")

        f.write("\n## Cell Lines\n")
        for c in reference_data["cell_lines"]:
            f.write(f"{c['cell_chembl_id']}\n")

    print(f"Saved IDs to {ids_file}")

    return 0


if __name__ == "__main__":
    exit(main())
