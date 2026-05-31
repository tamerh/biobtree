#!/usr/bin/env python3
"""
ClinGen Variant Pathogenicity Test Suite

Validates VCEP ACMG variant interpretations: lookup by Allele Registry CA id,
the ClinVar bridge (clingen_variant -> clinvar, which inherits dbSNP/gene/disease
links), gene edges and gene-symbol text search.
"""

import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class ClingenVariantTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _variants(self):
        ref = self.runner.reference_data
        return ref.get("variants", []) if isinstance(ref, dict) else []

    def _entry(self, ca_id):
        data = self.runner.lookup(ca_id)
        if not data:
            return None
        for r in data.get("results", []):
            if r.get("dataset_name") == "clingen_variant":
                return r
        return None

    @test
    def test_entry_exists(self):
        """A variant is retrievable by its Allele Registry CA id"""
        vs = self._variants()
        if not vs:
            return False, "No variants in reference data"
        ca = vs[0]["ca_id"]
        if self._entry(ca) is None:
            return False, f"No clingen_variant entry for {ca}"
        return True, f"Variant {ca} ({vs[0].get('gene_symbol','')}) found"

    @test
    def test_maps_to_clinvar(self):
        """Variant bridges to ClinVar via its ClinVar Variation Id"""
        vs = self._variants()
        for v in vs[:15]:
            if not v.get("clinvar_id"):
                continue
            e = self._entry(v["ca_id"])
            if e and self.runner.has_xref(e, "clinvar"):
                return True, f"{v['ca_id']} -> clinvar {v['clinvar_id']} OK"
        return False, "No sampled variant bridged to clinvar"

    @test
    def test_maps_to_gene(self):
        """Variant maps to hgnc/entrez/ensembl via its gene symbol"""
        vs = self._variants()
        for v in vs[:15]:
            e = self._entry(v["ca_id"])
            if e and any(self.runner.has_xref(e, ds) for ds in ("hgnc", "entrez", "ensembl")):
                return True, f"{v['ca_id']} -> gene OK"
        return False, "No sampled variant mapped to a gene dataset"

    @test
    def test_gene_symbol_text_search(self):
        """A variant's gene symbol resolves via text search"""
        vs = self._variants()
        sym = next((v["gene_symbol"] for v in vs if len(v.get("gene_symbol", "")) >= 3), None)
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
    custom = ClingenVariantTests(runner)

    for m in [
        custom.test_entry_exists,
        custom.test_maps_to_clinvar,
        custom.test_maps_to_gene,
        custom.test_gene_symbol_text_search,
    ]:
        runner.add_custom_test(m)

    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
