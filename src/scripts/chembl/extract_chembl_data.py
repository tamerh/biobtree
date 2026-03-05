#!/usr/bin/env python3
"""
ChEMBL SQLite Data Extraction Script for Biobtree

This script extracts target, molecule, and activity data from the ChEMBL SQLite
database and generates JSON Lines files for biobtree to consume.

Key features:
- Direct target→uniprot mappings (no intermediate target_component)
- Direct molecule→uniprot mappings (shortcut through activities)
- Activity data with bioactivity measurements

Output files:
  1. chembl_targets.jsonl - Target data with UniProt xrefs
  2. chembl_molecules.jsonl - Molecule data with UniProt xrefs (via activities)
  3. chembl_activities.jsonl - Activity measurements (optional)

Requirements:
    sqlite3 (standard library)

Usage:
    python extract_chembl_data.py --db /path/to/chembl_36.db --output-dir ./data
    python extract_chembl_data.py --db /path/to/chembl_36.db --output-dir ./data --test-mode

Author: Biobtree Project
Date: 2026-02
"""

import argparse
import json
import logging
import os
import sqlite3
import sys
import time
from collections import defaultdict
from datetime import datetime
from typing import Dict, List, Any, Optional

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger(__name__)


def write_json_lines(filepath: str, records: List[Dict], description: str):
    """Write records as JSON lines (one JSON object per line)."""
    logger.info(f"Writing {len(records):,} {description} to {filepath}")
    with open(filepath, 'w', encoding='utf-8') as f:
        for record in records:
            f.write(json.dumps(record, ensure_ascii=False) + '\n')
    logger.info(f"Successfully wrote {filepath}")


