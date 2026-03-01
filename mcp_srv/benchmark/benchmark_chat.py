#!/usr/bin/env python3
"""
Biobtree Chat Benchmark Script

Benchmarks the /chat endpoint with multiple models and questions.
Compares responses with and without biobtree tools, with auto-scoring
against ground truth.

Usage:
    python tests/benchmark_chat.py
    python tests/benchmark_chat.py --server http://localhost:8000
    python tests/benchmark_chat.py --models anthropic/claude-3-haiku openai/gpt-4o-mini
    python tests/benchmark_chat.py --tier 1
    python tests/benchmark_chat.py --tier 2 --models anthropic/claude-sonnet-4
    python tests/benchmark_chat.py --questions-file tests/questions.json
    python tests/benchmark_chat.py --results-dir my_results
"""

import argparse
import json
import os
import time
from datetime import datetime
from pathlib import Path
from typing import Optional

import httpx

# =============================================================================
# Configuration
# =============================================================================

DEFAULT_SERVER = "http://localhost:8000"
QUESTIONS_FILE = Path(__file__).parent / "questions.json"
RESULTS_DIR = Path(__file__).parent / "results"

# Models to benchmark (OpenRouter model IDs)
DEFAULT_MODELS = [
    "anthropic/claude-3-haiku",      # Fast, cheap
    "anthropic/claude-sonnet-4",     # Balanced
    # "openai/gpt-4o-mini",          # Fast, cheap
    # "openai/gpt-4o",               # High quality
    # "meta-llama/llama-3.3-70b-instruct",  # Open source
]

# Fallback questions if questions.json not found
FALLBACK_QUESTIONS = [
    {
        "id": "gene_protein_simple",
        "question": "What protein does the TP53 gene encode?",
        "category": "gene_to_protein",
        "tier": 1,
        "hops": 2,
        "why_llm_fails": "LLM knows the answer partially but may miss UniProt details.",
        "ground_truth": {
            "must_mention": ["TP53", "P04637"],
            "must_mention_any": ["tumor", "suppressor", "protein"],
            "expected_tools": ["biobtree_map"],
            "min_tool_calls": 1
        }
    },
    {
        "id": "drug_target",
        "question": "What are the protein targets of imatinib?",
        "category": "drug_to_target",
        "tier": 1,
        "hops": 2,
        "why_llm_fails": "LLM knows ABL1/BCR-ABL but may miss other targets in ChEMBL.",
        "ground_truth": {
            "must_mention": ["imatinib"],
            "must_mention_any": ["ABL", "KIT", "PDGFR", "target"],
            "expected_tools": ["biobtree_search", "biobtree_map"],
            "min_tool_calls": 1
        }
    },
]


def load_questions(questions_file: Path) -> list:
    """Load questions from external JSON file, fall back to hardcoded."""
    if questions_file.exists():
        with open(questions_file) as f:
            data = json.load(f)
        questions = data.get("questions", [])
        print(f"Loaded {len(questions)} questions from {questions_file}")
        return questions
    else:
        print(f"Warning: {questions_file} not found, using {len(FALLBACK_QUESTIONS)} fallback questions")
        return FALLBACK_QUESTIONS


# =============================================================================
# Benchmark Data (Split File Format)
# =============================================================================

def load_questions_registry(questions_file: Path = QUESTIONS_FILE) -> dict:
    """Load questions registry from JSON file."""
    if questions_file.exists():
        with open(questions_file) as f:
            return json.load(f)
    else:
        return {"metadata": {}, "questions": []}


def get_question_by_num(registry: dict, num: int) -> Optional[dict]:
    """Get question from registry by numeric ID."""
    for q in registry.get("questions", []):
        if q.get("id") == num:
            return q
    return None


def load_results(question_num: int, results_dir: Path = RESULTS_DIR) -> dict:
    """Load results for a specific question."""
    results_file = results_dir / f"{question_num}.json"
    if results_file.exists():
        with open(results_file) as f:
            return json.load(f)
    else:
        return {"id": question_num, "results": {}, "reviews": {}}


def save_results(question_num: int, results: dict, results_dir: Path = RESULTS_DIR):
    """Save results for a specific question."""
    results_dir.mkdir(parents=True, exist_ok=True)
    results_file = results_dir / f"{question_num}.json"
    with open(results_file, "w") as f:
        json.dump(results, f, indent=2)
    print(f"Saved results to: {results_file}")


def get_model_key(model_id: str) -> str:
    """Convert OpenRouter model ID to result key."""
    # e.g., "anthropic/claude-sonnet-4" -> "biobtree_sonnet"
    # e.g., "openai/gpt-4.1" -> "biobtree_gpt41"
    model_map = {
        "anthropic/claude-sonnet-4": "biobtree_sonnet",
        "openai/gpt-4.1": "biobtree_gpt41",
        "google/gemini-2.5-pro-preview-03-25": "biobtree_gemini25",
    }
    return model_map.get(model_id, model_id.replace("/", "_").replace("-", "_").replace(".", ""))


def find_question_by_id(data: dict, question_id: str) -> Optional[dict]:
    """Find a question in benchmark data by its ID."""
    for q in data.get("questions", []):
        if q.get("id") == question_id:
            return q
    return None


# =============================================================================
# Scoring
# =============================================================================

