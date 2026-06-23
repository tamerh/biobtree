"""Reader for BioBTree sorted index files (the on-disk edge list).

File: ``<dataset>_sorted.<chunk>.index.gz`` (and ``<tgt>_from_<src>_sorted...``).
Each line is tab-separated:

    subject_id \\t source_dataset_numeric_id \\t object_id \\t object_dataset_numeric_id [ \\t evidence ] [ \\t relationship ]

This module parses lines into ``RawXref`` and, given a ``DatasetRegistry`` +
``CategoryMap``, resolves both endpoints to dataset names and biolink categories.
"""

from __future__ import annotations

import gzip
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator

from .categories import CategoryMap
from .datasets import DatasetRegistry

# When object_dataset_id == this sentinel the line is a NODE PROPERTY (written by
# the Go `addProp3` path: key, from, attr_json, -1), not a cross-reference edge.
PROPERTY_SENTINEL = "-1"


@dataclass(frozen=True)
class RawXref:
    """One raw line from a sorted index file, datasets still as numeric ids.

    May be either an xref edge or a node property (see ``is_property``).
    """

    subject: str
    source_dataset_id: str
    object: str
    object_dataset_id: str
    evidence: str | None = None
    relationship: str | None = None

    @property
    def is_property(self) -> bool:
        """True if this line is a node attribute, not an edge."""
        return self.object_dataset_id == PROPERTY_SENTINEL


@dataclass(frozen=True)
class Endpoint:
    """A resolved edge endpoint."""

    local_id: str
    dataset: str | None
    category: str | None
    prefix: str | None


@dataclass(frozen=True)
class ResolvedXref:
    subject: Endpoint
    object: Endpoint
    evidence: str | None
    relationship: str | None


class IndexParseError(ValueError):
    pass


def parse_index_line(line: str) -> RawXref:
    """Parse one sorted-index line. Raises IndexParseError on malformed input."""
    parts = line.rstrip("\n").split("\t")
    if len(parts) < 4:
        raise IndexParseError(f"expected >=4 tab fields, got {len(parts)}: {line!r}")
    subject, src_db, obj, obj_db = parts[0], parts[1], parts[2], parts[3]
    if not (subject and src_db and obj and obj_db):
        raise IndexParseError(f"empty required field in line: {line!r}")
    evidence = parts[4] if len(parts) >= 5 and parts[4] != "" else None
    relationship = parts[5] if len(parts) >= 6 and parts[5] != "" else None
    return RawXref(subject, src_db, obj, obj_db, evidence, relationship)


def resolve_xref(
    raw: RawXref, registry: DatasetRegistry, categories: CategoryMap
) -> ResolvedXref:
    """Resolve numeric dataset ids -> dataset names + biolink categories."""

    def endpoint(local_id: str, ds_id: str) -> Endpoint:
        ds = registry.name_for_id(ds_id)
        cat = categories.category_for(ds) if ds else None
        prefix = categories.prefix_for(ds) if ds else None
        return Endpoint(local_id=local_id, dataset=ds, category=cat, prefix=prefix)

    return ResolvedXref(
        subject=endpoint(raw.subject, raw.source_dataset_id),
        object=endpoint(raw.object, raw.object_dataset_id),
        evidence=raw.evidence,
        relationship=raw.relationship,
    )


def iter_index_file(
    path: str | Path, counter: dict | None = None
) -> Iterator[RawXref]:
    """Stream a (gzipped) sorted index file, yielding RawXref.

    Blank lines are skipped. Malformed lines are skipped (not fatal) and, if a
    ``counter`` dict is supplied, tallied under ``counter['malformed']`` — one
    bad line must never abort a multi-hundred-million-line run.
    """
    path = Path(path)
    opener = gzip.open if path.suffix == ".gz" else open
    with opener(path, "rt", encoding="utf-8") as fh:  # type: ignore[operator]
        for line in fh:
            if not line or line.isspace():
                continue
            try:
                yield parse_index_line(line)
            except IndexParseError:
                if counter is not None:
                    counter["malformed"] = counter.get("malformed", 0) + 1
                continue