def extract_targets(conn: sqlite3.Connection, output_dir: str, test_mode: bool = False) -> int:
    """
    Extract targets with direct UniProt mappings and additional metadata.

    Includes:
    - Basic target info (name, type, organism, tax_id)
    - UniProt mappings (via target_components → component_sequences)
    - Binding sites
    - Target relations (SUBSET OF, OVERLAPS, etc.)
    - Protein classifications
    - Drug mechanisms associated with target
    """
    logger.info("Extracting targets with UniProt mappings and metadata...")
    start_time = time.time()

    # Step 1: Get basic target info with UniProt mappings
    query = """
    SELECT
        td.chembl_id AS target_id,
        td.pref_name AS name,
        td.target_type,
        td.organism,
        td.tax_id,
        td.species_group_flag,
        cs.accession AS uniprot_id,
        cs.description AS component_description
    FROM target_dictionary td
    JOIN target_components tc ON td.tid = tc.tid
    JOIN component_sequences cs ON tc.component_id = cs.component_id
    WHERE cs.db_source = 'SWISS-PROT'
      AND cs.accession IS NOT NULL
      AND cs.accession != ''
    ORDER BY td.chembl_id, cs.accession
    """

    if test_mode:
        query = query.replace("ORDER BY", "LIMIT 1000 --")

    cursor = conn.cursor()
    cursor.execute(query)

    # Aggregate UniProt IDs per target
    targets = defaultdict(lambda: {
        "target_id": "",
        "name": "",
        "target_type": "",
        "organism": "",
        "tax_id": None,
        "is_species_group": False,
        "uniprot_ids": [],
        "component_descriptions": [],
        "binding_sites": [],
        "relations": {"subset_of": [], "superset_of": [], "overlaps": [], "equivalent": []},
        "protein_classes": [],
        "mechanisms": []
    })

    row_count = 0
    target_tids = {}  # chembl_id -> tid mapping for later queries

    for row in cursor:
        target_id, name, target_type, organism, tax_id, species_group_flag, uniprot_id, comp_desc = row
        row_count += 1

        t = targets[target_id]
        t["target_id"] = target_id
        t["name"] = name or ""
        t["target_type"] = target_type or ""
        t["organism"] = organism or ""
        t["tax_id"] = tax_id
        t["is_species_group"] = bool(species_group_flag)

        if uniprot_id and uniprot_id not in t["uniprot_ids"]:
            t["uniprot_ids"].append(uniprot_id)
        if comp_desc and comp_desc not in t["component_descriptions"]:
            t["component_descriptions"].append(comp_desc)

    logger.info(f"Read {row_count:,} rows, found {len(targets):,} unique targets")

    # Step 2: Get binding sites
    logger.info("Loading binding sites...")
    cursor.execute("""
        SELECT td.chembl_id, bs.site_name
        FROM binding_sites bs
        JOIN target_dictionary td ON bs.tid = td.tid
        WHERE bs.site_name IS NOT NULL
    """)
    for target_id, site_name in cursor:
        if target_id in targets and site_name not in targets[target_id]["binding_sites"]:
            targets[target_id]["binding_sites"].append(site_name)

    # Step 3: Get target relations
    logger.info("Loading target relations...")
    cursor.execute("""
        SELECT td1.chembl_id, tr.relationship, td2.chembl_id
        FROM target_relations tr
        JOIN target_dictionary td1 ON tr.tid = td1.tid
        JOIN target_dictionary td2 ON tr.related_tid = td2.tid
    """)
    for target_id, relationship, related_id in cursor:
        if target_id in targets:
            rel_type = relationship.lower().replace(" ", "_")
            if rel_type == "subset_of" and related_id not in targets[target_id]["relations"]["subset_of"]:
                targets[target_id]["relations"]["subset_of"].append(related_id)
            elif rel_type == "superset_of" and related_id not in targets[target_id]["relations"]["superset_of"]:
                targets[target_id]["relations"]["superset_of"].append(related_id)
            elif rel_type == "overlaps_with" and related_id not in targets[target_id]["relations"]["overlaps"]:
                targets[target_id]["relations"]["overlaps"].append(related_id)
            elif rel_type == "equivalent_to" and related_id not in targets[target_id]["relations"]["equivalent"]:
                targets[target_id]["relations"]["equivalent"].append(related_id)

    # Step 4: Get protein classifications with FULL hierarchy (via component_class)
    # First get all classifications linked to targets, then build hierarchy paths
    logger.info("Loading protein classifications with full hierarchy...")

    # Get all protein classification hierarchy (for path building)
    cursor.execute("SELECT protein_class_id, parent_id, pref_name, class_level FROM protein_classification")
    pc_data = {}
    for pc_id, parent_id, name, level in cursor:
        pc_data[pc_id] = {"parent_id": parent_id, "name": name, "level": level}

    def build_path_from_root(pc_id):
        """Build path from root to given class"""
        path_parts = []
        current_id = pc_id
        while current_id is not None and current_id in pc_data:
            path_parts.append(pc_data[current_id]["name"])
            current_id = pc_data[current_id]["parent_id"]
        path_parts.reverse()  # Reverse to get root-to-leaf order
        return "/" + "/".join(path_parts)

    # Get classifications linked to targets via component_class
    cursor.execute("""
        WITH RECURSIVE class_hierarchy AS (
            -- Start with leaf classes linked to targets
            SELECT DISTINCT
                td.chembl_id AS target_id,
                pc.protein_class_id,
                pc.parent_id,
                pc.pref_name,
                pc.short_name,
                pc.class_level
            FROM target_dictionary td
            JOIN target_components tc ON td.tid = tc.tid
            JOIN component_class cc ON tc.component_id = cc.component_id
            JOIN protein_classification pc ON cc.protein_class_id = pc.protein_class_id

            UNION

            -- Walk up to parent classes
            SELECT
                ch.target_id,
                pc.protein_class_id,
                pc.parent_id,
                pc.pref_name,
                pc.short_name,
                pc.class_level
            FROM protein_classification pc
            JOIN class_hierarchy ch ON pc.protein_class_id = ch.parent_id
        )
        SELECT DISTINCT target_id, protein_class_id, pref_name, short_name, class_level
        FROM class_hierarchy
        ORDER BY target_id, class_level
    """)
    for target_id, pc_id, class_name, short_name, class_level in cursor:
        if target_id in targets:
            path = build_path_from_root(pc_id)
            pc = {
                "name": class_name or "",
                "short_name": short_name or "",
                "level": class_level,
                "path": path
            }
            if pc not in targets[target_id]["protein_classes"]:
                targets[target_id]["protein_classes"].append(pc)

    # Step 5: Get drug mechanisms for targets
    logger.info("Loading drug mechanisms...")
    cursor.execute("""
        SELECT DISTINCT td.chembl_id, dm.mechanism_of_action, dm.action_type
        FROM drug_mechanism dm
        JOIN target_dictionary td ON dm.tid = td.tid
        WHERE dm.mechanism_of_action IS NOT NULL
    """)
    for target_id, mechanism, action_type in cursor:
        if target_id in targets:
            mech = {"description": mechanism or "", "action": action_type or ""}
            if mech not in targets[target_id]["mechanisms"]:
                targets[target_id]["mechanisms"].append(mech)

    # Clean up empty relations
    for t in targets.values():
        t["relations"] = {k: v for k, v in t["relations"].items() if v}
        if not t["relations"]:
            del t["relations"]

    # Convert to list and write
    records = list(targets.values())
    output_path = os.path.join(output_dir, "chembl_targets.jsonl")
    write_json_lines(output_path, records, "targets")

    elapsed = time.time() - start_time
    logger.info(f"Target extraction completed in {elapsed:.2f}s")

    return len(records)


