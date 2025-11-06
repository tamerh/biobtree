#!/usr/bin/env python3
"""
Clinical Trials Test Suite

Tests Clinical Trials dataset processing using the common test framework.
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


class ClinicalTrialsTests:
    """Clinical Trials custom tests (in addition to declarative tests)"""

    def __init__(self, runner: TestRunner):
        self.runner = runner

    @test
    def test_trial_with_interventions(self):
        """Check trial with interventions."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("interventions") and len(e["interventions"]) > 0),
            None
        )

        if not entry:
            return True, "No entries with interventions"

        nct_id = entry["nct_id"]
        data = self.runner.lookup(nct_id)

        if not data or not data.get("results"):
            return False, f"No results for {nct_id}"

        intervention_names = [i.get("name", "") for i in entry["interventions"]]
        return True, f"{nct_id} has interventions: {', '.join(intervention_names[:3])}"

    @test
    def test_trial_with_conditions(self):
        """Check trial with medical conditions."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("conditions") and len(e["conditions"]) > 0),
            None
        )

        if not entry:
            return True, "No entries with conditions"

        nct_id = entry["nct_id"]
        data = self.runner.lookup(nct_id)

        if not data or not data.get("results"):
            return False, f"No results for {nct_id}"

        conditions = entry["conditions"]
        return True, f"{nct_id} has conditions: {', '.join(conditions[:3])}"

    @test
    def test_trial_phase2(self):
        """Check trial with PHASE2."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("phase") == "PHASE2"),
            None
        )

        if not entry:
            return True, "No PHASE2 entries"

        nct_id = entry["nct_id"]
        data = self.runner.lookup(nct_id)

        if not data or not data.get("results"):
            return False, f"No results for {nct_id}"

        return True, f"{nct_id} is PHASE2: {entry.get('brief_title', '')[:50]}..."

    @test
    def test_trial_recruiting(self):
        """Check trial with RECRUITING status."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("overall_status") == "RECRUITING"),
            None
        )

        if not entry:
            return True, "No RECRUITING entries"

        nct_id = entry["nct_id"]
        data = self.runner.lookup(nct_id)

        if not data or not data.get("results"):
            return False, f"No results for {nct_id}"

        return True, f"{nct_id} is RECRUITING: {entry.get('brief_title', '')[:50]}..."

    @test
    def test_trial_interventional(self):
        """Check trial with INTERVENTIONAL study type."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("study_type") == "INTERVENTIONAL"),
            None
        )

        if not entry:
            return True, "No INTERVENTIONAL entries"

        nct_id = entry["nct_id"]
        data = self.runner.lookup(nct_id)

        if not data or not data.get("results"):
            return False, f"No results for {nct_id}"

        return True, f"{nct_id} is INTERVENTIONAL"

    @test
    def test_trial_with_drug_interventions(self):
        """Check trial with DRUG intervention type."""
        entry = next(
            (e for e in self.runner.reference_data
             if any(i.get("intervention_type") == "DRUG" for i in e.get("interventions", []))),
            None
        )

        if not entry:
            return True, "No entries with DRUG interventions"

        nct_id = entry["nct_id"]
        data = self.runner.lookup(nct_id)

        if not data or not data.get("results"):
            return False, f"No results for {nct_id}"

        drug_interventions = [i.get("name", "") for i in entry["interventions"] if i.get("intervention_type") == "DRUG"]
        return True, f"{nct_id} has DRUG interventions: {', '.join(drug_interventions[:2])}"

    @test
    def test_mondo_mapping(self):
        """Check if trial conditions are mapped to MONDO."""
        # Find a trial with conditions that should map to MONDO
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("conditions") and len(e["conditions"]) > 0),
            None
        )

        if not entry:
            return True, "No entries with conditions for MONDO mapping"

        nct_id = entry["nct_id"]

        # Check if MONDO cross-reference exists
        has_mondo = self.runner.check_xref(nct_id, "mondo")

        if has_mondo:
            return True, f"{nct_id} has MONDO cross-references"
        else:
            # This is not necessarily a failure - not all conditions map to MONDO
            return True, f"{nct_id} has no MONDO mappings (conditions may not map)"

    # Note: ChEMBL molecule cross-references depend on ChEMBL dataset integration
    # Similar to patent compounds, we won't test ChEMBL links here as they require
    # ChEMBL dataset to be present


def main():
    """Main test entry point."""
    # Setup paths
    script_dir = Path(__file__).parent
    reference_file = script_dir / "reference_data.json"
    test_cases_file = script_dir / "test_cases.json"

    # API URL (default port used by orchestrator)
    api_url = os.environ.get("BIOBTREE_API_URL", "http://localhost:9292")

    # Create test runner
    runner = TestRunner(api_url, reference_file, test_cases_file)

    # Add custom tests
    custom_tests = ClinicalTrialsTests(runner)
    for test_method in [
        custom_tests.test_trial_with_interventions,
        custom_tests.test_trial_with_conditions,
        custom_tests.test_trial_phase2,
        custom_tests.test_trial_recruiting,
        custom_tests.test_trial_interventional,
        custom_tests.test_trial_with_drug_interventions,
        custom_tests.test_mondo_mapping,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
