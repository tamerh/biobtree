#!/usr/bin/env python3
"""
MeSH (Medical Subject Headings) Test Suite

Tests descriptor lookups, text search, synonyms, and pharmacological actions.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class MeshTests:
    """MeSH custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_descriptor_with_name(self):
        """Check descriptor has name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name")),
            None
        )
        if not entry:
            return False, "No descriptor with name in reference"

        mesh_id = entry["id"]
        mesh_name = entry["name"][:60]
        data = self.runner.lookup(mesh_id)

        if not data or not data.get("results"):
            return False, f"No results for {mesh_id}"

        return True, f"{mesh_id} has name: {mesh_name}..."

    @test
    def test_descriptor_with_entry_terms(self):
        """Check descriptor has entry terms (synonyms)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("entry_terms") and len(e["entry_terms"]) > 0),
            None
        )
        if not entry:
            return False, "No descriptor with entry terms in reference"

        mesh_id = entry["id"]
        term_count = len(entry["entry_terms"])
        entry_terms = entry.get("entry_terms", [])
        data = self.runner.lookup(mesh_id)

        if not data or not data.get("results"):
            return False, f"No results for {mesh_id}"

        # Show first entry term as example
        first_term = entry_terms[0][:40] if entry_terms else ""
        return True, f"{mesh_id} has {term_count} entry term(s) (e.g., '{first_term}...')"

    @test
    def test_descriptor_with_tree_numbers(self):
        """Check descriptor has tree numbers (hierarchical classification)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("tree_numbers") and len(e["tree_numbers"]) > 0),
            None
        )
        if not entry:
            return False, "No descriptor with tree numbers in reference"

        mesh_id = entry["id"]
        tree_count = len(entry["tree_numbers"])
        first_tree = entry["tree_numbers"][0]

        data = self.runner.lookup(mesh_id)
        if not data or not data.get("results"):
            return False, f"No results for {mesh_id}"

        return True, f"{mesh_id} has {tree_count} tree number(s) (e.g., {first_tree})"

    @test
    def test_descriptor_with_pharmacological_actions(self):
        """Check descriptor has pharmacological actions"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("pharmacological_actions") and len(e["pharmacological_actions"]) > 0),
            None
        )
        if not entry:
            return False, "No descriptor with pharmacological actions in reference"

        mesh_id = entry["id"]
        action_count = len(entry["pharmacological_actions"])

        data = self.runner.lookup(mesh_id)
        if not data or not data.get("results"):
            return False, f"No results for {mesh_id}"

        return True, f"{mesh_id} has {action_count} pharmacological action(s)"

    @test
    def test_text_search_by_descriptor_name(self):
        """Test text search by descriptor name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name") and len(e["name"]) > 5),
            None
        )
        if not entry:
            return False, "No descriptor with suitable name in reference"

        mesh_name = entry["name"]
        mesh_id = entry["id"]

        # Search by name
        data = self.runner.lookup(mesh_name)
        if not data or not data.get("results"):
            return False, f"No results for name: {mesh_name}"

        return True, f"Found results for '{mesh_name[:40]}...'"

    @test
    def test_text_search_by_entry_term(self):
        """Test text search by entry term (synonym)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("entry_terms") and len(e["entry_terms"]) > 0
             and len(e["entry_terms"][0]) > 5),
            None
        )
        if not entry:
            return False, "No descriptor with suitable entry term in reference"

        entry_term = entry["entry_terms"][0]
        mesh_id = entry["id"]

        # Search by entry term
        data = self.runner.lookup(entry_term)
        if not data or not data.get("results"):
            return False, f"No results for entry term: {entry_term}"

        return True, f"Found results for entry term '{entry_term[:40]}...'"

    @test
    def test_descriptor_with_scope_note(self):
        """Check descriptor has scope note (definition)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("scope_note") and len(e["scope_note"]) > 10),
            None
        )
        if not entry:
            return False, "No descriptor with scope note in reference"

        mesh_id = entry["id"]
        scope_preview = entry["scope_note"][:60]

        data = self.runner.lookup(mesh_id)
        if not data or not data.get("results"):
            return False, f"No results for {mesh_id}"

        return True, f"{mesh_id} has scope note: '{scope_preview}...'"

    @test
    def test_descriptor_with_allowable_qualifiers(self):
        """Check descriptor has allowable qualifiers"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("allowable_qualifiers") and len(e["allowable_qualifiers"]) > 0),
            None
        )
        if not entry:
            return False, "No descriptor with allowable qualifiers in reference"

        mesh_id = entry["id"]
        qual_count = len(entry["allowable_qualifiers"])
        first_qual = entry["allowable_qualifiers"][0]

        data = self.runner.lookup(mesh_id)
        if not data or not data.get("results"):
            return False, f"No results for {mesh_id}"

        return True, f"{mesh_id} has {qual_count} allowable qualifier(s) (e.g., {first_qual})"

    @test
    def test_case_insensitive_search(self):
        """Test case-insensitive search for descriptor names"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name") and len(e["name"]) > 5),
            None
        )
        if not entry:
            return False, "No descriptor with suitable name in reference"

        mesh_name = entry["name"]

        # Search with lowercase
        lowercase_name = mesh_name.lower()
        data = self.runner.lookup(lowercase_name)

        if not data or not data.get("results"):
            return False, f"No results for lowercase search: {lowercase_name}"

        return True, f"Case-insensitive search successful for '{mesh_name[:40]}'"


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
    custom_tests = MeshTests(runner)

    for test_method in [
        custom_tests.test_descriptor_with_name,
        custom_tests.test_descriptor_with_entry_terms,
        custom_tests.test_descriptor_with_tree_numbers,
        custom_tests.test_descriptor_with_pharmacological_actions,
        custom_tests.test_text_search_by_descriptor_name,
        custom_tests.test_text_search_by_entry_term,
        custom_tests.test_descriptor_with_scope_note,
        custom_tests.test_descriptor_with_allowable_qualifiers,
        custom_tests.test_case_insensitive_search,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
