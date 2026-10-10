#!/usr/bin/env python3
"""
JASPAR TFBS (Ensembl Regulatory Build motif features) test suite.

Interval dataset: JASPAR TF motifs mapped within the Ensembl Regulatory Build,
reachable by a variant's position via >>jaspar_tfbs. Each motif carries its
binding_matrix_id (-> jaspar matrix) and transcription_factors (-> hgnc genes).

Data under test is the hand-crafted fixture
(tests/datasets/jaspar_tfbs/jaspar_tfbs_fixture.gff3), NOT real Ensembl data.
"""
import sys, os, requests
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent.parent))
from common import TestRunner


class JasparTfbsTests:
    def __init__(self, runner):
        self.runner = runner
        self.api = runner.api_url

    def _positions(self):
        ref = self.runner.reference_data
        return ref.get("positions", []) if isinstance(ref, dict) else []

    def _map(self, chr, pos):
        return requests.get(f"{self.api}/ws/map/",
                            params={"i": f"{chr}:{pos}", "m": ">>jaspar_tfbs"}, timeout=15).json()

    def test_interval_query(self):
        """A variant position resolves to the overlapping TFBS motif feature"""
        for pz in self._positions():
            m = self._map(pz["chr"], pz["pos"])
            ids = [t.get("identifier", "").upper() for r in (m.get("results") or []) for t in r.get("targets", [])]
            if pz["expect_id"].upper() not in ids:
                return False, f"{pz['chr']}:{pz['pos']} did not return {pz['expect_id']} (got {ids[:5]})"
            return True, f"{pz['chr']}:{pz['pos']} -> {pz['expect_id']}"
        return False, "no positions"

    def test_tf_and_matrix(self):
        """Overlapping motif carries transcription_factors + binding_matrix_id"""
        for pz in self._positions():
            m = self._map(pz["chr"], pz["pos"])
            for r in (m.get("results") or []):
                for t in r.get("targets", []):
                    if t.get("identifier", "").upper() != pz["expect_id"].upper():
                        continue
                    a = (t.get("Attributes") or {}).get("JasparTfbs", {})
                    tfs = a.get("transcription_factors") or []
                    if pz["expect_tf"] not in tfs:
                        return False, f"{pz['expect_id']} TFs {tfs} missing {pz['expect_tf']}"
                    if a.get("binding_matrix_id") != pz["expect_matrix"]:
                        return False, f"{pz['expect_id']} matrix {a.get('binding_matrix_id')} != {pz['expect_matrix']}"
                    return True, f"{pz['expect_id']} -> TFs={tfs} matrix={a.get('binding_matrix_id')}"
        return False, "no motif matched for TF/matrix check"

    def test_cel_score_filter(self):
        """CEL filter double(jaspar_tfbs.score)>10 keeps high-score motifs, drops low.

        Filtering is exercised via the direct /ws/ endpoint; the position->interval
        map path (>>jaspar_tfbs) does not thread an inline CEL filter."""
        hi = self._positions()[1]  # chr11 high-score (12.80)
        lo = self._positions()[2]  # chr17 low-score (3.07)
        ids = ",".join([hi["expect_id"], lo["expect_id"]])
        resp = requests.get(f"{self.api}/ws/",
                            params={"i": ids, "d": "1", "f": "double(jaspar_tfbs.score)>10.0"}, timeout=15)
        if resp.status_code != 200:
            return False, f"HTTP {resp.status_code}: {resp.text[:150]}"
        rows = [r for r in resp.json().get("results", []) if "JasparTfbs" in (r.get("Attributes") or {})]
        kept = {r.get("identifier") for r in rows}
        if hi["expect_id"] not in kept:
            return False, f"high-score {hi['expect_id']} (12.8) was filtered out (kept {kept})"
        if lo["expect_id"] in kept:
            return False, f"low-score {lo['expect_id']} (3.07) leaked past >10 filter"
        return True, f"score filter kept {hi['expect_id']} (12.8), dropped {lo['expect_id']} (3.07)"


def main():
    script_dir = Path(__file__).parent
    runner = TestRunner(os.environ.get('BIOBTREE_API_URL', 'http://localhost:9292'), script_dir / "reference_data.json")
    c = JasparTfbsTests(runner)
    for m in [c.test_interval_query, c.test_tf_and_matrix, c.test_cel_score_filter]:
        runner.add_custom_test(m)
    runner.run_all_tests()
    return runner.print_summary()


if __name__ == "__main__":
    sys.exit(main())
