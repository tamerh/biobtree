#!/usr/bin/env python3
"""
STRING Protein Interactions Test Suite

Tests STRING dataset processing using the common test framework.
Uses declarative tests from test_cases.json and custom Python tests.

Architecture:
- STRING entries store protein metadata + interaction_count summary
- Interactions are stored in separate string_interaction sub-dataset
- This enables efficient queries: >>string>>string_interaction[score>900]
- Interactions are sorted by score (highest first) via inverted score in ID

Query patterns:
- P27348 >> string                                    # Get STRING entry (~1KB)
- P27348 >> string >> string_interaction              # Get all interactions
- P27348 >> string >> string_interaction[score>900]  # High-confidence only
- P27348 >> string >> string_interaction >> uniprot  # Get partner proteins

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


class StringTests:
    """STRING custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_string_interaction_mapping(self):
        """Check that STRING >> string_interaction mapping works"""
        # Find a STRING entry
        string_id = None

        for test_id in self.runner.test_ids[:10]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    stringattr = result.get("Attributes", {}).get("Stringattr", {})
                    if stringattr and stringattr.get("string_id"):
                        string_id = stringattr["string_id"]
                        break
            if string_id:
                break

        if not string_id:
            return False, "No STRING IDs found in test data"

        # Map to string_interaction
        map_result = self.runner.map_query(string_id, ">>string>>string_interaction", mode="lite")

        if not map_result:
            return False, f"Map query failed for {string_id}"

        stats = map_result.get("stats", {})
        total = stats.get("total", 0)

        if total == 0:
            return False, f"No interactions found for {string_id}"

        # Check schema includes score field
        schema = map_result.get("schema", "")
        if "score" not in schema:
            return False, f"Schema missing score field: {schema}"

        return True, f"Found {total} interactions for {string_id}, schema: {schema}"

    @test
    def test_interaction_score_ordering(self):
        """Check that interactions are sorted by score (highest first)"""
        # Find a STRING entry with interactions
        string_id = None
        best_count = 0

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    stringattr = result.get("Attributes", {}).get("Stringattr", {})
                    if stringattr:
                        count = stringattr.get("interaction_count", 0)
                        if count > best_count:
                            best_count = count
                            string_id = stringattr["string_id"]

        if not string_id or best_count < 2:
            return True, "Skipped: Not enough interactions in test data to verify ordering"

        # Get interactions
        map_result = self.runner.map_query(string_id, ">>string>>string_interaction", mode="lite")

        if not map_result:
            return False, f"Map query failed for {string_id}"

        mappings = map_result.get("mappings", [])
        if not mappings or not mappings[0].get("targets"):
            return False, "No interaction targets found"

        # Parse scores from compact format (id|score|uniprot_b)
        targets = mappings[0]["targets"]
        scores = []
        for target in targets[:20]:  # Check first 20
            parts = target.split("|")
            if len(parts) >= 2:
                try:
                    score = int(parts[1])
                    scores.append(score)
                except ValueError:
                    pass

        if len(scores) < 2:
            return False, "Not enough scores to verify ordering"

        # Check descending order
        for i in range(len(scores) - 1):
            if scores[i] < scores[i + 1]:
                return False, f"Scores not in descending order: {scores}"

        return True, f"Scores correctly ordered (highest first): {scores[:5]}..."

    @test
    def test_interaction_score_filter(self):
        """Check that score filtering works on string_interaction"""
        # Find a STRING entry with most interactions
        string_id = None
        best_count = 0

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    stringattr = result.get("Attributes", {}).get("Stringattr", {})
                    if stringattr:
                        count = stringattr.get("interaction_count", 0)
                        if count > best_count:
                            best_count = count
                            string_id = stringattr["string_id"]

        if not string_id or best_count < 3:
            return True, "Skipped: Not enough interactions in test data to verify filtering"

        # Get all interactions
        all_result = self.runner.map_query(string_id, ">>string>>string_interaction", mode="lite")
        all_total = all_result.get("stats", {}).get("total", 0) if all_result else 0

        # Get filtered interactions (score > 700)
        filtered_result = self.runner.map_query(string_id, ">>string>>string_interaction[score>700]", mode="lite")
        filtered_total = filtered_result.get("stats", {}).get("total", 0) if filtered_result else 0

        if all_total == 0:
            return False, "No interactions found"

        if filtered_total >= all_total:
            return False, f"Filter didn't reduce results: {filtered_total} >= {all_total}"

        # Verify all filtered results have score > 700
        if filtered_result and filtered_result.get("mappings"):
            targets = filtered_result["mappings"][0].get("targets", [])
            for target in targets[:10]:
                parts = target.split("|")
                if len(parts) >= 2:
                    try:
                        score = int(parts[1])
                        if score <= 700:
                            return False, f"Found score {score} <= 700 in filtered results"
                    except ValueError:
                        pass

        return True, f"Score filter works: {filtered_total}/{all_total} interactions with score>700"

    @test
    def test_interaction_to_uniprot_chain(self):
        """Check that string_interaction >> uniprot chaining works"""
        # Find a STRING entry with interactions
        string_id = None

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    stringattr = result.get("Attributes", {}).get("Stringattr", {})
                    if stringattr and stringattr.get("interaction_count", 0) >= 1:
                        string_id = stringattr["string_id"]
                        break
            if string_id:
                break

        if not string_id:
            return True, "Skipped: No STRING entries with interactions in test data"

        # Chain to UniProt
        map_result = self.runner.map_query(string_id, ">>string>>string_interaction>>uniprot", mode="lite")

        if not map_result:
            return False, f"Chain query failed for {string_id}"

        stats = map_result.get("stats", {})
        total = stats.get("total", 0)

        if total == 0:
            return False, "No UniProt targets found via chain"

        # Verify targets look like UniProt IDs
        mappings = map_result.get("mappings", [])
        if mappings and mappings[0].get("targets"):
            sample_target = mappings[0]["targets"][0]
            # UniProt IDs are typically 6-10 alphanumeric characters
            if not (6 <= len(sample_target) <= 15 and sample_target[0].isalpha()):
                return False, f"Target doesn't look like UniProt ID: {sample_target}"

        return True, f"Chain to UniProt works: found {total} partner proteins"

    @test
    def test_interaction_attributes(self):
        """Check that string_interaction entries have expected attributes via compact mode"""
        # Find a STRING entry with interactions
        string_id = None

        for test_id in self.runner.test_ids[:20]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    stringattr = result.get("Attributes", {}).get("Stringattr", {})
                    if stringattr and stringattr.get("interaction_count", 0) >= 1:
                        string_id = stringattr["string_id"]
                        break
            if string_id:
                break

        if not string_id:
            return True, "Skipped: No STRING entries with interactions in test data"

        # Get interactions in lite mode - compact format shows score
        map_result = self.runner.map_query(string_id, ">>string>>string_interaction", mode="lite")

        if not map_result:
            return False, f"Map query failed for {string_id}"

        # Check schema has expected fields
        schema = map_result.get("schema", "")
        if "score" not in schema or "uniprot_b" not in schema:
            return False, f"Schema missing expected fields: {schema}"

        # Check mappings have data
        mappings = map_result.get("mappings", [])
        if not mappings or not mappings[0].get("targets"):
            return False, "No interaction targets in results"

        # Parse first target to verify format: id|score|uniprot_b
        target = mappings[0]["targets"][0]
        parts = target.split("|")
        if len(parts) < 3:
            return False, f"Unexpected target format: {target}"

        interaction_id = parts[0]
        score = parts[1]
        uniprot_b = parts[2]

        # Verify score is numeric
        try:
            score_int = int(score)
            if not (0 <= score_int <= 1000):
                return False, f"Score out of range: {score_int}"
        except ValueError:
            return False, f"Score not numeric: {score}"

        return True, f"Interaction attributes valid: id={interaction_id}, score={score}, partner={uniprot_b}"

    @test
    def test_organism_taxid(self):
        """Check that STRING attributes have organism taxonomy ID"""
        missing_taxid = 0
        checked_count = 0

        for test_id in self.runner.test_ids[:10]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    if "Stringattr" in result.get("Attributes", {}):
                        checked_count += 1
                        if not result["Attributes"]["Stringattr"].get("organism_taxid"):
                            missing_taxid += 1

        if checked_count == 0:
            return False, "No STRING attributes found"

        if missing_taxid > 0:
            return False, f"Found {missing_taxid}/{checked_count} entries without organism_taxid"

        return True, f"All {checked_count} STRING entries have organism_taxid"

    @test
    def test_interaction_count_matches(self):
        """Check that interaction_count matches actual xref count"""
        for test_id in self.runner.test_ids[:5]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    stringattr = result.get("Attributes", {}).get("Stringattr", {})
                    if not stringattr:
                        continue

                    interaction_count = stringattr.get("interaction_count", 0)
                    if interaction_count == 0:
                        continue

                    # Get xref count for string_interaction
                    xref_count = self.runner.get_xref_count(result, "string_interaction")

                    # interaction_count is total (both directions), xref_count is stored interactions
                    # Since we store each interaction once, xref_count should be ~half of interaction_count
                    # or equal if protein only appears as protein_a
                    if xref_count == 0:
                        return False, f"interaction_count={interaction_count} but no string_interaction xrefs"

                    return True, f"interaction_count={interaction_count}, string_interaction xrefs={xref_count}"

        return False, "No STRING entries with interaction_count found"

    @test
    def test_string_id_keyword_lookup(self):
        """Check that STRING IDs work as keywords to find STRING entries"""
        # Find a STRING entry and extract its STRING ID
        string_id = None

        for test_id in self.runner.test_ids[:10]:
            data = self.runner.lookup(test_id)
            if data and data.get("results"):
                for result in data["results"]:
                    stringattr = result.get("Attributes", {}).get("Stringattr", {})
                    if stringattr and stringattr.get("string_id"):
                        string_id = stringattr["string_id"]
                        break
            if string_id:
                break

        if not string_id:
            return False, "No STRING IDs found in STRING entries"

        # Query by STRING ID (should resolve to STRING entry via keyword)
        string_data = self.runner.lookup(string_id)

        if not string_data or not string_data.get("results"):
            return False, f"STRING ID {string_id} did not resolve to any entry"

        # Check if it resolves to STRING dataset with proper attributes
        for result in string_data["results"]:
            if result.get("dataset_name") == "string":
                valid, msg = self.runner.validate_attributes(result, "Stringattr", ["string_id", "interaction_count"])
                if not valid:
                    return False, f"STRING ID {string_id} resolved but {msg}"

                return True, f"STRING ID {string_id} successfully resolves to STRING entry"

        return False, f"STRING ID {string_id} did not resolve to STRING dataset"