def extract_molecules(conn: sqlite3.Connection, output_dir: str, test_mode: bool = False) -> int:
    """
    Extract molecules with their properties and direct UniProt mappings.

    The UniProt mapping comes through activities:
    MOLECULE_DICTIONARY → ACTIVITIES → ASSAYS → TARGET → TARGET_COMPONENTS → COMPONENT_SEQUENCES

    This creates direct molecule→uniprot edges, enabling:
    >>uniprot>>chembl_molecule (instead of the 6-hop chain)

    Includes:
    - Basic properties (name, max_phase, molecule_type)
    - Structural data (smiles, inchi_key)
    - Compound properties (molecular_weight, formula, aromatic_rings, heavy_atoms, qed_weighted, ro3_pass)
    - Drug indications (mesh_id, efo_id, max_phase_for_ind)
    - Synonyms
    - ATC classifications
    - Molecule hierarchy (parent molecule)
    - UniProt mappings (via activities)

    We filter to activities with pchembl_value to ensure meaningful bioactivity.
    """
    logger.info("Extracting molecules with properties and UniProt mappings...")
    start_time = time.time()

    # First, get all molecule properties
    # Columns verified against ChEMBL 36 schema
    mol_query = """
    SELECT
        md.chembl_id AS molecule_id,
        md.pref_name AS name,
        md.max_phase,
        md.molecule_type,
        md.first_approval,
        md.oral,
        md.parenteral,
        md.topical,
        md.black_box_warning,
        md.natural_product,
        md.prodrug,
        cs.canonical_smiles AS smiles,
        cs.standard_inchi AS inchi,
        cs.standard_inchi_key AS inchi_key,
        cp.mw_freebase AS molecular_weight,
        cp.alogp,
        cp.hba,
        cp.hbd,
        cp.psa,
        cp.rtb,
        cp.full_mwt AS full_molecular_weight,
        cp.full_molformula AS formula,
        cp.aromatic_rings,
        cp.heavy_atoms,
        cp.qed_weighted,
        cp.ro3_pass,
        cp.num_ro5_violations
    FROM molecule_dictionary md
    LEFT JOIN compound_structures cs ON md.molregno = cs.molregno
    LEFT JOIN compound_properties cp ON md.molregno = cp.molregno
    """

    if test_mode:
        mol_query += " LIMIT 5000"

    cursor = conn.cursor()
    cursor.execute(mol_query)

    # Build molecule dict
    molecules = {}

    for row in cursor:
        mol_id = row[0]
        molecules[mol_id] = {
            "molecule_id": mol_id,
            "name": row[1] or "",
            "max_phase": row[2],
            "molecule_type": row[3] or "",
            "first_approval": row[4],
            "oral": bool(row[5]) if row[5] is not None else None,
            "parenteral": bool(row[6]) if row[6] is not None else None,
            "topical": bool(row[7]) if row[7] is not None else None,
            "black_box_warning": bool(row[8]) if row[8] is not None else None,
            "natural_product": bool(row[9]) if row[9] is not None else None,
            "prodrug": bool(row[10]) if row[10] is not None else None,
            "smiles": row[11] or "",
            "inchi": row[12] or "",
            "inchi_key": row[13] or "",
            "molecular_weight": row[14],
            "alogp": row[15],
            "hba": row[16],
            "hbd": row[17],
            "psa": row[18],
            "rtb": row[19],
            "full_molecular_weight": row[20],
            "formula": row[21] or "",
            "aromatic_rings": row[22],
            "heavy_atoms": row[23],
            "qed_weighted": row[24],
            "ro3_pass": row[25] or "",
            "lipinski_violations": row[26],
            "uniprot_ids": [],
            "target_ids": [],
            "indications": [],
            "synonyms": [],
            "atc_classifications": [],
            "parent_chembl_id": None
        }

    logger.info(f"Loaded {len(molecules):,} molecules with properties")

    # Step 2: Get drug indications (IDs and phase only, names available via xrefs)
    logger.info("Loading drug indications...")
    cursor.execute("""
        SELECT DISTINCT
            md.chembl_id,
            di.mesh_id,
            di.efo_id,
            di.max_phase_for_ind
        FROM drug_indication di
        JOIN molecule_dictionary md ON di.molregno = md.molregno
        WHERE di.mesh_id IS NOT NULL OR di.efo_id IS NOT NULL
    """)
    for mol_id, mesh_id, efo_id, max_phase in cursor:
        if mol_id in molecules:
            ind = {
                "mesh_id": mesh_id or "",
                "efo_id": efo_id or "",
                "max_phase": max_phase
            }
            if ind not in molecules[mol_id]["indications"]:
                molecules[mol_id]["indications"].append(ind)

    # Step 3: Get synonyms from molecule_synonyms table
    logger.info("Loading molecule synonyms...")
    cursor.execute("""
        SELECT DISTINCT md.chembl_id, ms.synonyms, ms.syn_type
        FROM molecule_synonyms ms
        JOIN molecule_dictionary md ON ms.molregno = md.molregno
        WHERE ms.synonyms IS NOT NULL
    """)
    for mol_id, synonym, syn_type in cursor:
        if mol_id in molecules and synonym:
            syn_entry = {"name": synonym, "type": syn_type or ""}
            if syn_entry not in molecules[mol_id]["synonyms"]:
                molecules[mol_id]["synonyms"].append(syn_entry)

    # Step 3b: Get additional names from compound_records table
    logger.info("Loading compound record names...")
    cursor.execute("""
        SELECT DISTINCT md.chembl_id, cr.compound_name
        FROM compound_records cr
        JOIN molecule_dictionary md ON cr.molregno = md.molregno
        WHERE cr.compound_name IS NOT NULL
          AND cr.compound_name != ''
    """)
    for mol_id, compound_name in cursor:
        if mol_id in molecules and compound_name:
            # Skip very long concatenated names (contain ::)
            if '::' in compound_name:
                continue
            syn_entry = {"name": compound_name, "type": "COMPOUND_RECORD"}
            if syn_entry not in molecules[mol_id]["synonyms"]:
                molecules[mol_id]["synonyms"].append(syn_entry)

    # Step 4: Get ATC classifications
    logger.info("Loading ATC classifications...")
    cursor.execute("""
        SELECT DISTINCT
            md.chembl_id,
            atc.level5 AS atc_code,
            atc.who_name AS description,
            atc.level1,
            atc.level1_description,
            atc.level2,
            atc.level2_description,
            atc.level3,
            atc.level3_description,
            atc.level4,
            atc.level4_description
        FROM molecule_atc_classification mac
        JOIN molecule_dictionary md ON mac.molregno = md.molregno
        JOIN atc_classification atc ON mac.level5 = atc.level5
    """)
    for row in cursor:
        mol_id = row[0]
        if mol_id in molecules:
            atc = {
                "code": row[1] or "",
                "description": row[2] or "",
                "level1": row[3] or "",
                "level1_desc": row[4] or "",
                "level2": row[5] or "",
                "level2_desc": row[6] or "",
                "level3": row[7] or "",
                "level3_desc": row[8] or "",
                "level4": row[9] or "",
                "level4_desc": row[10] or ""
            }
            if atc not in molecules[mol_id]["atc_classifications"]:
                molecules[mol_id]["atc_classifications"].append(atc)

    # Step 5: Get molecule hierarchy (parent and children)
    logger.info("Loading molecule hierarchy (parents)...")
    cursor.execute("""
        SELECT md_child.chembl_id, md_parent.chembl_id
        FROM molecule_hierarchy mh
        JOIN molecule_dictionary md_child ON mh.molregno = md_child.molregno
        JOIN molecule_dictionary md_parent ON mh.parent_molregno = md_parent.molregno
        WHERE mh.molregno != mh.parent_molregno
    """)
    for child_id, parent_id in cursor:
        if child_id in molecules:
            molecules[child_id]["parent_chembl_id"] = parent_id

    # Get children (reverse lookup)
    logger.info("Loading molecule hierarchy (children)...")
    cursor.execute("""
        SELECT md_parent.chembl_id, md_child.chembl_id
        FROM molecule_hierarchy mh
        JOIN molecule_dictionary md_parent ON mh.parent_molregno = md_parent.molregno
        JOIN molecule_dictionary md_child ON mh.molregno = md_child.molregno
        WHERE mh.molregno != mh.parent_molregno
    """)
    for parent_id, child_id in cursor:
        if parent_id in molecules:
            if "child_chembl_ids" not in molecules[parent_id]:
                molecules[parent_id]["child_chembl_ids"] = []
            if child_id not in molecules[parent_id]["child_chembl_ids"]:
                molecules[parent_id]["child_chembl_ids"].append(child_id)

    # Step 6: Get UniProt mappings through activities (with pchembl filter)
    logger.info("Loading UniProt mappings via activities...")
    uniprot_query = """
    SELECT DISTINCT
        md.chembl_id AS molecule_id,
        cs.accession AS uniprot_id,
        td.chembl_id AS target_id
    FROM molecule_dictionary md
    JOIN activities act ON md.molregno = act.molregno
    JOIN assays a ON act.assay_id = a.assay_id
    JOIN target_dictionary td ON a.tid = td.tid
    JOIN target_components tc ON td.tid = tc.tid
    JOIN component_sequences cs ON tc.component_id = cs.component_id
    WHERE cs.db_source = 'SWISS-PROT'
      AND cs.accession IS NOT NULL
      AND cs.accession != ''
      AND act.pchembl_value IS NOT NULL
    """

    if test_mode:
        uniprot_query += " LIMIT 10000"

    cursor.execute(uniprot_query)

    mapping_count = 0
    for row in cursor:
        mol_id, uniprot_id, target_id = row
        mapping_count += 1

        if mol_id in molecules:
            if uniprot_id and uniprot_id not in molecules[mol_id]["uniprot_ids"]:
                molecules[mol_id]["uniprot_ids"].append(uniprot_id)
            if target_id and target_id not in molecules[mol_id]["target_ids"]:
                molecules[mol_id]["target_ids"].append(target_id)

    logger.info(f"Processed {mapping_count:,} activity-based UniProt mappings")

    # Count molecules with UniProt mappings
    with_uniprot = sum(1 for m in molecules.values() if m["uniprot_ids"])
    logger.info(f"Molecules with UniProt mappings: {with_uniprot:,} ({100*with_uniprot/len(molecules):.1f}%)")

    # Clean up empty lists and None values
    for mol in molecules.values():
        if not mol["indications"]:
            del mol["indications"]
        if not mol["synonyms"]:
            del mol["synonyms"]
        if not mol["atc_classifications"]:
            del mol["atc_classifications"]
        if mol["parent_chembl_id"] is None:
            del mol["parent_chembl_id"]

    # Convert to list and write
    records = list(molecules.values())
    output_path = os.path.join(output_dir, "chembl_molecules.jsonl")
    write_json_lines(output_path, records, "molecules")

    elapsed = time.time() - start_time
    logger.info(f"Molecule extraction completed in {elapsed:.2f}s")

    return len(records)


