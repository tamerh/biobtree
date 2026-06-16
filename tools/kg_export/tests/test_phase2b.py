"""Phase 2b tests: reified edge building (symmetric + bipartite).

    python3 -m unittest tools.kg_export.tests.test_phase2b -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.edges import load_id_map
from tools.kg_export.predicates import PredicateMap
from tools.kg_export.reified import build_reified_edges

REPO_ROOT = Path(__file__).resolve().parents[3]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = REPO_ROOT / "mappings" / "categories.yaml"
PREDICATES_YAML = REPO_ROOT / "mappings" / "predicates.yaml"


class ReifiedRuleTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.pm = PredicateMap.load(PREDICATES_YAML)
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)

    def test_rules_load(self):
        r = self.pm.reified_rule("intact")
        self.assertEqual(r.kind, "symmetric")
        self.assertEqual(r.partner, "uniprot")
        b = self.pm.reified_rule("chembl_activity")
        self.assertEqual(b.kind, "bipartite")
        self.assertEqual((b.subject, b.object), ("chembl_molecule", "uniprot"))

    def test_reified_partners_are_real_node_datasets(self):
        for ds in self.pm.reified_datasets():
            r = self.pm.reified_rule(ds)
            self.assertIn(ds, self.reg, f"{ds}: reified dataset not in conf")
            roles = [r.partner] if r.kind == "symmetric" else [r.subject, r.object]
            for role in roles:
                self.assertIn(role, self.reg, f"{ds}: role {role} not in conf")
                self.assertTrue(
                    self.cats.is_node_dataset(role),
                    f"{ds}: role {role} is not a typed node",
                )


class BuildReifiedTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)
        cls.pm = PredicateMap.load(PREDICATES_YAML)

    def _id(self, name):
        return self.reg.by_name(name).numeric_id

    def _write(self, d, name, lines):
        with gzip.open(d / name, "wt") as fh:
            fh.write("".join(l + "\n" for l in lines))

    def test_symmetric_ppi(self):
        ia, up = self._id("intact"), self._id("uniprot")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "intact_sorted.1.index.gz", [
                f'I1\t{ia}\t{{"score":0.9}}\t-1',  # property -> ignored
                f"I1\t{ia}\tP2\t{up}",
                f"I1\t{ia}\tP1\t{up}",
                f"I2\t{ia}\tP3\t{up}",  # only one partner -> no edge
            ])
            out = tmp / "r.tsv"
            stats = build_reified_edges(
                tmp, self.reg, self.cats, self.pm, out,
                datasets=["intact"], stats_path=tmp / "s.json",
            )
            rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
            self.assertEqual(len(rows), 1)
            # cols: id, subject, predicate, object, primary, agg, kl, at
            _id, s, p, o, prim = rows[0][:5]
            self.assertEqual((s, o), ("UniProtKB:P1", "UniProtKB:P2"))  # sorted
            self.assertEqual(p, "biolink:physically_interacts_with")
            self.assertEqual(prim, "infores:intact")
            self.assertEqual(stats.edges_written, 1)

    def test_bipartite_bioactivity(self):
        ca, cm, up = self._id("chembl_activity"), self._id("chembl_molecule"), self._id("uniprot")
        bao = self._id("bao")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "chembl_activity_sorted.1.index.gz", [
                f"ACT1\t{ca}\tCHEMBL5\t{cm}",
                f"ACT1\t{ca}\tP9\t{up}",
                f"ACT1\t{ca}\tBAO_1\t{bao}",  # bao role ignored
            ])
            out = tmp / "r.tsv"
            stats = build_reified_edges(
                tmp, self.reg, self.cats, self.pm, out, datasets=["chembl_activity"],
            )
            rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
            self.assertEqual(len(rows), 1)
            _id, s, p, o = rows[0][:4]
            self.assertEqual(s, "CHEMBL.COMPOUND:CHEMBL5")
            self.assertEqual(o, "UniProtKB:P9")
            self.assertEqual(p, "biolink:interacts_with")

    def test_bipartite_expression_with_canonicalization(self):
        bg, en, ub = self._id("bgee"), self._id("ensembl"), self._id("uberon")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            (tmp / "id_map.tsv").write_text("member\tcanonical\nENSEMBL:ENSG1\tHGNC:1\n")
            id_map = load_id_map(tmp / "id_map.tsv")
            self._write(tmp, "bgee_sorted.1.index.gz", [
                f"BG1\t{bg}\tENSG1\t{en}",
                f"BG1\t{bg}\tUBERON:0000955\t{ub}",
            ])
            out = tmp / "r.tsv"
            build_reified_edges(
                tmp, self.reg, self.cats, self.pm, out,
                datasets=["bgee"], id_map=id_map,
            )
            rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
            self.assertEqual(len(rows), 1)
            _id, s, p, o = rows[0][:4]
            self.assertEqual(s, "HGNC:1")  # ensembl gene canonicalized
            self.assertEqual(o, "UBERON:0000955")
            self.assertEqual(p, "biolink:expressed_in")


if __name__ == "__main__":
    unittest.main()
