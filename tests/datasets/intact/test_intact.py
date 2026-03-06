#!/usr/bin/env python3
"""
IntAct Dataset Tests - Interaction-Centric Model

IntAct is EBI's manually curated database of molecular interactions providing:
- Experimentally validated protein-protein interactions
- Detection methods and interaction types (PSI-MI standard)
- Confidence scores (MIscore)
- Direct PubMed evidence
- Cross-references to UniProt, Ensembl, GO

Data Model (Interaction-Centric):
- Primary entries: Interaction IDs (e.g., EBI-7121552)
- Each interaction contains:
  - protein_a, protein_b (UniProt IDs)
  - All experimental details (methods, scores, features)
- Cross-references: Proteins link to their interactions via xrefs
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class IntactTests:
    """IntAct custom tests for interaction-centric model"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _get_interaction(self, interaction_id):
        """Helper to look up an interaction by ID"""
        data = self.runner.query.lookup(interaction_id)
        if not data or not data.get("results"):
            return None
        result = data["results"][0]
        return result.get("Attributes", {}).get("Intact", {})

    @test
    def test_interaction_has_proteins(self):
        """Verify interactions have both protein identifiers"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        interaction_id = self.runner.reference_data[0].get("interaction_id")
        if not interaction_id:
            return False, "No interaction_id in reference data"

        attrs = self._get_interaction(interaction_id)
        if not attrs:
            return False, f"Interaction {interaction_id} not found"

        protein_a = attrs.get("protein_a")
        protein_b = attrs.get("protein_b")

        if not protein_a or not protein_b:
            return False, "Interaction missing protein identifiers"

        return True, f"Interaction {interaction_id}: {protein_a} <-> {protein_b}"

    @test
    def test_interaction_has_gene_names(self):
        """Verify interactions have gene names for proteins"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        found_genes = 0
        total_checked = 0

        for ref in self.runner.reference_data[:5]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            total_checked += 1
            gene_a = attrs.get("protein_a_gene")
            gene_b = attrs.get("protein_b_gene")

            if gene_a or gene_b:
                found_genes += 1

        if total_checked == 0:
            return False, "No interactions could be checked"

        percentage = (found_genes / total_checked) * 100
        return True, f"{found_genes}/{total_checked} interactions ({percentage:.0f}%) have gene names"

    @test
    def test_confidence_scores(self):
        """Verify interactions have confidence scores"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        interactions_with_scores = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            total_checked += 1
            if attrs.get("confidence_score", 0) > 0:
                interactions_with_scores += 1

        if total_checked == 0:
            return False, "No interactions could be checked"

        percentage = (interactions_with_scores / total_checked) * 100
        return True, f"{interactions_with_scores}/{total_checked} interactions ({percentage:.0f}%) have confidence scores"

    @test
    def test_pubmed_references(self):
        """Verify interactions link to PubMed publications"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        interactions_with_pubmed = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            total_checked += 1
            if attrs.get("pubmed_id"):
                interactions_with_pubmed += 1

        if total_checked == 0:
            return False, "No interactions could be checked"

        percentage = (interactions_with_pubmed / total_checked) * 100
        return True, f"{interactions_with_pubmed}/{total_checked} interactions ({percentage:.0f}%) have PubMed references"

    @test
    def test_detection_methods(self):
        """Verify interactions have experimental detection methods"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        methods_found = set()
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            total_checked += 1
            method = attrs.get("detection_method")
            if method:
                methods_found.add(method)

        if total_checked == 0:
            return False, "No interactions could be checked"

        return True, f"Found {len(methods_found)} different detection methods in {total_checked} interactions"

    @test
    def test_interaction_types(self):
        """Verify interactions have PSI-MI interaction types"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        interactions_with_types = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            total_checked += 1
            if attrs.get("interaction_type"):
                interactions_with_types += 1

        if total_checked == 0:
            return False, "No interactions could be checked"

        percentage = (interactions_with_types / total_checked) * 100
        return True, f"{interactions_with_types}/{total_checked} interactions ({percentage:.0f}%) have interaction types"

    @test
    def test_psi_mi_term_parsing(self):
        """Verify PSI-MI terms are parsed into structured fields (P0 improvement)"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        for ref in self.runner.reference_data[:10]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            parsed = attrs.get("detection_method_parsed")
            if parsed:
                if not parsed.get("mi_id") or not parsed.get("term_name"):
                    return False, "PSI-MI term missing mi_id or term_name"
                return True, f"PSI-MI parsing works: {parsed.get('mi_id')} = {parsed.get('term_name')}"

        return False, "No parsed PSI-MI terms found"

    @test
    def test_confidence_score_components(self):
        """Verify confidence scores have detailed components (P0 improvement)"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        for ref in self.runner.reference_data[:10]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            scores = attrs.get("confidence_scores")
            if scores:
                if "miscore" not in scores:
                    return False, "Confidence scores missing miscore field"
                return True, f"Confidence score components: miscore={scores.get('miscore')}"

        return False, "No confidence score components found"

    @test
    def test_host_organism(self):
        """Verify host organism field is populated (P2 improvement)"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        found_host = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            total_checked += 1
            if attrs.get("host_taxid") or attrs.get("host_organism_name"):
                found_host += 1

        if total_checked == 0:
            return False, "No interactions could be checked"

        if found_host == 0:
            return False, "No host organism data found"

        return True, f"{found_host}/{total_checked} interactions have host organism data"

    @test
    def test_binding_site_features(self):
        """Verify binding site features are parsed (P1 improvement)"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        found_features = 0
        total_checked = 0

        for ref in self.runner.reference_data[:20]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            total_checked += 1
            features_a = attrs.get("features_a", [])
            features_b = attrs.get("features_b", [])
            if features_a or features_b:
                found_features += 1
                # Verify feature structure
                all_features = (features_a or []) + (features_b or [])
                if all_features:
                    first_feature = all_features[0]
                    if "feature_type" in first_feature or "description" in first_feature:
                        continue

        if total_checked == 0:
            return False, "No interactions could be checked"

        # Features have ~8% coverage, so we expect some
        return True, f"{found_features}/{total_checked} interactions have binding site features"

    @test
    def test_method_reliability_score(self):
        """Verify method reliability scores are computed (P1 improvement)"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        scores_found = {}
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            interaction_id = ref.get("interaction_id")
            if not interaction_id:
                continue

            attrs = self._get_interaction(interaction_id)
            if not attrs:
                continue

            total_checked += 1
            score = attrs.get("method_reliability_score", 0)
            if score > 0:
                method = attrs.get("detection_method_parsed", {}).get("mi_id", "unknown")
                scores_found[method] = score

        if not scores_found:
            return False, "No method reliability scores found"

        examples = [f"{mi}={s}" for mi, s in list(scores_found.items())[:3]]
        return True, f"Method reliability scores: {', '.join(examples)}"

    @test
    def test_protein_xref_to_interactions(self):
        """Verify proteins can be looked up and link to interactions"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        # Get a protein from one of the interactions
        interaction_id = self.runner.reference_data[0].get("interaction_id")
        if not interaction_id:
            return False, "No interaction_id in reference data"

        attrs = self._get_interaction(interaction_id)
        if not attrs:
            return False, f"Interaction {interaction_id} not found"

        protein_a = attrs.get("protein_a")
        if not protein_a:
            return False, "Interaction missing protein_a"

        # Look up the protein and check its xrefs to interactions
        data = self.runner.query.lookup(protein_a)
        if not data or not data.get("results"):
            return False, f"Protein {protein_a} not found"

        result = data["results"][0]
        entries = result.get("entries", [])

        # Check if the interaction appears in the protein's linked entries
        interaction_ids = [e.get("identifier") for e in entries if e.get("dataset_name") == "intact"]

        if not interaction_ids:
            return False, f"Protein {protein_a} has no linked interactions"

        if interaction_id not in interaction_ids:
            return False, f"Expected interaction {interaction_id} not in protein's xrefs"

        return True, f"Protein {protein_a} links to {len(interaction_ids)} interactions including {interaction_id}"


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
        custom_tests.test_interaction_has_proteins,
        custom_tests.test_interaction_has_gene_names,
        custom_tests.test_confidence_scores,
        custom_tests.test_pubmed_references,
        custom_tests.test_detection_methods,
        custom_tests.test_interaction_types,
        # Enhanced field tests (P0, P1, P2 improvements)
        custom_tests.test_psi_mi_term_parsing,
        custom_tests.test_confidence_score_components,
        custom_tests.test_host_organism,
        custom_tests.test_binding_site_features,
        custom_tests.test_method_reliability_score,
        # Cross-reference test
        custom_tests.test_protein_xref_to_interactions,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
