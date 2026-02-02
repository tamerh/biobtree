#!/usr/bin/env python3
"""
Biobtree Chat Benchmark Script

Benchmarks the /chat endpoint with multiple models and questions.
Compares responses with and without biobtree tools.

Usage:
    python tests/benchmark_chat.py
    python tests/benchmark_chat.py --server http://localhost:8000
    python tests/benchmark_chat.py --models claude-3-haiku gpt-4o-mini
    python tests/benchmark_chat.py --output results.json
"""

import argparse
import json
import time
from datetime import datetime
from pathlib import Path
from typing import Optional

import httpx

# =============================================================================
# Configuration
# =============================================================================

DEFAULT_SERVER = "http://localhost:8000"

# Models to benchmark (OpenRouter model IDs)
DEFAULT_MODELS = [
    "anthropic/claude-3-haiku",      # Fast, cheap
    "anthropic/claude-sonnet-4",     # Balanced
    # "openai/gpt-4o-mini",          # Fast, cheap
    # "openai/gpt-4o",               # High quality
    # "meta-llama/llama-3.3-70b-instruct",  # Open source
]

# Benchmark questions - mix of simple and complex queries
BENCHMARK_QUESTIONS = [
    {
        "id": "gene_protein_simple",
        "question": "What protein does the TP53 gene encode?",
        "category": "gene_to_protein",
        "difficulty": "easy"
    },
    {
        "id": "gene_protein_brca1",
        "question": "What proteins does BRCA1 encode and what are their functions?",
        "category": "gene_to_protein",
        "difficulty": "medium"
    },
    {
        "id": "drug_target",
        "question": "What are the protein targets of imatinib?",
        "category": "drug_to_target",
        "difficulty": "medium"
    },
    {
        "id": "gene_disease",
        "question": "What diseases are associated with mutations in the EGFR gene?",
        "category": "gene_to_disease",
        "difficulty": "medium"
    },
    {
        "id": "variant_clinical",
        "question": "What is the clinical significance of the rs1799853 variant?",
        "category": "variant_to_disease",
        "difficulty": "medium"
    },
    {
        "id": "protein_function",
        "question": "What is the function of UniProt protein P04637?",
        "category": "protein_info",
        "difficulty": "easy"
    },
    {
        "id": "gene_drugs",
        "question": "What drugs target the EGFR gene product?",
        "category": "gene_to_drug",
        "difficulty": "hard"
    },
    {
        "id": "pathway_genes",
        "question": "What genes are involved in the apoptosis pathway?",
        "category": "pathway_to_gene",
        "difficulty": "hard"
    },
]


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

            # Check for error in response
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
        results = {
            "question_id": question_data["id"],
            "question": question_data["question"],
            "category": question_data["category"],
            "difficulty": question_data["difficulty"],
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
            status = "✓" if model_results["with_tools"]["success"] else "✗"
            elapsed = model_results["with_tools"]["elapsed_seconds"]
            print(f"{status} ({elapsed:.1f}s)")

            # Without tools (baseline)
            if include_baseline:
                print(f"    Testing {model} without tools...", end=" ", flush=True)
                model_results["without_tools"] = self.run_query(
                    question_data["question"],
                    model,
                    with_tools=False
                )
                status = "✓" if model_results["without_tools"]["success"] else "✗"
                elapsed = model_results["without_tools"]["elapsed_seconds"]
                print(f"{status} ({elapsed:.1f}s)")

            results["models"][model] = model_results

        return results

    def run_benchmark(
        self,
        models: list,
        questions: list = None,
        include_baseline: bool = True
    ) -> dict:
        """Run full benchmark suite."""
        questions = questions or BENCHMARK_QUESTIONS

        print(f"\n{'='*60}")
        print("Biobtree Chat Benchmark")
        print(f"{'='*60}")
        print(f"Server: {self.server_url}")
        print(f"Models: {', '.join(models)}")
        print(f"Questions: {len(questions)}")
        print(f"Include baseline: {include_baseline}")
        print(f"{'='*60}\n")

        results = {
            "metadata": {
                "server": self.server_url,
                "models": models,
                "timestamp": datetime.now().isoformat(),
                "include_baseline": include_baseline,
                "total_questions": len(questions)
            },
            "questions": []
        }

        for i, q in enumerate(questions, 1):
            print(f"[{i}/{len(questions)}] {q['id']}: {q['question'][:50]}...")
            q_results = self.benchmark_question(q, models, include_baseline)
            results["questions"].append(q_results)
            print()

        # Calculate summary statistics
        results["summary"] = self._calculate_summary(results)

        return results

    def _calculate_summary(self, results: dict) -> dict:
        """Calculate summary statistics."""
        summary = {"models": {}}

        for model in results["metadata"]["models"]:
            model_stats = {
                "with_tools": {
                    "success_count": 0,
                    "total_time": 0,
                    "total_tool_calls": 0,
                    "total_iterations": 0
                },
                "without_tools": {
                    "success_count": 0,
                    "total_time": 0
                }
            }

            for q in results["questions"]:
                if model in q["models"]:
                    m = q["models"][model]

                    if "with_tools" in m and m["with_tools"]["success"]:
                        model_stats["with_tools"]["success_count"] += 1
                        model_stats["with_tools"]["total_time"] += m["with_tools"]["elapsed_seconds"]
                        model_stats["with_tools"]["total_tool_calls"] += len(m["with_tools"].get("tool_calls", []))
                        model_stats["with_tools"]["total_iterations"] += m["with_tools"].get("iterations", 0)

                    if "without_tools" in m and m["without_tools"]["success"]:
                        model_stats["without_tools"]["success_count"] += 1
                        model_stats["without_tools"]["total_time"] += m["without_tools"]["elapsed_seconds"]

            # Calculate averages
            n_questions = len(results["questions"])
            wt = model_stats["with_tools"]
            wot = model_stats["without_tools"]

            summary["models"][model] = {
                "with_tools": {
                    "success_rate": wt["success_count"] / n_questions if n_questions > 0 else 0,
                    "avg_time": wt["total_time"] / wt["success_count"] if wt["success_count"] > 0 else 0,
                    "avg_tool_calls": wt["total_tool_calls"] / wt["success_count"] if wt["success_count"] > 0 else 0,
                    "avg_iterations": wt["total_iterations"] / wt["success_count"] if wt["success_count"] > 0 else 0
                },
                "without_tools": {
                    "success_rate": wot["success_count"] / n_questions if n_questions > 0 else 0,
                    "avg_time": wot["total_time"] / wot["success_count"] if wot["success_count"] > 0 else 0
                }
            }

        return summary

    def print_summary(self, results: dict):
        """Print a formatted summary."""
        print(f"\n{'='*60}")
        print("BENCHMARK SUMMARY")
        print(f"{'='*60}\n")

        summary = results["summary"]

        # Table header
        print(f"{'Model':<35} {'Success':<10} {'Avg Time':<12} {'Avg Tools':<12}")
        print("-" * 70)

        for model, stats in summary["models"].items():
            wt = stats["with_tools"]
            model_short = model.split("/")[-1][:32]
            print(f"{model_short:<35} {wt['success_rate']*100:>6.1f}% {wt['avg_time']:>8.1f}s {wt['avg_tool_calls']:>8.1f}")

        print()

        # Baseline comparison
        if results["metadata"]["include_baseline"]:
            print("Baseline (without tools):")
            print(f"{'Model':<35} {'Success':<10} {'Avg Time':<12}")
            print("-" * 50)
            for model, stats in summary["models"].items():
                wot = stats["without_tools"]
                model_short = model.split("/")[-1][:32]
                print(f"{model_short:<35} {wot['success_rate']*100:>6.1f}% {wot['avg_time']:>8.1f}s")


# =============================================================================
# Main
# =============================================================================

def main():
    parser = argparse.ArgumentParser(description="Benchmark biobtree chat endpoint")
    parser.add_argument(
        "--server", "-s",
        default=DEFAULT_SERVER,
        help=f"Server URL (default: {DEFAULT_SERVER})"
    )
    parser.add_argument(
        "--models", "-m",
        nargs="+",
        default=DEFAULT_MODELS,
        help="Models to benchmark"
    )
    parser.add_argument(
        "--output", "-o",
        help="Output JSON file for results"
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

    args = parser.parse_args()

    # Filter questions if specified
    questions = BENCHMARK_QUESTIONS
    if args.questions:
        questions = [q for q in questions if q["id"] in args.questions]
        if not questions:
            print(f"No matching questions found. Available: {[q['id'] for q in BENCHMARK_QUESTIONS]}")
            return 1

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
    if args.output:
        output_path = Path(args.output)
        output_path.write_text(json.dumps(results, indent=2))
        print(f"\nResults saved to: {output_path}")

    return 0


if __name__ == "__main__":
    exit(main())
