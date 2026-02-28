# Xref Sorting System

This document explains the sorting system for cross-references (xrefs) in biobtree. Sorting applies to both forward and reverse xrefs.

## Overview

When querying xrefs (e.g., `hsa-miR-21-5p >> mirdb >> refseq` or `UBERON:0000955 >> uberon >> bgee`), results can be sorted by multiple levels such as species priority and interaction/expression scores. This ensures that human data appears first, and within each species, results are sorted by relevance metrics.

## Architecture

The sorting system has **4 phases**:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ PHASE 1: PARSER (e.g., mirdb.go, bgee.go)                                   │
│ - Parser has actual data (taxID, score)                                     │
│ - Calls ComputeSortLevelValue() to convert to sortable strings              │
│ - Calls addXrefWithSortLevels() to write xref with sort fields appended     │
└─────────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│ PHASE 2: BUCKET SORTING (bucket_sort.go - SortAllBuckets)                   │
│ - Each bucket file sorted independently                                     │
│ - Sort by: key (field 1) + sort level fields (last N fields)               │
│ - Sort fields NOT stripped yet (needed for merge)                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│ PHASE 3: K-WAY MERGE (bucket_sort.go - kWayMergeWithStripping)              │
│ - Multiple sorted bucket files merged into one                              │
│ - Comparison uses extractSortKey(): key + sort levels only                  │
│ - Sort fields STRIPPED during output                                        │
└─────────────────────────────────────────────────────────────────────────────┘
                                    ↓
┌─────────────────────────────────────────────────────────────────────────────┐
│ PHASE 4: FINAL INDEX                                                        │
│ - Clean 4-field format: KEY <tab> FROM <tab> VALUE <tab> DATASETID          │
│ - No sort fields remain                                                     │
│ - Results pre-sorted for queries                                            │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Configuration

### Config Format

Sorting is configured per-dataset in `conf/source.dataset.json` using the `xrefSort` field:

```json
{
  "mirdb": {
    "xrefSort": "refseq:speciesPriority,interactionScore"
  },
  "bgee": {
    "xrefSort": "uberon:speciesPriority,expressionScore;cl:speciesPriority,expressionScore"
  }
}
```

Format: `"target1:level1,level2;target2:level1,level2"`

- **target**: The dataset being mapped TO (e.g., `refseq` in `mirdb >> refseq`)
- **levels**: Comma-separated sort level names

### IMPORTANT: Config Names Are Cosmetic!

The sort level names in config (`speciesPriority`, `expressionScore`, etc.) are **only for human readability**. The system only uses the **COUNT** of sort levels.

These two configs work **identically**:
```json
"xrefSort": "refseq:speciesPriority,interactionScore"
"xrefSort": "refseq:banana,potato"
```

Both tell the system: "refseq xrefs have 2 sort fields to process"

The actual sort values must be computed by the parser code manually using `ComputeSortLevelValue()`.

## Phase 1: Parser - Writing Sort Values

### Available Sort Level Types

Defined in `update.go`:

```go
const (
    SortLevelSpeciesPriority  = "speciesPriority"   // taxID → "01", "02", etc.
    SortLevelExpressionScore  = "expressionScore"   // 0-100 → inverted
    SortLevelCellCount        = "cellCount"         // count → inverted
    SortLevelInteractionScore = "interactionScore"  // 0-1000 → inverted
)
```

### ComputeSortLevelValue Function

Converts actual data values to sortable strings (ascending lexicographic):

| Type | Input | Output | Logic |
|------|-------|--------|-------|
| `speciesPriority` | taxID "9606" | "01" | Human=01, Mouse=02, etc. |
| `speciesPriority` | taxID "12345" | "99" | Unknown species last |
| `expressionScore` | score 95.11 | "004.89" | 100 - score |
| `interactionScore` | score 950 | "0050" | 1000 - score |
| `cellCount` | count 1000000 | "999999999000" | max - count |

### Example: mirdb.go

```go
// Parser has actual data from file
taxID := "9606"           // from parsed line
score := 98.6             // from parsed line

// Convert score to int (0-1000 range)
scoreInt := int(math.Round(float64(score) * 10))  // 986

// Compute sort values
sortLevels := []string{
    ComputeSortLevelValue(SortLevelSpeciesPriority,
        map[string]interface{}{"taxID": taxID}),      // → "01"
    ComputeSortLevelValue(SortLevelInteractionScore,
        map[string]interface{}{"score": scoreInt}),   // → "0014" (1000-986)
}

// Write xref with sort fields
m.d.addXrefWithSortLevels(mirnaID, sourceID, refseqID, "refseq", sortLevels)
```

