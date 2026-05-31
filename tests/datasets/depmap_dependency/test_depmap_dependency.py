#!/usr/bin/env python3
"""
DepMap Dependency Test Suite

Validates per-cell-line dependency edges: gene -> depmap_dependency, and the
bridge depmap_dependency -> cellosaurus (cell lines depending on the gene).
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


class DepmapDependencyTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _genes(self):
        ref = self.runner.reference_data
        return ref.get("genes", []) if isinstance(ref, dict) else []

    @test
    def test_gene_has_dependencies(self):
        """A canonical gene has per-cell-line dependency edges"""
        for g in self._genes():
            if _mapped(self.runner.map_query(g["gene_id"], ">>entrez>>depmap_dependency", mode="lite")) > 0:
                return True, f"{g['gene_symbol']} -> depmap_dependency OK"
        return False, "No curated gene has depmap_dependency edges"

    @test
    def test_bridges_to_cellosaurus(self):
        """Dependency edges bridge to the cell line in Cellosaurus"""
        for g in self._genes():
            if _mapped(self.runner.map_query(g["gene_id"], ">>entrez>>depmap_dependency>>cellosaurus", mode="lite")) > 0:
                return True, f"{g['gene_symbol']} -> depmap_dependency -> cellosaurus OK"
        return False, "No dependency edge bridged to cellosaurus"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = DepmapDependencyTests(runner)
    for m in [custom.test_gene_has_dependencies, custom.test_bridges_to_cellosaurus]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
