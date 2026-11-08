#!/usr/bin/env python3
"""Unit tests for MCP server helpers (no network, no MCP runtime).

Covers the pieces added for token-efficient tool results and the new
enrichment tool:

- compact JSON serialization
- structural truncation at element boundaries (B3)
- Atlas Markdown section parsing (##/###/####, anchors, budget trimming)
- biobtree_enrich registration and dispatch to the API client

The real `mcp` package and `httpx` are not required: they are stubbed before
importing `mcp_srv.tools`, so this runs on a bare Python install.
"""

import asyncio
import json
import sys
import types
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parent.parent
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

# --- stub optional runtime deps ------------------------------------------------
_mcp = types.ModuleType("mcp")
_mcp_types = types.ModuleType("mcp.types")


class _Tool:
    def __init__(self, **kwargs):
        self.name = kwargs.get("name")
        self.description = kwargs.get("description")
        self.inputSchema = kwargs.get("inputSchema")


_mcp_types.Tool = _Tool
sys.modules.setdefault("mcp", _mcp)
sys.modules.setdefault("mcp.types", _mcp_types)

_client_mod = types.ModuleType("mcp_srv.biobtree_client")


class _BiobtreeClient:  # pragma: no cover - only used for type imports
    pass


class _BiobtreeError(Exception):
    pass


_client_mod.BiobtreeClient = _BiobtreeClient
_client_mod.BiobtreeError = _BiobtreeError
sys.modules.setdefault("mcp_srv.biobtree_client", _client_mod)

from mcp_srv import atlas  # noqa: E402
from mcp_srv import variant  # noqa: E402
from mcp_srv import tools  # noqa: E402
from mcp_srv.prompts import INPUT_SCHEMAS  # noqa: E402


class TestCompactAndTruncate(unittest.TestCase):
    def test_compact_json_has_no_whitespace(self):
        out = tools._compact_json({"a": 1, "b": [1, 2]})
        self.assertEqual(out, '{"a":1,"b":[1,2]}')

    def test_small_result_is_unchanged(self):
        payload = {"data": ["a", "b"], "stats": {"total": 2}}
        self.assertEqual(tools._truncate_result(payload, 10000), tools._compact_json(payload))

    def test_large_dict_truncates_at_element_boundary(self):
        payload = {
            "context": {"query": ">>hgnc>>msigdb"},
            "stats": {"total": 1000},
            "pagination": {"has_next": True, "next_token": "tok"},
            "data": ["x" * 50 for _ in range(1000)],
        }
        out = tools._truncate_result(payload, 2000)
        self.assertLessEqual(len(out), 2000)
        parsed = json.loads(out)  # must stay valid JSON
        self.assertTrue(parsed["truncated"])
        self.assertEqual(parsed["total"], 1000)
        self.assertLess(parsed["returned"], 1000)
        self.assertEqual(len(parsed["data"]), parsed["returned"])
        # The service token covers the untruncated page, so it must be dropped
        # rather than let the caller skip the elements we cut.
        self.assertFalse(parsed["pagination"]["has_next"])
        self.assertNotIn("next_token", parsed["pagination"])
        self.assertTrue(parsed["pagination"]["invalidated_by_truncation"])
        self.assertEqual(parsed["stats"]["total"], 1000)

    def test_truncation_reports_dropped_map_inputs(self):
        mappings = [
            {"input": f"GENE{i}", "source": f"HGNC:{i}|g{i}", "targets": ["t" * 60] * 80}
            for i in range(20)
        ]
        payload = {
            "stats": {"queried": 20, "total": 1600, "mapped": 20},
            "pagination": {"has_next": True, "next_token": "tok"},
            "mappings": mappings,
        }
        parsed = json.loads(tools._truncate_result(payload, 4000))
        self.assertTrue(parsed["truncated"])
        self.assertLess(parsed["returned"], 20)
        self.assertEqual(len(parsed["dropped_inputs"]), 20 - parsed["returned"])
        self.assertIn("GENE19", parsed["dropped_inputs"])

    def test_large_list_is_wrapped_in_data(self):
        out = tools._truncate_result(["y" * 40 for _ in range(500)], 1000)
        parsed = json.loads(out)
        self.assertTrue(parsed["truncated"])
        self.assertEqual(parsed["total"], 500)
        self.assertLess(parsed["returned"], 500)

    def test_dict_without_list_payload_returns_error_envelope(self):
        out = tools._truncate_result({"blob": "z" * 5000}, 500)
        parsed = json.loads(out)
        self.assertTrue(parsed["truncated"])
        self.assertEqual(parsed["returned"], 0)
        self.assertIn("error", parsed)


