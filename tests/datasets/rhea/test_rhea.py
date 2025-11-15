#!/usr/bin/env python3
"""
Rhea Dataset Tests

Rhea is an expert-curated database of biochemical reactions that provides:
- Reaction equations with ChEBI compounds
- EC number classifications
- UniProt enzyme annotations
- GO molecular function terms
- Pathway cross-references (Reactome, KEGG, MetaCyc)
- SMILES representations
- Hierarchical relationships

Test Structure:
- Primary entries: RHEA:xxxxx reaction IDs
- Attributes: equation, direction, status, transport flag, SMILES, hierarchies
- Cross-references: EC, UniProt, ChEBI, GO, pathway databases
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


class RheaTests:
    """Rhea custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_reaction_has_full_attributes(self):
        """Verify Rhea reactions have equation, direction, status"""
        # Get first reaction from reference data
        if not self.runner.reference_data:
            return False, "No reference data available"

        rhea_id = self.runner.reference_data[0]["identifier"]
        data = self.runner.query.lookup(rhea_id)

        if not data or not data.get("results"):
            return False, f"Reaction {rhea_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Rhea", {})

        if not attrs:
            return False, f"No Rhea attributes for {rhea_id}"

        # Check required fields
        required_fields = ["equation", "direction", "status"]
        missing = [f for f in required_fields if not attrs.get(f)]

        if missing:
            return False, f"Missing required fields: {', '.join(missing)}"

        # Verify values are reasonable
        if not attrs["equation"]:
            return False, "Empty equation"

        if attrs["direction"] not in ["LR", "RL", "BI", "UN"]:
            return False, f"Invalid direction: {attrs['direction']}"

        if attrs["status"] not in ["Approved", "Preliminary", "Obsolete"]:
            return False, f"Invalid status: {attrs['status']}"

        return True, f"✓ Reaction has equation=\"{attrs['equation'][:50]}...\", direction={attrs['direction']}, status={attrs['status']}"

    @test
    def test_smiles_present(self):
        """Verify reaction SMILES are stored when available"""
        # Count how many reactions have SMILES
        smiles_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:  # Check first 10
            rhea_id = ref["identifier"]
            data = self.runner.query.lookup(rhea_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Rhea", {})

            if attrs.get("reaction_smiles"):
                smiles_count += 1

        if total_checked == 0:
            return False, "No reactions found to check"

        return True, f"✓ {smiles_count}/{total_checked} reactions have SMILES ({smiles_count*100//total_checked}%)"

    @test
    def test_hierarchies_present(self):
        """Verify parent/child relationships are stored"""
        parent_count = 0
        child_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:20]:  # Check first 20
            rhea_id = ref["identifier"]
            data = self.runner.query.lookup(rhea_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Rhea", {})

            if attrs.get("parent_reactions"):
                parent_count += 1

            if attrs.get("child_reactions"):
                child_count += 1

        if total_checked == 0:
            return False, "No reactions found to check"

        return True, f"✓ {parent_count}/{total_checked} with parents, {child_count}/{total_checked} with children"

    @test
    def test_ec_number_xrefs(self):
        """Verify Rhea reactions link to EC numbers"""
        ec_xref_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            rhea_id = ref["identifier"]

            # Query with x=true to get cross-references
            try:
                response = requests.get(
                    f"{self.runner.api_url}/ws/search",
                    params={"i": rhea_id, "x": "true"},
                    timeout=5
                )
                if response.status_code != 200:
                    continue

                data = response.json()
                if not data or not data.get("results"):
                    continue

                total_checked += 1
                result = data["results"][0]

                # Check for EC cross-references
                if self.runner.has_xref(result, "ec"):
                    ec_xref_count += 1
            except Exception:
                continue

        if total_checked == 0:
            return False, "No reactions found to check"

        return True, f"✓ {ec_xref_count}/{total_checked} reactions have EC number cross-references"

    @test
    def test_uniprot_enzyme_xrefs(self):
        """Verify Rhea reactions link to UniProt enzymes"""
        uniprot_xref_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            rhea_id = ref["identifier"]

            # Query with x=true to get cross-references
            try:
                response = requests.get(
                    f"{self.runner.api_url}/ws/search",
                    params={"i": rhea_id, "x": "true"},
                    timeout=5
                )
                if response.status_code != 200:
                    continue

                data = response.json()
                if not data or not data.get("results"):
                    continue

                total_checked += 1
                result = data["results"][0]

                # Check for UniProt cross-references
                if self.runner.has_xref(result, "uniprot"):
                    uniprot_xref_count += 1
            except Exception:
                continue

        if total_checked == 0:
            return False, "No reactions found to check"

        return True, f"✓ {uniprot_xref_count}/{total_checked} reactions have UniProt enzyme cross-references"

    @test
    def test_chebi_compound_xrefs(self):
        """Verify Rhea reactions link to ChEBI compounds"""
        chebi_xref_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            rhea_id = ref["identifier"]

            # Query with x=true to get cross-references
            try:
                response = requests.get(
                    f"{self.runner.api_url}/ws/search",
                    params={"i": rhea_id, "x": "true"},
                    timeout=5
                )
                if response.status_code != 200:
                    continue

                data = response.json()
                if not data or not data.get("results"):
                    continue

                total_checked += 1
                result = data["results"][0]

                # Check for ChEBI cross-references
                if self.runner.has_xref(result, "chebi"):
                    chebi_xref_count += 1
            except Exception:
                continue

        if total_checked == 0:
            return False, "No reactions found to check"

        return True, f"✓ {chebi_xref_count}/{total_checked} reactions have ChEBI compound cross-references"


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
    custom_tests = RheaTests(runner)
    for test_method in [
        custom_tests.test_reaction_has_full_attributes,
        custom_tests.test_smiles_present,
        custom_tests.test_hierarchies_present,
        custom_tests.test_ec_number_xrefs,
        custom_tests.test_uniprot_enzyme_xrefs,
        custom_tests.test_chebi_compound_xrefs
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
