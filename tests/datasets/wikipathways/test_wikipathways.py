#!/usr/bin/env python3
"""WikiPathways tests.

One entry per pathway (WPID). The GMT release gives Entrez genes; human
pathways also get direct HGNC edges. Test mode processes up to three
pathways per species, so the fixture entries are stable.
"""
import os
import sys
from pathlib import Path

import requests

sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner, test


def _attr(runner, entry_id):
    data = runner.lookup(entry_id)
    for r in (data or {}).get("results", []):
        if r.get("identifier", "").upper() == entry_id.upper():
            return (r.get("Attributes") or {}).get("Wikipathways")
    return None


def _map(api, identifier, chain):
    return requests.get(
        f"{api}/ws/map/",
        params={"i": identifier, "m": chain, "mode": "lite"},
        timeout=60,
    ).json()


class WikiPathwaysTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url
        self.ref = None

    def _entries(self):
        ref = self.runner.reference_data
        if isinstance(ref, list):
            return ref
        if isinstance(ref, dict):
            return ref.get("entries", [])
        return []

    @test
    def test_entry_attributes(self):
        """WP100 carries the pathway name, species, taxid and gene count"""
        a = _attr(self.runner, "WP100")
        if a is None:
            return False, "WP100 not found"
        if a.get("name") != "Glutathione metabolism":
            return False, f"name={a.get('name')}"
        if a.get("species") != "Homo sapiens" or a.get("tax_id") != "9606":
            return False, f"species/tax={a.get('species')}/{a.get('tax_id')}"
        if int(a.get("gene_count") or 0) < 19:
            return False, f"gene_count={a.get('gene_count')}"
        return True, f"WP100: {a['gene_count']} genes, {a['species']}"

    @test
    def test_entrez_edges(self):
        """non-human pathways stay in the Entrez namespace"""
        r = _map(self.api, "WP1000", ">>wikipathways>>entrez")
        targets = []
        for m in r.get("mappings") or []:
            targets.extend(m.get("targets") or [])
        if not targets:
            return False, f"got {len(targets)} Entrez targets"
        return True, f"WP1000 -> {len(targets)} Entrez rows"

    @test
    def test_human_hgnc_edges(self):
        """human pathways link directly to HGNC"""
        r = _map(self.api, "WP100", ">>wikipathways>>hgnc")
        targets = []
        for m in r.get("mappings") or []:
            targets.extend(m.get("targets") or [])
        if not targets:
            return False, f"no HGNC targets: {str(r)[:160]}"
        if not all(t.split("|")[0].startswith("HGNC:") for t in targets):
            return False, f"non-HGNC target: {targets[:2]}"
        return True, f"WP100 -> {len(targets)} HGNC rows"

    @test
    def test_reverse_hgnc_edge(self):
        """A pathway gene reaches the pathway from HGNC"""
        r = _map(self.api, "WP100", ">>wikipathways>>hgnc")
        hgnc_id = None
        for m in r.get("mappings") or []:
            for t in m.get("targets") or []:
                hgnc_id = t.split("|")[0]
                break
            if hgnc_id:
                break
        if not hgnc_id:
            return False, "no HGNC target to test the reverse edge"
        back = _map(self.api, hgnc_id, ">>hgnc>>wikipathways")
        ids = []
        for m in back.get("mappings") or []:
            for t in m.get("targets") or []:
                ids.append(t.split("|")[0])
        if "WP100" not in ids:
            return False, f"{hgnc_id} did not reach WP100: {str(back)[:200]}"
        return True, f"{hgnc_id} -> {len(ids)} WikiPathways rows"

    @test
    def test_search_by_name(self):
        """Searching the pathway name returns the WPID"""
        r = requests.get(
            f"{self.api}/ws/search/",
            params={"i": "Glutathione metabolism", "mode": "lite"},
            timeout=30,
        ).json()
        if "WP100" not in [row.split("|")[0] for row in (r.get("data") or [])]:
            return False, f"WP100 not in search rows: {str(r)[:200]}"
        return True, "search -> WP100"

    @test
    def test_multiple_species(self):
        """Every fixture species keeps its own entry and taxid"""
        seen = set()
        for e in self._entries():
            a = _attr(self.runner, e["id"])
            if a is None:
                return False, f"missing {e['id']}"
            if a.get("species") != e["species"]:
                return False, f"{e['id']} species={a.get('species')} != {e['species']}"
            if a.get("tax_id") != e["tax_id"]:
                return False, f"{e['id']} tax_id={a.get('tax_id')} != {e['tax_id']}"
            if int(a.get("gene_count") or 0) < e["min_genes"]:
                return False, f"{e['id']} gene_count={a.get('gene_count')} < {e['min_genes']}"
            seen.add(e["species"])
        return True, f"{len(seen)} species: {sorted(seen)}"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(
        os.environ.get("BIOBTREE_API_URL", "http://localhost:9292"),
        script_dir / "reference_data.json",
    )
    c = WikiPathwaysTests(runner)
    for m in [
        c.test_entry_attributes,
        c.test_entrez_edges,
        c.test_human_hgnc_edges,
        c.test_reverse_hgnc_edge,
        c.test_search_by_name,
        c.test_multiple_species,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
