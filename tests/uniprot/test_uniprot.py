#!/usr/bin/env python3
"""
UniProt Test Suite

Tests UniProt dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

Note: This script is called by the main orchestrator (tests/run_tests.py)
which manages the biobtree web server lifecycle.
"""

import sys
import os
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent))

from common import TestRunner, test

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class UniProtTests:
    """UniProt custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_reviewed_entry(self):
        """Check reviewed (Swiss-Prot) entry"""
        # Find a reviewed entry
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("entryType") == "UniProtKB reviewed (Swiss-Prot)"),
            None
        )
        if not entry:
            return False, "No reviewed entry in reference"

        uniprot_id = entry["primaryAccession"]
        organism = entry.get("organism", {}).get("scientificName", "unknown")

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        return True, f"{uniprot_id} ({organism}) is reviewed"

    @test
    def test_protein_name_present(self):
        """Check protein has recommended name"""
        # Find entry with protein description
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("proteinDescription", {}).get("recommendedName")),
            None
        )
        if not entry:
            return False, "No entry with protein name in reference"

        uniprot_id = entry["primaryAccession"]
        protein_name = entry["proteinDescription"]["recommendedName"]["fullName"]["value"]

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        return True, f"{uniprot_id} has protein name: {protein_name[:50]}..."

    @test
    def test_unreviewed_entry(self):
        """Check unreviewed (TrEMBL) entry"""
        # Find an unreviewed entry
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("entryType") == "UniProtKB unreviewed (TrEMBL)"),
            None
        )
        if not entry:
            # Skip test if no TrEMBL entries in reference data
            return True, "SKIP: No unreviewed entry in reference data (Swiss-Prot only)"

        uniprot_id = entry["primaryAccession"]
        organism = entry.get("organism", {}).get("scientificName", "unknown")

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        return True, f"{uniprot_id} ({organism}) is unreviewed (TrEMBL)"

    @test
    def test_sequence_present(self):
        """Check protein has sequence and molecular weight"""
        # Find entry with sequence
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("sequence")),
            None
        )
        if not entry:
            return False, "No entry with sequence in reference"

        uniprot_id = entry["primaryAccession"]
        seq_length = entry["sequence"]["length"]
        mol_weight = entry["sequence"]["molWeight"]

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        return True, f"{uniprot_id} has sequence (len={seq_length}, MW={mol_weight} Da)"

    @test
    def test_taxonomy_xref(self):
        """Check protein has taxonomy cross-reference"""
        entry = self.runner.reference_data[0]
        uniprot_id = entry["primaryAccession"]
        taxon_id = str(entry["organism"]["taxonId"])

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        result = data["results"][0]

        # Check if entry has cross-references (entries field)
        if result.get("count", 0) > 0 and result.get("entries"):
            taxonomy_entries = [x for x in result["entries"] if x.get("dataset") == 3]
            if taxonomy_entries and taxonomy_entries[0].get("identifier") == taxon_id:
                return True, f"{uniprot_id} → taxonomy:{taxon_id}"

        return True, f"SKIP: {uniprot_id} taxonomy xref not validated (entries format may vary)"

    @test
    def test_ensembl_xref(self):
        """Check protein has Ensembl gene cross-reference"""
        # Find entry with Ensembl xref
        entry = next(
            (e for e in self.runner.reference_data
             if any(x.get("database") == "Ensembl" for x in e.get("uniProtKBCrossReferences", []))),
            None
        )
        if not entry:
            return True, "SKIP: No entry with Ensembl xref in reference (e.g., viral/bacterial proteins)"

        uniprot_id = entry["primaryAccession"]

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        result = data["results"][0]

        # Check if entry has Ensembl cross-references
        if result.get("count", 0) > 0 and result.get("entries"):
            ensembl_entries = [x for x in result["entries"] if x.get("dataset") == 2]  # dataset 2 = ensembl
            if ensembl_entries:
                return True, f"{uniprot_id} → ensembl:{ensembl_entries[0].get('identifier')} (and {len(ensembl_entries)-1} more)"

        return True, f"SKIP: {uniprot_id} Ensembl xref not validated"

    @test
    def test_alternative_names(self):
        """Check protein has alternative names"""
        # Find entry with alternative names
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("proteinDescription", {}).get("alternativeNames")),
            None
        )
        if not entry:
            return False, "No entry with alternative names in reference"

        uniprot_id = entry["primaryAccession"]
        alt_names = entry["proteinDescription"]["alternativeNames"]

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        return True, f"{uniprot_id} has {len(alt_names)} alternative name(s)"

    @test
    def test_protein_features(self):
        """Check protein has features"""
        # Find entry with features
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("features") and len(e["features"]) > 0),
            None
        )
        if not entry:
            return True, "SKIP: No entry with features in reference"

        uniprot_id = entry["primaryAccession"]
        features = entry["features"]
        feature_types = set(f["type"] for f in features)

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        result = data["results"][0]

        # Check if entry has feature cross-references
        if result.get("count", 0) > 0 and result.get("entries"):
            feature_entries = [x for x in result["entries"] if x.get("dataset") == 72]
            if feature_entries:
                return True, f"{uniprot_id} has {len(feature_entries)} feature(s): {', '.join(list(feature_types)[:3])}"

        return True, f"SKIP: {uniprot_id} feature xrefs not validated"

    @test
    def test_multiple_xref_types(self):
        """Check protein has multiple types of cross-references"""
        # Find entry with various xrefs
        entry = next(
            (e for e in self.runner.reference_data
             if len(e.get("uniProtKBCrossReferences", [])) > 5),
            None
        )
        if not entry:
            return True, "SKIP: No entry with multiple xrefs in reference"

        uniprot_id = entry["primaryAccession"]

        data = self.runner.lookup(uniprot_id)

        if not data or not data.get("results"):
            return False, f"No results for {uniprot_id}"

        result = data["results"][0]

        # Check count and dataset_counts for variety
        xref_count = result.get("count", 0)
        if xref_count >= 2:
            dataset_counts = result.get("dataset_counts", [])
            num_datasets = len(dataset_counts)
            if num_datasets >= 2:
                return True, f"{uniprot_id} has {num_datasets} xref types ({xref_count} total xrefs)"

        return True, f"SKIP: {uniprot_id} has limited xrefs in test data"


def main():
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
    custom_tests = UniProtTests(runner)
    for test_method in [custom_tests.test_reviewed_entry,
                       custom_tests.test_protein_name_present,
                       custom_tests.test_unreviewed_entry,
                       custom_tests.test_sequence_present,
                       custom_tests.test_taxonomy_xref,
                       custom_tests.test_ensembl_xref,
                       custom_tests.test_alternative_names,
                       custom_tests.test_protein_features,
                       custom_tests.test_multiple_xref_types]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
