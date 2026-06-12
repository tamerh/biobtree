# ESM2 Protein Similarity Dataset

## Overview

ESM2 protein embedding-based semantic similarity database enabling deep learning-powered similarity searches across the proteome. Provides top-N similar proteins based on cosine similarity of ESM2 embeddings, capturing functional and structural relationships that sequence similarity alone may miss.

**Source**: ESM2 embeddings from Meta AI's protein language model stored in Qdrant vector database
**Data Type**: Cosine similarity scores from ESM2 protein embeddings with rank ordering

## Integration Architecture

### Storage Model
**Primary Entries**: UniProt IDs (e.g., `Q6GZX4`)
**Searchable Text Links**: UniProt IDs indexed for text search
**Attributes Stored**: protein_id, similarities array (target proteins with cosine scores and rank), similarity_count, top_similarity, avg_similarity
**Cross-References**:
- ESM2 entry → Similar UniProt proteins (top N hits)

### Special Features
- **Semantic Similarity**: Captures functional/structural relationships beyond sequence identity
- **Fixed Top-N**: Each protein has exactly 50 similar proteins
- **Cosine Similarity**: Scores range from 0 to 1 (higher = more similar)
- **Rank Ordering**: Similarities ordered by score (rank 1 = most similar)
- **Fast Lookup**: Pre-computed embeddings enable instant similarity queries

## Use Cases

**1. Functional Homolog Discovery**
```
Query: Find functionally similar proteins regardless of sequence >> TP53 >> uniprot >> esm2_similarity >> uniprot
Use: Identify proteins with similar function even with low sequence identity
```

**2. Protein Function Prediction**
```
Query: Infer function from similar proteins >> Unknown protein >> esm2_similarity >> uniprot[reviewed=true]
Use: Transfer functional annotations from well-characterized proteins
```

**3. Drug Target Discovery**
```
Query: Find druggable proteins similar to target >> Drug target >> esm2_similarity >> uniprot >> chembl_target
Use: Identify novel drug targets with similar binding properties
```

**4. Structural Template Discovery**
```
Query: Find proteins with similar structure >> Novel protein >> esm2_similarity >> alphafold
Use: Identify structural templates for protein engineering
```

**5. Disease Variant Analysis**
```
Query: Find similar proteins for variant interpretation >> Disease protein >> esm2_similarity >> clinvar
Use: Understand variant impact by comparing to similar proteins
```

**6. Protein Family Expansion**
```
Query: Discover related proteins beyond sequence similarity >> Seed protein >> esm2_similarity >> uniprot >> taxonomy
Use: Build protein families including remote homologs
```

## Test Cases

**Current Tests** (13 total):
- 5 declarative tests (lookup, attribute_exists checks)
- 8 custom tests (ID format, similarity data, cosine range, rank ordering, cross-references, top scores)

**Coverage**:
- UniProt ID format validation
- Similarity array presence and structure
- Cosine similarity range validation (0-1)
- Rank ordering verification (descending by score)
- Target protein information (UniProt ID)
- Cross-references to similar proteins
- Top/average score calculations

**Recommended Additions**:
- Similarity threshold filtering tests
- Multi-hop similarity chains (A→B→C)
- Species-specific filtering tests
- Comparison with sequence-based similarity

## Performance

- **Test Build**: ~10-30 seconds (100 proteins with 5000 total hits)
- **Data Source**: ESM2 embeddings in Qdrant vector database (~573K proteins)
- **Update Frequency**: When ESM2 embeddings are regenerated
- **Total Entries**: ~573,000 proteins
- **Storage**: ~1.5 GB TSV file, ~2-3 GB database size
- **Processing**: Streaming TSV parser handles 28M+ lines efficiently

## Known Limitations

- **Top-50 Only**: Each protein stores exactly top 50 similar proteins
- **Fixed Snapshot**: Similarity pre-computed, not real-time embedding search
- **No Sequence Alignment**: Pure embedding similarity (no alignment positions)
- **1.0 Similarity**: Identical sequences have 1.0 similarity (duplicates in UniProt)
- **UniProt Coverage**: Limited to proteins with ESM2 embeddings

## Future Work

- Add filtering by similarity threshold
- Implement bidirectional similarity verification
- Add protein family clustering from embeddings
- Enable multi-hop similarity searches
- Add comparison with DIAMOND sequence similarity
- Implement similarity confidence scores

## Maintenance

- **Release Schedule**: Updated when ESM2 embeddings are regenerated
- **Data Format**: TSV (4 columns: query_id, target_id, cosine_similarity, rank)
- **Test Data**: 100 proteins from esm2_similarities_top50.tsv (test mode)
- **License**: Data follows UniProt license terms
- **Dependencies**: Requires UniProt dataset for cross-references

## Regenerating the data (the full update procedure)

Unlike `diamond_similarity` (which ships a ready flat TSV at an
external snapshot path), the ESM2 similarity TSV is **generated on the biobtree side** by querying a
Qdrant vector DB. So an ESM2 update is a two-step flow:

**Prerequisite (upstream embedding pipeline):** the ESM2 embeddings are (re)loaded into a Qdrant
collection named `esm2` and the Qdrant server is started (default
`http://localhost:6333`). Verify it's ready:
```bash
curl -s http://localhost:6333/collections/esm2 | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['points_count'])"
# expect a green collection with ~574k points
```

**Step 1 — export top-K similarities from Qdrant → TSV.** The export script needs
`qdrant-client` + `tqdm` (install into whichever env runs it). It writes to the exact
path the conf's `useLocalFile` expects (`raw_data/esm2_similarity/esm2_similarities_top50.tsv`,
relative to the biobtree root), and is checkpoint/resumable. Run from the biobtree
root:
```bash
mkdir -p raw_data/esm2_similarity
python src/scripts/esm2/export_esm2_similarities.py \
  --qdrant-url http://localhost:6333 \
  --collection esm2 \
  --output raw_data/esm2_similarity/esm2_similarities_top50.tsv \
  --top-k 50 --workers 4 \
  --checkpoint raw_data/esm2_similarity/export.checkpoint
```
Takes a few hours (~50 proteins/s × ~574k). Output: `query_id\ttarget_id\tcosine_similarity\trank` (the format `esm2_similarity.go` parses).

**Step 2 — re-index (update only), then fold into the next main generate:**
```bash
./bb.sh out_prod --only esm2_similarity --force          # update phase only
# later, in the batched evening generate:
./bb.sh out_prod --generate --federation main
./bb.sh out_prod --activate --federation main             # then restart the web server
```

Notes:
- The conf `path` is the **local** `raw_data/esm2_similarity/esm2_similarities_top50.tsv`
  (not an external snapshot path), because the file is produced locally by the
  export above.
- `--checkpoint` lets a long export resume after an interruption.

## References

- **ESM2**: Lin Z, et al. (2023) Evolutionary-scale prediction of atomic level protein structure with a language model. Science. 379(6637):1123-1130.
- **Meta AI**: https://github.com/facebookresearch/esm
- **Qdrant**: https://qdrant.tech
- **UniProt**: https://www.uniprot.org
- **License**: Free for academic use (follows UniProt terms)
