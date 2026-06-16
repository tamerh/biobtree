"""Phase 3 tests: merge, JSONL serialization, validation, manifest.

    python3 -m unittest tools.kg_export.tests.test_phase3 -v
"""

import json
import tempfile
import unittest
from pathlib import Path

from tools.kg_export import kgx


def _write(p, header, rows):
    p.write_text(header + "\n" + "".join(r + "\n" for r in rows))


class MergeTests(unittest.TestCase):
    def test_merge_nodes_dedup(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            _write(tmp / "n1.tsv", kgx.NODE_HEADER, [
                "HGNC:1\tbiolink:Gene\tAAA\tHGNC:1\tinfores:biobtree",
            ])
            _write(tmp / "n2.tsv", kgx.NODE_HEADER, [
                "HGNC:1\tbiolink:Gene\tAAA\tHGNC:1\tinfores:biobtree",  # dup
                "GO:1\tbiolink:MolecularActivity\tx\tGO:1\tinfores:biobtree",
            ])
            out = tmp / "nodes.tsv"
            n = kgx.merge_nodes([tmp / "n1.tsv", tmp / "n2.tsv"], out)
            self.assertEqual(n, 2)
            ids = [l.split("\t")[0] for l in out.read_text().splitlines()[1:]]
            self.assertEqual(sorted(ids), ["GO:1", "HGNC:1"])


class ValidateTests(unittest.TestCase):
    def _kg(self, tmp, edges):
        _write(tmp / "nodes.tsv", kgx.NODE_HEADER, [
            "HGNC:1\tbiolink:Gene\tAAA\tHGNC:1\tinfores:biobtree",
            "UniProtKB:P1\tbiolink:Protein\tp1\tUniProtKB:P1\tinfores:biobtree",
        ])
        _write(tmp / "edges.tsv", kgx.EDGE_HEADER, edges)

    def test_clean(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._kg(tmp, [
                "HGNC:1\tbiolink:has_gene_product\tUniProtKB:P1\tinfores:ensembl\tinfores:biobtree",
            ])
            r = kgx.validate(tmp / "nodes.tsv", tmp / "edges.tsv")
            self.assertTrue(r["ok"])
            self.assertEqual(r["edges"], 1)

    def test_dangling_and_bad_predicate(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._kg(tmp, [
                "HGNC:1\tbiolink:has_gene_product\tUniProtKB:MISSING\tinfores:x\tinfores:biobtree",
                "HGNC:1\trelated_to\tUniProtKB:P1\tinfores:x\tinfores:biobtree",
            ])
            r = kgx.validate(tmp / "nodes.tsv", tmp / "edges.tsv")
            self.assertFalse(r["ok"])
            self.assertEqual(r["dangling_object_edges"], 1)
            self.assertEqual(r["bad_predicate"], 1)


class JsonlAndManifestTests(unittest.TestCase):
    def test_jsonl_and_manifest(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            _write(tmp / "nodes.tsv", kgx.NODE_HEADER, [
                "HGNC:1\tbiolink:Gene\tAAA\tHGNC:1|ENSEMBL:E1\tinfores:biobtree",
            ])
            _write(tmp / "edges.tsv", kgx.EDGE_HEADER, [
                "HGNC:1\tbiolink:has_gene_product\tUniProtKB:P1\tinfores:ensembl\tinfores:biobtree",
            ])
            kgx.nodes_to_jsonl(tmp / "nodes.tsv", tmp / "nodes.jsonl")
            node = json.loads((tmp / "nodes.jsonl").read_text().splitlines()[0])
            self.assertEqual(node["category"], ["biolink:Gene"])
            self.assertEqual(node["equivalent_identifiers"], ["HGNC:1", "ENSEMBL:E1"])

            m = kgx.manifest(tmp / "nodes.tsv", tmp / "edges.tsv", data_version="v5")
            self.assertEqual(m["node_count"], 1)
            self.assertEqual(m["edge_count"], 1)
            self.assertEqual(m["data_version"], "v5")
            self.assertEqual(m["edge_predicates"]["biolink:has_gene_product"], 1)
            self.assertEqual(m["node_categories"]["biolink:Gene"], 1)


if __name__ == "__main__":
    unittest.main()
