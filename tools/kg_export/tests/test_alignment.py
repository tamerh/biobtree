"""Alignment test: the export must match the live biobtree SERVICE (the served
graph), not just be internally consistent. For a sample of entities across types,
the per-target forward edge counts read from the index files must equal what the
service reports — catching any drift between the export and biobtree's own graph.

Opt-in (needs the running service + the matching index dir, same DB version):
    BIOBTREE_INDEX_DIR=/data/biobtree/out_prod/main/index \
    BIOBTREE_URL=http://localhost:9291 \
        python3 -m unittest tools.kg_export.tests.test_alignment -v

Skipped if either is unavailable. NOTE: index dir and service must be the SAME
data version, else counts legitimately differ.
"""

import json
import os
import unittest
import urllib.request
from collections import Counter
from pathlib import Path

from tools.kg_export.datasets import DatasetRegistry
from tools.kg_export.index import iter_index_file

REPO_ROOT = Path(__file__).resolve().parents[3]
CONF_DIR = REPO_ROOT / "conf"

# (entity, its dataset, forward-sourced targets that should match the service
# count exactly — stable 1:1-ish xrefs stored in the entity's own forward file)
CASES = [
    {"id": "MONDO:0007254", "dataset": "mondo",
     "targets": ["doid", "uberon", "ncit", "umls", "medgen", "sctid"]},
    {"id": "HGNC:1100", "dataset": "hgnc",
     "targets": ["ensembl", "entrez"]},
]


def _index_dir():
    # opt-in only (needs the live service too) — must not run in the fast suite
    env = os.environ.get("BIOBTREE_INDEX_DIR")
    return env if env and Path(env).exists() else None


def _service_url():
    return os.environ.get("BIOBTREE_URL", "http://localhost:9291")


def _service_xref_counts(base, ident, dataset):
    url = f"{base}/ws/entry/?i={urllib.parse.quote(ident)}&s={dataset}"
    with urllib.request.urlopen(url, timeout=30) as r:
        d = json.load(r)
    return {x.split("|")[0]: int(x.split("|")[1])
            for x in d.get("xrefs", {}).get("data", [])}


import urllib.parse  # noqa: E402


def _index_forward_counts(index_dir, registry, dataset, subject):
    import glob
    files = glob.glob(str(Path(index_dir) / f"{dataset}_sorted.*.index.gz"))
    counts = Counter()
    for raw in iter_index_file(files[0]):
        if raw.subject != subject:
            if raw.subject > subject:
                break
            continue
        if raw.is_property:
            continue
        counts[registry.name_for_id(raw.object_dataset_id)] += 1
    return counts


class AlignmentTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.index_dir = _index_dir()
        if not cls.index_dir:
            raise unittest.SkipTest("no index dir (set BIOBTREE_INDEX_DIR)")
        cls.base = _service_url()
        try:
            urllib.request.urlopen(f"{cls.base}/ws/meta", timeout=10).read(1)
        except Exception as e:
            raise unittest.SkipTest(f"biobtree service unreachable at {cls.base}: {e}")
        cls.reg = DatasetRegistry.load(CONF_DIR)

    def test_forward_counts_match_service(self):
        for case in CASES:
            svc = _service_xref_counts(self.base, case["id"], case["dataset"])
            idx = _index_forward_counts(
                self.index_dir, self.reg, case["dataset"], case["id"])
            for t in case["targets"]:
                self.assertGreater(svc.get(t, 0), 0,
                                   f"{case['id']}: service has no {t}")
                self.assertEqual(
                    idx.get(t, 0), svc.get(t, 0),
                    f"{case['id']} -> {t}: index forward {idx.get(t,0)} != "
                    f"service {svc.get(t,0)} (export drifted from served graph)",
                )


if __name__ == "__main__":
    unittest.main()
