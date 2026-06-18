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

            # cols: id, subject, predicate, object, primary, agg, kl, at
            edges = {(r[1], r[2], r[3]) for r in rows}
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

    def test_enzyme_and_reaction_direct_edges(self):
        """uniprot>ec (enables) + rhea>chebi/uniprot (participant/catalysis flip)."""
        up, ec, rh, ch = (self._id("uniprot"), self._id("ec"),
                          self._id("rhea"), self._id("chebi"))
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "uniprot_sorted.1.index.gz", [
                f"P1\t{up}\t3.2.2.6\t{ec}",
                f"P1\t{up}\t1.1.1.-\t{ec}",
            ])
            self._write(tmp, "rhea_sorted.1.index.gz", [
                f'RHEA:10000\t{rh}\t{{"equation":"x"}}\t-1',
                f"RHEA:10000\t{rh}\tCHEBI:15377\t{ch}",
                f"RHEA:10000\t{rh}\tP1\t{up}",
            ])
            out = tmp / "edges.tsv"
            stats = build_edges(tmp, self.reg, self.cats, self.pm, out)
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            self.assertIn(("UniProtKB:P1", "biolink:enables", "EC:3.2.2.6"), edges)
            self.assertIn(("RHEA:10000", "biolink:has_participant", "CHEBI:15377"), edges)
            self.assertIn(("UniProtKB:P1", "biolink:enables", "RHEA:10000"), edges)  # flip
            self.assertEqual(stats.edges_written, 4)

    def test_brenda_enzyme_cluster_edges(self):
        """BRENDA enzyme (EC) cluster:
          * hmdb>brenda -> metabolite participates_in EC MolecularActivity
          * uniprot>brenda -> SKIP (EC dup of uniprot>ec; brenda id == ec id space)
          * brenda EC entries are NODES typed biolink:MolecularActivity (prefix EC)
          * brenda's child links (brenda_kinetics/inhibitor) + pubmed refs are not
            node datasets -> dropped_not_node (no spurious edges)
        Substrate/inhibitor chemicals are free text (no CURIE) -> kinetics/inhibitor
        reified edges are intentionally NOT authored (deferred).
        """
        hm, br, up, ec = (self._id("hmdb"), self._id("brenda"),
                          self._id("uniprot"), self._id("ec"))
        bk, bi, pm_ = (self._id("brenda_kinetics"), self._id("brenda_inhibitor"),
                       self._id("pubmed"))
        # brenda must be a typed node (MolecularActivity) so EC endpoints resolve
        self.assertTrue(self.cats.is_node_dataset("brenda"))
        self.assertEqual(self.cats.category_for("brenda"), "biolink:MolecularActivity")
        self.assertEqual(self.cats.prefix_for("brenda"), "EC")
        self.assertTrue(self.pm.rule_for("uniprot", "brenda").is_skip)

        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "hmdb_sorted.1.index.gz", [
                f"HMDB0004085\t{hm}\t1.1.1.100\t{br}",   # metabolite -> EC activity
                f'HMDB0004085\t{hm}\t{{"name":"x"}}\t-1',  # property -> skipped
            ])
            self._write(tmp, "uniprot_sorted.1.index.gz", [
                f"P1\t{up}\t3.2.1.39\t{br}",             # uniprot>brenda -> skip (EC dup)
                f"P1\t{up}\t3.2.1.39\t{ec}",             # the canonical uniprot>ec edge
            ])
            self._write(tmp, "brenda_sorted.1.index.gz", [
                f'1.1.1.1\t{br}\t{{"recommended_name":"alcohol dehydrogenase"}}\t-1',
                f"1.1.1.1\t{br}\t1.1.1.1|ETHANOL\t{bk}",   # -> kinetics key (not a node)
                f"1.1.1.1\t{br}\t1.1.1.1|NAD+\t{bi}",      # -> inhibitor key (not a node)
                f"1.1.1.1\t{br}\t12345\t{pm_}",            # -> pubmed (not a node)
            ])
            out = tmp / "edges.tsv"
            stats = build_edges(tmp, self.reg, self.cats, self.pm, out)
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            # only real edges: hmdb metabolite -> EC activity, and uniprot>ec
            self.assertIn(
                ("HMDB:HMDB0004085", "biolink:participates_in", "EC:1.1.1.100"), edges)
            self.assertIn(("UniProtKB:P1", "biolink:enables", "EC:3.2.1.39"), edges)
            self.assertEqual(stats.edges_written, 2)
            # skip rules: uniprot>brenda (EC dup) + brenda>pubmed (pubmed IS a
            # Publication node now, so it reaches the skip rule, not dropped)
            self.assertEqual(stats.skipped, 2)
            # the 2 brenda kinetics/inhibitor reification keys aren't nodes -> dropped
            self.assertEqual(stats.dropped_not_node, 2)
            self.assertEqual(stats.unmapped, 0)
            # no edge points at a kinetics/inhibitor reification key or a pubmed id
            endpoints = {e[0] for e in edges} | {e[2] for e in edges}
            self.assertNotIn("1.1.1.1|ETHANOL", endpoints)
            self.assertNotIn("PMID:12345", endpoints)

    def test_brenda_kinetics_inhibitor_not_reified(self):
        """kinetics/inhibitor are NOT reified rules: their only structured object
        is the free-text substrate/inhibitor (no CURIE) + numeric Km/Ki that
        can't be qualifiers yet -> deferred, no edges fabricated."""
        self.assertIsNone(self.pm.reified_rule("brenda_kinetics"))
        self.assertIsNone(self.pm.reified_rule("brenda_inhibitor"))
        self.assertIsNone(self.pm.reified_rule("brenda"))

    def test_primary_knowledge_source(self):
        cl, hg = self._id("clinvar"), self._id("hgnc")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "clinvar_sorted.1.index.gz", [f"VAR1\t{cl}\tHGNC:1\t{hg}"])
            out = tmp / "edges.tsv"
            build_edges(tmp, self.reg, self.cats, self.pm, out)
            row = out.read_text().splitlines()[1].split("\t")
            # cols: id, subject, predicate, object, primary, agg, kl, at
            self.assertEqual(row[4], "infores:clinvar")
            self.assertEqual(row[5], "infores:biobtree")
            self.assertEqual(row[6], "knowledge_assertion")


if __name__ == "__main__":
    unittest.main()
