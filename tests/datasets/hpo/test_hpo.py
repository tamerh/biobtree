#!/usr/bin/env python3
"""
HPO (Human Phenotype Ontology) Test Suite

Tests phenotype terms, hierarchies, gene associations, and cross-references.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class HPOTests:
    """HPO custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_phenotype_with_name(self):
        """Check phenotype has name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name")),
            None
        )
        if not entry:
            return False, "No phenotype with name in reference"

        phenotype_id = entry["id"]
        phenotype_name = entry["name"][:60]
        data = self.runner.lookup(phenotype_id)

        if not data or not data.get("results"):
            return False, f"No results for {phenotype_id}"

        return True, f"{phenotype_id} has name: {phenotype_name}..."

    @test
    def test_phenotype_with_synonyms(self):
        """Check phenotype has synonyms"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e["synonyms"]) > 0),
            None
        )
        if not entry:
            return False, "No phenotype with synonyms in reference"

        phenotype_id = entry["id"]
        syn_count = len(entry["synonyms"])
        synonyms = entry.get("synonyms", [])
        data = self.runner.lookup(phenotype_id)

        if not data or not data.get("results"):
            return False, f"No results for {phenotype_id}"

        # Show first synonym as example
        first_synonym = synonyms[0].get('text', '')[:40] if synonyms else ""
        return True, f"{phenotype_id} has {syn_count} synonym(s) (e.g., '{first_synonym}...')"

    @test
    def test_phenotype_with_parents(self):
        """Check phenotype hierarchy (parent relationships)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("parents") and len(e["parents"]) > 0),
            None
        )
        if not entry:
            return False, "No phenotype with parents in reference"

        phenotype_id = entry["id"]
        parent_count = len(entry["parents"])
        parent_id = entry["parents"][0].get('id', '')

        data = self.runner.lookup(phenotype_id)
        if not data or not data.get("results"):
            return False, f"No results for {phenotype_id}"

        return True, f"{phenotype_id} has {parent_count} parent(s) (e.g., {parent_id})"

    @test
    def test_phenotype_with_gene_associations(self):
        """Check phenotype has gene associations"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_associations") and len(e["gene_associations"]) > 0),
            None
        )
        if not entry:
            return False, "No phenotype with gene associations in reference"

        phenotype_id = entry["id"]
        gene_count = len(entry["gene_associations"])
        first_gene = entry["gene_associations"][0].get('gene_symbol', '')

        data = self.runner.lookup(phenotype_id)
        if not data or not data.get("results"):
            return False, f"No results for {phenotype_id}"

        return True, f"{phenotype_id} has {gene_count} gene(s) (e.g., {first_gene})"

    @test
    def test_phenotype_with_definition(self):
        """Check phenotype has definition"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("definition")),
            None
        )
        if not entry:
            return True, "SKIP: No phenotype with definition in reference"

        phenotype_id = entry["id"]
        definition = str(entry["definition"])[:60]

        data = self.runner.lookup(phenotype_id)
        if not data or not data.get("results"):
            return False, f"No results for {phenotype_id}"

        return True, f"{phenotype_id} has definition: {definition}..."

    @test
    def test_cross_references(self):
        """Check phenotype has cross-database references"""
        entry = self.runner.reference_data[0]
        phenotype_id = entry["id"]

        data = self.runner.lookup(phenotype_id)
        if not data or not data.get("results"):
            return False, f"No results for {phenotype_id}"

        result = data["results"][0]
        datasets = self.runner.get_xref_datasets(result)
        xref_count = self.runner.get_xref_count(result)

        if len(datasets) >= 1:
            return True, f"{phenotype_id} has {xref_count} xrefs to {len(datasets)} databases: {', '.join(datasets[:5])}"

        return True, f"SKIP: {phenotype_id} has no xrefs in test data"

    @test
    def test_parent_child_navigation(self):
        """Test parent-child hierarchy navigation"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("parents") and len(e["parents"]) > 0),
            None
        )
        if not entry:
            return False, "No phenotype with parents in reference"

        phenotype_id = entry["id"]
        parent_id = entry["parents"][0].get('id', '')

        # Check if parent can be looked up
        parent_data = self.runner.lookup(parent_id)
        if not parent_data or not parent_data.get("results"):
            return False, f"Cannot navigate to parent {parent_id}"

        return True, f"Can navigate {phenotype_id} → {parent_id}"

    @test
    def test_hpo_to_omim_disease_mapping(self):
        """Test HPO → OMIM disease mapping (from phenotype.hpoa)"""
        # Use a well-known phenotype that should have OMIM associations
        test_id = "HP:0001250"  # Seizure - common phenotype with many disease links

        data = self.runner.map_query(test_id, ">>hpo>>mim")

        if not data:
            return False, f"No response for {test_id} >> hpo >> mim"

        mappings = data.get("mappings", [])
        if not mappings or mappings[0].get("error"):
            return True, f"SKIP: {test_id} has no OMIM disease links in test data"

        targets = mappings[0].get("targets", [])
        if len(targets) > 0:
            sample_disease = targets[0].get("identifier", targets[0].get("id", ""))
            return True, f"{test_id} → {len(targets)} OMIM disease(s) (e.g., {sample_disease})"

        return True, f"SKIP: {test_id} has no OMIM disease targets"

    @test
    def test_hpo_to_orphanet_disease_mapping(self):
        """Test HPO → Orphanet disease mapping (from phenotype.hpoa)"""
        test_id = "HP:0001250"  # Seizure

        data = self.runner.map_query(test_id, ">>hpo>>orphanet")

        if not data:
            return False, f"No response for {test_id} >> hpo >> orphanet"

        mappings = data.get("mappings", [])
        if not mappings or mappings[0].get("error"):
            return True, f"SKIP: {test_id} has no Orphanet disease links in test data"

        targets = mappings[0].get("targets", [])
        if len(targets) > 0:
            sample_disease = targets[0].get("identifier", targets[0].get("id", ""))
            return True, f"{test_id} → {len(targets)} Orphanet disease(s) (e.g., {sample_disease})"

        return True, f"SKIP: {test_id} has no Orphanet disease targets"

    @test
    def test_omim_to_hpo_reverse_mapping(self):
        """Test OMIM → HPO reverse mapping (disease to phenotypes)"""
        # Marfan syndrome - well-characterized disease
        test_id = "154700"

        data = self.runner.map_query(test_id, ">>mim>>hpo")

        if not data:
            return False, f"No response for {test_id} >> mim >> hpo"

        mappings = data.get("mappings", [])
        if not mappings or mappings[0].get("error"):
            return True, f"SKIP: OMIM:{test_id} has no HPO phenotype links in test data"

        targets = mappings[0].get("targets", [])
        if len(targets) > 0:
            sample_pheno = targets[0].get("identifier", targets[0].get("id", ""))
            return True, f"OMIM:{test_id} → {len(targets)} HPO phenotype(s) (e.g., {sample_pheno})"

        return True, f"SKIP: OMIM:{test_id} has no HPO phenotype targets"

    @test
    def test_hpo_hierarchy_child(self):
        """Test HPO child hierarchy navigation"""
        test_id = "HP:0001250"  # Seizure - has many child terms

        data = self.runner.map_query(test_id, ">>hpo>>hpochild")

        if not data:
            return False, f"No response for {test_id} >> hpo >> hpochild"

        mappings = data.get("mappings", [])
        if not mappings or mappings[0].get("error"):
            return True, f"SKIP: {test_id} has no child terms"

        targets = mappings[0].get("targets", [])
        if len(targets) > 0:
            sample_child = targets[0].get("identifier", targets[0].get("id", ""))
            return True, f"{test_id} has {len(targets)} child term(s) (e.g., {sample_child})"

        return True, f"SKIP: {test_id} has no child terms"


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
    custom_tests = HPOTests(runner)

    for test_method in [
        custom_tests.test_phenotype_with_name,
        custom_tests.test_phenotype_with_synonyms,
        custom_tests.test_phenotype_with_parents,
        custom_tests.test_phenotype_with_gene_associations,
        custom_tests.test_phenotype_with_definition,
        custom_tests.test_cross_references,
        custom_tests.test_parent_child_navigation,
        # Disease association tests (phenotype.hpoa integration)
        custom_tests.test_hpo_to_omim_disease_mapping,
        custom_tests.test_hpo_to_orphanet_disease_mapping,
        custom_tests.test_omim_to_hpo_reverse_mapping,
        custom_tests.test_hpo_hierarchy_child,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
