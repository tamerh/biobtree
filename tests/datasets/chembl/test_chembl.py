#!/usr/bin/env python3
"""
ChEMBL Consolidated Test Suite

Tests all ChEMBL datasets (target, molecule, activity, assay, document, cell_line)
using the common test framework. Uses SQLite-based data extraction.

Architecture Notes (v2.0):
- SQLite-based extraction (not RDF)
- Direct uniprot xrefs on targets (no target_component intermediary)
- Molecular properties delegated to PubChem (smiles, inchi, weight, etc.)
- Protein classifications delegated to UniProt/GO/InterPro

Note: This script is called by the main orchestrator (tests/run_tests.py)
which manages the biobtree web server lifecycle.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class ChEMBLTests:
    """ChEMBL custom tests for all entity types"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    # =========================================================================
    # TARGET TESTS
    # =========================================================================

    @test
    def test_target_with_name(self):
        """Check target has preferred name"""
        entry = next(
            (e for e in self.runner.reference_data.get("targets", [])
             if e.get("pref_name")),
            None
        )
        if not entry:
            return False, "No target with pref_name in reference"

        target_id = entry["target_chembl_id"]
        pref_name = entry["pref_name"][:60]

        data = self.runner.lookup(target_id)
        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} has name: {pref_name}..."

    @test
    def test_target_with_type(self):
        """Check target has type"""
        entry = next(
            (e for e in self.runner.reference_data.get("targets", [])
             if e.get("target_type")),
            None
        )
        if not entry:
            return False, "No target with type in reference"

        target_id = entry["target_chembl_id"]
        target_type = entry["target_type"]

        data = self.runner.lookup(target_id)
        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} has type: {target_type}"

    @test
    def test_target_with_taxonomy(self):
        """Check target has taxonomy ID"""
        entry = next(
            (e for e in self.runner.reference_data.get("targets", [])
             if e.get("tax_id")),
            None
        )
        if not entry:
            return False, "No target with tax_id in reference"

        target_id = entry["target_chembl_id"]
        tax_id = entry["tax_id"]

        data = self.runner.lookup(target_id)
        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} has taxonomy: {tax_id}"

    @test
    def test_target_uniprot_xref(self):
        """Check target has direct UniProt cross-reference"""
        entry = next(
            (e for e in self.runner.reference_data.get("targets", [])
             if e.get("uniprot_ids") and len(e["uniprot_ids"]) > 0),
            None
        )
        if not entry:
            return False, "No target with UniProt IDs in reference"

        target_id = entry["target_chembl_id"]
        uniprot_id = entry["uniprot_ids"][0]

        data = self.runner.lookup(target_id)
        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        # Check for uniprot xref in the first result
        result = data["results"][0]
        if self.runner.has_xref(result, "uniprot", uniprot_id):
            return True, f"{target_id} has UniProt xref: {uniprot_id}"
        return False, f"{target_id} missing UniProt xref: {uniprot_id}"

    @test
    def test_single_protein_target(self):
        """Check single protein target type"""
        entry = next(
            (e for e in self.runner.reference_data.get("targets", [])
             if e.get("target_type") == "SINGLE PROTEIN"),
            None
        )
        if not entry:
            return False, "No SINGLE PROTEIN target in reference"

        target_id = entry["target_chembl_id"]

        data = self.runner.lookup(target_id)
        if not data or not data.get("results"):
            return False, f"No results for {target_id}"

        return True, f"{target_id} is SINGLE PROTEIN type"

    # =========================================================================
    # MOLECULE TESTS
    # =========================================================================

    @test
    def test_molecule_with_name(self):
        """Check molecule has preferred name"""
        entry = next(
            (e for e in self.runner.reference_data.get("molecules", [])
             if e.get("pref_name")),
            None
        )
        if not entry:
            return False, "No molecule with pref_name in reference"

        mol_id = entry["molecule_chembl_id"]
        pref_name = entry["pref_name"]

        data = self.runner.lookup(mol_id)
        if not data or not data.get("results"):
            return False, f"No results for {mol_id}"

        return True, f"{mol_id} has name: {pref_name}"

    @test
    def test_molecule_with_type(self):
        """Check molecule type"""
        entry = next(
            (e for e in self.runner.reference_data.get("molecules", [])
             if e.get("molecule_type")),
            None
        )
        if not entry:
            return False, "No molecule with type in reference"

        mol_id = entry["molecule_chembl_id"]
        mol_type = entry["molecule_type"]

        data = self.runner.lookup(mol_id)
        if not data or not data.get("results"):
            return False, f"No results for {mol_id}"

        return True, f"{mol_id} is type: {mol_type}"

    @test
    def test_molecule_with_synonyms(self):
        """Check molecule has synonyms (altNames)"""
        entry = next(
            (e for e in self.runner.reference_data.get("molecules", [])
             if e.get("synonyms") and len(e["synonyms"]) > 0),
            None
        )
        if not entry:
            # Skip test if no molecules have synonyms (depends on test data)
            return True, "SKIP: No molecule with synonyms in test data"

        mol_id = entry["molecule_chembl_id"]
        num_syns = len(entry["synonyms"])

        data = self.runner.lookup(mol_id)
        if not data or not data.get("results"):
            return False, f"No results for {mol_id}"

        return True, f"{mol_id} has {num_syns} synonym(s)"

    @test
    def test_small_molecule_type(self):
        """Check small molecule type"""
        entry = next(
            (e for e in self.runner.reference_data.get("molecules", [])
             if e.get("molecule_type") == "Small molecule"),
            None
        )
        if not entry:
            return False, "No small molecule in reference"

        mol_id = entry["molecule_chembl_id"]

        data = self.runner.lookup(mol_id)
        if not data or not data.get("results"):
            return False, f"No results for {mol_id}"

        return True, f"{mol_id} is Small molecule type"

    # =========================================================================
    # ACTIVITY TESTS
    # =========================================================================

    @test
    def test_activity_with_assay(self):
        """Check activity has assay reference"""
        entry = next(
            (e for e in self.runner.reference_data.get("activities", [])
             if e.get("assay_chembl_id")),
            None
        )
        if not entry:
            return False, "No activity with assay in reference"

        activity_id = entry["activity_chembl_id"]
        assay_id = entry["assay_chembl_id"]

        data = self.runner.lookup(activity_id)
        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"{activity_id} linked to assay {assay_id}"

    @test
    def test_activity_with_molecule(self):
        """Check activity has molecule reference"""
        entry = next(
            (e for e in self.runner.reference_data.get("activities", [])
             if e.get("molecule_chembl_id")),
            None
        )
        if not entry:
            return False, "No activity with molecule in reference"

        activity_id = entry["activity_chembl_id"]
        molecule_id = entry["molecule_chembl_id"]

        data = self.runner.lookup(activity_id)
        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"{activity_id} linked to molecule {molecule_id}"

    @test
    def test_activity_with_target(self):
        """Check activity has target reference"""
        entry = next(
            (e for e in self.runner.reference_data.get("activities", [])
             if e.get("target_chembl_id")),
            None
        )
        if not entry:
            return False, "No activity with target in reference"

        activity_id = entry["activity_chembl_id"]
        target_id = entry["target_chembl_id"]

        data = self.runner.lookup(activity_id)
        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"{activity_id} linked to target {target_id}"

    @test
    def test_activity_with_measurement(self):
        """Check activity has measurement value"""
        entry = next(
            (e for e in self.runner.reference_data.get("activities", [])
             if e.get("standard_value") and e.get("standard_type")),
            None
        )
        if not entry:
            return False, "No activity with measurement in reference"

        activity_id = entry["activity_chembl_id"]
        std_type = entry["standard_type"]
        std_value = entry["standard_value"]
        units = entry.get("standard_units", "")

        data = self.runner.lookup(activity_id)
        if not data or not data.get("results"):
            return False, f"No results for {activity_id}"

        return True, f"{activity_id}: {std_type} = {std_value} {units}"

    # =========================================================================
    # ASSAY TESTS
    # =========================================================================

    @test
    def test_assay_with_description(self):
        """Check assay has description"""
        entry = next(
            (e for e in self.runner.reference_data.get("assays", [])
             if e.get("description")),
            None
        )
        if not entry:
            return False, "No assay with description in reference"

        assay_id = entry["assay_chembl_id"]
        desc = entry["description"][:60]

        data = self.runner.lookup(assay_id)
        if not data or not data.get("results"):
            return False, f"No results for {assay_id}"

        return True, f"{assay_id} has description: {desc}..."

    @test
    def test_assay_with_type(self):
        """Check assay has type"""
        entry = next(
            (e for e in self.runner.reference_data.get("assays", [])
             if e.get("assay_type")),
            None
        )
        if not entry:
            return False, "No assay with type in reference"

        assay_id = entry["assay_chembl_id"]
        assay_type = entry["assay_type"]

        data = self.runner.lookup(assay_id)
        if not data or not data.get("results"):
            return False, f"No results for {assay_id}"

        return True, f"{assay_id} has type: {assay_type}"

    @test
    def test_assay_with_target(self):
        """Check assay has target reference"""
        entry = next(
            (e for e in self.runner.reference_data.get("assays", [])
             if e.get("target_chembl_id")),
            None
        )
        if not entry:
            return False, "No assay with target in reference"

        assay_id = entry["assay_chembl_id"]
        target_id = entry["target_chembl_id"]

        data = self.runner.lookup(assay_id)
        if not data or not data.get("results"):
            return False, f"No results for {assay_id}"

        return True, f"{assay_id} has target: {target_id}"

    # =========================================================================
    # DOCUMENT TESTS
    # =========================================================================

    @test
    def test_publication_document(self):
        """Check document with PUBLICATION type"""
        entry = next(
            (e for e in self.runner.reference_data.get("documents", [])
             if e.get("doc_type") == "PUBLICATION"),
            None
        )
        if not entry:
            return False, "No PUBLICATION document in reference"

        doc_id = entry["document_chembl_id"]
        title = entry.get("title", "")[:60]

        data = self.runner.lookup(doc_id)
        if not data or not data.get("results"):
            return False, f"No results for {doc_id}"

        return True, f"{doc_id} is PUBLICATION: {title}..."

    @test
    def test_document_with_journal(self):
        """Check document has journal information"""
        entry = next(
            (e for e in self.runner.reference_data.get("documents", [])
             if e.get("journal") and e["journal"] != ""),
            None
        )
        if not entry:
            return False, "No document with journal in reference"

        doc_id = entry["document_chembl_id"]
        journal = entry.get("journal", "unknown")

        data = self.runner.lookup(doc_id)
        if not data or not data.get("results"):
            return False, f"No results for {doc_id}"

        return True, f"{doc_id} published in: {journal}"

    @test
    def test_document_with_pubmed(self):
        """Check document has PubMed ID"""
        entry = next(
            (e for e in self.runner.reference_data.get("documents", [])
             if e.get("pubmed_id") and e["pubmed_id"] is not None),
            None
        )
        if not entry:
            return False, "No document with PubMed ID in reference"

        doc_id = entry["document_chembl_id"]
        pmid = entry["pubmed_id"]

        data = self.runner.lookup(doc_id)
        if not data or not data.get("results"):
            return False, f"No results for {doc_id}"

        return True, f"{doc_id} has PubMed ID: {pmid}"

    # =========================================================================
    # CELL LINE TESTS
    # =========================================================================

    @test
    def test_cell_line_with_name(self):
        """Check cell line has name"""
        entry = next(
            (e for e in self.runner.reference_data.get("cell_lines", [])
             if e.get("cell_name")),
            None
        )
        if not entry:
            return False, "No cell line with name in reference"

        cell_id = entry["cell_chembl_id"]
        name = entry["cell_name"]

        data = self.runner.lookup(cell_id)
        if not data or not data.get("results"):
            return False, f"No results for {cell_id}"

        return True, f"{cell_id} has name: {name}"

    @test
    def test_cell_line_with_organism(self):
        """Check cell line has source organism"""
        entry = next(
            (e for e in self.runner.reference_data.get("cell_lines", [])
             if e.get("cell_source_organism")),
            None
        )
        if not entry:
            return False, "No cell line with organism in reference"

        cell_id = entry["cell_chembl_id"]
        organism = entry["cell_source_organism"]

        data = self.runner.lookup(cell_id)
        if not data or not data.get("results"):
            return False, f"No results for {cell_id}"

        return True, f"{cell_id} from organism: {organism}"

    @test
    def test_cell_line_with_taxonomy(self):
        """Check cell line has taxonomy ID"""
        entry = next(
            (e for e in self.runner.reference_data.get("cell_lines", [])
             if e.get("cell_source_tax_id")),
            None
        )
        if not entry:
            return False, "No cell line with taxonomy in reference"

        cell_id = entry["cell_chembl_id"]
        tax_id = entry["cell_source_tax_id"]

        data = self.runner.lookup(cell_id)
        if not data or not data.get("results"):
            return False, f"No results for {cell_id}"

        return True, f"{cell_id} has taxonomy: {tax_id}"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        print("Run: python3 extract_reference_data.py")
        return 1

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add all custom tests
    custom_tests = ChEMBLTests(runner)
    test_methods = [
        # Target tests
        custom_tests.test_target_with_name,
        custom_tests.test_target_with_type,
        custom_tests.test_target_with_taxonomy,
        custom_tests.test_target_uniprot_xref,
        custom_tests.test_single_protein_target,
        # Molecule tests
        custom_tests.test_molecule_with_name,
        custom_tests.test_molecule_with_type,
        custom_tests.test_molecule_with_synonyms,
        custom_tests.test_small_molecule_type,
        # Activity tests
        custom_tests.test_activity_with_assay,
        custom_tests.test_activity_with_molecule,
        custom_tests.test_activity_with_target,
        custom_tests.test_activity_with_measurement,
        # Assay tests
        custom_tests.test_assay_with_description,
        custom_tests.test_assay_with_type,
        custom_tests.test_assay_with_target,
        # Document tests
        custom_tests.test_publication_document,
        custom_tests.test_document_with_journal,
        custom_tests.test_document_with_pubmed,
        # Cell line tests
        custom_tests.test_cell_line_with_name,
        custom_tests.test_cell_line_with_organism,
        custom_tests.test_cell_line_with_taxonomy,
    ]

    for test_method in test_methods:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