def score_response(answer: str, ground_truth: dict, tool_calls: list = None) -> dict:
    """
    Score a response against ground truth using keyword matching.

    Three scoring dimensions:
    - must_mention: all required terms must appear (coverage fraction)
    - must_mention_any: at least one alternative term must appear (0 or 1)
    - tool_only_terms: database-specific IDs that only tool-augmented answers contain (0 or 1)

    Final score = average of all applicable dimensions.
    """
    if not answer:
        return {
            "score": 0.0,
            "must_mention_found": [],
            "must_mention_missing": ground_truth.get("must_mention", []),
            "any_mention_found": [],
            "evidence_found": [],
            "tool_calls_count": 0,
            "expected_min_tools": ground_truth.get("min_tool_calls", 0),
            "used_tools": False,
            "hallucination_risk": True
        }

    answer_upper = answer.upper()
    tool_calls = tool_calls or []

    must = ground_truth.get("must_mention", [])
    must_any = ground_truth.get("must_mention_any", [])
    tool_only = ground_truth.get("tool_only_terms", [])
    min_tools = ground_truth.get("min_tool_calls", 0)

    must_found = [t for t in must if t.upper() in answer_upper]
    must_missing = [t for t in must if t.upper() not in answer_upper]
    any_found = [t for t in must_any if t.upper() in answer_upper]
    evidence_found = [t for t in tool_only if t.upper() in answer_upper]

    # Score components
    must_score = len(must_found) / len(must) if must else 1.0
    any_score = 1.0 if any_found or not must_any else 0.0
    evidence_score = 1.0 if evidence_found or not tool_only else 0.0

    # Combined score: average of all applicable dimensions
    if tool_only:
        score = (must_score + any_score + evidence_score) / 3.0
    else:
        score = (must_score + any_score) / 2.0

    used_tools = len(tool_calls) > 0

    return {
        "score": round(score, 3),
        "must_mention_found": must_found,
        "must_mention_missing": must_missing,
        "any_mention_found": any_found,
        "evidence_found": evidence_found,
        "tool_calls_count": len(tool_calls),
        "expected_min_tools": min_tools,
        "used_tools": used_tools,
        "hallucination_risk": not used_tools and must_score < 0.5
    }


# =============================================================================
# Benchmark Runner
# =============================================================================

