# Bucket Sort System

The bucket sort system provides optimized indexing for large datasets by distributing data across multiple bucket files, sorting them in parallel, and concatenating into a final sorted output.

## Architecture Overview

```
Processing Phase:
  Parser Goroutine 1 ──┐
  Parser Goroutine 2 ──┼──► Bucket Files (mutex per bucket)
  Parser Goroutine 3 ──┘    [bucket_000.txt, bucket_001.txt, ...]

Sorting Phase (parallel workers):
  Worker 1 ──► sorts bucket_000.txt
  Worker 2 ──► sorts bucket_001.txt
  ...
  Worker N ──► sorts bucket_N.txt

Concatenation Phase:
  All sorted buckets ──► dataset_sorted.X.index.gz
```

## Key Design Decisions

### Direct Mutex-Based Writes (No Worker Pool)

We use direct writes with per-bucket mutexes instead of a channel-based worker pool:

- **Why**: Channel/goroutine overhead was ~28 seconds in profiling
- **How**: Each parser goroutine writes directly to bucket files with mutex protection
- **Benefit**: Minimal overhead, simpler code, better performance

### Uncompressed During Processing

Bucket files are written uncompressed (`.txt`) during processing:

- **Why**: gzip compression was taking ~18% of CPU time in hot path
- **How**: Compression only happens during final concatenation phase
- **Benefit**: Faster processing, compression cost paid once at the end

### Pre-computed Bucket Keys

Bucket keys (`datasetID_bucketNum`) are pre-computed at initialization:

- **Why**: Avoids `fmt.Sprintf` on every write
- **How**: Keys stored in `bucketKeys` map, indexed by bucket number
- **Benefit**: Reduces allocations and CPU overhead

## Files

| File | Description |
|------|-------------|
| `bucket_config.go` | Configuration loading, link dataset mapping, system config |
| `bucket_methods.go` | Bucket assignment methods (numeric, uniprot, ontology, etc.) |
| `bucket_writer.go` | HybridWriterPool - direct write implementation |
| `bucket_sort.go` | Parallel sorting and concatenation |

## Configuration

Parameters in `conf/application.param.json`:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `bucketEnabled` | `yes` | Enable/disable bucket system |
| `bucketReadBufferSize` | `524288` | Read buffer size (512KB) |
| `bucketWriteBufferSize` | `65536` | Write buffer size (64KB) |
| `bucketSortWorkers` | `8` | Parallel workers for sorting |

Dataset-specific configuration in `conf/source.dataset.json`:

```json
{
  "taxonomy": {
    "id": "3",
    "bucketMethod": "numeric",
    "numBuckets": "50"
  },
  "dbsnp": {
    "id": "41",
    "bucketMethod": "rsid",
    "numBuckets": "100",
    "skipBucketSort": "yes"
  }
}
```

| Property | Default | Description |
|----------|---------|-------------|
| `bucketMethod` | - | Bucket assignment method (required for bucketing) |
| `numBuckets` | `100` | Number of bucket files to distribute data across |
| `skipBucketSort` | `no` | Skip sorting phase for datasets that run alone |

## Bucket Methods

| Method | ID Format | Example | Use Case |
|--------|-----------|---------|----------|
| `numeric` | Pure numbers | `9606` | taxonomy, ncbi_gene |
| `uniprot` | Letter+Digit prefix | `P12345` | uniprot (261 buckets) |
| `ontology` | PREFIX:NNNNN | `GO:0008150` | GO, HPO, MONDO, etc. |
| `mesh` | Letter+Numbers | `D000001` | MeSH descriptors |
| `alphabetic` | First letter A-Z | `BRCA1` | text search |
| `rsid` | rs + numbers | `rs123456789` | dbSNP variants |
| `hash` | Any string | fallback | generic fallback |

## Link Dataset Routing

Child/parent datasets (e.g., `hpoparent`, `hpochild`) route to their parent dataset's buckets:

```
hpoparent (ID:358) ──► hpo buckets (ID:58)
hpochild (ID:458)  ──► hpo buckets (ID:58)
```

This is configured via `linkdataset` property in `source.dataset.json` and handled by `linkDatasetMap` in `bucket_config.go`.

## Data Flow

### 1. Initialization
```go
LoadBucketSystemConfig()           // Load from application.param.json
bucketConfigs := LoadBucketConfigs() // Load from source.dataset.json
pool := NewHybridWriterPool(...)    // Create bucket files
```

### 2. Processing
```go
// From any parser goroutine:
pool.Write(datasetID, entityID, line)
// - Resolves link datasets
// - Calculates bucket number via Method()
// - Writes directly with mutex protection
// - Falls back to kvdatachan if no bucket config
```

### 3. Finalization
```go
pool.Close()                              // Flush and close all bucket files
SortAllBuckets(pool, 0)                   // Parallel sort with deduplication
ConcatenateBuckets(pool, indexDir, chunk) // Merge into compressed output
```

## Performance Characteristics

- **Memory**: ~512KB read buffer + 64KB write buffer per open file
- **Parallelism**: Sorting uses configurable worker count (default 8)
- **I/O**: Sequential writes during processing, parallel reads during sort
- **Compression**: Only at concatenation phase (gzip BestSpeed)

## Adding a New Dataset

1. Add `bucketMethod` and `numBuckets` to dataset in `source.dataset.json`:
   ```json
   "mydataset": {
     "id": "99",
     "bucketMethod": "numeric",
     "numBuckets": "50"
   }
   ```

2. If needed, add a new bucket method in `bucket_methods.go`:
   ```go
   func myBucket(id string, numBuckets int) int {
       // Return bucket number 0 to numBuckets-1
   }
   ```

3. Register in `BucketMethods` map:
   ```go
   var BucketMethods = map[string]BucketMethod{
       "mybucket": myBucket,
       // ...
   }
   ```
