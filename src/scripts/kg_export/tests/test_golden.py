"""Golden-entity tests: known biology must survive the full pipeline on REAL data.

This is the semantic-correctness layer (vs the structural unit tests). It builds
nodes/edges from a real built index dir and asserts well-known facts:
  * BRCA1/TP53 gene normalization (the 3 namespaces collapse to one node)
  * a known gene->protein has_gene_product edge resolves to the canonical gene

Skipped unless a built index dir is available. Set BIOBTREE_INDEX_DIR, else it
tries /data/biobtree/out_prod/main/index then /data2/out_prod_v5/main/index.

    BIOBTREE_INDEX_DIR=/data/biobtree/out_prod/main/index \
        python3 -m unittest kg_export.tests.test_golden -v
"""

import os
import tempfile
import unittest
from pathlib import Path

from kg_export.categories import CategoryMap
from kg_export.datasets import DatasetRegistry
from kg_export.edges import build_edges, load_id_map
from kg_export.nodes import build_nodes
from kg_export.predicates import PredicateMap

REPO_ROOT = Path(__file__).resolve().parents[4]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = Path(__file__).resolve().parents[1] / "mappings" / "categories.yaml"
PREDICATES_YAML = Path(__file__).resolve().parents[1] / "mappings" / "predicates.yaml"


def _find_index_dir() -> str | None:
    # Opt-in only: this builds from a real (large) index dir and is slow, so it
    # must NOT run during the default fast unittest discovery — require the env
    # var to be set explicitly.
    env = os.environ.get("BIOBTREE_INDEX_DIR")
    return env if env and Path(env).exists() else None


class GoldenEntityTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.index_dir = _find_index_dir()
        if not cls.index_dir:
            raise unittest.SkipTest("no built index dir (set BIOBTREE_INDEX_DIR)")
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)
        cls.pm = PredicateMap.load(PREDICATES_YAML)
        cls.tmp = tempfile.TemporaryDirectory()
        tmp = Path(cls.tmp.name)
        # Build genes (fast: hgnc forward carries the cross-namespace edges).
        build_nodes(
            cls.index_dir, cls.reg, cls.cats,
            tmp / "nodes.tsv", id_map_path=tmp / "id_map.tsv", datasets=["hgnc"],
        )
        cls.nodes = {}
        for line in (tmp / "nodes.tsv").read_text().splitlines()[1:]:
            p = line.split("\t")
            cls.nodes[p[0]] = {"category": p[1], "name": p[2], "equiv": set(p[3].split("|"))}
        cls.id_map = load_id_map(tmp / "id_map.tsv")
        cls.tmp_path = tmp

    @classmethod
    def tearDownClass(cls):
        if getattr(cls, "tmp", None):
            cls.tmp.cleanup()

    def _assert_gene(self, hgnc, symbol, ensg, ncbigene):
        self.assertIn(hgnc, self.nodes, f"{symbol} node ({hgnc}) missing")
        node = self.nodes[hgnc]
        self.assertEqual(node["category"], "biolink:Gene")
        self.assertEqual(node["name"], symbol)
        # the 3 namespaces collapsed into one node
        self.assertIn(f"ENSEMBL:{ensg}", node["equiv"], f"{symbol} missing Ensembl")
        self.assertIn(f"NCBIGene:{ncbigene}", node["equiv"], f"{symbol} missing NCBIGene")

    def test_brca1_normalization(self):
        self._assert_gene("HGNC:1100", "BRCA1", "ENSG00000012048", "672")

    def test_tp53_normalization(self):
        self._assert_gene("HGNC:11998", "TP53", "ENSG00000141510", "7157")

    def test_egfr_normalization(self):
        self._assert_gene("HGNC:3236", "EGFR", "ENSG00000146648", "1956")

    def test_gene_product_edge_canonicalized(self):
        """ensembl->uniprot has_gene_product, subject rewritten to the HGNC node."""
        tmp = self.tmp_path
        build_edges(
            self.index_dir, self.reg, self.cats, self.pm, tmp / "e.tsv",
            id_map=self.id_map, datasets=["ensembl"],
        )
        # find has_gene_product edges whose subject is the BRCA1 gene node
        found = False
        for line in (tmp / "e.tsv").read_text().splitlines()[1:]:
            p = line.split("\t")  # id, subject, predicate, object, ...
            if p[1] == "HGNC:1100" and p[2] == "biolink:has_gene_product":
                self.assertTrue(p[3].startswith("UniProtKB:"))
                found = True
                break
        self.assertTrue(found, "no canonicalized has_gene_product edge for BRCA1")


if __name__ == "__main__":
    unittest.main()