class BenchmarkRunner:
    def __init__(self, server_url: str, timeout: float = 120.0):
        self.server_url = server_url.rstrip("/")
        self.timeout = timeout
        self.client = httpx.Client(timeout=timeout)

    def run_query(
        self,
        question: str,
        model: str,
        with_tools: bool = True
    ) -> dict:
        """Run a single query and return results."""
        start_time = time.time()

        try:
            response = self.client.post(
                f"{self.server_url}/chat",
                json={
                    "question": question,
                    "model": model,
                    "with_tools": with_tools
                }
            )
            elapsed = time.time() - start_time

            if response.status_code != 200:
                return {
                    "success": False,
                    "error": f"HTTP {response.status_code}: {response.text[:200]}",
                    "elapsed_seconds": elapsed
                }

            data = response.json()

            if "error" in data:
                return {
                    "success": False,
                    "error": data["error"],
                    "elapsed_seconds": elapsed
                }

            return {
                "success": True,
                "answer": data.get("answer", ""),
                "model": data.get("model"),
                "tools_used": data.get("tools_used"),
                "tool_calls": data.get("tool_calls", []),
                "iterations": data.get("iterations", 0),
                "elapsed_seconds": elapsed,
                "answer_length": len(data.get("answer", "")),
                "usage": data.get("usage", {})
            }

        except httpx.TimeoutException:
            return {
                "success": False,
                "error": f"Timeout after {self.timeout}s",
                "elapsed_seconds": self.timeout
            }
        except Exception as e:
            return {
                "success": False,
                "error": str(e),
                "elapsed_seconds": time.time() - start_time
            }

    def benchmark_question(
        self,
        question_data: dict,
        models: list,
        include_baseline: bool = True
    ) -> dict:
        """Benchmark a single question across multiple models."""
        ground_truth = question_data.get("ground_truth", {})

        results = {
            "question_id": question_data["id"],
            "question": question_data["question"],
            "category": question_data.get("category", ""),
            "tier": question_data.get("tier", 0),
            "hops": question_data.get("hops", 0),
            "why_llm_fails": question_data.get("why_llm_fails", ""),
            "models": {}
        }

        for model in models:
            model_results = {}

            # With tools
            print(f"    Testing {model} with tools...", end=" ", flush=True)
            model_results["with_tools"] = self.run_query(
                question_data["question"],
                model,
                with_tools=True
            )
            wt = model_results["with_tools"]
            status = "+" if wt["success"] else "x"
            elapsed = wt["elapsed_seconds"]

            # Score with-tools response
            if wt["success"]:
                wt["scoring"] = score_response(
                    wt.get("answer", ""),
                    ground_truth,
                    wt.get("tool_calls", [])
                )
                score_str = f" score={wt['scoring']['score']:.2f}"
            else:
                wt["scoring"] = {"score": 0.0, "used_tools": False, "hallucination_risk": True}
                score_str = " FAILED"

            print(f"{status} ({elapsed:.1f}s{score_str})")

            # Without tools (baseline)
            if include_baseline:
                print(f"    Testing {model} without tools...", end=" ", flush=True)
                model_results["without_tools"] = self.run_query(
                    question_data["question"],
                    model,
                    with_tools=False
                )
                wot = model_results["without_tools"]
                status = "+" if wot["success"] else "x"
                elapsed = wot["elapsed_seconds"]

                # Score without-tools response
                if wot["success"]:
                    wot["scoring"] = score_response(
                        wot.get("answer", ""),
                        ground_truth,
                        []  # no tool calls in baseline
                    )
                    score_str = f" score={wot['scoring']['score']:.2f}"
                else:
                    wot["scoring"] = {"score": 0.0, "used_tools": False, "hallucination_risk": True}
                    score_str = " FAILED"

                print(f"{status} ({elapsed:.1f}s{score_str})")

            results["models"][model] = model_results

        return results

    def run_benchmark(
        self,
        models: list,
        questions: list,
        include_baseline: bool = True
    ) -> dict:
        """Run full benchmark suite."""
        print(f"\n{'='*60}")
        print("Biobtree Chat Benchmark")
        print(f"{'='*60}")
        print(f"Server: {self.server_url}")
        print(f"Models: {', '.join(models)}")
        print(f"Questions: {len(questions)}")
        print(f"Include baseline: {include_baseline}")
        tiers = sorted(set(q.get("tier", 0) for q in questions))
        print(f"Tiers: {tiers}")
        print(f"{'='*60}\n")

        results = {
            "metadata": {
                "server": self.server_url,
                "models": models,
                "timestamp": datetime.now().isoformat(),
                "include_baseline": include_baseline,
                "total_questions": len(questions),
                "tiers": tiers
            },
            "questions": []
        }

        for i, q in enumerate(questions, 1):
            tier_str = f"T{q.get('tier', '?')}"
            hops_str = f"{q.get('hops', '?')}h"
            print(f"[{i}/{len(questions)}] [{tier_str}/{hops_str}] {q['id']}: {q['question'][:60]}...")
            q_results = self.benchmark_question(q, models, include_baseline)
            results["questions"].append(q_results)
            print()

        # Calculate summary statistics
        results["summary"] = self._calculate_summary(results)

        return results

    def _calculate_summary(self, results: dict) -> dict:
        """Calculate summary statistics with scoring."""
        summary = {"models": {}}

        for model in results["metadata"]["models"]:
            model_stats = {
                "with_tools": {
                    "success_count": 0,
                    "total_time": 0,
                    "total_tool_calls": 0,
                    "total_iterations": 0,
                    "total_score": 0.0,
                    "hallucination_count": 0
                },
                "without_tools": {
                    "success_count": 0,
                    "total_time": 0,
                    "total_score": 0.0,
                    "hallucination_count": 0
                }
            }

            for q in results["questions"]:
                if model in q["models"]:
                    m = q["models"][model]

                    if "with_tools" in m:
                        wt = m["with_tools"]
                        if wt.get("success"):
                            model_stats["with_tools"]["success_count"] += 1
                            model_stats["with_tools"]["total_time"] += wt["elapsed_seconds"]
                            model_stats["with_tools"]["total_tool_calls"] += len(wt.get("tool_calls", []))
                            model_stats["with_tools"]["total_iterations"] += wt.get("iterations", 0)
                        scoring = wt.get("scoring", {})
                        model_stats["with_tools"]["total_score"] += scoring.get("score", 0.0)
                        if scoring.get("hallucination_risk"):
                            model_stats["with_tools"]["hallucination_count"] += 1

                    if "without_tools" in m:
                        wot = m["without_tools"]
                        if wot.get("success"):
                            model_stats["without_tools"]["success_count"] += 1
                            model_stats["without_tools"]["total_time"] += wot["elapsed_seconds"]
                        scoring = wot.get("scoring", {})
                        model_stats["without_tools"]["total_score"] += scoring.get("score", 0.0)
                        if scoring.get("hallucination_risk"):
                            model_stats["without_tools"]["hallucination_count"] += 1

            n_questions = len(results["questions"])
            wt = model_stats["with_tools"]
            wot = model_stats["without_tools"]

            summary["models"][model] = {
                "with_tools": {
                    "success_rate": wt["success_count"] / n_questions if n_questions > 0 else 0,
                    "avg_score": wt["total_score"] / n_questions if n_questions > 0 else 0,
                    "avg_time": wt["total_time"] / wt["success_count"] if wt["success_count"] > 0 else 0,
                    "avg_tool_calls": wt["total_tool_calls"] / wt["success_count"] if wt["success_count"] > 0 else 0,
                    "avg_iterations": wt["total_iterations"] / wt["success_count"] if wt["success_count"] > 0 else 0,
                    "hallucination_count": wt["hallucination_count"]
                },
                "without_tools": {
                    "success_rate": wot["success_count"] / n_questions if n_questions > 0 else 0,
                    "avg_score": wot["total_score"] / n_questions if n_questions > 0 else 0,
                    "avg_time": wot["total_time"] / wot["success_count"] if wot["success_count"] > 0 else 0,
                    "hallucination_count": wot["hallucination_count"]
                }
            }

        return summary

    def print_summary(self, results: dict):
        """Print a formatted summary with scores."""
        print(f"\n{'='*70}")
        print("BENCHMARK SUMMARY")
        print(f"{'='*70}\n")

        summary = results["summary"]

        # With tools table
        print("WITH TOOLS (LLM + Biobtree):")
        print(f"{'Model':<30} {'OK%':<8} {'Score':<8} {'Time':<8} {'Tools':<8} {'Halluc':<8}")
        print("-" * 70)

        for model, stats in summary["models"].items():
            wt = stats["with_tools"]
            model_short = model.split("/")[-1][:28]
            print(f"{model_short:<30} {wt['success_rate']*100:>5.0f}% {wt['avg_score']:>5.2f} {wt['avg_time']:>6.1f}s {wt['avg_tool_calls']:>5.1f} {wt['hallucination_count']:>5d}")

        # Baseline comparison
        if results["metadata"]["include_baseline"]:
            print(f"\nWITHOUT TOOLS (LLM only):")
            print(f"{'Model':<30} {'OK%':<8} {'Score':<8} {'Time':<8} {'Halluc':<8}")
            print("-" * 62)
            for model, stats in summary["models"].items():
                wot = stats["without_tools"]
                model_short = model.split("/")[-1][:28]
                print(f"{model_short:<30} {wot['success_rate']*100:>5.0f}% {wot['avg_score']:>5.2f} {wot['avg_time']:>6.1f}s {wot['hallucination_count']:>5d}")

        # Score improvement
        print(f"\nSCORE IMPROVEMENT (with tools - without tools):")
        print(f"{'Model':<30} {'Delta':<10}")
        print("-" * 40)
        for model, stats in summary["models"].items():
            wt_score = stats["with_tools"]["avg_score"]
            wot_score = stats["without_tools"]["avg_score"]
            delta = wt_score - wot_score
            model_short = model.split("/")[-1][:28]
            sign = "+" if delta >= 0 else ""
            print(f"{model_short:<30} {sign}{delta:>.3f}")

        print()


# =============================================================================
# Results Output
# =============================================================================