class TestAtlasSections(unittest.TestCase):
    MD = """# TP53 {#tp53}

Canonical: TP53

## Summary

Tumor suppressor.

## Identifiers {#ids}

HGNC:11998

### Expression profiles

High in all tissues.

#### Enriched among interaction partners

DNA damage response.

## Disease & clinical

Li-Fraumeni.
"""

    def test_heading_levels(self):
        self.assertEqual(atlas._heading_level("## Summary"), (2, "Summary"))
        self.assertEqual(atlas._heading_level("### Sub"), (3, "Sub"))
        self.assertEqual(atlas._heading_level("#### Deep"), (4, "Deep"))
        self.assertEqual(atlas._heading_level("# Title"), (0, ""))
        self.assertEqual(atlas._heading_level("##### Too deep"), (0, ""))
        self.assertEqual(atlas._heading_level("##NoSpace"), (0, ""))

    def test_headings_include_level_four_and_strip_anchors(self):
        heads = atlas._headings(self.MD)
        self.assertIn("Summary", heads)
        self.assertIn("Identifiers", heads)
        self.assertIn("Expression profiles", heads)
        self.assertIn("Enriched among interaction partners", heads)
        self.assertNotIn("Identifiers {#ids}", heads)
        self.assertNotIn("TP53 {#tp53}", heads)

    def test_headings_deduplicate_case_insensitively(self):
        md = "## Summary\ntext\n### summary\nmore\n"
        self.assertEqual(atlas._headings(md), ["Summary"])

    def test_block_level_four(self):
        blk = atlas._block(self.MD, "Enriched among interaction partners",
                           levels=("## ", "### ", "#### "))
        self.assertIsNotNone(blk)
        self.assertIn("DNA damage response.", blk)
        self.assertNotIn("Li-Fraumeni", blk)
        self.assertNotIn("High in all tissues", blk)

    def test_block_stops_at_same_level(self):
        blk = atlas._block(self.MD, "Summary")
        self.assertIn("Tumor suppressor.", blk)
        self.assertNotIn("HGNC:11998", blk)

    def test_digest_contains_header_summary_and_identifiers(self):
        digest = atlas._digest(self.MD)
        self.assertIn("TP53", digest)
        self.assertIn("Tumor suppressor.", digest)
        self.assertIn("HGNC:11998", digest)
        self.assertNotIn("Li-Fraumeni", digest)

    def test_budget_trims_oversized_sections(self):
        results = [
            {"entity": "A", "content": "a" * 40000},
            {"entity": "B", "content": "b" * 40000},
        ]
        atlas._enforce_budget(results)
        self.assertTrue(all(r.get("trimmed") for r in results))
        self.assertTrue(all("trimmed to fit" in r["content"] for r in results))
        self.assertLess(sum(len(r["content"]) for r in results), 50000)

    def test_budget_noop_when_small(self):
        results = [{"entity": "A", "content": "short"}]
        atlas._enforce_budget(results)
        self.assertNotIn("trimmed", results[0])


