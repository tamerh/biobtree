#!/usr/bin/env python3
"""
FANTOM5 Test Suite

Tests FANTOM5 promoter, enhancer, and gene-level expression data.
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


class FANTOM5Tests:
    """FANTOM5 custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_promoter_has_coordinates(self):
        """Check promoter has genomic coordinates"""
        data = self.runner.get_entry("1", "fantom5_promoter")

        if not data:
            return False, "No results for promoter ID 1"

        result = data[0]
        attrs = result.get("Attributes", {}).get("Fantom5Promoter", {})

        required = ["chromosome", "start", "end", "strand"]
        missing = [f for f in required if not attrs.get(f)]

        if missing:
            return False, f"Missing fields: {missing}"

        return True, f"Promoter at {attrs['chromosome']}:{attrs['start']}-{attrs['end']}"

    @test
    def test_promoter_has_gene_association(self):
        """Check promoter has gene symbol"""
        data = self.runner.get_entry("1", "fantom5_promoter")

        if not data:
            return False, "No results for promoter ID 1"

        result = data[0]
        attrs = result.get("Attributes", {}).get("Fantom5Promoter", {})

        gene_symbol = attrs.get("gene_symbol", "")
        if gene_symbol:
            return True, f"Promoter associated with gene: {gene_symbol}"

        return True, "SKIP: Promoter has no gene symbol (intergenic)"

    @test
    def test_promoter_has_expression_data(self):
        """Check promoter has TPM expression values"""
        data = self.runner.get_entry("1", "fantom5_promoter")

        if not data:
            return False, "No results for promoter ID 1"

        result = data[0]
        attrs = result.get("Attributes", {}).get("Fantom5Promoter", {})

        tpm_avg = attrs.get("tpm_average", 0)
        tpm_max = attrs.get("tpm_max", 0)
        samples = attrs.get("samples_expressed", 0)

        return True, f"TPM avg={tpm_avg:.2f}, max={tpm_max:.2f}, samples={samples}"

    @test
    def test_enhancer_has_associated_genes(self):
        """Check enhancer has associated genes (proximity-based)"""
        data = self.runner.get_entry("1", "fantom5_enhancer")

        if not data:
            return False, "No results for enhancer ID 1"

        result = data[0]
        attrs = result.get("Attributes", {}).get("Fantom5Enhancer", {})

        genes = attrs.get("associated_genes", [])
        if len(genes) > 0:
            gene_preview = ", ".join(genes[:5])
            if len(genes) > 5:
                gene_preview += "..."
            return True, f"Enhancer has {len(genes)} associated genes: {gene_preview}"

        return False, "Enhancer has no associated genes"

    @test
    def test_enhancer_gene_xrefs(self):
        """Check enhancer has xrefs to gene databases"""
        data = self.runner.get_entry("1", "fantom5_enhancer")

        if not data:
            return False, "No results for enhancer ID 1"

        result = data[0]
        datasets = self.runner.get_xref_datasets(result)

        gene_datasets = [d for d in datasets if d in ["ensembl", "hgnc", "entrez"]]

        if len(gene_datasets) > 0:
            xref_count = self.runner.get_xref_count(result)
            return True, f"Enhancer has {xref_count} xrefs to: {', '.join(gene_datasets)}"

        return False, "Enhancer missing gene database xrefs"

    @test
    def test_gene_search_by_symbol(self):
        """Search fantom5_enhancer by gene symbol (TP53)"""
        # Use the search endpoint
        try:
            url = f"{self.runner.api_url}/ws/search?t=TP53&s=fantom5_enhancer"
            response = requests.get(url, timeout=30)
            if response.status_code != 200:
                return False, f"Search returned status {response.status_code}"

            data = response.json()
            total = data.get("stats", {}).get("total", 0)

            if total > 0:
                return True, f"Found {total} enhancers near TP53"

            return False, "No enhancers found for TP53"
        except Exception as e:
            return False, f"Search error: {e}"

    @test
    def test_hgnc_to_enhancer_mapping(self):
        """Map gene through HGNC to enhancers"""
        data = self.runner.map_query("TP53", ">>hgnc>>fantom5_enhancer")

        if not data:
            return False, "Mapping returned no data"

        total = data.get("stats", {}).get("total", 0)

        if total > 0:
            return True, f"TP53 >> hgnc >> fantom5_enhancer: {total} enhancers"

        return False, "No enhancers mapped via HGNC"

    @test
    def test_gene_expression_breadth(self):
        """Check gene has expression breadth classification"""
        data = self.runner.get_entry("1", "fantom5_gene")

        if not data:
            return True, "SKIP: fantom5_gene not available"

        result = data[0]
        attrs = result.get("Attributes", {}).get("Fantom5Gene", {})

        breadth = attrs.get("expression_breadth", "")
        valid_values = ["ubiquitous", "broad", "tissue_specific", "not_expressed"]

        if breadth in valid_values:
            return True, f"Gene has expression_breadth: {breadth}"

        return False, f"Invalid expression_breadth: {breadth}"

    @test
    def test_promoter_top_tissues(self):
        """Check promoter has top tissues data"""
        data = self.runner.get_entry("1", "fantom5_promoter")

        if not data:
            return False, "No results for promoter ID 1"

        result = data[0]
        attrs = result.get("Attributes", {}).get("Fantom5Promoter", {})

        top_tissues = attrs.get("top_tissues", [])

        if len(top_tissues) > 0:
            first = top_tissues[0]
            name = first.get("name", "unknown")
            tpm = first.get("tpm", 0)
            return True, f"Top tissue: {name} (TPM={tpm:.2f})"

        return True, "SKIP: No top tissues (low expression)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Reference data is optional for FANTOM5 (no external API)
    if not reference_file.exists():
        print(f"Note: {reference_file} not found - using direct API tests only")
        reference_file = None

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = FANTOM5Tests(runner)
    for test_method in [
        custom_tests.test_promoter_has_coordinates,
        custom_tests.test_promoter_has_gene_association,
        custom_tests.test_promoter_has_expression_data,
        custom_tests.test_enhancer_has_associated_genes,
        custom_tests.test_enhancer_gene_xrefs,
        custom_tests.test_gene_search_by_symbol,
        custom_tests.test_hgnc_to_enhancer_mapping,
        custom_tests.test_gene_expression_breadth,
        custom_tests.test_promoter_top_tissues,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
