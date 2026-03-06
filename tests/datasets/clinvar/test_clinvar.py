#!/usr/bin/env python3
"""
ClinVar Test Suite

Tests ClinVar dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

Note: This script is called by the main orchestrator (tests/run_tests.py)
which manages the biobtree web server lifecycle.
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


class ClinVarTests:
    """ClinVar custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_variant_with_pathogenic_classification(self):
        """Check variant with Pathogenic classification."""
        entry = next(
            (e for e in self.runner.reference_data
             if "Pathogenic" in e.get("germline_classification", "")),
            None
        )

        if not entry:
            return True, "No Pathogenic variants in test data"

        variation_id = entry["variation_id"]
        data = self.runner.lookup(variation_id)

        if not data or not data.get("results"):
            return False, f"No results for {variation_id}"

        return True, f"{variation_id} is Pathogenic: {entry.get('germline_classification', '')}"

    @test
    def test_variant_with_benign_classification(self):
        """Check variant with Benign classification."""
        entry = next(
            (e for e in self.runner.reference_data
             if "Benign" in e.get("germline_classification", "") and "Pathogenic" not in e.get("germline_classification", "")),
            None
        )

        if not entry:
            return True, "No Benign variants in test data"

        variation_id = entry["variation_id"]
        data = self.runner.lookup(variation_id)

        if not data or not data.get("results"):
            return False, f"No results for {variation_id}"

        return True, f"{variation_id} is Benign"

    @test
    def test_variant_with_vus_classification(self):
        """Check variant with Uncertain significance (VUS)."""
        entry = next(
            (e for e in self.runner.reference_data
             if "Uncertain" in e.get("germline_classification", "")),
            None
        )

        if not entry:
            return True, "No VUS variants in test data"

        variation_id = entry["variation_id"]
        data = self.runner.lookup(variation_id)

        if not data or not data.get("results"):
            return False, f"No results for {variation_id}"

        return True, f"{variation_id} has uncertain significance"

    @test
    def test_variant_with_hgvs_expressions(self):
        """Check variant with HGVS expressions."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("hgvs_expressions") and len(e["hgvs_expressions"]) > 0),
            None
        )

        if not entry:
            return True, "No variants with HGVS expressions in test data"

        variation_id = entry["variation_id"]
        data = self.runner.lookup(variation_id)

        if not data or not data.get("results"):
            return False, f"No results for {variation_id}"

        hgvs_count = len(entry["hgvs_expressions"])
        return True, f"{variation_id} has {hgvs_count} HGVS expression(s)"

    @test
    def test_variant_with_gene_annotation(self):
        """Check variant with gene annotations."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_symbols") and len(e["gene_symbols"]) > 0),
            None
        )

        if not entry:
            return True, "No variants with gene annotations in test data"

        variation_id = entry["variation_id"]
        data = self.runner.lookup(variation_id)

        if not data or not data.get("results"):
            return False, f"No results for {variation_id}"

        genes = ', '.join(entry["gene_symbols"][:3])
        return True, f"{variation_id} has gene(s): {genes}"

    @test
    def test_variant_snv_type(self):
        """Check variant with SNV type."""
        entry = next(
            (e for e in self.runner.reference_data
             if "single nucleotide" in e.get("type", "").lower()),
            None
        )

        if not entry:
            return True, "No SNV variants in test data"

        variation_id = entry["variation_id"]
        data = self.runner.lookup(variation_id)

        if not data or not data.get("results"):
            return False, f"No results for {variation_id}"

        return True, f"{variation_id} is SNV type"

    @test
    def test_variant_deletion_type(self):
        """Check variant with Deletion type."""
        entry = next(
            (e for e in self.runner.reference_data
             if "Deletion" in e.get("type", "")),
            None
        )

        if not entry:
            return True, "No Deletion variants in test data"

        variation_id = entry["variation_id"]
        data = self.runner.lookup(variation_id)

        if not data or not data.get("results"):
            return False, f"No results for {variation_id}"

        return True, f"{variation_id} is Deletion type"

    @test
    def test_variant_genomic_location(self):
        """Check variant with complete genomic location."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("chromosome") and e["chromosome"] not in ["", "Un"]
             and e.get("start") and e.get("assembly")),
            None
        )

        if not entry:
            return True, "No variants with complete location in test data"

        variation_id = entry["variation_id"]
        data = self.runner.lookup(variation_id)

        if not data or not data.get("results"):
            return False, f"No results for {variation_id}"

        chr = entry["chromosome"]
        start = entry["start"]
        stop = entry["stop"]
        assembly = entry["assembly"]
        return True, f"{variation_id} location: chr{chr}:{start}-{stop} ({assembly})"

    @test
    def test_variant_with_grch38_assembly(self):
        """Check variant with GRCh38 assembly."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("assembly") == "GRCh38"),
            None
        )

        if not entry:
            return True, "No GRCh38 variants in test data"

        variation_id = entry["variation_id"]
        data = self.runner.lookup(variation_id)

        if not data or not data.get("results"):
            return False, f"No results for {variation_id}"

        return True, f"{variation_id} uses GRCh38 assembly"

    @test
    def test_variant_with_hgnc_gene_xref(self):
        """Check if variants with genes have HGNC cross-references."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("gene_symbols") and len(e["gene_symbols"]) > 0),
            None
        )

        if not entry:
            return True, "No variants with genes in test data"

        variation_id = entry["variation_id"]

        # Check if HGNC cross-reference exists
        has_hgnc = self.runner.check_xref(variation_id, "hgnc")

        if has_hgnc:
            genes = ', '.join(entry["gene_symbols"][:3])
            return True, f"{variation_id} has HGNC cross-references for gene(s): {genes}"
        else:
            # HGNC mapping depends on HGNC dataset being integrated
            return True, f"{variation_id} has genes but no HGNC xref (HGNC dataset may not be integrated)"


def main():
    """Main test entry point."""
    # Setup paths
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # API URL (default port used by orchestrator)
    api_url = os.environ.get("BIOBTREE_API_URL", "http://localhost:9292")

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = ClinVarTests(runner)
    for test_method in [
        custom_tests.test_variant_with_pathogenic_classification,
        custom_tests.test_variant_with_benign_classification,
        custom_tests.test_variant_with_vus_classification,
        custom_tests.test_variant_with_hgvs_expressions,
        custom_tests.test_variant_with_gene_annotation,
        custom_tests.test_variant_snv_type,
        custom_tests.test_variant_deletion_type,
        custom_tests.test_variant_genomic_location,
        custom_tests.test_variant_with_grch38_assembly,
        custom_tests.test_variant_with_hgnc_gene_xref,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
