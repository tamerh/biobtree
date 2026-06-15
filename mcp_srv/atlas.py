"""
Sugi Atlas integration for the biobtree MCP.

Grounds answers about genes, diseases and drugs in curated Atlas pages
(https://sugi.bio/atlas/<type>/<slug>/index.md).

Design (see ATLAS_MCP_INTEGRATION.md):
- The name->slug manifest is loaded once and cached in memory (refresh = restart
  the MCP). It gives synonym-robust resolution.
- Page content is fetched fresh per call (no app cache; HTTP caching handles
  efficiency) so it always reflects the latest Atlas. The fetch doubles as the
  coverage check: 200 = covered, 404 = skipped (never a dead link).
- Whole page is returned by default; the model can narrow with summary/section.
"""

import json
import logging
import re
import urllib.error
import urllib.request

logger = logging.getLogger(__name__)

ATLAS_BASE = "https://sugi.bio/atlas"
MANIFEST_URL = ATLAS_BASE + "/manifest.json"
ATLAS_TYPES = ("gene", "disease", "drug")
_UA = {"User-Agent": "biobtree-mcp"}

_manifest = None  # cached at first use; None until loaded


def _load_manifest():
    """Fetch + cache the Atlas name->slug manifest once (per process)."""
    global _manifest
    if _manifest is not None:
        return _manifest
    data = {t: {} for t in ATLAS_TYPES}
    try:
        req = urllib.request.Request(MANIFEST_URL, headers=_UA)
        with urllib.request.urlopen(req, timeout=90) as r:
            m = json.load(r)
        for t in ATLAS_TYPES:
            if isinstance(m.get(t), dict):
                data[t] = m[t]
        logger.info("Atlas manifest loaded: %s", {t: len(data[t]) for t in ATLAS_TYPES})
    except Exception as e:
        logger.warning("Atlas manifest load failed (%s); falling back to slugify", e)
    _manifest = data
    return _manifest


def _slugify(name):
    return re.sub(r"[^a-z0-9]+", "-", name.strip().lower()).strip("-")


def _resolve(name):
    """(type, slug) candidates for a name via the manifest, or None if unknown."""
    m = _load_manifest()
    n = name.strip()
    out = []
    for t in ATLAS_TYPES:
        slug = m[t].get(n) or m[t].get(n.lower())
        if slug:
            out.append((t, slug))
    return out or None


def _fetch_md(t, slug):
    url = "%s/%s/%s/index.md" % (ATLAS_BASE, t, slug)
    try:
        req = urllib.request.Request(url, headers=_UA)
        with urllib.request.urlopen(req, timeout=30) as r:
            if r.status == 200:
                body = r.read().decode("utf-8", "replace")
                return body, "%s/%s/%s/" % (ATLAS_BASE, t, slug)
    except urllib.error.HTTPError:
        return None, None
    except Exception as e:
        logger.warning("Atlas md fetch failed %s: %s", url, e)
    return None, None


def _headings(md):
    """Section names (## and ###), in document order — the valid `section=` values.
    Including sub-sections makes zones like 'Expression profiles' / 'Tissue specificity'
    discoverable, not just the top-level sections. De-duplicated (case-insensitive,
    first occurrence wins) so the list stays clean when a ## and ### share a name."""
    out, seen = [], set()
    for ln in md.splitlines():
        name = None
        if ln.startswith("## "):
            name = ln[3:].strip()
        elif ln.startswith("### "):
            name = ln[4:].strip()
        if name and name.lower() not in seen:
            seen.add(name.lower())
            out.append(name)
    return out


def _header(md):
    """Lines before the first '## ' (title + canonical/version line)."""
    out = []
    for ln in md.splitlines():
        if ln.startswith("## "):
            break
        out.append(ln)
    return "\n".join(out).strip()


def _block(md, name, levels=("## ",)):
    """The heading block whose title matches `name` (case-insensitive substring) at one
    of `levels`, from that heading until the next heading of same-or-higher level.
    Returns None if no heading matches."""
    target = name.strip().lower()
    out, capturing, cap_prefix = [], False, None
    for ln in md.splitlines():
        is_h2, is_h3 = ln.startswith("## "), ln.startswith("### ")
        if capturing:
            if is_h2 or (cap_prefix == "### " and is_h3):
                break
            out.append(ln)
            continue
        for lvl in levels:
            if ln.startswith(lvl) and target in ln[len(lvl):].strip().lower():
                capturing, cap_prefix = True, lvl
                out.append(ln)
                break
    return "\n".join(out).strip() if capturing else None


def _digest(md):
    """Compact, always-fits digest: title + the Summary and Identifiers sections."""
    parts = [_header(md)]
    for name in ("Summary", "Identifiers"):
        b = _block(md, name, levels=("## ",))
        if b:
            parts.append(b)
    return "\n\n".join(p for p in parts if p).strip()


# Soft budget on total content chars across the batch. A single section can be huge
# (drug Indications/bioactivity tables), so several entities x one big section can blow
# the MCP result limit. We trim oversized blocks and leave a breadcrumb telling the model
# how to get the full block (one entity at a time, or a narrower ### sub-section).
# Calibrated from observed MCP limits: a ~33KB single section returns fine, a ~54KB
# two-entity section overflowed -> keep total under ~45KB.
_MAX_TOTAL = 45000


def _enforce_budget(results):
    total = sum(len(r.get("content", "")) for r in results)
    if total <= _MAX_TOTAL or not results:
        return
    each = max(6000, _MAX_TOTAL // len(results))
    for r in results:
        c = r.get("content", "")
        if len(c) > each:
            r["content"] = c[:each].rstrip() + (
                "\n\n…[trimmed to fit; call atlas with this single entity"
                " (and a narrower section= from `sections`) for the full block]"
            )
            r["trimmed"] = True


def atlas_lookup(entities, section=None, full=False):
    """Resolve each entity to its Atlas page and return grounding content.

    Returns {"results": [{entity, type, canonical_url, content, sections}], "not_covered": [...]}.
    Default content is the compact digest (Summary + Identifiers); section= returns one
    zone; full=True returns the whole page (large).
    """
    results, not_covered = [], []
    for name in entities or []:
        if not name or not name.strip():
            continue
        resolved = _resolve(name)
        # Manifest miss -> slugify + try each type (recovers brand-new pages).
        candidates = resolved if resolved else [(t, _slugify(name)) for t in ATLAS_TYPES]
        hit = None
        for (t, slug) in candidates:
            md, canonical = _fetch_md(t, slug)
            if md:
                hit = (t, md, canonical)
                break
        if not hit:
            not_covered.append(name)
            continue
        t, md, canonical = hit
        item = {
            "entity": name,
            "type": t,
            "canonical_url": canonical,
            "sections": _headings(md),
        }
        if full:
            item["content"] = md
        elif section:
            blk = _block(md, section, levels=("## ", "### "))
            if blk is not None:
                item["content"] = blk
            else:
                item["content"] = _digest(md)
                item["section_note"] = "section '%s' not found; pick one from sections" % section
        else:
            item["content"] = _digest(md)
        results.append(item)
    _enforce_budget(results)
    return {"results": results, "not_covered": not_covered}
