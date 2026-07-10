#!/usr/bin/env python3
"""
MaveDB Test Suite.

Validates biobtree's functional-assay evidence layer (Multiplexed Assays of
Variant Effect, CC0 — PS3/BS3-grade direct experimental evidence). Entries are
keyed by MaveDB variant URN and xref to uniprot / transcript / ensembl / hgnc
via the score-set target metadata (MAVE-HGVS).

NOTE: MavedbAttr.score is stored as a raw string (MAVE scores may be "NA"), so
it is not numerically filterable; a parallel numeric score field is a follow-up.

Data under test is the small fixture (tests/datasets/mavedb/fixture/), NOT the
full MaveDB.
"""

import sys
import os
import requests
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


def _rows(data, attr_name):
    out = []
    for r in (data or {}).get("results", []):
        if attr_name in (r.get("Attributes") or {}):
            out.append(r)
    return out


def _targets(data):
    return [t for res in (data or {}).get("results", []) for t in res.get("targets", [])]


class MavedbTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner
        self.api = runner.api_url

    def _variants(self):
        ref = self.runner.reference_data
        return ref.get("variants", []) if isinstance(ref, dict) else []

    def _score_sets(self):
        ref = self.runner.reference_data
        return ref.get("score_sets", []) if isinstance(ref, dict) else []

    @test
    def test_variant_lookup(self):
        """A MaveDB variant URN resolves to a Mavedb record with functional-score fields"""
        for v in self._variants():
            # URN contains '#', so pass via params for proper encoding
            data = requests.get(f"{self.api}/ws/", params={"i": v["id"], "d": "1"}, timeout=15).json()
            for r in _rows(data, "Mavedb"):
                a = r["Attributes"]["Mavedb"]
                for f in ("gene_symbol", "score", "score_set", "uniprot"):
                    if f not in a:
                        return False, f"{v['id']} missing field {f}"
                return True, (f"{v['id']} -> gene={a.get('gene_symbol')} "
                              f"score={a.get('score')} uniprot={a.get('uniprot')}")
        return False, "No MaveDB variant URN resolved to a Mavedb record"

    @test
    def test_values_match_fixture(self):
        """Stored gene_symbol / uniprot / score match the fixture"""
        for v in self._variants():
            data = requests.get(f"{self.api}/ws/", params={"i": v["id"], "d": "1"}, timeout=15).json()
            for r in _rows(data, "Mavedb"):
                a = r["Attributes"]["Mavedb"]
                if a.get("gene_symbol") != v["gene"]:
                    return False, f"{v['id']} gene: got {a.get('gene_symbol')}, want {v['gene']}"
                if a.get("uniprot") != v["uniprot"]:
                    return False, f"{v['id']} uniprot: got {a.get('uniprot')}, want {v['uniprot']}"
                if abs(float(a.get("score", 0)) - float(v["score"])) > 1e-9:
                    return False, f"{v['id']} score: got {a.get('score')}, want {v['score']}"
                return True, f"{v['id']} gene/uniprot/score match fixture"
        return False, "No MaveDB variant matched for value check"

    @test
    def test_uniprot_join(self):
        """A protein reaches its functional-assay variants (P63279 -> UBE2I score set)"""
        data = requests.get(f"{self.api}/ws/map/",
                            params={"i": "P63279", "m": ">>uniprot>>mavedb"}, timeout=15).json()
        tgts = _targets(data)
        if not tgts:
            return False, "P63279 >>uniprot>>mavedb returned no targets"
        genes = {(t.get("Attributes") or {}).get("Mavedb", {}).get("gene_symbol") for t in tgts}
        if "UBE2I" in genes:
            return True, f"P63279 >>uniprot>>mavedb -> {len(tgts)} UBE2I functional-score variants"
        return False, f"expected UBE2I among targets, got {sorted(g for g in genes if g)}"

    @test
    def test_cel_filter_gene(self):
        """CEL filter mavedb.gene_symbol==\"UBE2I\" selects the UBE2I score-set variants"""
        # gather UBE2I variant ids via the uniprot join, then filter them
        joined = requests.get(f"{self.api}/ws/map/",
                             params={"i": "P63279", "m": ">>uniprot>>mavedb"}, timeout=15).json()
        ids = [t.get("identifier") for t in _targets(joined) if t.get("identifier")]
        if not ids:
            return False, "no mavedb ids to filter"
        data = requests.get(f"{self.api}/ws/",
                            params={"i": ",".join(ids[:20]), "d": "1",
                                    "f": 'mavedb.gene_symbol == "UBE2I"'}, timeout=15).json()
        rows = _rows(data, "Mavedb")
        if not rows:
            return False, "gene_symbol filter returned no results"
        for r in rows:
            if r["Attributes"]["Mavedb"].get("gene_symbol") != "UBE2I":
                return False, f"filter leaked {r.get('identifier')}"
        return True, f"CEL gene_symbol filter kept {len(rows)} UBE2I variants"


    @test
    def test_per_set_license(self):
        """Each variant carries its score set's data license (CC0 vs CC BY-NC-SA)"""
        for uniprot, want in [("P63279", "CC0"), ("P38398", "CC BY-NC-SA 4.0")]:
            data = requests.get(f"{self.api}/ws/map/",
                                params={"i": uniprot, "m": ">>uniprot>>mavedb"}, timeout=15).json()
            lics = {(t.get("Attributes") or {}).get("Mavedb", {}).get("license") for t in _targets(data)}
            lics = {l for l in lics if l}
            if want not in lics:
                return False, f"{uniprot}: expected license {want!r}, got {sorted(lics)}"
        return True, "per-set license captured (UBE2I score set=CC0, BRCA1 score set=CC BY-NC-SA 4.0)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = MavedbTests(runner)
    for m in [
        custom.test_variant_lookup,
        custom.test_values_match_fixture,
        custom.test_uniprot_join,
        custom.test_cel_filter_gene,
        custom.test_per_set_license,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
