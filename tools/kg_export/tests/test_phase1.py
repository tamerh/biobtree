"""Phase 1 tests: CURIE rendering, name extraction, node normalization.

    python3 -m unittest tools.kg_export.tests.test_phase1 -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.curie import to_curie
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.nodes import build_nodes, extract_name

REPO_ROOT = Path(__file__).resolve().parents[3]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = REPO_ROOT / "mappings" / "categories.yaml"

# numeric ids (from conf): hgnc=10, ensembl=2, entrez=4, uniprot=1
HGNC, ENSEMBL, ENTREZ, UNIPROT = "10", "2", "4", "1"


class CurieTests(unittest.TestCase):
    def test_bare_id_gets_prefix(self):
        self.assertEqual(to_curie("NCBIGene", "41"), "NCBIGene:41")
        self.assertEqual(to_curie("CHEMBL.COMPOUND", "CHEMBL1"), "CHEMBL.COMPOUND:CHEMBL1")
        self.assertEqual(to_curie("ENSEMBL", "ENSG001"), "ENSEMBL:ENSG001")

    def test_already_prefixed_kept(self):
        self.assertEqual(to_curie("HGNC", "HGNC:100"), "HGNC:100")
        self.assertEqual(to_curie("MONDO", "MONDO:0001"), "MONDO:0001")
        # prefix casing normalized to the authored prefix
        self.assertEqual(to_curie("HGNC", "hgnc:100"), "HGNC:100")


class ExtractNameTests(unittest.TestCase):
    def test_gene_prefers_symbol(self):
        j = '{"names":["acid sensing ion channel"],"symbols":["ASIC1"]}'
        self.assertEqual(extract_name(j), "ASIC1")

    def test_disease_scalar_name(self):
        self.assertEqual(extract_name('{"name":"adrenal insufficiency"}'), "adrenal insufficiency")

    def test_protein_names_list(self):
        self.assertEqual(extract_name('{"names":["L-2-hydroxyglutarate dehydrogenase"]}'), "L-2-hydroxyglutarate dehydrogenase")

    def test_bad_json(self):
        self.assertIsNone(extract_name("not json"))
        self.assertIsNone(extract_name('["a","b"]'))

    def test_pubchem_title(self):
        self.assertEqual(extract_name('{"cid":"1","title":"2-amino-1-phenylethanol"}'),
                         "2-amino-1-phenylethanol")

    def test_chembl_nested_wrapper(self):
        # chembl_molecule nests the name under a single {"molecule": {...}} key
        self.assertEqual(extract_name('{"molecule":{"type":"Small molecule","name":"OMEPRAZOLE"}}'),
                         "OMEPRAZOLE")

    def test_single_wrapper_only_when_no_toplevel_name(self):
        # a top-level name wins; don't descend a non-dict single value
        self.assertEqual(extract_name('{"name":"flat"}'), "flat")
        self.assertIsNone(extract_name('{"molecule":{"type":"x"}}'))


class BuildNodesTests(unittest.TestCase):
    def _write(self, d: Path, name: str, lines: list[str]) -> None:
        with gzip.open(d / name, "wt") as fh:
            fh.write("".join(l + "\n" for l in lines))

    def _build(self, tmp: Path):
        reg = DatasetRegistry.load(CONF_DIR)
        cats = CategoryMap.load(CATEGORIES_YAML)
        out = tmp / "nodes.tsv"
        stats = build_nodes(tmp, reg, cats, out, stats_path=tmp / "stats.json")
        rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
        by_id = {r[0]: r for r in rows}
        return stats, by_id

    def test_gene_cluster_and_no_cross_category_merge(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "hgnc_sorted.1.index.gz", [
                'HGNC:1\t10\t{"symbols":["AAA"],"names":["alpha"]}\t-1',  # property
                f"HGNC:1\t{HGNC}\tENSG1\t{ENSEMBL}",   # gene->gene identity
                f"HGNC:1\t{HGNC}\t11\t{ENTREZ}",       # gene->gene identity
                f"HGNC:1\t{HGNC}\tP1\t{UNIPROT}",      # gene->protein (NOT identity)
            ])
            self._write(tmp, "ensembl_sorted.1.index.gz", [
                'ENSG1\t2\t{"name":"ensembl gene 1"}\t-1',
            ])
            self._write(tmp, "uniprot_sorted.1.index.gz", [
                'P1\t1\t{"names":["prot1"]}\t-1',
            ])
            stats, by_id = self._build(tmp)

            # Gene cluster: hgnc + ensembl + entrez merged into ONE node.
            self.assertIn("HGNC:1", by_id)
            gene = by_id["HGNC:1"]
            self.assertEqual(gene[1], "biolink:Gene")
            self.assertEqual(gene[2], "AAA")  # canonical hgnc symbol
            eqs = set(gene[3].split("|"))
            self.assertEqual(eqs, {"HGNC:1", "ENSEMBL:ENSG1", "NCBIGene:11"})

            # Protein is a SEPARATE node — gene->protein must not have merged.
            self.assertIn("UniProtKB:P1", by_id)
            self.assertEqual(by_id["UniProtKB:P1"][1], "biolink:Protein")
            self.assertNotIn("UniProtKB:P1", eqs)

            self.assertEqual(stats.nodes_written, 2)
            self.assertEqual(stats.merges, 2)
            self.assertEqual(stats.multi_member_clusters, 1)
            self.assertEqual(stats.mixed_category_clusters, 0)

    def test_singleton_node_when_no_identity_edge(self):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "chebi_sorted.1.index.gz", [
                'CHEBI:100\t9\t{"name":"water"}\t-1',  # chebi id is 9? resolved by reg
            ])
            # Use a real chebi edge-free property; chebi numeric id resolved from conf.
            reg = DatasetRegistry.load(CONF_DIR)
            chebi_id = reg.by_name("chebi").numeric_id
            # rewrite with correct id
            self._write(tmp, "chebi_sorted.1.index.gz", [
                f'CHEBI:100\t{chebi_id}\t{{"name":"water"}}\t-1',
            ])
            stats, by_id = self._build(tmp)
            self.assertIn("CHEBI:100", by_id)
            self.assertEqual(by_id["CHEBI:100"][1], "biolink:SmallMolecule")
            self.assertEqual(by_id["CHEBI:100"][3], "CHEBI:100")  # equiv = self only


if __name__ == "__main__":
    unittest.main()
