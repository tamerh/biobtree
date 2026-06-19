"""General node-attribute layer: each entry's own attributes -> its node's properties.

In BioBTree every entry already carries its full attribute set; `compact_fields` is
just a convenience projection surfaced inline in mapping responses (a performance
shortcut). A materialized KG has no such distinction -- a node IS the entry -- so we
attach the entry's attributes directly as node properties, which is what makes the
API's `/ws/filter` (CEL over attributes) reproducible as a Neo4j/Cypher `WHERE`.

Default mode = ``all``: every top-level field becomes a property -- scalars and
scalar lists kept as-is, one level of nested dicts flattened (``sequence.length`` ->
``<ds>_sequence_length``); lists of objects are skipped (heavy/relational -- those
belong as edges). Mode ``compact`` carries only the dataset's conf ``compact_fields``
(BioBTree's own curation of the high-value filterable fields) -- the opt-in slim knob
for heavy datasets.

Keys are **dataset-prefixed** (``entrez_symbol``, ``ensembl_biotype``) so a merged
node (one gene = HGNC+Ensembl+NCBIGene) collects every namespace's attributes without
collision. Output is a node-attribute table (``node \t {json}``) merged at assemble
via ``--node-attributes`` (same format as attributes.py). Config: mappings/node_attributes.yaml.
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

_MAX_STR = 500  # cap long free-text (summary/definition/comments) to keep nodes lean
_MAX_SYNONYMS = 50  # cap synonym list length per node-dataset (chebi has hundreds)
_SCALAR = (str, int, float, bool)

# runtime-built datasets have no categories.yaml entry; their node ids use these
# prefixes (matching go.py / refseq.py / mesh.py) so synonyms land on the right node.
_RUNTIME_PREFIXES = {"go": "GO", "refseq": "refseq", "mesh": "MESH"}


@dataclass
class NodeAttrStats:
    datasets_processed: int = 0
    rows_written: int = 0
    fields_extracted: int = 0
    by_dataset: dict = field(default_factory=lambda: defaultdict(int))

    def to_json(self) -> dict:
        d = self.__dict__.copy()
        d["by_dataset"] = dict(self.by_dataset)
        return d


def load_config(path: str | Path) -> dict:
    return yaml.safe_load(Path(path).read_text()) or {}


def _cap(v):
    if isinstance(v, str) and len(v) > _MAX_STR:
        return v[:_MAX_STR]
    return v


def _scalar_list(v) -> bool:
    return isinstance(v, list) and bool(v) and all(isinstance(x, _SCALAR) for x in v)


def _extract_all(d: dict, ds: str, exclude: set) -> dict:
    """All top-level fields: scalars + scalar-lists kept; one level of nested dicts
    flattened; lists-of-objects skipped."""
    out: dict = {}
    for k, v in d.items():
        if k in exclude:
            continue
        key = f"{ds}_{k}"
        if isinstance(v, _SCALAR):
            out[key] = _cap(v)
        elif _scalar_list(v):
            out[key] = [_cap(x) for x in v]
        elif isinstance(v, dict):
            for sk, sv in v.items():
                if sk in exclude:
                    continue
                if isinstance(sv, _SCALAR):
                    out[f"{key}_{sk}"] = _cap(sv)
                elif _scalar_list(sv):
                    out[f"{key}_{sk}"] = [_cap(x) for x in sv]
        # list of dicts -> skipped (relational / heavy)
    return out


def _parse_compact(spec: str) -> list[tuple[list[str], bool]]:
    """conf compact_fields/attrs spec -> [(path, is_list)]. Handles `field`,
    `[]field`, `field.sub`, `[]field.sub`."""
    out = []
    for tok in (spec or "").split(","):
        tok = tok.strip()
        if not tok:
            continue
        is_list = tok.startswith("[]")
        tok = tok[2:] if is_list else tok
        out.append((tok.split("."), is_list))
    return out


def _extract_compact(d: dict, ds: str, spec: list[tuple[list[str], bool]], exclude: set) -> dict:
    out: dict = {}
    for path, is_list in spec:
        if path[0] in exclude:
            continue
        key = f"{ds}_{'_'.join(path)}"
        if len(path) == 1:
            v = d.get(path[0])
        else:  # one level of nesting: dict path or list-of-dicts leaf
            base = d.get(path[0])
            if isinstance(base, dict):
                v = base.get(path[1])
            elif isinstance(base, list):
                v = [x.get(path[1]) for x in base if isinstance(x, dict) and x.get(path[1]) is not None]
            else:
                v = None
        if v in (None, "", []):
            continue
        if isinstance(v, _SCALAR):
            out[key] = _cap(v)
        elif _scalar_list(v):
            out[key] = [_cap(x) for x in v]
    return out


def _collect_synonyms(d: dict, fields: list[str]) -> list[str]:
    """Gather a dataset's alias/synonym fields into one deduped string list (the
    node's searchable `synonym` slot). Fields may be `synonyms`, `names`, nested
    `molecule.altNames`, list-of-objects `[]x.y`, or scalar (`common_name`)."""
    out: list[str] = []
    seen: set[str] = set()

    def add(x):
        if isinstance(x, str):
            x = x.strip()
            if x and x not in seen and len(out) < _MAX_SYNONYMS:
                seen.add(x)
                out.append(_cap(x))

    for f in fields:
        path = f.split(".")
        if len(path) == 1:
            v = d.get(path[0])
        else:
            base = d.get(path[0])
            if isinstance(base, dict):
                v = base.get(path[1])
            elif isinstance(base, list):
                v = [x.get(path[1]) for x in base if isinstance(x, dict)]
            else:
                v = None
        if isinstance(v, str):
            add(v)
        elif isinstance(v, list):
            for x in v:
                add(x)
    return out


def build_node_attributes(
    index_dir: str | Path,
    registry: DatasetRegistry,
    categories: CategoryMap,
    config: dict,
    out_path: str | Path,
    id_map: dict[str, str] | None = None,
    stats_path: str | Path | None = None,
) -> NodeAttrStats:
    index_dir = Path(index_dir)
    id_map = id_map or {}
    stats = NodeAttrStats()
    default_mode = (config.get("defaults") or {}).get("mode", "all")
    global_exclude = set(config.get("exclude") or [])
    attr_datasets = config.get("datasets") or {}
    syn_datasets = config.get("synonyms") or {}

    out_path = Path(out_path)
    out_path.parent.mkdir(parents=True, exist_ok=True)

    with kgx.xopen(out_path, "wt") as out:
        for ds in sorted(set(attr_datasets) | set(syn_datasets)):
            prefix = categories.prefix_for(ds) or _RUNTIME_PREFIXES.get(ds)
            if not prefix:
                continue
            files = sorted(glob.glob(str(index_dir / f"{ds}_sorted.*.index.gz")))
            if not files:
                continue
            attr_cfg = attr_datasets.get(ds)  # None -> no prefixed attrs for this ds
            syn_fields = syn_datasets.get(ds)  # None -> no synonyms for this ds
            spec = None
            exclude = global_exclude
            mode = None
            if attr_cfg is not None:
                attr_cfg = attr_cfg or {}
                mode = attr_cfg.get("mode", default_mode)
                exclude = global_exclude | set(attr_cfg.get("exclude") or [])
                if mode == "compact":
                    meta = registry.by_name(ds)
                    raw = meta.raw if meta else {}
                    spec = _parse_compact(raw.get("compact_fields") or raw.get("attrs") or "")
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
                    out_attrs: dict = {}
                    if attr_cfg is not None:
                        out_attrs.update(_extract_compact(d, ds, spec, exclude) if mode == "compact"
                                         else _extract_all(d, ds, exclude))
                    if syn_fields:
                        syns = _collect_synonyms(d, syn_fields)
                        if syns:
                            out_attrs["synonym"] = syns
                    if not out_attrs:
                        continue
                    node = to_curie(prefix, raw.subject)
                    node = id_map.get(node, node)
                    out.write(f"{node}\t{json.dumps(out_attrs)}\n")
                    stats.rows_written += 1
                    stats.fields_extracted += len(out_attrs)
                    stats.by_dataset[ds] += 1

    if stats_path:
        Path(stats_path).write_text(json.dumps(stats.to_json(), indent=2))
    return stats