def save_legacy_results(results: dict, results_dir: Path):
    """Save results to timestamped directory with raw JSON and summary markdown (legacy format)."""
    results_dir.mkdir(parents=True, exist_ok=True)

    # Save raw results
    raw_path = results_dir / "raw_responses.json"
    raw_path.write_text(json.dumps(results, indent=2))
    print(f"Raw results saved to: {raw_path}")

    # Generate and save summary markdown
    md = generate_summary_md(results)
    md_path = results_dir / "summary.md"
    md_path.write_text(md)
    print(f"Summary saved to: {md_path}")


def generate_summary_md(results: dict) -> str:
    """Generate a paper-ready markdown summary table."""
    meta = results["metadata"]
    summary = results["summary"]
    timestamp = meta["timestamp"][:19]

    lines = []
    lines.append(f"# Biobtree Benchmark Results — {timestamp}")
    lines.append("")
    lines.append(f"**Server:** {meta['server']}  ")
    lines.append(f"**Models:** {', '.join(meta['models'])}  ")
    lines.append(f"**Questions:** {meta['total_questions']}  ")
    lines.append(f"**Tiers:** {meta.get('tiers', 'all')}  ")
    lines.append("")

    # Per-question comparison table
    lines.append("## Per-Question Results")
    lines.append("")

    # Build header
    model_names = meta["models"]
    header_parts = ["#", "Question", "Tier", "Hops"]
    for m in model_names:
        short = m.split("/")[-1][:20]
        header_parts.append(f"+Tools ({short})")
        if meta.get("include_baseline", True):
            header_parts.append(f"-Tools ({short})")
        header_parts.append(f"Calls ({short})")
    lines.append("| " + " | ".join(header_parts) + " |")
    lines.append("| " + " | ".join(["---"] * len(header_parts)) + " |")

    # Per-question rows
    for i, q in enumerate(results["questions"], 1):
        row = [
            str(i),
            q["question_id"],
            str(q.get("tier", "?")),
            str(q.get("hops", "?"))
        ]
        for m in model_names:
            if m in q["models"]:
                md = q["models"][m]
                wt = md.get("with_tools", {})
                wot = md.get("without_tools", {})

                wt_score = wt.get("scoring", {}).get("score", 0.0)
                row.append(f"{wt_score:.2f}")

                if meta.get("include_baseline", True):
                    wot_score = wot.get("scoring", {}).get("score", 0.0)
                    row.append(f"{wot_score:.2f}")

                tool_count = wt.get("scoring", {}).get("tool_calls_count", 0)
                row.append(str(tool_count))
            else:
                row.extend(["—"] * (3 if meta.get("include_baseline", True) else 2))

        lines.append("| " + " | ".join(row) + " |")

    lines.append("")

    # Summary statistics
    lines.append("## Summary Statistics")
    lines.append("")
    lines.append("| Model | Mode | Avg Score | Success Rate | Avg Time | Avg Tool Calls | Hallucination Risk |")
    lines.append("| --- | --- | --- | --- | --- | --- | --- |")

    for model, stats in summary["models"].items():
        short = model.split("/")[-1]
        wt = stats["with_tools"]
        wot = stats["without_tools"]

        lines.append(
            f"| {short} | +Tools | {wt['avg_score']:.3f} | {wt['success_rate']*100:.0f}% | "
            f"{wt['avg_time']:.1f}s | {wt['avg_tool_calls']:.1f} | {wt['hallucination_count']} |"
        )
        if meta.get("include_baseline", True):
            lines.append(
                f"| {short} | -Tools | {wot['avg_score']:.3f} | {wot['success_rate']*100:.0f}% | "
                f"{wot['avg_time']:.1f}s | — | {wot['hallucination_count']} |"
            )

    lines.append("")

    # Score improvement section
    lines.append("## Score Improvement (+Tools vs -Tools)")
    lines.append("")
    lines.append("| Model | +Tools Avg | -Tools Avg | Delta |")
    lines.append("| --- | --- | --- | --- |")

    for model, stats in summary["models"].items():
        short = model.split("/")[-1]
        wt_score = stats["with_tools"]["avg_score"]
        wot_score = stats["without_tools"]["avg_score"]
        delta = wt_score - wot_score
        sign = "+" if delta >= 0 else ""
        lines.append(f"| {short} | {wt_score:.3f} | {wot_score:.3f} | {sign}{delta:.3f} |")

    lines.append("")

    # Tier breakdown
    lines.append("## Tier Breakdown")
    lines.append("")

    tier_questions = {}
    for q in results["questions"]:
        t = q.get("tier", 0)
        tier_questions.setdefault(t, []).append(q)

    for tier in sorted(tier_questions.keys()):
        qs = tier_questions[tier]
        lines.append(f"### Tier {tier} ({len(qs)} questions)")
        lines.append("")

        for model in model_names:
            short = model.split("/")[-1]
            wt_scores = []
            wot_scores = []
            for q in qs:
                if model in q["models"]:
                    md = q["models"][model]
                    wt_s = md.get("with_tools", {}).get("scoring", {}).get("score", 0.0)
                    wt_scores.append(wt_s)
                    wot_s = md.get("without_tools", {}).get("scoring", {}).get("score", 0.0)
                    wot_scores.append(wot_s)

            wt_avg = sum(wt_scores) / len(wt_scores) if wt_scores else 0.0
            wot_avg = sum(wot_scores) / len(wot_scores) if wot_scores else 0.0
            delta = wt_avg - wot_avg
            sign = "+" if delta >= 0 else ""
            lines.append(f"- **{short}**: +Tools={wt_avg:.3f}, -Tools={wot_avg:.3f}, Delta={sign}{delta:.3f}")

        lines.append("")

    # Per-question details
    lines.append("## Detailed Results")
    lines.append("")

    for q in results["questions"]:
        lines.append(f"### {q['question_id']}")
        lines.append(f"**Q:** {q['question']}  ")
        lines.append(f"**Tier:** {q.get('tier', '?')} | **Hops:** {q.get('hops', '?')} | **Category:** {q.get('category', '?')}  ")
        if q.get("why_llm_fails"):
            lines.append(f"**Why LLM fails:** {q['why_llm_fails']}  ")
        lines.append("")

        for model, md in q["models"].items():
            short = model.split("/")[-1]
            lines.append(f"**{short}:**")

            for mode_key, mode_label in [("with_tools", "+Tools"), ("without_tools", "-Tools")]:
                if mode_key in md:
                    r = md[mode_key]
                    scoring = r.get("scoring", {})
                    if r.get("success"):
                        answer_preview = r.get("answer", "")[:200]
                        if len(r.get("answer", "")) > 200:
                            answer_preview += "..."
                        lines.append(f"- {mode_label}: score={scoring.get('score', 0):.2f}, "
                                     f"tools={scoring.get('tool_calls_count', 0)}, "
                                     f"time={r['elapsed_seconds']:.1f}s")
                        if scoring.get("must_mention_missing"):
                            lines.append(f"  - Missing: {scoring['must_mention_missing']}")
                        if scoring.get("evidence_found"):
                            lines.append(f"  - Evidence (tool-only): {scoring['evidence_found']}")
                        elif not scoring.get("evidence_found") and mode_key == "without_tools":
                            lines.append(f"  - No tool-only evidence (expected)")
                        if scoring.get("hallucination_risk"):
                            lines.append(f"  - **HALLUCINATION RISK**: No tools used, low keyword match")
                    else:
                        lines.append(f"- {mode_label}: FAILED — {r.get('error', 'unknown')}")

            lines.append("")

    return "\n".join(lines)


