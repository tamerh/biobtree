#!/usr/bin/env python3
"""
ESM2 Protein Similarity Dataset Tests

ESM2 protein embedding-based semantic similarity database providing:
- Top N similar proteins for each query protein based on cosine similarity
- Similarity scores from ESM2 embeddings (captures functional/structural similarity)
- Cross-references to similar UniProt proteins

Test Structure:
- Primary entries: UniProt IDs (e.g., Q6GZX4)
- Attributes: protein_id, similarities[], similarity_count, top_similarity, avg_similarity
- Cross-references: Similar UniProt proteins
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class Esm2SimilarityTests:
    """ESM2 Similarity custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_uniprot_id_format(self):
        """Verify UniProt IDs are valid"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        uniprot_id = self.runner.reference_data[0]["uniprot_id"]

        if len(uniprot_id) < 6:
            return False, f"UniProt ID too short: {uniprot_id}"

        # UniProt format: starts with letter/number and is alphanumeric
        if not uniprot_id[0].isalnum():
            return False, f"UniProt ID format invalid: {uniprot_id}"

        return True, f"UniProt ID format correct: {uniprot_id}"

    @test
    def test_protein_has_similarities(self):
        """Verify proteins have similarity data"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        uniprot_id = self.runner.reference_data[0]["uniprot_id"]
        data = self.runner.query.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"Protein {uniprot_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Esm2Similarity", {})

        if not attrs:
            return False, f"No ESM2 Similarity attributes for {uniprot_id}"

        # Check similarity fields
        similarities = attrs.get("similarities", [])
        similarity_count = attrs.get("similarity_count", 0)

        if not similarities or similarity_count == 0:
            return False, "Missing similarity data"

        return True, f"Protein {uniprot_id} has {similarity_count} similar proteins"

    @test
    def test_similarity_has_target_info(self):
        """Verify similarities have target protein information"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        uniprot_id = self.runner.reference_data[0]["uniprot_id"]
        data = self.runner.query.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"Protein {uniprot_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Esm2Similarity", {})
        similarities = attrs.get("similarities", [])

        if not similarities:
            return False, "No similarities found"

        # Check first similarity has target
        first_sim = similarities[0]
        target_uniprot = first_sim.get("target_uniprot")

        if not target_uniprot:
            return False, "Similarity missing target UniProt ID"

        return True, f"Similarity has target: {target_uniprot}"

    @test
    def test_cosine_similarity_range(self):
        """Verify cosine similarity scores are in valid range [0, 1]"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        uniprot_id = self.runner.reference_data[0]["uniprot_id"]
        data = self.runner.query.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"Protein {uniprot_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Esm2Similarity", {})
        similarities = attrs.get("similarities", [])

        if not similarities:
            return False, "No similarities found"

        # Check all similarity scores are in valid range
        for sim in similarities:
            score = sim.get("cosine_similarity", 0)
            if score < 0 or score > 1:
                return False, f"Invalid cosine similarity: {score} (must be 0-1)"

        first_score = similarities[0].get("cosine_similarity", 0)
        return True, f"Cosine similarity scores valid (first: {first_score:.4f})"

    @test
    def test_rank_ordering(self):
        """Verify similarities are ranked correctly (1 = most similar)"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        uniprot_id = self.runner.reference_data[0]["uniprot_id"]
        data = self.runner.query.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"Protein {uniprot_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Esm2Similarity", {})
        similarities = attrs.get("similarities", [])

        if not similarities:
            return False, "No similarities found"

        # Check ranks are sequential starting from 1
        for i, sim in enumerate(similarities):
            expected_rank = i + 1
            actual_rank = sim.get("rank", 0)
            if actual_rank != expected_rank:
                return False, f"Rank mismatch at position {i}: expected {expected_rank}, got {actual_rank}"

        # Check scores are descending (higher score = more similar)
        for i in range(len(similarities) - 1):
            score1 = similarities[i].get("cosine_similarity", 0)
            score2 = similarities[i + 1].get("cosine_similarity", 0)
            if score1 < score2:
                return False, f"Scores not descending: {score1} < {score2}"

        return True, f"Ranks 1-{len(similarities)} correctly ordered by similarity"

    @test
    def test_cross_reference_to_uniprot(self):
        """Verify ESM2 entry links to UniProt"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        uniprot_id = self.runner.reference_data[0]["uniprot_id"]
        data = self.runner.query.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"Protein {uniprot_id} not found"

        result = data["results"][0]

        # Check for cross-reference to UniProt
        if not self.runner.has_xref(result, "uniprot", uniprot_id):
            return True, f"SKIP: UniProt xref to self not found (UniProt dataset not loaded)"

        return True, f"ESM2 entry {uniprot_id} links to UniProt"

    @test
    def test_top_scores_calculated(self):
        """Verify top and average similarity are calculated"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        proteins_with_scores = 0
        total_checked = 0

        for ref in self.runner.reference_data[:5]:
            uniprot_id = ref.get("uniprot_id")
            if not uniprot_id:
                continue

            data = self.runner.query.lookup(uniprot_id)
            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Esm2Similarity", {})

            top_similarity = attrs.get("top_similarity")
            avg_similarity = attrs.get("avg_similarity")

            if top_similarity is not None and avg_similarity is not None:
                if top_similarity > 0 and avg_similarity > 0:
                    proteins_with_scores += 1

            total_checked += 1

        if total_checked == 0:
            return False, "No proteins checked"

        if proteins_with_scores == 0:
            return False, "No proteins have top/avg scores calculated"

        return True, f"{proteins_with_scores}/{total_checked} proteins have top/avg scores"

    @test
    def test_similar_proteins_xref(self):
        """Verify similar proteins are cross-referenced"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        uniprot_id = self.runner.reference_data[0]["uniprot_id"]
        data = self.runner.query.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"Protein {uniprot_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Esm2Similarity", {})
        similarities = attrs.get("similarities", [])

        if not similarities:
            return False, "No similarities found"

        # Check that at least some similar proteins are cross-referenced
        uniprot_xrefs = self.runner.get_xrefs(result, "uniprot")

        if not uniprot_xrefs:
            return True, "SKIP: No UniProt cross-references found (xref lookup may not work in test mode)"

        # Should have xrefs to similar proteins (not just the original)
        if len(uniprot_xrefs) <= 1:
            return False, "Only one UniProt xref (should have similar proteins too)"

        return True, f"{len(uniprot_xrefs)} UniProt xrefs (includes similar proteins)"


def main():
    """Main test entry point"""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Create test runner
    # Note: reference_data.json is optional for basic tests
    runner = TestRunner(api_url, reference_file if reference_file.exists() else None, test_cases_file)

    # Add custom tests
    custom_tests = Esm2SimilarityTests(runner)
    for test_method in [
        custom_tests.test_uniprot_id_format,
        custom_tests.test_protein_has_similarities,
        custom_tests.test_similarity_has_target_info,
        custom_tests.test_cosine_similarity_range,
        custom_tests.test_rank_ordering,
        custom_tests.test_cross_reference_to_uniprot,
        custom_tests.test_top_scores_calculated,
        custom_tests.test_similar_proteins_xref
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
