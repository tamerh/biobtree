"""Dataset registry: resolve BioBTree's numeric dataset ids to dataset metadata.

The sorted index files encode datasets by their numeric ``id`` (config field),
not by name:

    subject_id \\t source_dataset_numeric_id \\t object_id \\t object_dataset_numeric_id [...]

This module loads ``conf/*.dataset.json`` and builds the ``id -> name`` map (and
name -> metadata) the exporter needs to make sense of those columns.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

# Order matters only for error messages; ids are globally unique (asserted on load).
CONF_FILES = (
    "source1.dataset.json",
    "source2.dataset.json",
    "xref1.dataset.json",
    "xref2.optional.dataset.json",
)


@dataclass(frozen=True)
class Dataset:
    """Metadata for one BioBTree dataset, as declared in conf/*.dataset.json."""

    name: str
    numeric_id: str
    group: str | None
    federation: str
    aliases: tuple[str, ...]
    url_template: str | None
    source_file: str
    raw: dict

    @property
    def is_child(self) -> bool:
        """Heuristic: ontology/parent-child helper datasets (e.g. goparent)."""
        return self.name.endswith(("child", "parent"))


class DatasetRegistry:
    """Lookup BioBTree datasets by numeric id or by name."""

    def __init__(self, datasets: Iterable[Dataset]):
        self._by_name: dict[str, Dataset] = {}
        self._by_id: dict[str, Dataset] = {}
        collisions: dict[str, list[str]] = {}
        for ds in datasets:
            self._by_name[ds.name] = ds
            if ds.numeric_id in self._by_id:
                collisions.setdefault(ds.numeric_id, [self._by_id[ds.numeric_id].name])
                collisions[ds.numeric_id].append(ds.name)
            else:
                self._by_id[ds.numeric_id] = ds
        if collisions:
            detail = "; ".join(f"id {k}: {v}" for k, v in collisions.items())
            raise ValueError(f"numeric dataset id collisions detected: {detail}")

    # -- construction -------------------------------------------------------

    @classmethod
    def load(cls, conf_dir: str | Path) -> "DatasetRegistry":
        conf_dir = Path(conf_dir)
        datasets: list[Dataset] = []
        for fname in CONF_FILES:
            path = conf_dir / fname
            if not path.exists():
                continue
            data = json.loads(path.read_text())
            for name, cfg in data.items():
                if not isinstance(cfg, dict) or "id" not in cfg:
                    continue
                aliases = tuple(
                    a.strip()
                    for a in str(cfg.get("aliases", "")).split(",")
                    if a.strip()
                )
                datasets.append(
                    Dataset(
                        name=name,
                        numeric_id=str(cfg["id"]),
                        group=cfg.get("group") or None,
                        federation=cfg.get("federation") or "main",
                        aliases=aliases,
                        url_template=cfg.get("url") or None,
                        source_file=fname,
                        raw=cfg,
                    )
                )
        if not datasets:
            raise FileNotFoundError(f"no dataset configs found under {conf_dir}")
        return cls(datasets)

    # -- lookups ------------------------------------------------------------

    def by_id(self, numeric_id: str | int) -> Dataset | None:
        return self._by_id.get(str(numeric_id))

    def by_name(self, name: str) -> Dataset | None:
        return self._by_name.get(name)

    def name_for_id(self, numeric_id: str | int) -> str | None:
        ds = self.by_id(numeric_id)
        return ds.name if ds else None

    def __len__(self) -> int:
        return len(self._by_name)

    def __contains__(self, name: object) -> bool:
        return name in self._by_name

    def names(self) -> list[str]:
        return list(self._by_name)