# =============================================================================
# New Unified Benchmark Commands
# =============================================================================

def cmd_list(questions_file: Path, results_dir: Path) -> int:
    """List all questions with their status."""
    registry = load_questions_registry(questions_file)
    questions = registry.get("questions", [])

    if not questions:
        print("No questions in registry.")
        return 0

    print(f"\n{'='*70}")
    print(f"BENCHMARK QUESTIONS ({len(questions)})")
    print(f"{'='*70}\n")

    for q in questions:
        num = q.get("id", "?")
        qid = q.get("question_id", "?")
        tier = q.get("tier", "?")
        cat = q.get("category", "?")

        # Load results for this question
        results_data = load_results(num, results_dir)
        results = results_data.get("results", {})
        reviews = results_data.get("reviews", {})

        # Count results by type
        biobtree_count = sum(1 for k in results if k.startswith("biobtree_"))
        web_count = sum(1 for k in results if k.startswith("web_"))
        review_count = len(reviews)

        # Status indicators
        status = []
        if biobtree_count > 0:
            status.append(f"{biobtree_count} biobtree")
        if web_count > 0:
            status.append(f"{web_count} web")
        if review_count > 0:
            status.append(f"{review_count} reviews")

        status_str = ", ".join(status) if status else "no results"

        print(f"  {num}. [{qid}] T{tier}/{cat}")
        print(f"     Q: {q.get('question', '')[:60]}...")
        print(f"     Status: {status_str}")
        print()

    return 0


def cmd_show(questions_file: Path, results_dir: Path, question_ref: str) -> int:
    """Show all data for a specific question (by number or ID)."""
    registry = load_questions_registry(questions_file)
    questions = registry.get("questions", [])

    # Try as number first
    try:
        num = int(question_ref)
        q = get_question_by_num(registry, num)
        if not q:
            print(f"Invalid number. Choose 1-{len(questions)}")
            return 1
    except ValueError:
        # Try as question_id
        q = None
        for question in questions:
            if question.get("question_id") == question_ref:
                q = question
                break
        if not q:
            print(f"Question '{question_ref}' not found.")
            print(f"Available: {[q.get('question_id') for q in questions]}")
            return 1

    num = q.get("id")
    qid = q.get("question_id", "?")

    print(f"\n{'='*70}")
    print(f"QUESTION #{num}: {qid}")
    print(f"{'='*70}\n")

    print(f"Question: {q.get('question', '')}")
    print(f"Category: {q.get('category', '?')} | Tier: {q.get('tier', '?')} | Hops: {q.get('hops', '?')}")
    print(f"Theme: {q.get('theme', 'none')}")
    print(f"\nWhy LLM fails: {q.get('why_llm_fails', 'N/A')}")

    gt = q.get("ground_truth", {})
    if gt:
        print(f"\nGround Truth:")
        print(f"  Must mention: {gt.get('must_mention', [])}")
        print(f"  Must mention any: {gt.get('must_mention_any', [])}")
        print(f"  Tool-only terms: {gt.get('tool_only_terms', [])}")
        if gt.get("biobtree_chain"):
            print(f"  Chain: {gt.get('biobtree_chain')}")

    # Load results from separate file
    results_data = load_results(num, results_dir)
    results = results_data.get("results", {})
    if results:
        print(f"\n{'='*40}")
        print("RESULTS")
        print(f"{'='*40}")
        for model_key, r in results.items():
            print(f"\n[{model_key}]")
            print(f"  Model: {r.get('model', '?')}")
            print(f"  Timestamp: {r.get('timestamp', '?')}")
            print(f"  Auto-score: {r.get('auto_score', 'N/A')}")
            if r.get("tool_calls"):
                print(f"  Tool calls: {r.get('tool_calls')}")
            if r.get("elapsed_seconds"):
                print(f"  Time: {r.get('elapsed_seconds'):.1f}s")
            usage = r.get("usage", {})
            if usage and usage.get("total_tokens"):
                print(f"  Tokens: {usage.get('prompt_tokens', 0):,} prompt + {usage.get('completion_tokens', 0):,} completion = {usage.get('total_tokens', 0):,} total")
            if r.get("found_terms"):
                print(f"  Found terms: {r.get('found_terms')}")
            if r.get("missing_terms"):
                print(f"  Missing: {r.get('missing_terms')}")
            if r.get("notes"):
                print(f"  Notes: {r.get('notes')}")
            # Show response preview
            resp = r.get("response", "")
            if resp:
                preview = resp[:200] + "..." if len(resp) > 200 else resp
                print(f"  Response: {preview}")

    reviews = results_data.get("reviews", {})
    if reviews:
        print(f"\n{'='*40}")
        print("REVIEWS")
        print(f"{'='*40}")
        for review_key, rev in sorted(reviews.items()):
            print(f"\n[{review_key}]")
            print(f"  Timestamp: {rev.get('timestamp', '?')}")
            if rev.get("reviewer"):
                print(f"  Reviewer: {rev.get('reviewer')}")
            if rev.get("outcome"):
                print(f"  Outcome: {rev.get('outcome')}")
            if rev.get("status"):
                print(f"  Status: {rev.get('status')}")
            if rev.get("key_finding"):
                print(f"  Key finding: {rev.get('key_finding')}")
            if rev.get("preprint_section"):
                print(f"  Preprint section: {rev.get('preprint_section')}")
            if rev.get("notes"):
                print(f"  Notes: {rev.get('notes')}")

    print()
    return 0


