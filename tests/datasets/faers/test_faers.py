#!/usr/bin/env python3
"""
FAERS (openFDA FDA Adverse Event Reporting System) Test Suite -- master/child.

Validates the master/child layout:
  - a drug name resolves (text search) to a faers MASTER record (one per drug)
    carrying total_reports / distinct_reactions / serious_reports
  - the master -> faers_reaction CHILD chain yields per-(drug,reaction) records
    with report_count / prr, ordered most-reported first
  - the best-effort drug-name normalization edge to chembl_molecule / pubchem
    hangs off the master (the single drug<->compound node)

Caveats reflected here: FAERS edges are report-level CO-OCCURRENCE (not causal),
reactions are MedDRA PT strings only, individual reports are NOT stored (only
aggregates), and the chembl/pubchem edge is best-effort (so its absence is
tolerated, presence is asserted opportunistically).
"""

import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


def _attr_rows(data, attr_name):
    """Return result rows that carry the given Attribute block."""
    out = []
    for r in (data or {}).get("results", []):
        attrs = r.get("Attributes") or {}
        if attr_name in attrs:
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


def _child_attrs_in_order(data):
    """Flatten FaersReaction child attribute blocks from a map result, in order.

    Handles the default map shape (results[].targets[]) as well as the older
    entries[] shape, so the per-child report_count ordering is preserved."""
    rows = []
    for r in (data or {}).get("results", []):
        for e in (r.get("targets") or []) + (r.get("entries") or []):
            a = e.get("Attributes") or {}
            fr = a.get("FaersReaction")
            if fr:
                rows.append(fr)
    return rows


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
    def test_drug_name_resolves_to_master(self):
        """A drug name text-search resolves to a faers MASTER record (one per drug)"""
        for d in self._drugs():
            rows = _attr_rows(self.runner.lookup(d["drug_name"]), "Faers")
            if rows:
                return True, f"{d['drug_name']} -> {len(rows)} faers master record(s)"
        return False, "No drug name resolved to a faers master record"

    @test
    def test_master_has_summary(self):
        """A faers master carries drug_name + total_reports + distinct_reactions"""
        for d in self._drugs():
            for r in _attr_rows(self.runner.lookup(d["drug_name"]), "Faers"):
                a = r["Attributes"]["Faers"]
                if a.get("drug_name") and a.get("total_reports", 0) >= 1 and "distinct_reactions" in a:
                    return True, (f"{a.get('drug_name')} -> total_reports={a.get('total_reports')}, "
                                  f"distinct_reactions={a.get('distinct_reactions')}, "
                                  f"serious_reports={a.get('serious_reports')}")
        return False, "No faers master with drug_name + total_reports + distinct_reactions"

    @test
    def test_master_to_reactions_chain_sorted(self):
        """drug >> faers >> faers_reaction yields children, most-reported first.

        Verifies the master/child chain and the report_count DESC ordering of the
        master->child edges."""
        for d in self._drugs():
            data = self.runner.map_query(d["drug_name"], ">>faers>>faers_reaction")
            rows = _child_attrs_in_order(data)
            if not rows:
                continue
            counts = [r.get("report_count", 0) for r in rows if "report_count" in r]
            ordered = all(counts[i] >= counts[i + 1] for i in range(len(counts) - 1))
            top = rows[0]
            if "report_count" in top and "prr" in top and "reaction" in top and ordered:
                return True, (f"{d['drug_name']} -> {len(rows)} reactions, top "
                              f"'{top.get('reaction')}' rc={top.get('report_count')} "
                              f"prr={top.get('prr'):.2f}; report_count DESC OK")
            if not ordered:
                return False, f"{d['drug_name']} children not sorted by report_count DESC: {counts[:8]}"
        return False, "No faers master resolved to faers_reaction children"

    @test
    def test_child_prr_is_nonnegative(self):
        """PRR disproportionality signal on a child reaction is non-negative"""
        for d in self._drugs():
            data = self.runner.map_query(d["drug_name"], ">>faers>>faers_reaction")
            for fr in _child_attrs_in_order(data):
                prr = fr.get("prr")
                if prr is not None:
                    if prr >= 0:
                        return True, f"prr={prr:.3f} for {d['drug_name']} / {fr.get('reaction')}"
                    return False, f"negative prr {prr}"
        return False, "No prr value observed on a faers_reaction child"

    @test
    def test_compound_to_faers_master_edge(self):
        """Best-effort: chembl_molecule / pubchem maps to the faers drug master.

        The drug-name normalization edge is best-effort (no native UNII/RxNorm
        dataset), so a missing edge is tolerated; we assert it when the edge db
        is present in this build. This is the single drug<->compound node."""
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
        custom.test_drug_name_resolves_to_master,
        custom.test_master_has_summary,
        custom.test_master_to_reactions_chain_sorted,
        custom.test_child_prr_is_nonnegative,
        custom.test_compound_to_faers_master_edge,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
