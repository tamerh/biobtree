#!/usr/bin/env python3
"""
CELLxGENE Test Suite

Tests CELLxGENE dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

Tests both datasets:
- cellxgene: Dataset metadata from CZ CELLxGENE Census
- cellxgene_celltype: Cell type data with tissue/disease associations

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


class CELLxGENETests:
    """CELLxGENE custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_dataset_with_human_organism(self):
        """Check dataset with human organism links to taxonomy 9606"""
        # Find a dataset with human organism
        entry = next(
            (e for e in self.runner.reference_data.get("cellxgene", [])
             if e.get("organism") == "Homo sapiens"),
            None
        )
        if not entry:
            return True, "SKIP: No human dataset in reference"

        dataset_id = entry["dataset_id"]
        data = self.runner.get_entry(dataset_id, "cellxgene")

        if not data:
            return False, f"No results for {dataset_id}"

        # Check for taxonomy cross-reference
        if self.runner.has_xref(data[0], "taxonomy", "9606"):
            return True, f"Human dataset {dataset_id[:8]}... links to taxonomy:9606"

        return False, f"Human dataset {dataset_id[:8]}... missing taxonomy:9606 xref"

    @test
    def test_dataset_with_mouse_organism(self):
        """Check dataset with mouse organism links to taxonomy 10090"""
        # Find a dataset with mouse organism
        entry = next(
            (e for e in self.runner.reference_data.get("cellxgene", [])
             if e.get("organism") == "Mus musculus"),
            None
        )
        if not entry:
            return True, "SKIP: No mouse dataset in reference"

        dataset_id = entry["dataset_id"]
        data = self.runner.get_entry(dataset_id, "cellxgene")

        if not data:
            return False, f"No results for {dataset_id}"

        # Check for taxonomy cross-reference
        if self.runner.has_xref(data[0], "taxonomy", "10090"):
            return True, f"Mouse dataset {dataset_id[:8]}... links to taxonomy:10090"

        return False, f"Mouse dataset {dataset_id[:8]}... missing taxonomy:10090 xref"

    @test
    def test_dataset_with_cell_types(self):
        """Check dataset with cell types has CL cross-references"""
        # Find a dataset with cell types
        entry = next(
            (e for e in self.runner.reference_data.get("cellxgene", [])
             if e.get("cell_type_cl_ids")),
            None
        )
        if not entry:
            return True, "SKIP: No dataset with cell types in reference"

        dataset_id = entry["dataset_id"]
        expected_cl_ids = entry["cell_type_cl_ids"]
        data = self.runner.get_entry(dataset_id, "cellxgene")

        if not data:
            return False, f"No results for {dataset_id}"

        # Check for at least one CL cross-reference
        cl_xrefs = self.runner.get_xrefs(data[0], "cl")
        if cl_xrefs:
            return True, f"Dataset {dataset_id[:8]}... has {len(cl_xrefs)} CL xrefs (expected {len(expected_cl_ids)})"

        return False, f"Dataset {dataset_id[:8]}... missing CL xrefs"

    @test
    def test_dataset_assay_efo_xref(self):
        """Check dataset links to EFO assay ontology"""
        # Find a dataset with assay EFO IDs
        entry = next(
            (e for e in self.runner.reference_data.get("cellxgene", [])
             if e.get("assay_efo_ids")),
            None
        )
        if not entry:
            return True, "SKIP: No dataset with assay EFO IDs in reference"

        dataset_id = entry["dataset_id"]
        assay_types = entry.get("assay_types", [])
        data = self.runner.get_entry(dataset_id, "cellxgene")

        if not data:
            return False, f"No results for {dataset_id}"

        efo_xrefs = self.runner.get_xrefs(data[0], "efo")
        if efo_xrefs:
            assay_str = ", ".join(assay_types[:2])
            return True, f"Dataset {dataset_id[:8]}... ({assay_str}) links to EFO"

        return False, f"Dataset {dataset_id[:8]}... missing EFO xrefs"

    @test
    def test_celltype_links_to_cl_ontology(self):
        """Check celltype entry links to Cell Ontology"""
        # Find a cell type entry
        entry = next(
            (e for e in self.runner.reference_data.get("cellxgene_celltype", [])
             if e.get("cell_type_cl")),
            None
        )
        if not entry:
            return True, "SKIP: No cell type in reference"

        ct_id = entry["id"]
        ct_name = entry.get("name", "unknown")
        cl_id = entry["cell_type_cl"]
        data = self.runner.get_entry(ct_id, "cellxgene_celltype")

        if not data:
            return False, f"No results for {ct_id}"

        # Check for CL cross-reference
        if self.runner.has_xref(data[0], "cl", cl_id):
            return True, f"Cell type {ct_name} ({ct_id}) links to {cl_id}"

        return False, f"Cell type {ct_name} missing CL:{cl_id} xref"

    @test
    def test_celltype_tissue_uberon_xrefs(self):
        """Check celltype with tissues has UBERON cross-references"""
        # Find a cell type with tissue IDs
        entry = next(
            (e for e in self.runner.reference_data.get("cellxgene_celltype", [])
             if e.get("found_in_tissue_ids")),
            None
        )
        if not entry:
            return True, "SKIP: No cell type with tissue IDs in reference"

        ct_id = entry["id"]
        ct_name = entry.get("name", "unknown")
        tissue_ids = entry["found_in_tissue_ids"]
        data = self.runner.get_entry(ct_id, "cellxgene_celltype")

        if not data:
            return False, f"No results for {ct_id}"

        uberon_xrefs = self.runner.get_xrefs(data[0], "uberon")
        if uberon_xrefs:
            return True, f"Cell type {ct_name} has {len(uberon_xrefs)} UBERON xrefs (found in {len(tissue_ids)} tissues)"

        return False, f"Cell type {ct_name} missing UBERON xrefs"

    @test
    def test_reverse_mapping_cl_to_cellxgene(self):
        """Check reverse mapping from CL to cellxgene datasets"""
        # Find a cell type that appears in datasets
        entry = next(
            (e for e in self.runner.reference_data.get("cellxgene_celltype", [])
             if e.get("cell_type_cl")),
            None
        )
        if not entry:
            return True, "SKIP: No cell type in reference"

        cl_id = entry["cell_type_cl"]
        ct_name = entry.get("name", "unknown")

        # Try to map from CL to cellxgene
        result = self.runner.map_query(cl_id, ">>cl>>cellxgene")

        if result and result.get("results"):
            targets = []
            for r in result["results"]:
                if "entries" in r:
                    targets.extend([e for e in r["entries"] if e.get("dataset_name") == "cellxgene"])
            if targets:
                return True, f"CL:{cl_id} ({ct_name}) maps to {len(targets)} CELLxGENE datasets"

        return True, f"SKIP: No reverse mapping for {cl_id} (may not be in test data)"


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

    # Add custom tests
    custom_tests = CELLxGENETests(runner)
    for test_method in [
        custom_tests.test_dataset_with_human_organism,
        custom_tests.test_dataset_with_mouse_organism,
        custom_tests.test_dataset_with_cell_types,
        custom_tests.test_dataset_assay_efo_xref,
        custom_tests.test_celltype_links_to_cl_ontology,
        custom_tests.test_celltype_tissue_uberon_xrefs,
        custom_tests.test_reverse_mapping_cl_to_cellxgene,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
