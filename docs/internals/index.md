# Technical Internals

This section contains deep technical documentation for developers working on biobtree's core systems.

## Architecture Documents

| Document | Description |
|----------|-------------|
| [K-Way Merge](k-way-merge.md) | Generate phase: merging thousands of sorted files with bounded memory |
| [Bucket System](bucket-system.md) | Update phase: distributing data across bucket files for parallel sorting |
| [State Management](state-management.md) | Complete data pipeline with state tracking and crash recovery |
| [Sorting System](sorting-system.md) | Xref sorting by species priority and scores |

## Quick Overview

### Data Pipeline

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│  UPDATE  │───▶│   SORT   │───▶│  MERGE   │───▶│   WEB    │
│  PHASE   │    │  PHASE   │    │  PHASE   │    │  SERVER  │
└──────────┘    └──────────┘    └──────────┘    └──────────┘
     │               │               │
     ▼               ▼               ▼
  Bucket          Sorted          LMDB
  Files           Files          Database
```

### Key Optimizations

- **Memory Efficiency**: Worker pool with 8 workers × 40MB buffers (vs 100GB+ naive approach)
- **Crash Recovery**: Three-state tracking (processing → processed → merged)
- **Incremental Updates**: Only rebuild changed datasets
- **Parallel Processing**: Bucket files enable parallel sorting

## Source Code Location

These documents are also maintained alongside the source code:

- `src/generate/README.md` - K-Way Merge
- `src/update/BUCKET_SYSTEM.md` - Bucket System
- `src/update/DATA_UPDATE_STATE_MANAGEMENT.md` - State Management
- `src/update/SORTING_SYSTEM.md` - Sorting System
