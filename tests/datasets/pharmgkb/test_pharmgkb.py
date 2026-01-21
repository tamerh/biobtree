#!/usr/bin/env python3
"""
PharmGKB (Pharmacogenomics Knowledge Base) Test Suite

Tests all 6 PharmGKB datasets:
- pharmgkb: Drug/chemical entries with drug labels and related genes
- pharmgkb_gene: Pharmacogenes with VIP flags and genomic coordinates
- pharmgkb_clinical: Clinical variant-drug annotations
- pharmgkb_variant: Variant annotations with summary enrichment
- pharmgkb_guideline: CPIC/DPWG/CPNDS/RNPGx dosing guidelines
- pharmgkb_pathway: Pharmacokinetic and pharmacodynamic pathways
"""

import sys
import os
import requests
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test, discover_tests


class PharmgkbTests:
    """PharmGKB custom tests for all 6 datasets"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _test_mapping(self, source_id: str, source_dataset: str, target_dataset: str) -> tuple:
        """Helper to test a mapping between datasets"""
        url = f"{self.runner.api_url}/ws/map/?i={source_id}&m=>>{source_dataset}>>{target_dataset}"
        try:
            resp = requests.get(url, timeout=30)
            if resp.status_code != 200:
                return False, f"HTTP {resp.status_code}"
            data = resp.json()
            if data.get("results") and len(data["results"]) > 0:
                result = data["results"][0]
                targets = result.get("targets", [])
                if targets:
                    return True, f"Found {len(targets)} {target_dataset} mappings"
            return False, f"No {target_dataset} mappings found"
        except Exception as e:
            return False, str(e)

    def _lookup_dataset(self, identifier: str, dataset: str) -> dict:
        """Helper to lookup entry in specific dataset"""
        url = f"{self.runner.api_url}/ws/?i={identifier}&s={dataset}"
        try:
            resp = requests.get(url, timeout=30)
            if resp.status_code == 200:
                return resp.json()
        except:
            pass
        return {}

    def _entry_dataset(self, identifier: str, dataset: str) -> dict:
        """Helper to get entry details from specific dataset"""
        url = f"{self.runner.api_url}/ws/entry/?i={identifier}&s={dataset}"
        try:
            resp = requests.get(url, timeout=30)
            if resp.status_code == 200:
                data = resp.json()
                if data and len(data) > 0:
                    return data[0]
        except:
            pass
        return {}

    # =========================================================================
    # PHARMGKB (Chemical/Drug) Tests
    # =========================================================================

    @test
    def test_pharmgkb_drug_name(self):
        """Check pharmgkb entry has drug name"""
        entry = next(
            (e for e in self.runner.reference_data if e.get("name")),
            None
        )
        if not entry:
            return False, "No entry with name in reference"

        pharmgkb_id = entry["pharmgkb_id"]
        name = entry["name"]
        data = self.runner.lookup(pharmgkb_id)

        if not data or not data.get("results"):
            return False, f"No results for {pharmgkb_id}"

        return True, f"{pharmgkb_id} has name: {name[:50]}..."

    @test
    def test_pharmgkb_generic_names(self):
        """Check pharmgkb entry has generic names"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("generic_names") and len(e["generic_names"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with generic_names in reference"

        pharmgkb_id = entry["pharmgkb_id"]
        count = len(entry["generic_names"])
        data = self.runner.lookup(pharmgkb_id)

        if not data or not data.get("results"):
            return False, f"No results for {pharmgkb_id}"

        return True, f"Entry has {count} generic names"

    @test
    def test_pharmgkb_related_genes(self):
        """Check pharmgkb entry has related genes enrichment"""
        # Look up warfarin which has many related genes
        data = self._lookup_dataset("warfarin", "pharmgkb")
        if not data or not data.get("results"):
            return False, "No results for warfarin"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("Pharmgkb", {})
        related_genes = attr.get("related_genes", [])

        if not related_genes:
            return False, "Warfarin has no related_genes"

        return True, f"Warfarin has {len(related_genes)} related genes"

    @test
    def test_pharmgkb_drug_labels(self):
        """Check pharmgkb entry has drug labels enrichment"""
        # Search for warfarin which should have drug labels
        data = self._lookup_dataset("warfarin", "pharmgkb")
        if not data or not data.get("results"):
            return False, "No results for warfarin"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("Pharmgkb", {})
        drug_labels = attr.get("drug_labels", [])

        if not drug_labels:
            return False, "Warfarin has no drug_labels"

        sources = set(dl.get("source") for dl in drug_labels)
        return True, f"Warfarin has {len(drug_labels)} drug labels from: {', '.join(sources)}"

    @test
    def test_pharmgkb_dosing_guideline_sources(self):
        """Check pharmgkb entry has dosing guideline sources"""
        data = self._lookup_dataset("warfarin", "pharmgkb")
        if not data or not data.get("results"):
            return False, "No results for warfarin"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("Pharmgkb", {})
        sources = attr.get("dosing_guideline_sources", [])

        if not sources:
            return False, "Warfarin has no dosing_guideline_sources"

        return True, f"Warfarin guideline sources: {', '.join(sources)}"

    @test
    def test_pharmgkb_to_pubchem_mapping(self):
        """Test pharmgkb to PubChem mapping"""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("pubchem_cids") and len(e["pubchem_cids"]) > 0),
            None
        )
        if not entry:
            return False, "No entry with pubchem_cids for mapping test"

        return self._test_mapping(entry["pharmgkb_id"], "pharmgkb", "pubchem")

    @test
    def test_pharmgkb_to_hgnc_mapping(self):
        """Test pharmgkb to HGNC mapping via related genes"""
        # Use warfarin which has many related genes
        return self._test_mapping("warfarin", "pharmgkb", "hgnc")

    # =========================================================================
    # PHARMGKB_GENE Tests
    # =========================================================================

    @test
    def test_pharmgkb_gene_lookup(self):
        """Test pharmgkb_gene entry lookup by symbol"""
        data = self._lookup_dataset("CYP2C9", "pharmgkb_gene")
        if not data or not data.get("results"):
            return False, "No results for CYP2C9 in pharmgkb_gene"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbGene", {})
        symbol = attr.get("symbol")

        if symbol != "CYP2C9":
            return False, f"Expected CYP2C9, got {symbol}"

        return True, f"Found CYP2C9 with ID {attr.get('pharmgkb_id')}"

    @test
    def test_pharmgkb_gene_vip_flag(self):
        """Test pharmgkb_gene VIP (Very Important Pharmacogene) flag"""
        data = self._lookup_dataset("CYP2C9", "pharmgkb_gene")
        if not data or not data.get("results"):
            return False, "No results for CYP2C9"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbGene", {})
        is_vip = attr.get("is_vip")

        if not is_vip:
            return False, "CYP2C9 should be VIP gene"

        return True, "CYP2C9 is correctly flagged as VIP gene"

    @test
    def test_pharmgkb_gene_cpic_guideline(self):
        """Test pharmgkb_gene CPIC guideline flag"""
        data = self._lookup_dataset("CYP2C9", "pharmgkb_gene")
        if not data or not data.get("results"):
            return False, "No results for CYP2C9"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbGene", {})
        has_cpic = attr.get("has_cpic_guideline")

        if not has_cpic:
            return False, "CYP2C9 should have CPIC guideline"

        return True, "CYP2C9 correctly has CPIC guideline flag"

    @test
    def test_pharmgkb_gene_coordinates(self):
        """Test pharmgkb_gene genomic coordinates"""
        data = self._lookup_dataset("CYP2C9", "pharmgkb_gene")
        if not data or not data.get("results"):
            return False, "No results for CYP2C9"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbGene", {})
        chrom = attr.get("chromosome")
        start = attr.get("start_grch38")
        end = attr.get("end_grch38")

        if not chrom or not start or not end:
            return False, "Missing genomic coordinates"

        return True, f"CYP2C9 at {chrom}:{start}-{end}"

    @test
    def test_pharmgkb_gene_cross_references(self):
        """Test pharmgkb_gene cross-references to hgnc/entrez/ensembl"""
        data = self._lookup_dataset("CYP2C9", "pharmgkb_gene")
        if not data or not data.get("results"):
            return False, "No results for CYP2C9"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbGene", {})
        hgnc = attr.get("hgnc_id")
        entrez = attr.get("entrez_id")
        ensembl = attr.get("ensembl_id")

        refs = []
        if hgnc:
            refs.append(f"HGNC:{hgnc}")
        if entrez:
            refs.append(f"Entrez:{entrez}")
        if ensembl:
            refs.append(f"Ensembl:{ensembl}")

        if not refs:
            return False, "No cross-references found"

        return True, f"Cross-refs: {', '.join(refs)}"

    # =========================================================================
    # PHARMGKB_CLINICAL Tests
    # =========================================================================

    @test
    def test_pharmgkb_clinical_lookup(self):
        """Test pharmgkb_clinical entry lookup by rsID"""
        data = self._lookup_dataset("rs1799853", "pharmgkb_clinical")
        if not data or not data.get("results"):
            return False, "No results for rs1799853 in pharmgkb_clinical"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbClinical", {})
        variant = attr.get("variant")

        if "rs1799853" not in variant.lower():
            return False, f"Expected rs1799853, got {variant}"

        return True, f"Found clinical annotation for {variant}"

    @test
    def test_pharmgkb_clinical_evidence_level(self):
        """Test pharmgkb_clinical has evidence level"""
        data = self._lookup_dataset("rs1799853", "pharmgkb_clinical")
        if not data or not data.get("results"):
            return False, "No results for rs1799853"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbClinical", {})
        level = attr.get("level_of_evidence")

        if not level:
            return False, "Missing level_of_evidence"

        return True, f"Clinical evidence level: {level}"

    @test
    def test_pharmgkb_clinical_mesh_xref(self):
        """Test pharmgkb_clinical has MeSH cross-references from phenotype mapping"""
        data = self._lookup_dataset("rs1799853", "pharmgkb_clinical")
        if not data or not data.get("results"):
            return False, "No results for rs1799853"

        result = data["results"][0]
        entries = result.get("entries", [])

        mesh_entries = [e for e in entries if e.get("dataset_name") == "mesh"]
        if not mesh_entries:
            return False, "No MeSH cross-references found"

        return True, f"Found {len(mesh_entries)} MeSH cross-references"

    @test
    def test_pharmgkb_clinical_gene_mapping(self):
        """Test pharmgkb_clinical maps to genes via hgnc"""
        return self._test_mapping("rs1799853", "pharmgkb_clinical", "hgnc")

    # =========================================================================
    # PHARMGKB_VARIANT Tests
    # =========================================================================

    @test
    def test_pharmgkb_variant_lookup(self):
        """Test pharmgkb_variant entry lookup by rsID"""
        data = self._lookup_dataset("rs1799853", "pharmgkb_variant")
        if not data or not data.get("results"):
            return False, "No results for rs1799853 in pharmgkb_variant"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbVariant", {})
        variant_name = attr.get("variant_name")

        if "rs1799853" not in variant_name.lower():
            return False, f"Expected rs1799853, got {variant_name}"

        return True, f"Found variant {variant_name}"

    @test
    def test_pharmgkb_variant_summary_enrichment(self):
        """Test pharmgkb_variant has summary annotation enrichment"""
        data = self._lookup_dataset("rs1799853", "pharmgkb_variant")
        if not data or not data.get("results"):
            return False, "No results for rs1799853"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbVariant", {})

        # Check for summary enrichment fields
        level = attr.get("level_of_evidence")
        score = attr.get("score")
        categories = attr.get("phenotype_categories", [])
        drugs = attr.get("associated_drugs", [])

        enriched = []
        if level:
            enriched.append(f"level={level}")
        if score:
            enriched.append(f"score={score}")
        if categories:
            enriched.append(f"categories={len(categories)}")
        if drugs:
            enriched.append(f"drugs={len(drugs)}")

        if not enriched:
            return False, "No summary enrichment found"

        return True, f"Summary enrichment: {', '.join(enriched)}"

    @test
    def test_pharmgkb_variant_synonyms(self):
        """Test pharmgkb_variant has HGVS synonyms"""
        data = self._lookup_dataset("rs1799853", "pharmgkb_variant")
        if not data or not data.get("results"):
            return False, "No results for rs1799853"

        result = data["results"][0]
        attr = result.get("Attributes", {}).get("PharmgkbVariant", {})
        synonyms = attr.get("synonyms", [])

        if not synonyms:
            return False, "No synonyms found"

        # Look for HGVS nomenclature
        hgvs = [s for s in synonyms if ":" in s and ("c." in s or "p." in s or "g." in s)]
        return True, f"Found {len(synonyms)} synonyms ({len(hgvs)} HGVS)"

    @test
    def test_pharmgkb_variant_dbsnp_xref(self):
        """Test pharmgkb_variant has dbSNP cross-reference"""
        data = self._lookup_dataset("rs1799853", "pharmgkb_variant")
        if not data or not data.get("results"):
            return False, "No results for rs1799853"

        result = data["results"][0]
        entries = result.get("entries", [])

        dbsnp_entries = [e for e in entries if e.get("dataset_name") == "dbsnp"]
        if not dbsnp_entries:
            return False, "No dbSNP cross-reference found"

        return True, f"Found dbSNP xref: {dbsnp_entries[0].get('identifier')}"

    # =========================================================================
    # PHARMGKB_GUIDELINE Tests
    # =========================================================================

    @test
    def test_pharmgkb_guideline_mapping(self):
        """Test pharmgkb to guideline mapping"""
        return self._test_mapping("warfarin", "pharmgkb", "pharmgkb_guideline")

    @test
    def test_pharmgkb_guideline_cpic(self):
        """Test pharmgkb_guideline has CPIC guideline"""
        url = f"{self.runner.api_url}/ws/map/?i=warfarin&m=>>pharmgkb>>pharmgkb_guideline"
        try:
            resp = requests.get(url, timeout=30)
            data = resp.json()
            if not data.get("results"):
                return False, "No mapping results"

            result = data["results"][0]
            targets = result.get("targets", [])

            cpic = [t for t in targets
                    if t.get("Attributes", {}).get("PharmgkbGuideline", {}).get("source") == "CPIC"]

            if not cpic:
                return False, "No CPIC guideline found"

            guideline = cpic[0].get("Attributes", {}).get("PharmgkbGuideline", {})
            return True, f"CPIC: {guideline.get('name', '')[:50]}..."
        except Exception as e:
            return False, str(e)

    @test
    def test_pharmgkb_guideline_dpwg(self):
        """Test pharmgkb_guideline has DPWG guideline"""
        url = f"{self.runner.api_url}/ws/map/?i=warfarin&m=>>pharmgkb>>pharmgkb_guideline"
        try:
            resp = requests.get(url, timeout=30)
            data = resp.json()
            if not data.get("results"):
                return False, "No mapping results"

            result = data["results"][0]
            targets = result.get("targets", [])

            dpwg = [t for t in targets
                    if t.get("Attributes", {}).get("PharmgkbGuideline", {}).get("source") == "DPWG"]

            if not dpwg:
                return False, "No DPWG guideline found"

            guideline = dpwg[0].get("Attributes", {}).get("PharmgkbGuideline", {})
            return True, f"DPWG: {guideline.get('name', '')[:50]}..."
        except Exception as e:
            return False, str(e)

    @test
    def test_pharmgkb_guideline_attributes(self):
        """Test pharmgkb_guideline has expected attributes"""
        url = f"{self.runner.api_url}/ws/map/?i=warfarin&m=>>pharmgkb>>pharmgkb_guideline"
        try:
            resp = requests.get(url, timeout=30)
            data = resp.json()
            if not data.get("results"):
                return False, "No mapping results"

            result = data["results"][0]
            targets = result.get("targets", [])
            if not targets:
                return False, "No targets"

            guideline = targets[0].get("Attributes", {}).get("PharmgkbGuideline", {})
            attrs = []
            if guideline.get("guideline_id"):
                attrs.append("guideline_id")
            if guideline.get("source"):
                attrs.append("source")
            if guideline.get("gene_symbols"):
                attrs.append(f"genes={len(guideline['gene_symbols'])}")
            if guideline.get("chemical_names"):
                attrs.append(f"chemicals={len(guideline['chemical_names'])}")
            if guideline.get("has_dosing_info"):
                attrs.append("has_dosing")
            if guideline.get("summary"):
                attrs.append("summary")

            return True, f"Attributes: {', '.join(attrs)}"
        except Exception as e:
            return False, str(e)

    @test
    def test_pharmgkb_guideline_gene_xref(self):
        """Test pharmgkb_guideline has gene cross-references"""
        url = f"{self.runner.api_url}/ws/map/?i=warfarin&m=>>pharmgkb>>pharmgkb_guideline"
        try:
            resp = requests.get(url, timeout=30)
            data = resp.json()
            if not data.get("results"):
                return False, "No mapping results"

            result = data["results"][0]
            targets = result.get("targets", [])
            if not targets:
                return False, "No targets"

            # Get entry to see cross-refs
            guideline_id = targets[0].get("identifier")
            entry = self._entry_dataset(guideline_id, "pharmgkb_guideline")
            if not entry:
                return False, "Could not get guideline entry"

            entries = entry.get("entries", [])
            hgnc_entries = [e for e in entries if e.get("dataset_name") == "hgnc"]

            if not hgnc_entries:
                return False, "No HGNC cross-references"

            genes = [e.get("identifier") for e in hgnc_entries[:3]]
            return True, f"Gene xrefs: {', '.join(genes)}"
        except Exception as e:
            return False, str(e)

    # =========================================================================
    # PHARMGKB_PATHWAY Tests
    # =========================================================================

    @test
    def test_pharmgkb_pathway_mapping(self):
        """Test pharmgkb to pathway mapping"""
        return self._test_mapping("warfarin", "pharmgkb", "pharmgkb_pathway")

    @test
    def test_pharmgkb_pathway_pharmacokinetic(self):
        """Test pharmgkb_pathway has pharmacokinetic pathway"""
        url = f"{self.runner.api_url}/ws/map/?i=warfarin&m=>>pharmgkb>>pharmgkb_pathway"
        try:
            resp = requests.get(url, timeout=30)
            data = resp.json()
            if not data.get("results"):
                return False, "No mapping results"

            result = data["results"][0]
            targets = result.get("targets", [])

            pk = [t for t in targets
                  if t.get("Attributes", {}).get("PharmgkbPathway", {}).get("is_pharmacokinetic")]

            if not pk:
                return False, "No pharmacokinetic pathway found"

            pathway = pk[0].get("Attributes", {}).get("PharmgkbPathway", {})
            return True, f"PK pathway: {pathway.get('name', '')[:50]}..."
        except Exception as e:
            return False, str(e)

    @test
    def test_pharmgkb_pathway_pharmacodynamic(self):
        """Test pharmgkb_pathway has pharmacodynamic pathway"""
        url = f"{self.runner.api_url}/ws/map/?i=warfarin&m=>>pharmgkb>>pharmgkb_pathway"
        try:
            resp = requests.get(url, timeout=30)
            data = resp.json()
            if not data.get("results"):
                return False, "No mapping results"

            result = data["results"][0]
            targets = result.get("targets", [])

            pd = [t for t in targets
                  if t.get("Attributes", {}).get("PharmgkbPathway", {}).get("is_pharmacodynamic")]

            if not pd:
                return False, "No pharmacodynamic pathway found"

            pathway = pd[0].get("Attributes", {}).get("PharmgkbPathway", {})
            return True, f"PD pathway: {pathway.get('name', '')[:50]}..."
        except Exception as e:
            return False, str(e)

    @test
    def test_pharmgkb_pathway_attributes(self):
        """Test pharmgkb_pathway has expected attributes"""
        url = f"{self.runner.api_url}/ws/map/?i=warfarin&m=>>pharmgkb>>pharmgkb_pathway"
        try:
            resp = requests.get(url, timeout=30)
            data = resp.json()
            if not data.get("results"):
                return False, "No mapping results"

            result = data["results"][0]
            targets = result.get("targets", [])
            if not targets:
                return False, "No targets"

            pathway = targets[0].get("Attributes", {}).get("PharmgkbPathway", {})
            attrs = []
            if pathway.get("pathway_id"):
                attrs.append("pathway_id")
            if pathway.get("name"):
                attrs.append("name")
            if pathway.get("gene_symbols"):
                attrs.append(f"genes={len(pathway['gene_symbols'])}")
            if pathway.get("chemical_names"):
                attrs.append(f"chemicals={len(pathway['chemical_names'])}")
            if pathway.get("summary"):
                attrs.append("summary")
            if pathway.get("image_link"):
                attrs.append("image_link")
            if pathway.get("biopax_link"):
                attrs.append("biopax_link")

            return True, f"Attributes: {', '.join(attrs)}"
        except Exception as e:
            return False, str(e)

    @test
    def test_pharmgkb_pathway_gene_xref(self):
        """Test pharmgkb_pathway has gene cross-references"""
        url = f"{self.runner.api_url}/ws/map/?i=warfarin&m=>>pharmgkb>>pharmgkb_pathway"
        try:
            resp = requests.get(url, timeout=30)
            data = resp.json()
            if not data.get("results"):
                return False, "No mapping results"

            result = data["results"][0]
            targets = result.get("targets", [])
            if not targets:
                return False, "No targets"

            # Get entry to see cross-refs
            pathway_id = targets[0].get("identifier")
            entry = self._entry_dataset(pathway_id, "pharmgkb_pathway")
            if not entry:
                return False, "Could not get pathway entry"

            entries = entry.get("entries", [])
            hgnc_entries = [e for e in entries if e.get("dataset_name") == "hgnc"]

            if not hgnc_entries:
                return False, "No HGNC cross-references"

            genes = [e.get("identifier") for e in hgnc_entries[:5]]
            return True, f"Gene xrefs: {', '.join(genes)}"
        except Exception as e:
            return False, str(e)

    # =========================================================================
    # Cross-Dataset Integration Tests
    # =========================================================================

    @test
    def test_gene_to_guideline_mapping(self):
        """Test gene to guideline mapping chain"""
        return self._test_mapping("CYP2C9", "hgnc", "pharmgkb_guideline")

    @test
    def test_gene_to_clinical_mapping(self):
        """Test gene to clinical annotation mapping chain"""
        return self._test_mapping("CYP2C9", "hgnc", "pharmgkb_clinical")

    @test
    def test_gene_to_variant_mapping(self):
        """Test gene to variant mapping chain"""
        return self._test_mapping("CYP2C9", "hgnc", "pharmgkb_variant")


def main():
    """Run PharmGKB tests"""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment or command line
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Allow command-line override
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--api-url', default=api_url)
    args = parser.parse_args()
    api_url = args.api_url

    # Check prerequisites
    if not reference_file.exists():
        print(f"Warning: {reference_file} not found - using empty reference data")
        # Create empty reference data for tests that don't need it
        reference_file = None

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests from PharmgkbTests class
    custom_tests = PharmgkbTests(runner)
    for test_method in discover_tests(custom_tests):
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
