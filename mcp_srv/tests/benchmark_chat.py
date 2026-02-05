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
                "answer_length": len(data.get("answer", ""))
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

def save_results(results: dict, results_dir: Path):
    """Save results to timestamped directory with raw JSON and summary markdown."""
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
# Main
# =============================================================================

def main():
    parser = argparse.ArgumentParser(
        description="Benchmark biobtree chat endpoint with auto-scoring",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python tests/benchmark_chat.py --tier 1
  python tests/benchmark_chat.py --tier 2 --models anthropic/claude-sonnet-4
  python tests/benchmark_chat.py --questions brca1_pathogenic_variants tp53_interactions
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
        default=None,
        help="Results output directory (default: results/YYYY-MM-DD_HHMMSS)"
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
        help="Specific question IDs to run (default: all)"
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Load questions and show what would run, without calling API"
    )

    args = parser.parse_args()

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
        questions = [q for q in questions if q["id"] in args.questions]
        if not questions:
            all_ids = [q["id"] for q in load_questions(args.questions_file)]
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
            print(f"  {i}. [{q['id']}] T{tier}/{hops}h")
            print(f"     Q: {q['question'][:80]}...")
            print(f"     Must mention: {must}")
            print(f"     Must mention any: {must_any}")
            print()
        print(f"Models: {args.models}")
        print(f"Baseline: {not args.no_baseline}")
        return 0

    # Determine results directory
    if args.results_dir:
        results_dir = args.results_dir
    else:
        timestamp = datetime.now().strftime("%Y-%m-%d_%H%M%S")
        results_dir = Path(__file__).parent / "results" / timestamp

    # Run benchmark
    runner = BenchmarkRunner(args.server, timeout=args.timeout)
    results = runner.run_benchmark(
        models=args.models,
        questions=questions,
        include_baseline=not args.no_baseline
    )

    # Print summary
    runner.print_summary(results)

    # Save results
    save_results(results, results_dir)

    return 0


if __name__ == "__main__":
    exit(main())
