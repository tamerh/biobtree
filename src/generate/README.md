# Generate Package - K-Way Merge Implementation

## Overview

The `generate` package handles the final merge phase of biobtree's data processing pipeline. It performs a k-way merge of thousands of sorted chunk files into a single LMDB database.

## Architecture

### Problem Statement

When processing large datasets (e.g., 7.8 billion KV lines across 2558 files), the naive approach of one goroutine per file with dedicated buffers would require:
- 2558 files × 40MB buffer = ~102GB memory

### Solution: Worker-Based K-Way Merge

Instead of per-file goroutines, we use a worker pool architecture:
- **8 workers × 40MB = ~320MB** for read buffers
- Shared buffer pool via `sync.Pool`
- Min-heap for efficient minimum key finding

## Key Components

### 1. fileState
Minimal per-file state without permanent buffer:
```go
type fileState struct {
    file      *os.File
    gz        *gzip.Reader
    r         *bufio.Reader
    curKey    string        // Current minimum key for this file
    nextLine  [5]string     // Buffered line data
    eof       bool
    complete  bool
    heapIndex int           // Position in heap
}
```

### 2. fileHeap (Min-Heap)
Implements `container/heap.Interface` for O(log n) minimum key operations:
- `Push/Pop`: O(log n)
- `Fix`: O(log n) after updating a key
- Always yields the file with the smallest current key

### 3. workerPool
Fixed number of workers sharing a buffer pool:
```go
type workerPool struct {
    numWorkers int
    bufferPool *sync.Pool   // Shared 40MB buffers
    jobCh      chan readJob
}
```

### 4. readJob
Work unit for reading next key from a file:
```go
type readJob struct {
    fs          *fileState
    skipUntil   string       // For checkpoint resume
    resultCh    chan<- *fileState
    mergeCh     *chan kvMessage
    initialRead bool         // First read: buffer only, don't send to merge
}
```

## Merge Algorithm

```
1. Initialize all files, read first key from each (initialRead=true)
2. Build min-heap from all fileStates
3. Loop until heap empty:
   a. Pop minimum key from heap
   b. Collect ALL files with that same key
   c. Send buffered data to merge channel
   d. Submit read jobs to worker pool (read next key)
   e. Wait for results, update heap
   f. Batch write to LMDB when batch full
4. Flush final batch, close database
```

## Memory Management

| Component | Old Approach | New Approach | Savings |
|-----------|--------------|--------------|---------|
| Read buffers | 2558 × 40MB = 102GB | 8 × 40MB = 320MB | ~100GB |
| Merge arrays | 720,000 entries = 11.8GB | 50,000 entries = ~800MB | ~11GB |
| Total | ~115GB | ~1.5GB | ~113GB |

**Key insight:** The worker pool is the critical optimization (~90% of savings).
We don't need 2558 concurrent readers - just enough workers to saturate I/O
while processing keys sequentially. The heap provides CPU efficiency (O(log n)
vs O(n) for finding minimum) but doesn't impact memory.

## Checkpoint/Resume

The merge supports resuming from checkpoints:
- Saves progress every N keys (configurable)
- Stores: last key, file positions, total counts
- On resume: skips keys ≤ checkpoint key

## Configuration

Key parameters in `application.param.json`:
- `tmpRuneSize`: Buffer size per worker (default: 10,000,000 runes = 40MB)
- `batchSize`: LMDB write batch size (default: 100,000)
- `mergeCheckpointInterval`: Keys between checkpoints (default: 100,000)

## Performance Notes

- LMDB writes use append mode (0x10) for sequential key insertion
- Workers read compressed gzip files on-the-fly
- Heap operations are O(log n) where n = number of active files
- Files are closed immediately when exhausted to release resources