RESPONSE_FILE = Path(__file__).parent / "response.txt"

def cmd_add_web(questions_file: Path, results_dir: Path) -> int:
    """Interactive: add a web LLM or CLI response for a question."""
    registry = load_questions_registry(questions_file)
    questions = registry.get("questions", [])

    if not questions:
        print("No questions in registry. Add questions first.")
        return 1

    # Show available questions with numbers
    print("\nAvailable questions:")
    for q in questions:
        print(f"  {q.get('id')}. {q.get('question_id')}")

    # Get question by number or ID
    choice = input("\nQuestion #: ").strip()
    if not choice:
        print("No question selected. Aborting.")
        return 1

    # Try as number first
    try:
        num = int(choice)
        q = get_question_by_num(registry, num)
        if not q:
            print(f"Invalid number. Choose 1-{len(questions)}")
            return 1
    except ValueError:
        # Try as question_id
        q = None
        for question in questions:
            if question.get("question_id") == choice:
                q = question
                break
        if not q:
            print(f"Question '{choice}' not found.")
            return 1

    num = q.get("id")
    qid = q.get("question_id")
    print(f"Selected: #{num} {qid}")

    # Get model by number
    web_models = [
        ("web_chatgpt52", "ChatGPT 5.2 (web)"),
        ("web_gemini3_pro", "Gemini 3 Pro (web)"),
        ("web_claude46", "Claude 4.6 (web)"),
        ("claude_46_opentargets_mcp_desktop", "Claude 4.6 + OpenTargets MCP (desktop)"),
        ("cli_claude_biobtree_mcp", "Claude CLI + Biobtree MCP"),
    ]
    print("\nModels (web or CLI):")
    for i, (key, name) in enumerate(web_models, 1):
        print(f"  {i}. {name} ({key})")

    model_choice = input("Model #: ").strip()
    if not model_choice:
        print("No model selected. Aborting.")
        return 1

    try:
        idx = int(model_choice) - 1
        if 0 <= idx < len(web_models):
            model_key, model_name = web_models[idx]
        else:
            print(f"Invalid number. Choose 1-{len(web_models)}")
            return 1
    except ValueError:
        # Allow typing the key directly
        model_key = model_choice
        model_name = model_choice.replace("web_", "").replace("_", " ").title()

    print(f"Selected: {model_name}")

    # Get response from file (default) or inline
    default_file = RESPONSE_FILE
    print(f"\nResponse file [{default_file}]:")
    print("  - Press Enter to read from default file")
    print("  - Or type a different file path")
    print("  - Or type 'paste' to paste inline (end with END)")

    file_input = input("> ").strip()

    if file_input.lower() == "paste":
        # Inline paste mode
        print("Paste response, then type END on a new line:")
        lines = []
        while True:
            try:
                line = input()
            except EOFError:
                break
            if line.strip().upper() == "END":
                break
            lines.append(line)
        response = "\n".join(lines)
    else:
        # File mode
        file_path = file_input if file_input else str(default_file)
        try:
            with open(file_path, 'r') as f:
                response = f.read().strip()
            print(f"Read {len(response)} chars from {file_path}")
        except Exception as e:
            print(f"Error reading file: {e}")
            return 1

    if not response:
        print("No response provided. Aborting.")
        return 1

    # Auto-score
    gt = q.get("ground_truth", {})
    scoring = score_response(response, gt, [])
    print(f"\nAuto-score: {scoring['score']:.2f}")
    print(f"  Found: {scoring.get('must_mention_found', []) + scoring.get('any_mention_found', [])}")
    print(f"  Missing: {scoring.get('must_mention_missing', [])}")

    # Allow override
    override = input("Override score? [enter or new score]: ").strip()
    final_score = float(override) if override else scoring["score"]

    # Notes
    notes = input("Notes: ").strip()

    # Entered by
    entered_by = input("Entered by [initials]: ").strip() or "USER"

    # Build result entry
    result_entry = {
        "model": model_name if "MCP" in model_name else (model_name + " (web)" if "(web)" not in model_name else model_name),
        "timestamp": datetime.now().strftime("%Y-%m-%d"),
        "response": response,
        "auto_score": final_score,
        "entered_by": entered_by,
        "found_terms": scoring.get("must_mention_found", []) + scoring.get("any_mention_found", []) + scoring.get("evidence_found", []),
        "missing_terms": scoring.get("must_mention_missing", [])
    }
    if notes:
        result_entry["notes"] = notes

    # Load existing results for this question
    results_data = load_results(num, results_dir)
    results_data["id"] = num
    results_data["question_id"] = qid
    if "results" not in results_data:
        results_data["results"] = {}
    results_data["results"][model_key] = result_entry

    # Save to results file
    save_results(num, results_data, results_dir)
    print(f"\nAdded {model_key} result for #{num} {qid}")
    return 0