def extract_activities(conn: sqlite3.Connection, output_dir: str, test_mode: bool = False) -> int:
    """
    Extract bioactivity data linking molecules to targets and proteins.

    This provides detailed activity measurements (IC50, Ki, EC50, etc.)
    for users who need quantitative data.

    Includes BAO (BioAssay Ontology) endpoint annotation for activity type classification.
    """
    logger.info("Extracting activity data...")
    start_time = time.time()

    query = """
    SELECT
        act.activity_id,
        md.chembl_id AS molecule_id,
        td.chembl_id AS target_id,
        cs.accession AS uniprot_id,
        act.standard_type,
        act.standard_relation,
        act.standard_value,
        act.standard_units,
        act.pchembl_value,
        a.assay_type,
        a.confidence_score,
        act.bao_endpoint
    FROM activities act
    JOIN molecule_dictionary md ON act.molregno = md.molregno
    JOIN assays a ON act.assay_id = a.assay_id
    JOIN target_dictionary td ON a.tid = td.tid
    JOIN target_components tc ON td.tid = tc.tid
    JOIN component_sequences cs ON tc.component_id = cs.component_id
    WHERE cs.db_source = 'SWISS-PROT'
      AND cs.accession IS NOT NULL
      AND act.pchembl_value IS NOT NULL
    ORDER BY act.activity_id
    """

    if test_mode:
        query = query.replace("ORDER BY", "LIMIT 10000 --")

    cursor = conn.cursor()
    cursor.execute(query)

    records = []
    for row in cursor:
        # Convert BAO_0000190 to BAO:0000190 format
        bao_endpoint = row[11] or ""
        if bao_endpoint and bao_endpoint.startswith("BAO_"):
            bao_endpoint = "BAO:" + bao_endpoint[4:]

        record = {
            "activity_id": row[0],
            "molecule_id": row[1],
            "target_id": row[2],
            "uniprot_id": row[3],
            "standard_type": row[4] or "",
            "standard_relation": row[5] or "",
            "standard_value": row[6],
            "standard_units": row[7] or "",
            "pchembl_value": row[8],
            "assay_type": row[9] or "",
            "confidence_score": row[10],
            "bao_endpoint": bao_endpoint
        }
        records.append(record)

    logger.info(f"Found {len(records):,} activity records")

    output_path = os.path.join(output_dir, "chembl_activities.jsonl")
    write_json_lines(output_path, records, "activities")

    elapsed = time.time() - start_time
    logger.info(f"Activity extraction completed in {elapsed:.2f}s")

    return len(records)


