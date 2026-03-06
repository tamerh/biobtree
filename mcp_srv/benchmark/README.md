# Biobtree Benchmark System

Compares LLM+biobtree responses against web LLMs (ChatGPT, Gemini, Claude) for biological database queries. Produces scored results for preprint.

## File Structure

```
mcp_srv/benchmark/
  questions.json        # Question registry with ground truth
  results/
    1.json              # Per-question results (API + web + reviews)
    2.json
    ...
  benchmark_export.json # Combined export for analysis
  benchmark_chat.py     # Main tool
```

## Quick Commands

```bash
# List all questions with status
python benchmark_chat.py --list

# Show details for a question
python benchmark_chat.py --show 1

# Run API benchmark for specific questions
python benchmark_chat.py --run-benchmark --questions 1 2 3

# Add web LLM responses interactively
python benchmark_chat.py --add-web

# Generate combined export
python benchmark_chat.py --combine
```

## Workflow: Adding a New Test

### 1. Design the Question

Find questions where biobtree adds value:
- **Enumeration**: "List ALL X" (LLMs give 5-10 examples, biobtree gives 100+)
- **Data accuracy**: Specific database values (LLMs may fabricate)
- **Cross-database**: Multi-hop chains (HPO→Orphanet→Ensembl→ClinVar)
- **Data freshness**: Post-training cutoff data (NCT07 trials, new drugs)

Avoid questions where LLMs already know the answer (well-published biology).

### 2. Test with MCP (Interactive Discovery)

In a Claude Code session with biobtree MCP configured, test the question interactively:

```
# Search for the gene/entity
biobtree_search(terms="SCN9A", dataset="ensembl")
→ ENSG00000169432

# Get detailed data (expression, variants, etc.)
biobtree_entry(identifier="ENSG00000169432", dataset="bgee")
→ Expression scores by tissue

# Test cross-database chains
biobtree_map(terms="ENSG00000169432", chain=">>ensembl>>uniprot>>chembl_molecule")
→ Verify mappings exist
```

This helps you:
- Discover exact ground truth values (e.g., "DRG expression score: 88.05")
- Find data gaps before formalizing the question
- Verify chains return meaningful results

### 3. Verify Chain via API (Optional)

```bash
# Alternative: test via curl
curl "http://localhost:8000/ws/map/?i=GENE&m=>>ensembl>>uniprot>>chembl"
```

### 4. Add to questions.json

```json
{
  "id": 11,
  "question_id": "descriptive_id",
  "question": "The full question text...",
  "category": "category_name",
  "tier": 2,
  "hops": 3,
  "theme": "theme_name",
  "why_llm_fails": "Why LLMs cannot answer this correctly",
  "ground_truth": {
    "must_mention": ["REQUIRED", "TERMS"],
    "must_mention_any": ["any", "of", "these"],
    "tool_only_terms": ["database_specific_ids", "exact_values"],
    "biobtree_chain": ">>dataset1>>dataset2",
    "key_values": { "metric": "value" }
  }
}
```

### 5. Run API Benchmark

```bash
python benchmark_chat.py --run-benchmark --questions 11 \
  --models anthropic/claude-sonnet-4 openai/gpt-4.1 google/gemini-2.5-pro-preview-03-25
```

### 6. Add Web Responses

Ask the same question to web LLMs (ChatGPT, Gemini, Claude with web search), then:

```bash
python benchmark_chat.py --add-web
# Select question, select model, paste response
```

### 7. Write Review

Edit `results/11.json` and add review:

```json
"reviews": {
  "review_1": {
    "outcome": "biobtree_win",  // or "tie", "mixed_competitive"
    "key_finding": "Summary of what happened",
    "quotable": "Paper-ready quote"
  }
}
```

### 8. Combine for Export

```bash
python benchmark_chat.py --combine --output benchmark_export.json
```

## Scoring

Auto-score based on keyword matching (0.0-1.0):
- `must_mention`: All required terms must appear
- `must_mention_any`: At least one alternative term
- `tool_only_terms`: Database-specific IDs (evidence of real data)

Note: Auto-scoring catches keywords but not semantic errors. Reviews capture nuanced findings (e.g., fabricated values, wrong IDs).

## Current Results Summary

| Outcome | Count | Examples |
|---------|-------|----------|
| biobtree_win | 8 | NPC1 variants (21x undercount), APOE AlphaMissense (fabricated score) |
| tie | 1 | LRRK2 cell type (well-published biology) |
| mixed | 1 | Alkaptonuria trials (LLMs found core data) |

## Models

**API (via OpenRouter)**:
- `anthropic/claude-sonnet-4`
- `openai/gpt-4.1`
- `google/gemini-2.5-pro-preview-03-25`

**Web baselines** (manual):
- ChatGPT 5.2 with web search
- Gemini 3 Pro with web search
- Claude 4.6 with web search

**MCP baselines** (manual):
- Claude 4.6 with OpenTargets MCP (Desktop)

Set `OPENROUTER_API_KEY` in environment or `config.py`.
