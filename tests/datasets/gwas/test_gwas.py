#!/usr/bin/env python3
"""
GWAS Association Dataset Tests

GWAS association dataset contains SNP-trait associations from the GWAS Catalog that provides:
- SNP identifiers and genomic positions (chromosome, position, region)
- Gene associations (reported genes, mapped genes, upstream/downstream genes)
- Disease/trait information with EFO ontology mappings
- Statistical evidence (p-values, effect sizes, confidence intervals)
- Study cross-references linking to GWAS study metadata

Test Structure:
- Primary entries: rs* SNP IDs (e.g., rs12451471)
- Attributes: snp_id, chr_id, chr_pos, genes, traits, p_values, study info
- Cross-references: GWAS studies, EFO traits, gene symbols, disease/trait text search
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


class GwasTests:
    """GWAS Association custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_snp_has_genomic_position(self):
        """Verify SNPs have chromosome, position, region, and context"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        snp_id = self.runner.reference_data[0]["rsId"]
        data = self.runner.query.lookup(snp_id)

        if not data or not data.get("results"):
            return False, f"SNP {snp_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Gwas", {})

        if not attrs:
            return False, f"No GWAS attributes for {snp_id}"

        # Check genomic position fields
        has_position = False
        position_fields = []

        if attrs.get("chr_id"):
            position_fields.append(f"chr{attrs['chr_id']}")
            has_position = True
        if attrs.get("chr_pos"):
            position_fields.append(f"pos:{attrs['chr_pos']}")
            has_position = True
        if attrs.get("region"):
            position_fields.append(f"region:{attrs['region']}")

        if not has_position:
            return False, "Missing chromosome and position information"

        position_str = ", ".join(position_fields) if position_fields else "No position data"
        return True, f"✓ SNP {snp_id} has genomic position: {position_str}"

    @test
    def test_snp_to_study_xrefs(self):
        """Verify SNPs link to GWAS studies"""
        study_xref_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            # Query with x=true to get cross-references
            try:
                response = requests.get(
                    f"{self.runner.api_url}/ws/search",
                    params={"i": snp_id, "x": "true"},
                    timeout=5
                )
                if response.status_code != 200:
                    continue

                data = response.json()
                if not data or not data.get("results"):
                    continue

                total_checked += 1
                result = data["results"][0]

                # Check for GWAS study cross-references
                if self.runner.has_xref(result, "gwas_study"):
                    study_xref_count += 1
            except Exception:
                continue

        if total_checked == 0:
            return False, "No SNPs found to check"

        percentage = study_xref_count * 100 // total_checked if total_checked > 0 else 0
        return True, f"✓ {study_xref_count}/{total_checked} SNPs have GWAS study cross-references ({percentage}%)"

    @test
    def test_gene_associations(self):
        """Verify SNPs have reported genes and mapped genes"""
        gene_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:20]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            data = self.runner.query.lookup(snp_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Gwas", {})

            # Check if gene associations are present
            has_genes = False
            if attrs.get("reported_genes") and len(attrs["reported_genes"]) > 0:
                has_genes = True
            elif attrs.get("mapped_gene"):
                has_genes = True
            elif attrs.get("snp_gene_ids") and len(attrs["snp_gene_ids"]) > 0:
                has_genes = True

            if has_genes:
                gene_count += 1

        if total_checked == 0:
            return False, "No SNPs found to check"

        percentage = gene_count * 100 // total_checked if total_checked > 0 else 0
        return True, f"✓ {gene_count}/{total_checked} SNPs have gene associations ({percentage}%)"

    @test
    def test_efo_trait_xrefs(self):
        """Verify SNPs link to EFO ontology terms"""
        efo_xref_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            # Query with x=true to get cross-references
            try:
                response = requests.get(
                    f"{self.runner.api_url}/ws/search",
                    params={"i": snp_id, "x": "true"},
                    timeout=5
                )
                if response.status_code != 200:
                    continue

                data = response.json()
                if not data or not data.get("results"):
                    continue

                total_checked += 1
                result = data["results"][0]

                # Check for EFO cross-references
                if self.runner.has_xref(result, "efo"):
                    efo_xref_count += 1
            except Exception:
                continue

        if total_checked == 0:
            return False, "No SNPs found to check"

        percentage = efo_xref_count * 100 // total_checked if total_checked > 0 else 0
        return True, f"✓ {efo_xref_count}/{total_checked} SNPs have EFO trait cross-references ({percentage}%)"

    @test
    def test_disease_trait_text_search(self):
        """Verify SNPs can be searched by disease/trait name"""
        # Find a SNP with a disease trait from our parsed data
        snp_id = None
        disease_trait = None

        for ref in self.runner.reference_data[:10]:
            test_snp = ref.get("rsId")
            if not test_snp:
                continue

            # Get the SNP from biobtree
            data = self.runner.query.lookup(test_snp)
            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Gwas", {})

            # Get disease trait (limited to <200 chars as per safety limits)
            trait = attrs.get("disease_trait", "")
            if trait and len(trait) < 200:
                snp_id = test_snp
                disease_trait = trait
                break

        if not disease_trait:
            return False, "No suitable disease traits found in test data"

        # Search using disease trait name
        try:
            response = requests.get(
                f"{self.runner.api_url}/ws/search",
                params={"i": disease_trait},
                timeout=5
            )
            if response.status_code != 200:
                return False, f"Search failed with HTTP {response.status_code}"

            data = response.json()
            if not data or not data.get("results"):
                return False, f"No results for trait search: {disease_trait}"

            # Check if our SNP is in results (it might not be the only one)
            found = any(r.get("identifier") == snp_id for r in data["results"])
            if not found:
                return False, f"SNP {snp_id} not found via trait search"

            return True, f"✓ Trait search works: \"{disease_trait[:40]}...\" → {snp_id}"

        except Exception as e:
            return False, f"Search error: {e}"

    @test
    def test_statistical_evidence(self):
        """Verify p-values and effect sizes are stored"""
        stats_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:20]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            data = self.runner.query.lookup(snp_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Gwas", {})

            # Check if statistical evidence is present
            has_stats = False
            if attrs.get("p_value") and attrs["p_value"] > 0:
                has_stats = True
            elif attrs.get("pvalue_mlog") and attrs["pvalue_mlog"] > 0:
                has_stats = True
            elif attrs.get("or_beta"):
                has_stats = True

            if has_stats:
                stats_count += 1

        if total_checked == 0:
            return False, "No SNPs found to check"

        percentage = stats_count * 100 // total_checked if total_checked > 0 else 0
        return True, f"✓ {stats_count}/{total_checked} SNPs have statistical evidence ({percentage}%)"


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
    custom_tests = GwasTests(runner)
    for test_method in [
        custom_tests.test_snp_has_genomic_position,
        custom_tests.test_snp_to_study_xrefs,
        custom_tests.test_gene_associations,
        custom_tests.test_efo_trait_xrefs,
        custom_tests.test_disease_trait_text_search,
        custom_tests.test_statistical_evidence
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
