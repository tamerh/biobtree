"""
Sugi Variant integration for the biobtree MCP.

Grounds answers about a specific variant in the Sugi Variant reference
(https://sugi.bio/variant), mirroring the Atlas tool: the MCP is a thin client
over the resolver, and every answer cites a canonical URL.

Two corpora are served by the same resolver:
- small variants (ClinVar/AlphaMissense page per variant)
- clinical structural variants from dbVar (`kind: "structural_variant"`)

Policy (see the Sugi Variant request):
- one hit -> fetch the Markdown page and return a digest, one section, or full
- many hits -> return the compact list only (`ambiguous`), never auto-select
  "the pathogenic one"; `include="digests"` may fetch a few summaries
- no hits -> `not_found`; a failed fetch -> `fetch_errors` (distinct)
"""

import json
import logging
import os
import ssl
import urllib.error
import urllib.parse
import urllib.request

from .atlas import _block, _enforce_budget, _headings

logger = logging.getLogger(__name__)

# VARIANT_URL lets the deployment point the fetch at an internal origin, exactly
# like ATLAS_URL. In prod it is the public URL DNS-routed to the host's nginx.
VARIANT_BASE = os.environ.get("VARIANT_URL", "https://sugi.bio/variant").rstrip("/")
_UA = {"User-Agent": "biobtree-mcp"}

# Same Cloudflare Origin CA situation as Atlas: VARIANT_INSECURE_TLS=1 skips
# verification for this host-local hop only.
_SSL_CTX = None
if VARIANT_BASE.startswith("https") and os.environ.get("VARIANT_INSECURE_TLS") == "1":
    _SSL_CTX = ssl.create_default_context()
    _SSL_CTX.check_hostname = False
    _SSL_CTX.verify_mode = ssl.CERT_NONE

MAX_HITS = 25
_MAX_DIGESTS = 5

_AMBIGUOUS_NOTE = (
    "ambiguous: multiple variants match this key. Do not guess and do not "
    "summarize all candidates as if they were one variant; pass slug= / gene= "
    "or ask the user which change they mean."
)


def _localize(url):
    """Point a resolver-returned URL at VARIANT_BASE, preserving its path.
    The resolver builds absolute public URLs; when the deployment fetches through
    an internal origin (VARIANT_URL) that would bypass the local route."""
    if not url:
        return url
    p = urllib.parse.urlparse(url)
    path = p.path or ""
    marker = urllib.parse.urlparse(VARIANT_BASE).path.rstrip("/")
    if marker and path.startswith(marker):
        path = path[len(marker):]
    return VARIANT_BASE + "/" + path.lstrip("/")


def _resolve(query, gene=None, slug=None, limit=MAX_HITS):
    """Call the variant resolver. Returns (data, release, error)."""
    params = {"q": query, "limit": str(limit)}
    if gene:
        params["gene"] = gene
    if slug:
        params["slug"] = slug
    url = VARIANT_BASE + "/resolve.json?" + urllib.parse.urlencode(params)
    try:
        req = urllib.request.Request(url, headers=_UA)
        with urllib.request.urlopen(req, timeout=30, context=_SSL_CTX) as r:
            release = r.headers.get("X-Sugi-Variant-Release", "")
            return json.load(r), release, None
    except urllib.error.HTTPError as e:
        logger.warning("Variant resolve failed %s -> HTTP %s", url, e.code)
        return None, "", "HTTP %s" % e.code
    except Exception as e:
        logger.warning("Variant resolve failed %s: %s", url, e)
        return None, "", str(e)


def _fetch_md(url):
    """Return (body, canonical_url, error); 404 is a clean miss, not an error."""
    url = _localize(url)
    try:
        req = urllib.request.Request(url, headers=_UA)
        with urllib.request.urlopen(req, timeout=30, context=_SSL_CTX) as r:
            if r.status == 200:
                body = r.read().decode("utf-8", "replace")
                return body, _localize(url[:-3] if url.endswith(".md") else url), None
            return None, None, "HTTP %s" % r.status
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return None, None, None
        logger.warning("Variant md fetch blocked/failed %s -> HTTP %s", url, e.code)
        return None, None, "HTTP %s" % e.code
    except Exception as e:
        logger.warning("Variant md fetch failed %s: %s", url, e)
        return None, None, str(e)


