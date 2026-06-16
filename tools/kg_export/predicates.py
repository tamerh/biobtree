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


@dataclass(frozen=True)
class ReifiedRule:
    """An intermediate-entry dataset whose entries link real entities.

    kind == 'symmetric': all partners are `partner` dataset; emit undirected pairs.
    kind == 'bipartite': entries link a `subject` partner and an `object` partner;
        emit subject --predicate--> object.
    """

    dataset: str
    kind: str
    predicate: str
    partner: str | None = None  # symmetric
    subject: str | None = None  # bipartite
    object: str | None = None  # bipartite
    note: str | None = None


class PredicateMap:
    def __init__(
        self,
        direct: dict[str, PredicateRule],
        reified: dict[str, ReifiedRule] | None = None,
    ):
        self._direct = direct
        self._reified = reified or {}

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

        reified: dict[str, ReifiedRule] = {}
        for ds, cfg in (doc.get("reified", {}) or {}).items():
            if not isinstance(cfg, dict):
                raise ValueError(f"predicates.yaml reified: {ds!r} must be a mapping")
            kind = cfg.get("kind")
            if kind not in ("symmetric", "bipartite"):
                raise ValueError(f"predicates.yaml reified {ds!r}: bad kind {kind!r}")
            if "predicate" not in cfg:
                raise ValueError(f"predicates.yaml reified {ds!r}: needs 'predicate'")
            if kind == "symmetric" and not cfg.get("partner"):
                raise ValueError(f"predicates.yaml reified {ds!r}: symmetric needs 'partner'")
            if kind == "bipartite" and not (cfg.get("subject") and cfg.get("object")):
                raise ValueError(f"predicates.yaml reified {ds!r}: bipartite needs subject+object")
            reified[ds] = ReifiedRule(
                dataset=ds,
                kind=kind,
                predicate=cfg["predicate"],
                partner=cfg.get("partner"),
                subject=cfg.get("subject"),
                object=cfg.get("object"),
                note=cfg.get("note"),
            )
        return cls(direct, reified)

    @staticmethod
    def key(src_dataset: str, tgt_dataset: str) -> str:
        return f"{src_dataset}>{tgt_dataset}"

    def rule_for(self, src_dataset: str, tgt_dataset: str) -> PredicateRule | None:
        return self._direct.get(self.key(src_dataset, tgt_dataset))

    def pairs(self) -> list[str]:
        return list(self._direct)

    def reified_rule(self, dataset: str) -> ReifiedRule | None:
        return self._reified.get(dataset)

    def reified_datasets(self) -> list[str]:
        return list(self._reified)
