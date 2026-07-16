#!/usr/bin/env python3
"""
ESM1b (protein language-model variant effect) Test Suite.

Validates biobtree's ESM1b predictor layer. LLR <= 0, more negative = more
damaging. KEY SCHEME: "uniprot:protein_variant" (e.g. "P01116:G12D") — protein-
level, joins to Atlas variants via (uniprot_id, protein_variant), the same single-
letter WT+pos+mut notation AlphaMissense uses.

Data under test is the hand-crafted fixture (tests/datasets/esm1b/esm1b_fixture.tsv),
NOT real ESM1b data.
"""

import sys
import os
import re
import requests
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


def _attr_rows(data, attr_name):
    out = []
    for r in (data or {}).get("results", []):
        attrs = r.get("Attributes") or {}
        if attr_name in attrs:
            out.append(r)
    return out


class Esm1bTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _variants(self):
        ref = self.runner.reference_data
        return ref.get("variants", []) if isinstance(ref, dict) else []

    def _row(self, vid):
        for r in _attr_rows(self.runner.lookup(vid), "Esm1B"):
            if r.get("identifier") == vid:
                return r
        return None

    @test
    def test_variant_lookup(self):
        """A uniprot:protein_variant id resolves to an Esm1b record with the LLR"""
        for v in self._variants():
            r = self._row(v["id"])
            if r is not None:
                a = r["Attributes"]["Esm1B"]
                if "esm1b_llr" not in a:
                    return False, f"{v['id']} missing esm1b_llr"
                return True, f"{v['id']} -> llr={a.get('esm1b_llr')} ({a.get('gene_symbol')})"
        return False, "No uniprot:protein_variant id resolved to an Esm1b record"

    @test
    def test_key_scheme(self):
        """Entry ID is uniprot:protein_variant (single-letter WT+pos+mut)"""
        pat = re.compile(r"^[A-Z0-9-]+:[A-Z][0-9]+[A-Z]$")
        for v in self._variants():
            r = self._row(v["id"])
            if r is not None:
                ident = r.get("identifier", "")
                if not pat.match(ident):
                    return False, f"key '{ident}' not uniprot:protein_variant"
                return True, f"key scheme OK: '{ident}'"
        return False, "No esm1b record to check key scheme"

    @test
    def test_score_values(self):
        """Stored LLR values match the fixture (incl. sign)"""
        for v in self._variants():
            r = self._row(v["id"])
            if r is None:
                continue
            got = float(r["Attributes"]["Esm1B"].get("esm1b_llr", 0.0))
            want = float(v["esm1b_llr"])
            if abs(got - want) > 0.01:
                return False, f"{v['id']} llr: got {got}, want {want}"
            return True, f"{v['id']} llr {got} matches fixture"
        return False, "No esm1b record matched for value check"

    @test
    def test_uniprot_xref(self):
        """Each ESM1b entry cross-references its UniProt protein"""
        r = self._row("P01116:G12D")
        if r is None:
            return False, "P01116:G12D not found"
        entries = r.get("entries", [])
        xr = r.get("xrefs", {})
        # xref surfaces either as embedded 'entries' or in the xrefs summary
        joined = {e.get("dataset_name"): e.get("identifier") for e in entries}
        if joined.get("uniprot") == "P01116":
            return True, "xref -> uniprot P01116 present"
        # fallback: xrefs summary lists uniprot
        data = xr.get("data", [])
        if any(str(d).startswith("uniprot") for d in data):
            return True, f"xref -> uniprot present ({data})"
        return False, f"no uniprot xref found (entries={joined}, xrefs={data})"

    @test
    def test_cel_filter_damaging(self):
        """CEL filter esm1b_llr < -5 keeps damaging variants, drops tolerated"""
        ids = ",".join(v["id"] for v in self._variants())
        url = f"{self.runner.api_url}/ws/?i={ids}&d=1&f=esm1b.esm1b_llr<-5.0"
        resp = requests.get(url, timeout=15)
        if resp.status_code != 200:
            return False, f"HTTP {resp.status_code}"
        rows = _attr_rows(resp.json(), "Esm1B")
        if not rows:
            return False, "filter returned no results"
        for r in rows:
            llr = float(r["Attributes"]["Esm1B"].get("esm1b_llr", 0.0))
            if llr >= -5.0:
                return False, f"filter leaked {r.get('identifier')} llr={llr}"
        idents = {r.get("identifier") for r in rows}
        if "P60484:A123V" in idents:
            return False, "tolerated A123V (llr -1.2) was not filtered out"
        return True, f"CEL filter esm1b_llr<-5 kept {len(rows)} damaging variant(s)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = Esm1bTests(runner)
    for m in [
        custom.test_variant_lookup,
        custom.test_key_scheme,
        custom.test_score_values,
        custom.test_uniprot_xref,
        custom.test_cel_filter_damaging,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
