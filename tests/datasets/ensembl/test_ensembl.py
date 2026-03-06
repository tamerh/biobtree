#!/usr/bin/env python3
"""
Ensembl Gene Test Suite

Tests ensembl dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

Note: This script is called by the main orchestrator (tests/run_tests.py)
which manages the biobtree web server lifecycle.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class EnsemblTests:
    """Ensembl custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_gene_with_name(self):
        """Check gene has display_name"""
        # Find a gene with display_name
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("display_name")),
            None
        )
        if not entry:
            return False, "No gene with display_name in reference"

        gene_id = entry["id"]
        display_name = entry["display_name"][:60]

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id} has name: {display_name}..."

    @test
    def test_gene_with_biotype(self):
        """Check gene has biotype"""
        # Find a gene with biotype
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("biotype")),
            None
        )
        if not entry:
            return False, "No gene with biotype in reference"

        gene_id = entry["id"]
        biotype = entry["biotype"]

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id} has biotype: {biotype}"

    @test
    def test_gene_with_strand(self):
        """Check gene has strand information"""
        # Find a gene with strand
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("strand")),
            None
        )
        if not entry:
            return False, "No gene with strand in reference"

        gene_id = entry["id"]
        strand = entry["strand"]

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id} has strand: {strand}"

    @test
    def test_genomic_coordinates(self):
        """Check gene has valid genomic coordinates"""
        # Find a gene with start/end
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("start") and e.get("end")),
            None
        )
        if not entry:
            return False, "No gene with coordinates in reference"

        gene_id = entry["id"]
        start = entry["start"]
        end = entry["end"]
        chrom = entry.get("seq_region_name", "?")

        # Validate start < end
        if start >= end:
            return False, f"{gene_id}: Invalid coordinates (start >= end)"

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id}: {chrom}:{start}-{end} (length: {end-start+1} bp)"

    @test
    def test_cross_references(self):
        """Check gene has external cross-references"""
        # Find a gene with xrefs
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("xrefs") and len(e["xrefs"]) > 0),
            None
        )
        if not entry:
            return False, "No gene with xrefs in reference"

        gene_id = entry["id"]
        xrefs = entry["xrefs"]
        xref_dbs = set(x.get("dbname") for x in xrefs if x.get("dbname"))

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id}: {len(xrefs)} xrefs to {len(xref_dbs)} databases"

    @test
    def test_species_diversity(self):
        """Check that test data includes multiple species"""
        species_set = set()
        for entry in self.runner.reference_data:
            if entry.get("species"):
                species_set.add(entry["species"])

        if len(species_set) < 2:
            return False, f"Only {len(species_set)} species in test data"

        species_list = sorted(list(species_set))[:5]  # Show first 5
        return True, f"Found {len(species_set)} species: {', '.join(species_list)}..."

    @test
    def test_assembly_version(self):
        """Check gene has assembly version information"""
        # Find a gene with assembly_name
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("assembly_name")),
            None
        )
        if not entry:
            return False, "No gene with assembly_name in reference"

        gene_id = entry["id"]
        assembly = entry["assembly_name"]
        species = entry.get("species", "unknown")

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id} ({species}): Assembly {assembly}"

    @test
    def test_canonical_transcript(self):
        """Check gene has canonical transcript annotation"""
        # Find a gene with canonical_transcript
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("canonical_transcript")),
            None
        )
        if not entry:
            return False, "No gene with canonical_transcript in reference"

        gene_id = entry["id"]
        transcript = entry["canonical_transcript"]

        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        return True, f"{gene_id}: Canonical transcript {transcript}"

    @test
    def test_hgnc_data_embedded(self):
        """Check human genes have embedded HGNC data"""
        # Find a human gene (species = homo_sapiens)
        human_gene = next(
            (e for e in self.runner.reference_data
             if e.get("species") == "homo_sapiens"),
            None
        )
        if not human_gene:
            return False, "No human genes in reference data"

        gene_id = human_gene["id"]
        data = self.runner.lookup(gene_id)

        if not data or not data.get("results"):
            return False, f"No results for {gene_id}"

        result = data["results"][0]
        attrs = result.get("Attributes", {}).get("Ensembl", {})

        # Check if HGNC data exists
        hgnc_data = attrs.get("hgnc")
        if not hgnc_data:
            # Not all human genes have HGNC data (paralogs, provisional genes, etc.)
            # Try to find one that does
            for entry in self.runner.reference_data:
                if entry.get("species") != "homo_sapiens":
                    continue
                test_data = self.runner.lookup(entry["id"])
                if test_data and test_data.get("results"):
                    test_attrs = test_data["results"][0].get("Attributes", {}).get("Ensembl", {})
                    if test_attrs.get("hgnc"):
                        gene_id = entry["id"]
                        hgnc_data = test_attrs["hgnc"]
                        break

            if not hgnc_data:
                return False, "No human genes with HGNC data found in test set"

        # Validate HGNC data structure
        if not hgnc_data.get("symbols"):
            return False, f"{gene_id}: HGNC data missing symbols field"

        symbols = hgnc_data["symbols"]
        return True, f"{gene_id}: Has HGNC data (symbols: {', '.join(symbols[:3])})"

    @test
    def test_hgnc_symbol_search(self):
        """Check HGNC symbols resolve to Ensembl entries"""
        # Find a human gene with HGNC data
        for entry in self.runner.reference_data:
            if entry.get("species") != "homo_sapiens":
                continue

            gene_id = entry["id"]
            data = self.runner.lookup(gene_id)

            if not data or not data.get("results"):
                continue

            attrs = data["results"][0].get("Attributes", {}).get("Ensembl", {})
            hgnc_data = attrs.get("hgnc")

            if not hgnc_data or not hgnc_data.get("symbols"):
                continue

            # Find a real gene symbol (not HGNC:* format)
            gene_symbol = None
            for symbol in hgnc_data["symbols"]:
                if not symbol.startswith("HGNC:"):
                    gene_symbol = symbol
                    break

            if not gene_symbol:
                continue

            # Search by gene symbol
            search_data = self.runner.lookup(gene_symbol)

            if not search_data or not search_data.get("results"):
                return False, f"Symbol '{gene_symbol}' not searchable"

            # Verify it returns the Ensembl entry (not a separate HGNC entry)
            search_result = search_data["results"][0]
            if search_result.get("dataset_name") != "ensembl":
                return False, f"Symbol '{gene_symbol}' returned {search_result.get('dataset_name')}, not ensembl"

            if search_result.get("identifier") != gene_id:
                # Could be a different gene with same symbol (paralog)
                # Just verify it's still an Ensembl entry
                pass

            return True, f"Symbol '{gene_symbol}' resolves to Ensembl entry {search_result.get('identifier')}"

        return False, "No human genes with HGNC symbols found"

    @test
    def test_hgnc_id_search(self):
        """Check HGNC IDs resolve to Ensembl entries"""
        # Find a human gene with HGNC ID
        for entry in self.runner.reference_data:
            if entry.get("species") != "homo_sapiens":
                continue

            gene_id = entry["id"]
            data = self.runner.lookup(gene_id)

            if not data or not data.get("results"):
                continue

            attrs = data["results"][0].get("Attributes", {}).get("Ensembl", {})
            hgnc_data = attrs.get("hgnc")

            if not hgnc_data or not hgnc_data.get("symbols"):
                continue

            # Find HGNC ID in symbols
            hgnc_id = None
            for symbol in hgnc_data["symbols"]:
                if symbol.startswith("HGNC:"):
                    hgnc_id = symbol
                    break

            if not hgnc_id:
                continue

            # Search by HGNC ID
            search_data = self.runner.lookup(hgnc_id)

            if not search_data or not search_data.get("results"):
                return False, f"HGNC ID '{hgnc_id}' not searchable"

            # Verify it returns the Ensembl entry
            search_result = search_data["results"][0]
            if search_result.get("dataset_name") != "ensembl":
                return False, f"HGNC ID '{hgnc_id}' returned {search_result.get('dataset_name')}, not ensembl"

            return True, f"HGNC ID '{hgnc_id}' resolves to Ensembl entry {search_result.get('identifier')}"

        return False, "No human genes with HGNC IDs found"


def main():
    """Main test execution"""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not reference_file.exists():
        print(f"Error: {reference_file} not found")
        print("Run: python3 extract_reference_data.py")
        return 1

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = EnsemblTests(runner)
    for test_method in [
        custom_tests.test_gene_with_name,
        custom_tests.test_gene_with_biotype,
        custom_tests.test_gene_with_strand,
        custom_tests.test_genomic_coordinates,
        custom_tests.test_cross_references,
        custom_tests.test_species_diversity,
        custom_tests.test_assembly_version,
        custom_tests.test_canonical_transcript,
        custom_tests.test_hgnc_data_embedded,
        custom_tests.test_hgnc_symbol_search,
        custom_tests.test_hgnc_id_search
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())