# Biobtree Data Update State Management

## Overview

This document describes the complete flow of biobtree's data update system, including bucket-based processing, state management, and incremental update capabilities.

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           BIOBTREE DATA PIPELINE                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│   ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐ │
│   │  CHECK   │───▶│  UPDATE  │───▶│ GENERATE │───▶│   WEB    │───▶│  QUERY   │ │
│   │  PHASE   │    │  PHASE   │    │  PHASE   │    │  SERVER  │    │  API     │ │
│   └──────────┘    └──────────┘    └──────────┘    └──────────┘    └──────────┘ │
│        │               │               │                                        │
│        ▼               ▼               ▼                                        │
│   ┌──────────┐    ┌──────────┐    ┌──────────┐                                 │
│   │  Source  │    │  Bucket  │    │   LMDB   │                                 │
│   │  Change  │    │  Files   │    │ Database │                                 │
│   │Detection │    │  (.txt)  │    │  (.mdb)  │                                 │
│   └──────────┘    └──────────┘    └──────────┘                                 │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Directory Structure

```
out/
├── dataset_state.json              # Persistent state tracking
├── index/
│   ├── {dataset}/                  # Per-dataset bucket directories
│   │   ├── forward/                # Dataset's own entries
│   │   │   ├── bucket_000.txt
│   │   │   ├── bucket_001.txt
│   │   │   └── ...
│   │   └── from_{source}/          # Reverse xrefs from other datasets
│   │       ├── bucket_000.txt
│   │       └── ...
│   │
│   ├── _derived/                   # Derived datasets (textsearch)
│   │   └── textsearch/
│   │       └── from_{source}/
│   │           └── bucket_*.txt
│   │
│   ├── {dataset}_sorted.{chunkIdx}.index.gz           # Merged forward
│   └── {dataset}_from_{source}_sorted.{chunkIdx}.index.gz  # Merged reverse
│
└── db/
    ├── data.mdb                    # LMDB data file
    └── lock.mdb                    # LMDB lock file
```

---

## State Machine

```
                    ┌─────────────────────────────────────────┐
                    │           DATASET STATES                │
                    └─────────────────────────────────────────┘
                                      │
        ┌─────────────────────────────┼─────────────────────────────┐
        ▼                             ▼                             ▼
   ┌─────────┐                  ┌───────────┐                 ┌──────────┐
   │  (new)  │                  │processing │                 │  merged  │
   │         │                  │           │                 │          │
   └────┬────┘                  └─────┬─────┘                 └────┬─────┘
        │                             │                             │
        │  Start Update               │  Crash?                     │  New Batch
        │                             │                             │  Adds Reverse
        ▼                             ▼                             ▼
   ┌─────────┐                  ┌───────────┐                 ┌──────────┐
   │processing│────────────────▶│ CLEANUP   │                 │  Check   │
   │         │   On Recovery    │ Required  │                 │  from_*  │
   └────┬────┘                  └───────────┘                 └────┬─────┘
        │                                                          │
        │  Update Complete                              New sources found
        ▼                                                          │
   ┌─────────┐                                                     ▼
   │processed│──────────────────────────────────────────────▶┌──────────┐
   │         │                  Merge Phase                  │  Merge   │
   └────┬────┘                                               │  New     │
        │                                                    │  Sources │
        │  Merge Complete                                    └────┬─────┘
        ▼                                                         │
   ┌─────────┐◀───────────────────────────────────────────────────┘
   │ merged  │
   │         │
   └─────────┘
```

---

