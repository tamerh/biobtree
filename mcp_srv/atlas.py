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
import os
import re
import ssl
import urllib.error
import urllib.request

logger = logging.getLogger(__name__)

# ATLAS_URL lets the deployment point the fetch at an internal origin. In prod it is the
# public URL but DNS-routed (via the container's extra_hosts) to the host's own nginx, so
# the fetch bypasses Cloudflare's bot challenge. Default stays public for dev/local use.
ATLAS_BASE = os.environ.get("ATLAS_URL", "https://sugi.bio/atlas").rstrip("/")
MANIFEST_URL = ATLAS_BASE + "/manifest.json"
ATLAS_TYPES = ("gene", "disease", "drug")
_UA = {"User-Agent": "biobtree-mcp"}

# The host origin presents a Cloudflare Origin CA cert, which is not in the public trust
# store, so verified TLS fails on this host-local hop. ATLAS_INSECURE_TLS=1 (set in the
# prod compose) skips verification for the Atlas fetch only; unset = normal verification.
_SSL_CTX = None
if ATLAS_BASE.startswith("https") and os.environ.get("ATLAS_INSECURE_TLS") == "1":
    _SSL_CTX = ssl.create_default_context()
    _SSL_CTX.check_hostname = False
    _SSL_CTX.verify_mode = ssl.CERT_NONE

_manifest = None  # cached at first use; None until loaded


def _load_manifest():
    """Fetch + cache the Atlas name->slug manifest once (per process)."""
    global _manifest
    if _manifest is not None:
        return _manifest
    data = {t: {} for t in ATLAS_TYPES}
    try:
        req = urllib.request.Request(MANIFEST_URL, headers=_UA)
        with urllib.request.urlopen(req, timeout=90, context=_SSL_CTX) as r:
            m = json.load(r)
        for t in ATLAS_TYPES:
            if isinstance(m.get(t), dict):
                data[t] = m[t]
        logger.info("Atlas manifest loaded: %s", {t: len(data[t]) for t in ATLAS_TYPES})
    except Exception as e:
        logger.warning("Atlas manifest load failed (%s); falling back to slugify", e)
    _manifest = data
    return _manifest


def _slug_for(t, name):
    """Type-aware slug for the manifest-miss fallback. Gene pages are keyed by the exact
    HGNC symbol (uppercase, hyphens kept: TP53, A1BG-AS1), so we must NOT lowercase them;
    disease/drug pages use lowercase-hyphenated slugs."""
    n = name.strip()
    if t == "gene":
        return re.sub(r"[^A-Za-z0-9-]+", "-", n).strip("-").upper()
    return re.sub(r"[^a-z0-9]+", "-", n.lower()).strip("-")


def _resolve(name):
    """(type, slug) candidates for a name via the manifest, or None if unknown."""
    m = _load_manifest()
    n = name.strip()
    out = []
    for t in ATLAS_TYPES:
        # Try verbatim, then lower (disease/drug), then upper (gene symbols like TP53).
        slug = m[t].get(n) or m[t].get(n.lower()) or m[t].get(n.upper())
        if slug:
            out.append((t, slug))
    return out or None


def _fetch_md(t, slug):
    """Return (body, canonical_url, error). error is None on a 200 hit or a clean 404
    (page genuinely absent); it is a short string only when the fetch itself failed
    (challenge/block/5xx/network) — which is NOT the same as "not covered", so callers
    can tell a blocked fetch from an absent page instead of silently merging them."""
    url = "%s/%s/%s/index.md" % (ATLAS_BASE, t, slug)
    try:
        req = urllib.request.Request(url, headers=_UA)
        with urllib.request.urlopen(req, timeout=30, context=_SSL_CTX) as r:
            if r.status == 200:
                body = r.read().decode("utf-8", "replace")
                return body, "%s/%s/%s/" % (ATLAS_BASE, t, slug), None
            return None, None, "HTTP %s" % r.status
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return None, None, None
        logger.warning("Atlas fetch blocked/failed %s -> HTTP %s", url, e.code)
        return None, None, "HTTP %s" % e.code
    except Exception as e:
        logger.warning("Atlas md fetch failed %s: %s", url, e)
        return None, None, str(e)


def _heading_level(ln):
    """Return (level, title) for a Markdown heading of level 2-4, else (0, "")."""
    if not ln.startswith("#"):
        return 0, ""
    n = 0
    while n < len(ln) and ln[n] == "#":
        n += 1
    if n < 2 or n > 4 or n >= len(ln) or ln[n] != " ":
        return 0, ""
    return n, ln[n + 1:].strip()


def _headings(md):
    """Section names (##, ### and ####), in document order — the valid `section=` values.
    Including sub-sections makes zones like 'Expression profiles' / 'Tissue specificity'
    / 'Enriched among interaction partners' discoverable, not just the top-level
    sections. De-duplicated (case-insensitive, first occurrence wins) so the list stays
    clean when a ## and ### share a name."""
    out, seen = [], set()
    for ln in md.splitlines():
        level, name = _heading_level(ln)
        if not level or not name:
            continue
        # Drop the trailing {#anchor} so `section=` values are the readable title.
        name = re.sub(r"\s*\{#[^}]*\}\s*$", "", name).strip()
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
    out, capturing, cap_level = [], False, 0
    for ln in md.splitlines():
        level, _ = _heading_level(ln)
        if capturing:
            if level and level <= cap_level:
                break
            out.append(ln)
            continue
        for lvl in levels:
            if ln.startswith(lvl) and target in ln[len(lvl):].strip().lower():
                capturing, cap_level = True, len(lvl) - 1
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
    results, not_covered, fetch_errors = [], [], []
    for name in entities or []:
        if not name or not name.strip():
            continue
        resolved = _resolve(name)
        # Manifest miss -> type-aware slug + try each type (recovers brand-new pages).
        candidates = resolved if resolved else [(t, _slug_for(t, name)) for t in ATLAS_TYPES]
        hit, errored = None, False
        for (t, slug) in candidates:
            md, canonical, err = _fetch_md(t, slug)
            if md:
                hit = (t, md, canonical)
                break
            if err:
                errored = True
        if not hit:
            # A blocked/failed fetch is reported apart from a genuine "not covered" so a
            # transport problem never masquerades as missing coverage.
            (fetch_errors if errored else not_covered).append(name)
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
            blk = _block(md, section, levels=("## ", "### ", "#### "))
            if blk is not None:
                item["content"] = blk
            else:
                item["content"] = _digest(md)
                item["section_note"] = "section '%s' not found; pick one from sections" % section
        else:
            item["content"] = _digest(md)
        results.append(item)
    _enforce_budget(results)
    out = {"results": results, "not_covered": not_covered}
    if fetch_errors:
        out["fetch_errors"] = fetch_errors
    return out
