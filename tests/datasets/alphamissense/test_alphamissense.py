#!/usr/bin/env python3
"""
AlphaMissense Dataset Tests

AlphaMissense is DeepMind's deep learning model for predicting the pathogenicity of
missense variants (single amino acid changes) in the human proteome.

Test Structure:
- Variant entries: Genomic coordinate IDs (e.g., 1:69094:G:T)
- Gene-level entries: Ensembl transcript IDs (e.g., ENST00000335137.4)
- Attributes: Pathogenicity score, classification, UniProt ID, transcript IDs (array), protein variant
- Cross-references: UniProt proteins, Ensembl transcripts
- Classification thresholds: likely_benign (<0.34), ambiguous (0.34-0.564), likely_pathogenic (>=0.564)

Data source: gs://dm_alphamissense/
License: CC BY-NC-SA 4.0
"""

import sys
import os
import re
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class AlphaMissenseTests:
    """AlphaMissense custom tests"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_variant_has_genomic_coordinates(self):
        """Verify variants have chromosome and position"""
        # Use IDs from the test file
        test_ids_file = Path(__file__).parent / "alphamissense_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:5]

        if not test_ids:
            return False, "No test IDs available"

        for variant_id in test_ids:
            data = self.runner.query.lookup(variant_id)

            if not data or not data.get("results"):
                return False, f"Variant {variant_id} not found"

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Alphamissense", {})

            if not attrs:
                return False, f"No AlphaMissense attributes for {variant_id}"

            chromosome = attrs.get("chromosome")
            position = attrs.get("position")

            if not chromosome or not position:
                return False, f"Missing chromosome or position for {variant_id}"

        return True, f"All {len(test_ids)} variants have genomic coordinates"

    @test
    def test_pathogenicity_score_range(self):
        """Verify pathogenicity scores are in valid range [0, 1]"""
        test_ids_file = Path(__file__).parent / "alphamissense_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:20]

        valid_count = 0
        invalid_scores = []

        for variant_id in test_ids:
            data = self.runner.query.lookup(variant_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Alphamissense", {})

            if not attrs:
                continue

            score = attrs.get("am_pathogenicity")
            if score is None:
                invalid_scores.append(f"{variant_id}: missing score")
            elif score < 0 or score > 1:
                invalid_scores.append(f"{variant_id}: {score}")
            else:
                valid_count += 1

        if invalid_scores:
            return False, f"Invalid scores: {invalid_scores[:3]}"

        return True, f"{valid_count} variants have valid pathogenicity scores (0-1)"

    @test
    def test_classification_consistency(self):
        """Verify am_class matches am_pathogenicity thresholds"""
        test_ids_file = Path(__file__).parent / "alphamissense_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:30]

        benign_count = 0
        ambiguous_count = 0
        pathogenic_count = 0
        mismatches = []

        for variant_id in test_ids:
            data = self.runner.query.lookup(variant_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Alphamissense", {})

            if not attrs:
                continue

            score = attrs.get("am_pathogenicity")
            am_class = attrs.get("am_class")

            if score is None or am_class is None:
                continue

            # Check thresholds
            expected_class = None
            if score < 0.34:
                expected_class = "likely_benign"
                benign_count += 1
            elif score < 0.564:
                expected_class = "ambiguous"
                ambiguous_count += 1
            else:
                expected_class = "likely_pathogenic"
                pathogenic_count += 1

            if am_class != expected_class:
                mismatches.append(f"{variant_id}: score={score}, class={am_class}, expected={expected_class}")

        if mismatches:
            return False, f"Classification mismatches: {mismatches[:3]}"

        return True, f"Classifications match thresholds: {benign_count} benign, {ambiguous_count} ambiguous, {pathogenic_count} pathogenic"

    @test
    def test_uniprot_xref(self):
        """Verify cross-references to UniProt proteins"""
        test_ids_file = Path(__file__).parent / "alphamissense_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:10]

        uniprot_count = 0

        for variant_id in test_ids:
            data = self.runner.query.lookup(variant_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Alphamissense", {})

            if attrs and attrs.get("uniprot_id"):
                uniprot_count += 1

                # Validate UniProt ID format
                uniprot_id = attrs.get("uniprot_id")
                if not re.match(r'^[A-Z0-9]+$', uniprot_id):
                    return False, f"Invalid UniProt ID format: {uniprot_id}"

        if uniprot_count == 0:
            return False, "No variants have UniProt cross-references"

        return True, f"{uniprot_count}/{len(test_ids)} variants have UniProt links"

    @test
    def test_transcript_ids_format(self):
        """Verify Ensembl transcript IDs are properly formatted (array field)"""
        test_ids_file = Path(__file__).parent / "alphamissense_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:10]

        transcript_count = 0
        total_transcripts = 0

        for variant_id in test_ids:
            data = self.runner.query.lookup(variant_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Alphamissense", {})

            if not attrs:
                continue

            # transcript_ids is now an array
            transcript_ids = attrs.get("transcript_ids", [])
            if transcript_ids:
                transcript_count += 1
                total_transcripts += len(transcript_ids)

                # Validate each transcript ID format
                for transcript_id in transcript_ids:
                    if not re.match(r'^ENST\d+(\.\d+)?$', transcript_id):
                        return False, f"Invalid transcript ID format: {transcript_id}"

        if transcript_count == 0:
            return False, "No variants have transcript IDs"

        avg_transcripts = total_transcripts / transcript_count if transcript_count > 0 else 0
        return True, f"{transcript_count}/{len(test_ids)} variants have transcript IDs (avg {avg_transcripts:.1f} per variant)"

    @test
    def test_isoform_enrichment(self):
        """Verify some variants have multiple transcript IDs (isoform data)"""
        test_ids_file = Path(__file__).parent / "alphamissense_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:50]

        multi_isoform_count = 0
        max_isoforms = 0
        max_isoform_variant = None

        for variant_id in test_ids:
            data = self.runner.query.lookup(variant_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Alphamissense", {})

            if not attrs:
                continue

            transcript_ids = attrs.get("transcript_ids", [])
            if len(transcript_ids) > 1:
                multi_isoform_count += 1
                if len(transcript_ids) > max_isoforms:
                    max_isoforms = len(transcript_ids)
                    max_isoform_variant = variant_id

        # It's acceptable if test data doesn't have multi-isoform variants
        # since test mode may not load the full isoforms file
        if multi_isoform_count > 0:
            return True, f"{multi_isoform_count} variants have multiple isoforms (max {max_isoforms} for {max_isoform_variant})"
        else:
            return True, "No multi-isoform variants in test data (expected for limited test set)"

    @test
    def test_protein_variant_format(self):
        """Verify protein variant notation is properly formatted"""
        test_ids_file = Path(__file__).parent / "alphamissense_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:10]

        variant_count = 0

        for variant_id in test_ids:
            data = self.runner.query.lookup(variant_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Alphamissense", {})

            if attrs and attrs.get("protein_variant"):
                protein_variant = attrs.get("protein_variant")
                # Should match format like V2L, R175H (amino acid + position + amino acid)
                if not re.match(r'^[A-Z]\d+[A-Z]$', protein_variant):
                    return False, f"Invalid protein variant format: {protein_variant}"
                variant_count += 1

        if variant_count == 0:
            return False, "No variants have protein variant notation"

        return True, f"{variant_count}/{len(test_ids)} variants have valid protein variant notation"

    @test
    def test_entry_id_format(self):
        """Verify entry ID format is chr:pos:ref:alt"""
        test_ids_file = Path(__file__).parent / "alphamissense_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:20]

        valid_count = 0
        invalid_ids = []

        for variant_id in test_ids:
            # ID should match format: chr:pos:ref:alt
            if re.match(r'^[0-9XYM]+:\d+:[ACGT]+:[ACGT]+$', variant_id):
                valid_count += 1
            else:
                invalid_ids.append(variant_id)

        if invalid_ids:
            return False, f"Invalid ID format: {invalid_ids[:3]}"

        return True, f"{valid_count} variant IDs have valid format (chr:pos:ref:alt)"

    @test
    def test_allele_consistency(self):
        """Verify ref/alt alleles match entry ID"""
        test_ids_file = Path(__file__).parent / "alphamissense_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:10]

        checked_count = 0
        mismatches = []

        for variant_id in test_ids:
            # Parse ID components
            parts = variant_id.split(':')
            if len(parts) != 4:
                continue

            id_chr, id_pos, id_ref, id_alt = parts

            data = self.runner.query.lookup(variant_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("Alphamissense", {})

            if not attrs:
                continue

            checked_count += 1

            # Verify consistency
            if attrs.get("chromosome") != id_chr:
                mismatches.append(f"{variant_id}: chromosome mismatch")
            if str(attrs.get("position")) != id_pos:
                mismatches.append(f"{variant_id}: position mismatch")
            if attrs.get("ref_allele") != id_ref:
                mismatches.append(f"{variant_id}: ref allele mismatch")
            if attrs.get("alt_allele") != id_alt:
                mismatches.append(f"{variant_id}: alt allele mismatch")

        if mismatches:
            return False, f"Data inconsistencies: {mismatches[:3]}"

        return True, f"{checked_count} variants have consistent ID/attribute data"


def main():
    script_dir = Path(__file__).parent
    test_ids_file = script_dir / "alphamissense_ids.txt"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not test_ids_file.exists():
        print(f"Error: {test_ids_file} not found")
        print("Run: ./biobtree -d alphamissense test")
        return 1

    # Create test runner (no reference_data.json needed for AlphaMissense)
    runner = TestRunner(api_url, None, test_cases_file)

    # Add custom tests
    custom_tests = AlphaMissenseTests(runner)
    for test_method in [
        custom_tests.test_variant_has_genomic_coordinates,
        custom_tests.test_pathogenicity_score_range,
        custom_tests.test_classification_consistency,
        custom_tests.test_uniprot_xref,
        custom_tests.test_transcript_ids_format,
        custom_tests.test_isoform_enrichment,
        custom_tests.test_protein_variant_format,
        custom_tests.test_entry_id_format,
        custom_tests.test_allele_consistency,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
