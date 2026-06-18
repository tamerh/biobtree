"""Phase 2e tests: ontology subclass_of + cross-ontology close_match.

    python3 -m unittest tools.kg_export.tests.test_phase2e_ontology -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.ontology import build_ontology

REPO_ROOT = Path(__file__).resolve().parents[3]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = REPO_ROOT / "mappings" / "categories.yaml"


class OntologyBuildTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)

    def _id(self, name):
        return self.reg.by_name(name).numeric_id

    def _run(self, fname, lines):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            with gzip.open(tmp / fname, "wt") as fh:
                fh.write("".join(l + "\n" for l in lines))
            out = tmp / "e.tsv"
            stats = build_ontology(tmp, self.reg, self.cats, out,
                                   stats_path=tmp / "s.json")
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            return stats, edges

    def test_subclass_and_close_match(self):
        mondo, par, ch, doid, ub = (
            self._id("mondo"), self._id("mondoparent"), self._id("mondochild"),
            self._id("doid"), self._id("uberon"),
        )
        lines = [
            f"MONDO:0000005\t{mondo}\tMONDO:0004907\t{par}",   # subclass_of
            f"MONDO:0000005\t{mondo}\tMONDO:0099999\t{ch}",    # child -> skipped
            f"MONDO:0000005\t{mondo}\tDOID:1612\t{doid}",      # close_match (Disease)
            f"MONDO:0000005\t{mondo}\tUBERON:0000310\t{ub}",   # diff cat -> skip
            f'MONDO:0000005\t{mondo}\t{{"name":"x"}}\t-1',     # property -> skip
        ]
        stats, edges = self._run("mondo_sorted.1.index.gz", lines)
        self.assertIn(("MONDO:0000005", "biolink:subclass_of", "MONDO:0004907"), edges)
        self.assertIn(("MONDO:0000005", "biolink:close_match", "DOID:1612"), edges)
        # child (reverse) not emitted; cross-category (uberon) not emitted
        self.assertNotIn(("MONDO:0000005", "biolink:subclass_of", "MONDO:0099999"), edges)
        self.assertEqual(stats.subclass_edges, 1)
        self.assertEqual(stats.close_match_edges, 1)
        self.assertEqual(len(edges), 2)

    def test_phenotype_cross_species_close_match(self):
        """uPheno hub -> HP/MP/ZP are all PhenotypicFeature -> close_match."""
        up, par, hpo, zp = (
            self._id("upheno"), self._id("uphenoparent"),
            self._id("hpo"), self._id("zp"),
        )
        lines = [
            f"UPHENO:0000001\t{up}\tUPHENO:0000002\t{par}",   # subclass_of
            f"UPHENO:0000001\t{up}\tHP:0000001\t{hpo}",       # close_match
            f"UPHENO:0000001\t{up}\tZP:0000000\t{zp}",        # close_match
        ]
        stats, edges = self._run("upheno_sorted.1.index.gz", lines)
        self.assertIn(("UPHENO:0000001", "biolink:subclass_of", "UPHENO:0000002"), edges)
        self.assertIn(("UPHENO:0000001", "biolink:close_match", "HP:0000001"), edges)
        self.assertIn(("UPHENO:0000001", "biolink:close_match", "ZP:0000000"), edges)

    def test_go_hierarchy_only_no_close_match(self):
        """GO is runtime-typed (3 aspects) -> subclass_of only, no close_match."""
        go, par = self._id("go"), self._id("goparent")
        lines = [
            f"GO:0008150\t{go}\tGO:0009987\t{par}",
        ]
        stats, edges = self._run("go_sorted.1.index.gz", lines)
        self.assertIn(("GO:0008150", "biolink:subclass_of", "GO:0009987"), edges)
        self.assertEqual(stats.close_match_edges, 0)


if __name__ == "__main__":
    unittest.main()
