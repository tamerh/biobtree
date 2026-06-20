"""Showcase tests: gene/compound resolution + bioactivity extraction.

    python3 -m unittest tools.kg_export.tests.test_showcase -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.showcase import build_showcase

REPO = Path(__file__).resolve().parents[3]
CONF = REPO / "conf"
CATS = REPO / "mappings" / "categories.yaml"


class ShowcaseTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF)
        cls.cats = CategoryMap.load(CATS)

    def _id(self, n):
        return self.reg.by_name(n).numeric_id

    def _write(self, d, name, lines):
        with gzip.open(d / name, "wt") as fh:
            fh.write("".join(l + "\n" for l in lines))

    def test_resolve_and_bioactivity(self):
        en, up, hg, cm, pc, ca = (self._id("entrez"), self._id("uniprot"), self._id("hgnc"),
                                  self._id("chembl_molecule"), self._id("pubchem"),
                                  self._id("chembl_activity"))
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            # TP53 human gene (entrez 7157), mouse Trp53 (must be filtered out by tax_id)
            self._write(tmp, "entrez_sorted.1.index.gz", [
                f'7157\t{en}\t{{"symbol":"TP53","tax_id":"9606"}}\t-1',
                f'22059\t{en}\t{{"symbol":"TP53","tax_id":"10090"}}\t-1',   # mouse -> excluded
            ])
            self._write(tmp, "uniprot_from_hgnc_sorted.1.index.gz", [
                f"P04637\t{up}\tHGNC:11998\t{hg}",                          # TP53 protein
            ])
            self._write(tmp, "chembl_molecule_sorted.1.index.gz", [
                f'CHEMBL25\t{cm}\t{{"molecule":{{"name":"ASPIRIN"}}}}\t-1',
            ])
            self._write(tmp, "pubchem_sorted.1.index.gz", [
                f'2244\t{pc}\t{{"title":"x","synonyms":["aspirin","2-acetoxybenzoic acid"]}}\t-1',
            ])
            self._write(tmp, "chembl_activity_sorted.1.index.gz", [
                f"ACT1\t{ca}\tCHEMBL25\t{cm}", f"ACT1\t{ca}\tP04637\t{up}",
                f'ACT1\t{ca}\t{{"x":1}}\t-1',
            ])
            id_map = {"NCBIGene:7157": "HGNC:11998"}
            gf = tmp / "genes.txt"
            stats = build_showcase(tmp, self.reg, self.cats, {"genes": ["TP53"], "compounds": ["aspirin"]},
                                   id_map, tmp / "sc_n.tsv", tmp / "sc_e.tsv", gf)

            self.assertEqual(stats.genes_resolved, 1)            # only human TP53
            self.assertEqual(gf.read_text().split(), ["7157"])   # entrez filter for dbSNP
            self.assertEqual(stats.proteins, 1)                  # P04637
            self.assertEqual(stats.compounds_resolved, 2)        # CHEMBL25 + PUBCHEM 2244

            edges = {(p[1], p[2], p[3]) for p in
                     (l.split("\t") for l in (tmp / "sc_e.tsv").read_text().splitlines()[1:])}
            self.assertIn(("CHEMBL.COMPOUND:CHEMBL25", "biolink:interacts_with", "UniProtKB:P04637"), edges)

    def test_bioactivity_kept_by_protein_match(self):
        """An edge survives when only the TARGET protein is a showcase gene's product
        (compound not in the list)."""
        en, up, hg, cm, ca = (self._id("entrez"), self._id("uniprot"), self._id("hgnc"),
                              self._id("chembl_molecule"), self._id("chembl_activity"))
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "entrez_sorted.1.index.gz", [f'7157\t{en}\t{{"symbol":"TP53","tax_id":"9606"}}\t-1'])
            self._write(tmp, "uniprot_from_hgnc_sorted.1.index.gz", [f"P04637\t{up}\tHGNC:11998\t{hg}"])
            self._write(tmp, "chembl_activity_sorted.1.index.gz", [
                f"ACT1\t{ca}\tCHEMBL999\t{cm}", f"ACT1\t{ca}\tP04637\t{up}",  # random compound, TP53 target
            ])
            stats = build_showcase(tmp, self.reg, self.cats, {"genes": ["TP53"], "compounds": []},
                                   {"NCBIGene:7157": "HGNC:11998"}, tmp / "n.tsv", tmp / "e.tsv", tmp / "g.txt")
            self.assertEqual(stats.bioactivity_edges, 1)         # kept via protein match


if __name__ == "__main__":
    unittest.main()
