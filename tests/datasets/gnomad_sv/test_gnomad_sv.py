#!/usr/bin/env python3
"""gnomAD SV tests.

The test fixture has four SV sites. Large SVs (>1 Mb) deliberately skip the
interval index; small ones are point-queryable.
"""
import os
import sys
from pathlib import Path

import requests

sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner, test


def _entry(runner, entry_id):
    data = runner.lookup(entry_id)
    for r in (data or {}).get("results", []):
        if r.get("identifier", "").upper() == entry_id.upper():
            return r
    return None


def _attr(runner, entry_id):
    r = _entry(runner, entry_id)
    if r is None:
        return None
    return (r.get("Attributes") or {}).get("GnomadSv")


class GnomadSvTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _entries(self):
        ref = self.runner.reference_data
        if isinstance(ref, list):
            return ref
        if isinstance(ref, dict):
            return ref.get("entries", [])
        return []

    @test
    def test_entry_attributes(self):
        """The DUP carries coordinates, AF and predicted gene/effect"""
        a = _attr(self.runner, "gnomAD-SV_v3_DUP_chr1_test1")
        if a is None:
            return False, "DUP record not found"
        if a.get("chromosome") != "1" or a.get("svtype") != "DUP":
            return False, f"chromosome/svtype={a.get('chromosome')}/{a.get('svtype')}"
        if int(a.get("start") or 0) != 10000 or int(a.get("end") or 0) != 295666:
            return False, f"coordinates={a.get('start')}-{a.get('end')}"
        if a.get("af") != "0.001997" or a.get("filters") != "PASS":
            return False, f"af/filters={a.get('af')}/{a.get('filters')}"
        if "OR4F5" not in (a.get("predicted_genes") or []):
            return False, f"predicted_genes={a.get('predicted_genes')}"
        if "COPY_GAIN:OR4F5" not in (a.get("predicted_effects") or []):
            return False, f"predicted_effects={a.get('predicted_effects')}"
        return True, f"DUP 1:{a['start']}-{a['end']} AF={a['af']} gene=OR4F5"

    @test
    def test_gene_xref(self):
        """A PREDICTED_* gene creates an HGNC xref"""
        r = requests.get(
            f"{self.api}/ws/entry/",
            params={"i": "gnomAD-SV_v3_DUP_chr1_test1", "s": "gnomad_sv"},
            timeout=30,
        ).json()
        if not r or not r.get("identifier"):
            return False, "DUP record not found"
        counts = {}
        for row in ((r.get("xrefs") or {}).get("data") or []):
            name, _, n = row.partition("|")
            try:
                counts[name] = int(n)
            except ValueError:
                counts[name] = 0
        if counts.get("hgnc", 0) < 1:
            return False, f"no HGNC xref: {counts}"
        return True, f"xrefs={counts}"

    @test
    def test_small_sv_interval_query(self):
        """A position inside a small SV returns the site"""
        r = requests.get(
            f"{self.api}/ws/map/",
            params={"i": "1:20000", "m": ">>gnomad_sv", "mode": "lite"},
            timeout=30,
        ).json()
        targets = []
        for m in r.get("mappings") or []:
            targets.extend(m.get("targets") or [])
        ids = [t.split("|")[0] for t in targets]
        if not any(i.upper() == "GNOMAD-SV_V3_DUP_CHR1_TEST1" for i in ids):
            return False, f"DUP not in interval hits: {ids[:5]}"
        return True, f"1:20000 -> {len(ids)} SV(s)"

    @test
    def test_large_sv_not_interval_indexed(self):
        """SVs over 1 Mb are intentionally not in the interval index"""
        r = requests.get(
            f"{self.api}/ws/map/",
            params={"i": "1:2000000", "m": ">>gnomad_sv", "mode": "lite"},
            timeout=30,
        ).json()
        targets = []
        for m in r.get("mappings") or []:
            targets.extend(m.get("targets") or [])
        ids = [t.split("|")[0] for t in targets]
        if any(i.upper() == "GNOMAD-SV_V3_DEL_CHR1_TEST2" for i in ids):
            return False, "large DEL unexpectedly returned by interval index"
        return True, "large SV stays out of the interval index"

    @test
    def test_coordinate_key_search(self):
        """The chr:pos:end:svtype key finds the site"""
        r = requests.get(
            f"{self.api}/ws/search/",
            params={"i": "1:10000:295666:DUP", "mode": "lite"},
            timeout=30,
        ).json()
        ids = [row.split("|")[0] for row in (r.get("data") or [])]
        if not any(i.upper() == "GNOMAD-SV_V3_DUP_CHR1_TEST1" for i in ids):
            return False, f"coordinate key did not resolve: {ids[:5]}"
        return True, "coordinate key -> DUP site"

    @test
    def test_all_fixture_records(self):
        """Every fixture record carries its declared coordinates/type"""
        for e in self._entries():
            a = _attr(self.runner, e["id"])
            if a is None:
                return False, f"missing {e['id']}"
            if a.get("chromosome") != e["chromosome"] or a.get("svtype") != e["svtype"]:
                return False, f"{e['id']} chromosome/svtype mismatch"
            if int(a.get("start") or 0) != e["start"] or int(a.get("end") or 0) != e["end"]:
                return False, f"{e['id']} coordinates mismatch"
            if a.get("af") != e["af"]:
                return False, f"{e['id']} af={a.get('af')} != {e['af']}"
        return True, f"{len(self._entries())} records verified"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(
        os.environ.get("BIOBTREE_API_URL", "http://localhost:9292"),
        script_dir / "reference_data.json",
    )
    c = GnomadSvTests(runner)
    for m in [
        c.test_entry_attributes,
        c.test_gene_xref,
        c.test_small_sv_interval_query,
        c.test_large_sv_not_interval_indexed,
        c.test_coordinate_key_search,
        c.test_all_fixture_records,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
