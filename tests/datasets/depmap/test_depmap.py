#!/usr/bin/env python3
"""
DepMap Test Suite

Validates the per-gene CRISPR essentiality aggregate: gene -> depmap, the
essentiality attributes (pct_dependent / common_essential), and gene edges.
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


def _targets(data):
    out = []
    for m in data.get("mappings", []) if data else []:
        out.extend(m.get("targets", []))
    return out


class DepmapTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _genes(self):
        ref = self.runner.reference_data
        return ref.get("genes", []) if isinstance(ref, dict) else []

    @test
    def test_gene_has_depmap(self):
        """A canonical gene maps to a DepMap essentiality entry"""
        for g in self._genes():
            if _mapped(self.runner.map_query(g["gene_id"], ">>depmap", mode="lite")) > 0:
                return True, f"{g['gene_symbol']} ({g['gene_id']}) -> depmap OK"
        return False, "No curated gene mapped to depmap"

    @test
    def test_essentiality_compact(self):
        """DepMap compact carries gene_symbol|pct_dependent|common_essential"""
        for g in self._genes():
            data = self.runner.map_query(g["gene_id"], ">>depmap", mode="lite")
            for t in _targets(data):
                # schema: id|gene_symbol|pct_dependent|common_essential
                if g["gene_symbol"] in str(t) and "|" in str(t):
                    return True, f"{g['gene_symbol']} compact = {t}"
        return False, "No depmap compact row with expected fields"

    @test
    def test_maps_to_gene(self):
        """DepMap entry maps back to the gene (entrez/hgnc/ensembl)"""
        for g in self._genes():
            for ds in ("entrez", "hgnc", "ensembl"):
                if _mapped(self.runner.map_query(g["gene_id"], ">>depmap>>" + ds, mode="lite")) > 0:
                    return True, f"{g['gene_symbol']} depmap -> {ds} OK"
        return False, "No depmap entry mapped to a gene dataset"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = DepmapTests(runner)
    for m in [custom.test_gene_has_depmap, custom.test_essentiality_compact, custom.test_maps_to_gene]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
