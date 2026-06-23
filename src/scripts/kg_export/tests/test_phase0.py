"""Phase 0 tests: dataset id resolution + category map + index-line resolution.

Run from the repo root:
    python3 -m unittest kg_export.tests.test_phase0 -v

Optional real-data smoke test: point BIOBTREE_INDEX_DIR at a built index dir
(e.g. /data/biobtree/out/main/index) to parse a real sorted file; skipped if
unset/absent.
"""

import gzip
import os
import tempfile
import unittest
from pathlib import Path

from kg_export.categories import CategoryMap
from kg_export.datasets import DatasetRegistry
from kg_export.index import (
    IndexParseError,
    iter_index_file,
    parse_index_line,
    resolve_xref,
)

REPO_ROOT = Path(__file__).resolve().parents[4]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = Path(__file__).resolve().parents[1] / "mappings" / "categories.yaml"


class DatasetRegistryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF_DIR)

    def test_loads_without_id_collision(self):
        # load() raises on collision; reaching here means none. Sanity on size.
        self.assertGreater(len(self.reg), 400)

    def test_known_numeric_ids(self):
        # Verified against a real sorted line: HGNC:100  10  41  4
        self.assertEqual(self.reg.name_for_id("10"), "hgnc")
        self.assertEqual(self.reg.name_for_id("4"), "entrez")
        self.assertEqual(self.reg.name_for_id("2"), "ensembl")
        self.assertEqual(self.reg.name_for_id("8"), "refseq")

    def test_unknown_id_returns_none(self):
        self.assertIsNone(self.reg.name_for_id("99999999"))

    def test_metadata_fields(self):
        uni = self.reg.by_name("uniprot")
        self.assertIsNotNone(uni)
        self.assertEqual(uni.numeric_id, "1")
        self.assertIn("UniProtKB", uni.aliases)


class CategoryMapTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.cats = CategoryMap.load(CATEGORIES_YAML)
        cls.reg = DatasetRegistry.load(CONF_DIR)

    def test_core_categories(self):
        self.assertEqual(self.cats.category_for("hgnc"), "biolink:Gene")
        self.assertEqual(self.cats.category_for("uniprot"), "biolink:Protein")
        self.assertEqual(self.cats.category_for("chebi"), "biolink:SmallMolecule")
        self.assertEqual(self.cats.prefix_for("entrez"), "NCBIGene")

    def test_edge_only_dataset_is_not_node(self):
        # string_interaction produces edges, not nodes -> absent from categories.
        self.assertFalse(self.cats.is_node_dataset("string_interaction"))
        self.assertIsNone(self.cats.category_for("string_interaction"))

    def test_every_categorized_dataset_exists_in_config(self):
        """Guards against typos in categories.yaml dataset names."""
        unknown = [d for d in self.cats.datasets() if d not in self.reg]
        self.assertEqual(unknown, [], f"categories.yaml names not in conf: {unknown}")

    def test_canonical_priority_datasets_exist(self):
        for category in self.cats.categories():
            for ds in self.cats.priority_for(category):
                self.assertIn(
                    ds, self.reg, f"priority dataset {ds!r} ({category}) not in conf"
                )


class IndexLineTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)

    def test_parse_minimal(self):
        raw = parse_index_line("HGNC:100\t10\t41\t4")
        self.assertEqual(raw.subject, "HGNC:100")
        self.assertEqual(raw.source_dataset_id, "10")
        self.assertEqual(raw.object, "41")
        self.assertEqual(raw.object_dataset_id, "4")
        self.assertIsNone(raw.evidence)
        self.assertIsNone(raw.relationship)

    def test_property_sentinel(self):
        # object_dataset_id == "-1" marks a node property, not an edge.
        raw = parse_index_line("HGNC:100\t10\t{\"name\":\"x\"}\t-1")
        self.assertTrue(raw.is_property)
        edge = parse_index_line("HGNC:100\t10\t41\t4")
        self.assertFalse(edge.is_property)

    def test_parse_with_evidence_and_relationship(self):
        raw = parse_index_line("A\t1\tB\t2\tECO:0000269\tis_a")
        self.assertEqual(raw.evidence, "ECO:0000269")
        self.assertEqual(raw.relationship, "is_a")

    def test_parse_rejects_short_and_empty(self):
        with self.assertRaises(IndexParseError):
            parse_index_line("A\t1\tB")
        with self.assertRaises(IndexParseError):
            parse_index_line("A\t\tB\t2")

    def test_resolve_to_categories(self):
        raw = parse_index_line("HGNC:100\t10\t41\t4")
        res = resolve_xref(raw, self.reg, self.cats)
        self.assertEqual(res.subject.dataset, "hgnc")
        self.assertEqual(res.subject.category, "biolink:Gene")
        self.assertEqual(res.object.dataset, "entrez")
        self.assertEqual(res.object.category, "biolink:Gene")

    def test_iter_index_file_roundtrip(self):
        lines = ["HGNC:100\t10\t41\t4\n", "HGNC:100\t10\tENSG00000110881\t2\n"]
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "sample_sorted.1.index.gz"
            with gzip.open(p, "wt") as fh:
                fh.writelines(lines)
            rows = list(iter_index_file(p))
        self.assertEqual(len(rows), 2)
        self.assertEqual(rows[1].object_dataset_id, "2")


class RealDataSmokeTest(unittest.TestCase):
    """Optional: parse a real sorted file if an index dir is available."""

    def setUp(self):
        self.index_dir = os.environ.get("BIOBTREE_INDEX_DIR")
        if not self.index_dir:
            default = Path("/data/biobtree/out/main/index")
            self.index_dir = str(default) if default.exists() else None
        if not self.index_dir:
            self.skipTest("no index dir (set BIOBTREE_INDEX_DIR)")

    def test_first_lines_resolve(self):
        reg = DatasetRegistry.load(CONF_DIR)
        cats = CategoryMap.load(CATEGORIES_YAML)
        files = sorted(Path(self.index_dir).glob("hgnc_sorted.*.index.gz"))
        if not files:
            self.skipTest("no hgnc_sorted file in index dir")
        edges = 0
        props = 0
        for raw in iter_index_file(files[0]):
            self.assertEqual(reg.name_for_id(raw.source_dataset_id), "hgnc")
            if raw.is_property:
                props += 1
                continue
            res = resolve_xref(raw, reg, cats)
            # Every non-property object dataset id must resolve to a known dataset.
            self.assertIsNotNone(
                res.object.dataset,
                f"unresolved object dataset id {raw.object_dataset_id!r}",
            )
            edges += 1
            if edges >= 200:
                break
        self.assertGreater(edges, 0)
        # hgnc carries node properties (names etc.) interleaved with edges.
        self.assertGreater(props + edges, 0)


if __name__ == "__main__":
    unittest.main()
