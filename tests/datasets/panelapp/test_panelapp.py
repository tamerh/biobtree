#!/usr/bin/env python3
"""
PanelApp (Genomics England clinical gene panels) Test Suite -- master/child.

Validates the master/child layout:
  - a panel id resolves to a panelapp MASTER record (one per panel) carrying
    name / disease_group / number_of_genes
  - the master -> panelapp_gene CHILD chain yields per-(panel,gene) records with
    gene_symbol / confidence (green|amber) / mode_of_inheritance
  - a panel gene resolves to HGNC (and onward to ensembl / mim / mondo)
  - a gene symbol text-search reaches its panelapp_gene record

Scope reflected here: only green (level 3) + amber (level 2) genes are ingested.
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


def _child_attrs(data):
    """Flatten PanelappGene child attribute blocks from a map result."""
    rows = []
    for r in (data or {}).get("results", []):
        for e in (r.get("targets") or []) + (r.get("entries") or []):
            a = e.get("Attributes") or {}
            pg = a.get("PanelappGene")
            if pg:
                rows.append(pg)
    return rows


def _mapped(data):
    if not data:
        return 0
    st = data.get("stats", {})
    if st.get("mapped"):
        return st["mapped"]
    n = 0
    for r in data.get("results", []):
        n += len(r.get("entries", [])) or len(r.get("targets", [])) or r.get("count", 0)
    return n


class PanelappTests:
    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _panels(self):
        ref = self.runner.reference_data
        return ref.get("panels", []) if isinstance(ref, dict) else []

    def _genes(self):
        ref = self.runner.reference_data
        return ref.get("genes", []) if isinstance(ref, dict) else []

    @test
    def test_panel_master_exists(self):
        """A panel id resolves to a panelapp MASTER record with name + number_of_genes"""
        for p in self._panels():
            for r in _attr_rows(self.runner.lookup(p["panel_id"]), "Panelapp"):
                a = r["Attributes"]["Panelapp"]
                if a.get("name"):
                    return True, (f"{p['panel_id']} -> '{a.get('name')}' "
                                  f"genes={a.get('number_of_genes')} group={a.get('disease_group')}")
        return False, "No panel id resolved to a panelapp master record"

    @test
    def test_panel_to_genes_chain(self):
        """panel >> panelapp >> panelapp_gene yields child gene records (green/amber)"""
        for p in self._panels():
            rows = _child_attrs(self.runner.map_query(p["panel_id"], ">>panelapp>>panelapp_gene"))
            if rows:
                conf = {r.get("confidence") for r in rows}
                if conf and conf <= {"green", "amber"}:
                    return True, (f"{p['panel_id']} -> {len(rows)} gene records, "
                                  f"confidence={sorted(conf)}")
                return False, f"unexpected confidence values: {conf}"
        return False, "No panel master resolved to panelapp_gene children"

    @test
    def test_gene_symbol_resolves_to_child(self):
        """A gene symbol text-search reaches its panelapp_gene record"""
        for g in self._genes():
            rows = _attr_rows(self.runner.lookup(g["gene_symbol"]), "PanelappGene")
            for r in rows:
                a = r["Attributes"]["PanelappGene"]
                if a.get("gene_symbol", "").upper() == g["gene_symbol"].upper():
                    return True, f"{g['gene_symbol']} -> panelapp_gene ({a.get('panel_name')})"
        return False, "No gene symbol resolved to a panelapp_gene record"

    @test
    def test_gene_to_hgnc_edge(self):
        """A panel gene resolves to HGNC: gene >> panelapp_gene >> hgnc"""
        for g in self._genes():
            if _mapped(self.runner.map_query(g["gene_symbol"], ">>panelapp_gene>>hgnc", mode="lite")) > 0:
                return True, f"{g['gene_symbol']} >>panelapp_gene>>hgnc OK"
        return False, "No panelapp_gene resolved to hgnc"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        return 1

    runner = TestRunner(api_url, reference_file, test_cases_file)
    custom = PanelappTests(runner)
    for m in [
        custom.test_panel_master_exists,
        custom.test_panel_to_genes_chain,
        custom.test_gene_symbol_resolves_to_child,
        custom.test_gene_to_hgnc_edge,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
