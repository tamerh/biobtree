"""General node-attribute layer tests (entry attrs -> node properties).

    python3 -m unittest tools.kg_export.tests.test_nodeattrs -v
"""

import gzip
import json
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.nodeattrs import build_node_attributes

REPO = Path(__file__).resolve().parents[3]
CONF = REPO / "conf"
CATS = REPO / "mappings" / "categories.yaml"


class NodeAttrTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF)
        cls.cats = CategoryMap.load(CATS)

    def _run(self, ds, lines, config, id_map=None):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            with gzip.open(tmp / f"{ds}_sorted.1.index.gz", "wt") as fh:
                fh.write("".join(l + "\n" for l in lines))
            out = tmp / "na.tsv"
            build_node_attributes(tmp, self.reg, self.cats, config, out, id_map=id_map or {})
            rows = {}
            for line in out.read_text().splitlines():
                node, js = line.split("\t", 1)
                rows[node] = json.loads(js)
            return rows

    def test_all_mode_flatten_lists_and_skip_objects(self):
        nid = self.reg.by_name("chebi").numeric_id
        prop = json.dumps({
            "name": "X", "star_rating": 3, "synonyms": ["a", "b"],
            "sequence": {"sub": 1, "sublist": ["p"]},     # nested dict -> flattened
            "rels": [{"k": 1}],                            # list of dicts -> skipped
        })
        rows = self._run("chebi", [f"CHEBI:1\t{nid}\t{prop}\t-1"], {"defaults": {"mode": "all"}, "datasets": {"chebi": {}}})
        a = rows["CHEBI:1"]
        self.assertEqual(a["chebi_name"], "X")
        self.assertEqual(a["chebi_star_rating"], 3)
        self.assertEqual(a["chebi_synonyms"], ["a", "b"])
        self.assertEqual(a["chebi_sequence_sub"], 1)        # one-level flatten
        self.assertEqual(a["chebi_sequence_sublist"], ["p"])
        self.assertNotIn("chebi_rels", a)                   # list-of-objects dropped

    def test_compact_mode_uses_conf_compact_fields(self):
        # chebi compact_fields = name,formula,star_rating
        nid = self.reg.by_name("chebi").numeric_id
        prop = json.dumps({"name": "X", "formula": "C6", "star_rating": 2, "definition": "long"})
        rows = self._run("chebi", [f"CHEBI:9\t{nid}\t{prop}\t-1"], {"datasets": {"chebi": {"mode": "compact"}}})
        self.assertEqual(rows["CHEBI:9"], {"chebi_name": "X", "chebi_formula": "C6", "chebi_star_rating": 2})

    def test_exclude_field(self):
        nid = self.reg.by_name("chebi").numeric_id
        prop = json.dumps({"name": "X", "definition": "drop me"})
        rows = self._run("chebi", [f"CHEBI:2\t{nid}\t{prop}\t-1"],
                         {"defaults": {"mode": "all"}, "datasets": {"chebi": {"exclude": ["definition"]}}})
        self.assertIn("chebi_name", rows["CHEBI:2"])
        self.assertNotIn("chebi_definition", rows["CHEBI:2"])

    def test_id_map_canonicalizes_to_merged_node(self):
        """entrez gene attrs attach to the canonical (HGNC) gene node via id_map,
        and keys are dataset-prefixed so they don't collide with ensembl/hgnc attrs."""
        nid = self.reg.by_name("entrez").numeric_id
        prop = json.dumps({"symbol": "BRCA1", "type": "protein-coding", "chromosome": "17"})
        rows = self._run("entrez", [f"672\t{nid}\t{prop}\t-1"],
                         {"defaults": {"mode": "all"}, "datasets": {"entrez": {}}},
                         id_map={"NCBIGene:672": "HGNC:1100"})
        self.assertIn("HGNC:1100", rows)              # canonicalized
        self.assertEqual(rows["HGNC:1100"]["entrez_symbol"], "BRCA1")
        self.assertEqual(rows["HGNC:1100"]["entrez_chromosome"], "17")


if __name__ == "__main__":
    unittest.main()
