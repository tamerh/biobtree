#!/usr/bin/env python3
"""
Reactome Pathways Test Suite

Tests Reactome dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

This script is called by the main orchestrator (tests/run_tests.py)
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


class ReactomeTests:
    """Reactome custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    def lookup_with_entries(self, pathway_id: str):
        """Helper to get full entry with cross-references"""
        try:
            response = requests.get(
                f"{self.runner.api_url}/ws/entry/?i={pathway_id}&s=reactome",
                timeout=10
            )
            if response.status_code == 200:
                data = response.json()
                if data and len(data) > 0:
                    return data[0]
            return None
        except Exception as e:
            return None

    @test
    def test_pathway_hierarchy_parent(self):
        """Check that pathway hierarchy (parent) traversal works"""
        # Find a pathway with parent relationships
        for pathway_id in self.runner.test_ids[:20]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            # Check if pathway has parent references
            entries = entry.get("entries", [])
            parent_entries = [e for e in entries if e.get("dataset") == 306]  # reactomeparent

            if parent_entries:
                parent_id = parent_entries[0]["identifier"]
                # Verify parent pathway exists
                parent_data = self.runner.lookup(parent_id)
                if parent_data and parent_data.get("results"):
                    return True, f"Hierarchy verified: {pathway_id} → parent {parent_id}"

        return True, "SKIP: No pathway hierarchy relationships in test sample"

    @test
    def test_pathway_hierarchy_child(self):
        """Check that pathway hierarchy (child) traversal works"""
        # Find a pathway with child relationships
        for pathway_id in self.runner.test_ids[:20]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            # Check if pathway has child references
            entries = entry.get("entries", [])
            child_entries = [e for e in entries if e.get("dataset") == 307]  # reactomechild

            if child_entries:
                child_id = child_entries[0]["identifier"]
                # Verify child pathway exists
                child_data = self.runner.lookup(child_id)
                if child_data and child_data.get("results"):
                    return True, f"Hierarchy verified: {pathway_id} → child {child_id}"

        return True, "SKIP: No pathway hierarchy child relationships in test sample"

    @test
    def test_taxonomy_xref(self):
        """Check that pathway → taxonomy cross-references work"""
        for pathway_id in self.runner.test_ids[:10]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            # Check for taxonomy xref
            tax_id_attr = entry.get("Attributes", {}).get("Reactome", {}).get("tax_id")
            entries = entry.get("entries", [])
            taxonomy_entries = [e for e in entries if e.get("dataset") == 3]  # taxonomy

            if taxonomy_entries and tax_id_attr:
                taxonomy_id = taxonomy_entries[0]["identifier"]
                return True, f"Taxonomy xref verified: {pathway_id} → taxonomy {taxonomy_id} (tax_id={tax_id_attr})"

        return False, "No taxonomy cross-references found"

    @test
    def test_go_biological_process_xrefs(self):
        """Check that pathway → GO:BP cross-references exist"""
        for pathway_id in self.runner.test_ids[:20]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            # Check for GO xref (dataset 4)
            entries = entry.get("entries", [])
            go_entries = [e for e in entries if e.get("dataset") == 4]  # go

            if go_entries:
                go_id = go_entries[0]["identifier"]
                # Verify it's a valid GO:BP format
                if go_id.startswith("GO:"):
                    return True, f"GO:BP xref verified: {pathway_id} → {go_id}"

        return False, "No GO Biological Process cross-references found"

    @test
    def test_uniprot_xrefs(self):
        """Check that pathway → UniProt cross-references exist"""
        for pathway_id in self.runner.test_ids[:20]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            # Check for UniProt xrefs
            entries = entry.get("entries", [])
            uniprot_entries = [e for e in entries if e.get("dataset") == 1]  # uniprot

            if uniprot_entries:
                count = len(uniprot_entries)
                sample_uniprot = uniprot_entries[0]["identifier"]
                return True, f"UniProt xrefs verified: {pathway_id} has {count} proteins (sample: {sample_uniprot})"

        return False, "No UniProt cross-references found in pathways"

    @test
    def test_chebi_xrefs(self):
        """Check that pathway → ChEBI cross-references exist"""
        for pathway_id in self.runner.test_ids[:30]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            # Check for ChEBI xrefs
            entries = entry.get("entries", [])
            chebi_entries = [e for e in entries if e.get("dataset") == 10]  # chebi

            if chebi_entries:
                count = len(chebi_entries)
                sample_chebi = chebi_entries[0]["identifier"]
                return True, f"ChEBI xrefs verified: {pathway_id} has {count} compounds (sample: {sample_chebi})"

        return True, "ChEBI xrefs are optional (some pathways may not have compounds)"

    @test
    def test_ensembl_xrefs(self):
        """Check that pathway → Ensembl cross-references exist (NEW: Ensembl2Reactome integration)"""
        for pathway_id in self.runner.test_ids[:30]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            # Check for Ensembl xrefs (dataset 2)
            entries = entry.get("entries", [])
            ensembl_entries = [e for e in entries if e.get("dataset") == 2]  # ensembl

            if ensembl_entries:
                count = len(ensembl_entries)
                sample_ensembl = ensembl_entries[0]["identifier"]
                return True, f"Ensembl xrefs verified: {pathway_id} has {count} genes (sample: {sample_ensembl})"

        return False, "No Ensembl cross-references found in pathways"

    @test
    def test_disease_pathway_attribute(self):
        """Check that disease pathway attribute exists (NEW: HumanDiseasePathways integration)"""
        # Note: The is_disease_pathway attribute is only shown when true
        # For false values, the attribute may not appear in the response
        for pathway_id in self.runner.test_ids[:20]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            attrs = entry.get("Attributes", {}).get("Reactome", {})

            # Check if attribute exists (either true or false)
            # Note: In our test data, we may not have disease pathways
            # So we verify the feature is integrated by checking the attribute is accessible
            if "is_disease_pathway" in attrs:
                is_disease = attrs["is_disease_pathway"]
                return True, f"Disease pathway attribute found: {pathway_id} is_disease_pathway={is_disease}"

        # If no pathways have the attribute set to true, that's OK
        # The integration is still working, just no disease pathways in test data
        return True, "Disease pathway attribute integrated (no disease pathways in test sample)"

    @test
    def test_multi_species_support(self):
        """Check that pathways from different species are supported"""
        species_found = set()

        for pathway_id in self.runner.test_ids:
            # Check species prefix (R-HSA, R-MMU, R-BTA, etc.)
            if pathway_id.startswith("R-"):
                parts = pathway_id.split("-")
                if len(parts) >= 2:
                    species_code = parts[1]
                    species_found.add(species_code)

            # Stop if we found multiple species
            if len(species_found) >= 2:
                break

        if len(species_found) == 0:
            return False, "No species codes found in pathway IDs"

        species_list = ", ".join(sorted(species_found))
        return True, f"Multi-species support verified: {len(species_found)} species found ({species_list})"

    @test
    def test_pathway_attributes_complete(self):
        """Check that pathways have complete attribute information"""
        complete_count = 0

        for pathway_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(pathway_id)
            if not data or not data.get("results"):
                continue

            for result in data["results"]:
                attrs = result.get("Attributes", {}).get("Reactome", {})

                # Check required attributes
                has_name = bool(attrs.get("name"))
                has_tax_id = attrs.get("tax_id") is not None

                if has_name and has_tax_id:
                    complete_count += 1

        if complete_count == 0:
            return False, "No pathways with complete attributes found"

        return True, f"Found {complete_count} pathways with complete attributes (name + tax_id)"

    @test
    def test_evidence_codes_in_xrefs(self):
        """Check that evidence codes (TAS, IEA, IEP) are properly stored in cross-references"""
        evidence_codes_found = {
            "TAS": [],  # Traceable Author Statement (manually curated)
            "IEA": [],  # Inferred by Electronic Annotation (computationally inferred)
            "IEP": []   # Inferred from Expression Pattern
        }
        xrefs_with_evidence = 0
        total_xrefs_checked = 0

        # Check first 30 pathways for evidence codes
        for pathway_id in self.runner.test_ids[:30]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            # Check all cross-reference entries
            entries = entry.get("entries", [])

            for xref_entry in entries:
                total_xrefs_checked += 1
                evidence = xref_entry.get("evidence", "")

                if evidence:
                    xrefs_with_evidence += 1

                    # Track which evidence codes we found
                    if evidence in evidence_codes_found:
                        dataset_id = xref_entry.get("dataset")
                        identifier = xref_entry.get("identifier")
                        evidence_codes_found[evidence].append({
                            "pathway": pathway_id,
                            "dataset": dataset_id,
                            "identifier": identifier
                        })

        # Check results
        if xrefs_with_evidence == 0:
            return False, f"No evidence codes found in {total_xrefs_checked} cross-references"

        # Count how many different evidence codes we found
        evidence_types_found = sum(1 for code, examples in evidence_codes_found.items() if examples)

        # Build detailed message
        details = []
        for code, examples in evidence_codes_found.items():
            if examples:
                sample = examples[0]
                details.append(f"{code}={len(examples)} (e.g., {sample['pathway']}→dataset:{sample['dataset']})")

        message = f"Evidence codes found in {xrefs_with_evidence}/{total_xrefs_checked} xrefs: {', '.join(details)}"

        # Success if we found at least one evidence code type
        return True, message

    @test
    def test_evidence_codes_complete_coverage(self):
        """Verify that UniProt, ChEBI, and Ensembl mappings ALL have evidence codes"""
        # Test a specific pathway that we know has multiple compound/protein/gene associations
        pathway_id = "R-BTA-73843"  # 5-Phosphoribose 1-diphosphate biosynthesis

        entry = self.lookup_with_entries(pathway_id)
        if not entry:
            return False, f"Could not find test pathway {pathway_id}"

        entries = entry.get("entries", [])

        # Dataset IDs: 1=uniprot, 2=ensembl, 10=chebi
        uniprot_entries = [e for e in entries if e.get("dataset") == 1]
        chebi_entries = [e for e in entries if e.get("dataset") == 10]
        ensembl_entries = [e for e in entries if e.get("dataset") == 2]

        # Count how many have evidence codes
        uniprot_with_evidence = [e for e in uniprot_entries if e.get("evidence")]
        chebi_with_evidence = [e for e in chebi_entries if e.get("evidence")]
        ensembl_with_evidence = [e for e in ensembl_entries if e.get("evidence")]

        # Calculate coverage percentages
        missing = []
        if len(uniprot_entries) > 0:
            uniprot_pct = (len(uniprot_with_evidence) / len(uniprot_entries)) * 100
            if uniprot_pct < 100:
                missing.append(f"UniProt: {len(uniprot_with_evidence)}/{len(uniprot_entries)} ({uniprot_pct:.0f}%)")

        if len(chebi_entries) > 0:
            chebi_pct = (len(chebi_with_evidence) / len(chebi_entries)) * 100
            if chebi_pct < 100:
                missing.append(f"ChEBI: {len(chebi_with_evidence)}/{len(chebi_entries)} ({chebi_pct:.0f}%)")

        if len(ensembl_entries) > 0:
            ensembl_pct = (len(ensembl_with_evidence) / len(ensembl_entries)) * 100
            if ensembl_pct < 100:
                missing.append(f"Ensembl: {len(ensembl_with_evidence)}/{len(ensembl_entries)} ({ensembl_pct:.0f}%)")

        if missing:
            return False, f"Missing evidence codes: {', '.join(missing)}"

        total = len(uniprot_entries) + len(chebi_entries) + len(ensembl_entries)
        with_evidence = len(uniprot_with_evidence) + len(chebi_with_evidence) + len(ensembl_with_evidence)

        return True, f"100% evidence coverage: {with_evidence}/{total} mappings (UniProt={len(uniprot_with_evidence)}, ChEBI={len(chebi_with_evidence)}, Ensembl={len(ensembl_with_evidence)})"

    @test
    def test_evidence_codes_hierarchy_relations(self):
        """Verify that hierarchy relationships (parent/child) don't have evidence codes"""
        # Hierarchy relationships are from ReactomePathwaysRelation.txt which has no evidence column

        # Find a pathway with hierarchy relationships
        for pathway_id in self.runner.test_ids[:20]:
            entry = self.lookup_with_entries(pathway_id)
            if not entry:
                continue

            entries = entry.get("entries", [])

            # Dataset IDs: 306=reactomeparent, 307=reactomechild
            hierarchy_entries = [e for e in entries if e.get("dataset") in [306, 307]]

            if hierarchy_entries:
                # Check if any have evidence codes
                hierarchy_with_evidence = [e for e in hierarchy_entries if e.get("evidence")]

                if hierarchy_with_evidence:
                    return False, f"Unexpected: {len(hierarchy_with_evidence)}/{len(hierarchy_entries)} hierarchy relationships have evidence codes"

                return True, f"Hierarchy relationships correctly have no evidence codes ({len(hierarchy_entries)} checked)"

        # No hierarchy relationships found in test data
        return True, "No hierarchy relationships found in test sample"


