"""Subgraph projection tests: human spine, full vs scoped categories, per-source
caps, edge anchoring + endpoint pull-in, completeness.

    python3 -m unittest tools.kg_export.tests.test_subgraph -v
"""

import tempfile
import unittest
from pathlib import Path

from tools.kg_export import kgx
from tools.kg_export.subgraph import build_subgraph, check_completeness


def _w(p, header, rows):
    p.write_text(header + "\n" + "".join(r + "\n" for r in rows))


def _edge(subj, pred, obj, primary):
    return kgx.format_edge(subj, pred, obj, primary).rstrip("\n")


CONFIG = {
    "taxon": "NCBITaxon:9606",
    "full_categories": ["biolink:Disease", "biolink:OrganismTaxon"],
    "scoped_categories": ["biolink:Gene", "biolink:Protein", "biolink:SmallMolecule"],
    "full_sources": ["mondo"],
    "default_cap": 100,
    "caps": {"string_interaction": 2},
}


class SubgraphTests(unittest.TestCase):
    def _run(self, nodes, edges, config=CONFIG):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            _w(tmp / "n.tsv", kgx.NODE_HEADER, nodes)
            _w(tmp / "e.tsv", kgx.EDGE_HEADER, edges)
            st = build_subgraph(tmp / "n.tsv", tmp / "e.tsv", config,
                                tmp / "sn.tsv", tmp / "se.tsv")
            kept_n = {l.split("\t")[0] for l in (tmp / "sn.tsv").read_text().splitlines()[1:]}
            kept_e = {(p[1], p[2], p[3]) for p in
                      (l.split("\t") for l in (tmp / "se.tsv").read_text().splitlines()[1:])}
            return st, kept_n, kept_e

    def test_spine_full_vs_scoped_taxon(self):
        nodes = [
            "MONDO:1\tbiolink:Disease\td\tMONDO:1\tinfores:biobtree",          # full -> kept
            "HGNC:1\tbiolink:Gene\th\tHGNC:1\tinfores:biobtree",               # human gene (HGNC) -> kept
            "MGI:9\tbiolink:Gene\tm\tMGI:9\tinfores:biobtree",                 # mouse gene, no in_taxon -> dropped
            "NCBITaxon:9606\tbiolink:OrganismTaxon\thuman\tNCBITaxon:9606\tinfores:biobtree",
        ]
        edges = [_edge("HGNC:1", "biolink:in_taxon", "NCBITaxon:9606", "infores:ensembl")]
        st, kept_n, _ = self._run(nodes, edges)
        self.assertIn("MONDO:1", kept_n)       # full category
        self.assertIn("HGNC:1", kept_n)        # human gene
        self.assertNotIn("MGI:9", kept_n)      # non-human scoped, not pulled by any edge

    def test_uncapped_full_source_and_capped_big_source(self):
        nodes = [f"UniProtKB:P{i}\tbiolink:Protein\tp\tUniProtKB:P{i}\tinfores:biobtree" for i in range(5)]
        nodes.append("HGNC:1\tbiolink:Gene\th\tHGNC:1\tinfores:biobtree")
        edges = [_edge("HGNC:1", "biolink:in_taxon", "NCBITaxon:9606", "infores:ensembl")]
        # 4 string_interaction edges from the human gene -> cap 2 keeps 2
        for i in range(4):
            edges.append(_edge("HGNC:1", "biolink:physically_interacts_with",
                               f"UniProtKB:P{i}", "infores:string_interaction"))
        st, kept_n, kept_e = self._run(nodes, edges)
        ppi = [e for e in kept_e if e[1] == "biolink:physically_interacts_with"]
        self.assertEqual(len(ppi), 2)                 # capped at 2
        self.assertEqual(st.capped_dropped, 2)

    def test_object_axis_cap(self):
        """Cap by OBJECT: clinvar variant--is_sequence_variant_of-->gene must cap
        variants PER GENE (the gene is the object), not per variant."""
        cfg = dict(CONFIG)
        cfg["caps"] = {"clinvar": {"n": 2, "by": "object"}}
        nodes = ["HGNC:1\tbiolink:Gene\th\tHGNC:1\tinfores:biobtree"]
        nodes += [f"CLINVAR:{i}\tbiolink:SequenceVariant\tv\tCLINVAR:{i}\tinfores:biobtree"
                  for i in range(3)]
        edges = [_edge("HGNC:1", "biolink:in_taxon", "NCBITaxon:9606", "infores:ensembl")]
        for i in range(3):  # 3 variants -> same gene; cap 2 (per object) keeps 2
            edges.append(_edge(f"CLINVAR:{i}", "biolink:is_sequence_variant_of",
                               "HGNC:1", "infores:clinvar"))
        st, kept_n, kept_e = self._run(nodes, edges, cfg)
        var = [e for e in kept_e if e[1] == "biolink:is_sequence_variant_of"]
        self.assertEqual(len(var), 2)                 # 3 -> 2 (capped per gene/object)
        self.assertEqual(st.capped_dropped, 1)

    def test_structural_children_exon_pass3b(self):
        """transcript--has_part-->exon must survive even though the Ensembl transcript
        is only an endpoint (never in spine): pass 3b anchors children on kept subjects."""
        cfg = dict(CONFIG)
        cfg["scoped_categories"] = CONFIG["scoped_categories"] + ["biolink:Transcript", "biolink:Exon"]
        nodes = [
            "HGNC:1\tbiolink:Gene\th\tHGNC:1\tinfores:biobtree",
            "ENSEMBL:T1\tbiolink:Transcript\tt\tENSEMBL:T1\tinfores:biobtree",   # endpoint only
            "ENSEMBL:E1\tbiolink:Exon\te\tENSEMBL:E1\tinfores:biobtree",
            "ENSEMBL:E2\tbiolink:Exon\te\tENSEMBL:E2\tinfores:biobtree",
        ]
        edges = [
            _edge("HGNC:1", "biolink:in_taxon", "NCBITaxon:9606", "infores:ensembl"),
            _edge("ENSEMBL:T1", "biolink:transcribed_from", "HGNC:1", "infores:ensembl"),  # T1 -> spine gene
            _edge("ENSEMBL:T1", "biolink:has_part", "ENSEMBL:E1", "infores:ensembl"),
            _edge("ENSEMBL:T1", "biolink:has_part", "ENSEMBL:E2", "infores:ensembl"),
        ]
        st, kept_n, kept_e = self._run(nodes, edges, cfg)
        self.assertIn("ENSEMBL:T1", kept_n)                   # transcript kept (endpoint)
        self.assertIn("ENSEMBL:E1", kept_n)                   # exon pulled in by pass 3b
        self.assertIn("ENSEMBL:E2", kept_n)
        self.assertEqual(len([e for e in kept_e if e[1] == "biolink:has_part"]), 2)

    def test_edge_pulls_in_capped_neighbour(self):
        """A compound (scoped, non-human) is NOT in the spine but enters via a kept
        edge from a human protein."""
        nodes = [
            "HGNC:1\tbiolink:Gene\th\tHGNC:1\tinfores:biobtree",
            "CHEMBL.COMPOUND:C1\tbiolink:SmallMolecule\tc\tCHEMBL.COMPOUND:C1\tinfores:biobtree",
        ]
        edges = [
            _edge("HGNC:1", "biolink:in_taxon", "NCBITaxon:9606", "infores:ensembl"),
            _edge("CHEMBL.COMPOUND:C1", "biolink:affects", "HGNC:1", "infores:chembl_activity"),
        ]
        st, kept_n, kept_e = self._run(nodes, edges)
        self.assertIn("CHEMBL.COMPOUND:C1", kept_n)   # pulled in by the anchored edge
        self.assertIn(("CHEMBL.COMPOUND:C1", "biolink:affects", "HGNC:1"), kept_e)

    def test_full_prefix_keeps_chemical_and_its_crossref(self):
        """A curated chemical (CHEBI, in full_prefixes) is kept full despite being a
        scoped category + non-human, and its close_match pulls in the PUBCHEM twin --
        so molecule cross-refs (close_match) survive (the human/disease/molecule model)."""
        cfg = dict(CONFIG)
        cfg["full_prefixes"] = ["CHEBI"]
        nodes = [
            "HGNC:1\tbiolink:Gene\th\tHGNC:1\tinfores:biobtree",
            "CHEBI:1\tbiolink:SmallMolecule\tc\tCHEBI:1\tinfores:biobtree",                # curated -> full
            "PUBCHEM.COMPOUND:9\tbiolink:SmallMolecule\tp\tPUBCHEM.COMPOUND:9\tinfores:biobtree",  # scoped catalog
        ]
        edges = [
            _edge("HGNC:1", "biolink:in_taxon", "NCBITaxon:9606", "infores:ensembl"),
            _edge("CHEBI:1", "biolink:close_match", "PUBCHEM.COMPOUND:9", "infores:hmdb"),
        ]
        st, kept_n, kept_e = self._run(nodes, edges, cfg)
        self.assertIn("CHEBI:1", kept_n)                       # full prefix, no anchor needed
        self.assertIn("PUBCHEM.COMPOUND:9", kept_n)            # pulled in via close_match
        self.assertIn(("CHEBI:1", "biolink:close_match", "PUBCHEM.COMPOUND:9"), kept_e)

    def test_unanchored_edge_dropped(self):
        nodes = ["HGNC:1\tbiolink:Gene\th\tHGNC:1\tinfores:biobtree"]
        edges = [
            _edge("HGNC:1", "biolink:in_taxon", "NCBITaxon:9606", "infores:ensembl"),
            _edge("MGI:9", "biolink:orthologous_to", "MGI:8", "infores:ortholog"),  # neither in spine
        ]
        st, _, kept_e = self._run(nodes, edges)
        self.assertEqual(st.unanchored_dropped, 1)
        self.assertNotIn(("MGI:9", "biolink:orthologous_to", "MGI:8"), kept_e)

    def test_completeness_flags_missing_predicate(self):
        st, _, _ = self._run(
            ["HGNC:1\tbiolink:Gene\th\tHGNC:1\tinfores:biobtree"],
            [_edge("HGNC:1", "biolink:in_taxon", "NCBITaxon:9606", "infores:ensembl")])
        mani = {"node_categories": {"biolink:Gene": 1, "biolink:Disease": 9},
                "edge_predicates": {"biolink:in_taxon": 1, "biolink:affects": 5}}
        comp = check_completeness(st, mani)
        self.assertFalse(comp["ok"])
        self.assertIn("biolink:Disease", comp["missing_categories"])
        self.assertIn("biolink:affects", comp["missing_predicates"])


if __name__ == "__main__":
    unittest.main()
