"""Phase 2d tests: RefSeq type-split nodes + typed edges.

    python3 -m unittest kg_export.tests.test_phase2d_refseq -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from kg_export.categories import CategoryMap
from kg_export.datasets import DatasetRegistry
from kg_export.refseq import build_refseq

REPO_ROOT = Path(__file__).resolve().parents[4]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = Path(__file__).resolve().parents[1] / "mappings" / "categories.yaml"


class RefseqBuildTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)

    def _id(self, name):
        return self.reg.by_name(name).numeric_id

    def _write(self, d, lines):
        with gzip.open(d / "refseq_sorted.1.index.gz", "wt") as fh:
            fh.write("".join(l + "\n" for l in lines))

    def _run(self, lines, id_map=None):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, lines)
            nout, eout = tmp / "n.tsv", tmp / "e.tsv"
            stats = build_refseq(tmp, self.reg, self.cats, nout, eout,
                                 id_map=id_map, stats_path=tmp / "s.json")
            nodes = {r.split("\t")[0]: r.split("\t")[1]
                     for r in nout.read_text().splitlines()[1:]}
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in eout.read_text().splitlines()[1:]}
            return stats, nodes, edges

    def test_type_split_and_edges(self):
        rs, en, tx, up = (self._id("refseq"), self._id("entrez"),
                          self._id("taxonomy"), self._id("uniprot"))
        lines = [
            # mRNA block: gene, taxon, protein product, uniprot, + property
            f"NM_000014\t{rs}\t2\t{en}",
            f"NM_000014\t{rs}\t9606\t{tx}",
            f"NM_000014\t{rs}\tNP_000005\t{rs}",
            f"NM_000014\t{rs}\tP01023\t{up}",
            f'NM_000014\t{rs}\t{{"type":"mRNA","symbol":"A2M","description":"alpha-2-macroglobulin mRNA"}}\t-1',
            # protein block: gene (-> has_gene_product flip), taxon, + property
            f"NP_000005\t{rs}\t2\t{en}",
            f"NP_000005\t{rs}\t9606\t{tx}",
            f'NP_000005\t{rs}\t{{"type":"protein","symbol":"A2M","description":"alpha-2-macroglobulin"}}\t-1',
            # ncRNA block: gene, taxon, + property
            f"NR_000002\t{rs}\t27209\t{en}",
            f"NR_000002\t{rs}\t10090\t{tx}",
            f'NR_000002\t{rs}\t{{"type":"ncRNA","symbol":"Snord32a"}}\t-1',
        ]
        stats, nodes, edges = self._run(lines)

        # --- typed nodes ---
        self.assertEqual(nodes["refseq:NM_000014"], "biolink:Transcript")
        self.assertEqual(nodes["refseq:NP_000005"], "biolink:Protein")
        self.assertEqual(nodes["refseq:NR_000002"], "biolink:NoncodingRNAProduct")
        self.assertEqual(stats.nodes_written, 3)

        # --- mRNA edges ---
        self.assertIn(("refseq:NM_000014", "biolink:transcribed_from", "NCBIGene:2"), edges)
        self.assertIn(("refseq:NM_000014", "biolink:in_taxon", "NCBITaxon:9606"), edges)
        self.assertIn(("refseq:NM_000014", "biolink:translates_to", "refseq:NP_000005"), edges)
        self.assertIn(("refseq:NM_000014", "biolink:translates_to", "UniProtKB:P01023"), edges)
        # --- protein: Gene has_gene_product Protein (flipped) ---
        self.assertIn(("NCBIGene:2", "biolink:has_gene_product", "refseq:NP_000005"), edges)
        # protein block must NOT re-emit the reverse translates_to (dedup by RNA side)
        self.assertNotIn(("refseq:NP_000005", "biolink:translates_to", "refseq:NM_000014"), edges)
        # --- ncRNA edges ---
        self.assertIn(("refseq:NR_000002", "biolink:transcribed_from", "NCBIGene:27209"), edges)

    def test_gene_canonicalized_via_id_map(self):
        rs, en, tx = self._id("refseq"), self._id("entrez"), self._id("taxonomy")
        lines = [
            f"NM_1\t{rs}\t2\t{en}",
            f"NM_1\t{rs}\t9606\t{tx}",
            f'NM_1\t{rs}\t{{"type":"mRNA","symbol":"A2M"}}\t-1',
        ]
        _, _, edges = self._run(lines, id_map={"NCBIGene:2": "HGNC:7"})
        self.assertIn(("refseq:NM_1", "biolink:transcribed_from", "HGNC:7"), edges)

    def test_untyped_accession_skipped(self):
        rs, tx = self._id("refseq"), self._id("taxonomy")
        lines = [
            f"ZZ_999\t{rs}\t9606\t{tx}",
            f'ZZ_999\t{rs}\t{{"type":"mystery"}}\t-1',
        ]
        stats, nodes, edges = self._run(lines)
        self.assertEqual(stats.nodes_written, 0)
        self.assertEqual(stats.untyped, 1)
        self.assertFalse(edges)


if __name__ == "__main__":
    unittest.main()
