#!/usr/bin/env python3
"""
ClinGen Gene-Disease Validity Test Suite

Validates curated gene-disease validity assertions: lookup by assertion id,
gene edges (hgnc/entrez/ensembl), disease edges (mondo) and gene-symbol text
search. Each assertion carries the classification (Definitive..Refuted) + MOI.
"""

import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class ClingenGeneValidityTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _curations(self):
        ref = self.runner.reference_data
        return ref.get("curations", []) if isinstance(ref, dict) else []

    def _entry(self, assertion_id):
        data = self.runner.lookup(assertion_id)
        if not data:
            return None
        for r in data.get("results", []):
            if r.get("dataset_name") == "clingen_gene_validity":
                return r
        return None

    @test
    def test_entry_exists(self):
        """A validity assertion is retrievable by its assertion id"""
        cur = self._curations()
        if not cur:
            return False, "No curations in reference data"
        aid = cur[0]["assertion_id"]
        if self._entry(aid) is None:
            return False, f"No clingen_gene_validity entry for {aid}"
        return True, f"Assertion {cur[0]['gene_symbol']} ({aid}) found"

    @test
    def test_maps_to_gene(self):
        """Assertion maps to hgnc/entrez/ensembl"""
        cur = self._curations()
        for c in cur[:15]:
            e = self._entry(c["assertion_id"])
            if e and any(self.runner.has_xref(e, ds) for ds in ("hgnc", "entrez", "ensembl")):
                return True, f"{c['assertion_id']} -> gene OK"
        return False, "No sampled assertion mapped to a gene dataset"

    @test
    def test_maps_to_mondo(self):
        """Assertion maps to its MONDO disease"""
        cur = self._curations()
        for c in cur[:15]:
            e = self._entry(c["assertion_id"])
            if e and self.runner.has_xref(e, "mondo"):
                return True, f"{c['assertion_id']} -> mondo OK"
        return False, "No sampled assertion mapped to mondo"

    @test
    def test_gene_symbol_text_search(self):
        """A curated gene symbol resolves via text search"""
        cur = self._curations()
        sym = next((c["gene_symbol"] for c in cur if len(c.get("gene_symbol", "")) >= 3), None)
        if not sym:
            return False, "No suitable gene symbol in reference"
        data = self.runner.lookup(sym)
        if not data or not data.get("results"):
            return False, f"No results for symbol {sym}"
        return True, f"Text search found '{sym}'"


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
    custom = ClingenGeneValidityTests(runner)

    for m in [
        custom.test_entry_exists,
        custom.test_maps_to_gene,
        custom.test_maps_to_mondo,
        custom.test_gene_symbol_text_search,
    ]:
        runner.add_custom_test(m)

    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