def extract_assays(conn: sqlite3.Connection, output_dir: str, test_mode: bool = False) -> int:
    """
    Extract assay data with target links.

    Includes BAO (BioAssay Ontology) format annotation for assay type classification.
    """
    logger.info("Extracting assays...")
    start_time = time.time()

    query = """
    SELECT
        a.chembl_id AS assay_id,
        a.description,
        a.assay_type,
        a.assay_test_type,
        a.confidence_score,
        a.assay_category,
        a.assay_cell_type,
        a.assay_tissue,
        a.assay_subcellular_fraction,
        a.assay_strain,
        td.chembl_id AS target_id,
        d.chembl_id AS document_id,
        src.src_description AS source,
        a.bao_format
    FROM assays a
    LEFT JOIN target_dictionary td ON a.tid = td.tid
    LEFT JOIN docs d ON a.doc_id = d.doc_id
    LEFT JOIN source src ON a.src_id = src.src_id
    ORDER BY a.assay_id
    """

    if test_mode:
        query = query.replace("ORDER BY", "LIMIT 5000 --")

    cursor = conn.cursor()
    cursor.execute(query)

    records = []
    for row in cursor:
        # Convert BAO_0000019 to BAO:0000019 format
        bao_format = row[13] or ""
        if bao_format and bao_format.startswith("BAO_"):
            bao_format = "BAO:" + bao_format[4:]

        record = {
            "assay_id": row[0],
            "description": row[1] or "",
            "assay_type": row[2] or "",
            "test_type": row[3] or "",
            "confidence_score": row[4],
            "category": row[5] or "",
            "cell_type": row[6] or "",
            "tissue": row[7] or "",
            "subcellular_fraction": row[8] or "",
            "strain": row[9] or "",
            "target_id": row[10] or "",
            "document_id": row[11] or "",
            "source": row[12] or "",
            "bao_format": bao_format
        }
        records.append(record)

    logger.info(f"Found {len(records):,} assay records")

    output_path = os.path.join(output_dir, "chembl_assays.jsonl")
    write_json_lines(output_path, records, "assays")

    elapsed = time.time() - start_time
    logger.info(f"Assay extraction completed in {elapsed:.2f}s")

    return len(records)


