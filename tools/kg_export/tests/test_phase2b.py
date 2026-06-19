"""Phase 2b tests: reified edge building (symmetric + bipartite).

    python3 -m unittest tools.kg_export.tests.test_phase2b -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.edges import load_id_map
from tools.kg_export.predicates import PredicateMap
from tools.kg_export.reified import build_reified_edges

REPO_ROOT = Path(__file__).resolve().parents[3]
CONF_DIR = REPO_ROOT / "conf"
CATEGORIES_YAML = REPO_ROOT / "mappings" / "categories.yaml"
PREDICATES_YAML = REPO_ROOT / "mappings" / "predicates.yaml"


class ReifiedRuleTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.pm = PredicateMap.load(PREDICATES_YAML)
        cls.reg = DatasetRegistry.load(CONF_DIR)
        cls.cats = CategoryMap.load(CATEGORIES_YAML)

    def test_rules_load(self):
        r = self.pm.reified_rule("intact")
        self.assertEqual(r.kind, "pairwise")
        self.assertEqual((r.partner, r.subject_field, r.object_field),
                         ("uniprot", "protein_a", "protein_b"))
        s = self.pm.reified_rule("diamond_similarity")
        self.assertEqual(s.kind, "star")
        b = self.pm.reified_rule("chembl_activity")
        self.assertEqual(b.kind, "bipartite")
        self.assertEqual((b.subject, b.object), ("chembl_molecule", "uniprot"))

    def test_reified_partners_are_real_node_datasets(self):
        from tools.kg_export.reified import _RUNTIME_PREFIXES
        for ds in self.pm.reified_datasets():
            r = self.pm.reified_rule(ds)
            self.assertIn(ds, self.reg, f"{ds}: reified dataset not in conf")
            roles = ([r.partner] if r.kind in ("pairwise", "star")
                     else [r.subject, r.object])
            for role in roles:
                self.assertIn(role, self.reg, f"{ds}: role {role} not in conf")
                # categories.yaml node OR a runtime-typed dataset (refseq/go)
                self.assertTrue(
                    self.cats.is_node_dataset(role) or role in _RUNTIME_PREFIXES,
                    f"{ds}: role {role} is not a typed node",
                )


class BuildReifiedTests(unittest.TestCase):
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

    def test_pairwise_ppi_no_clique_fabrication(self):
        """A hub interaction (1 bait, 3 preys) must emit the 3 real pairs, not 6."""
        ia, up = self._id("intact"), self._id("uniprot")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            # 4 partners (NDC80 + 3 preys), real pairs are all NDC80-centered
            props = [
                '{"protein_a":"O14777","protein_b":"Q8NBT2"}',
                '{"protein_a":"O14777","protein_b":"Q9BZD4"}',
                '{"protein_a":"O14777","protein_b":"Q9HBM1"}',
            ]
            lines = [
                f"EBI1\t{ia}\tO14777\t{up}", f"EBI1\t{ia}\tQ8NBT2\t{up}",
                f"EBI1\t{ia}\tQ9BZD4\t{up}", f"EBI1\t{ia}\tQ9HBM1\t{up}",
            ] + [f"EBI1\t{ia}\t{p}\t-1" for p in props]
            self._write(tmp, "intact_sorted.1.index.gz", lines)
            out = tmp / "r.tsv"
            stats = build_reified_edges(
                tmp, self.reg, self.cats, self.pm, out, datasets=["intact"],
            )
            edges = {(r.split("\t")[1], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            self.assertEqual(stats.edges_written, 3)  # NOT 6 (no clique)
            self.assertEqual(edges, {
                ("UniProtKB:O14777", "UniProtKB:Q8NBT2"),
                ("UniProtKB:O14777", "UniProtKB:Q9BZD4"),
                ("UniProtKB:O14777", "UniProtKB:Q9HBM1"),
            })
            # the fabricated prey-prey pair must be absent
            self.assertNotIn(("UniProtKB:Q8NBT2", "UniProtKB:Q9BZD4"), edges)

    def test_pairwise_symbol_resolution_directed(self):
        """collectri: tf_gene/target_gene symbols resolved to HGNC, directed."""
        co, hg = self._id("collectri"), self._id("hgnc")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            # hgnc property lines provide symbol -> HGNC id
            self._write(tmp, "hgnc_sorted.1.index.gz", [
                f'HGNC:11998\t{hg}\t{{"symbols":["TP53"]}}\t-1',
                f'HGNC:1784\t{hg}\t{{"symbols":["CDKN1A"]}}\t-1',
            ])
            self._write(tmp, "collectri_sorted.1.index.gz", [
                f'TP53:CDKN1A\t{co}\t{{"tf_gene":"TP53","target_gene":"CDKN1A","regulation":"Activation"}}\t-1',
                f"TP53:CDKN1A\t{co}\tHGNC:11998\t{hg}", f"TP53:CDKN1A\t{co}\tHGNC:1784\t{hg}",
            ])
            out = tmp / "r.tsv"
            build_reified_edges(tmp, self.reg, self.cats, self.pm, out, datasets=["collectri"])
            rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
            self.assertEqual(len(rows), 1)
            _id, s, p, o = rows[0][:4]
            self.assertEqual((s, p, o), ("HGNC:11998", "biolink:regulates", "HGNC:1784"))  # TF->target

    def test_pairwise_cross_complex_members(self):
        """cellphonedb: genes_a x genes_b all-pairs, symbol-resolved to HGNC.

        partner_a={ALDH1A2}, partner_b={RARG,RXRG} -> 2 member-gene edges; the
        union edge-lines (which mix both sides) must NOT be cliqued.
        """
        cp, hg = self._id("cellphonedb"), self._id("hgnc")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "hgnc_sorted.1.index.gz", [
                f'HGNC:15472\t{hg}\t{{"symbols":["ALDH1A2"]}}\t-1',
                f'HGNC:9866\t{hg}\t{{"symbols":["RARG"]}}\t-1',
                f'HGNC:10479\t{hg}\t{{"symbols":["RXRG"]}}\t-1',
            ])
            prop = ('{"partner_a":"lig","partner_b":"rec",'
                    '"directionality":"Ligand-Receptor",'
                    '"genes_a":["ALDH1A2"],"genes_b":["RARG","RXRG"]}')
            # the union edge-lines carry BOTH sides' members (no per-side info)
            self._write(tmp, "cellphonedb_sorted.1.index.gz", [
                f"CPI-1\t{cp}\tHGNC:15472\t{hg}",
                f"CPI-1\t{cp}\tHGNC:9866\t{hg}",
                f"CPI-1\t{cp}\tHGNC:10479\t{hg}",
                f"CPI-1\t{cp}\t{prop}\t-1",
            ])
            out = tmp / "r.tsv"
            stats = build_reified_edges(
                tmp, self.reg, self.cats, self.pm, out, datasets=["cellphonedb"],
            )
            edges = {(r.split("\t")[1], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            self.assertEqual(stats.edges_written, 2)  # 1 x 2, NOT a 3-node clique
            self.assertEqual(edges, {
                ("HGNC:15472", "HGNC:9866"),
                ("HGNC:15472", "HGNC:10479"),
            })
            # the two receptor members must not be wired to each other
            self.assertNotIn(("HGNC:9866", "HGNC:10479"), edges)
            for _id, s, p, o, *_ in (
                l.split("\t") for l in out.read_text().splitlines()[1:]
            ):
                self.assertEqual(p, "biolink:interacts_with")

    def test_star_similarity_no_clique(self):
        """diamond: query -> each hit; never hit<->hit, never self."""
        dia, up = self._id("diamond_similarity"), self._id("uniprot")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "diamond_similarity_sorted.1.index.gz", [
                f"QUERY1\t{dia}\tQUERY1\t{up}",  # self -> excluded
                f"QUERY1\t{dia}\tHIT1\t{up}",
                f"QUERY1\t{dia}\tHIT2\t{up}",
            ])
            out = tmp / "r.tsv"
            stats = build_reified_edges(
                tmp, self.reg, self.cats, self.pm, out, datasets=["diamond_similarity"],
            )
            edges = {(r.split("\t")[1], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            self.assertEqual(edges, {
                ("UniProtKB:QUERY1", "UniProtKB:HIT1"),
                ("UniProtKB:QUERY1", "UniProtKB:HIT2"),
            })
            self.assertNotIn(("UniProtKB:HIT1", "UniProtKB:HIT2"), edges)  # no clique
            self.assertEqual(stats.edges_written, 2)

    def test_bipartite_pharmgkb_variant_to_gene(self):
        """pharmgkb_variant: rsID variant -> gene (hgnc); ignores ensembl dup,
        the symbol field, and entries with no rsID."""
        pv, db, hg, en = (self._id("pharmgkb_variant"), self._id("dbsnp"),
                          self._id("hgnc"), self._id("ensembl"))
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "pharmgkb_variant_sorted.1.index.gz", [
                f"PA166153539\t{pv}\tRS699\t{db}",
                f"PA166153539\t{pv}\tHGNC:333\t{hg}",
                f"PA166153539\t{pv}\tENSG00000135744\t{en}",
                f'PA166153539\t{pv}\t{{"variant_id":"PA166153539"}}\t-1',
                f"PA999\t{pv}\tHGNC:1\t{hg}",  # no rsID -> emit nothing
            ])
            out = tmp / "r.tsv"
            stats = build_reified_edges(tmp, self.reg, self.cats, self.pm, out,
                                        datasets=["pharmgkb_variant"])
            rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
            self.assertEqual(stats.edges_written, 1)  # ensembl dup ignored
            self.assertEqual(tuple(rows[0][1:4]),
                             ("DBSNP:RS699", "biolink:is_sequence_variant_of", "HGNC:333"))

    def test_pairwise_biogrid_require_physical(self):
        """biogrid_interaction: physical pairs only; genetic dropped."""
        bg, up = self._id("biogrid_interaction"), self._id("uniprot")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            props = [
                '{"interactor_a_id":"P39730","interactor_b_id":"P02309","experimental_system_type":"physical"}',
                '{"interactor_a_id":"P39730","interactor_b_id":"P09440","experimental_system_type":"genetic"}',
            ]
            lines = [f"E1\t{bg}\tP39730\t{up}", f"E1\t{bg}\tP02309\t{up}",
                     f"E1\t{bg}\tP09440\t{up}"] + [f"E1\t{bg}\t{p}\t-1" for p in props]
            self._write(tmp, "biogrid_interaction_sorted.1.index.gz", lines)
            out = tmp / "r.tsv"
            stats = build_reified_edges(tmp, self.reg, self.cats, self.pm, out,
                                        datasets=["biogrid_interaction"])
            edges = {(r.split("\t")[1], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            self.assertEqual(stats.edges_written, 1)
            self.assertEqual(edges, {("UniProtKB:P39730", "UniProtKB:P02309")})

    def test_bipartite_bioactivity(self):
        ca, cm, up = self._id("chembl_activity"), self._id("chembl_molecule"), self._id("uniprot")
        bao = self._id("bao")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "chembl_activity_sorted.1.index.gz", [
                f"ACT1\t{ca}\tCHEMBL5\t{cm}",
                f"ACT1\t{ca}\tP9\t{up}",
                f"ACT1\t{ca}\tBAO_1\t{bao}",  # bao role ignored
            ])
            out = tmp / "r.tsv"
            stats = build_reified_edges(
                tmp, self.reg, self.cats, self.pm, out, datasets=["chembl_activity"],
            )
            rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
            self.assertEqual(len(rows), 1)
            _id, s, p, o = rows[0][:4]
            self.assertEqual(s, "CHEMBL.COMPOUND:CHEMBL5")
            self.assertEqual(o, "UniProtKB:P9")
            self.assertEqual(p, "biolink:interacts_with")

    def test_bipartite_via_resolution(self):
        """gtopdb_interaction: ligand + gtopdb-target id; via gtopdb -> uniprot."""
        gi, gl, gt, up = (self._id("gtopdb_interaction"), self._id("gtopdb_ligand"),
                          self._id("gtopdb"), self._id("uniprot"))
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            # gtopdb forward: target T1 -> uniprot P1 (the resolution map)
            self._write(tmp, "gtopdb_sorted.1.index.gz", [f"T1\t{gt}\tP1\t{up}"])
            # interaction entry: ligand L1 + target T1
            self._write(tmp, "gtopdb_interaction_sorted.1.index.gz", [
                f"I1\t{gi}\tL1\t{gl}", f"I1\t{gi}\tT1\t{gt}",
            ])
            out = tmp / "r.tsv"
            build_reified_edges(tmp, self.reg, self.cats, self.pm, out,
                                datasets=["gtopdb_interaction"])
            rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
            self.assertEqual(len(rows), 1)
            _id, s, p, o = rows[0][:4]
            self.assertEqual(s, "GTOPDB:L1")           # ligand
            self.assertEqual(o, "UniProtKB:P1")        # resolved target
            self.assertEqual(p, "biolink:interacts_with")

    def test_bipartite_expression_with_canonicalization(self):
        bg, en, ub = self._id("bgee"), self._id("ensembl"), self._id("uberon")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            (tmp / "id_map.tsv").write_text("member\tcanonical\nENSEMBL:ENSG1\tHGNC:1\n")
            id_map = load_id_map(tmp / "id_map.tsv")
            self._write(tmp, "bgee_sorted.1.index.gz", [
                f"BG1\t{bg}\tENSG1\t{en}",
                f"BG1\t{bg}\tUBERON:0000955\t{ub}",
            ])
            out = tmp / "r.tsv"
            build_reified_edges(
                tmp, self.reg, self.cats, self.pm, out,
                datasets=["bgee"], id_map=id_map,
            )
            rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
            self.assertEqual(len(rows), 1)
            _id, s, p, o = rows[0][:4]
            self.assertEqual(s, "HGNC:1")  # ensembl gene canonicalized
            self.assertEqual(o, "UBERON:0000955")
            self.assertEqual(p, "biolink:expressed_in")

    def test_bipartite_fantom5_promoter_gene_to_tissue(self):
        """FANTOM5 promoter: gene(hgnc)->tissue(uberon) expression.

        The promoter entry also links a cell type (cl) and the gene's other
        namespaces (ensembl/entrez/taxonomy); only the hgnc->uberon edge is
        emitted. The CAGE-peak region id itself is never a node endpoint.
        """
        fp = self._id("fantom5_promoter")
        hg, en, ub, cl, tx = (self._id("hgnc"), self._id("ensembl"),
                              self._id("uberon"), self._id("cl"), self._id("taxonomy"))
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            # one promoter entry (id "1") annotated to gene HGNC:42092, expressed
            # in two tissues + one cell type; the property line is the CAGE peak.
            self._write(tmp, "fantom5_promoter_sorted.1.index.gz", [
                f"1\t{fp}\tHGNC:42092\t{hg}",
                f"1\t{fp}\tENSG00000225972\t{en}",
                f"1\t{fp}\t9606\t{tx}",
                f"1\t{fp}\tUBERON:0002048\t{ub}",   # lung
                f"1\t{fp}\tUBERON:0000178\t{ub}",   # blood
                f"1\t{fp}\tCL:0000235\t{cl}",       # macrophage (ignored: object is uberon)
                f'1\t{fp}\t{{"fantom5_peak_id":"hg19::chr1:564571..564600,+;hg_1.1","hgnc_id":"HGNC:42092"}}\t-1',
            ])
            out = tmp / "r.tsv"
            stats = build_reified_edges(
                tmp, self.reg, self.cats, self.pm, out,
                datasets=["fantom5_promoter"],
            )
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            self.assertEqual(stats.edges_written, 2)  # 1 gene x 2 tissues
            self.assertEqual(edges, {
                ("HGNC:42092", "biolink:expressed_in", "UBERON:0002048"),
                ("HGNC:42092", "biolink:expressed_in", "UBERON:0000178"),
            })
            # cell type is not the object dataset; no cl edge
            self.assertNotIn(
                ("HGNC:42092", "biolink:expressed_in", "CL:0000235"), edges)
            # the CAGE-peak region id is never an endpoint
            endpoints = {e[0] for e in edges} | {e[2] for e in edges}
            self.assertNotIn("hg19::chr1:564571..564600,+;hg_1.1", endpoints)

    def test_fantom5_enhancer_authored_as_regulatory_region(self):
        """fantom5_enhancer is now a RegulatoryRegion -> gene (proximity) rule."""
        r = self.pm.reified_rule("fantom5_enhancer")
        self.assertIsNotNone(r)
        self.assertEqual(r.subject_field, "fantom5_enhancer_id")  # coordinate id from JSON
        self.assertEqual(r.predicate, "biolink:associated_with")
        self.assertEqual(self.cats.category_for("fantom5_enhancer"), "biolink:RegulatoryRegion")

    def test_bioactivity_assay_type_qualifier(self):
        """chembl_activity attaches the in-group BAO assay type as a qualifier."""
        ca, cm, up, ba = (self._id("chembl_activity"), self._id("chembl_molecule"),
                          self._id("uniprot"), self._id("bao"))
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "chembl_activity_sorted.1.index.gz", [
                f"CHEMBL_ACT_1\t{ca}\tCHEMBL25\t{cm}",
                f"CHEMBL_ACT_1\t{ca}\tP35354\t{up}",
                f"CHEMBL_ACT_1\t{ca}\tBAO:0000034\t{ba}",
                f'CHEMBL_ACT_1\t{ca}\t{{"x":1}}\t-1',
            ])
            out = tmp / "r.tsv"
            build_reified_edges(tmp, self.reg, self.cats, self.pm, out, datasets=["chembl_activity"])
            rows = [l.split("\t") for l in out.read_text().splitlines()[1:]]
            self.assertEqual(len(rows), 1)
            # cols: id, s, p, o, primary, agg, kl, at, has_evidence, qualifiers
            s, p, o, quals = rows[0][1], rows[0][2], rows[0][3], rows[0][9]
            self.assertEqual((s, p, o), ("CHEMBL.COMPOUND:CHEMBL25", "biolink:interacts_with", "UniProtKB:P35354"))
            self.assertEqual(quals, "assay_type=BAO:0000034")

    def test_gwas_extra_objects_disease_and_attribute(self):
        """gwas gene-trait emits to BOTH mondo (disease) and oba (attribute)."""
        gw, hg, mo, ob = (self._id("gwas"), self._id("hgnc"),
                          self._id("mondo"), self._id("oba"))
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            # one association group: gene + a disease trait + an attribute trait
            self._write(tmp, "gwas_sorted.1.index.gz", [
                f"GCST1_1\t{gw}\tHGNC:4883\t{hg}",
                f"GCST1_1\t{gw}\tMONDO:0005150\t{mo}",
                f"GCST1_1\t{gw}\tOBA:0000061\t{ob}",
                f'GCST1_1\t{gw}\t{{"snp_id":"rs1"}}\t-1',
            ])
            out = tmp / "r.tsv"
            build_reified_edges(tmp, self.reg, self.cats, self.pm, out, datasets=["gwas"])
            edges = {(r.split("\t")[1], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            self.assertIn(("HGNC:4883", "MONDO:0005150"), edges)
            self.assertIn(("HGNC:4883", "OBA:0000061"), edges)  # the extra object


if __name__ == "__main__":
    unittest.main()
