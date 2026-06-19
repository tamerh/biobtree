"""Numeric/value NODE attributes (#1 part A).

Some datasets aren't relationships or entities -- their content IS a scalar
*about* an existing entity (gnomad_constraint pLI/LOEUF about a gene, alphafold
pLDDT about a protein, depmap essentiality about a gene, alphamissense_transcript
mean pathogenicity about a transcript). This builder reads each such dataset's
property JSON, canonicalizes the subject to the entity's node CURIE (via the
entity dataset's prefix + the Phase-1 id_map), extracts the configured fields,
and writes a node-attribute table:

    node_curie \t {"gnomad_pli": 1.9e-05, "gnomad_loeuf": 1.005}

`assemble --node-attributes <table>` merges these into nodes.jsonl as properties
(a node can collect attributes from several datasets). Config: mappings/attributes.yaml.
"""

from __future__ import annotations

import glob
import json
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

import yaml

from . import kgx
from .categories import CategoryMap
from .curie import to_curie
from .datasets import DatasetRegistry
from .index import iter_index_file


@dataclass
class AttrStats:
    datasets_processed: int = 0
    rows_written: int = 0
    fields_extracted: int = 0
    by_dataset: dict = field(default_factory=lambda: defaultdict(int))

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_dataset"] = dict(self.by_dataset)
        return d


def _num(v):
    """Coerce a property scalar to float when it looks numeric, else keep string."""
    s = str(v).strip()
    try:
        return float(s)
    except (ValueError, TypeError):
        return s


def load_config(path: str | Path) -> dict:
    return (yaml.safe_load(Path(path).read_text()) or {}).get("node_attributes", {})


def build_attributes(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    config: dict,
    out_path: str | Path,
    id_map: dict[str, str] | None = None,
    stats_path: str | Path | None = None,
) -> AttrStats:
    index_dir = Path(index_dir)
    id_map = id_map or {}
    stats = AttrStats()
    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)

    with kgx.xopen(out_path, "wt") as out:
        for ds, cfg in config.items():
            prefix = categories.prefix_for(cfg["entity"])
            if not prefix:
                continue
            fields = cfg["fields"]  # {out_property: json_key}
            files = sorted(glob.glob(str(index_dir / f"{ds}_sorted.*.index.gz")))
            if not files:
                continue
            stats.datasets_processed += 1
            for path in files:
                for raw in iter_index_file(path):
                    if not raw.is_property:
                        continue
                    try:
                        d = json.loads(raw.object)
                    except (ValueError, TypeError):
                        continue
                    if not isinstance(d, dict):
                        continue
                    attrs = {}
                    for out_key, jkey in fields.items():
                        v = d.get(jkey)
                        if v not in (None, "", []):
                            attrs[out_key] = _num(v)
                    if not attrs:
                        continue
                    node = to_curie(prefix, raw.subject)
                    node = id_map.get(node, node)
                    out.write(f"{node}\t{json.dumps(attrs)}\n")
                    stats.rows_written += 1
                    stats.fields_extracted += len(attrs)
                    stats.by_dataset[ds] += 1

    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats


def load_attributes(path: str | Path) -> dict[str, dict]:
    """Merge a node-attribute table into {node_id: {property: value, ...}}."""
    merged: dict[str, dict] = {}
    p = Path(path)
    if not p.exists():
        return merged
    with kgx.xopen(p, "rt") as fh:
        for line in fh:
            line = line.rstrip("\n")
            if not line or "\t" not in line:
                continue
            node, js = line.split("\t", 1)
            try:
                attrs = json.loads(js)
            except (ValueError, TypeError):
                continue
            merged.setdefault(node, {}).update(attrs)
    return merged