### Data Format Written to Bucket

```
HSA-MIR-21-5P <tab> 89 <tab> NM_003373 <tab> 42 <tab> 01 <tab> 0014
     ↑              ↑           ↑           ↑        ↑        ↑
   key          source       value      target   species   score
  (field1)     (field2)    (field3)   (field4)  (field5) (field6)
                                                 └──── sort levels ────┘
```

## Phase 2: Bucket Sorting

### What Happens

Each bucket file is sorted independently using Unix `sort` command (or in-memory for small files).

### Dynamic Sort Spec Generation

```go
func generateDynamicSortSpec(totalFields, sortLevelCount int) (string, int) {
    baseFields := totalFields - sortLevelCount  // e.g., 6 - 2 = 4
    sortLevelStart := baseFields + 1            // e.g., 5

    // Build sort spec: key first, then sort level fields
    sortSpec := "-k1,1"
    for i := 0; i < sortLevelCount; i++ {
        fieldPos := sortLevelStart + i
        sortSpec += fmt.Sprintf(" -k%d,%d", fieldPos, fieldPos)
    }

    return sortSpec, sortLevelStart
    // Returns: "-k1,1 -k5,5 -k6,6", stripField=5
}
```

### Example Sort Command

For 6-field data with 2 sort levels:
```bash
sort -t$'\t' -k1,1 -k5,5 -k6,6 bucket_file.txt
```

This sorts by:
1. Key (field 1) - alphabetically
2. Species priority (field 5) - "01" before "02" before "99"
3. Score (field 6) - "0014" before "0050" (lower = higher original score)

### Sort Levels NOT Stripped Yet

After bucket sorting, files still have all 6 fields. Sort levels are kept because they're needed for the merge phase.

## Phase 3: K-Way Merge with Stripping

### Why K-Way Merge?

Multiple bucket files need to be merged into a single sorted output. Since each bucket is already sorted, we use k-way merge (heap-based) for efficiency.

### The Key Insight: extractSortKey()

During merge, we compare lines to maintain sort order. But we can't compare full lines because middle fields (source, value, target IDs) would affect ordering.

```go
func extractSortKey(line string, baseFields int) string {
    fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")

    if len(fields) <= baseFields {
        return fields[0]  // Just key if no sort levels
    }

    // Build: key + sort level fields only
    var sb strings.Builder
    sb.WriteString(fields[0])              // Key
    for i := baseFields; i < len(fields); i++ {
        sb.WriteByte('\t')
        sb.WriteString(fields[i])          // Sort levels
    }
    return sb.String()
}
```

### Example Comparison

Given two lines:
```
HSA-MIR-21-5P <tab> 89 <tab> NM_003373 <tab> 42 <tab> 01 <tab> 0014
HSA-MIR-21-5P <tab> 89 <tab> NM_001276320 <tab> 42 <tab> 01 <tab> 0021
```

Full line comparison would incorrectly order by `NM_001276320` vs `NM_003373`.

extractSortKey extracts:
```
HSA-MIR-21-5P <tab> 01 <tab> 0014
HSA-MIR-21-5P <tab> 01 <tab> 0021
```

Now "0014" < "0021", so NM_003373 (score 98.6) correctly comes before NM_001276320 (score 97.9).

### Stripping During Output

As lines are written to the merged output, sort fields are stripped:

```go
func kWayMergeWithStripping(outputPath string, sortedFiles []string, baseFields int) {
    // ... heap-based merge ...

    for heap.Len() > 0 {
        minEntry := heap.Pop()
        line := minEntry.line

        // Strip sort levels: keep only first baseFields
        fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
        if len(fields) > baseFields {
            line = strings.Join(fields[:baseFields], "\t") + "\n"
        }

        writer.WriteString(line)
    }
}
```

## Phase 4: Final Index

After merge, the index contains clean 4-field format:

```
HSA-MIR-21-5P <tab> 89 <tab> NM_003373 <tab> 42
HSA-MIR-21-5P <tab> 89 <tab> NM_001276320 <tab> 42
```

Results are pre-sorted: human data first, higher scores first.