def cmd_run_benchmark(
    server_url: str,
    questions_file: Path,
    results_dir: Path,
    models: list,
    question_ids: Optional[list],
    tier: Optional[int],
    timeout: float,
    include_baseline: bool,
    force: bool = False
) -> int:
    """Run benchmark and save results to results/N.json files."""
    registry = load_questions_registry(questions_file)
    all_questions = registry.get("questions", [])

    if not all_questions:
        print("No questions in registry. Add questions to questions.json first.")
        return 1

    # MUST specify question - no accidental run-all
    if not question_ids:
        print("\nAvailable questions:")
        for q in all_questions:
            print(f"  {q.get('id')}. {q.get('question_id')}")
        print("\nYou must specify a question. Use:")
        print("  --questions <number>   (by number)")
        print("  --questions <id>       (by question_id)")
        return 1

    # Resolve question by number or question_id
    questions = []
    for qref in question_ids:
        try:
            num = int(qref)
            q = get_question_by_num(registry, num)
            if q:
                questions.append(q)
            else:
                print(f"Invalid number {qref}. Choose 1-{len(all_questions)}")
                return 1
        except ValueError:
            # Try as question_id
            q = None
            for question in all_questions:
                if question.get("question_id") == qref:
                    q = question
                    break
            if q:
                questions.append(q)
            else:
                print(f"Question '{qref}' not found.")
                return 1

    if not questions:
        print("No questions selected.")
        return 1

    print(f"\n{'='*60}")
    print("Biobtree Benchmark (Split Format)")
    print(f"{'='*60}")
    print(f"Server: {server_url}")
    print(f"Models: {', '.join(models)}")
    print(f"Questions: {len(questions)}")
    print(f"Baseline: {include_baseline}")
    print(f"Output: {results_dir}")
    print(f"{'='*60}\n")

    runner = BenchmarkRunner(server_url, timeout=timeout)

    for i, q in enumerate(questions, 1):
        num = q.get("id")
        qid = q.get("question_id")
        tier_str = f"T{q.get('tier', '?')}"
        hops_str = f"{q.get('hops', '?')}h"
        print(f"[{i}/{len(questions)}] [{tier_str}/{hops_str}] #{num} {qid}")
        print(f"  Q: {q.get('question', '')[:60]}...")

        gt = q.get("ground_truth", {})

        # Load existing results for this question
        results_data = load_results(num, results_dir)
        results_data["id"] = num
        results_data["question_id"] = qid
        if "results" not in results_data:
            results_data["results"] = {}

        for model in models:
            model_key = get_model_key(model)
            existing_results = results_data.get("results", {})

            # Check if results already exist
            if model_key in existing_results and not force:
                existing_score = existing_results[model_key].get("auto_score", "?")
                print(f"\n  Skipping {model} ({model_key}) - already has results (score={existing_score})")
                print(f"    Use --force to re-run and save as new entry")
                continue

            # If force and exists, use timestamped key
            if model_key in existing_results and force:
                timestamp_suffix = datetime.now().strftime("%Y%m%d_%H%M%S")
                save_key = f"{model_key}_{timestamp_suffix}"
                print(f"\n  Re-running {model} (will save as {save_key})...")
            else:
                save_key = model_key
                print(f"\n  Testing {model} ({model_key})...")

            # Run with tools
            print(f"    With tools...", end=" ", flush=True)
            result = runner.run_query(q.get("question"), model, with_tools=True)

            if result.get("success"):
                scoring = score_response(
                    result.get("answer", ""),
                    gt,
                    result.get("tool_calls", [])
                )
                usage = result.get("usage", {})
                tokens_str = f", tokens={usage.get('total_tokens', 0):,}" if usage.get('total_tokens') else ""
                print(f"score={scoring['score']:.2f}, tools={len(result.get('tool_calls', []))}, time={result['elapsed_seconds']:.1f}s{tokens_str}")

                # Store result
                result_entry = {
                    "model": model,
                    "timestamp": datetime.now().isoformat(),
                    "response": result.get("answer", ""),
                    "tool_calls": len(result.get("tool_calls", [])),
                    "elapsed_seconds": round(result["elapsed_seconds"], 1),
                    "auto_score": scoring["score"],
                    "found_terms": scoring.get("must_mention_found", []) + scoring.get("any_mention_found", []) + scoring.get("evidence_found", []),
                    "missing_terms": scoring.get("must_mention_missing", []),
                    "usage": {
                        "prompt_tokens": usage.get("prompt_tokens", 0),
                        "completion_tokens": usage.get("completion_tokens", 0),
                        "total_tokens": usage.get("total_tokens", 0)
                    }
                }

                results_data["results"][save_key] = result_entry
            else:
                print(f"FAILED: {result.get('error', 'unknown')}")

            # Run without tools (baseline)
            if include_baseline:
                print(f"    Without tools...", end=" ", flush=True)
                baseline = runner.run_query(q.get("question"), model, with_tools=False)

                if baseline.get("success"):
                    scoring = score_response(baseline.get("answer", ""), gt, [])
                    print(f"score={scoring['score']:.2f}, time={baseline['elapsed_seconds']:.1f}s")
                else:
                    print(f"FAILED: {baseline.get('error', 'unknown')}")

        # Save results for this question
        save_results(num, results_data, results_dir)
        print()

    print(f"\nBenchmark complete. Results saved to {results_dir}")

    return 0