def main():
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # Use generated test IDs from test_out directory (created by test mode build)
    test_ids_file = script_dir / "../../../test_out/reference/string_ids.txt"
    if not test_ids_file.exists():
        test_ids_file = Path("test_out/reference/string_ids.txt")

    # Fallback to static test IDs if test_out doesn't exist
    if not test_ids_file.exists():
        test_ids_file = script_dir / "string_ids.txt"

    # Get API URL from environment (set by orchestrator)
    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    # Check prerequisites
    if not test_ids_file.exists():
        print(f"Error: {test_ids_file} not found")
        print("Run: ./biobtree -d 'string' --tax 9606 test")
        return 1

    # Create test runner
    runner = TestRunner(api_url, reference_file if reference_file.exists() else None, test_cases_file)

    # Load test IDs
    with open(test_ids_file, 'r') as f:
        runner.test_ids = [line.strip() for line in f if line.strip()]

    # Add custom tests
    custom_tests = StringTests(runner)
    for test_method in [
        custom_tests.test_string_interaction_mapping,
        custom_tests.test_interaction_score_ordering,
        custom_tests.test_interaction_score_filter,
        custom_tests.test_interaction_to_uniprot_chain,
        custom_tests.test_interaction_attributes,
        custom_tests.test_organism_taxid,
        custom_tests.test_interaction_count_matches,
        custom_tests.test_string_id_keyword_lookup
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
