"""Predicate map: BioBTree dataset pair -> biolink predicate.

Loads ``mappings/predicates.yaml``. Keyed by ordered ``"src>tgt"``.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import yaml


@dataclass(frozen=True)
class PredicateRule:
    predicate: str | None  # None when skip is set
    flip: bool = False
    skip: str | None = None
    note: str | None = None

    @property
    def is_skip(self) -> bool:
        return self.skip is not None


class PredicateMap:
    def __init__(self, direct: dict[str, PredicateRule]):
        self._direct = direct

    @classmethod
    def load(cls, path: str | Path) -> "PredicateMap":
        doc = yaml.safe_load(Path(path).read_text()) or {}
        direct: dict[str, PredicateRule] = {}
        for key, cfg in (doc.get("direct", {}) or {}).items():
            if not isinstance(cfg, dict):
                raise ValueError(f"predicates.yaml: {key!r} must be a mapping")
            rule = PredicateRule(
                predicate=cfg.get("predicate"),
                flip=bool(cfg.get("flip", False)),
                skip=cfg.get("skip"),
                note=cfg.get("note"),
            )
            if rule.predicate is None and rule.skip is None:
                raise ValueError(f"predicates.yaml: {key!r} needs 'predicate' or 'skip'")
            direct[key] = rule
        return cls(direct)

    @staticmethod
    def key(src_dataset: str, tgt_dataset: str) -> str:
        return f"{src_dataset}>{tgt_dataset}"

    def rule_for(self, src_dataset: str, tgt_dataset: str) -> PredicateRule | None:
        return self._direct.get(self.key(src_dataset, tgt_dataset))

    def pairs(self) -> list[str]:
        return list(self._direct)