## Phase 1: Startup & Initialization

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         STARTUP PHASE                                            │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Load dataset_state.json      │
                    │  (or create if not exists)    │
                    └───────────────┬───────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Check for Interrupted        │
                    │  Datasets (status=processing) │
                    └───────────────┬───────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
                    ▼                               ▼
        ┌───────────────────┐           ┌───────────────────┐
        │  Found Interrupted │           │  No Interrupted   │
        │     Datasets       │           │     Datasets      │
        └─────────┬─────────┘           └─────────┬─────────┘
                  │                               │
                  ▼                               │
        ┌───────────────────┐                     │
        │  FOR EACH:        │                     │
        │  1. Log warning   │                     │
        │  2. Clean buckets │                     │
        │  3. Clean sorted  │                     │
        │  4. Remove state  │                     │
        └─────────┬─────────┘                     │
                  │                               │
                  └───────────────┬───────────────┘
                                  │
                                  ▼
                    ┌───────────────────────────────┐
                    │  Load Bucket Configurations   │
                    │  from Dataconf                │
                    └───────────────┬───────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Build Link Dataset Map       │
                    │  (child → parent mappings)    │
                    └───────────────┬───────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Initialize Bucket Writers    │
                    │  (lazy file creation)         │
                    └───────────────────────────────┘
```

---

## Phase 2: Update (Data Processing)

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         UPDATE PHASE (Per Dataset)                               │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Mark Status = "processing"   │
                    │  Save dataset_state.json      │
                    └───────────────┬───────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Cleanup Old Bucket Files     │
                    │  for this dataset             │
                    └───────────────┬───────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │  Clean:                       │
                    │  • {dataset}/forward/*        │
                    │  • {dataset}/from_*/*         │
                    │  • {dataset}_sorted.*.gz      │
                    │  • {dataset}_from_*.gz        │
                    └───────────────┬───────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Parse Source Data            │
                    │  (FTP download or local)      │
                    └───────────────┬───────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
                    ▼                               ▼
        ┌───────────────────┐           ┌───────────────────┐
        │  Forward XRefs    │           │  Reverse XRefs    │
        │                   │           │                   │
        │  Entry → Target   │           │  Target → Entry   │
        │                   │           │                   │
        │  addXref()        │           │  (automatic)      │
        │  addXrefBucketed()│           │                   │
        └─────────┬─────────┘           └─────────┬─────────┘
                  │                               │
                  ▼                               ▼
        ┌───────────────────┐           ┌───────────────────┐
        │  WriteForward()   │           │  WriteReverse()   │
        │                   │           │                   │
        │  {dataset}/       │           │  {target}/        │
        │   forward/        │           │   from_{dataset}/ │
        │    bucket_N.txt   │           │    bucket_N.txt   │
        └───────────────────┘           └───────────────────┘
                  │                               │
                  └───────────────┬───────────────┘
                                  │
                                  ▼
                    ┌───────────────────────────────┐
                    │  Mark Status = "processed"    │
                    │  Save dataset_state.json      │
                    └───────────────────────────────┘
```

---

## Phase 3: Generate (Sort & Merge)

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         GENERATE PHASE                                           │
└─────────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Enumerate All Datasets       │
                    │  with Bucket Configs          │
                    └───────────────┬───────────────┘
                                    │
                                    ▼
               ┌────────────────────────────────────────┐
               │         FOR EACH DATASET               │
               └────────────────────┬───────────────────┘
                                    │
                                    ▼
                    ┌───────────────────────────────┐
                    │  Check Dataset Status         │
                    └───────────────┬───────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
                    ▼                               ▼
        ┌───────────────────┐           ┌───────────────────┐
        │  Status: merged   │           │ Status: processed │
        │  (from prev batch)│           │  (current batch)  │
        └─────────┬─────────┘           └─────────┬─────────┘
                  │                               │
                  ▼                               │
        ┌───────────────────┐                     │
        │  Check for NEW    │                     │
        │  from_* sources   │                     │
        │  without .gz file │                     │
        └─────────┬─────────┘                     │
                  │                               │
        ┌─────────┴─────────┐                     │
        │                   │                     │
        ▼                   ▼                     │
   ┌─────────┐        ┌──────────┐                │
   │No new   │        │Found new │                │
   │sources  │        │from_*    │                │
   └────┬────┘        └────┬─────┘                │
        │                  │                      │
        │ Skip             │                      │
        │                  └──────────────────────┤
        │                                         │
        │                                         ▼
        │                         ┌───────────────────────────────┐
        │                         │  Get Bucket Files Per Source  │
        │                         │  (forward, from_*, ...)       │
        │                         └───────────────┬───────────────┘
        │                                         │
        │                                         ▼
        │                         ┌───────────────────────────────┐
        │                         │  FOR EACH SOURCE:             │
        │                         │  1. Sort bucket files         │
        │                         │  2. K-way merge               │
        │                         │  3. Write compressed .gz      │
        │                         │  4. Clean bucket files        │
        │                         └───────────────┬───────────────┘
        │                                         │
        │                                         ▼
        │                         ┌───────────────────────────────┐
        │                         │  Mark Status = "merged"       │
        │                         │  Save dataset_state.json      │
        │                         └───────────────┬───────────────┘
        │                                         │
        └─────────────────────────┬───────────────┘
                                  │
                                  ▼
                    ┌───────────────────────────────┐
                    │  Generate LMDB Database       │
                    │  from all .gz files           │
                    └───────────────────────────────┘
