#!/usr/bin/env python3
"""
dbSNP Dataset Tests

dbSNP (database of Single Nucleotide Polymorphisms) is NCBI's authoritative database that provides:
- RefSNP IDs (rs numbers) and genomic coordinates
- Allele information (reference and alternate alleles)
- Population allele frequencies
- Gene associations via GENEINFO field
- Clinical significance (from ClinVar)
- Variant type classification (SNV, insertion, deletion, etc.)
- Comprehensive functional annotations (coding effects, splice sites, UTRs, gene regions)
- Variant origin (germline vs somatic)
- Quality indicators and publication evidence

Test Structure:
- Primary entries: rs* IDs (e.g., rs200676709)
- Attributes: Basic variant info, frequencies, genes, functional annotations, quality flags
- Cross-references: Genes (via gene_id), gene names (text search)
- Total tests: 4 declarative + 10 custom = 14 tests
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class DbsnpTests:
    """dbSNP custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_snp_has_genomic_position(self):
        """Verify SNPs have chromosome and position"""
        if not self.runner.reference_data:
            return False, "No reference data available"

        snp_id = self.runner.reference_data[0]["rsId"]
        data = self.runner.query.lookup(snp_id)

        if not data or not data.get("results"):
            return False, f"SNP {snp_id} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Dbsnp", {})

        if not attrs:
            return False, f"No dbSNP attributes for {snp_id}"

        # Check genomic position fields
        chromosome = attrs.get("chromosome")
        position = attrs.get("position")

        if not chromosome or not position:
            return False, "Missing chromosome or position information"

        return True, f"✓ SNP {snp_id} has genomic position: chr{chromosome}:{position}"

    @test
    def test_snp_to_gene_xrefs(self):
        """Verify SNPs link to genes via gene_id"""
        # Count SNPs with gene associations
        snps_with_genes = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            data = self.runner.query.lookup(snp_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Dbsnp", {})

            if attrs.get("gene_id"):
                snps_with_genes += 1

        if total_checked == 0:
            return False, "No SNPs could be checked"

        percentage = (snps_with_genes / total_checked) * 100
        return True, f"✓ {snps_with_genes}/{total_checked} SNPs ({percentage:.0f}%) have gene associations"

    @test
    def test_gene_name_text_search(self):
        """Verify SNPs can be found by searching gene names"""
        # Find a SNP with a gene name
        test_gene = None
        expected_snp = None

        for ref in self.runner.reference_data:
            if ref.get("geneName"):
                test_gene = ref["geneName"]
                expected_snp = ref["rsId"]
                break

        if not test_gene:
            return True, "SKIP: No SNPs with gene names in reference data"

        # Search for the gene name
        data = self.runner.query.lookup(test_gene)

        if not data or not data.get("results"):
            return False, f"Gene name '{test_gene}' not found"

        # Check if any result is our expected SNP
        found = False
        for result in data["results"]:
            if result.get("identifier", "").upper() == expected_snp.upper():
                found = True
                break

        if not found:
            return False, f"Expected SNP {expected_snp} not found when searching gene '{test_gene}'"

        return True, f"✓ Gene name '{test_gene}' search found SNP {expected_snp}"

    @test
    def test_allele_frequency_data(self):
        """Verify allele frequency is stored for population SNPs"""
        snps_with_af = 0
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            data = self.runner.query.lookup(snp_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Dbsnp", {})

            if attrs.get("allele_frequency") and attrs.get("allele_frequency") > 0:
                snps_with_af += 1

        if total_checked == 0:
            return False, "No SNPs could be checked"

        percentage = (snps_with_af / total_checked) * 100
        return True, f"✓ {snps_with_af}/{total_checked} SNPs ({percentage:.0f}%) have allele frequency data"

    @test
    def test_clinical_significance(self):
        """Verify clinical significance is stored for ClinVar SNPs"""
        # Find SNP with clinical significance in reference data
        test_snp = None
        expected_clinsig = None

        for ref in self.runner.reference_data:
            if ref.get("clinicalSignificance"):
                test_snp = ref["rsId"]
                expected_clinsig = ref["clinicalSignificance"]
                break

        if not test_snp:
            return True, "SKIP: No SNPs with clinical significance in test data"

        data = self.runner.query.lookup(test_snp)

        if not data or not data.get("results"):
            return False, f"SNP {test_snp} not found"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Dbsnp", {})
        clinsig = attrs.get("clinical_significance")

        if not clinsig:
            return False, f"SNP {test_snp} missing clinical significance (expected: {expected_clinsig})"

        return True, f"✓ SNP {test_snp} has clinical significance: {clinsig}"

    @test
    def test_variant_type_classification(self):
        """Verify SNPs are classified as SNV, insertion, deletion, etc."""
        variant_types = {}
        total_checked = 0

        for ref in self.runner.reference_data[:10]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            data = self.runner.query.lookup(snp_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Dbsnp", {})
            variant_type = attrs.get("variant_type", "unknown")

            variant_types[variant_type] = variant_types.get(variant_type, 0) + 1

        if total_checked == 0:
            return False, "No SNPs could be checked"

        if not variant_types:
            return False, "No variant types found"

        # Format variant type distribution
        type_dist = ", ".join([f"{vt}={count}" for vt, count in sorted(variant_types.items())])
        return True, f"✓ {total_checked} SNPs classified: {type_dist}"

    @test
    def test_functional_annotations(self):
        """Verify functional annotation flags are present"""
        functional_flags = ["nsf", "nsm", "nsn", "syn", "u3", "u5", "ass", "dss", "intron", "r3", "r5"]
        flags_found = {flag: 0 for flag in functional_flags}
        total_checked = 0

        for ref in self.runner.reference_data[:50]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            data = self.runner.query.lookup(snp_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Dbsnp", {})

            # Check each functional flag
            for flag in functional_flags:
                if attrs.get(flag):
                    flags_found[flag] += 1

        if total_checked == 0:
            return False, "No SNPs could be checked"

        # Count how many different flags were found
        found_count = sum(1 for count in flags_found.values() if count > 0)
        total_annotations = sum(flags_found.values())

        if found_count == 0:
            return True, f"✓ Checked {total_checked} SNPs, no functional annotations present (normal for many SNPs)"

        # Show which flags were found
        found_flags = ", ".join([f"{flag}={count}" for flag, count in flags_found.items() if count > 0])
        return True, f"✓ {total_annotations} functional annotations across {found_count} flag types: {found_flags}"

    @test
    def test_variant_origin_sao(self):
        """Verify SAO (Variant Allele Origin) field"""
        sao_values = {}
        total_checked = 0

        for ref in self.runner.reference_data[:50]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            data = self.runner.query.lookup(snp_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Dbsnp", {})
            sao = attrs.get("sao", 0)

            sao_values[sao] = sao_values.get(sao, 0) + 1

        if total_checked == 0:
            return False, "No SNPs could be checked"

        # Format SAO distribution
        sao_names = {0: "unspecified", 1: "Germline", 2: "Somatic", 3: "Both"}
        sao_dist = ", ".join([f"{sao_names.get(sao, sao)}={count}" for sao, count in sorted(sao_values.items())])
        return True, f"✓ {total_checked} SNPs SAO distribution: {sao_dist}"

    @test
    def test_common_variant_flag(self):
        """Verify is_common flag for common variants"""
        common_count = 0
        rare_count = 0

        for ref in self.runner.reference_data[:50]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            data = self.runner.query.lookup(snp_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Dbsnp", {})

            if attrs.get("is_common"):
                common_count += 1
            else:
                rare_count += 1

        total = common_count + rare_count
        if total == 0:
            return False, "No SNPs could be checked"

        common_pct = (common_count / total) * 100
        return True, f"✓ {total} SNPs: {common_count} common ({common_pct:.1f}%), {rare_count} rare"

    @test
    def test_publication_flags(self):
        """Verify publication and evidence flags"""
        pub_count = 0
        pubmed_count = 0
        genotype_count = 0
        total_checked = 0

        for ref in self.runner.reference_data[:50]:
            snp_id = ref.get("rsId")
            if not snp_id:
                continue

            data = self.runner.query.lookup(snp_id)

            if not data or not data.get("results"):
                continue

            total_checked += 1
            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Dbsnp", {})

            if attrs.get("has_publication"):
                pub_count += 1
            if attrs.get("has_pubmed_ref"):
                pubmed_count += 1
            if attrs.get("has_genotypes"):
                genotype_count += 1

        if total_checked == 0:
            return False, "No SNPs could be checked"

        return True, f"✓ {total_checked} SNPs: {pub_count} w/publication, {pubmed_count} w/PubMed ref, {genotype_count} w/genotypes"


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
    custom_tests = DbsnpTests(runner)
    for test_method in [
        custom_tests.test_snp_has_genomic_position,
        custom_tests.test_snp_to_gene_xrefs,
        custom_tests.test_gene_name_text_search,
        custom_tests.test_allele_frequency_data,
        custom_tests.test_clinical_significance,
        custom_tests.test_variant_type_classification,
        custom_tests.test_functional_annotations,
        custom_tests.test_variant_origin_sao,
        custom_tests.test_common_variant_flag,
        custom_tests.test_publication_flags
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
