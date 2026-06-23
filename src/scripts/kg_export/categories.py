"""Node category map: BioBTree dataset -> biolink category + CURIE prefix.

Loads ``mappings/categories.yaml`` (the authored table) and exposes lookups the
exporter uses to type nodes and to pick canonical identifiers during
normalization.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import yaml


@dataclass(frozen=True)
class CategoryEntry:
    dataset: str
    category: str
    prefix: str
    note: str | None = None


class CategoryMap:
    """Dataset -> biolink category, plus per-category canonical priority."""

    def __init__(
        self,
        entries: dict[str, CategoryEntry],
        canonical_priority: dict[str, list[str]],
        identity_pairs: set[frozenset[str]] | None = None,
    ):
        self._entries = entries
        self._priority = canonical_priority
        self._identity_pairs = identity_pairs or set()

    # -- construction -------------------------------------------------------

    @classmethod
    def load(cls, path: str | Path) -> "CategoryMap":
        doc = yaml.safe_load(Path(path).read_text()) or {}
        raw_entries = doc.get("datasets", {}) or {}
        entries: dict[str, CategoryEntry] = {}
        for dataset, cfg in raw_entries.items():
            if not isinstance(cfg, dict):
                raise ValueError(f"categories.yaml: entry for {dataset!r} must be a mapping")
            try:
                entries[dataset] = CategoryEntry(
                    dataset=dataset,
                    category=cfg["category"],
                    prefix=cfg["prefix"],
                    note=cfg.get("note"),
                )
            except KeyError as e:
                raise ValueError(
                    f"categories.yaml: entry for {dataset!r} missing required field {e}"
                ) from None
        priority = doc.get("canonical_priority", {}) or {}
        pairs = {
            frozenset(p)
            for p in (doc.get("identity_pairs", []) or [])
            if len(p) == 2
        }
        return cls(entries, priority, pairs)

    # -- lookups ------------------------------------------------------------

    def category_for(self, dataset: str) -> str | None:
        e = self._entries.get(dataset)
        return e.category if e else None

    def prefix_for(self, dataset: str) -> str | None:
        e = self._entries.get(dataset)
        return e.prefix if e else None

    def entry_for(self, dataset: str) -> CategoryEntry | None:
        return self._entries.get(dataset)

    def is_node_dataset(self, dataset: str) -> bool:
        """True if this dataset contributes typed nodes (vs. edge-only)."""
        return dataset in self._entries

    def priority_for(self, category: str) -> list[str]:
        """Dataset order for choosing the canonical CURIE within a category."""
        return list(self._priority.get(category, []))

    def is_identity_pair(self, dataset_a: str, dataset_b: str) -> bool:
        """True if a cross-ref between these datasets means 'same entity'."""
        return frozenset((dataset_a, dataset_b)) in self._identity_pairs

    def identity_pairs(self) -> set[frozenset[str]]:
        return set(self._identity_pairs)

    def datasets(self) -> list[str]:
        return list(self._entries)

    def categories(self) -> set[str]:
        return {e.category for e in self._entries.values()}
