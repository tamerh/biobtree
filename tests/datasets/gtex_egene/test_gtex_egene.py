#!/usr/bin/env python3
"""
GTEx eGene (v10 gene-level) test suite.

Gene-keyed (ENSG) GTEx summary: one entry per gene holds its per-tissue
eGene (eQTL) / sGene (sQTL) call — is_egene, q-value, lead variant, slope.
This is the gene-anchored view the variant-keyed `gtex` dataset cannot be
aggregated into, reachable as ensembl >> gtex_egene.

Data under test is the hand-crafted fixture
(tests/datasets/gtex_egene/gtex_egene_fixture.tsv), NOT real GTEx data.
"""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner, test


def _rows(data, attr_name):
    return [r for r in (data or {}).get("results", []) if attr_name in (r.get("Attributes") or {})]


class GtexEgeneTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _genes(self):
        ref = self.runner.reference_data
        return ref.get("genes", []) if isinstance(ref, dict) else []

    def _attr(self, gid):
        for r in _rows(self.runner.lookup(gid), "GtexEgene"):
            if r.get("identifier") == gid:
                return r["Attributes"]["GtexEgene"]
        return None

    @test
    def test_gene_lookup(self):
        """An ENSG id resolves to a gene-level GTEx record with tissues"""
        for g in self._genes():
            a = self._attr(g["id"])
            if a is not None:
                tis = a.get("tissues") or []
                if not tis:
                    return False, f"{g['id']} has no tissues"
                return True, f"{g['id']} ({a.get('gene_name')}) -> {len(tis)} tissue(s)"
        return False, "No gene resolved to a GTEx eGene record"

    @test
    def test_per_tissue_egene_call(self):
        """TP53: 3 tissue rows, 2 significant (is_egene), both QTL types kept"""
        g = next((x for x in self._genes() if x["id"] == "ENSG00000141510"), None)
        a = self._attr(g["id"])
        if a is None:
            return False, f"{g['id']} not found"
        tis = a.get("tissues") or []
        if len(tis) != g["n_tissues"]:
            return False, f"expected {g['n_tissues']} tissue rows, got {len(tis)}"
        if a.get("gene_name") != g["gene_name"]:
            return False, f"gene_name {a.get('gene_name')} != {g['gene_name']}"
        egenes = [t for t in tis if t.get("is_egene")]
        if len(egenes) != g["egene_count"]:
            return False, f"expected {g['egene_count']} significant eGene rows, got {len(egenes)}"
        types = {t.get("qtl_type") for t in tis}
        for q in g["qtl_types"]:
            if q not in types:
                return False, f"qtl_type {q} missing from {types}"
        return True, f"TP53 -> {len(tis)} tissues, {len(egenes)} significant, types={sorted(types)}"

    @test
    def test_lead_variant_and_slope_sign(self):
        """A significant row carries its lead variant key and a signed slope"""
        a = self._attr("ENSG00000141510")
        if a is None:
            return False, "TP53 not found"
        sq = next((t for t in (a.get("tissues") or []) if t.get("qtl_type") == "sqtl"), None)
        if sq is None:
            return False, "no sqtl tissue row for TP53"
        if not (sq.get("lead_variant") or "").count(":") == 3:
            return False, f"lead_variant not a chr:pos:ref:alt key: {sq.get('lead_variant')!r}"
        if not (sq.get("slope") or "").startswith("-"):
            return False, f"negative sQTL slope not preserved: {sq.get('slope')!r}"
        return True, f"sqtl lead={sq.get('lead_variant')} slope={sq.get('slope')} (sign kept)"

    @test
    def test_reachable_from_ensembl(self):
        """ensembl >> gtex_egene resolves the gene's GTEx record"""
        r = requests.get(f"{self.api}/ws/map/", params={"i": "ENSG00000141510", "m": ">>gtex_egene"}, timeout=15).json()
        for res in r.get("results", []):
            for t in res.get("targets", []):
                if t.get("dataset_name") == "gtex_egene" and t.get("identifier") == "ENSG00000141510":
                    return True, "ensembl ENSG00000141510 >> gtex_egene resolved"
        return False, f"ensembl >> gtex_egene did not resolve: {str(r)[:160]}"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'),
                        script_dir / "reference_data.json")
    c = GtexEgeneTests(runner)
    for m in [c.test_gene_lookup, c.test_per_tissue_egene_call,
              c.test_lead_variant_and_slope_sign, c.test_reachable_from_ensembl]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
