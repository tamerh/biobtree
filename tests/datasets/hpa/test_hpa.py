#!/usr/bin/env python3
"""
Human Protein Atlas (HPA) Test Suite

Tests the gene-level card (hpa) plus the expression / pathology / antibody
children parsed from proteinatlas.xml.gz. Declarative cases live in
test_cases.json; this script adds a couple of custom checks.

Called by the main orchestrator (tests/run_tests.py), which manages the
biobtree web server lifecycle.
"""

import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test  # noqa: E402


class HpaTests:
    """HPA custom tests (in addition to the declarative test_cases.json)."""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_gene_card_has_subcellular(self):
        """hpa gene card exposes subcellular location or a tissue-specificity call."""
        res = self.runner.lookup("ENSG00000000003")
        if not res:
            return False, "ENSG00000000003 not found"
        for r in res.get("results", []):
            attr = r.get("source", {}).get("Attributes", {}).get("Hpa")
            if attr:
                if attr.get("subcellular_main") or attr.get("rna_tissue_specificity"):
                    return True, "subcellular / specificity present"
                return False, "Hpa attr missing subcellular and specificity"
        return False, "no Hpa attribute on ENSG00000000003"

    @test
    def test_expression_reachable(self):
        """ENSG -> hpa_expression returns per-tissue expression entities."""
        res = self.runner.map_query("ENSG00000000003", ">>hpa>>hpa_expression")
        if not res:
            return False, "no hpa_expression mapping"
        return True, "hpa_expression reachable"


def main():
    api_url = os.environ.get("BIOBTREE_API_URL", "http://localhost:9292")
    here = Path(__file__).parent
    reference_file = here / "reference_data.json"
    test_cases_file = here / "test_cases.json"

    runner = TestRunner(
        api_url,
        reference_file if reference_file.exists() else None,
        test_cases_file,
    )

    custom = HpaTests(runner)
    for m in [custom.test_gene_card_has_subcellular, custom.test_expression_reachable]:
        runner.add_custom_test(m)

    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
