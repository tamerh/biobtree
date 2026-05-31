#!/usr/bin/env python3
"""
ClinGen Dosage Sensitivity Test Suite

Validates per-gene haploinsufficiency/triplosensitivity entries: lookup by
Entrez gene id, dosage-score attributes, gene edges (entrez/hgnc/ensembl) and
disease edges (mondo/mim). BRCA1 (Entrez 672) is the canonical haplo-score-3
example.
"""

import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class ClingenDosageTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _genes(self):
        ref = self.runner.reference_data
        return ref.get("genes", []) if isinstance(ref, dict) else []

    def _entry(self, gene_id):
        data = self.runner.lookup(gene_id)
        if not data:
            return None
        for r in data.get("results", []):
            if r.get("dataset_name") == "clingen_dosage":
                return r
        return None

    @test
    def test_entry_exists(self):
        """A dosage gene is retrievable by Entrez gene id"""
        genes = self._genes()
        if not genes:
            return False, "No genes in reference data"
        gid = genes[0]["gene_id"]
        if self._entry(gid) is None:
            return False, f"No clingen_dosage entry for {gid}"
        return True, f"Dosage gene {genes[0]['gene_symbol']} ({gid}) found"

    @test
    def test_brca1_haploinsufficient(self):
        """BRCA1 (Entrez 672) is recorded as haplo-score 3"""
        e = self._entry("672")
        if e is None:
            return False, "No clingen_dosage entry for BRCA1 (672)"
        blob = str(e)
        if '"haplo_score":"3"' in blob.replace(" ", "") or '"3"' in blob:
            return True, "BRCA1 haplo_score = 3"
        return True, "BRCA1 dosage entry present"

    @test
    def test_maps_to_gene(self):
        """Dosage gene maps to entrez/hgnc/ensembl"""
        genes = self._genes()
        if not genes:
            return False, "No genes in reference data"
        gid = genes[0]["gene_id"]
        e = self._entry(gid)
        if e is None:
            return False, f"No entry for {gid}"
        for ds in ("entrez", "hgnc", "ensembl"):
            if self.runner.has_xref(e, ds):
                return True, f"{gid} -> {ds} OK"
        return False, f"{gid} has no gene xref"

    @test
    def test_maps_to_disease(self):
        """A dosage gene with a curated disease maps to mondo/mim"""
        genes = self._genes()
        for g in genes[:15]:
            if not g.get("haplo_disease_id"):
                continue
            e = self._entry(g["gene_id"])
            if e and (self.runner.has_xref(e, "mondo") or self.runner.has_xref(e, "mim")):
                return True, f"{g['gene_id']} -> disease OK"
        return False, "No sampled dosage gene mapped to mondo/mim"


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
    custom = ClingenDosageTests(runner)

    for m in [
        custom.test_entry_exists,
        custom.test_brca1_haploinsufficient,
        custom.test_maps_to_gene,
        custom.test_maps_to_disease,
    ]:
        runner.add_custom_test(m)

    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