class TestEnrichTool(unittest.TestCase):
    def test_tool_registered_for_mcp_and_chat(self):
        self.assertIn("biobtree_enrich", [t.name for t in tools.MCP_TOOLS])
        self.assertIn(
            "biobtree_enrich",
            [t["function"]["name"] for t in tools.CHAT_TOOLS],
        )
        self.assertIn("biobtree_enrich", INPUT_SCHEMAS)
        self.assertEqual(INPUT_SCHEMAS["biobtree_enrich"]["required"], ["genes"])

    def test_execute_tool_dispatches_to_client_enrich(self):
        calls = {}

        class FakeClient:
            async def enrich(self, genes, collection="C5:GO", limit=20):
                calls.update(genes=genes, collection=collection, limit=limit)
                return {
                    "query": {"mapped_genes": len(genes)},
                    "results": [{"id": "M1", "name": "GOBP_X", "fold": 10.0, "fdr": 1e-5}],
                    "tested_sets": 1,
                }

        out = asyncio.run(
            tools.execute_tool(
                "biobtree_enrich",
                {"genes": ["TP53", "BRCA1"], "collection": "C5:GO:BP", "limit": 3},
                FakeClient(),
                max_result_length=50000,
            )
        )
        parsed = json.loads(out)
        self.assertEqual(calls, {"genes": ["TP53", "BRCA1"], "collection": "C5:GO:BP", "limit": 3})
        self.assertEqual(parsed["results"][0]["name"], "GOBP_X")
        self.assertEqual(parsed["query"]["mapped_genes"], 2)

    def test_execute_tool_accepts_comma_separated_genes(self):
        calls = {}

        class FakeClient:
            async def enrich(self, genes, collection="C5:GO", limit=20):
                calls["genes"] = genes
                return {"query": {}, "results": [], "tested_sets": 0}

        asyncio.run(
            tools.execute_tool(
                "biobtree_enrich",
                {"genes": "TP53, BRCA1"},
                FakeClient(),
                max_result_length=50000,
            )
        )
        self.assertEqual(calls["genes"], ["TP53", "BRCA1"])


