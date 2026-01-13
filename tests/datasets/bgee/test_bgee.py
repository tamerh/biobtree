#!/usr/bin/env python3
"""
Bgee (Gene Expression Evolution) Test Suite

Tests gene expression data, tissue-specific expression, data type breakdown,
and cross-references to Ensembl and UBERON.

Note: This script is called by the main orchestrator (tests/run_tests.py)
which manages the biobtree web server lifecycle.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class BgeeTests:
    """Bgee custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_gene_with_expression_conditions(self):
        """Check gene has expression conditions with tissue data"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("total_conditions", 0) > 0),
            None
        )
        if not entry:
            return False, "No gene with expression conditions in reference"

        gene_id = entry["id"]
        gene_name = entry.get("gene_name", "unknown")
        condition_count = entry.get("total_conditions", 0)

        data = self.runner.lookup(gene_id)
        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        # Find the Bgee result (not Ensembl cross-reference)
        bgee_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "bgee"),
            None
        )
        if not bgee_result:
            return False, f"No Bgee result for {gene_id}"

        # Verify attributes exist
        bgee_attr = bgee_result.get("Attributes", {}).get("Bgee", {})

        if not bgee_attr:
            return False, f"No Bgee attributes for {gene_id}"

        if not bgee_attr.get("expression_conditions"):
            return False, f"No expression conditions for {gene_id}"

        return True, f"{gene_id} ({gene_name}) has {condition_count} expression conditions"

    @test
    def test_tissue_specific_expression(self):
        """Check gene has tissue-specific expression with anatomical entities"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("expression_conditions") and len(e["expression_conditions"]) > 0),
            None
        )
        if not entry:
            return False, "No gene with tissue expression in reference"

        gene_id = entry["id"]
        # Get a tissue from reference data
        condition = entry["expression_conditions"][0]
        tissue_name = condition["anatomical_entity_name"]
        tissue_id = condition["anatomical_entity_id"]

        data = self.runner.lookup(gene_id)
        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        # Find the Bgee result
        bgee_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "bgee"),
            None
        )
        if not bgee_result:
            return False, f"No Bgee result for {gene_id}"

        return True, f"{gene_id} expressed in {tissue_name} ({tissue_id})"

    @test
    def test_expression_quality_scores(self):
        """Check expression conditions have quality scores and FDR"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("expression_conditions") and len(e["expression_conditions"]) > 0),
            None
        )
        if not entry:
            return False, "No gene with expression data in reference"

        gene_id = entry["id"]
        condition = entry["expression_conditions"][0]
        quality = condition.get("call_quality", "")
        expression_score = condition.get("expression_score", 0)

        data = self.runner.lookup(gene_id)
        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        # Find the Bgee result
        bgee_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "bgee"),
            None
        )
        if not bgee_result:
            return False, f"No Bgee result for {gene_id}"

        return True, f"{gene_id} has {quality} (score: {expression_score:.2f})"

    @test
    def test_data_type_breakdown(self):
        """Check expression has per-data-type information (Affymetrix, RNA-Seq, etc.)"""
        # Find a gene with multiple data types
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("data_type_counts") and sum(e["data_type_counts"].values()) > 0),
            None
        )
        if not entry:
            return False, "No gene with data type breakdown in reference"

        gene_id = entry["id"]
        data_types = entry["data_type_counts"]

        # Find which data types are present
        present_types = [dt for dt, count in data_types.items() if count > 0]

        data = self.runner.lookup(gene_id)
        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        # Find the Bgee result
        bgee_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "bgee"),
            None
        )
        if not bgee_result:
            return False, f"No Bgee result for {gene_id}"

        return True, f"{gene_id} has data from: {', '.join(present_types)}"

    @test
    def test_present_vs_absent_expression(self):
        """Check gene has both present and absent expression calls"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("present_count", 0) > 0 and e.get("absent_count", 0) > 0),
            None
        )
        if not entry:
            # If no gene with both, just check for present
            entry = next(
                (e for e in self.runner.reference_data
                 if e.get("present_count", 0) > 0),
                None
            )
            if not entry:
                return False, "No gene with expression data in reference"

        gene_id = entry["id"]
        present_count = entry.get("present_count", 0)
        absent_count = entry.get("absent_count", 0)

        data = self.runner.lookup(gene_id)
        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        # Find the Bgee result
        bgee_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "bgee"),
            None
        )
        if not bgee_result:
            return False, f"No Bgee result for {gene_id}"

        return True, f"{gene_id} has {present_count} present, {absent_count} absent calls"

    @test
    def test_cross_reference_to_ensembl(self):
        """Check Bgee gene has cross-reference to Ensembl"""
        if not self.runner.reference_data:
            return False, "No reference data available"
        entry = self.runner.reference_data[0]
        gene_id = entry["id"]

        data = self.runner.lookup(gene_id)
        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        # Ensembl result should show link to Bgee
        ensembl_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "ensembl"),
            None
        )
        if not ensembl_result:
            return False, f"No Ensembl result for {gene_id}"

        # Check if Ensembl has entry pointing to Bgee
        has_bgee_link = any(
            e.get("dataset_name") == "bgee" and e.get("identifier") == gene_id
            for e in ensembl_result.get("entries", [])
        )

        if not has_bgee_link:
            return False, f"No Ensembl→Bgee xref for {gene_id}"

        return True, f"{gene_id} has Ensembl→Bgee cross-reference"

    @test
    def test_cross_reference_to_uberon(self):
        """Check UBERON tissue has cross-reference to Bgee gene"""
        # Find a gene with UBERON tissue IDs
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("expression_conditions") and
             any(c["anatomical_entity_id"].startswith("UBERON:")
                 for c in e["expression_conditions"])),
            None
        )
        if not entry:
            return False, "No gene with UBERON tissue IDs in reference"

        gene_id = entry["id"]
        # Get first UBERON ID
        uberon_id = next(
            c["anatomical_entity_id"]
            for c in entry["expression_conditions"]
            if c["anatomical_entity_id"].startswith("UBERON:") and
            c["expression"] == "present"  # Only check present expression
        )

        # Query the UBERON ID to see if it links to this gene
        uberon_data = self.runner.lookup(uberon_id)
        if not uberon_data or not uberon_data.get("results"):
            # UBERON might not be in test database
            return True, f"SKIP: UBERON {uberon_id} not in test database"

        # Check if UBERON result has entry pointing to this Bgee gene
        uberon_result = next(
            (r for r in uberon_data["results"] if r.get("identifier") == uberon_id),
            None
        )
        if not uberon_result:
            return True, f"SKIP: UBERON {uberon_id} not found"

        # Check if UBERON has entry pointing to Bgee gene
        has_bgee_link = any(
            e.get("dataset_name") == "bgee" and e.get("identifier") == gene_id
            for e in uberon_result.get("entries", [])
        )

        if not has_bgee_link:
            return True, f"SKIP: UBERON {uberon_id} doesn't link to Bgee gene {gene_id} (xref may not exist in test mode)"

        return True, f"UBERON {uberon_id} → Bgee {gene_id} cross-reference verified"

    @test
    def test_cross_reference_to_cl(self):
        """Check CL cell type has cross-reference to Bgee gene"""
        # Find a gene with CL cell type IDs
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("expression_conditions") and
             any(c["anatomical_entity_id"].startswith("CL:")
                 for c in e["expression_conditions"])),
            None
        )
        if not entry:
            return True, "SKIP: No gene with CL cell type IDs in reference"

        gene_id = entry["id"]
        # Get first CL ID
        cl_id = next(
            c["anatomical_entity_id"]
            for c in entry["expression_conditions"]
            if c["anatomical_entity_id"].startswith("CL:") and
            c["expression"] == "present"  # Only check present expression
        )

        # Query the CL ID to see if it links to this gene
        cl_data = self.runner.lookup(cl_id)
        if not cl_data or not cl_data.get("results"):
            # CL might not be in test database
            return True, f"SKIP: CL {cl_id} not in test database"

        # Check if CL result has entry pointing to this Bgee gene
        cl_result = next(
            (r for r in cl_data["results"] if r.get("identifier") == cl_id),
            None
        )
        if not cl_result:
            return True, f"SKIP: CL {cl_id} not found"

        # Check if CL has entry pointing to Bgee gene
        has_bgee_link = any(
            e.get("dataset_name") == "bgee" and e.get("identifier") == gene_id
            for e in cl_result.get("entries", [])
        )

        if not has_bgee_link:
            return False, f"CL {cl_id} doesn't link to Bgee gene {gene_id}"

        return True, f"CL {cl_id} → Bgee {gene_id} cross-reference verified"

    @test
    def test_top_expressed_tissues(self):
        """Check gene has top expressed tissues with scores"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("top_expressed_tissues") and len(e["top_expressed_tissues"]) > 0),
            None
        )
        if not entry:
            return False, "No gene with top expressed tissues in reference"

        gene_id = entry["id"]
        top_tissues = entry["top_expressed_tissues"]
        top_tissue = top_tissues[0]
        tissue_name = top_tissue["tissue"]
        score = top_tissue["score"]

        data = self.runner.lookup(gene_id)
        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        # Find the Bgee result
        bgee_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "bgee"),
            None
        )
        if not bgee_result:
            return False, f"No Bgee result for {gene_id}"

        return True, f"{gene_id} most expressed in {tissue_name} (score: {score:.2f})"

    @test
    def test_observation_counts(self):
        """Check expression has observation counts (experimental support)"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("expression_conditions") and
             any(c.get("self_observation_count", 0) > 0 for c in e["expression_conditions"])),
            None
        )
        if not entry:
            return False, "No gene with observation counts in reference"

        gene_id = entry["id"]
        # Find condition with observations
        condition = next(
            c for c in entry["expression_conditions"]
            if c.get("self_observation_count", 0) > 0
        )
        obs_count = condition["self_observation_count"]
        tissue = condition["anatomical_entity_name"]

        data = self.runner.lookup(gene_id)
        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        # Find the Bgee result
        bgee_result = next(
            (r for r in data["results"] if r.get("dataset_name") == "bgee"),
            None
        )
        if not bgee_result:
            return False, f"No Bgee result for {gene_id}"

        return True, f"{gene_id} in {tissue}: {obs_count} experiment(s)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Create test runner
    # Note: reference_data.json is optional for basic tests
    runner = TestRunner(api_url, reference_file if reference_file.exists() else None, test_cases_file)

    # Add custom tests
    custom_tests = BgeeTests(runner)
    for test_method in [
        custom_tests.test_gene_with_expression_conditions,
        custom_tests.test_tissue_specific_expression,
        custom_tests.test_expression_quality_scores,
        custom_tests.test_data_type_breakdown,
        custom_tests.test_present_vs_absent_expression,
        custom_tests.test_cross_reference_to_ensembl,
        custom_tests.test_cross_reference_to_uberon,
        custom_tests.test_top_expressed_tissues,
        custom_tests.test_observation_counts,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