def cmd_combine(questions_file: Path, results_dir: Path, output_file: Path) -> int:
    """Combine questions and results into a single JSON file for export."""
    registry = load_questions_registry(questions_file)
    questions = registry.get("questions", [])

    if not questions:
        print("No questions in registry.")
        return 1

    # Build combined output
    combined = {
        "metadata": registry.get("metadata", {}),
        "questions": []
    }

    for q in questions:
        num = q.get("id")
        qid = q.get("question_id")

        # Load results for this question
        results_data = load_results(num, results_dir)

        # Merge question metadata with results
        combined_q = {
            "id": num,
            "question_id": qid,
            "question": q.get("question", ""),
            "category": q.get("category", ""),
            "tier": q.get("tier", 0),
            "hops": q.get("hops", 0),
            "theme": q.get("theme", ""),
            "why_llm_fails": q.get("why_llm_fails", ""),
            "ground_truth": q.get("ground_truth", {}),
            "results": results_data.get("results", {}),
            "reviews": results_data.get("reviews", {})
        }

        combined["questions"].append(combined_q)

    # Write combined file
    output_file.parent.mkdir(parents=True, exist_ok=True)
    with open(output_file, "w") as f:
        json.dump(combined, f, indent=2)

    # Print summary
    total_results = sum(len(q.get("results", {})) for q in combined["questions"])
    total_reviews = sum(len(q.get("reviews", {})) for q in combined["questions"])

    print(f"\nCombined output written to: {output_file}")
    print(f"  Questions: {len(combined['questions'])}")
    print(f"  Total results: {total_results}")
    print(f"  Total reviews: {total_reviews}")
    print(f"  File size: {output_file.stat().st_size / 1024:.1f} KB")

    return 0


# =============================================================================
# Main
# =============================================================================

def main():
    parser = argparse.ArgumentParser(
        description="Benchmark biobtree chat endpoint with auto-scoring",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Primary commands (split file format):
  python tests/benchmark_chat.py --list
  python tests/benchmark_chat.py --show 1
  python tests/benchmark_chat.py --run-benchmark --questions 1 --models anthropic/claude-sonnet-4
  python tests/benchmark_chat.py --add-web
  python tests/benchmark_chat.py --combine --output benchmark_export.json

  # Legacy commands (questions.json only):
  python tests/benchmark_chat.py --dry-run
        """
    )
    parser.add_argument(
        "--server", "-s",
        default=DEFAULT_SERVER,
        help=f"Server URL (default: {DEFAULT_SERVER})"
    )
    parser.add_argument(
        "--models", "-m",
        nargs="+",
        default=DEFAULT_MODELS,
        help="Models to benchmark (OpenRouter model IDs)"
    )
    parser.add_argument(
        "--questions-file",
        type=Path,
        default=QUESTIONS_FILE,
        help=f"Path to questions JSON file (default: {QUESTIONS_FILE})"
    )
    parser.add_argument(
        "--results-dir",
        type=Path,
        default=RESULTS_DIR,
        help=f"Results directory (default: {RESULTS_DIR})"
    )
    parser.add_argument(
        "--tier",
        type=int,
        choices=[1, 2, 3],
        help="Filter questions by tier (1=LLM partial, 2=LLM fails, 3=complex)"
    )
    parser.add_argument(
        "--no-baseline",
        action="store_true",
        help="Skip baseline (without tools) tests"
    )
    parser.add_argument(
        "--timeout", "-t",
        type=float,
        default=120.0,
        help="Request timeout in seconds (default: 120)"
    )
    parser.add_argument(
        "--questions", "-q",
        nargs="+",
        help="Specific question numbers or IDs to run"
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Load questions and show what would run, without calling API"
    )

    # Primary commands (split file format)
    parser.add_argument(
        "--run-benchmark",
        action="store_true",
        help="Run benchmark and save results to results/N.json files"
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Force re-run even if results exist (saves as new entry with timestamp)"
    )
    parser.add_argument(
        "--add-web",
        action="store_true",
        help="Interactive: add a web LLM response for a question"
    )
    parser.add_argument(
        "--list",
        action="store_true",
        help="List all questions with their status"
    )
    parser.add_argument(
        "--show",
        type=str,
        metavar="QUESTION",
        help="Show all data for a specific question (by number or ID)"
    )
    parser.add_argument(
        "--combine",
        action="store_true",
        help="Combine questions and results into a single JSON file for export"
    )
    parser.add_argument(
        "--output", "-o",
        type=Path,
        default=Path(__file__).parent / "benchmark_export.json",
        help="Output file for --combine (default: benchmark_export.json)"
    )

    args = parser.parse_args()

    # Primary commands (split file format)
    if args.list:
        return cmd_list(args.questions_file, args.results_dir)

    if args.show:
        return cmd_show(args.questions_file, args.results_dir, args.show)

    if args.add_web:
        return cmd_add_web(args.questions_file, args.results_dir)

    if args.combine:
        return cmd_combine(args.questions_file, args.results_dir, args.output)

    if args.run_benchmark:
        return cmd_run_benchmark(
            args.server,
            args.questions_file,
            args.results_dir,
            args.models,
            args.questions,
            args.tier,
            args.timeout,
            not args.no_baseline,
            args.force
        )

    # Legacy commands below - use questions.json for dry-run

    # Load questions
    questions = load_questions(args.questions_file)

    # Filter by tier
    if args.tier:
        questions = [q for q in questions if q.get("tier") == args.tier]
        if not questions:
            print(f"No questions found for tier {args.tier}")
            return 1

    # Filter by specific question IDs
    if args.questions:
        questions = [q for q in questions if str(q.get("id")) in args.questions or q.get("question_id") in args.questions]
        if not questions:
            all_ids = [q.get("question_id") for q in load_questions(args.questions_file)]
            print(f"No matching questions found. Available: {all_ids}")
            return 1

    # Dry run - just show what would happen
    if args.dry_run:
        print(f"\nDRY RUN — {len(questions)} questions would be tested:\n")
        for i, q in enumerate(questions, 1):
            tier = q.get("tier", "?")
            hops = q.get("hops", "?")
            gt = q.get("ground_truth", {})
            must = gt.get("must_mention", [])
            must_any = gt.get("must_mention_any", [])
            print(f"  {i}. [{q.get('question_id', q.get('id'))}] T{tier}/{hops}h")
            print(f"     Q: {q['question'][:80]}...")
            print(f"     Must mention: {must}")
            print(f"     Must mention any: {must_any}")
            print()
        print(f"Models: {args.models}")
        print(f"Baseline: {not args.no_baseline}")
        return 0

    # No command specified - show help
    print("No command specified. Use --list, --show, --run-benchmark, --add-web, or --combine")
    print("Run with -h for help.")
    return 1


if __name__ == "__main__":
    exit(main())