## Handling Different Field Counts

The system handles various scenarios:

### Standard Xref (no sorting, no evidence)
```
4 fields: KEY <tab> FROM <tab> VALUE <tab> DATASETID
```

### Xref with Evidence (no sorting)
```
6 fields: KEY <tab> FROM <tab> VALUE <tab> DATASETID <tab> EVIDENCE <tab> RELATIONSHIP
```

### Xref with Sorting (no evidence)
```
6 fields: KEY <tab> FROM <tab> VALUE <tab> DATASETID <tab> SORT1 <tab> SORT2
```

### Xref with Evidence AND Sorting
```
8 fields: KEY <tab> FROM <tab> VALUE <tab> DATASETID <tab> EVIDENCE <tab> REL <tab> SORT1 <tab> SORT2
```

### Dynamic Detection

The system detects field count from actual data and uses config to determine sort level count:

```go
sortLevelCount := GetSortLevelCount(datasetName)
baseFields := totalFields - sortLevelCount
```

## Dummy Sort Values

For datasets with sorting configured, ALL xrefs (even entries) need consistent field counts. The system adds dummy sort values ("0000") to non-sorted xrefs:

```go
func GetDummySortValues(datasetName string) string {
    count := GetSortLevelCount(datasetName)
    if count == 0 {
        return ""
    }
    result := ""
    for i := 0; i < count; i++ {
        result += "\t0000"
    }
    return result  // e.g., "\t0000\t0000" for 2 sort levels
}
```

This ensures entry lines (which don't have real sort values) sort before xrefs (which have "01", "02", etc.):
```
HSA-MIR-21-5P <tab> 89 <tab> ... <tab> 0000 <tab> 0000   ← Entry (first)
HSA-MIR-21-5P <tab> 89 <tab> ... <tab> 01 <tab> 0014    ← Xref (after)
```

## Species Priority Map

```go
var modelSpeciesPriority = map[string]string{
    "9606":  "01", // Human (Homo sapiens)
    "10090": "02", // Mouse (Mus musculus)
    "10116": "03", // Rat (Rattus norvegicus)
    "7955":  "04", // Zebrafish (Danio rerio)
    "7227":  "05", // Fruit fly (Drosophila melanogaster)
    "6239":  "06", // Nematode (Caenorhabditis elegans)
    "4932":  "07", // Yeast (Saccharomyces cerevisiae)
    "3702":  "08", // Arabidopsis (Arabidopsis thaliana)
}
// All other species → "99"
```

## Key Files

| File | Purpose |
|------|---------|
| `update.go` | `SortLevelType` constants, `ComputeSortLevelValue()`, `addXrefWithSortLevels()`, species priority map |
| `bucket_sort.go` | `SortAllBuckets()`, `generateDynamicSortSpec()`, `extractSortKey()`, `kWayMergeWithStripping()` |
| `bucket_config.go` | `LoadXrefSortConfigs()`, `GetXrefSortLevels()`, `GetSortLevelCount()`, `GetDummySortValues()` |

## Adding Sorting to New Dataset

1. **Add config** in `conf/source.dataset.json`:
   ```json
   "your_dataset": {
     "xrefSort": "target_dataset:level1,level2"
   }
   ```
   (Level names are cosmetic - just use count you need)

2. **Compute sort values** in your parser:
   ```go
   sortLevels := []string{
       ComputeSortLevelValue(SortLevelSpeciesPriority,
           map[string]interface{}{"taxID": taxID}),
       ComputeSortLevelValue(SortLevelInteractionScore,
           map[string]interface{}{"score": scoreInt}),
   }
   ```

3. **Use addXrefWithSortLevels**:
   ```go
   d.addXrefWithSortLevels(sourceID, sourceDatasetID, targetID, "target_dataset", sortLevels)
   ```

4. **Rebuild** the dataset

## Common Issues

### Float Rounding
Use `math.Round()` when converting float scores to int:
```go
// WRONG: int(98.6 * 10) = 985 (truncation)
// RIGHT: int(math.Round(98.6 * 10)) = 986
scoreInt := int(math.Round(score * 10))
```

### Inconsistent Field Counts
All xrefs for a dataset must have same field count. Use `GetDummySortValues()` for entries/non-sorted xrefs.

### Sort Level Mismatch
Config count must match what parser writes. If config says 2 levels but parser writes 1, sorting breaks.