class TestVariantTool(unittest.TestCase):
    MD = """# NM_000314.8(PTEN):c.389G>A (p.Arg130Gln)

https://sugi.bio/variant/pten-p-arg130gln

_Release v1.6.0_

## Summary

Pathogenic in PTEN.

## Identity {#identity}

rs121909229, 10:87933148:G:A

## Clinical significance

PTEN hamartoma tumor syndrome.
"""

    def test_tool_registered_for_mcp_and_chat(self):
        self.assertIn("biobtree_variant", [t.name for t in tools.MCP_TOOLS])
        self.assertIn(
            "biobtree_variant",
            [t["function"]["name"] for t in tools.CHAT_TOOLS],
        )
        self.assertIn("biobtree_variant", INPUT_SCHEMAS)
        self.assertEqual(INPUT_SCHEMAS["biobtree_variant"]["required"], ["entities"])

    def test_digest_is_summary_plus_identity(self):
        d = variant._digest(self.MD)
        self.assertIn("PTEN", d)
        self.assertIn("Pathogenic in PTEN", d)
        self.assertIn("rs121909229", d)
        self.assertNotIn("hamartoma", d)

    def test_compact_hit_drops_noise(self):
        hit = {
            "kind": "variant", "slug": "pten-p-arg130gln", "gene": "PTEN",
            "hgvs_p": "p.Arg130Gln", "classification": "Pathogenic",
            "canonical_url": "https://sugi.bio/variant/pten-p-arg130gln",
            "markdown_url": "https://sugi.bio/variant/pten-p-arg130gln.md",
            "cached": True,
        }
        out = variant._compact_hit(hit)
        self.assertEqual(out["slug"], "pten-p-arg130gln")
        self.assertEqual(out["canonical_url"], hit["canonical_url"])
        self.assertNotIn("markdown_url", out)
        self.assertNotIn("cached", out)

    def _patch(self, data, md=None, section_of=None):
        class Ctx:
            def __enter__(self_inner):
                self_inner.resolve = mock.patch.object(
                    variant, "_resolve", return_value=(data, "v1.6.0", None)
                )
                self_inner.fetch = mock.patch.object(
                    variant, "_fetch_md",
                    side_effect=lambda url: (md, url[:-3], None),
                )
                self_inner.resolve.start()
                self_inner.fetch.start()
                return self_inner

            def __exit__(self_inner, *exc):
                self_inner.fetch.stop()
                self_inner.resolve.stop()
                return False

        return Ctx()

    def test_single_hit_returns_digest_and_sections(self):
        data = {"ambiguous": False, "hits": [{
            "kind": "variant", "slug": "pten-p-arg130gln",
            "canonical_url": "https://sugi.bio/variant/pten-p-arg130gln",
            "markdown_url": "https://sugi.bio/variant/pten-p-arg130gln.md",
        }]}
        with self._patch(data, md=self.MD):
            out = variant.variant_lookup(["PTEN R130Q"])
        self.assertEqual(len(out["results"]), 1)
        item = out["results"][0]
        self.assertIn("Pathogenic in PTEN", item["content"])
        self.assertIn("Identity", item["sections"])
        self.assertEqual(out["ambiguous"], [])
        self.assertEqual(out["not_found"], [])

    def test_ambiguous_returns_compact_list_without_pages(self):
        hits = [
            {"kind": "variant", "slug": "pten-p-arg130gln", "gene": "PTEN",
             "canonical_url": "https://sugi.bio/variant/pten-p-arg130gln",
             "markdown_url": "https://sugi.bio/variant/pten-p-arg130gln.md"},
            {"kind": "variant", "slug": "pten-p-arg130leu", "gene": "PTEN",
             "canonical_url": "https://sugi.bio/variant/pten-p-arg130leu",
             "markdown_url": "https://sugi.bio/variant/pten-p-arg130leu.md"},
        ]
        data = {"ambiguous": True, "hits": hits}
        with mock.patch.object(variant, "_resolve", return_value=(data, "v1.6.0", None)), \
                mock.patch.object(variant, "_fetch_md") as fetch:
            out = variant.variant_lookup(["rs121909229"])
            fetch.assert_not_called()
        self.assertEqual(out["results"], [])
        self.assertEqual(len(out["ambiguous"]), 1)
        entry = out["ambiguous"][0]
        self.assertTrue(entry["ambiguous"])
        self.assertEqual(len(entry["hits"]), 2)
        self.assertIn("Do not guess", entry["note"])

    def test_ambiguous_include_digests_fetches_limited_pages(self):
        hits = [
            {"kind": "variant", "slug": "v%d" % i, "gene": "G%d" % i,
             "markdown_url": "https://sugi.bio/variant/v%d.md" % i}
            for i in range(8)
        ]
        data = {"ambiguous": True, "hits": hits}
        with self._patch(data, md=self.MD):
            out = variant.variant_lookup(["rsX"], include="digests")
        self.assertEqual(len(out["ambiguous"][0]["digests"]), variant._MAX_DIGESTS)

    def test_not_found_is_distinct_from_fetch_error(self):
        with mock.patch.object(variant, "_resolve", return_value=(None, "", None)):
            out = variant.variant_lookup(["unknown"])
        self.assertEqual(out["not_found"], ["unknown"])
        self.assertNotIn("fetch_errors", out)

        with mock.patch.object(variant, "_resolve", return_value=(None, "", "HTTP 503")):
            out = variant.variant_lookup(["flaky"])
        self.assertEqual(out["not_found"], [])
        self.assertEqual(out["fetch_errors"][0]["query"], "flaky")

    def test_section_selects_one_zone(self):
        data = {"ambiguous": False, "hits": [{
            "kind": "variant", "slug": "pten-p-arg130gln",
            "canonical_url": "https://sugi.bio/variant/pten-p-arg130gln",
            "markdown_url": "https://sugi.bio/variant/pten-p-arg130gln.md",
        }]}
        with self._patch(data, md=self.MD):
            out = variant.variant_lookup(["PTEN R130Q"], section="Clinical significance")
        self.assertIn("hamartoma", out["results"][0]["content"])
        self.assertNotIn("rs121909229", out["results"][0]["content"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
