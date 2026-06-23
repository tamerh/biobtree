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

    kind == 'pairwise': each PROPERTY line's JSON names the real binary pair via
        `subject_field`/`object_field` (both rendered with `partner`'s prefix).
        Emits exactly the asserted pairs — no clique. (intact, string_interaction)
    kind == 'star': the group key (subject) is one entity (`partner`); emit it to
        each edge-line partner of `partner` dataset (excluding itself).
        (diamond/esm2 similarity: query -> hits)
    kind == 'bipartite': entries link a `subject`-role partner and an `object`-role
        partner (by dataset); emit subject --predicate--> object. (bioactivity,
        dependency, expression)
    """

    dataset: str
    kind: str
    predicate: str
    partner: str | None = None  # pairwise, star
    subject: str | None = None  # bipartite
    object: str | None = None  # bipartite
    extra_subjects: list | None = None  # bipartite (plain): additional SUBJECT
                                        # datasets (e.g. alliance_disease: the gene
                                        # appears in one of several species
                                        # namespaces per entry)
    extra_objects: list | None = None  # bipartite (plain): additional object
                                       # datasets emitted with the same predicate
                                       # (e.g. gwas gene -> mondo AND oba traits)
    subject_field: str | None = None  # pairwise (JSON key)
    normalize_subject: bool = False  # bipartite (subject==dataset): the group KEY is
                                     # itself the stable id but colon-bearing (a variant
                                     # coordinate chr:pos:ref:alt); normalize :/- -> _
    object_field: str | None = None  # pairwise (JSON key)
    via: str | None = None  # bipartite: the in-entry intermediate id (dataset)
                            # whose <via>_sorted forward resolves to `object`
    resolve: str | None = None  # pairwise: 'symbol' -> field values are gene
                                # symbols, resolved to `partner` ids by symbol map
    require: dict | None = None  # pairwise: only emit if property fields match
                                 # (e.g. {database_a: UNIPROT} to keep protein-protein)
    qualifiers: dict | None = None  # {slot_name: qualifier_dataset} — in-group
                                    # partners of that dataset become edge
                                    # qualifiers (e.g. {assay_type: bao} on
                                    # bioactivity; {phenotypic_quality: pato} on gwas)
    qualifier_fields: dict | None = None  # {slot_name: property_json_key} — pull a
                                          # SCALAR from the group's property JSON onto
                                          # the edge (e.g. {p_value: p_value} on gwas,
                                          # {confidence: confidence} on panelapp)
    cross: bool = False  # pairwise: subject_field/object_field are LISTS; emit the
                         # all-pairs cross-product (each subject member x each object
                         # member). Used when both endpoints are multi-gene complexes
                         # whose per-side member symbols are spelled out (cellphonedb).
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
            if kind not in ("pairwise", "star", "bipartite"):
                raise ValueError(f"predicates.yaml reified {ds!r}: bad kind {kind!r}")
            if "predicate" not in cfg:
                raise ValueError(f"predicates.yaml reified {ds!r}: needs 'predicate'")
            if kind == "pairwise" and not (
                cfg.get("partner") and cfg.get("subject_field") and cfg.get("object_field")
            ):
                raise ValueError(
                    f"predicates.yaml reified {ds!r}: pairwise needs partner+subject_field+object_field"
                )
            if kind == "star" and not cfg.get("partner"):
                raise ValueError(f"predicates.yaml reified {ds!r}: star needs 'partner'")
            if kind == "bipartite" and not (cfg.get("subject") and cfg.get("object")):
                raise ValueError(f"predicates.yaml reified {ds!r}: bipartite needs subject+object")
            reified[ds] = ReifiedRule(
                dataset=ds,
                kind=kind,
                predicate=cfg["predicate"],
                partner=cfg.get("partner"),
                subject=cfg.get("subject"),
                object=cfg.get("object"),
                extra_subjects=cfg.get("extra_subjects"),
                extra_objects=cfg.get("extra_objects"),
                subject_field=cfg.get("subject_field"),
                object_field=cfg.get("object_field"),
                normalize_subject=bool(cfg.get("normalize_subject", False)),
                via=cfg.get("via"),
                resolve=cfg.get("resolve"),
                require=cfg.get("require"),
                qualifiers=cfg.get("qualifiers"),
                qualifier_fields=cfg.get("qualifier_fields"),
                cross=bool(cfg.get("cross", False)),
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
