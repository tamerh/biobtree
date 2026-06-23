"""Phase 3 tests: merge, JSONL serialization, validation, manifest.

    python3 -m unittest kg_export.tests.test_phase3 -v
"""

import json
import tempfile
import unittest
from pathlib import Path

from kg_export import kgx


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

    def test_merge_nodes_dedup_by_id_different_content(self):
        """Same id from two files (different content) collapses to exactly one node
        (sort-based dedup, memory-flat)."""
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            _write(tmp / "n1.tsv", kgx.NODE_HEADER, [
                "DBSNP:RS1\tbiolink:SequenceVariant\trs1\tDBSNP:RS1\tinfores:biobtree",
            ])
            _write(tmp / "n2.tsv", kgx.NODE_HEADER, [
                "DBSNP:RS1\tbiolink:SequenceVariant\t\tDBSNP:RS1\tinfores:biobtree",  # boundary dup, no name
            ])
            out = tmp / "nodes.tsv"
            n = kgx.merge_nodes([tmp / "n1.tsv", tmp / "n2.tsv"], out)
            ids = [l.split("\t")[0] for l in out.read_text().splitlines()[1:]]
            self.assertEqual(n, 1)
            self.assertEqual(ids, ["DBSNP:RS1"])

    def test_merge_edges_dedup_by_id(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            # same logical edge from two sources -> same id -> collapses to one;
            # a different edge survives.
            e1 = kgx.format_edge("UniProtKB:A", "biolink:physically_interacts_with",
                                 "UniProtKB:B", "infores:intact").rstrip("\n")
            e2 = kgx.format_edge("UniProtKB:A", "biolink:physically_interacts_with",
                                 "UniProtKB:C", "infores:intact").rstrip("\n")
            _write(tmp / "e1.tsv", kgx.EDGE_HEADER, [e1, e2])
            _write(tmp / "e2.tsv", kgx.EDGE_HEADER, [e1])  # duplicate of e1
            out = tmp / "edges.tsv"
            res = kgx.merge_edges([tmp / "e1.tsv", tmp / "e2.tsv"], out)
            self.assertEqual(res["input"], 3)
            self.assertEqual(res["written"], 2)
            self.assertEqual(res["removed"], 1)
            _write(tmp / "nodes.tsv", kgx.NODE_HEADER, [
                "UniProtKB:A\tbiolink:Protein\ta\tUniProtKB:A\tinfores:biobtree",
                "UniProtKB:B\tbiolink:Protein\tb\tUniProtKB:B\tinfores:biobtree",
                "UniProtKB:C\tbiolink:Protein\tc\tUniProtKB:C\tinfores:biobtree",
            ])
            r = kgx.validate(tmp / "nodes.tsv", out)
            self.assertEqual(r["duplicate_edges"], 0)


class StubNodeTests(unittest.TestCase):
    def test_stub_nodes_typed_from_prefix(self):
        from kg_export.categories import CategoryMap
        cats = CategoryMap.load(
            Path(__file__).resolve().parents[1] / "mappings" / "categories.yaml")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            _write(tmp / "nodes.tsv", kgx.NODE_HEADER, [
                "HGNC:1\tbiolink:Gene\tg\tHGNC:1\tinfores:biobtree",
            ])
            _write(tmp / "edges.tsv", kgx.EDGE_HEADER, [
                # endpoints not in nodes: an ensembl gene, a protein, an untypeable id
                kgx.format_edge("HGNC:1", "biolink:has_gene_product",
                                "UniProtKB:P1", "infores:ensembl").rstrip("\n"),
                kgx.format_edge("ENSEMBL:ENSG9", "biolink:expressed_in",
                                "UBERON:1", "infores:bgee").rstrip("\n"),
                kgx.format_edge("ENSEMBL:ENST9", "biolink:translates_to",
                                "UniProtKB:P2", "infores:x").rstrip("\n"),
                kgx.format_edge("JUNK:1", "biolink:related_to",
                                "HGNC:1", "infores:x").rstrip("\n"),
            ])
            info = kgx.add_stub_nodes(tmp / "nodes.tsv", tmp / "edges.tsv", cats)
            nodes = {l.split("\t")[0]: l.split("\t")[1]
                     for l in (tmp / "nodes.tsv").read_text().splitlines()[1:]}
            self.assertEqual(nodes["UniProtKB:P1"], "biolink:Protein")
            self.assertEqual(nodes["ENSEMBL:ENSG9"], "biolink:Gene")
            self.assertEqual(nodes["ENSEMBL:ENST9"], "biolink:Transcript")  # pattern
            self.assertEqual(nodes["UBERON:1"], "biolink:GrossAnatomicalStructure")
            self.assertNotIn("JUNK:1", nodes)  # untypeable -> left dangling
            self.assertEqual(info["untyped_endpoints"], 1)
            # after stubs, validation has no dangling except the untyped JUNK:1
            r = kgx.validate(tmp / "nodes.tsv", tmp / "edges.tsv")
            self.assertEqual(r["dangling_subject_edges"], 1)  # JUNK:1 subject
            self.assertEqual(r["dangling_object_edges"], 0)


class ValidateTests(unittest.TestCase):
    def _kg(self, tmp, edges):
        _write(tmp / "nodes.tsv", kgx.NODE_HEADER, [
            "HGNC:1\tbiolink:Gene\tAAA\tHGNC:1\tinfores:biobtree",
            "UniProtKB:P1\tbiolink:Protein\tp1\tUniProtKB:P1\tinfores:biobtree",
        ])
        _write(tmp / "edges.tsv", kgx.EDGE_HEADER,
               [e.rstrip("\n") for e in edges])

    def test_clean(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._kg(tmp, [
                kgx.format_edge("HGNC:1", "biolink:has_gene_product",
                                "UniProtKB:P1", "infores:ensembl"),
            ])
            r = kgx.validate(tmp / "nodes.tsv", tmp / "edges.tsv")
            self.assertTrue(r["ok"], r)
            self.assertEqual(r["edges"], 1)

    def test_dangling_and_bad_predicate(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._kg(tmp, [
                kgx.format_edge("HGNC:1", "biolink:has_gene_product",
                                "UniProtKB:MISSING", "infores:x"),
                kgx.format_edge("HGNC:1", "related_to", "UniProtKB:P1", "infores:x"),
            ])
            r = kgx.validate(tmp / "nodes.tsv", tmp / "edges.tsv")
            self.assertFalse(r["ok"])
            self.assertEqual(r["dangling_object_edges"], 1)
            self.assertEqual(r["bad_predicate"], 1)

    def test_non_biolink_prefix_flagged(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            _write(tmp / "nodes.tsv", kgx.NODE_HEADER, [
                "HGNC:1\tbiolink:Gene\tg\tHGNC:1\tinfores:biobtree",       # ok
                "cellosaurus:CVCL_1\tbiolink:CellLine\tc\tcellosaurus:CVCL_1\tinfores:biobtree",  # canonical now
                "SWISSLIPID:10\tbiolink:SmallMolecule\ts\tSWISSLIPID:10\tinfores:biobtree",  # still non-canonical
            ])
            _write(tmp / "edges.tsv", kgx.EDGE_HEADER, [])
            r = kgx.validate(tmp / "nodes.tsv", tmp / "edges.tsv")
            self.assertEqual(r["non_biolink_prefixes"], {"SWISSLIPID": 1})
            self.assertNotIn("cellosaurus", r["non_biolink_prefixes"])
            self.assertNotIn("HGNC", r["non_biolink_prefixes"])

    def test_bad_category_detected(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            _write(tmp / "nodes.tsv", kgx.NODE_HEADER, [
                "X:1\tbiolink:GeneSet\tx\tX:1\tinfores:biobtree",  # invalid class
            ])
            _write(tmp / "edges.tsv", kgx.EDGE_HEADER, [])
            r = kgx.validate(tmp / "nodes.tsv", tmp / "edges.tsv")
            self.assertEqual(r["bad_category"], 1)
            self.assertFalse(r["ok"])


class StreamingValidateTests(unittest.TestCase):
    """validate_streaming: billion-scale gate -- shape checks streamed, dangling/dup
    taken from the construction (merge/stub) stats instead of giant in-memory sets."""

    def _kg(self, tmp, nodes, edges):
        _write(tmp / "nodes.tsv", kgx.NODE_HEADER, nodes)
        _write(tmp / "edges.tsv", kgx.EDGE_HEADER, edges)

    def test_streaming_clean(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._kg(tmp, [
                "HGNC:1\tbiolink:Gene\tg\tHGNC:1\tinfores:biobtree",
                "DBSNP:RS1\tbiolink:SequenceVariant\trs1\tDBSNP:RS1\tinfores:biobtree",
            ], [kgx.format_edge("DBSNP:RS1", "biolink:is_sequence_variant_of",
                                "HGNC:1", "infores:dbsnp").rstrip("\n")])
            r = kgx.validate_streaming(tmp / "nodes.tsv", tmp / "edges.tsv",
                                       removed_edges=12, stub_untyped=0)
            self.assertEqual(r["mode"], "streaming")
            self.assertTrue(r["ok"], r)
            self.assertEqual(r["edges"], 1)
            self.assertEqual(r["duplicate_edges_removed_at_merge"], 12)

    def test_streaming_untyped_dangling_fails(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._kg(tmp, ["HGNC:1\tbiolink:Gene\tg\tHGNC:1\tinfores:biobtree"], [])
            r = kgx.validate_streaming(tmp / "nodes.tsv", tmp / "edges.tsv",
                                       stub_untyped=3)
            self.assertEqual(r["untyped_dangling_endpoints"], 3)
            self.assertFalse(r["ok"])

    def test_streaming_bad_predicate_and_category(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._kg(tmp, ["X:1\tbiolink:GeneSet\tx\tX:1\tinfores:biobtree"],  # bad category
                     [kgx.format_edge("X:1", "related_to", "X:1", "infores:x").rstrip("\n")])
            r = kgx.validate_streaming(tmp / "nodes.tsv", tmp / "edges.tsv")
            self.assertEqual(r["bad_category"], 1)
            self.assertEqual(r["bad_predicate"], 1)
            self.assertFalse(r["ok"])


class JsonlAndManifestTests(unittest.TestCase):
    def test_jsonl_and_manifest(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            _write(tmp / "nodes.tsv", kgx.NODE_HEADER, [
                "HGNC:1\tbiolink:Gene\tAAA\tHGNC:1|ENSEMBL:E1\tinfores:biobtree",
            ])
            _write(tmp / "edges.tsv", kgx.EDGE_HEADER, [
                kgx.format_edge("HGNC:1", "biolink:has_gene_product",
                                "UniProtKB:P1", "infores:ensembl").rstrip("\n"),
            ])
            kgx.nodes_to_jsonl(tmp / "nodes.tsv", tmp / "nodes.jsonl")
            node = json.loads((tmp / "nodes.jsonl").read_text().splitlines()[0])
            self.assertEqual(node["category"], ["biolink:Gene", "biolink:NamedThing"])
            self.assertEqual(node["equivalent_identifiers"], ["HGNC:1", "ENSEMBL:E1"])

            kgx.edges_to_jsonl(tmp / "edges.tsv", tmp / "edges.jsonl")
            edge = json.loads((tmp / "edges.jsonl").read_text().splitlines()[0])
            self.assertEqual(edge["subject"], "HGNC:1")
            self.assertEqual(edge["knowledge_level"], "not_provided")
            self.assertTrue(edge["id"].startswith("biobtree:"))

            m = kgx.manifest(tmp / "nodes.tsv", tmp / "edges.tsv", data_version="v5")
            self.assertEqual(m["node_count"], 1)
            self.assertEqual(m["edge_count"], 1)
            self.assertEqual(m["data_version"], "v5")
            self.assertEqual(m["edge_predicates"]["biolink:has_gene_product"], 1)
            self.assertEqual(m["node_categories"]["biolink:Gene"], 1)
            self.assertIn("license", m)


if __name__ == "__main__":
    unittest.main()
