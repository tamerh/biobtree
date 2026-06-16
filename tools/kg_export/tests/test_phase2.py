"""Phase 2 tests: predicate map + edge building.

    python3 -m unittest tools.kg_export.tests.test_phase2 -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.edges import build_edges, load_id_map
from tools.kg_export.predicates import PredicateMap

REPO_ROOT = Path(__file__).resolve().parents[3]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = REPO_ROOT / "mappings" / "categories.yaml"
PREDICATES_YAML = REPO_ROOT / "mappings" / "predicates.yaml"


class PredicateMapTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.pm = PredicateMap.load(PREDICATES_YAML)
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)

    def test_direct_rule(self):
        r = self.pm.rule_for("ensembl", "uniprot")
        self.assertEqual(r.predicate, "biolink:has_gene_product")
        self.assertFalse(r.flip)
        self.assertFalse(r.is_skip)

    def test_flip_rule(self):
        r = self.pm.rule_for("reactome", "ensembl")
        self.assertTrue(r.flip)
        self.assertEqual(r.predicate, "biolink:participates_in")

    def test_skip_rule(self):
        self.assertTrue(self.pm.rule_for("ensembl", "entrez").is_skip)

    def test_authored_pairs_reference_real_datasets(self):
        for key in self.pm.pairs():
            src, tgt = key.split(">")
            self.assertIn(src, self.reg, f"{key}: unknown src dataset")
            self.assertIn(tgt, self.reg, f"{key}: unknown tgt dataset")

    def test_emitting_pairs_are_node_to_node(self):
        # Any pair we actually EMIT must have two typed (node) endpoints.
        for key in self.pm.pairs():
            r = self.pm.rule_for(*key.split(">"))
            if r.is_skip:
                continue
            src, tgt = key.split(">")
            self.assertTrue(self.cats.is_node_dataset(src), f"{key}: src not a node")
            self.assertTrue(self.cats.is_node_dataset(tgt), f"{key}: tgt not a node")


class BuildEdgesTests(unittest.TestCase):
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

    def test_build_edges_end_to_end(self):
        cl, hg, en, up, re, ip, rs = (
            self._id("clinvar"), self._id("hgnc"), self._id("ensembl"),
            self._id("uniprot"), self._id("reactome"), self._id("interpro"),
            self._id("refseq"),
        )
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            # id_map: ensembl gene canonicalizes to its HGNC node
            (tmp / "id_map.tsv").write_text("member\tcanonical\nENSEMBL:ENSG1\tHGNC:1\n")
            id_map = load_id_map(tmp / "id_map.tsv")

            self._write(tmp, "clinvar_sorted.1.index.gz", [
                'VAR1\t' + cl + '\t{"x":1}\t-1',          # property -> skipped
                f"VAR1\t{cl}\tHGNC:1\t{hg}",              # variant -> gene
            ])
            self._write(tmp, "ensembl_sorted.1.index.gz", [
                f"ENSG1\t{en}\tP1\t{up}",                 # gene -> protein (canon HGNC:1)
                f"ENSG1\t{en}\t999\t{self._id('entrez')}",  # identity -> skip
            ])
            self._write(tmp, "reactome_sorted.1.index.gz", [
                f"R-1\t{re}\tENSG1\t{en}",                # flip -> gene participates_in pathway
            ])
            self._write(tmp, "interpro_sorted.1.index.gz", [
                f"IPR1\t{ip}\tP1\t{up}",                  # interpro>uniprot: unmapped
            ])
            self._write(tmp, "hgnc_sorted.1.index.gz", [
                f"HGNC:1\t{hg}\tNM_1\t{rs}",              # hgnc>refseq: refseq not a node
            ])

            out = tmp / "edges.tsv"
            stats = build_edges(
                tmp, self.reg, self.cats, self.pm, out,
                id_map=id_map, stats_path=tmp / "edges.stats.json",
            )
            rows = [tuple(l.split("\t")) for l in out.read_text().splitlines()[1:]]

            edges = {(s, p, o) for s, p, o, _, _ in rows}
            self.assertIn(("CLINVAR:VAR1", "biolink:is_sequence_variant_of", "HGNC:1"), edges)
            # ensembl gene rewritten to canonical HGNC:1
            self.assertIn(("HGNC:1", "biolink:has_gene_product", "UniProtKB:P1"), edges)
            # flip: subject is the (canonicalized) gene, object is the pathway
            self.assertIn(("HGNC:1", "biolink:participates_in", "REACT:R-1"), edges)

            self.assertEqual(stats.edges_written, 3)
            self.assertEqual(stats.property_lines, 1)
            self.assertEqual(stats.skipped, 1)        # ensembl>entrez identity
            self.assertEqual(stats.unmapped, 1)       # interpro>uniprot
            self.assertEqual(stats.dropped_not_node, 1)  # hgnc>refseq
            self.assertEqual(stats.unmapped_pairs["interpro>uniprot"], 1)

    def test_primary_knowledge_source(self):
        cl, hg = self._id("clinvar"), self._id("hgnc")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "clinvar_sorted.1.index.gz", [f"VAR1\t{cl}\tHGNC:1\t{hg}"])
            out = tmp / "edges.tsv"
            build_edges(tmp, self.reg, self.cats, self.pm, out)
            row = out.read_text().splitlines()[1].split("\t")
            self.assertEqual(row[3], "infores:clinvar")
            self.assertEqual(row[4], "infores:biobtree")


if __name__ == "__main__":
    unittest.main()
