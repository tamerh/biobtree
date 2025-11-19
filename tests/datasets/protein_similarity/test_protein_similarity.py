#!/usr/bin/env python3
"""
Protein Similarity Dataset Tests

DIAMOND BLASTP protein sequence similarity database providing:
- Top N similar proteins for each query protein
- Alignment statistics (identity, e-value, bitscore)
- Sequence alignment positions
- Cross-organism homology mapping

Test Structure:
- Primary entries: DIAMOND IDs with "d" prefix (e.g., dP01942)
- Attributes: protein_id, similarities[], similarity_count, top_identity, top_bitscore
- Cross-references: Similar UniProt proteins
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class ProteinSimilarityTests:
    """Protein Similarity custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_diamond_id_format(self):
        """Verify DIAMOND IDs have 'd' prefix"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        diamond_id = self.runner.reference_data[0]["diamond_id"]

        if not diamond_id.startswith('d'):
            return False, f"DIAMOND ID missing 'd' prefix: {diamond_id}"

        if len(diamond_id) < 2:
            return False, f"DIAMOND ID too short: {diamond_id}"

        return True, f"✓ DIAMOND ID format correct: {diamond_id}"

    @test
    def test_protein_has_similarities(self):
        """Verify proteins have similarity data"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        diamond_id = self.runner.reference_data[0]["diamond_id"]
        data = self.runner.query.lookup(diamond_id)

        if not data or not data.get("results"):
            return False, f"Protein {diamond_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("ProteinSimilarity", {})

        if not attrs:
            return False, f"No Protein Similarity attributes for {diamond_id}"

        # Check similarity fields
        similarities = attrs.get("similarities", [])
        similarity_count = attrs.get("similarity_count", 0)

        if not similarities or similarity_count == 0:
            return False, "Missing similarity data"

        return True, f"✓ Protein {diamond_id} has {similarity_count} similar proteins"

    @test
    def test_similarity_has_target_info(self):
        """Verify similarities have target protein information"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        diamond_id = self.runner.reference_data[0]["diamond_id"]
        data = self.runner.query.lookup(diamond_id)

        if not data or not data.get("results"):
            return False, f"Protein {diamond_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("ProteinSimilarity", {})
        similarities = attrs.get("similarities", [])

        if not similarities:
            return False, "No similarities found"

        # Check first similarity has target
        first_sim = similarities[0]
        target_uniprot = first_sim.get("target_uniprot")
        target_name = first_sim.get("target_name")

        if not target_uniprot:
            return False, "Similarity missing target UniProt ID"

        if not target_name:
            return False, "Similarity missing target name"

        return True, f"✓ Similarity has target: {target_uniprot} ({target_name})"

    @test
    def test_alignment_statistics(self):
        """Verify similarities have alignment statistics"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        diamond_id = self.runner.reference_data[0]["diamond_id"]
        data = self.runner.query.lookup(diamond_id)

        if not data or not data.get("results"):
            return False, f"Protein {diamond_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("ProteinSimilarity", {})
        similarities = attrs.get("similarities", [])

        if not similarities:
            return False, "No similarities found"

        first_sim = similarities[0]

        # Check required alignment fields
        required_fields = ["identity", "alignment_length", "evalue", "bitscore"]
        missing = [f for f in required_fields if f not in first_sim]

        if missing:
            return False, f"Missing alignment fields: {', '.join(missing)}"

        identity = first_sim.get("identity", 0)
        if identity < 0 or identity > 100:
            return False, f"Invalid identity value: {identity}"

        return True, f"✓ Alignment stats present (identity: {identity}%)"

    @test
    def test_cross_reference_to_uniprot(self):
        """Verify DIAMOND ID links to UniProt"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        diamond_id = self.runner.reference_data[0]["diamond_id"]
        expected_uniprot = self.runner.reference_data[0].get("uniprot_id")

        if not expected_uniprot:
            return False, "Reference data missing UniProt ID"

        data = self.runner.query.lookup(diamond_id)

        if not data or not data.get("results"):
            return False, f"Protein {diamond_id} not found"

        result = data["results"][0]

        # Check for cross-reference to UniProt
        if not self.runner.has_xref(result, "uniprot", expected_uniprot):
            return False, f"Missing xref to UniProt: {expected_uniprot}"

        return True, f"✓ DIAMOND ID {diamond_id} links to UniProt {expected_uniprot}"

    @test
    def test_top_scores_calculated(self):
        """Verify top identity and bitscore are calculated"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        proteins_with_scores = 0
        total_checked = 0

        for ref in self.runner.reference_data[:5]:
            diamond_id = ref.get("diamond_id")
            if not diamond_id:
                continue

            data = self.runner.query.lookup(diamond_id)
            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("ProteinSimilarity", {})

            top_identity = attrs.get("top_identity")
            top_bitscore = attrs.get("top_bitscore")

            if top_identity is not None and top_bitscore is not None:
                if top_identity > 0 and top_bitscore > 0:
                    proteins_with_scores += 1

            total_checked += 1

        if total_checked == 0:
            return False, "No proteins checked"

        if proteins_with_scores == 0:
            return False, "No proteins have top scores calculated"

        return True, f"✓ {proteins_with_scores}/{total_checked} proteins have top scores"

    @test
    def test_similar_proteins_xref(self):
        """Verify similar proteins are cross-referenced"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        diamond_id = self.runner.reference_data[0]["diamond_id"]
        data = self.runner.query.lookup(diamond_id)

        if not data or not data.get("results"):
            return False, f"Protein {diamond_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("ProteinSimilarity", {})
        similarities = attrs.get("similarities", [])

        if not similarities:
            return False, "No similarities found"

        # Check that at least some similar proteins are cross-referenced
        uniprot_xrefs = self.runner.get_xrefs(result, "uniprot")

        if not uniprot_xrefs:
            return False, "No UniProt cross-references found"

        # Should have xrefs to similar proteins (not just the original)
        if len(uniprot_xrefs) <= 1:
            return False, "Only one UniProt xref (should have similar proteins too)"

        return True, f"✓ {len(uniprot_xrefs)} UniProt xrefs (includes similar proteins)"


def main():
    """Main test entry point"""
    runner = TestRunner(
        dataset_name="protein_similarity",
        test_cases_file=Path(__file__).parent / "test_cases.json",
        reference_data_file=Path(__file__).parent / "reference_data.json"
    )

    # Run declarative tests from JSON
    runner.run_declarative_tests()

    # Run custom tests
    custom_tests = ProteinSimilarityTests(runner)
    runner.run_custom_tests(custom_tests)

    # Print results
    runner.print_summary()
    return 0 if runner.all_passed() else 1


if __name__ == "__main__":
    sys.exit(main())