def extract_documents(conn: sqlite3.Connection, output_dir: str, test_mode: bool = False) -> int:
    """
    Extract document/literature data.
    """
    logger.info("Extracting documents...")
    start_time = time.time()

    query = """
    SELECT
        d.chembl_id AS document_id,
        d.title,
        d.doc_type,
        d.pubmed_id,
        d.doi,
        d.journal,
        d.year,
        d.volume,
        d.first_page,
        d.last_page,
        d.authors
    FROM docs d
    ORDER BY d.doc_id
    """

    if test_mode:
        query = query.replace("ORDER BY", "LIMIT 1000 --")

    cursor = conn.cursor()
    cursor.execute(query)

    records = []
    for row in cursor:
        record = {
            "document_id": row[0],
            "title": row[1] or "",
            "doc_type": row[2] or "",
            "pubmed_id": row[3],
            "doi": row[4] or "",
            "journal": row[5] or "",
            "year": row[6],
            "volume": row[7] or "",
            "first_page": row[8] or "",
            "last_page": row[9] or "",
            "authors": row[10] or ""
        }
        records.append(record)

    logger.info(f"Found {len(records):,} document records")

    output_path = os.path.join(output_dir, "chembl_documents.jsonl")
    write_json_lines(output_path, records, "documents")

    elapsed = time.time() - start_time
    logger.info(f"Document extraction completed in {elapsed:.2f}s")

    return len(records)


