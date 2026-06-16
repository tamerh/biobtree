"""Tests for code-review fixes (resilience, CURIE safety, cross-chunk reified).

    python3 -m unittest tools.kg_export.tests.test_review_fixes -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.curie import to_curie
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.index import iter_index_file
from tools.kg_export.nodes import tsv_safe
from tools.kg_export.predicates import PredicateMap
from tools.kg_export.reified import build_reified_edges

REPO_ROOT = Path(__file__).resolve().parents[3]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = REPO_ROOT / "mappings" / "categories.yaml"
PREDICATES_YAML = REPO_ROOT / "mappings" / "predicates.yaml"


class ResilienceTests(unittest.TestCase):
    def test_malformed_line_skipped_and_counted(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "x_sorted.1.index.gz"
            with gzip.open(p, "wt") as fh:
                fh.write("A\t1\tB\t2\n")     # ok
                fh.write("garbage line\n")   # malformed (1 field)
                fh.write("\n")               # blank -> skipped, not counted
                fh.write("C\t3\tD\t4\n")     # ok
            counter = {}
            rows = list(iter_index_file(p, counter))
            self.assertEqual(len(rows), 2)               # did not abort
            self.assertEqual(counter.get("malformed"), 1)  # one bad line tallied


class CurieSafetyTests(unittest.TestCase):
    def test_foreign_curie_not_double_prefixed(self):
        # a bare id carrying a foreign prefix must not become HMDB:CHEBI:1
        self.assertEqual(to_curie("HMDB", "CHEBI:1"), "CHEBI:1")
        self.assertEqual(to_curie("NCBIGene", "41"), "NCBIGene:41")
        self.assertEqual(to_curie("HGNC", "HGNC:5"), "HGNC:5")


class TsvSafeTests(unittest.TestCase):
    def test_strips_tab_newline(self):
        self.assertEqual(tsv_safe("a\tb\nc\rd"), "a b c d")


class ReifiedCrossChunkTests(unittest.TestCase):
    def test_subject_split_across_chunks_is_joined(self):
        # star: a query's hits split across two chunks must group as one entry
        reg = DatasetRegistry.load(CONF_DIR)
        cats = CategoryMap.load(CATEGORIES_YAML)
        pm = PredicateMap.load(PREDICATES_YAML)
        dia = reg.by_name("diamond_similarity").numeric_id
        up = reg.by_name("uniprot").numeric_id
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            with gzip.open(tmp / "diamond_similarity_sorted.1.index.gz", "wt") as fh:
                fh.write(f"Q1\t{dia}\tHIT1\t{up}\n")
            with gzip.open(tmp / "diamond_similarity_sorted.2.index.gz", "wt") as fh:
                fh.write(f"Q1\t{dia}\tHIT2\t{up}\n")
            out = tmp / "r.tsv"
            build_reified_edges(tmp, reg, cats, pm, out, datasets=["diamond_similarity"])
            edges = {(l.split("\t")[1], l.split("\t")[3])
                     for l in out.read_text().splitlines()[1:]}
            self.assertEqual(edges, {
                ("UniProtKB:Q1", "UniProtKB:HIT1"),
                ("UniProtKB:Q1", "UniProtKB:HIT2"),
            })


if __name__ == "__main__":
    unittest.main()
