#!/usr/bin/env python3
"""
GeneRIF Test Suite

Validates NCBI GeneRIF cited functional claims: gene -> generif claims, and the
generif -> pubmed citation edge. Reached via the gene (Entrez).
"""

import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


def _mapped(data):
    if not data:
        return 0
    st = data.get("stats", {})
    if st.get("mapped"):
        return st["mapped"]
    return sum(len(m.get("targets", [])) for m in data.get("mappings", []))


class GenerifTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _genes(self):
        ref = self.runner.reference_data
        return ref.get("genes", []) if isinstance(ref, dict) else []

    @test
    def test_gene_has_generif(self):
        """A gene maps to its GeneRIF claims"""
        for g in self._genes()[:15]:
            if _mapped(self.runner.map_query(g["gene_id"], ">>entrez>>generif", mode="lite")) > 0:
                return True, f"gene {g['gene_id']} -> generif OK"
        return False, "No sampled gene mapped to generif"

    @test
    def test_generif_to_pubmed(self):
        """GeneRIF claims carry their PubMed citation"""
        for g in self._genes()[:15]:
            if _mapped(self.runner.map_query(g["gene_id"], ">>entrez>>generif>>pubmed", mode="lite")) > 0:
                return True, f"gene {g['gene_id']} -> generif -> pubmed OK"
        return False, "No sampled gene reached pubmed via generif"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        print("Run: python3 extract_reference_data.py")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = GenerifTests(runner)
    for m in [custom.test_gene_has_generif, custom.test_generif_to_pubmed]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