```

---

## Detailed: Sort & Merge Process

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    SORT & MERGE DETAIL (K-Way Merge)                             │
└─────────────────────────────────────────────────────────────────────────────────┘

    INPUT: Multiple unsorted bucket files

    bucket_000.txt          bucket_001.txt          bucket_019.txt
    ┌──────────────┐        ┌──────────────┐        ┌──────────────┐
    │ GO:0000005   │        │ GO:0000002   │        │ GO:0000001   │
    │ GO:0000003   │        │ GO:0000007   │        │ GO:0000004   │
    │ GO:0000009   │        │ GO:0000006   │        │ GO:0000008   │
    └──────────────┘        └──────────────┘        └──────────────┘
           │                       │                       │
           ▼                       ▼                       ▼
    ┌─────────────────────────────────────────────────────────────┐
    │                    STEP 1: SORT EACH FILE                    │
    │                    (In-place using sort command)             │
    └─────────────────────────────────────────────────────────────┘
           │                       │                       │
           ▼                       ▼                       ▼
    ┌──────────────┐        ┌──────────────┐        ┌──────────────┐
    │ GO:0000003   │        │ GO:0000002   │        │ GO:0000001   │
    │ GO:0000005   │        │ GO:0000006   │        │ GO:0000004   │
    │ GO:0000009   │        │ GO:0000007   │        │ GO:0000008   │
    └──────────────┘        └──────────────┘        └──────────────┘
           │                       │                       │
           └───────────────────────┼───────────────────────┘
                                   │
                                   ▼
    ┌─────────────────────────────────────────────────────────────┐
    │                    STEP 2: K-WAY MERGE                       │
    │                                                              │
    │   Readers:  [GO:0000003]  [GO:0000002]  [GO:0000001]        │
    │                                              ▲               │
    │                                              │               │
    │                                         MIN KEY              │
    │                                                              │
    │   Output: GO:0000001 → write to .gz                         │
    │           Advance reader 3                                   │
    │                                                              │
    │   Readers:  [GO:0000003]  [GO:0000002]  [GO:0000004]        │
    │                              ▲                               │
    │                              │                               │
    │                         MIN KEY                              │
    │                                                              │
    │   Output: GO:0000002 → write to .gz                         │
    │           Advance reader 2                                   │
    │                                                              │
    │   ... continue until all readers exhausted ...               │
    └─────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
    ┌─────────────────────────────────────────────────────────────┐
    │   OUTPUT: go_from_uniprot_sorted.123456.index.gz            │
    │                                                              │
    │   GO:0000001  70  P12345  1                                 │
    │   GO:0000002  70  Q67890  1                                 │
    │   GO:0000003  70  P11111  1                                 │
    │   GO:0000004  70  P22222  1                                 │
    │   ...                                                        │
    └─────────────────────────────────────────────────────────────┘
```

