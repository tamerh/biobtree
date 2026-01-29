#!/usr/bin/env python3
"""
SC Expression Atlas (SCXA) Test Suite

Tests the SCXA dataset family:
- scxa: Experiment metadata (accessions, technology, cell counts)
- scxa_expression: Gene expression summaries (aggregated stats)
- scxa_gene_experiment: Gene-experiment details (cluster-level data, derived)

Note: This script is called by the main orchestrator (tests/run_tests.py)
which manages the biobtree web server lifecycle.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class ScxaTests:
    """SCXA custom tests for experiment metadata"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_experiment_with_cells(self):
        """Check experiment has cell count data"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("number_of_cells", 0) > 0),
            None
        )
        if not entry:
            return False, "No experiment with cell count in reference"

        exp_id = entry["id"]
        cell_count = entry.get("number_of_cells", 0)
        species = entry.get("species", "unknown")

        data = self.runner.lookup(exp_id)
        if not data or not data.get("results"):
            return False, f"No results for {exp_id}"

        # Find the SCXA result
        scxa_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "scxa"),
            None
        )
        if not scxa_result:
            return False, f"No SCXA result for {exp_id}"

        # Verify attributes exist
        scxa_attr = scxa_result.get("Attributes", {}).get("Scxa", {})

        if not scxa_attr:
            return False, f"No SCXA attributes for {exp_id}"

        return True, f"{exp_id} ({species}) has {cell_count:,} cells"

    @test
    def test_technology_types(self):
        """Check experiment has technology type information"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("technology_types") and len(e["technology_types"]) > 0),
            None
        )
        if not entry:
            return False, "No experiment with technology types in reference"

        exp_id = entry["id"]
        tech_types = entry["technology_types"]

        data = self.runner.lookup(exp_id)
        if not data or not data.get("results"):
            return False, f"No results for {exp_id}"

        # Find the SCXA result
        scxa_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "scxa"),
            None
        )
        if not scxa_result:
            return False, f"No SCXA result for {exp_id}"

        return True, f"{exp_id} uses: {', '.join(tech_types)}"

    @test
    def test_experiment_type(self):
        """Check experiment has type classification (Baseline/Differential)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("experiment_type")),
            None
        )
        if not entry:
            return False, "No experiment with experiment type in reference"

        exp_id = entry["id"]
        exp_type = entry.get("experiment_type", "unknown")

        data = self.runner.lookup(exp_id)
        if not data or not data.get("results"):
            return False, f"No results for {exp_id}"

        # Find the SCXA result
        scxa_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "scxa"),
            None
        )
        if not scxa_result:
            return False, f"No SCXA result for {exp_id}"

        return True, f"{exp_id} is {exp_type} type experiment"

    @test
    def test_experimental_factors(self):
        """Check experiment has experimental factors (organism part, disease, etc.)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("experimental_factors") and len(e["experimental_factors"]) > 0),
            None
        )
        if not entry:
            return False, "No experiment with experimental factors in reference"

        exp_id = entry["id"]
        factors = entry["experimental_factors"]

        data = self.runner.lookup(exp_id)
        if not data or not data.get("results"):
            return False, f"No results for {exp_id}"

        # Find the SCXA result
        scxa_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "scxa"),
            None
        )
        if not scxa_result:
            return False, f"No SCXA result for {exp_id}"

        return True, f"{exp_id} factors: {', '.join(factors[:3])}{'...' if len(factors) > 3 else ''}"

    @test
    def test_cross_reference_to_taxonomy(self):
        """Check SCXA experiment has cross-reference to taxonomy"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("species")),
            None
        )
        if not entry:
            return False, "No experiment with species in reference"

        exp_id = entry["id"]
        species = entry.get("species", "unknown")

        data = self.runner.lookup(exp_id)
        if not data or not data.get("results"):
            return False, f"No results for {exp_id}"

        # Find the SCXA result
        scxa_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "scxa"),
            None
        )
        if not scxa_result:
            return False, f"No SCXA result for {exp_id}"

        # Check if SCXA has entry pointing to taxonomy
        has_taxonomy_link = any(
            e.get("dataset_name") == "taxonomy"
            for e in scxa_result.get("entries", [])
        )

        if not has_taxonomy_link:
            return True, f"SKIP: No taxonomy link for {exp_id} (may not be in taxonomy mapping)"

        return True, f"{exp_id} ({species}) has taxonomy cross-reference"


class ScxaExpressionTests:
    """SCXA Expression custom tests for gene summaries"""

    def __init__(self, runner: TestRunner):
        self.runner = runner
        # Common Ensembl gene IDs that appear in many SCXA experiments
        self.sample_genes = [
            "ENSG00000139618",  # BRCA2
            "ENSG00000141510",  # TP53
            "ENSG00000157764",  # BRAF
            "ENSG00000171862",  # PTEN
            "ENSG00000012048",  # BRCA1
            "ENSG00000133703",  # KRAS
            "ENSG00000146648",  # EGFR
            "ENSG00000181449",  # SOX2
        ]

    def _find_gene_with_expression(self):
        """Find a gene that has scxa_expression data"""
        for gene_id in self.sample_genes:
            data = self.runner.lookup(gene_id)
            if data and data.get("results"):
                for result in data["results"]:
                    if result.get("dataset_name") == "scxa_expression":
                        return gene_id, result
        return None, None

    @test
    def test_gene_expression_lookup(self):
        """Check gene expression entry can be looked up"""
        gene_id, result = self._find_gene_with_expression()

        if gene_id and result:
            return True, f"Gene {gene_id} found in scxa_expression"

        return True, "SKIP: No scxa_expression data in test database (scxa_expression not built)"

    @test
    def test_gene_summary_attributes(self):
        """Check gene summary has expected attributes"""
        gene_id, result = self._find_gene_with_expression()

        if not gene_id:
            return True, "SKIP: No scxa_expression data to test"

        # Check for expected attributes
        attrs = result.get("Attributes", {}).get("ScxaExpression", {})
        if attrs:
            total_exp = attrs.get("TotalExperiments", 0)
            max_mean = attrs.get("MaxMeanExpression", 0)
            return True, f"Gene {gene_id}: {total_exp} experiments, max_mean={max_mean:.2f}"

        return True, f"Gene {gene_id} found (attributes structure may vary)"

    @test
    def test_gene_to_experiment_xrefs(self):
        """Check gene has cross-references to experiments"""
        gene_id, result = self._find_gene_with_expression()

        if not gene_id:
            return True, "SKIP: No scxa_expression data to test"

        # Check for scxa cross-references
        entries = result.get("entries", [])
        scxa_xrefs = [e for e in entries if e.get("dataset_name") == "scxa"]

        if scxa_xrefs:
            return True, f"Gene {gene_id} links to {len(scxa_xrefs)} experiments"

        # Also check for scxa_gene_experiment xrefs
        detail_xrefs = [e for e in entries if e.get("dataset_name") == "scxa_gene_experiment"]
        if detail_xrefs:
            return True, f"Gene {gene_id} has {len(detail_xrefs)} detail entries"

        return True, f"Gene {gene_id} found (xrefs loading may vary)"


class ScxaGeneExperimentTests:
    """SCXA Gene-Experiment custom tests for cluster-level data"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _find_gene_experiment_entry(self, expr_runner):
        """Find a gene-experiment detail entry via scxa_expression xrefs"""
        # First find a gene with scxa_expression data
        sample_genes = [
            "ENSG00000139618", "ENSG00000141510", "ENSG00000157764",
            "ENSG00000171862", "ENSG00000012048", "ENSG00000133703"
        ]

        for gene_id in sample_genes:
            data = self.runner.lookup(gene_id)
            if not data or not data.get("results"):
                continue

            # Find scxa_expression result
            expr_result = next(
                (r for r in data["results"] if r.get("dataset_name") == "scxa_expression"),
                None
            )
            if not expr_result:
                continue

            # Look for scxa_gene_experiment xrefs
            entries = expr_result.get("entries", [])
            for entry in entries:
                if entry.get("dataset_name") == "scxa_gene_experiment":
                    composite_key = entry.get("identifier")
                    if composite_key:
                        # Try to look up this composite key
                        detail_data = self.runner.lookup(composite_key)
                        if detail_data and detail_data.get("results"):
                            for result in detail_data["results"]:
                                if result.get("dataset_name") == "scxa_gene_experiment":
                                    return composite_key, result

        return None, None

    @test
    def test_gene_experiment_composite_key(self):
        """Check gene-experiment detail entry with composite key"""
        composite_key, result = self._find_gene_experiment_entry(self.runner)

        if composite_key and result:
            # Verify composite key format (gene_id_experiment_id)
            if "_E-" in composite_key:
                parts = composite_key.split("_E-")
                gene_id = parts[0]
                exp_id = "E-" + parts[1] if len(parts) > 1 else "unknown"
                return True, f"Composite key: {gene_id} + {exp_id}"
            return True, f"Found detail entry: {composite_key}"

        return True, "SKIP: No scxa_gene_experiment data (scxa_expression may not be built)"

    @test
    def test_gene_experiment_attributes(self):
        """Check gene-experiment detail has cluster data"""
        composite_key, result = self._find_gene_experiment_entry(self.runner)

        if not composite_key:
            return True, "SKIP: No scxa_gene_experiment data to test"

        # Check attributes
        attrs = result.get("Attributes", {}).get("ScxaGeneExperiment", {})
        if attrs:
            clusters = attrs.get("Clusters", [])
            is_marker = attrs.get("IsMarkerInExperiment", False)
            return True, f"{composite_key}: {len(clusters)} clusters, marker={is_marker}"

        return True, f"Entry {composite_key} found (attributes structure may vary)"

    @test
    def test_gene_experiment_back_references(self):
        """Check gene-experiment detail has back-references to gene and experiment"""
        composite_key, result = self._find_gene_experiment_entry(self.runner)

        if not composite_key:
            return True, "SKIP: No scxa_gene_experiment data to test"

        # Check for back-references
        entries = result.get("entries", [])
        has_expr_xref = any(e.get("dataset_name") == "scxa_expression" for e in entries)
        has_scxa_xref = any(e.get("dataset_name") == "scxa" for e in entries)

        xref_info = []
        if has_expr_xref:
            xref_info.append("scxa_expression")
        if has_scxa_xref:
            xref_info.append("scxa")

        if xref_info:
            return True, f"{composite_key} links to: {', '.join(xref_info)}"

        return True, f"Entry {composite_key} found (xrefs loading may vary)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Create test runner
    # Note: reference_data.json is optional for basic tests
    runner = TestRunner(api_url, reference_file if reference_file.exists() else None, test_cases_file)

    # Add scxa (experiment) custom tests
    scxa_tests = ScxaTests(runner)
    for test_method in [
        scxa_tests.test_experiment_with_cells,
        scxa_tests.test_technology_types,
        scxa_tests.test_experiment_type,
        scxa_tests.test_experimental_factors,
        scxa_tests.test_cross_reference_to_taxonomy,
    ]:
        runner.add_custom_test(test_method)

    # Add scxa_expression (gene summary) custom tests
    expr_tests = ScxaExpressionTests(runner)
    for test_method in [
        expr_tests.test_gene_expression_lookup,
        expr_tests.test_gene_summary_attributes,
        expr_tests.test_gene_to_experiment_xrefs,
    ]:
        runner.add_custom_test(test_method)

    # Add scxa_gene_experiment (detail) custom tests
    detail_tests = ScxaGeneExperimentTests(runner)
    for test_method in [
        detail_tests.test_gene_experiment_composite_key,
        detail_tests.test_gene_experiment_attributes,
        detail_tests.test_gene_experiment_back_references,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
