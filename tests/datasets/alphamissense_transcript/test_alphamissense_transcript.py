#!/usr/bin/env python3
"""
AlphaMissense Transcript Dataset Tests

AlphaMissense Transcript provides transcript-level pathogenicity summaries from
DeepMind's AlphaMissense predictions. Each entry represents an Ensembl transcript
with its mean pathogenicity score across all possible missense variants.

Test Structure:
- Transcript entries: Ensembl transcript IDs with version (e.g., ENST00000335137.4)
- Attributes: transcript_id, mean_am_pathogenicity
- Cross-references: Ensembl transcripts
- Related dataset: alphamissense (variant-level)

Data source: gs://dm_alphamissense/AlphaMissense_gene_hg38.tsv.gz
License: CC BY-NC-SA 4.0
"""

import sys
import os
import re
from pathlib import Path

# Add common test framework to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test


class AlphaMissenseTranscriptTests:
    """AlphaMissense Transcript custom tests"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_transcript_id_format(self):
        """Verify transcript IDs follow ENST pattern with version"""
        test_ids_file = Path(__file__).parent / "alphamissense_transcript_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_transcript_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:20]

        if not test_ids:
            return False, "No test IDs available"

        valid_count = 0
        invalid_ids = []

        for transcript_id in test_ids:
            # ID should match format: ENST followed by digits, dot, version
            if re.match(r'^ENST\d+\.\d+$', transcript_id):
                valid_count += 1
            else:
                invalid_ids.append(transcript_id)

        if invalid_ids:
            return False, f"Invalid transcript ID format: {invalid_ids[:3]}"

        return True, f"{valid_count} transcript IDs have valid format (ENST*.version)"

    @test
    def test_mean_pathogenicity_range(self):
        """Verify mean pathogenicity scores are in valid range [0, 1]"""
        test_ids_file = Path(__file__).parent / "alphamissense_transcript_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_transcript_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:20]

        valid_count = 0
        invalid_scores = []

        for transcript_id in test_ids:
            data = self.runner.query.lookup(transcript_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("AlphamissenseTranscript", {})

            if not attrs:
                continue

            score = attrs.get("mean_am_pathogenicity")
            if score is None:
                invalid_scores.append(f"{transcript_id}: missing score")
            elif score < 0 or score > 1:
                invalid_scores.append(f"{transcript_id}: {score}")
            else:
                valid_count += 1

        if invalid_scores:
            return False, f"Invalid scores: {invalid_scores[:3]}"

        return True, f"{valid_count} transcripts have valid mean pathogenicity scores (0-1)"

    @test
    def test_transcript_has_attributes(self):
        """Verify transcripts have all required attributes"""
        test_ids_file = Path(__file__).parent / "alphamissense_transcript_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_transcript_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:10]

        if not test_ids:
            return False, "No test IDs available"

        valid_count = 0
        missing_attrs = []

        for transcript_id in test_ids:
            data = self.runner.query.lookup(transcript_id)

            if not data or not data.get("results"):
                missing_attrs.append(f"{transcript_id}: not found")
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("AlphamissenseTranscript", {})

            if not attrs:
                missing_attrs.append(f"{transcript_id}: no attributes")
                continue

            # Check required fields
            if not attrs.get("transcript_id"):
                missing_attrs.append(f"{transcript_id}: missing transcript_id")
            elif attrs.get("mean_am_pathogenicity") is None:
                missing_attrs.append(f"{transcript_id}: missing mean_am_pathogenicity")
            else:
                valid_count += 1

        if missing_attrs and valid_count == 0:
            return False, f"Missing attributes: {missing_attrs[:3]}"

        return True, f"{valid_count}/{len(test_ids)} transcripts have all required attributes"

    @test
    def test_attribute_consistency(self):
        """Verify transcript_id attribute matches entry ID"""
        test_ids_file = Path(__file__).parent / "alphamissense_transcript_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_transcript_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:10]

        checked_count = 0
        mismatches = []

        for transcript_id in test_ids:
            data = self.runner.query.lookup(transcript_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("AlphamissenseTranscript", {})

            if not attrs:
                continue

            checked_count += 1
            attr_transcript_id = attrs.get("transcript_id")

            if attr_transcript_id != transcript_id:
                mismatches.append(f"{transcript_id}: attr={attr_transcript_id}")

        if mismatches:
            return False, f"ID mismatches: {mismatches[:3]}"

        return True, f"{checked_count} transcripts have consistent IDs"

    @test
    def test_pathogenicity_distribution(self):
        """Check distribution of mean pathogenicity scores"""
        test_ids_file = Path(__file__).parent / "alphamissense_transcript_ids.txt"
        if not test_ids_file.exists():
            return False, "alphamissense_transcript_ids.txt not found"

        with open(test_ids_file) as f:
            test_ids = [line.strip() for line in f if line.strip()][:100]

        scores = []
        for transcript_id in test_ids:
            data = self.runner.query.lookup(transcript_id)

            if not data or not data.get("results"):
                continue

            result = data["results"][0]
            attrs = result.get("Attributes", {}).get("AlphamissenseTranscript", {})

            if attrs and attrs.get("mean_am_pathogenicity") is not None:
                scores.append(attrs.get("mean_am_pathogenicity"))

        if not scores:
            return False, "No scores found"

        avg_score = sum(scores) / len(scores)
        min_score = min(scores)
        max_score = max(scores)

        return True, f"Scores: avg={avg_score:.3f}, min={min_score:.3f}, max={max_score:.3f} (n={len(scores)})"


def main():
    script_dir = Path(__file__).parent
    test_ids_file = script_dir / "alphamissense_transcript_ids.txt"
    test_cases_file = script_dir / "test_cases.json"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not test_ids_file.exists():
        print(f"Error: {test_ids_file} not found")
        print("Run: ./biobtree -d alphamissense,alphamissense_transcript test")
        return 1

    # Create test runner
    runner = TestRunner(api_url, None, test_cases_file)

    # Add custom tests
    custom_tests = AlphaMissenseTranscriptTests(runner)
    for test_method in [
        custom_tests.test_transcript_id_format,
        custom_tests.test_mean_pathogenicity_range,
        custom_tests.test_transcript_has_attributes,
        custom_tests.test_attribute_consistency,
        custom_tests.test_pathogenicity_distribution,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