---

## Incremental Update Flow

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    INCREMENTAL UPDATE SCENARIO                                   │
└─────────────────────────────────────────────────────────────────────────────────┘

SCENARIO: Batch processing where GO is processed before UniProt

    ┌─────────────────────────────────────────────────────────────────────┐
    │                         BATCH 1                                      │
    │                                                                      │
    │   Process: GO (ontology only)                                       │
    │                                                                      │
    │   Creates:                                                          │
    │   • go/forward/bucket_*.txt     (GO hierarchy)                      │
    │   • go_sorted.*.index.gz        (merged)                            │
    │                                                                      │
    │   State: GO = "merged"                                              │
    └─────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ Time passes...
                                    ▼
    ┌─────────────────────────────────────────────────────────────────────┐
    │                         BATCH 2                                      │
    │                                                                      │
    │   Process: UniProt                                                  │
    │                                                                      │
    │   Creates:                                                          │
    │   • uniprot/forward/bucket_*.txt   (UniProt entries)                │
    │   • go/from_uniprot/bucket_*.txt   (GO ← UniProt reverse xrefs)     │
    │                                     ▲                               │
    │                                     │                               │
    │                              NEW REVERSE MAPPINGS                    │
    │                              (3.2 million lines)                     │
    │                                                                      │
    │   State: UniProt = "processed"                                      │
    │   State: GO = "merged" (unchanged)                                  │
    └─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
    ┌─────────────────────────────────────────────────────────────────────┐
    │                    GENERATE (with fix)                               │
    │                                                                      │
    │   For GO (status = "merged"):                                       │
    │   ┌─────────────────────────────────────────────────────────────┐   │
    │   │  Check for new from_* directories                           │   │
    │   │                                                             │   │
    │   │  Found: go/from_uniprot/ (no matching .gz file)            │   │
    │   │                                                             │   │
    │   │  Action: Merge only the new source                         │   │
    │   │  Output: go_from_uniprot_sorted.*.index.gz                 │   │
    │   └─────────────────────────────────────────────────────────────┘   │
    │                                                                      │
    │   For UniProt (status = "processed"):                               │
    │   ┌─────────────────────────────────────────────────────────────┐   │
    │   │  Normal full merge of all sources                          │   │
    │   │  Output: uniprot_sorted.*.index.gz                         │   │
    │   │          + all uniprot_from_*_sorted.*.index.gz            │   │
    │   └─────────────────────────────────────────────────────────────┘   │
    │                                                                      │
    │   RESULT: GO now has reverse mappings from UniProt!                 │
    └─────────────────────────────────────────────────────────────────────┘
```

---

## Bucket File Format

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    BUCKET FILE LINE FORMAT                                       │
└─────────────────────────────────────────────────────────────────────────────────┘

Forward XRef (addXref):
┌───────────────┬─────────────────┬───────────────┬──────────────┬──────────┬─────────────┐
│  SOURCE_ID    │  SOURCE_DATASET │  TARGET_ID    │ TARGET_DSID  │ EVIDENCE │ RELATIONSHIP│
│  (uppercase)  │     (ID)        │  (uppercase)  │   (number)   │ (opt)    │   (opt)     │
└───────────────┴─────────────────┴───────────────┴──────────────┴──────────┴─────────────┘

Example:
P38398	1	GO:0006281	70	IDA

Reverse XRef (automatic):
┌───────────────┬─────────────────┬───────────────┬──────────────┬──────────┬─────────────┐
│  TARGET_ID    │  TARGET_DATASET │  SOURCE_ID    │ SOURCE_DSID  │ EVIDENCE │ RELATIONSHIP│
│  (uppercase)  │     (ID)        │  (uppercase)  │   (number)   │ (opt)    │   (opt)     │
└───────────────┴─────────────────┴───────────────┴──────────────┴──────────┴─────────────┘

Example:
GO:0006281	70	P38398	1	IDA

Text Search Link:
┌───────────────┬─────────────────┬───────────────┬──────────────┐
│    KEYWORD    │   TEXT_LINK_ID  │  ENTRY_ID     │  DATASET_ID  │
│  (uppercase)  │      (0)        │  (uppercase)  │   (number)   │
└───────────────┴─────────────────┴───────────────┴──────────────┘

Example:
BRCA1	0	ENSG00000012048	2
```

