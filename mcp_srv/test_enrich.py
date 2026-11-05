"""Unit tests for the gene-set enrichment helper (no network)."""

import unittest

from mcp_srv.enrich import _bh_fdr, _hypergeom_sf, enrich_from_mappings


class TestEnrich(unittest.TestCase):
    def test_hypergeom_strong_overlap(self):
        # 10 of 20 query genes in a 100-gene set is extremely unlikely by chance.
        self.assertLess(_hypergeom_sf(10, 43633, 100, 20), 1e-15)

    def test_hypergeom_weak_overlap(self):
        self.assertGreater(_hypergeom_sf(1, 43633, 100, 20), 0.01)

    def test_bh_fdr(self):
        q = _bh_fdr([0.01, 0.02, 0.5])
        self.assertAlmostEqual(q[0], 0.03)
        self.assertAlmostEqual(q[1], 0.03)
        self.assertAlmostEqual(q[2], 0.5)

    def test_enrich_from_mappings(self):
        mappings = [
            {"input": "A", "source": "HGNC:1|A", "targets": ["M1|GOBP_X|C5:GO|d|100"]},
            {"input": "B", "source": "HGNC:2|B", "targets": ["M1|GOBP_X|C5:GO|d|100"]},
            {"input": "A", "source": "HGNC:1|A", "targets": ["M2|GOBP_Y|C5:GO|d|10"]},
        ]
        out = enrich_from_mappings(mappings, collection="C5:GO", limit=5)
        self.assertEqual(out["query"]["mapped_genes"], 2)
        self.assertEqual(out["results"][0]["id"], "M2")  # smaller set, higher fold
        self.assertEqual(out["results"][0]["overlap"], 1)

    def test_alias_dedup(self):
        # TP53 and HGNC:11998 resolve to the same source and count once.
        mappings = [
            {"input": "TP53", "source": "HGNC:11998|p53", "targets": ["M1|GOBP_X|C5:GO|d|100"]},
            {"input": "HGNC:11998", "source": "HGNC:11998|p53", "targets": ["M1|GOBP_X|C5:GO|d|100"]},
        ]
        out = enrich_from_mappings(mappings, collection="C5:GO", limit=5)
        self.assertEqual(out["query"]["mapped_genes"], 1)
        self.assertEqual(out["results"][0]["overlap"], 1)

    def test_collection_namespace(self):
        mappings = [
            {"input": "A", "source": "HGNC:1|A", "targets": [
                "M1|GOBP_X|C5:GO|d|100",
                "M2|GOMF_Y|C5:GO|d|100",
            ]},
        ]
        out = enrich_from_mappings(mappings, collection="C5:GO:BP", limit=5)
        self.assertEqual([r["id"] for r in out["results"]], ["M1"])


if __name__ == "__main__":
    unittest.main()
