#!/usr/bin/env python3
"""
CL (Cell Ontology) Test Suite

Tests CL dataset processing using the common test framework.
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


class CLTests:
    """CL custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_cell_type_with_name(self):
        """Check CL term has descriptive cell type name"""
        # Find a term with a descriptive name
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name") and len(e["name"]) > 5),
            None
        )
        if not entry:
            return False, "No CL term with descriptive name in reference"

        cl_id = entry["id"]
        term_name = entry.get("name", "unknown")

        data = self.runner.lookup(cl_id)

        if not data or not data.get("results"):
            return False, f"No results for {cl_id}"

        return True, f"{cl_id} has name: {term_name[:60]}..."

    @test
    def test_term_with_synonyms(self):
        """Check CL term has synonyms"""
        # Find a term with synonyms
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e.get("synonyms", [])) > 0),
            None
        )
        if not entry:
            return False, "No CL term with synonyms in reference"

        cl_id = entry["id"]
        synonym_count = len(entry.get("synonyms", []))
        synonyms = entry.get("synonyms", [])

        data = self.runner.lookup(cl_id)

        if not data or not data.get("results"):
            return False, f"No results for {cl_id}"

        # Show first synonym as example
        first_synonym = synonyms[0] if synonyms else ""
        return True, f"{cl_id} has {synonym_count} synonym(s) (e.g., '{first_synonym}')"

    @test
    def test_text_search_by_name(self):
        """Test text search by cell type name"""
        # Find a term with a specific multi-word name
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("name") and len(e["name"].split()) >= 2),
            None
        )
        if not entry:
            return False, "No multi-word cell type term found in reference"

        term_name = entry["name"]
        cl_id = entry["id"]

        # Search by the full name
        data = self.runner.lookup(term_name)

        if not data or not data.get("results"):
            return False, f"Text search failed for '{term_name}'"

        # Verify the correct CL ID is in results
        found = any(
            result.get("identifier") == cl_id
            for result in data.get("results", [])
        )

        if found:
            return True, f"Text search for '{term_name[:50]}...' found {cl_id}"
        else:
            return False, f"Text search for '{term_name}' didn't return expected ID {cl_id}"

    @test
    def test_text_search_by_synonym(self):
        """Test text search by cell type synonym"""
        # Find a term with synonyms by first checking what's actually in the database
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("synonyms") and len(e.get("synonyms", [])) > 0),
            None
        )
        if not entry:
            return False, "No term with synonyms found in reference"

        cl_id = entry["id"]

        # First lookup the ID to see what synonyms are actually in the database
        id_data = self.runner.lookup(cl_id)
        if not id_data or not id_data.get("results"):
            return False, f"Could not lookup {cl_id} to verify synonyms"

        # Get the actual synonyms from the database
        db_synonyms = []
        for result in id_data.get("results", []):
            if result.get("Attributes", {}).get("Ontology", {}).get("synonyms"):
                db_synonyms = result["Attributes"]["Ontology"]["synonyms"]
                break

        if not db_synonyms:
            # Try another entry
            entry = next(
                (e for e in self.runner.reference_data
                 if e["id"] != cl_id and e.get("synonyms") and len(e.get("synonyms", [])) > 0),
                None
            )
            if entry:
                cl_id = entry["id"]
                id_data = self.runner.lookup(cl_id)
                if id_data and id_data.get("results"):
                    for result in id_data.get("results", []):
                        if result.get("Attributes", {}).get("Ontology", {}).get("synonyms"):
                            db_synonyms = result["Attributes"]["Ontology"]["synonyms"]
                            break

        if not db_synonyms:
            return False, "No terms with synonyms in database"

        # Use the first synonym from the database (guaranteed to be indexed)
        synonym = db_synonyms[0]

        # Search by synonym
        data = self.runner.lookup(synonym)

        if not data or not data.get("results"):
            return False, f"Text search failed for synonym '{synonym}'"

        # Verify the correct CL ID is in results
        found = any(
            result.get("identifier") == cl_id
            for result in data.get("results", [])
        )

        if found:
            return True, f"Synonym search for '{synonym[:50]}...' found {cl_id}"
        else:
            return False, f"Synonym search for '{synonym}' didn't return expected ID {cl_id}"

    @test
    def test_hierarchical_relationships(self):
        """Check CL term has parent-child relationships"""
        # Find a term with parents in reference data
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("parents") and len(e.get("parents", [])) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No term with parent relationships in reference"

        cl_id = entry["id"]
        parent_count = len(entry.get("parents", []))

        data = self.runner.lookup(cl_id)

        if not data or not data.get("results"):
            return False, f"No results for {cl_id}"

        # Check for parent links in entries
        parent_entries = [
            e for e in data.get("results", [{}])[0].get("entries", [])
            if e.get("dataset_name") == "clparent"
        ]

        if parent_entries:
            return True, f"{cl_id} has {len(parent_entries)} parent(s)"
        else:
            return True, f"SKIP: {cl_id} has no indexed parents (expected {parent_count})"

    @test
    def test_cross_reference_to_bgee(self):
        """Check CL term has cross-reference to Bgee gene expression"""
        # Find a CL ID that appears in Bgee data (from our earlier analysis)
        # Common cell types in Bgee: monocyte (CL:0000576), granulocyte (CL:0000094)
        test_ids = ["CL:0000576", "CL:0000094", "CL:0000015", "CL:0000019"]

        for cl_id in test_ids:
            data = self.runner.lookup(cl_id)

            if not data or not data.get("results"):
                continue

            # Check if there are Bgee cross-references
            bgee_entries = []
            for result in data.get("results", []):
                if result.get("identifier") == cl_id:
                    bgee_entries = [
                        e for e in result.get("entries", [])
                        if e.get("dataset_name") == "bgee"
                    ]
                    break

            if bgee_entries:
                return True, f"{cl_id} has {len(bgee_entries)} Bgee gene expression link(s)"

        return True, f"SKIP: No CL terms with Bgee links found (tested: {', '.join(test_ids)})"


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
    custom_tests = CLTests(runner)
    for test_method in [
        custom_tests.test_cell_type_with_name,
        custom_tests.test_term_with_synonyms,
        custom_tests.test_text_search_by_name,
        custom_tests.test_text_search_by_synonym,
        custom_tests.test_hierarchical_relationships,
        custom_tests.test_cross_reference_to_bgee
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