---

## Bucket Method Selection

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    BUCKET METHOD BY DATASET                                      │
└─────────────────────────────────────────────────────────────────────────────────┘

Dataset         Method          Example ID           Bucket Calculation
─────────────────────────────────────────────────────────────────────────────────
uniprot         uniprot         P38398               Hash of accession → 0-260
go              go              GO:0006281           Numeric part after "GO:"
ensembl         ensembl_hybrid  ENSG00000012048      Prefix-based (6 sets)
chembl          chembl          CHEMBL25             Numeric part after prefix
mesh            mesh            D000001              Numeric part after letter
interpro        interpro        IPR000001            Numeric part after "IPR"
taxonomy        numeric         9606                 Direct numeric
pubchem         numeric         2244                 Direct numeric
reactome        reactome        R-HSA-12345          Numeric part after last "-"
clinical_trials nct             NCT06401707          Numeric part after "NCT"
efo/mondo/hpo   ontology        EFO:0000400          Hash-based
textsearch      alphabetic      BRCA1                First letter → 0-54

Bucket Count: Default 100 (some datasets override: uniprot=261, alphabetic=55)
```

---

## Cleanup Operations

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    CLEANUP SCENARIOS                                             │
└─────────────────────────────────────────────────────────────────────────────────┘

1. INCREMENTAL UPDATE CLEANUP (Before processing a dataset)
   ─────────────────────────────────────────────────────────
   When dataset X is being re-processed:

   Clean (X's own data and X's contributions TO others):
   ├── index/X/forward/*.txt           # X's own bucket files
   ├── index/X_sorted.*.index.gz       # X's own merged forward
   │
   Also clean X's contributions TO other datasets:
   ├── index/Y/from_X/*.txt            # X's reverse xrefs in Y
   ├── index/Y_from_X_sorted.*.index.gz
   └── ... for all datasets Y

   PRESERVED (contributions FROM other datasets TO X):
   ├── index/X/from_Y/*.txt            # Y's reverse xrefs in X (Y unchanged)
   └── index/X_from_Y_sorted.*.index.gz # Already merged from Y

   NOTE: When X is updated, contributions FROM other datasets (like Y) are
   preserved because Y hasn't changed. If Y also needs updating, run Y
   separately to refresh its contributions.


2. CRASH RECOVERY CLEANUP (On startup)
   ─────────────────────────────────────
   For datasets with status = "processing":

   Same as incremental update cleanup
   + Remove dataset from state


3. POST-MERGE CLEANUP (After successful merge)
   ────────────────────────────────────────────
   When keepBucketFiles = false:

   Clean:
   ├── index/X/forward/*.txt           # Bucket files now in .gz
   └── index/X/from_*/*.txt            # Bucket files now in .gz

   Keep:
   └── index/X/from_*/ directories     # Needed for incremental detection
```

---

## State File Schema

```json
{
  "datasets": {
    "go": {
      "name": "go",
      "id": "70",
      "status": "merged",
      "last_build_time": "2026-01-17T09:49:22Z",
      "source_url": "http://purl.obolibrary.org/obo/go.owl",
      "source_date": "2026-01-15T00:00:00Z",
      "source_size": 12345678,
      "kv_size": 2826320,
      "source_contributions": {
        "forward": 45000,
        "uniprot": 2500000,
        "ensembl": 281320
      },
      "build_duration_sec": 37.87
    },
    "uniprot": {
      "name": "uniprot",
      "id": "1",
      "status": "processed",
      "last_build_time": "2026-01-17T11:33:06Z",
      "source_url": "ftp://ftp.ebi.ac.uk/.../uniprot_sprot.xml.gz",
      "source_date": "2025-10-15T14:17:00Z",
      "source_size": 925885124,
      "kv_size": 77728075,
      "source_contributions": {
        "forward": 77728075
      },
      "build_duration_sec": 164.57
    }
  },
  "last_updated": "2026-01-17T11:33:06Z"
}
```

