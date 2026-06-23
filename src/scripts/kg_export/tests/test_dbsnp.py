"""dbSNP optional builder tests.

    python3 -m unittest kg_export.tests.test_dbsnp -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from kg_export.categories import CategoryMap
from kg_export.datasets import DatasetRegistry
from kg_export.dbsnp import build_dbsnp

REPO_ROOT = Path(__file__).resolve().parents[4]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = Path(__file__).resolve().parents[1] / "mappings" / "categories.yaml"


class DbsnpBuildTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)

    def _id(self, name):
        return self.reg.by_name(name).numeric_id

    def _run(self, lines, id_map=None, max_variants=None):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            with gzip.open(tmp / "dbsnp_sorted.1.index.gz", "wt") as fh:
                fh.write("".join(l + "\n" for l in lines))
            nout, eout = tmp / "n.tsv", tmp / "e.tsv"
            stats = build_dbsnp(tmp, self.reg, self.cats, nout, eout,
                                id_map=id_map, max_variants=max_variants,
                                stats_path=tmp / "s.json")
            nodes = {r.split("\t")[0]: r.split("\t")[1]
                     for r in nout.read_text().splitlines()[1:]}
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in eout.read_text().splitlines()[1:]}
            return stats, nodes, edges

    def test_variant_nodes_and_gene_edges(self):
        ds, en, rs = self._id("dbsnp"), self._id("entrez"), self._id("refseq")
        lines = [
            f"RS10\t{ds}\t1021\t{en}",                # variant -> gene
            f"RS10\t{ds}\tNM_001259\t{rs}",           # transcript -> skipped (v1)
            f'RS10\t{ds}\t{{"rs_id":"rs10","variant_type":"snv"}}\t-1',
            f"RS1000\t{ds}\t177\t{en}",
            f'RS1000\t{ds}\t{{"rs_id":"rs1000"}}\t-1',
        ]
        stats, nodes, edges = self._run(lines)
        self.assertEqual(nodes["DBSNP:rs10"], "biolink:SequenceVariant")  # canonical lowercase id
        self.assertIn(("DBSNP:rs10", "biolink:is_sequence_variant_of", "NCBIGene:1021"), edges)
        self.assertEqual(stats.nodes_written, 2)
        self.assertEqual(stats.edges_written, 2)  # transcript line not emitted in v1

    def test_gene_canonicalized_and_cap(self):
        ds, en = self._id("dbsnp"), self._id("entrez")
        lines = [
            f"RS1\t{ds}\t1021\t{en}", f'RS1\t{ds}\t{{"rs_id":"rs1"}}\t-1',
            f"RS2\t{ds}\t177\t{en}",  f'RS2\t{ds}\t{{"rs_id":"rs2"}}\t-1',
        ]
        stats, _, edges = self._run(lines, id_map={"NCBIGene:1021": "HGNC:5"}, max_variants=1)
        self.assertEqual(stats.variants, 1)  # cap honored
        self.assertIn(("DBSNP:rs1", "biolink:is_sequence_variant_of", "HGNC:5"), edges)


if __name__ == "__main__":
    unittest.main()
