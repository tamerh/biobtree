#!/usr/bin/env python3
"""ChIP-Atlas Target Genes tests.

One entry per (genome, TF, TSS window). Targets are genes whose TSS is within
the window of a ChIP-seq peak for the TF. Human targets link to HGNC; other
species link to Entrez by taxid. Test build processes the first two TFs per
genome, so the fixture entries are stable.
"""
import os
import sys
from pathlib import Path

import requests

sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner, test


def _entry(data, entry_id):
    for r in (data or {}).get("results", []):
        if r.get("identifier", "").upper() == entry_id.upper():
            return r
    return None


class ChipAtlasTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url
        self.ref = None

    def _entries(self):
        ref = self.runner.reference_data
        if isinstance(ref, list):
            return ref
        if isinstance(ref, dict):
            return ref.get("entries", [])
        return []

    def _attr(self, entry_id):
        data = self.runner.lookup(entry_id)
        r = _entry(data, entry_id)
        if not r:
            return None
        return (r.get("Attributes") or {}).get("ChipAtlasTfbs")

    @test
    def test_entry_attributes(self):
        """hg38 AATF has TF/genome/taxid/window and target summary attrs"""
        a = self._attr("CHIPATLAS:hg38:AATF:1")
        if a is None:
            return False, "CHIPATLAS:hg38:AATF:1 not found"
        if a.get("tf") != "AATF" or a.get("genome") != "hg38":
            return False, f"unexpected tf/genome: {a.get('tf')}/{a.get('genome')}"
        if a.get("tax_id") != "9606" or a.get("distance_kb") != 1:
            return False, f"unexpected taxid/distance: {a.get('tax_id')}/{a.get('distance_kb')}"
        if not a.get("target_count") or not a.get("experiment_count"):
            return False, f"missing counts: {a}"
        if a.get("top_targets_schema") != "target|avg_score":
            return False, f"unexpected top_targets_schema: {a.get('top_targets_schema')}"
        if not a.get("top_targets"):
            return False, "no top_targets"
        return True, f"AATF hg38: {a['target_count']} targets, {a['experiment_count']} experiments, {a['cell_type_count']} cell types"

    @test
    def test_human_targets_link_to_hgnc(self):
        """chip_atlas_tfbs >> hgnc returns target genes for a human TF"""
        r = requests.get(
            f"{self.api}/ws/map/",
            params={"i": "CHIPATLAS:hg38:AATF:1", "m": ">>chip_atlas_tfbs>>hgnc", "mode": "lite"},
            timeout=30,
        ).json()
        targets = []
        for m in r.get("mappings") or []:
            targets.extend(m.get("targets") or [])
        if not targets:
            return False, f"no hgnc targets: {str(r)[:200]}"
        if not all(t.split("|")[0].startswith("HGNC:") for t in targets):
            return False, f"non-HGNC target: {targets[:3]}"
        return True, f"AATF hg38 -> {len(targets)} HGNC targets"

    @test
    def test_nonhuman_targets_link_to_entrez(self):
        """A mouse TF links to Entrez genes, not human HGNC"""
        r = requests.get(
            f"{self.api}/ws/map/",
            params={"i": "CHIPATLAS:mm10:Acaa2:1", "m": ">>chip_atlas_tfbs>>entrez", "mode": "lite"},
            timeout=60,
        ).json()
        targets = []
        for m in r.get("mappings") or []:
            targets.extend(m.get("targets") or [])
        if not targets:
            return False, f"no Entrez targets: {str(r)[:200]}"
        return True, f"Acaa2 mm10 -> {len(targets)} Entrez targets"

    @test
    def test_all_genomes_present(self):
        """Every requested ChIP-Atlas genome has at least one entry"""
        genomes = set()
        for e in self._entries():
            a = self._attr(e["id"])
            if a is None:
                return False, f"missing entry {e['id']}"
            if a.get("genome") != e["genome"]:
                return False, f"{e['id']} genome={a.get('genome')} != {e['genome']}"
            if a.get("tax_id") != e["tax_id"]:
                return False, f"{e['id']} tax_id={a.get('tax_id')} != {e['tax_id']}"
            if int(a.get("target_count") or 0) < e["min_targets"]:
                return False, f"{e['id']} target_count={a.get('target_count')} < {e['min_targets']}"
            genomes.add(e["genome"])
        expected = {"hg38", "hg19", "mm10", "mm9", "rn6", "dm6", "dm3", "ce11", "ce10", "sacCer3", "TAIR12"}
        if genomes != expected:
            return False, f"genomes {sorted(genomes)} != {sorted(expected)}"
        return True, f"all {len(genomes)} genomes present"

    @test
    def test_reachable_from_hgnc(self):
        """A target HGNC gene reaches the ChIP-Atlas entry (reverse xref edge)"""
        # Use a target HGNC id from the AATF entry, so this does not depend on
        # whether the TF itself falls inside the limited hgnc test build.
        first = requests.get(
            f"{self.api}/ws/map/",
            params={"i": "CHIPATLAS:hg38:AATF:1", "m": ">>chip_atlas_tfbs>>hgnc", "mode": "lite"},
            timeout=30,
        ).json()
        hgnc_id = None
        for m in first.get("mappings") or []:
            for t in m.get("targets") or []:
                hgnc_id = t.split("|")[0]
                break
            if hgnc_id:
                break
        if not hgnc_id:
            return False, "no HGNC target to test the reverse edge"
        r = requests.get(
            f"{self.api}/ws/map/",
            params={"i": hgnc_id, "m": ">>hgnc>>chip_atlas_tfbs", "mode": "lite"},
            timeout=30,
        ).json()
        ids = []
        for m in r.get("mappings") or []:
            for t in m.get("targets") or []:
                ids.append(t.split("|")[0])
        if not any(i.upper().startswith("CHIPATLAS:HG38:AATF:") for i in ids):
            return False, f"{hgnc_id} did not reach chip_atlas: {str(r)[:240]}"
        return True, f"{hgnc_id} -> {len(ids)} ChIP-Atlas entries"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(
        os.environ.get("BIOBTREE_API_URL", "http://localhost:9292"),
        script_dir / "reference_data.json",
    )
    c = ChipAtlasTests(runner)
    for m in [
        c.test_entry_attributes,
        c.test_human_targets_link_to_hgnc,
        c.test_nonhuman_targets_link_to_entrez,
        c.test_all_genomes_present,
        c.test_reachable_from_hgnc,
    ]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
