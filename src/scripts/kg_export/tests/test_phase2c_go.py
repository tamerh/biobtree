"""Phase 2c tests: GO term typing + aspect-dependent annotation edges.

    python3 -m unittest kg_export.tests.test_phase2c_go -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from kg_export.categories import CategoryMap
from kg_export.datasets import DatasetRegistry
from kg_export.edges import load_id_map
from kg_export.go import build_go, build_go_terms

REPO_ROOT = Path(__file__).resolve().parents[4]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = Path(__file__).resolve().parents[1] / "mappings" / "categories.yaml"


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
                f"P1\t{up}\tGO:0003674\t{go}\tECO:0000314",  # MF -> enables, w/ evidence
                f"P1\t{up}\tGO:0005575\t{go}",   # CC -> located_in, no evidence
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

            # cols: id, subject, predicate, object, ...
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in e_out.read_text().splitlines()[1:]}
            self.assertIn(("UniProtKB:P1", "biolink:enables", "GO:0003674"), edges)
            self.assertIn(("UniProtKB:P1", "biolink:located_in", "GO:0005575"), edges)
            # ensembl gene canonicalized to HGNC, BP -> actively_involved_in
            self.assertIn(("HGNC:1", "biolink:actively_involved_in", "GO:0008150"), edges)
            self.assertEqual(stats.edges_written, 3)
            self.assertEqual(stats.nodes_written, 3)
            # ECO evidence forwarded to has_evidence (col 9); only the MF edge has it
            rows = {r.split("\t")[1: 4][0] + "|" + r.split("\t")[2]: r.split("\t")
                    for r in e_out.read_text().splitlines()[1:]}
            mf = rows["UniProtKB:P1|biolink:enables"]
            self.assertEqual(mf[8], "ECO:0000314")
            self.assertEqual(stats.edges_with_evidence, 1)


if __name__ == "__main__":
    unittest.main()