def extract_cell_lines(conn: sqlite3.Connection, output_dir: str, test_mode: bool = False) -> int:
    """
    Extract cell line data.
    """
    logger.info("Extracting cell lines...")
    start_time = time.time()

    query = """
    SELECT
        c.chembl_id AS cell_line_id,
        c.cell_name,
        c.cell_description,
        c.cell_source_tissue,
        c.cell_source_organism,
        c.cell_source_tax_id,
        c.clo_id,
        c.efo_id,
        c.cellosaurus_id
    FROM cell_dictionary c
    ORDER BY c.cell_id
    """

    if test_mode:
        query = query.replace("ORDER BY", "LIMIT 500 --")

    cursor = conn.cursor()
    cursor.execute(query)

    records = []
    for row in cursor:
        record = {
            "cell_line_id": row[0],
            "name": row[1] or "",
            "description": row[2] or "",
            "tissue": row[3] or "",
            "organism": row[4] or "",
            "tax_id": row[5],
            "clo_id": row[6] or "",
            "efo_id": row[7] or "",
            "cellosaurus_id": row[8] or ""
        }
        records.append(record)

    logger.info(f"Found {len(records):,} cell line records")

    output_path = os.path.join(output_dir, "chembl_cell_lines.jsonl")
    write_json_lines(output_path, records, "cell lines")

    elapsed = time.time() - start_time
    logger.info(f"Cell line extraction completed in {elapsed:.2f}s")

    return len(records)


