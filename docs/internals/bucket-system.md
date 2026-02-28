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

Concatenation Phase (per-source files):
  entrez/forward/       ──► entrez_sorted.X.index.gz
  entrez/from_refseq/   ──► entrez_from_refseq_sorted.X.index.gz
  refseq/forward/       ──► refseq_sorted.X.index.gz
  refseq/from_entrez/   ──► refseq_from_entrez_sorted.X.index.gz
```

## Per-Source File Design

Each dataset's output is split into separate files by source, enabling granular incremental updates:

| Source Directory | Output File | Description |
|-----------------|-------------|-------------|
| `{dataset}/forward/` | `{dataset}_sorted.X.index.gz` | Dataset's own entries |
| `{dataset}/from_{source}/` | `{dataset}_from_{source}_sorted.X.index.gz` | Xrefs from another dataset |
| `_derived/textsearch/from_{source}/` | `textsearch_{source}_sorted.X.index.gz` | Text search entries |

**Incremental Update Example** - When entrez needs update:
```bash
# Automatically cleaned by CleanupForIncrementalUpdate():
rm entrez_sorted.*.index.gz              # entrez's own data
rm textsearch_entrez_sorted.*.index.gz   # entrez's textsearch contribution
rm *_from_entrez_sorted.*.index.gz       # entrez's xrefs TO other datasets

# Then only entrez is re-processed - other datasets keep their data
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
| `numeric` | Pure numbers | `9606` | taxonomy, ncbi_gene, pubmed |
| `uniprot` | Letter+Digit prefix | `P12345` | uniprot (261 buckets) |
| `ontology` | PREFIX:NNNNN | `GO:0008150` | chebi, hpo, uberon, efo, mondo, etc. |
| `mesh` | Letter+Numbers | `D000001` | MeSH descriptors |
| `alphabetic` | First letter A-Z | `BRCA1` | text search, gcst |
| `alphanum` | First alphanumeric | `A123`, `9XYZ` | generic alphanumeric |
| `rsid` | rs + numbers | `rs123456789` | dbSNP variants |
| `chembl` | CHEMBL + numbers | `CHEMBL123456` | chembl_molecule, chembl_activity, etc. |
| `go` | GO:NNNNNNN | `GO:0008150` | Gene Ontology |
| `hmdb` | HMDB + numbers | `HMDB0000001` | HMDB metabolites |
| `nct` | NCT + numbers | `NCT06401707` | Clinical trials |
| `rhea` | RHEA:NNNN | `RHEA:16066` | Rhea reactions |
| `reactome` | R-XXX-NNNN | `R-HSA-12345` | Reactome pathways |
| `gwas` | GCST + numbers | `GCST000001_rs380390` | GWAS associations |

## Multi-Bucket-Set Routing

For datasets with mixed ID formats (e.g., patents), multiple bucket methods can be specified as a comma-separated list. The system tries each method in order and uses the first one that matches:

```json
{
  "patent": {
    "id": "26",
    "bucketMethod": "patent_us,patent_ep,patent_wo,patent_other"
  }
}
```

This produces separate sorted output files for each bucket set:
- `patent_sorted_1.X.index.gz` (US patents)
- `patent_sorted_2.X.index.gz` (EP patents)
- `patent_sorted_3.X.index.gz` (WO patents)
- `patent_sorted_4.X.index.gz` (Other patents)

Each bucket method returns `-1` if the ID doesn't match its pattern, causing the system to try the next method.

### Patent Bucket Methods

| Method | Pattern | Example | Description |
|--------|---------|---------|-------------|
| `patent_us` | US-XXXXX-X | `US-5153197-A` | US patents (alphanumeric on part after "US-") |
| `patent_ep` | EP-XXXXX-X | `EP-1234567-A1` | European patents |
| `patent_wo` | WO-XXXXX-X | `WO-2020123456-A1` | WIPO patents |
| `patent_other` | Other formats | `CA-2987654-A1`, `RE43229` | All other patents (alphabetic) |

## Link Dataset Routing

Child/parent datasets (e.g., `hpoparent`, `hpochild`) route to their parent dataset's buckets:

```
hpoparent (ID:358) ──► hpo buckets (ID:58)
hpochild (ID:458)  ──► hpo buckets (ID:58)
```

This is configured via `linkdataset` property in `source.dataset.json` and handled by `linkDatasetMap` in `bucket_config.go`.

### Link Datasets with Own Bucket Config

Link datasets can override the parent's bucket routing by specifying their own `bucketMethod`. This is useful when the link dataset uses different ID formats than the parent:

```json
{
  "patent_compound": {
    "id": "352",
    "linkdataset": "chembl_molecule",
    "bucketMethod": "numeric"
  }
}
```

In this example, `patent_compound` is a link dataset to `chembl_molecule`, but uses numeric compound IDs (e.g., `15766161`) instead of ChEMBL IDs. By specifying its own `bucketMethod: "numeric"`, it routes to its own buckets instead of the parent's ChEMBL buckets.

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
pool.WriteForward(datasetID, datasetName, entityID, line)  // Write to {dataset}/forward/
pool.WriteReverse(targetDatasetID, entityID, line, sourceDatasetName)  // Write to {dataset}/from_{source}/
// - Resolves link datasets
// - Calculates bucket number via Method()
// - Creates directories lazily
// - Writes directly with mutex protection
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
