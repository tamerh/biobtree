#!/usr/bin/env python3
"""SpliceAI (Ensembl MANE source) test suite."""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner


class SpliceaiTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _ref(self, key):
        r = self.runner.reference_data
        return r.get(key, []) if isinstance(r, dict) else []

    def _attr(self, vid):
        for r in requests.get(f"{self.api}/ws/", params={"i": vid, "s": "spliceai", "d": "0"}, timeout=15).json().get("results", []):
            if r.get("dataset_name") == "spliceai":
                return r["Attributes"].get("Spliceai", {})
        return None

    def test_all_alts_present(self):
        """Every alt at a position is kept, with correct effect + delta (fixes #1/#2)"""
        for v in self._ref("present"):
            a = self._attr(v["id"])
            if a is None:
                return False, f"{v['id']} missing (should be present)"
            if a.get("effect") != v["effect"]:
                return False, f"{v['id']} effect {a.get('effect')} != {v['effect']}"
            if a.get("ds_dl") != v["ds_dl"]:
                return False, f"{v['id']} ds_dl {a.get('ds_dl')!r} != {v['ds_dl']!r}"
            if a.get("gene_symbol") != v["gene"]:
                return False, f"{v['id']} gene {a.get('gene_symbol')} != {v['gene']}"
        return True, f"all {len(self._ref('present'))} alts present with deltas"

    def test_below_threshold_filtered(self):
        """Variants with all deltas < 0.2 are not ingested"""
        for vid in self._ref("absent"):
            if self._attr(vid) is not None:
                return False, f"{vid} should have been filtered out"
        return True, "below-threshold variants filtered"

    def test_four_deltas_stored(self):
        """All four DS_* and DP_* fields are present"""
        a = self._attr("8:42305277:T:C")
        if a is None:
            return False, "IKBKB variant missing"
        for k in ("ds_ag", "ds_al", "ds_dg", "ds_dl", "dp_ag", "dp_al", "dp_dg", "dp_dl"):
            if k not in a:
                return False, f"missing delta field {k}"
        return True, f"4 deltas+positions present (DS_DG={a.get('ds_dg')})"

    def test_masked_deltas(self):
        """Masked deltas are merged per variant; masked 0 vs raw>=0.2 = negative evidence"""
        # IKBKB: raw donor-gain 0.98, masking zeroes it (strengthens existing site)
        a = self._attr("8:42305277:T:C")
        if a is None:
            return False, "IKBKB variant missing"
        if a.get("ds_dg") != "0.98":
            return False, f"raw ds_dg {a.get('ds_dg')!r} != 0.98"
        if a.get("ds_dg_masked") != "0.00":
            return False, f"masked ds_dg_masked {a.get('ds_dg_masked')!r} != 0.00 (should be masked to 0)"
        # LNPK: real donor-loss site, masked == raw
        b = self._attr("2:175937343:C:A")
        if b is None or b.get("ds_dl") != "0.99" or b.get("ds_dl_masked") != "0.99":
            return False, f"LNPK masked mismatch: ds_dl={b and b.get('ds_dl')} masked={b and b.get('ds_dl_masked')}"
        return True, "masked merged: IKBKB raw 0.98/masked 0.00, LNPK raw=masked 0.99"

    def test_raw_only_empty_masked(self):
        """A position the masked VCF omits is kept from raw with empty masked deltas"""
        a = self._attr("1:200000:C:A")  # raw acceptor-loss 0.30; masked VCF omits this position
        if a is None:
            return False, "raw-only variant 1:200000:C:A missing (merge-join should keep it)"
        if a.get("ds_al") != "0.30" or a.get("effect") != "acceptor_loss":
            return False, f"raw-only deltas wrong: ds_al={a.get('ds_al')} effect={a.get('effect')}"
        if a.get("ds_al_masked"):
            return False, f"masked should be empty for a masked-omitted position (got {a.get('ds_al_masked')!r})"
        return True, "raw-only position kept (AL 0.30) with empty masked (merge-join skipped it correctly)"

    def test_indels_present(self):
        """Illumina indel scores are ingested (Ensembl is SNV-only)"""
        d = self._attr("2:175937340:CA:C")  # deletion, acceptor loss 0.34
        if d is None:
            return False, "indel 2:175937340:CA:C missing"
        if d.get("ds_al") != "0.34" or d.get("effect") != "acceptor_loss":
            return False, f"indel deltas wrong: ds_al={d.get('ds_al')} effect={d.get('effect')}"
        ins = self._attr("17:43045703:C:CT")  # insertion, donor gain 0.46
        if ins is None or ins.get("ds_dg") != "0.46":
            return False, f"insertion 17:43045703:C:CT missing or wrong (ds_dg={ins and ins.get('ds_dg')})"
        # indels have no masked file -> masked deltas empty
        if d.get("ds_al_masked"):
            return False, f"indel should have empty masked (got {d.get('ds_al_masked')!r})"
        return True, "indels ingested: CA>C (AL 0.34), C>CT (DG 0.46), masked empty"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'), script_dir / "reference_data.json")
    c = SpliceaiTests(runner)
    for m in [c.test_all_alts_present, c.test_below_threshold_filtered, c.test_four_deltas_stored,
              c.test_masked_deltas, c.test_raw_only_empty_masked, c.test_indels_present]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
