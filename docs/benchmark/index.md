# Benchmark

BioBTree was evaluated as a tool set for large language models (LLMs). The same
LLM answered the same questions under different conditions, each giving it a
different set of tools:

| condition | tools |
|---|---|
| no tools | none (the model's own knowledge) |
| BioBTree | the BioBTree MCP tools (search, map, entry, atlas, enrich; plus variant on the variant tasks) |
| web search | web search and page fetch |
| domain MCP servers | the Open Targets Platform MCP server or BioMCP |

The questions, the harness, the per-question answers and the scoring code are in
**[tamerh/biobtree-bench](https://github.com/tamerh/biobtree-bench)**. The full
study design, statistics and discussion are in the
[preprint](https://doi.org/10.5281/zenodo.24144112).

## Setup

- **Models:** `deepseek-v4.1-flash` (DeepSeek) for the main runs; every set was
  repeated with `claude-haiku-5-5` (Haiku).
- **Protocol:** temperature 0, at most 15 tool-calling steps, 16k output tokens.
- **Data:** BioBTree v2.20.6, release of 9 November 2026.
- **Question sets:**
  - *GeneTuring* (900): the nine database-lookup modules, 100 questions each.
  - *LAB-Bench DbQA* (160): database questions from LAB-Bench.
  - *Variant retrieval* (86): clinical significance, frequency and consequence of
    variants.
  - *Variant batch*: one table of 30 variants in a single answer.
  - *Aggregation* (20): counts and sets across sources.
- **Scoring:** automatic, against the gold answer of each question.

## Results

### GeneTuring, by module

Correct answers out of 100 per module and 900 overall.

| module | DeepSeek, no tools | DeepSeek, BioBTree | DeepSeek, web search | Haiku, no tools | Haiku, BioBTree | Haiku, web search |
|---|---:|---:|---:|---:|---:|---:|
| Gene location | 43 | 95 | 85 | 31 | 95 | 87 |
| SNP location | 0 | 99 | 98 | 0 | 99 | 100 |
| Gene name conversion | 38 | 100 | 99 | 6 | 99 | 98 |
| Gene SNP association | 0 | 100 | 99 | 0 | 100 | 100 |
| Gene alias | 42 | 95 | 93 | 23 | 96 | 79 |
| Gene-disease association | 77 | 86 | 67 | 63 | 81 | 80 |
| TF regulation | 79 | 84 | 34 | 76 | 99 | 76 |
| Gene ontology | 4 | 74 | 44 | 6 | 72 | 13 |
| Protein-coding genes | 96 | 100 | 100 | 98 | 100 | 100 |
| **overall** | 379/900 | **833/900** | 719/900 | 303/900 | **841/900** | 733/900 |

### All sets

Mean per question. Cost is at the provider's list prices, including discounts for
cached input; the BioBTree and web search tools themselves incurred no charge.

| set | model | condition | correct | calls | tokens | seconds | cost |
|---|---|---|---:|---:|---:|---:|---:|
| GeneTuring (900) | DeepSeek | no tools | 379/900 | 0.0 | 1,751 | 19 | $0.0010 |
| | DeepSeek | BioBTree | 833/900 | 4.8 | 53,410 | 21 | $0.0016 |
| | DeepSeek | web search | 719/900 | 11.8 | 136,834 | 96 | $0.0089 |
| | Haiku | no tools | 303/900 | 0.0 | 909 | 5 | $0.0004 |
| | Haiku | BioBTree | 841/900 | 2.5 | 39,277 | 5 | $0.0025 |
| | Haiku | web search | 733/900 | 4.4 | 36,662 | 57 | $0.0023 |
| LAB-Bench (160) | DeepSeek | no tools | 72/160 | 0.0 | 3,649 | 31 | $0.0021 |
| | DeepSeek | BioBTree | 157/160 | 6.4 | 77,291 | 34 | $0.0032 |
| | DeepSeek | web search | 151/160 | 8.6 | 62,257 | 65 | $0.0090 |
| | Haiku | no tools | 137/160 | 0.0 | 1,012 | 5 | $0.0004 |
| | Haiku | BioBTree | 159/160 | 5.0 | 71,260 | 9 | $0.0039 |
| | Haiku | web search | 160/160 | 4.6 | 43,273 | 18 | $0.0041 |
| Variant retrieval (86) | DeepSeek | no tools | 7/86 | 0.0 | 2,341 | 36 | $0.0014 |
| | DeepSeek | BioBTree | 82/86 | 2.2 | 27,757 | 13 | $0.0017 |
| | DeepSeek | web search | 80/86 | 8.9 | 60,195 | 50 | $0.0046 |
| | Haiku | no tools | 13/86 | 0.0 | 765 | 4 | $0.0003 |
| | Haiku | BioBTree | 82/86 | 1.8 | 38,023 | 5 | $0.0025 |
| | Haiku | web search | 81/86 | 3.5 | 19,257 | 8 | $0.0019 |
| Variant batch (30 rows) | DeepSeek | BioBTree | 30/30 | 4.0 | 44,040 | 26 | $0.0049 |
| | DeepSeek | web search | 30/30 | 16.0 | 181,388 | 62 | $0.0122 |
| | Haiku | BioBTree | 30/30 | 2.0 | 101,254 | 27 | $0.0081 |
| | Haiku | web search | 30/30 | 4.0 | 127,778 | 33 | $0.0127 |

On the aggregation set, DeepSeek answered 16/20 with BioBTree and 11/20 with web
search, using 4.8 and 18.0 calls per question.

### Domain MCP servers

GeneTuring with Haiku, correct answers out of 100 per module and 900 overall.

| module | BioBTree | Open Targets MCP | BioMCP | web search |
|---|---:|---:|---:|---:|
| Gene location | 95 | 62 | 51 | 87 |
| SNP location | 99 | 1 | 96 | 100 |
| Gene name conversion | 99 | 97 | 7 | 98 |
| Gene SNP association | 100 | 3 | 100 | 100 |
| Gene alias | 96 | 97 | 37 | 79 |
| Gene-disease association | 81 | 83 | 78 | 80 |
| TF regulation | 99 | 83 | 82 | 76 |
| Gene ontology | 72 | 28 | 30 | 13 |
| Protein-coding genes | 100 | 100 | 100 | 100 |
| **overall** | **841/900** | 554/900 | 581/900 | 733/900 |

Each server's accuracy follows the scope of its data. The servers were run against
their live services (Open Targets MCP 2026.7.1, BioMCP 0.7.3).

## Summary

- With web search, both models located single records almost as well as with
  BioBTree; the differences were largest on questions that ask for a complete set
  (TF regulation, gene ontology) or need identifiers reconciled across sources.
- With BioBTree, the models needed fewer tool calls than with web search on every
  set except LAB-Bench with Haiku, and less wall-clock time in every case; token
  use showed no consistent direction.
- Results depend on the model, the provider and the data release; see the
  preprint's Limitations for the known sources of scoring error.

## Reproducing

```bash
git clone https://github.com/tamerh/biobtree-bench
cd biobtree-bench
export OPENROUTER_API_KEY=...
export MCP_URL=https://sugi.bio/biobtree/mcp
python3 runner.py --questions questions/anchor_geneturing_900.json \
  --models <model> --conditions no_tools mcp web --max-iterations 15
```

See the repository README for the question sets, conditions and the per-question
results file (`cells.csv`).