The `source_contributions` field tracks lines per source:
- `forward`: Dataset's own entries
- `{dataset_name}`: Reverse xrefs contributed by that dataset (e.g., `uniprot`, `ensembl`)

---

## Key Code Paths

| Operation | File | Function |
|-----------|------|----------|
| State loading | `dataset_state.go` | `LoadDatasetState()` |
| State saving | `dataset_state.go` | `SaveDatasetState()` |
| Crash recovery | `bucket_cleanup.go` | `CleanupInterruptedDatasets()` |
| Bucket writing | `bucket_writer.go` | `WriteForward()`, `WriteReverse()` |
| XRef creation | `update.go` | `addXref()`, `addXrefBucketed()` |
| Bucket sorting | `bucket_sort.go` | `SortBucketFile()` |
| K-way merge | `bucket_sort.go` | `concatenateSourceFiles()` |
| Incremental merge | `bucket_sort.go` | `concatenateOneDatasetIncremental()` |
| Cleanup | `bucket_cleanup.go` | `CleanupForIncrementalUpdate()` |

---

## Error Handling & Recovery

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    FAILURE SCENARIOS & RECOVERY                                  │
└─────────────────────────────────────────────────────────────────────────────────┘

1. CRASH DURING UPDATE
   ────────────────────
   State: dataset.status = "processing"

   Recovery:
   • On next startup, CleanupInterruptedDatasets() detects this
   • Cleans all bucket files and sorted files for the dataset
   • Removes dataset from state
   • Dataset will be fully re-processed


2. CRASH DURING MERGE
   ───────────────────
   State: dataset.status = "processed"

   Recovery:
   • Dataset status is still "processed"
   • Generate phase will re-merge from existing bucket files
   • No data loss (bucket files preserved until merge succeeds)


3. PARTIAL MERGE (some sources merged, then crash)
   ────────────────────────────────────────────────
   State: dataset.status = "processed"
   Some .gz files exist, some don't

   Recovery:
   • Generate phase checks for existing .gz files
   • Only merges sources without existing output files
   • Completes the partial merge


4. DISK FULL DURING WRITE
   ───────────────────────
   State: depends on when it happened

   Recovery:
   • Free disk space
   • If status = "processing": full re-process on restart
   • If status = "processed": re-run generate
```

---

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `keepBucketFiles` | `false` | Preserve bucket files after merge (debugging) |
| `bucketEnabled` | `true` | Enable bucket-based processing |
| `bucketReadBuffer` | 512KB | Buffer size for reading bucket files |
| `bucketWriteBuffer` | 64KB | Buffer size for writing bucket files |
| `bucketSortWorkers` | 8 | Parallel workers for sorting |
| `bucketConcatWorkers` | 4 | Parallel workers for merging |

---

## Summary

The biobtree data update system uses a **bucket-based approach** with **incremental update support**:

1. **Bucket files** distribute writes across multiple files to avoid memory issues
2. **Forward/Reverse separation** enables tracking where xrefs came from
3. **State tracking** enables crash recovery and incremental updates
4. **K-way merge** produces globally sorted output efficiently
5. **Incremental merge**: Already-merged datasets check for new reverse sources from later batches
6. **Source contributions**: Per-source line counts tracked in `source_contributions` field
7. **Preservation**: Reverse xrefs FROM other datasets are preserved during cleanup (not deleted)

This architecture enables processing of large datasets (billions of xrefs) with:
- Bounded memory usage
- Crash recovery
- Incremental updates
- Parallel processing
