#!/usr/bin/env python3
"""
DrugCentral Test Suite

Tests drug-target interactions, chemical structure data,
and cross-references to UniProt.
"""

import sys
import os
import requests
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class DrugcentralTests:
    """DrugCentral custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _test_mapping(self, source_id: str, target_dataset: str) -> tuple:
        """Helper to test a mapping from DrugCentral to another dataset"""
        url = f"{self.runner.api_url}/ws/map/?i={source_id}&m=>>drugcentral>>{target_dataset}"
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
    def test_entry_with_drug_name(self):
        """Check entry has drug name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("drug_name")),
            None
        )
        if not entry:
            return True, "SKIP: No entry with drug_name in reference"

        struct_id = entry["struct_id"]
        drug_name = entry["drug_name"]
        data = self.runner.lookup(struct_id)

        if not data or not data.get("results"):
            return False, f"No results for {struct_id}"

        return True, f"{struct_id} has name: {drug_name[:50]}"

    @test
    def test_entry_with_targets(self):
        """Check entry has target interactions"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("targets") and len(e["targets"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with targets in reference"

        struct_id = entry["struct_id"]
        count = len(entry["targets"])
        data = self.runner.lookup(struct_id)

        if not data or not data.get("results"):
            return False, f"No results for {struct_id}"

        return True, f"Entry has {count} target interactions"

    @test
    def test_entry_with_smiles(self):
        """Check entry has SMILES structure"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("smiles") and len(e["smiles"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with smiles in reference"

        struct_id = entry["struct_id"]
        smiles = entry["smiles"]
        data = self.runner.lookup(struct_id)

        if not data or not data.get("results"):
            return False, f"No results for {struct_id}"

        return True, f"Entry has SMILES: {smiles[:40]}..."

    @test
    def test_entry_with_inchi_key(self):
        """Check entry has InChI Key"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("inchi_key") and len(e["inchi_key"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with inchi_key in reference"

        struct_id = entry["struct_id"]
        inchi_key = entry["inchi_key"]
        data = self.runner.lookup(struct_id)

        if not data or not data.get("results"):
            return False, f"No results for {struct_id}"

        return True, f"Entry has InChI Key: {inchi_key}"

    @test
    def test_entry_with_cas_rn(self):
        """Check entry has CAS Registry Number"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("cas_rn") and len(e["cas_rn"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with cas_rn in reference"

        struct_id = entry["struct_id"]
        cas_rn = entry["cas_rn"]
        data = self.runner.lookup(struct_id)

        if not data or not data.get("results"):
            return False, f"No results for {struct_id}"

        return True, f"Entry has CAS RN: {cas_rn}"

    @test
    def test_text_search_by_drug_name(self):
        """Test text search by drug name"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("drug_name") and len(e["drug_name"]) >= 4),
            None
        )
        if not entry:
            return True, "SKIP: No entry with suitable drug_name in reference"

        drug_name = entry["drug_name"]

        # Search by drug name
        data = self.runner.lookup(drug_name)
        if not data or not data.get("results"):
            return False, f"No results for drug: {drug_name}"

        return True, f"Found results for '{drug_name[:40]}'"

    @test
    def test_text_search_by_inchi_key(self):
        """Test text search by InChI Key"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("inchi_key") and len(e["inchi_key"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with inchi_key in reference"

        inchi_key = entry["inchi_key"]

        # Search by InChI Key
        data = self.runner.lookup(inchi_key)
        if not data or not data.get("results"):
            return False, f"No results for InChI Key: {inchi_key}"

        return True, f"Found results for InChI Key"

    @test
    def test_entry_with_action_types(self):
        """Check entry has action types"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("action_types") and len(e["action_types"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with action_types in reference"

        struct_id = entry["struct_id"]
        action_types = entry["action_types"]
        data = self.runner.lookup(struct_id)

        if not data or not data.get("results"):
            return False, f"No results for {struct_id}"

        return True, f"Entry has action types: {', '.join(action_types[:3])}"

    @test
    def test_entry_with_target_classes(self):
        """Check entry has target classes"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("target_classes") and len(e["target_classes"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with target_classes in reference"

        struct_id = entry["struct_id"]
        target_classes = entry["target_classes"]
        data = self.runner.lookup(struct_id)

        if not data or not data.get("results"):
            return False, f"No results for {struct_id}"

        return True, f"Entry has target classes: {', '.join(target_classes[:3])}"

    @test
    def test_mapping_drugcentral_to_uniprot(self):
        """Test DrugCentral -> UniProt mapping (target proteins)"""
        # Find an entry with targets that have UniProt accessions
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("targets") and
             any(t.get("uniprot_accession") for t in e["targets"])),
            None
        )
        if not entry:
            return True, "SKIP: No entry with UniProt-linked targets in reference"

        struct_id = entry["struct_id"]
        return self._test_mapping(struct_id, "uniprot")

    @test
    def test_target_with_activity_data(self):
        """Check target has activity data (IC50, Ki, etc.)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("targets") and
             any(t.get("act_value") and t.get("act_type") for t in e["targets"])),
            None
        )
        if not entry:
            return True, "SKIP: No entry with activity data in reference"

        struct_id = entry["struct_id"]
        target = next(t for t in entry["targets"] if t.get("act_value"))
        act_type = target.get("act_type", "?")
        act_value = target.get("act_value", "?")

        data = self.runner.lookup(struct_id)

        if not data or not data.get("results"):
            return False, f"No results for {struct_id}"

        return True, f"Target has activity: {act_type}={act_value}"

    @test
    def test_target_with_tdl(self):
        """Check target has TDL (Target Development Level)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("targets") and
             any(t.get("tdl") for t in e["targets"])),
            None
        )
        if not entry:
            return True, "SKIP: No entry with TDL in reference"

        struct_id = entry["struct_id"]
        target = next(t for t in entry["targets"] if t.get("tdl"))
        tdl = target.get("tdl", "?")

        data = self.runner.lookup(struct_id)

        if not data or not data.get("results"):
            return False, f"No results for {struct_id}"

        return True, f"Target has TDL: {tdl[:30]}"


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
    custom_tests = DrugcentralTests(runner)

    for test_method in [
        # Attribute tests
        custom_tests.test_entry_with_drug_name,
        custom_tests.test_entry_with_targets,
        custom_tests.test_entry_with_smiles,
        custom_tests.test_entry_with_inchi_key,
        custom_tests.test_entry_with_cas_rn,
        custom_tests.test_text_search_by_drug_name,
        custom_tests.test_text_search_by_inchi_key,
        custom_tests.test_entry_with_action_types,
        custom_tests.test_entry_with_target_classes,
        # Activity and TDL tests
        custom_tests.test_target_with_activity_data,
        custom_tests.test_target_with_tdl,
        # Cross-reference mapping tests
        custom_tests.test_mapping_drugcentral_to_uniprot,
    ]:
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
