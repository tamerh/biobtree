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

## References

- **ESM2**: Lin Z, et al. (2023) Evolutionary-scale prediction of atomic level protein structure with a language model. Science. 379(6637):1123-1130.
- **Meta AI**: https://github.com/facebookresearch/esm
- **Qdrant**: https://qdrant.tech
- **UniProt**: https://www.uniprot.org
- **License**: Free for academic use (follows UniProt terms)
