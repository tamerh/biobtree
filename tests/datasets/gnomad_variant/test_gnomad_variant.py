#!/usr/bin/env python3
"""
gnomAD v4 per-variant Test Suite.

Validates biobtree's per-variant population-frequency layer (AF / grpmax / FAF /
per-ancestry AF), keyed chr:pos:ref:alt (GRCh38) — same scheme as alphamissense
/ spliceai, and DISTINCT from the gene-level gnomad_constraint (id 800).

Data under test is the hand-crafted VCF fixture
(tests/datasets/gnomad_variant/gnomad_variant_fixture.vcf), NOT real gnomAD data.
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


class GnomadVariantTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner
        self.api = runner.api_url

    def _variants(self):
        ref = self.runner.reference_data
        return ref.get("variants", []) if isinstance(ref, dict) else []

    def _get(self, params):
        return requests.get(f"{self.api}/ws/", params=params, timeout=15).json()

    @test
    def test_variant_lookup(self):
        """chr:pos:ref:alt resolves to a GnomadVariant record with AF fields"""
        for v in self._variants():
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                a = r["Attributes"]["GnomadVariant"]
                for f in ("af", "af_grpmax", "grpmax_ancestry"):
                    if f not in a:
                        return False, f"{v['id']} missing field {f}"
                return True, (f"{v['id']} -> af={a.get('af')} grpmax={a.get('af_grpmax')} "
                              f"({a.get('grpmax_ancestry')})")
        return False, "No chr:pos:ref:alt id resolved to a GnomadVariant record"

    @test
    def test_key_scheme_chr_pos_ref_alt(self):
        """Entry ID is chr:pos:ref:alt (3 colons), NOT chr:pos"""
        for v in self._variants():
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                ident = r.get("identifier", "")
                if ident.count(":") != 3:
                    return False, f"unexpected key '{ident}' (expected chr:pos:ref:alt)"
                return True, f"key scheme OK: '{ident}' is chr:pos:ref:alt"
        return False, "No GnomadVariant record found to check key scheme"

    @test
    def test_af_values(self):
        """Stored AF / grpmax match the fixture"""
        for v in self._variants():
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                a = r["Attributes"]["GnomadVariant"]
                if abs(float(a.get("af", -1)) - float(v["af"])) > 1e-9:
                    return False, f"{v['id']} af: got {a.get('af')}, want {v['af']}"
                if a.get("grpmax_ancestry") != v["grpmax_ancestry"]:
                    return False, f"{v['id']} grpmax_ancestry: got {a.get('grpmax_ancestry')}, want {v['grpmax_ancestry']}"
                return True, f"{v['id']} af/grpmax match fixture"
        return False, "No GnomadVariant record matched for value check"

    @test
    def test_ac_an_faf99_joint_values(self):
        """AC / AN (sampling depth) and faf99_joint are populated and match the fixture"""
        for v in self._variants():
            if "ac" not in v:
                continue
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                a = r["Attributes"]["GnomadVariant"]
                if int(a.get("ac", -1)) != int(v["ac"]):
                    return False, f"{v['id']} ac: got {a.get('ac')}, want {v['ac']}"
                if int(a.get("an", -1)) != int(v["an"]):
                    return False, f"{v['id']} an: got {a.get('an')}, want {v['an']}"
                if abs(float(a.get("faf99_joint", -1)) - float(v["faf99_joint"])) > 1e-9:
                    return False, f"{v['id']} faf99_joint: got {a.get('faf99_joint')}, want {v['faf99_joint']}"
                return True, (f"{v['id']} -> ac={a.get('ac')} an={a.get('an')} "
                              f"faf99_joint={a.get('faf99_joint')}")
        return False, "No GnomadVariant record carried ac/an/faf99_joint"

    @test
    def test_per_callset_ac_an_af(self):
        """Per-callset exome/genome ac/an/af are populated and match the fixture"""
        for v in self._variants():
            if "ac_exomes" not in v:
                continue
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                a = r["Attributes"]["GnomadVariant"]
                for f in ("ac_exomes", "an_exomes", "ac_genomes", "an_genomes"):
                    if int(a.get(f, -1)) != int(v[f]):
                        return False, f"{v['id']} {f}: got {a.get(f)}, want {v[f]}"
                if abs(float(a.get("af_exomes", -1)) - float(v["af_exomes"])) > 1e-9:
                    return False, f"{v['id']} af_exomes: got {a.get('af_exomes')}, want {v['af_exomes']}"
                return True, (f"{v['id']} -> exome ac/an={a.get('ac_exomes')}/{a.get('an_exomes')} "
                              f"af_exomes={a.get('af_exomes')}; genome an={a.get('an_genomes')}")
        return False, "No GnomadVariant record carried per-callset fields"

    @test
    def test_per_callset_faf(self):
        """Per-callset FAF (faf95/faf99 exomes/genomes) populated and match the fixture"""
        for v in self._variants():
            if "faf95_exomes" not in v:
                continue
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                a = r["Attributes"]["GnomadVariant"]
                for f in ("faf95_exomes", "faf99_exomes", "faf95_genomes", "faf99_genomes"):
                    want = float(v[f])
                    got = float(a.get(f, 0) or 0)
                    if abs(got - want) > 1e-9:
                        return False, f"{v['id']} {f}: got {a.get(f)}, want {want}"
                return True, (f"{v['id']} -> faf95_exomes={a.get('faf95_exomes')} "
                              f"faf95_genomes={a.get('faf95_genomes')}")
        return False, "No GnomadVariant record carried per-callset FAF"

    @test
    def test_exome_only_af_dilution(self):
        """For an exome-only variant, joint af < af_exomes (genome AN dilutes the joint) —
        af_exomes is the correct single-callset frequency the caller should use"""
        for v in self._variants():
            if not v.get("exome_only"):
                continue
            for r in _rows(self._get({"i": v["id"], "d": "1"}), "GnomadVariant"):
                a = r["Attributes"]["GnomadVariant"]
                # genomes observed nothing (ac_genomes 0 -> af_genomes absent/0) but contribute AN
                if int(a.get("ac_genomes", 0)) != 0:
                    return False, f"{v['id']} expected ac_genomes=0"
                if int(a.get("an_genomes", 0)) <= 0:
                    return False, f"{v['id']} expected an_genomes>0 (dilution source)"
                af_joint = float(a.get("af", 0))
                af_ex = float(a.get("af_exomes", 0))
                if not (af_joint < af_ex):
                    return False, f"{v['id']} expected joint af {af_joint} < af_exomes {af_ex}"
                return True, f"{v['id']} exome-only: joint af={af_joint} < af_exomes={af_ex} (dilution confirmed)"
        return False, "No exome_only variant found to check dilution"

    @test
    def test_cel_filter_by_allele_count(self):
        """CEL filter on the new ac field works (ac >= 1000 keeps the common variants)"""
        ids = ",".join(v["id"] for v in self._variants())
        data = self._get({"i": ids, "d": "1", "f": "gnomad_variant.ac >= 1000"})
        rows = _rows(data, "GnomadVariant")
        if not rows:
            return False, "ac filter returned no results"
        for r in rows:
            if int(r["Attributes"]["GnomadVariant"].get("ac", 0)) < 1000:
                return False, f"filter leaked {r.get('identifier')} (ac<1000)"
        return True, f"CEL filter ac>=1000 kept {len(rows)} variants"

    @test
    def test_dbsnp_rsid_join(self):
        """rsID reaches the variant frequency via the dbsnp hub (rs2691305 -> 1:69094:G:A)"""
        data = requests.get(f"{self.api}/ws/map/",
                            params={"i": "rs2691305", "m": ">>dbsnp>>gnomad_variant"},
                            timeout=15).json()
        idents = {t.get("identifier")
                  for res in data.get("results", []) for t in res.get("targets", [])}
        if "1:69094:G:A" in idents:
            return True, "rs2691305 >>dbsnp>>gnomad_variant -> 1:69094:G:A"
        return False, f"rsID->dbsnp->gnomad_variant join failed; got {sorted(idents)[:5]}"

    @test
    def test_cel_filter_rare(self):
        """CEL filter af<0.001 keeps rare variants and drops the common one (1:69094 af=0.152)"""
        ids = ",".join(v["id"] for v in self._variants())
        data = self._get({"i": ids, "d": "1", "f": "gnomad_variant.af < 0.001"})
        rows = _rows(data, "GnomadVariant")
        if not rows:
            return False, "filter returned no results"
        idents = {r.get("identifier") for r in rows}
        if "1:69094:G:A" in idents:
            return False, "common variant 1:69094:G:A (af=0.152) leaked past af<0.001 filter"
        for r in rows:
            if float(r["Attributes"]["GnomadVariant"].get("af", 1)) >= 0.001:
                return False, f"filter leaked {r.get('identifier')}"
        return True, f"CEL filter af<0.001 kept {len(rows)} rare variants"


    @test
    def test_large_indel_findable_by_coordinate(self):
        """A large indel whose full key exceeds the LMDB limit is still findable by
        its full chr:pos:ref:alt (read-side hashing), with full alt kept in attrs"""
        alt = "ACGTTGCA" * 65  # 520 bases -> full chr:pos:ref:alt key > 511 bytes
        full = "3:5000000:A:" + alt
        data = requests.get(f"{self.api}/ws/", params={"i": full, "d": "1"}, timeout=15).json()
        rows = _rows(data, "GnomadVariant")
        if not rows:
            return False, "large indel NOT found by its full coordinate"
        a = rows[0]["Attributes"]["GnomadVariant"]
        if a.get("alt_allele") != alt:
            return False, f"full alt allele not preserved (got {len(a.get('alt_allele') or '')}b)"
        if abs(float(a.get("af", -1)) - 0.0007) > 1e-9:
            return False, f"af mismatch: got {a.get('af')}, want 0.0007"
        return True, f"large indel found by full coordinate; full {len(alt)}b alt preserved, af={a.get('af')}"

    @test
    def test_large_indel_findable_by_rsid(self):
        """The same large indel is also reachable via the dbsnp rsID hub"""
        data = requests.get(f"{self.api}/ws/map/",
                            params={"i": "rs555555", "m": ">>dbsnp>>gnomad_variant"}, timeout=15).json()
        for t in _targets(data):
            if (t.get("Attributes") or {}).get("GnomadVariant", {}).get("position") == 5000000:
                return True, "rs555555 >>dbsnp>>gnomad_variant reached the large indel"
        return False, "large indel not reachable via the rsID hub"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = GnomadVariantTests(runner)
    for m in [
        custom.test_variant_lookup,
        custom.test_key_scheme_chr_pos_ref_alt,
        custom.test_af_values,
        custom.test_ac_an_faf99_joint_values,
        custom.test_per_callset_ac_an_af,
        custom.test_per_callset_faf,
        custom.test_exome_only_af_dilution,
        custom.test_cel_filter_by_allele_count,
        custom.test_dbsnp_rsid_join,
        custom.test_cel_filter_rare,
        custom.test_large_indel_findable_by_coordinate,
        custom.test_large_indel_findable_by_rsid,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
