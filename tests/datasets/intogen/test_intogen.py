#!/usr/bin/env python3
"""
intOGen Test Suite

Validates the somatic cancer driver-gene catalog: gene entries keyed by symbol,
the gene hub (HGNC/Ensembl), the consensus ROLE (oncogene/tumor-suppressor),
and disease (MONDO) edges that power cancer -> driver-gene routes.
"""

import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class IntogenTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _genes(self):
        ref = self.runner.reference_data
        return ref.get("genes", []) if isinstance(ref, dict) else []

    def _entry(self, sym):
        """Return the intogen result entry (with its xrefs) for a gene symbol."""
        data = self.runner.lookup(sym)
        if not data:
            return None
        for r in data.get("results", []):
            if r.get("dataset_name") == "intogen":
                return r
        return None

    @test
    def test_gene_entry_exists(self):
        """A driver gene is retrievable by symbol"""
        genes = self._genes()
        if not genes:
            return False, "No genes in reference data"
        sym = genes[0]["symbol"]
        if self._entry(sym) is None:
            return False, f"No intogen entry for {sym}"
        return True, f"Driver gene {sym} found"

    @test
    def test_gene_maps_to_hgnc(self):
        """Driver gene maps to HGNC (gene hub)"""
        genes = self._genes()
        if not genes:
            return False, "No genes in reference data"
        sym = genes[0]["symbol"]
        e = self._entry(sym)
        if e is None or not self.runner.has_xref(e, "hgnc"):
            return False, f"{sym} has no hgnc xref"
        return True, f"{sym} -> hgnc OK"

    @test
    def test_gene_maps_to_ensembl(self):
        """Driver gene maps to Ensembl"""
        genes = self._genes()
        if not genes:
            return False, "No genes in reference data"
        sym = genes[0]["symbol"]
        e = self._entry(sym)
        if e is None or not self.runner.has_xref(e, "ensembl"):
            return False, f"{sym} has no ensembl xref"
        return True, f"{sym} -> ensembl OK"

    @test
    def test_role_attribute(self):
        """Entry carries a consensus ROLE matching the reference (Act/LoF/ambiguous)"""
        genes = self._genes()
        if not genes:
            return False, "No genes in reference data"
        g = genes[0]
        e = self._entry(g["symbol"])
        if e is None:
            return False, f"No intogen entry for {g['symbol']}"
        attrs = e.get("Attributes", {}) or e.get("attributes", {})
        ig = attrs.get("Intogen") or attrs.get("intogen") or {}
        role = ig.get("role")
        if not role:
            return False, f"No role attribute for {g['symbol']}"
        if role != g["role"]:
            return False, f"{g['symbol']} role {role} != reference {g['role']}"
        return True, f"{g['symbol']} role = {role}"

    @test
    def test_gene_maps_to_mondo(self):
        """Driver gene maps to a disease (MONDO) — powers cancer -> driver routes"""
        genes = self._genes()
        g = next((g for g in genes if g.get("cancer_types")), None)
        if not g:
            return False, "No gene with cancer types in reference"
        e = self._entry(g["symbol"])
        if e is None or not self.runner.has_xref(e, "mondo"):
            return False, f"{g['symbol']} has no mondo xref"
        return True, f"{g['symbol']} -> mondo OK"


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
    custom = IntogenTests(runner)

    for m in [
        custom.test_gene_entry_exists,
        custom.test_gene_maps_to_hgnc,
        custom.test_gene_maps_to_ensembl,
        custom.test_role_attribute,
        custom.test_gene_maps_to_mondo,
    ]:
        runner.add_custom_test(m)

    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
