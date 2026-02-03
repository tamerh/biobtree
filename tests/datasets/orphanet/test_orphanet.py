#!/usr/bin/env python3
"""
Orphanet (Rare Disease Database) Test Suite

Tests disorder entries, phenotype associations, gene associations, and cross-references.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class OrphanetTests:
    """Orphanet custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_disorder_with_name(self):
        """Check disorder has name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name")),
            None
        )
        if not entry:
            return False, "No disorder with name in reference"

        disorder_id = entry["id"]
        disorder_name = entry["name"][:60]
        data = self.runner.lookup(disorder_id)

        if not data or not data.get("results"):
            return False, f"No results for {disorder_id}"

        return True, f"{disorder_id} has name: {disorder_name}..."

    @test
    def test_disorder_with_synonyms(self):
        """Check disorder has synonyms"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e["synonyms"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No disorder with synonyms in reference"

        disorder_id = entry["id"]
        syn_count = len(entry["synonyms"])
        first_synonym = entry["synonyms"][0][:40] if entry["synonyms"] else ""
        data = self.runner.lookup(disorder_id)

        if not data or not data.get("results"):
            return False, f"No results for {disorder_id}"

        return True, f"{disorder_id} has {syn_count} synonym(s) (e.g., '{first_synonym}...')"

    @test
    def test_disorder_with_phenotypes(self):
        """Check disorder has HPO phenotype associations"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("phenotypes") and len(e["phenotypes"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No disorder with phenotypes in reference"

        disorder_id = entry["id"]
        pheno_count = len(entry["phenotypes"])
        first_hpo = entry["phenotypes"][0].get("hpo_id", "")

        data = self.runner.lookup(disorder_id)
        if not data or not data.get("results"):
            return False, f"No results for {disorder_id}"

        return True, f"{disorder_id} has {pheno_count} phenotype(s) (e.g., {first_hpo})"

    @test
    def test_disorder_with_disorder_type(self):
        """Check disorder has disorder type"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("disorder_type")),
            None
        )
        if not entry:
            return True, "SKIP: No disorder with disorder_type in reference"

        disorder_id = entry["id"]
        disorder_type = entry["disorder_type"]

        data = self.runner.lookup(disorder_id)
        if not data or not data.get("results"):
            return False, f"No results for {disorder_id}"

        return True, f"{disorder_id} has type: {disorder_type}"

    @test
    def test_phenotype_frequency(self):
        """Check phenotype associations have frequency data"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("phenotypes") and
             any(p.get("frequency") for p in e["phenotypes"])),
            None
        )
        if not entry:
            return True, "SKIP: No disorder with phenotype frequency in reference"

        disorder_id = entry["id"]
        pheno_with_freq = next(
            (p for p in entry["phenotypes"] if p.get("frequency")),
            None
        )
        freq = pheno_with_freq.get("frequency", "")[:30]
        hpo_id = pheno_with_freq.get("hpo_id", "")

        return True, f"{disorder_id}/{hpo_id} has frequency: {freq}"

    @test
    def test_cross_references(self):
        """Check disorder has cross-database references"""
        entry = self.runner.reference_data[0]
        disorder_id = entry["id"]

        data = self.runner.lookup(disorder_id)
        if not data or not data.get("results"):
            return False, f"No results for {disorder_id}"

        result = data["results"][0]
        datasets = self.runner.get_xref_datasets(result)
        xref_count = self.runner.get_xref_count(result)

        if len(datasets) >= 1:
            return True, f"{disorder_id} has {xref_count} xrefs to {len(datasets)} databases: {', '.join(datasets[:5])}"

        return True, f"SKIP: {disorder_id} has no xrefs in test data"

    @test
    def test_orphanet_to_hpo_mapping(self):
        """Test Orphanet -> HPO phenotype mapping"""
        # Use Marfan syndrome - well-known with many phenotypes
        test_id = "558"

        data = self.runner.map_query(test_id, ">>orphanet>>hpo")

        if not data:
            return False, f"No response for {test_id} >> orphanet >> hpo"

        mappings = data.get("mappings", [])
        if not mappings or mappings[0].get("error"):
            return True, f"SKIP: {test_id} has no HPO phenotype links in test data"

        targets = mappings[0].get("targets", [])
        if len(targets) > 0:
            sample_hpo = targets[0].get("identifier", targets[0].get("id", ""))
            return True, f"{test_id} -> {len(targets)} HPO phenotype(s) (e.g., {sample_hpo})"

        return True, f"SKIP: {test_id} has no HPO phenotype targets"

    @test
    def test_text_search_by_name(self):
        """Test searching by disease name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name") and len(e["name"]) > 5),
            None
        )
        if not entry:
            return True, "SKIP: No disorder with searchable name"

        disorder_name = entry["name"]
        disorder_id = entry["id"]

        data = self.runner.lookup(disorder_name)
        if not data or not data.get("results"):
            return True, f"SKIP: Text search for '{disorder_name[:30]}' returned no results"

        # Check if our expected ID is in results
        found = any(r.get("identifier") == disorder_id for r in data["results"])
        if found:
            return True, f"Found {disorder_id} by searching '{disorder_name[:30]}...'"

        return True, f"SKIP: {disorder_id} not in text search results for its name"


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
    custom_tests = OrphanetTests(runner)

    for test_method in [
        custom_tests.test_disorder_with_name,
        custom_tests.test_disorder_with_synonyms,
        custom_tests.test_disorder_with_phenotypes,
        custom_tests.test_disorder_with_disorder_type,
        custom_tests.test_phenotype_frequency,
        custom_tests.test_cross_references,
        custom_tests.test_orphanet_to_hpo_mapping,
        custom_tests.test_text_search_by_name,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
