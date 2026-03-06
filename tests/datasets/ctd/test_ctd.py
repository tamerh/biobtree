#!/usr/bin/env python3
"""
CTD (Comparative Toxicogenomics Database) Test Suite

Tests chemical-gene interactions, chemical-disease associations,
and cross-references to MeSH, Entrez, MONDO, EFO, OMIM, and PubMed.
"""

import sys
import os
import requests
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class CtdTests:
    """CTD custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _test_mapping(self, source_id: str, target_dataset: str) -> tuple:
        """Helper to test a mapping from CTD to another dataset"""
        url = f"{self.runner.api_url}/ws/map/?i={source_id}&m=>>ctd>>{target_dataset}"
        try:
            resp = requests.get(url, timeout=30)
            if resp.status_code != 200:
                return False, f"HTTP {resp.status_code}"
            data = resp.json()
            if data.get("results") and len(data["results"]) > 0:
                result = data["results"][0]
                targets = result.get("targets", [])
                if targets:
                    return True, f"Found {len(targets)} {target_dataset} mappings"
            return False, f"No {target_dataset} mappings found"
        except Exception as e:
            return False, str(e)

    @test
    def test_entry_with_chemical_name(self):
        """Check entry has chemical name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("chemical_name")),
            None
        )
        if not entry:
            return False, "No entry with chemical_name in reference"

        chemical_id = entry["chemical_id"]
        chemical_name = entry["chemical_name"]
        data = self.runner.lookup(chemical_id)

        if not data or not data.get("results"):
            return False, f"No results for {chemical_id}"

        return True, f"{chemical_id} has name: {chemical_name[:50]}..."

    @test
    def test_entry_with_gene_interactions(self):
        """Check entry has gene interactions"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_interactions") and len(e["gene_interactions"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with gene_interactions in reference"

        chemical_id = entry["chemical_id"]
        count = len(entry["gene_interactions"])
        data = self.runner.lookup(chemical_id)

        if not data or not data.get("results"):
            return False, f"No results for {chemical_id}"

        return True, f"Entry has {count} gene interactions"

    @test
    def test_entry_with_disease_associations(self):
        """Check entry has disease associations"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("disease_associations") and len(e["disease_associations"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with disease_associations in reference"

        chemical_id = entry["chemical_id"]
        count = len(entry["disease_associations"])
        data = self.runner.lookup(chemical_id)

        if not data or not data.get("results"):
            return False, f"No results for {chemical_id}"

        return True, f"Entry has {count} disease associations"

    @test
    def test_text_search_by_chemical_name(self):
        """Test text search by chemical name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("chemical_name") and len(e["chemical_name"]) >= 5),
            None
        )
        if not entry:
            return False, "No entry with suitable chemical_name in reference"

        chemical_name = entry["chemical_name"]

        # Search by chemical name
        data = self.runner.lookup(chemical_name)
        if not data or not data.get("results"):
            return False, f"No results for chemical: {chemical_name}"

        return True, f"Found results for '{chemical_name[:40]}'"

    @test
    def test_entry_with_synonyms(self):
        """Check entry has synonyms"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e["synonyms"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with synonyms in reference"

        chemical_id = entry["chemical_id"]
        synonyms = entry["synonyms"]
        data = self.runner.lookup(chemical_id)

        if not data or not data.get("results"):
            return False, f"No results for {chemical_id}"

        return True, f"Entry has synonyms: {synonyms[0][:40]}..."

    @test
    def test_entry_with_mesh_tree(self):
        """Check entry has MeSH tree numbers"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("mesh_tree_numbers") and len(e["mesh_tree_numbers"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with mesh_tree_numbers in reference"

        chemical_id = entry["chemical_id"]
        tree_numbers = entry["mesh_tree_numbers"]
        data = self.runner.lookup(chemical_id)

        if not data or not data.get("results"):
            return False, f"No results for {chemical_id}"

        return True, f"Entry has MeSH tree: {tree_numbers[0]}"

    @test
    def test_mapping_ctd_to_mesh(self):
        """Test CTD → MeSH mapping (chemical vocabulary)"""
        # Find an entry with a valid chemical_id (MeSH format)
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("chemical_id")),
            None
        )
        if not entry:
            return False, "No entry with chemical_id in reference"

        chemical_id = entry["chemical_id"]
        return self._test_mapping(chemical_id, "mesh")

    @test
    def test_mapping_ctd_to_entrez(self):
        """Test CTD → Entrez Gene mapping (gene interactions)"""
        # Find an entry with gene interactions
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_interactions") and len(e["gene_interactions"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with gene_interactions in reference"

        chemical_id = entry["chemical_id"]
        return self._test_mapping(chemical_id, "entrez")

    @test
    def test_mapping_ctd_to_mondo(self):
        """Test CTD → MONDO mapping (via OMIM disease IDs)"""
        # Find an entry with disease associations that have OMIM IDs
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("disease_associations") and
             any(da.get("omim_ids") for da in e["disease_associations"])),
            None
        )
        if not entry:
            return False, "No entry with OMIM-linked diseases in reference"

        chemical_id = entry["chemical_id"]
        return self._test_mapping(chemical_id, "mondo")

    @test
    def test_mapping_ctd_to_efo(self):
        """Test CTD → EFO mapping (via MeSH disease IDs)"""
        # Find an entry with disease associations
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("disease_associations") and len(e["disease_associations"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with disease_associations in reference"

        chemical_id = entry["chemical_id"]
        return self._test_mapping(chemical_id, "efo")

    @test
    def test_mapping_ctd_to_taxonomy(self):
        """Test CTD → Taxonomy mapping (organism context)"""
        # Find an entry with gene interactions that have organism IDs
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_interactions") and
             any(gi.get("organism_id") for gi in e["gene_interactions"])),
            None
        )
        if not entry:
            return False, "No entry with organism-linked interactions in reference"

        chemical_id = entry["chemical_id"]
        return self._test_mapping(chemical_id, "taxonomy")

    @test
    def test_mapping_ctd_to_pubchem(self):
        """Test CTD → PubChem mapping (compound structure)"""
        # Find an entry with PubChem CID
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("pubchem_cid")),
            None
        )
        if not entry:
            return False, "No entry with pubchem_cid in reference"

        chemical_id = entry["chemical_id"]
        return self._test_mapping(chemical_id, "pubchem")


def main():
    """Main test execution"""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        print("Run: python3 extract_reference_data.py")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom_tests = CtdTests(runner)

    for test_method in [
        # Attribute tests
        custom_tests.test_entry_with_chemical_name,
        custom_tests.test_entry_with_gene_interactions,
        custom_tests.test_entry_with_disease_associations,
        custom_tests.test_text_search_by_chemical_name,
        custom_tests.test_entry_with_synonyms,
        custom_tests.test_entry_with_mesh_tree,
        # Cross-reference mapping tests
        custom_tests.test_mapping_ctd_to_mesh,
        custom_tests.test_mapping_ctd_to_entrez,
        custom_tests.test_mapping_ctd_to_mondo,
        custom_tests.test_mapping_ctd_to_efo,
        custom_tests.test_mapping_ctd_to_taxonomy,
        custom_tests.test_mapping_ctd_to_pubchem,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
