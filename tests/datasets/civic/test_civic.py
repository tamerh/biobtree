#!/usr/bin/env python3
"""
CIViC (Clinical Interpretation of Variants in Cancer) Test Suite

Validates the somatic-cancer integration: gene/variant/evidence/assertion
entries, the gene hub (HGNC/Entrez/Ensembl), disease (DOID -> MONDO) edges,
and therapy (ChEMBL) druggability edges.
"""

import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class CivicTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _genes(self):
        ref = self.runner.reference_data
        return ref.get("genes", []) if isinstance(ref, dict) else []

    def _evidence(self):
        ref = self.runner.reference_data
        return ref.get("evidence", []) if isinstance(ref, dict) else []

    @test
    def test_gene_entry_exists(self):
        """A CIViC gene feature is retrievable by feature_id"""
        genes = self._genes()
        if not genes:
            return False, "No genes in reference data"
        fid = genes[0]["feature_id"]
        data = self.runner.lookup(fid)
        if not data or not data.get("results"):
            return False, f"No results for feature_id {fid}"
        return True, f"Gene {genes[0]['name']} (feature {fid}) found"

    @test
    def test_gene_text_search(self):
        """Gene symbol text search returns the CIViC gene"""
        genes = self._genes()
        sym = next((g["name"] for g in genes if len(g.get("name", "")) >= 3), None)
        if not sym:
            return False, "No suitable gene symbol in reference"
        data = self.runner.lookup(sym)
        if not data or not data.get("results"):
            return False, f"No results for symbol {sym}"
        return True, f"Text search found gene '{sym}'"

    @test
    def test_gene_maps_to_hgnc(self):
        """CIViC gene maps to HGNC (gene hub)"""
        genes = self._genes()
        if not genes:
            return False, "No genes in reference data"
        fid = genes[0]["feature_id"]
        if not self.runner.query.has_path(fid, "hgnc"):
            return False, f"feature {fid} did not map to hgnc"
        return True, f"feature {fid} -> hgnc OK"

    @test
    def test_gene_maps_to_entrez(self):
        """CIViC gene maps to Entrez via its authoritative entrez_id"""
        genes = self._genes()
        if not genes:
            return False, "No genes in reference data"
        fid = genes[0]["feature_id"]
        if not self.runner.query.has_path(fid, "entrez"):
            return False, f"feature {fid} did not map to entrez"
        return True, f"feature {fid} -> entrez OK"

    @test
    def test_evidence_exists(self):
        """A CIViC evidence item is retrievable by evidence_id"""
        ev = self._evidence()
        if not ev:
            return False, "No evidence in reference data"
        eid = ev[0]["evidence_id"]
        data = self.runner.lookup(eid)
        if not data or not data.get("results"):
            return False, f"No results for evidence_id {eid}"
        return True, f"Evidence {eid} found ({ev[0]['disease']})"

    @test
    def test_evidence_maps_to_doid(self):
        """Evidence item links to its DOID disease (DOID -> MONDO bridge)"""
        ev = self._evidence()
        if not ev:
            return False, "No evidence in reference data"
        eid = ev[0]["evidence_id"]
        if not self.runner.query.has_path(eid, "doid"):
            return False, f"evidence {eid} did not map to doid"
        return True, f"evidence {eid} -> doid OK"


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
    custom = CivicTests(runner)

    for m in [
        custom.test_gene_entry_exists,
        custom.test_gene_text_search,
        custom.test_gene_maps_to_hgnc,
        custom.test_gene_maps_to_entrez,
        custom.test_evidence_exists,
        custom.test_evidence_maps_to_doid,
    ]:
        runner.add_custom_test(m)

    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
