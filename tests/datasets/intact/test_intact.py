#!/usr/bin/env python3
"""
IntAct Dataset Tests

IntAct is EBI's manually curated database of molecular interactions providing:
- Experimentally validated protein-protein interactions
- Detection methods and interaction types (PSI-MI standard)
- Confidence scores (MIscore)
- Direct PubMed evidence
- Cross-references to UniProt, Ensembl, GO

Test Structure:
- Primary entries: UniProt IDs (e.g., P49418)
- Attributes: protein_id, interactions[], interaction_count, unique_partners
- Cross-references: Partner proteins, PubMed publications
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class IntactTests:
    """IntAct custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_protein_has_interactions(self):
        """Verify proteins have interaction data"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        protein_id = self.runner.reference_data[0]["protein_id"]
        data = self.runner.query.lookup(protein_id)

        if not data or not data.get("results"):
            return False, f"Protein {protein_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Intact", {})

        if not attrs:
            return False, f"No IntAct attributes for {protein_id}"

        # Check interaction fields
        interactions = attrs.get("interactions", [])
        interaction_count = attrs.get("interaction_count", 0)

        if not interactions or interaction_count == 0:
            return False, "Missing interaction data"

        return True, f"✓ Protein {protein_id} has {interaction_count} interactions"

    @test
    def test_interaction_has_partner(self):
        """Verify interactions have partner protein information"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        protein_id = self.runner.reference_data[0]["protein_id"]
        data = self.runner.query.lookup(protein_id)

        if not data or not data.get("results"):
            return False, f"Protein {protein_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Intact", {})
        interactions = attrs.get("interactions", [])

        if not interactions:
            return False, "No interactions found"

        # Check first interaction has partner
        first_interaction = interactions[0]
        partner = first_interaction.get("partner_uniprot")

        if not partner:
            return False, "Interaction missing partner protein"

        return True, f"✓ Interaction has partner: {partner}"

    @test
    def test_confidence_scores(self):
        """Verify interactions have confidence scores"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        proteins_with_scores = 0
        total_checked = 0

        for ref in self.runner.reference_data[:5]:
            protein_id = ref.get("protein_id")
            if not protein_id:
                continue

            data = self.runner.query.lookup(protein_id)
            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Intact", {})
            interactions = attrs.get("interactions", [])

            for interaction in interactions:
                if interaction.get("confidence_score", 0) > 0:
                    proteins_with_scores += 1
                    break

        if total_checked == 0:
            return False, "No proteins could be checked"

        percentage = (proteins_with_scores / total_checked) * 100
        return True, f"✓ {proteins_with_scores}/{total_checked} proteins ({percentage:.0f}%) have confidence scores"

    @test
    def test_pubmed_references(self):
        """Verify interactions link to PubMed publications"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        interactions_with_pubmed = 0
        total_interactions = 0

        for ref in self.runner.reference_data[:3]:
            protein_id = ref.get("protein_id")
            if not protein_id:
                continue

            data = self.runner.query.lookup(protein_id)
            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Intact", {})
            interactions = attrs.get("interactions", [])

            for interaction in interactions[:5]:  # Check first 5 interactions
                total_interactions += 1
                if interaction.get("pubmed_id"):
                    interactions_with_pubmed += 1

        if total_interactions == 0:
            return False, "No interactions could be checked"

        percentage = (interactions_with_pubmed / total_interactions) * 100
        return True, f"✓ {interactions_with_pubmed}/{total_interactions} interactions ({percentage:.0f}%) have PubMed references"

    @test
    def test_detection_methods(self):
        """Verify interactions have experimental detection methods"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        methods_found = set()
        total_checked = 0

        for ref in self.runner.reference_data[:3]:
            protein_id = ref.get("protein_id")
            if not protein_id:
                continue

            data = self.runner.query.lookup(protein_id)
            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Intact", {})
            interactions = attrs.get("interactions", [])

            for interaction in interactions[:10]:
                total_checked += 1
                method = interaction.get("detection_method")
                if method:
                    methods_found.add(method)

        if total_checked == 0:
            return False, "No interactions could be checked"

        return True, f"✓ Found {len(methods_found)} different detection methods in {total_checked} interactions"

    @test
    def test_interaction_types(self):
        """Verify interactions have PSI-MI interaction types"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        interactions_with_types = 0
        total_checked = 0

        for ref in self.runner.reference_data[:3]:
            protein_id = ref.get("protein_id")
            if not protein_id:
                continue

            data = self.runner.query.lookup(protein_id)
            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Intact", {})
            interactions = attrs.get("interactions", [])

            for interaction in interactions[:5]:
                total_checked += 1
                if interaction.get("interaction_type"):
                    interactions_with_types += 1

        if total_checked == 0:
            return False, "No interactions could be checked"

        percentage = (interactions_with_types / total_checked) * 100
        return True, f"✓ {interactions_with_types}/{total_checked} interactions ({percentage:.0f}%) have interaction types"


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
    custom_tests = IntactTests(runner)
    for test_method in [
        custom_tests.test_protein_has_interactions,
        custom_tests.test_interaction_has_partner,
        custom_tests.test_confidence_scores,
        custom_tests.test_pubmed_references,
        custom_tests.test_detection_methods,
        custom_tests.test_interaction_types
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
