#!/usr/bin/env python3
"""
BindingDB (Binding Affinity Database) Test Suite

Tests binding affinity data, cross-references to UniProt/PubChem/ChEMBL,
and compound-target relationships.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class BindingdbTests:
    """BindingDB custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_entry_with_target_name(self):
        """Check entry has target name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_name")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with target_name in reference"

        bindingdb_id = entry["bindingdb_id"]
        target_name = entry["target_name"]
        data = self.runner.lookup(bindingdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {bindingdb_id}"

        return True, f"{bindingdb_id} has target: {target_name[:50]}..."

    @test
    def test_entry_with_ligand_name(self):
        """Check entry has ligand name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("ligand_name")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with ligand_name in reference"

        bindingdb_id = entry["bindingdb_id"]
        ligand_name = entry["ligand_name"]
        data = self.runner.lookup(bindingdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {bindingdb_id}"

        return True, f"Entry has ligand: {ligand_name[:50]}..."

    @test
    def test_entry_with_affinity_data(self):
        """Check entry has binding affinity (Ki, IC50, Kd, or EC50)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("ki") or e.get("ic50") or e.get("kd") or e.get("ec50")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with affinity data in reference"

        bindingdb_id = entry["bindingdb_id"]
        affinity_type = None
        affinity_value = None
        for atype in ["ki", "ic50", "kd", "ec50"]:
            if entry.get(atype):
                affinity_type = atype.upper()
                affinity_value = entry[atype]
                break

        data = self.runner.lookup(bindingdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {bindingdb_id}"

        return True, f"Entry has {affinity_type}: {affinity_value}"

    @test
    def test_text_search_by_ligand_name(self):
        """Test text search by ligand name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("ligand_name") and len(e["ligand_name"]) >= 3),
            None
        )
        if not entry:
            return True, "SKIP: No entry with suitable ligand_name in reference"

        ligand_name = entry["ligand_name"]

        # Search by ligand name
        data = self.runner.lookup(ligand_name)
        if not data or not data.get("results"):
            return False, f"No results for ligand: {ligand_name}"

        return True, f"Found results for ligand '{ligand_name[:40]}'"

    @test
    def test_text_search_by_target_name(self):
        """Test text search by target name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_name") and len(e["target_name"]) > 5),
            None
        )
        if not entry:
            return True, "SKIP: No entry with suitable target_name in reference"

        target_name = entry["target_name"]

        # Search by target name
        data = self.runner.lookup(target_name)
        if not data or not data.get("results"):
            return False, f"No results for target: {target_name}"

        return True, f"Found results for target '{target_name[:40]}'"

    @test
    def test_entry_with_organism(self):
        """Check entry has target organism"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_source_organism")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with target_source_organism in reference"

        bindingdb_id = entry["bindingdb_id"]
        organism = entry["target_source_organism"]
        data = self.runner.lookup(bindingdb_id)

        if not data or not data.get("results"):
            return False, f"No results for {bindingdb_id}"

        return True, f"Entry has organism: {organism}"


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
    custom_tests = BindingdbTests(runner)

    for test_method in [
        custom_tests.test_entry_with_target_name,
        custom_tests.test_entry_with_ligand_name,
        custom_tests.test_entry_with_affinity_data,
        custom_tests.test_text_search_by_ligand_name,
        custom_tests.test_text_search_by_target_name,
        custom_tests.test_entry_with_organism,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
