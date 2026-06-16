"""Phase 2c tests: GO term typing + aspect-dependent annotation edges.

    python3 -m unittest tools.kg_export.tests.test_phase2c_go -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.edges import load_id_map
from tools.kg_export.go import build_go, build_go_terms

REPO_ROOT = Path(__file__).resolve().parents[3]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = REPO_ROOT / "mappings" / "categories.yaml"


class GoTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)

    def _id(self, name):
        return self.reg.by_name(name).numeric_id

    def _write(self, d, name, lines):
        with gzip.open(d / name, "wt") as fh:
            fh.write("".join(l + "\n" for l in lines))

    def _go_file(self, tmp):
        go = self._id("go")
        self._write(tmp, "go_sorted.1.index.gz", [
            f'GO:0003674\t{go}\t{{"type":"molecular_function","name":"molecular_function"}}\t-1',
            f'GO:0008150\t{go}\t{{"type":"biological_process","name":"biological_process"}}\t-1',
            f'GO:0005575\t{go}\t{{"type":"cellular_component","name":"cellular_component"}}\t-1',
        ])

    def test_build_go_terms(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._go_file(tmp)
            terms = build_go_terms(tmp, self.reg)
            self.assertEqual(terms["GO:0003674"], ("molecular_function", "molecular_function"))
            self.assertEqual(terms["GO:0008150"][0], "biological_process")

    def test_nodes_and_edges_aspect_mapping(self):
        go, up, en = self._id("go"), self._id("uniprot"), self._id("ensembl")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._go_file(tmp)
            (tmp / "id_map.tsv").write_text("member\tcanonical\nENSEMBL:ENSG1\tHGNC:1\n")
            id_map = load_id_map(tmp / "id_map.tsv")
            self._write(tmp, "uniprot_sorted.1.index.gz", [
                f"P1\t{up}\tGO:0003674\t{go}",   # MF -> enables
                f"P1\t{up}\tGO:0005575\t{go}",   # CC -> located_in
            ])
            self._write(tmp, "ensembl_sorted.1.index.gz", [
                f"ENSG1\t{en}\tGO:0008150\t{go}",  # BP -> involved; gene canonicalized
            ])
            n_out, e_out = tmp / "go_nodes.tsv", tmp / "go_edges.tsv"
            stats = build_go(tmp, self.reg, self.cats, n_out, e_out, id_map=id_map)

            nodes = {r.split("\t")[0]: r.split("\t")[1]
                     for r in n_out.read_text().splitlines()[1:]}
            self.assertEqual(nodes["GO:0003674"], "biolink:MolecularActivity")
            self.assertEqual(nodes["GO:0008150"], "biolink:BiologicalProcess")
            self.assertEqual(nodes["GO:0005575"], "biolink:CellularComponent")

            edges = {(r.split("\t")[0], r.split("\t")[1], r.split("\t")[2])
                     for r in e_out.read_text().splitlines()[1:]}
            self.assertIn(("UniProtKB:P1", "biolink:enables", "GO:0003674"), edges)
            self.assertIn(("UniProtKB:P1", "biolink:located_in", "GO:0005575"), edges)
            # ensembl gene canonicalized to HGNC, BP -> actively_involved_in
            self.assertIn(("HGNC:1", "biolink:actively_involved_in", "GO:0008150"), edges)
            self.assertEqual(stats.edges_written, 3)
            self.assertEqual(stats.nodes_written, 3)


if __name__ == "__main__":
    unittest.main()
