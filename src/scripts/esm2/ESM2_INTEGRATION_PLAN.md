# ESM2 Similarity Dataset Integration Plan

## Overview

Integrate ESM2 protein embedding similarities into biobtree, enabling queries like:
```
P04637 >> esm2_similarity >> uniprot
```

**Data source**: Pre-exported TSV from Qdrant ESM2 collection
**Format**: `query_id	target_id	cosine_similarity	rank`
**Size**: ~573K proteins × 50 similar = ~28.6M rows

## Implementation Checklist

### Phase 1: Configuration

#### 1.1 Dataset ID
- **Next available ID**: `750`

#### 1.2 Add to `conf/source1.dataset.json`
```json
"esm2_similarity": {
  "id": "750",
  "name": "ESM2 Protein Similarity",
  "path": "raw_data/esm2_similarity/esm2_similarities_top50.tsv",
  "aliases": "esm2,esm-2,protein embedding,semantic similarity,embedding similarity",
  "url": "https://www.uniprot.org/uniprotkb/£{id}",
  "useLocalFile": "yes",
  "hasFilter": "yes",
  "attrs": "protein_id,[]similarities,similarity_count,top_similarity,avg_similarity",
  "test_entries_count": "100",
  "bucketMethod": "uniprot"
}
```

**Bucket method**: `uniprot` (same as protein_similarity, IDs are UniProt accessions like P04637)

---

### Phase 2: Protocol Buffers

#### 2.1 Add to `src/pbuf/attr.proto` (after ProteinSimilarityAttr ~line 1100)

```protobuf
// ESM2 embedding-based protein similarity
message Esm2Similarity {
  string target_uniprot = 1;    // Target UniProt ID
  float cosine_similarity = 2;  // Cosine similarity score (0-1)
  int32 rank = 3;               // Rank among similar proteins (1 = most similar)
}

message Esm2SimilarityAttr {
  string protein_id = 1;                      // Query UniProt ID
  repeated Esm2Similarity similarities = 2;  // Top N similar proteins
  int32 similarity_count = 3;                 // Number of similar proteins stored
  float top_similarity = 4;                   // Highest cosine similarity
  float avg_similarity = 5;                   // Average cosine similarity
}
```

#### 2.2 Add to `src/pbuf/app.proto` (in oneof attributes, after protein_similarity)

Find line with `ProteinSimilarityAttr protein_similarity=45;` and add after:
```protobuf
Esm2SimilarityAttr esm2_similarity=XX;  // Use next available number
```

#### 2.3 Compile Protocol Buffers
```bash
make proto
```

---

### Phase 3: Parser Implementation

#### 3.1 Create `src/update/esm2_similarity.go`

Following protein_similarity.go pattern:
- Stream TSV file line by line
- Group similarities by query protein
- Create Esm2SimilarityAttr with statistics
- Save with addProp3()
- Create xrefs with addXref()

**Key functions**:
```go
// Save entry with attributes
p.d.addProp3(proteinID, sourceID, marshaledAttr)

// Cross-references to UniProt
p.d.addXref(proteinID, sourceID, proteinID, "uniprot", false)
p.d.addXref(proteinID, sourceID, targetUniProt, "uniprot", false)

// Text search
p.d.addXref(proteinID, textLinkID, proteinID, p.source, true)

// Progress signal (REQUIRED at end)
p.d.progChan <- &progressInfo{dataset: p.source, done: true}
```

---

### Phase 4: Registration

#### 4.1 Update `src/update/update.go`

Add after protein_similarity case (~line 724):
```go
case "esm2_similarity":
    d.wg.Add(1)
    es := esm2Similarity{source: data, d: d}
    d.datasets2 = append(d.datasets2, data)
    go es.update()
    break
```

---

### Phase 5: Merge Logic

#### 5.1 Update `src/generate/mergeg.go`

Add after protein_similarity case (two locations ~line 1836 and ~line 2267):
```go
case "esm2_similarity":
    attr := &pbuf.Esm2SimilarityAttr{}
    barr := []byte((*kvProp[k])[0].value)
    ffjson.Unmarshal(barr, attr)
    xref.Attributes = &pbuf.Xref_Esm2Similarity{attr}
```

---

### Phase 6: Filter Support

#### 6.1 Update `src/service/service.go`

Add CEL type registration:
```go
cel.Types(&pbuf.Esm2SimilarityAttr{}),
```

Add CEL declaration:
```go
decls.NewIdent("esm2_similarity", decls.NewObjectType("pbuf.Esm2SimilarityAttr"), nil),
```

#### 6.2 Update `src/service/mapfilter.go`

Add filter evaluation case for esm2_similarity dataset.

---

### Phase 7: MCP Schema

#### 7.1 Update `mcp_srv/schema.py`

Add to SCHEMA_EDGES:
```python
"esm2_similarity": ["uniprot"],
```

Add to SCHEMA_FILTERS:
```python
"esm2_similarity": {
    "top_similarity": "float - highest cosine similarity (0-1)",
    "avg_similarity": "float - average cosine similarity",
    "similarity_count": "int - number of similar proteins"
}
```

Add to SCHEMA_EXAMPLES:
```python
"esm2_similarity": "P04637"
```

---

### Phase 8: Build and Test

```bash
# 1. Build
make proto
make build

# 2. Test update with sample data
./biobtree -d "esm2_similarity" update

# 3. Generate database
./biobtree --keep generate

# 4. Start web server
./biobtree web &

# 5. Test queries
curl "http://localhost:9292/ws/?i=P04637"
curl "http://localhost:9292/ws/map/?i=P04637&m=>>esm2_similarity>>uniprot"
```

---

### Phase 9: Tests (Optional)

Create `tests/datasets/esm2_similarity/`:
- `test_cases.json`
- `README.md`
- `esm2_similarity_ids.txt`

---

## Files to Modify Summary

| File | Action | Description |
|------|--------|-------------|
| `conf/source1.dataset.json` | ADD | Dataset configuration |
| `src/pbuf/attr.proto` | ADD | Esm2Similarity, Esm2SimilarityAttr messages |
| `src/pbuf/app.proto` | ADD | Add to oneof attributes |
| `src/update/esm2_similarity.go` | NEW | Parser implementation |
| `src/update/update.go` | ADD | Register dataset case |
| `src/generate/mergeg.go` | ADD | Unmarshal case (2 locations) |
| `src/service/service.go` | ADD | CEL type and declaration |
| `src/service/mapfilter.go` | ADD | Filter evaluation case |
| `mcp_srv/schema.py` | ADD | Edges, filters, examples |

---

## Comparison: protein_similarity vs esm2_similarity

| Aspect | protein_similarity (DIAMOND) | esm2_similarity (ESM2) |
|--------|------------------------------|------------------------|
| Method | Sequence alignment (BLASTP) | Embedding cosine similarity |
| Metrics | identity, bitscore, evalue | cosine_similarity, rank |
| Captures | Sequence homology | Functional/structural similarity |
| Use case | Orthologs, paralogs | Function prediction |

Both datasets are complementary - DIAMOND finds sequence-similar proteins, ESM2 finds functionally-similar proteins even without sequence homology.