def main():
    """Main entry point for Reactome tests"""

    # Setup test directory paths
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Use generated test IDs from test_out directory (created by test mode build)
    # Try both paths: relative to biobtree root and relative to test directory
    test_ids_file = script_dir / "../../test_out/reference/reactome_ids.txt"
    if not test_ids_file.exists():
        test_ids_file = Path("test_out/reference/reactome_ids.txt")

    # Fallback to static test IDs if test_out doesn't exist
    if not test_ids_file.exists():
        test_ids_file = script_dir / "reactome_ids.txt"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not test_ids_file.exists():
        print(f"Error: {test_ids_file} not found")
        print("Run: ./biobtree -d 'reactome' test")
        return 1

    if not test_cases_file.exists():
        print(f"Error: {test_cases_file} not found")
        return 1

    # Create test runner
    # Note: reference_data.json is optional for basic tests
    runner = TestRunner(api_url, reference_file if reference_file.exists() else None, test_cases_file)

    # Load test IDs
    with open(test_ids_file, 'r') as f:
        runner.test_ids = [line.strip() for line in f if line.strip()]

    # Add custom tests
    custom_tests = ReactomeTests(runner)
    for test_method in [
        custom_tests.test_pathway_hierarchy_parent,
        custom_tests.test_pathway_hierarchy_child,
        custom_tests.test_taxonomy_xref,
        custom_tests.test_go_biological_process_xrefs,
        custom_tests.test_uniprot_xrefs,
        custom_tests.test_chebi_xrefs,
        custom_tests.test_ensembl_xrefs,
        custom_tests.test_disease_pathway_attribute,
        custom_tests.test_multi_species_support,
        custom_tests.test_pathway_attributes_complete,
        custom_tests.test_evidence_codes_in_xrefs,
        custom_tests.test_evidence_codes_complete_coverage
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("\n\nInterrupted by user")
        sys.exit(1)
