#!/usr/bin/env python3
"""dbVar clinical structural-variant tests.

The test build ingests the first 500 records of the clinical
pathogenic_or_likely_pathogenic VCF. Entries are keyed by nssv call id.
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


class DbvarTests:
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
        """nssv16255736 is a 1p36 deletion with clinical significance"""
        r = _entry(self.runner, "nssv16255736")
        if r is None:
            return False, "nssv16255736 not found"
        a = (r.get("Attributes") or {}).get("Dbvar")
        if a is None:
            return False, "Dbvar attributes missing"
        if a.get("chromosome") != "1" or a.get("svtype") != "DEL":
            return False, f"chromosome/svtype={a.get('chromosome')}/{a.get('svtype')}"
        if int(a.get("start") or 0) != 10001 or int(a.get("end") or 0) != 5532775:
            return False, f"coordinates={a.get('start')}-{a.get('end')}"
        if "Pathogenic" not in (a.get("clinical_significance") or ""):
            return False, f"clinical_significance={a.get('clinical_significance')}"
        return True, f"nssv16255736: {a['svtype']} {a['chromosome']}:{a['start']}-{a['end']} {a['clinical_significance']}"

    @test
    def test_clinvar_and_disease_xrefs(self):
        """A dbVar call links to ClinVar and phenotype/disease ids"""
        r = requests.get(
            f"{self.api}/ws/entry/",
            params={"i": "nssv16255736", "s": "dbvar"},
            timeout=30,
        ).json()
        if not r or not r.get("identifier"):
            return False, "nssv16255736 not found"
        counts = {}
        for row in ((r.get("xrefs") or {}).get("data") or []):
            name, _, n = row.partition("|")
            try:
                counts[name] = int(n)
            except ValueError:
                counts[name] = 0
        if counts.get("clinvar", 0) < 1:
            return False, f"no ClinVar xref: {counts}"
        if counts.get("mim", 0) < 1:
            return False, f"no MIM xref: {counts}"
        return True, f"xrefs={counts}"

    @test
    def test_interval_query(self):
        """A genomic position returns the SVs that cover it"""
        r = requests.get(
            f"{self.api}/ws/map/",
            params={"i": "1:10001", "m": ">>dbvar", "mode": "lite"},
            timeout=30,
        ).json()
        targets = []
        for m in r.get("mappings") or []:
            targets.extend(m.get("targets") or [])
        ids = [t.split("|")[0] for t in targets]
        if not any(i.upper() == "NSSV16255736" for i in ids):
            return False, f"nssv16255736 not in interval hits: {ids[:5]}"
        return True, f"1:10001 -> {len(ids)} SV(s)"

    @test
    def test_all_fixture_coordinates(self):
        """Each fixture record carries its declared coordinates and type"""
        for e in self._entries():
            r = _entry(self.runner, e["id"])
            if r is None:
                return False, f"missing {e['id']}"
            a = (r.get("Attributes") or {}).get("Dbvar") or {}
            if a.get("chromosome") != e["chromosome"] or a.get("svtype") != e["svtype"]:
                return False, f"{e['id']} chromosome/svtype mismatch"
            if int(a.get("start") or 0) != e["start"] or int(a.get("end") or 0) != e["end"]:
                return False, f"{e['id']} coordinates mismatch: {a.get('start')}-{a.get('end')}"
            if e["clinical_significance"] not in (a.get("clinical_significance") or ""):
                return False, f"{e['id']} clinical_significance={a.get('clinical_significance')}"
        return True, f"{len(self._entries())} records verified"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(
        os.environ.get("BIOBTREE_API_URL", "http://localhost:9292"),
        script_dir / "reference_data.json",
    )
    c = DbvarTests(runner)
    for m in [
        c.test_entry_attributes,
        c.test_clinvar_and_disease_xrefs,
        c.test_interval_query,
        c.test_all_fixture_coordinates,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
