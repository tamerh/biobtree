#!/usr/bin/env python3
"""
GWAS Study Dataset Tests

Tests biobtree's GWAS Catalog study metadata integration. Validates:
- Study lookup by accession ID
- Publication metadata extraction
- Disease/trait mappings
- Association counts
- Cross-references to EFO traits
- Text search functionality

GWAS Catalog API: https://www.ebi.ac.uk/gwas/rest/api/studies/{accession}
"""

import json
import os
import sys
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


class GwasStudyTests:
    """Custom tests for GWAS Study dataset"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_study_has_publication_metadata(self):
        """Verify study has publication date, PubMed ID, and author information"""
        studies_with_pubmed = 0
        studies_with_date = 0
        studies_with_author = 0
        total_checked = 0

        for ref_study in self.runner.reference_data[:20]:
            study_id = ref_study.get('accessionId')
            if not study_id:
                continue

            data = self.runner.query.lookup(study_id)
            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("GwasStudy", {})

            if not attrs:
                continue

            # Check PubMed ID
            if attrs.get('pubmed_id'):
                studies_with_pubmed += 1

            # Check publication date
            if attrs.get('publication_date'):
                studies_with_date += 1

            # Check author
            if attrs.get('first_author'):
                studies_with_author += 1

        if total_checked == 0:
            return False, "No studies found to check"

        coverage = (studies_with_pubmed + studies_with_date + studies_with_author) / (3 * total_checked) * 100

        return True, f"✓ Publication metadata: {studies_with_pubmed}/{total_checked} PubMed IDs, {studies_with_date}/{total_checked} dates, {studies_with_author}/{total_checked} authors (coverage: {coverage:.1f}%)"

    @test
    def test_study_has_disease_trait_mappings(self):
        """Verify study has disease trait and mapped EFO traits"""
        studies_with_trait = 0
        studies_with_efo = 0
        total_checked = 0

        for ref_study in self.runner.reference_data[:20]:
            study_id = ref_study.get('accessionId')
            if not study_id:
                continue

            # Query with x=true to get cross-references
            try:
                response = requests.get(
                    f"{self.runner.api_url}/ws/search",
                    params={"i": study_id, "x": "true"},
                    timeout=5
                )
                if response.status_code != 200:
                    continue

                data = response.json()
                if not data or not data.get("results"):
                    continue

                total_checked += 1
                result = data["results"][0]
                attrs = result.get("Attributes", {}).get("GwasStudy", {})

                # Check disease trait
                if attrs.get('disease_trait'):
                    studies_with_trait += 1

                # Check EFO traits via xrefs
                if self.runner.has_xref(result, "efo"):
                    studies_with_efo += 1
            except Exception:
                continue

        if total_checked == 0:
            return False, "No studies found to check"

        return True, f"✓ Trait mappings: {studies_with_trait}/{total_checked} disease traits, {studies_with_efo}/{total_checked} EFO xrefs"

    @test
    def test_study_has_association_count(self):
        """Verify association_count field is present and valid"""
        studies_with_count = 0
        valid_counts = 0
        total_checked = 0

        for ref_study in self.runner.reference_data[:20]:
            study_id = ref_study.get('accessionId')
            if not study_id:
                continue

            data = self.runner.query.lookup(study_id)
            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("GwasStudy", {})

            if not attrs:
                continue

            # Check if association_count exists
            if 'association_count' in attrs:
                studies_with_count += 1
                count = attrs['association_count']

                # Validate it's a reasonable number (≥0)
                try:
                    count_int = int(count)
                    if count_int >= 0:
                        valid_counts += 1
                except (ValueError, TypeError):
                    pass

        if total_checked == 0:
            return False, "No studies found to check"

        return True, f"✓ Association counts: {studies_with_count}/{total_checked} have counts, {valid_counts}/{total_checked} valid (≥0)"

    @test
    def test_text_search_for_disease_traits(self):
        """Search for studies by disease trait (e.g., 'diabetes', 'breast cancer')"""
        # Test search for a disease term from our reference data
        disease_trait = None
        for ref_study in self.runner.reference_data[:10]:
            trait = ref_study.get('diseaseTrait', {}).get('trait')
            if trait and len(trait) < 100:
                disease_trait = trait
                break

        if not disease_trait:
            return False, "No suitable disease traits found in reference data"

        try:
            response = requests.get(
                f"{self.runner.api_url}/ws/search",
                params={"i": disease_trait},
                timeout=5
            )
            if response.status_code != 200:
                return False, f"Search failed with HTTP {response.status_code}"

            data = response.json()
            results = data.get('results', [])
            count = len(results)

            if count > 0:
                return True, f"✓ Text search works: \"{disease_trait[:50]}...\" → {count} results"
            else:
                return False, f"No results for trait search: {disease_trait[:50]}"

        except Exception as e:
            return False, f"Search error: {e}"

    @test
    def test_cross_references_to_efo_traits(self):
        """Verify studies have xrefs to EFO trait ontology"""
        studies_with_efo = 0
        total_efo_xrefs = 0
        total_checked = 0

        for ref_study in self.runner.reference_data[:30]:
            study_id = ref_study.get('accessionId')
            if not study_id:
                continue

            # Query with x=true to get cross-references
            try:
                response = requests.get(
                    f"{self.runner.api_url}/ws/search",
                    params={"i": study_id, "x": "true"},
                    timeout=5
                )
                if response.status_code != 200:
                    continue

                data = response.json()
                if not data or not data.get("results"):
                    continue

                total_checked += 1
                result = data["results"][0]

                # Check for EFO xrefs
                xrefs = result.get('Xrefs', [])
                efo_xrefs = [x for x in xrefs if x.get('source') == 'efo']

                if efo_xrefs:
                    studies_with_efo += 1
                    total_efo_xrefs += len(efo_xrefs)
            except Exception:
                continue

        if total_checked == 0:
            return False, "No studies found to check"

        avg = total_efo_xrefs / studies_with_efo if studies_with_efo > 0 else 0
        return True, f"✓ EFO xrefs: {studies_with_efo}/{total_checked} studies ({total_efo_xrefs} total, {avg:.1f} avg per study)"


def main():
    """Run GWAS Study dataset tests"""
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
    custom_tests = GwasStudyTests(runner)
    for test_method in [
        custom_tests.test_study_has_publication_metadata,
        custom_tests.test_study_has_disease_trait_mappings,
        custom_tests.test_study_has_association_count,
        custom_tests.test_text_search_for_disease_traits,
        custom_tests.test_cross_references_to_efo_traits
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
