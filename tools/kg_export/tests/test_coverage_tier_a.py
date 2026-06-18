"""Tier-A coverage additions (from the source1 coverage audit): alliance_disease,
clinical_trials, cellxgene_celltype, ctd_gene_interaction, civic, ortholog/paralog.

    python3 -m unittest tools.kg_export.tests.test_coverage_tier_a -v
"""

import gzip
import tempfile
import unittest
from pathlib import Path

from tools.kg_export.categories import CategoryMap
from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.edges import build_edges
from tools.kg_export.predicates import PredicateMap
from tools.kg_export.reified import build_reified_edges

REPO_ROOT = Path(__file__).resolve().parents[3]
CONF = REPO_ROOT / "conf"
CATS = REPO_ROOT / "mappings" / "categories.yaml"
PREDS = REPO_ROOT / "mappings" / "predicates.yaml"


class TierACoverageTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.reg = DatasetRegistry.load(CONF)
        cls.cats = CategoryMap.load(CATS)
        cls.pm = PredicateMap.load(PREDS)

    def _id(self, n):
        return self.reg.by_name(n).numeric_id

    def _write(self, d, name, lines):
        with gzip.open(d / name, "wt") as fh:
            fh.write("".join(l + "\n" for l in lines))

    def _reified(self, name, lines, ds):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, name, lines)
            out = tmp / "r.tsv"
            stats = build_reified_edges(tmp, self.reg, self.cats, self.pm, out, datasets=[ds])
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            return stats, edges

    def test_alliance_disease_cross_species(self):
        """gene (human OR a MOD namespace) -> DOID; PubMed citation not an edge."""
        ad, hg, mg, do, pm = (self._id("alliance_disease"), self._id("hgnc"),
                              self._id("mgi"), self._id("doid"), self._id("pubmed"))
        _, edges = self._reified("alliance_disease_sorted.1.index.gz", [
            f"AGR1\t{ad}\tHGNC:1100\t{hg}", f"AGR1\t{ad}\tDOID:1612\t{do}",
            f"AGR1\t{ad}\t12345\t{pm}", f'AGR1\t{ad}\t{{"x":1}}\t-1',
            f"AGR2\t{ad}\tMGI:95713\t{mg}", f"AGR2\t{ad}\tDOID:162\t{do}",
        ], "alliance_disease")
        self.assertIn(("HGNC:1100", "biolink:gene_associated_with_condition", "DOID:1612"), edges)
        self.assertIn(("MGI:95713", "biolink:gene_associated_with_condition", "DOID:162"), edges)
        self.assertEqual(len(edges), 2)  # pubmed citation not wired

    def test_clinical_trials_drug_to_condition(self):
        ct, cm, mo, ef, pm = (self._id("clinical_trials"), self._id("chembl_molecule"),
                              self._id("mondo"), self._id("efo"), self._id("pubmed"))
        stats, edges = self._reified("clinical_trials_sorted.1.index.gz", [
            f'NCT1\t{ct}\t{{"phase":"PHASE3"}}\t-1', f"NCT1\t{ct}\tCHEMBL193\t{cm}",
            f"NCT1\t{ct}\tMONDO:0015898\t{mo}", f"NCT1\t{ct}\tEFO:0000311\t{ef}",
            f"NCT1\t{ct}\t12345\t{pm}",
        ], "clinical_trials")
        self.assertEqual(stats.edges_written, 2)  # mondo + efo, NOT pubmed
        self.assertEqual(edges, {
            ("CHEMBL.COMPOUND:CHEMBL193", "biolink:in_clinical_trials_for", "MONDO:0015898"),
            ("CHEMBL.COMPOUND:CHEMBL193", "biolink:in_clinical_trials_for", "EFO:0000311"),
        })

    def test_cellxgene_celltype_to_tissue(self):
        cc, cl, ub, mo = (self._id("cellxgene_celltype"), self._id("cl"),
                          self._id("uberon"), self._id("mondo"))
        stats, edges = self._reified("cellxgene_celltype_sorted.1.index.gz", [
            f"CL_0000235\t{cc}\tCL:0000235\t{cl}", f"CL_0000235\t{cc}\tUBERON:0002048\t{ub}",
            f"CL_0000235\t{cc}\tMONDO:0005148\t{mo}", f'CL_0000235\t{cc}\t{{"x":1}}\t-1',
            f"UNKNOWN\t{cc}\tMONDO:0005148\t{mo}",  # no cl self-link -> nothing
        ], "cellxgene_celltype")
        self.assertEqual(stats.edges_written, 1)  # cl->uberon only; disease/self/UNKNOWN dropped
        self.assertIn(("CL:0000235", "biolink:located_in", "UBERON:0002048"), edges)

    def test_ctd_gene_interaction(self):
        gi, ctd, en, tx, pm = (self._id("ctd_gene_interaction"), self._id("ctd"),
                               self._id("entrez"), self._id("taxonomy"), self._id("pubmed"))
        stats, edges = self._reified("ctd_gene_interaction_sorted.1.index.gz", [
            f"C1_10257\t{gi}\tC1\t{ctd}", f"C1_10257\t{gi}\t10257\t{en}",
            f"C1_10257\t{gi}\t9606\t{tx}", f"C1_10257\t{gi}\t999\t{pm}",
            f'C1_10257\t{gi}\t{{"x":1}}\t-1',
        ], "ctd_gene_interaction")
        self.assertEqual(stats.edges_written, 1)  # chemical->gene; taxon+pubmed ignored
        self.assertIn(("MESH:C1", "biolink:affects", "NCBIGene:10257"), edges)

    def test_civic_evidence_variant_to_disease_and_drug(self):
        ce, cv, mo, cm = (self._id("civic_evidence"), self._id("civic_variant"),
                          self._id("mondo"), self._id("chembl_molecule"))
        stats, edges = self._reified("civic_evidence_sorted.1.index.gz", [
            f"EV1\t{ce}\t64\t{cv}",  # the variant (object line) -> becomes subject
            f"EV1\t{ce}\tMONDO:0005402\t{mo}", f"EV1\t{ce}\tCHEMBL1236682\t{cm}",
            f'EV1\t{ce}\t{{"x":1}}\t-1',
        ], "civic_evidence")
        self.assertEqual(stats.edges_written, 2)  # variant->disease + variant->drug
        self.assertIn(("civic.vid:64", "biolink:associated_with", "MONDO:0005402"), edges)
        self.assertIn(("civic.vid:64", "biolink:associated_with", "CHEMBL.COMPOUND:CHEMBL1236682"), edges)

    def test_compara_homology_direct(self):
        en, orth, para = self._id("ensembl"), self._id("ortholog"), self._id("paralog")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "ensembl_sorted.1.index.gz", [
                f"ENSG1\t{en}\tENSMUSG1\t{orth}", f"ENSG1\t{en}\tENSG2\t{para}",
                f"ENSG1\t{orth}\tENSG1\t{en}", f"ENSG1\t{para}\tENSG1\t{en}",  # self-ref skips
            ])
            out = tmp / "e.tsv"
            stats = build_edges(tmp, self.reg, self.cats, self.pm, out)
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            self.assertIn(("ENSEMBL:ENSG1", "biolink:orthologous_to", "ENSEMBL:ENSMUSG1"), edges)
            self.assertIn(("ENSEMBL:ENSG1", "biolink:paralogous_to", "ENSEMBL:ENSG2"), edges)
            self.assertEqual(stats.skipped, 2)  # the two self-ref back-links

    def test_mesh_is_not_a_node(self):
        """MeSH is multi-type (chemicals+diseases+...) with no usable type field on
        91% of records -> deliberately NOT a node; reachable only as an xref endpoint."""
        self.assertFalse(self.cats.is_node_dataset("mesh"))
        self.assertIsNone(self.cats.category_for("mesh"))


if __name__ == "__main__":
    unittest.main()
