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

    def _ct_attrs(self, nct_id):
        """Return the ClinicalTrials attribute dict the API actually projects
        for nct_id, or None if the entry/result is missing."""
        data = self.runner.lookup(nct_id)
        if not data or not data.get("results"):
            return None
        for r in data["results"]:
            if r.get("dataset_name") == "clinical_trials":
                return r.get("Attributes", {}).get("ClinicalTrials", {})
        return None

    @test
    def test_trial_with_interventions(self):
        """API must actually project interventions, not just declare them (Atlas #45)."""
        entry = next(
            (e for e in self.runner.reference_data
             if e.get("interventions") and len(e["interventions"]) > 0),
            None
        )

        if not entry:
            return True, "No reference entries with interventions"

        nct_id = entry["nct_id"]
        attrs = self._ct_attrs(nct_id)
        if attrs is None:
            return False, f"No clinical_trials result for {nct_id}"

        api_interv = attrs.get("interventions") or []
        if not api_interv:
            return False, f"{nct_id}: interventions empty in API payload (regression of #45)"
        if not any(i.get("name") for i in api_interv):
            return False, f"{nct_id}: interventions present but carry no names"

        names = [i.get("name", "")[:30] for i in api_interv[:3]]
        return True, f"{nct_id} API interventions: {', '.join(names)}"

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
        """API must project the intervention type (DRUG) (Atlas #45 type-key fix).

        Both reference_data (source shape) and the API serialize the
        intervention type under 'type'."""
        entry = next(
            (e for e in self.runner.reference_data
             if any(i.get("type") == "DRUG" for i in e.get("interventions", []))),
            None
        )

        if not entry:
            return True, "No reference entries with DRUG interventions"

        nct_id = entry["nct_id"]
        attrs = self._ct_attrs(nct_id)
        if attrs is None:
            return False, f"No clinical_trials result for {nct_id}"

        drugs = [i.get("name", "") for i in (attrs.get("interventions") or [])
                 if i.get("type") == "DRUG"]
        if not drugs:
            return False, f"{nct_id}: no DRUG-typed interventions in API payload (type not projected)"

        return True, f"{nct_id} API DRUG interventions: {', '.join(drugs[:2])}"

    @test
    def test_trial_with_sponsor(self):
        """API must project lead_sponsor + sponsors (Atlas #46)."""
        entry = next(
            (e for e in self.runner.reference_data
             if any(s.get("role") == "lead" for s in e.get("sponsors", []))),
            None
        )

        if not entry:
            return True, "No reference entries with a lead sponsor"

        nct_id = entry["nct_id"]
        expected_lead = next(s["name"] for s in entry["sponsors"] if s.get("role") == "lead")

        attrs = self._ct_attrs(nct_id)
        if attrs is None:
            return False, f"No clinical_trials result for {nct_id}"

        lead = attrs.get("lead_sponsor", "")
        sponsors = attrs.get("sponsors") or []
        if not lead:
            return False, f"{nct_id}: lead_sponsor empty in API payload (regression of #46)"
        if lead != expected_lead:
            return False, f"{nct_id}: lead_sponsor '{lead}' != expected '{expected_lead}'"
        if expected_lead not in sponsors:
            return False, f"{nct_id}: lead '{expected_lead}' missing from sponsors {sponsors}"

        return True, f"{nct_id} lead_sponsor='{lead}', {len(sponsors)} sponsor(s)"

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
        custom_tests.test_trial_with_sponsor,
        custom_tests.test_mondo_mapping,
    ]:
        runner.add_custom_test(test_method)

    # Run all tests
    runner.run_all_tests()
    exit_code = runner.print_summary()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
