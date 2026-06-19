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
from tools.kg_export.mesh import build_mesh
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

    # --- New BioBTree datasets (round 3) --------------------------------------

    def test_alliance_phenotype_cross_species(self):
        """Alliance gene→phenotype: species-paired gene namespace -> phenotype
        ontology (mgi/rgd->mp, wormbase->wbphenotype, xenbase->xpo); pubmed not wired."""
        ap, mg, wb, xb, mp, wbp, xpo, pm = (
            self._id("alliance_phenotype"), self._id("mgi"), self._id("wormbase"),
            self._id("xenbase"), self._id("mp"), self._id("wbphenotype"),
            self._id("xpo"), self._id("pubmed"))
        _, edges = self._reified("alliance_phenotype_sorted.1.index.gz", [
            f"AGRP1\t{ap}\tMGI:6478931\t{mg}", f"AGRP1\t{ap}\tMP:0009269\t{mp}",
            f"AGRP1\t{ap}\t31738183\t{pm}", f'AGRP1\t{ap}\t{{"x":1}}\t-1',
            f"AGRP2\t{ap}\tWBGENE00004857\t{wb}", f"AGRP2\t{ap}\tWBPhenotype:0002295\t{wbp}",
            f"AGRP3\t{ap}\tXB-GENE-6255375\t{xb}", f"AGRP3\t{ap}\tXPO:0103424\t{xpo}",
        ], "alliance_phenotype")
        self.assertIn(("MGI:6478931", "biolink:has_phenotype", "MP:0009269"), edges)
        self.assertIn(("WB:WBGENE00004857", "biolink:has_phenotype", "WBPhenotype:0002295"), edges)
        self.assertIn(("Xenbase:XB-GENE-6255375", "biolink:has_phenotype", "XPO:0103424"), edges)
        self.assertEqual(len(edges), 3)  # pubmed citation not wired

    def test_faers_drug_to_adverse_reaction(self):
        """FAERS hub -> drug(chembl/pubchem) x reaction; free-text-only drug emits nothing."""
        fa, cm, pc, fr = (self._id("faers"), self._id("chembl_molecule"),
                          self._id("pubchem"), self._id("faers_reaction"))
        P = "biolink:has_adverse_event"
        stats, edges = self._reified("faers_sorted.1.index.gz", [
            f'FAERS_D1\t{fa}\t{{"drug_name":"aspirin"}}\t-1',
            f"FAERS_D1\t{fa}\tCHEMBL25\t{cm}", f"FAERS_D1\t{fa}\t2244\t{pc}",
            f"FAERS_D1\t{fa}\tFAERS_RX_AAA\t{fr}", f"FAERS_D1\t{fa}\tFAERS_RX_BBB\t{fr}",
            f'FAERS_D2\t{fa}\t{{"drug_name":"unmapped"}}\t-1', f"FAERS_D2\t{fa}\tFAERS_RX_CCC\t{fr}",
        ], "faers")
        self.assertIn(("CHEMBL.COMPOUND:CHEMBL25", P, "faers.reaction:FAERS_RX_AAA"), edges)
        self.assertIn(("PUBCHEM.COMPOUND:2244", P, "faers.reaction:FAERS_RX_BBB"), edges)
        self.assertEqual(stats.edges_written, 4)  # 2 drug ids x 2 reactions; D2 emits 0

    def test_panelapp_gene_to_disease(self):
        """PanelApp: gene(HGNC) -> MONDO; OMIM mim + ensembl-dup not disease edges;
        no-MONDO entry emits nothing."""
        pg, hg, en, mi, mo = (self._id("panelapp_gene"), self._id("hgnc"),
                              self._id("ensembl"), self._id("mim"), self._id("mondo"))
        stats, edges = self._reified("panelapp_gene_sorted.1.index.gz", [
            f"105_SMAD2\t{pg}\t601366\t{mi}", f"105_SMAD2\t{pg}\tENSG00000175387\t{en}",
            f"105_SMAD2\t{pg}\tHGNC:6768\t{hg}", f"105_SMAD2\t{pg}\tMONDO:0018954\t{mo}",
            f'105_SMAD2\t{pg}\t{{"confidence":"amber"}}\t-1',
            f"1_ABL1\t{pg}\tHGNC:76\t{hg}",  # no MONDO -> nothing
        ], "panelapp_gene")
        self.assertEqual(stats.edges_written, 1)
        self.assertIn(("HGNC:6768", "biolink:gene_associated_with_condition", "MONDO:0018954"), edges)

    def test_mesh_is_not_a_categories_node(self):
        """MeSH (multi-type, 91% untyped chemicals) is NOT a static categories node;
        its disease subset is emitted by the mesh.py runtime builder instead."""
        self.assertFalse(self.cats.is_node_dataset("mesh"))
        self.assertIsNone(self.cats.category_for("mesh"))

    # --- Atlas-validated deferrals, now added ---------------------------------

    def test_mirdb_mirna_to_transcript(self):
        """miRDB: miRNA (group key) -> refseq transcript (runtime-prefixed object)."""
        mi, rs = self._id("mirdb"), self._id("refseq")
        _, edges = self._reified("mirdb_sorted.1.index.gz", [
            f'HSA-MIR-1\t{mi}\t{{"mirna_id":"hsa-mir-1"}}\t-1',
            f"HSA-MIR-1\t{mi}\tNM_000001\t{rs}",
        ], "mirdb")
        self.assertIn(("mirbase.mature:HSA-MIR-1", "biolink:affects", "refseq:NM_000001"), edges)

    def test_generif_pub_to_gene(self):
        """GeneRIF: publication (PMID) -> gene (mentions)."""
        gr, en, pm = self._id("generif"), self._id("entrez"), self._id("pubmed")
        _, edges = self._reified("generif_sorted.1.index.gz", [
            f"7157_111_0\t{gr}\t7157\t{en}", f"7157_111_0\t{gr}\t111\t{pm}",
            f'7157_111_0\t{gr}\t{{"x":1}}\t-1',
        ], "generif")
        self.assertIn(("PMID:111", "biolink:mentions", "NCBIGene:7157"), edges)

    def test_jaspar_motif_to_tf_protein(self):
        """JASPAR motif -> TF protein (directly_physically_interacts_with) + in_taxon."""
        ja, up, tx = self._id("jaspar"), self._id("uniprot"), self._id("taxonomy")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "jaspar_sorted.1.index.gz", [
                f"MA0001.1\t{ja}\tP29383\t{up}", f"MA0001.1\t{ja}\t3702\t{tx}",
                f'MA0001.1\t{ja}\t{{"name":"AGL3"}}\t-1',
            ])
            out = tmp / "e.tsv"
            build_edges(tmp, self.reg, self.cats, self.pm, out)
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in out.read_text().splitlines()[1:]}
            self.assertIn(("jaspar:MA0001.1", "biolink:directly_physically_interacts_with",
                           "UniProtKB:P29383"), edges)
            self.assertIn(("jaspar:MA0001.1", "biolink:in_taxon", "NCBITaxon:3702"), edges)

    def test_mesh_disease_subset_and_close_match(self):
        """mesh.py: only disease-tree (C*/F03*) MeSH -> Disease nodes; mondo->mesh
        close_match for those; chemical-tree MeSH excluded."""
        me, mo = self._id("mesh"), self._id("mondo")
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            self._write(tmp, "mesh_sorted.1.index.gz", [
                f'D1\t{me}\t{{"tree_numbers":["C04.557"],"descriptor_name":"neoplasm"}}\t-1',
                f'D2\t{me}\t{{"tree_numbers":["D02.491"],"descriptor_name":"a chemical"}}\t-1',
            ])
            self._write(tmp, "mondo_sorted.1.index.gz", [
                f"MONDO:1\t{mo}\tD1\t{me}",   # disease mesh -> close_match
                f"MONDO:2\t{mo}\tD2\t{me}",   # chemical mesh -> NOT emitted
            ])
            nout, eout = tmp / "n.tsv", tmp / "e.tsv"
            stats = build_mesh(tmp, self.reg, self.cats, nout, eout)
            nodes = {r.split("\t")[0]: r.split("\t")[1] for r in nout.read_text().splitlines()[1:]}
            edges = {(r.split("\t")[1], r.split("\t")[2], r.split("\t")[3])
                     for r in eout.read_text().splitlines()[1:]}
            self.assertEqual(stats.disease_nodes, 1)  # only D1 (C-tree)
            self.assertEqual(nodes.get("MESH:D1"), "biolink:Disease")
            self.assertNotIn("MESH:D2", nodes)  # chemical tree excluded
            self.assertEqual(edges, {("MONDO:1", "biolink:close_match", "MESH:D1")})


if __name__ == "__main__":
    unittest.main()
