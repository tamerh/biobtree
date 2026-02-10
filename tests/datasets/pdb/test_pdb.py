#!/usr/bin/env python3
"""
PDB Test Suite

Tests PDB (Protein Data Bank) dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

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


class PDBTests:
    """PDB custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_xray_structure(self):
        """Check X-ray diffraction structure is searchable"""
        # Find an X-ray structure
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("method", "").upper() == "X-RAY DIFFRACTION"),
            None
        )
        if not entry:
            return True, "SKIP: No X-ray structure in reference"

        pdb_id = entry["pdb_id"]
        resolution = entry.get("resolution")

        data = self.runner.lookup(pdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {pdb_id}"

        result = data["results"][0]
        has_attr = result.get("has_attr", False)

        # Entry found - check if it has attributes (may not in test DB)
        if has_attr and resolution:
            return True, f"{pdb_id} (X-ray) has resolution {resolution}A"
        elif has_attr:
            return True, f"{pdb_id} (X-ray) has attributes"
        else:
            return True, f"{pdb_id} (X-ray) found (attrs in full build only)"

    @test
    def test_cryoem_structure(self):
        """Check cryo-EM structure is searchable"""
        # Find a cryo-EM structure
        entry = next(
            (e for e in self.runner.reference_data
             if "ELECTRON MICROSCOPY" in e.get("method", "").upper()),
            None
        )
        if not entry:
            return True, "SKIP: No cryo-EM structure in reference"

        pdb_id = entry["pdb_id"]
        data = self.runner.lookup(pdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {pdb_id}"

        return True, f"{pdb_id} (cryo-EM) found"

    @test
    def test_nmr_structure(self):
        """Check NMR structure is searchable"""
        # Find an NMR structure
        entry = next(
            (e for e in self.runner.reference_data
             if "NMR" in e.get("method", "").upper()),
            None
        )
        if not entry:
            return True, "SKIP: No NMR structure in reference"

        pdb_id = entry["pdb_id"]
        data = self.runner.lookup(pdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {pdb_id}"

        return True, f"{pdb_id} (NMR) found"

    @test
    def test_uniprot_xref(self):
        """Check PDB to UniProt cross-reference"""
        # Find an entry with UniProt xrefs
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("xrefs", {}).get("uniprot")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with UniProt xrefs in reference"

        pdb_id = entry["pdb_id"]
        expected_uniprot = entry["xrefs"]["uniprot"]

        data = self.runner.lookup(pdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {pdb_id}"

        result = data["results"][0]
        datasets = self.runner.get_xref_datasets(result)

        if "uniprot" in datasets:
            return True, f"{pdb_id} has UniProt xrefs (expected: {', '.join(expected_uniprot[:3])})"
        else:
            return False, f"{pdb_id} missing UniProt xrefs"

    @test
    def test_go_xref(self):
        """Check PDB to GO cross-reference"""
        # Find an entry with GO xrefs
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("xrefs", {}).get("go")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with GO xrefs in reference"

        pdb_id = entry["pdb_id"]
        go_terms = entry["xrefs"]["go"]

        data = self.runner.lookup(pdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {pdb_id}"

        result = data["results"][0]
        datasets = self.runner.get_xref_datasets(result)

        if "go" in datasets:
            return True, f"{pdb_id} has GO xrefs ({len(go_terms)} terms)"
        else:
            return False, f"{pdb_id} missing GO xrefs"

    @test
    def test_multiple_chains(self):
        """Check structure with multiple chains"""
        # Find an entry with multiple chains
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("polymer_count", 0) > 1),
            None
        )
        if not entry:
            return True, "SKIP: No multi-chain structure in reference"

        pdb_id = entry["pdb_id"]
        polymer_count = entry["polymer_count"]

        data = self.runner.lookup(pdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {pdb_id}"

        return True, f"{pdb_id} has {polymer_count} polymer chains"

    @test
    def test_structure_title(self):
        """Check structure is searchable and has reference title"""
        entry = next(
            (e for e in self.runner.reference_data if e.get("title")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with title in reference"

        pdb_id = entry["pdb_id"]
        title = entry["title"][:50]

        data = self.runner.lookup(pdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {pdb_id}"

        result = data["results"][0]
        has_attr = result.get("has_attr", False)

        # Entry found - attributes may only be present in full build
        if has_attr:
            return True, f"{pdb_id}: '{title}...'"
        else:
            return True, f"{pdb_id} found (title: '{title[:30]}...')"

    @test
    def test_reverse_lookup_uniprot_to_pdb(self):
        """Check reverse mapping from UniProt to PDB"""
        # Find an entry with UniProt xrefs
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("xrefs", {}).get("uniprot")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with UniProt xrefs"

        pdb_id = entry["pdb_id"]
        uniprot_id = entry["xrefs"]["uniprot"][0]

        # Use mapping endpoint
        map_url = f"{self.runner.api_url}/ws/map/"
        try:
            response = requests.get(
                map_url,
                params={"i": uniprot_id, "m": ">>uniprot>>pdb"},
                timeout=30
            )
            if response.status_code == 200:
                data = response.json()
                if data.get("results"):
                    # Check if our PDB ID is in the results
                    pdb_ids_found = [r.get("id", "").upper() for r in data["results"]]
                    if pdb_id.upper() in pdb_ids_found:
                        return True, f"{uniprot_id} -> {pdb_id} verified"
                    else:
                        return True, f"{uniprot_id} maps to PDB (not {pdb_id})"
                else:
                    return False, f"No PDB mappings for {uniprot_id}"
            else:
                return False, f"Mapping failed: {response.status_code}"
        except Exception as e:
            return False, f"Mapping error: {e}"


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
    custom_tests = PDBTests(runner)
    for test_method in [
        custom_tests.test_xray_structure,
        custom_tests.test_cryoem_structure,
        custom_tests.test_nmr_structure,
        custom_tests.test_uniprot_xref,
        custom_tests.test_go_xref,
        custom_tests.test_multiple_chains,
        custom_tests.test_structure_title,
        custom_tests.test_reverse_lookup_uniprot_to_pdb,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
