#!/usr/bin/env python3
"""
DrugCentral Test Suite

Tests the `drugcentral` dataset (approved/marketed drugs):
- drug record carries name/INN/InChIKey + approval booleans + target_count
- drug -> uniprot (target) edge resolves
- drug -> uniprot includes the curated MOA target
- text search by drug name resolves to the struct_id

Requires a running server on localhost:9292 built with `drugcentral`.
"""

import sys
import os
import requests
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent))

from common import TestRunner, test, discover_tests


# amlodipine: a well-known FDA-approved calcium channel blocker.
# struct_id 183; MOA targets are the L-type Ca channel alpha-1 subunits
# CACNA1C (Q13936) / CACNA1D (Q01668).
AMLODIPINE = "183"
AMLODIPINE_MOA_TARGET = "Q13936"


class DrugcentralTests:
    """DrugCentral custom tests."""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    def _map(self, source_id: str, chain: str):
        url = f"{self.runner.api_url}/ws/map/?i={source_id}&m={chain}"
        resp = requests.get(url, timeout=30)
        if resp.status_code != 200:
            return None
        return resp.json()

    @test
    def test_drug_record_attrs(self):
        """Drug entry carries name, InChIKey, approval flag and target_count."""
        data = self._map(AMLODIPINE, ">>drugcentral")
        if not data or not data.get("results"):
            return False, f"No drugcentral record for {AMLODIPINE}"
        attrs = data["results"][0]["source"].get("Attributes", {}).get("Drugcentral", {})
        if attrs.get("name", "").lower() != "amlodipine":
            return False, f"Unexpected name: {attrs.get('name')}"
        if not attrs.get("inchikey"):
            return False, "Missing InChIKey"
        if not attrs.get("fda_approved"):
            return False, "Expected fda_approved=true for amlodipine"
        if int(attrs.get("target_count", 0)) < 1:
            return False, "Expected at least one target"
        return True, (
            f"amlodipine: targets={attrs.get('target_count')} "
            f"moa={len(attrs.get('moa_targets', []))} fda={attrs.get('fda_approved')}"
        )

    @test
    def test_drug_to_uniprot(self):
        """Drug -> uniprot target edge resolves."""
        data = self._map(AMLODIPINE, ">>drugcentral>>uniprot")
        if not data or not data.get("results"):
            return False, f"No mapping for {AMLODIPINE}"
        targets = data["results"][0].get("targets", [])
        ids = [t["identifier"] for t in targets]
        if not ids:
            return False, "No uniprot targets found"
        return True, f"Found {len(ids)} uniprot targets"

    @test
    def test_drug_to_moa_target(self):
        """Drug -> uniprot includes the curated MOA target (CACNA1C)."""
        data = self._map(AMLODIPINE, ">>drugcentral>>uniprot")
        if not data or not data.get("results"):
            return False, f"No mapping for {AMLODIPINE}"
        ids = [t["identifier"] for t in data["results"][0].get("targets", [])]
        if AMLODIPINE_MOA_TARGET not in ids:
            return False, f"{AMLODIPINE_MOA_TARGET} not among targets {ids[:10]}"
        return True, f"MOA target {AMLODIPINE_MOA_TARGET} present"

    @test
    def test_text_search_name(self):
        """Text search by drug name resolves to the struct_id."""
        url = f"{self.runner.api_url}/ws/search/?i=amlodipine&s=drugcentral"
        resp = requests.get(url, timeout=30)
        if resp.status_code != 200:
            return False, f"HTTP {resp.status_code}"
        data = resp.json()
        results = data.get("results", [])
        ids = [r.get("identifier") for r in results]
        if AMLODIPINE not in ids:
            return False, f"struct_id {AMLODIPINE} not in search results {ids[:10]}"
        return True, "amlodipine search resolves to struct_id 183"


def main():
    """Run DrugCentral tests."""
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    api_url = os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292')

    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--api-url', default=api_url)
    args = parser.parse_args()
    api_url = args.api_url

    if not reference_file.exists():
        reference_file = None

    runner = TestRunner(api_url, reference_file, test_cases_file)

    custom_tests = DrugcentralTests(runner)
    for test_method in discover_tests(custom_tests):
        runner.add_custom_test(test_method)

    runner.run_all_tests()
    exit_code = runner.print_summary()
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