def get_database_stats(conn: sqlite3.Connection) -> Dict[str, int]:
    """Get counts from main tables for verification."""
    stats = {}
    tables = [
        "molecule_dictionary",
        "target_dictionary",
        "target_components",
        "component_sequences",
        "activities",
        "assays",
        "docs",
        "cell_dictionary"
    ]

    cursor = conn.cursor()
    for table in tables:
        try:
            cursor.execute(f"SELECT COUNT(*) FROM {table}")
            stats[table] = cursor.fetchone()[0]
        except sqlite3.Error:
            stats[table] = -1

    return stats


def main():
    parser = argparse.ArgumentParser(
        description="Extract ChEMBL data from SQLite for Biobtree integration"
    )
    parser.add_argument(
        "--db", "-d",
        required=True,
        help="Path to ChEMBL SQLite database file (chembl_XX.db)"
    )
    parser.add_argument(
        "--output-dir", "-o",
        default="./chembl_data",
        help="Output directory for JSON Lines files (default: ./chembl_data)"
    )
    parser.add_argument(
        "--test-mode", "-t",
        action="store_true",
        help="Run in test mode with limited records"
    )
    parser.add_argument(
        "--skip-activities",
        action="store_true",
        help="Skip extraction of activities (large dataset)"
    )

    args = parser.parse_args()

    # Verify database exists
    if not os.path.exists(args.db):
        logger.error(f"Database file not found: {args.db}")
        sys.exit(1)

    # Create output directory
    os.makedirs(args.output_dir, exist_ok=True)
    logger.info(f"Output directory: {args.output_dir}")

    total_start = time.time()

    # Connect to database
    logger.info(f"Connecting to database: {args.db}")
    conn = sqlite3.connect(args.db)
    conn.row_factory = None  # Use tuple for efficiency

    try:
        # Show database stats
        stats = get_database_stats(conn)
        logger.info("Database statistics:")
        for table, count in stats.items():
            logger.info(f"  - {table}: {count:,}")

        extraction_stats = {}

        # 1. Extract targets with UniProt mappings
        extraction_stats["targets"] = extract_targets(conn, args.output_dir, args.test_mode)

        # 2. Extract molecules with UniProt mappings (via activities)
        extraction_stats["molecules"] = extract_molecules(conn, args.output_dir, args.test_mode)

        # 3. Extract activities (optional, large dataset)
        if not args.skip_activities:
            extraction_stats["activities"] = extract_activities(conn, args.output_dir, args.test_mode)
        else:
            logger.info("Skipping activities extraction (--skip-activities)")

        # 4. Extract assays
        extraction_stats["assays"] = extract_assays(conn, args.output_dir, args.test_mode)

        # 5. Extract documents
        extraction_stats["documents"] = extract_documents(conn, args.output_dir, args.test_mode)

        # 6. Extract cell lines
        extraction_stats["cell_lines"] = extract_cell_lines(conn, args.output_dir, args.test_mode)

        # Write metadata
        metadata = {
            "extraction_date": datetime.now().isoformat(),
            "database": os.path.basename(args.db),
            "test_mode": args.test_mode,
            "database_stats": stats,
            "extraction_stats": extraction_stats
        }
        metadata_path = os.path.join(args.output_dir, "extraction_metadata.json")
        with open(metadata_path, 'w') as f:
            json.dump(metadata, f, indent=2)

        total_elapsed = time.time() - total_start
        logger.info("=" * 60)
        logger.info("EXTRACTION COMPLETE")
        logger.info(f"Total time: {total_elapsed:.2f}s ({total_elapsed/60:.1f} minutes)")
        logger.info(f"Output directory: {args.output_dir}")
        logger.info("Extraction statistics:")
        for key, value in extraction_stats.items():
            logger.info(f"  - {key}: {value:,} records")
        logger.info("=" * 60)

    finally:
        conn.close()
        logger.info("Database connection closed")


if __name__ == "__main__":
    main()
