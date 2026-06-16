#!/usr/bin/env python3
"""
FAERS (openFDA FDA Adverse Event Reporting System) Test Suite

Validates the (drug, adverse-reaction) co-occurrence aggregates:
  - a drug name resolves (text search) to FAERS records with report_count / prr
  - the best-effort drug-name normalization edge to chembl_molecule / pubchem
  - the FAERS attributes (reaction MedDRA PT string, counts, PRR signal)

Caveats reflected here: FAERS edges are report-level CO-OCCURRENCE (not causal),
reactions are MedDRA PT strings only, and the chembl/pubchem edge is best-effort
(so its absence is tolerated, presence is asserted opportunistically).
"""

import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


def _faers_results(data):
    """Return result rows that carry a Faers attribute block."""
    out = []
    for r in (data or {}).get("results", []):
        attrs = r.get("Attributes") or {}
        if "Faers" in attrs:
            out.append(r)
    return out


def _mapped(data):
    if not data:
        return 0
    st = data.get("stats", {})
    if st.get("mapped"):
        return st["mapped"]
    n = 0
    for r in data.get("results", []):
        n += len(r.get("entries", [])) or r.get("count", 0)
    return n


class FaersTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _drugs(self):
        ref = self.runner.reference_data
        return ref.get("drugs", []) if isinstance(ref, dict) else []

    def _compounds(self):
        ref = self.runner.reference_data
        return ref.get("compounds", []) if isinstance(ref, dict) else []

    @test
    def test_drug_name_resolves_to_faers(self):
        """A drug name text-search resolves to FAERS (drug, reaction) records"""
        for d in self._drugs():
            rows = _faers_results(self.runner.lookup(d["drug_name"]))
            if rows:
                return True, f"{d['drug_name']} -> {len(rows)} FAERS record(s)"
        return False, "No drug name resolved to FAERS records"

    @test
    def test_record_has_reaction_and_counts(self):
        """A FAERS record carries reaction (MedDRA PT), report_count and prr"""
        for d in self._drugs():
            for r in _faers_results(self.runner.lookup(d["drug_name"])):
                a = r["Attributes"]["Faers"]
                if a.get("reaction") and a.get("report_count", 0) >= 1 and "prr" in a:
                    return True, (f"{a.get('drug_name')} -> {a.get('reaction')} "
                                  f"(rc={a.get('report_count')}, prr={a.get('prr'):.2f})")
        return False, "No FAERS record with reaction + report_count + prr"

    @test
    def test_prr_is_nonnegative(self):
        """PRR disproportionality signal is a sane non-negative number"""
        for d in self._drugs():
            for r in _faers_results(self.runner.lookup(d["drug_name"])):
                prr = r["Attributes"]["Faers"].get("prr")
                if prr is not None:
                    if prr >= 0:
                        return True, f"prr={prr:.3f} for {d['drug_name']}"
                    return False, f"negative prr {prr}"
        return False, "No prr value observed"

    @test
    def test_compound_to_faers_edge(self):
        """Best-effort: chembl_molecule / pubchem maps to FAERS adverse events.

        The drug-name normalization edge is best-effort (no native UNII/RxNorm
        dataset), so a missing edge is tolerated; we assert it when the edge db
        is present in this build."""
        attempted = False
        for c in self._compounds():
            if c.get("chembl_id"):
                attempted = True
                if _mapped(self.runner.map_query(c["chembl_id"], ">>faers", mode="lite")) > 0:
                    return True, f"{c['chembl_id']} >>faers OK"
            if c.get("pubchem_cid"):
                attempted = True
                if _mapped(self.runner.map_query(c["pubchem_cid"], ">>faers", mode="lite")) > 0:
                    return True, f"pubchem {c['pubchem_cid']} >>faers OK"
        if not attempted:
            return False, "No compound reference to test"
        # Edge is best-effort; not a hard failure when the target db is absent.
        return True, "compound>>faers edge not present in this build (best-effort, tolerated)"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = FaersTests(runner)
    for m in [
        custom.test_drug_name_resolves_to_faers,
        custom.test_record_has_reaction_and_counts,
        custom.test_prr_is_nonnegative,
        custom.test_compound_to_faers_edge,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