def _digest(md):
    """Compact digest: title/URL/release header + Summary (small variants) or
    Identity (both corpora). Structural pages have no Summary section."""
    parts = []
    for ln in md.splitlines():
        if ln.startswith("## "):
            break
        parts.append(ln)
    head = "\n".join(parts).strip()
    md_parts = [head]
    for name in ("Summary", "Identity", "Identifiers"):
        b = _block(md, name, levels=("## ",))
        if b:
            md_parts.append(b)
    return "\n\n".join(p for p in md_parts if p).strip()


def _compact_hit(h):
    """One ambiguity candidate, reduced to the fields a model needs to choose."""
    keep = (
        "kind", "slug", "gene", "hgvs_p", "hgvs_c", "classification",
        "review_status", "rsid", "coordinate", "variation_id",
        "nssv", "nsv", "chromosome", "start", "end", "svtype", "significance",
    )
    out = {k: h[k] for k in keep if h.get(k) not in (None, "")}
    out["canonical_url"] = h.get("canonical_url")
    return out


def variant_lookup(entities, gene=None, slug=None, section=None, full=False,
                   include="none"):
    """Resolve each query and return grounding content for a variant.

    Returns:
      results:     [{entity, kind, slug, canonical_url, release, content, sections}]
      ambiguous:   [{query, ambiguous: True, kind, release, hits, note[, digests]}]
      not_found:   queries with no hit (genuine coverage gap)
      fetch_errors: queries whose resolver/page fetch failed (transport problem)
    """
    results, ambiguous, not_found, fetch_errors = [], [], [], []
    want_digests = str(include or "none").lower() == "digests"

    for name in entities or []:
        if not name or not name.strip():
            continue
        query = name.strip()
        data, release, err = _resolve(query, gene=gene, slug=slug)
        if err:
            fetch_errors.append({"query": query, "error": err})
            continue
        if not data or data.get("not_found") or not data.get("hits"):
            not_found.append(query)
            continue

        hits = data["hits"]
        if len(hits) == 1 and not data.get("ambiguous"):
            hit = hits[0]
            md_url = hit.get("markdown_url") or ""
            if not md_url:
                fetch_errors.append({"query": query, "error": "resolver returned no markdown_url"})
                continue
            md, canonical, ferr = _fetch_md(md_url)
            if ferr:
                fetch_errors.append({"query": query, "error": ferr})
                continue
            if not md:
                not_found.append(query)
                continue
            item = {
                "entity": query,
                "kind": hit.get("kind", ""),
                "slug": hit.get("slug") or hit.get("nssv", ""),
                "canonical_url": hit.get("canonical_url") or canonical,
                "release": release,
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
                    item["section_note"] = (
                        "section '%s' not found; pick one from sections" % section
                    )
            else:
                item["content"] = _digest(md)
            results.append(item)
            continue

        entry = {
            "query": query,
            "ambiguous": True,
            "kind": data.get("kind", ""),
            "release": release,
            "hits": [_compact_hit(h) for h in hits],
            "note": _AMBIGUOUS_NOTE,
        }
        if want_digests:
            digests = []
            for h in hits[:_MAX_DIGESTS]:
                md_url = h.get("markdown_url") or ""
                if not md_url:
                    continue
                md, _canonical, ferr = _fetch_md(md_url)
                if ferr or not md:
                    continue
                digests.append({
                    "label": h.get("gene") or h.get("nssv") or h.get("slug", ""),
                    "canonical_url": h.get("canonical_url"),
                    "content": _digest(md),
                })
            entry["digests"] = digests
        ambiguous.append(entry)

    _enforce_budget(results)
    out = {"results": results, "ambiguous": ambiguous, "not_found": not_found}
    if fetch_errors:
        out["fetch_errors"] = fetch_errors
    return out
