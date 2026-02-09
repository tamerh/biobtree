# Biobtree Dataset Integration Agent

You are a specialized agent for integrating new datasets into biobtree. Before starting any work, read and follow these instructions completely.

## Input
Dataset name or description: $ARGUMENTS

## Pre-Work: Research Phase

### Step 1: Scan Available Data Sources
Before writing any code, thoroughly research the dataset:

1. **Find Official Data Source**
   - Search for the dataset's official website and documentation
   - Identify all available download formats and locations

2. **Data Source Priority** (STRICT ORDER):
   - **First preference: FTP** - If FTP location exists, use it (most reliable for large files)
   - **Second preference: HTTP/HTTPS** - Use if no FTP available
   - Check existing biobtree parsers in `src/update/*.go` for similar patterns

3. **Analyze Data Format**
   - If multiple formats available (XML, JSON, TSV, RDF, etc.), choose the most efficient:
     - TSV/CSV: Fastest parsing, lowest memory
     - JSON: Good for nested data, moderate efficiency
     - XML: Use for complex hierarchical data
     - RDF: Use only if no other format available
   - Document format choice rationale

4. **Data Completeness Assessment**
   - Identify ALL available data files from the source
   - List what data can be included vs what must be excluded
   - Biobtree can handle very large datasets - include as much as possible

## Implementation Checklist

Work through these steps sequentially. Use TodoWrite to track progress.

### Phase 1: Configuration

#### 1.1 Find Next Available Dataset ID
```bash
# Check existing IDs in source.dataset.json to find next available
grep '"id":' conf/source.dataset.json conf/default.dataset.json | sort -t'"' -k4 -n
```

#### 1.2 Add to `conf/source.dataset.json`
Required fields:
```json
{
  "datasetname": {
    "id": "<next_available_id>",
    "name": "DatasetName",
    "path": "<ftp_or_http_path>",
    "aliases": "alias1,alias2",
    "url": "https://example.org/entry/£{id}",
    "useLocalFile": "no",
    "hasFilter": "yes",
    "attrs": "field1,field2,[]arrayField",
    "test_entries_count": "100",
    "bucketMethod": "<appropriate_method>"
  }
}
```

#### 1.2.1 Choosing the Right Bucket Method

**CRITICAL: Bucket method preserves lexicographic order for k-way merge. Choose carefully!**

Study existing buckets in `src/update/bucket_methods.go` and follow conventions:

**By ID Pattern - Choose Matching Existing Method:**

| ID Format | Bucket Method | Example | Buckets |
|-----------|---------------|---------|---------|
| Pure numeric | `numeric` | `9606`, `12345` | 100 (first 2 digits) |
| PREFIX:NUMBER | `ontology` | `HP:0001234`, `CHEBI:12345`, `UBERON:0000001` | 100 |
| GO:XXXXXXX | `go` | `GO:0008150` | 100 |
| Letter+digits | `alphanum` | `HGNC:1234`, `D000001` | 37 fixed |
| Alphabetic start | `alphabetic` | `TP53`, `BRCA1` | 55 fixed |
| PREFIX+numeric | See specific methods below | | |

**Dataset-Specific Methods (use if ID format matches):**

| Method | ID Format | Example |
|--------|-----------|---------|
| `uniprot` | Letter+digit start | `P12345`, `Q9Y6K9` |
| `upi` | UPI+hex | `UPI00000001A2` |
| `rnacentral` | URS+hex | `URS000149A9AF` |
| `rsid` | rs+numeric | `rs123456789` |
| `chembl` | CHEMBL variants | `CHEMBL123456`, `CHEMBL_ACT_93229` |
| `interpro` | IPR+numeric | `IPR000001` |
| `hmdb` | HMDB+numeric | `HMDB0000001` |
| `nct` | NCT+numeric | `NCT06401707` |
| `lipidmaps` | LM+2char+numeric | `LMFA00000001` |
| `rhea` | RHEA:numeric | `RHEA:16066` |
| `reactome` | R-XXX-numeric | `R-HSA-12345` |
| `mesh` | Letter+numeric | `D000001`, `C012345` |
| `gwas` | GCST+numeric | `GCST000001_rs380390` |
| `string` | taxid.protein | `9606.ENSP00000377769` |
| `ensembl_hybrid` | Ensembl gene prefixes | `ENSG00000139618` |

**Decision Tree for New Datasets:**

```
1. Does ID format match an EXISTING bucket method?
   → YES: Use that method (follow convention!)
   → NO: Continue to step 2

2. What is the ID structure?
   → Pure numeric (e.g., 12345): Use "numeric"
   → PREFIX:NUMBER (e.g., ABC:12345): Use "ontology"
   → Prefix+numeric (e.g., ABC12345): Consider creating new method OR use "alphanum"
   → Alphanumeric mixed: Use "alphanum" (37 buckets)
   → Text/keywords: Use "alphabetic" (55 buckets)

3. For large datasets (>10M entries), consider:
   → Creating a dataset-specific bucket method in bucket_methods.go
   → This ensures optimal distribution and avoids bucket hotspots
```

**Creating a New Bucket Method (if needed):**

Add to `src/update/bucket_methods.go`:
```go
// datasetnameBucket - DATASETNAME_PREFIX+numeric → numeric part after prefix
func datasetnameBucket(id string, numBuckets int) int {
    if len(id) < 4 || !strings.HasPrefix(id, "PREFIX") {
        panic("datasetnameBucket: invalid format: " + id)
    }
    return numericLexBucket(id[6:], numBuckets) // 6 = len("PREFIX")
}
```

Register in `BucketMethods` map:
```go
var BucketMethods = map[string]BucketMethod{
    // ... existing methods ...
    "datasetname": datasetnameBucket,
}
```

**Fixed Bucket Counts (cannot be changed):**
- `alphabetic`: 55 buckets
- `alphanum`: 37 buckets
- `uniprot`: 261 buckets
- `upi`: 256 buckets
- `rnacentral`: 256 buckets

#### 1.3 Add to `conf/xref1.dataset.json` (if creates derived xrefs)
Add entries for datasets that appear only via cross-references.

### Phase 2: Protocol Buffers

#### 2.1 Define Attribute Message in `src/pbuf/attr.proto`
```protobuf
message DatasetNameAttr {
  string name = 1;
  string description = 2;
  repeated string synonyms = 3;
  // Add all fields that should be stored and filterable
}
```

Follow existing patterns - check similar datasets in attr.proto.

#### 2.2 Compile Protocol Buffers
```bash
make proto
```

### Phase 3: Parser Implementation

#### 3.1 Create Parser in `src/update/datasetname.go`

**CRITICAL PATTERNS TO FOLLOW:**

1. **Stream Processing** - Never load entire file into memory:
```go
// For JSON streaming
p := jsparser.NewJSONParser(br, "elementPath")
for j := range p.Stream() {
    // Process each element
}

// For XML streaming
decoder := xml.NewDecoder(br)
for {
    token, err := decoder.Token()
    // Process tokens
}

// For TSV/line-based
scanner := bufio.NewScanner(br)
for scanner.Scan() {
    line := scanner.Text()
    // Process line
}
```

2. **Data Reader Pattern** (FTP preferred):
```go
// Standard FTP/HTTP reader
br, _, ftpFile, client, localFile, _, err := getDataReaderNew("datasetname", d.ebiFtp, d.ebiFtpPath, path)
check(err)
defer closeResources(ftpFile, localFile, client)
```

3. **Test Mode Support**:
```go
testLimit := config.GetTestLimit("datasetname")
var idLogFile *os.File
if config.IsTestMode() {
    idLogFile = openIDLogFile(config.TestRefDir, "datasetname_ids.txt")
    if idLogFile != nil {
        defer idLogFile.Close()
    }
}
// ... in loop:
if idLogFile != nil {
    logProcessedID(idLogFile, entryid)
}
// Check test limit
if testLimit > 0 && entryCount >= int64(testLimit) {
    break
}
```

4. **Key Function Calls**:
```go
// Save entry with attributes (REQUIRED for primary entries)
d.addProp3(entryID, datasetID, marshaledAttr)

// Text search indexing (for searchable terms)
d.addXref(searchTerm, textLinkID, entryID, "datasetname", true)

// Cross-references to other datasets
// IMPORTANT: 2nd param = FROM dataset ID (numeric string)
//            4th param = TO dataset NAME (string)
d.addXref(entryID, datasetID, targetID, "targetDatasetName", false)

// Cross-reference via gene symbol lookup (creates xrefs to Ensembl genes)
// REQUIRES: --lookupdb flag during update to load lookup database
d.addXrefViaGeneSymbol(geneSymbol, "", entryID, "datasetname", datasetID)
```

5. **Progress Tracking (REQUIRED)**:

Every dataset parser MUST signal completion to the progress tracker. This is critical for the update system to know when processing is complete.

```go
// Signal completion at the END of the update() function (REQUIRED)
// This MUST be called when processing is done
<parser>.d.progChan <- &progressInfo{dataset: <parser>.source, done: true}

// Example in a parser struct method:
func (p *datasetParser) update() {
    // ... processing logic ...

    // At the very end, signal completion
    p.d.progChan <- &progressInfo{dataset: p.source, done: true}
}
```

**Optional: Progress Updates During Processing**:
For long-running parsers, send periodic progress updates:
```go
// Report progress periodically (e.g., every N entries or time interval)
p.d.progChan <- &progressInfo{dataset: p.source, currentKBPerSec: kbytesPerSecond}

// Or just signal activity without speed metric
p.d.progChan <- &progressInfo{dataset: p.source}
```

**progressInfo struct fields**:
- `dataset`: Dataset name (use `p.source`)
- `done`: Set to `true` when processing is complete (REQUIRED at end)
- `currentKBPerSec`: Optional progress metric (entries/sec or KB/sec)
- `waiting`: Used for dependency waiting states
- `mergeOnly`: Used internally when dataset only needs merge

6. **Error Handling with Context**:
```go
func (p *parser) check(err error, operation string) {
    checkWithContext(err, p.source, operation)
}
```

7. **Debugging and Handling Problematic Data**:

When encountering parsing issues, follow this approach:

**a) Add Temporary Debug Logs**:
```go
// Add temporary logs to understand what's happening
log.Printf("DEBUG [datasetname] Processing entry: id=%s, line=%d", entryID, lineNum)
log.Printf("DEBUG [datasetname] Field values: field1=%q, field2=%q", field1, field2)

// Log suspicious data patterns
if len(field) > 1000 {
    log.Printf("DEBUG [datasetname] Unusually long field at line %d: len=%d", lineNum, len(field))
}
```

**b) Skipping Problematic Data (LAST RESORT ONLY)**:

Skipping is the LAST RESORT - only skip if data is truly problematic and cannot be fixed:

```go
// Track skipped entries for reporting
var skippedCount int64
var skippedReasons = make(map[string]int)

// In parsing loop:
if entryID == "" {
    skippedCount++
    skippedReasons["empty_id"]++
    log.Printf("SKIP [datasetname] Line %d: Empty entry ID - raw: %q", lineNum, rawLine[:min(100, len(rawLine))])
    continue
}

if !isValidFormat(field) {
    skippedCount++
    skippedReasons["invalid_format"]++
    log.Printf("SKIP [datasetname] Line %d: Invalid format for entry %s - value: %q", lineNum, entryID, field)
    continue
}

// At end of processing, report summary:
if skippedCount > 0 {
    log.Printf("WARNING [datasetname] Skipped %d entries. Reasons: %v", skippedCount, skippedReasons)
}
```

**c) Skip Decision Guidelines**:
- **DO skip**: Completely malformed lines, missing required fields (ID), corrupt data
- **DON'T skip**: Optional fields missing (use empty/default), minor formatting issues (try to normalize)
- **Always try to fix/normalize first** before deciding to skip
- **Log every skip** with: line number, entry ID (if available), reason, sample of problematic data

**d) Document All Skipped Data**:
After successful parsing, document in `tests/datasets/datasetname/README.md` under "Known Limitations":
- Types of data that were skipped
- Approximate count/percentage skipped
- Reasons for skipping
- Whether this affects query results

### Phase 4: Merge Logic

#### 4.1 Update `src/generate/mergeg.go`

Add to the `xref` struct:
```go
type xref struct {
    // ... existing fields ...
    DatasetName *pbuf.DatasetNameAttr
}
```

Add unmarshal case in the switch statement:
```go
case <dataset_id>:
    xr.DatasetName = &pbuf.DatasetNameAttr{}
    err := proto.Unmarshal(valbyte, xr.DatasetName)
    if err != nil {
        log.Println("DatasetName unmarshal error:", err)
    }
```

**Without this step, attributes will appear empty in query results!**

### Phase 5: Filter Support (if hasFilter="yes")

#### 5.1 Update `src/service/service.go`
Add CEL type registration:
```go
cel.Types(&pbuf.DatasetNameAttr{}),
```

Add CEL declaration:
```go
decls.NewIdent("datasetname", decls.NewObjectType("pbuf.DatasetNameAttr"), nil),
```

#### 5.2 Update `src/service/mapfilter.go`
Add filter evaluation case for the dataset.

### Phase 6: Build and Verify

**IMPORTANT: Biobtree Command Structure**
- Extra parameters ALWAYS come BEFORE the command
- Commands: `update`, `generate`, `build`, `web`, `test`

```bash
# 1. Build the binary
make build

# 2. Run UPDATE phase (data processing → creates index files in out/index/)
./biobtree -d "datasetname" update

# If using addXrefViaGeneSymbol (gene symbol → Ensembl lookup), add --lookupdb:
./biobtree -d "datasetname" --lookupdb update

# With custom output directory (--out-dir BEFORE command):
./biobtree -d "datasetname" --out-dir /path/to/output update

# 3. Run GENERATE phase (merge index files → creates database)
# ALWAYS use --keep to preserve index files for investigation!
./biobtree --keep generate

# With custom output directory:
./biobtree --keep --out-dir /path/to/output generate

# 4. Start web server (background mode)
./biobtree web &
# Or with nohup:
nohup ./biobtree web > web.log 2>&1 &

# 5. Test with curl requests (default port: 9292)
curl "http://localhost:9292/ws/?i=TEST_ID"
curl "http://localhost:9292/ws/map/?i=TEST_ID&m=>>datasetname>>uniprot"
```

**Troubleshooting: Inspect Index Files**

If something goes wrong, check the generated index files directly:

```bash
# List generated index files
ls -la out/index/*.gz

# View contents of index file (tab-separated: key, db, value, valuedb, evidence, relationship)
zcat out/index/0_0.datasetname.index.gz | head -20

# Check specific entries
zgrep "^EXPECTED_ID" out/index/*.datasetname.index.gz

# Count entries per file
zcat out/index/*.datasetname.index.gz | wc -l

# Check for malformed lines (should have 5-6 tab-separated fields)
zcat out/index/0_0.datasetname.index.gz | awk -F'\t' 'NF != 5 && NF != 6 {print NR": "$0}' | head

# Sample random entries to verify format
zcat out/index/*.datasetname.index.gz | shuf -n 10
```

**Index File Format (6 columns, tab-separated):**
```
key     db      value   valuedb evidence        relationship
12345   45      P12345  uniprot
BRCA1   0       12345   datasetname TRUE
```
- Column 1: Key (entry ID or search term)
- Column 2: Dataset ID (numeric)
- Column 3: Value (linked ID or marshaled protobuf for attributes)
- Column 4: Value dataset name
- Column 5: Evidence (TRUE for text search links)
- Column 6: Relationship type (optional, e.g., "left_neighbor")

### Phase 7: Tests

#### 7.1 Create Test Directory
```bash
mkdir -p tests/datasets/datasetname
```

#### 7.2 Required Test Files
- `test_cases.json` - Declarative tests
- `test_datasetname.py` - Custom tests with main()
- `extract_reference_data.py` - Fetch reference data from source API
- `reference_data.json` - Cached API responses
- `datasetname_ids.txt` - Test entry IDs
- `README.md` - Following standard format from tests/README.md

#### 7.3 Add to Orchestrator
Edit `tests/run_tests.py`:
- Add to help text
- Add to `all_datasets` dictionary
- Add dependency handling if needed

#### 7.4 Run Tests
```bash
python3 tests/run_tests.py datasetname
```
**Note**: The test runner automatically includes `--lookupdb` flag, enabling cross-references via gene symbol lookup (addXrefViaGeneSymbol).

## Documentation Requirements

### In `tests/datasets/datasetname/README.md`:
Document in "Known Limitations" section:
- Any data NOT included and why
- Any format limitations
- Any API/source limitations
- LMDB key length issues (if applicable)

### In main `README.md`:
- Add dataset to Features list (if significant)
- Add query examples

## Common Pitfalls to Avoid

1. **Wrong parameter order in addXref**: 2nd param is dataset ID (numeric), 4th is dataset name (string)
2. **Forgetting `make proto`**: Always recompile after changing .proto files
3. **Missing mergeg.go case**: Results in empty attributes
4. **Not creating bidirectional xrefs**: If A links to B, consider if B should link to A
5. **Loading entire file into memory**: Always use stream processing
6. **Ignoring test mode**: Always implement test_entries_count support
7. **Skipping data too eagerly**: Always try to normalize/fix data before skipping - skipping is LAST RESORT
8. **Silent failures**: Always log skipped entries with reason; never silently ignore problematic data
9. **No debug logging**: When parsing fails, add temporary debug logs to understand what's happening
10. **Wrong bucket method**: Always check existing bucket methods first - use matching method for similar ID patterns
11. **Creating unnecessary bucket methods**: Use existing methods when ID format matches; only create new method if truly unique
12. **Missing progress tracker completion signal**: ALWAYS call `p.d.progChan <- &progressInfo{dataset: p.source, done: true}` at the end of the update() function. Without this, the update system won't know the dataset finished processing
13. **Missing --lookupdb flag**: If using `addXrefViaGeneSymbol` to create cross-references via gene symbol lookup, you MUST use `--lookupdb` during update. Without it, no xrefs will be created and entries will appear isolated. The test runner (`tests/run_tests.py`) automatically includes this flag

## Reference Examples

Study these existing parsers for patterns:
- `src/update/hgnc.go` - JSON streaming, simple structure
- `src/update/taxonomy.go` - XML streaming
- `src/update/gwas.go` - TSV parsing
- `src/update/ontology.go` - OWL/RDF parsing
- `src/update/chembl.go` - RDF streaming with complex xrefs
- `src/update/entrez.go` - Multiple FTP files, TSV parsing

## Output

After completing integration, provide:
1. Summary of files created/modified
2. Dataset statistics (entry count, xref types)
3. Sample queries demonstrating functionality
4. Any limitations documented

## Post-Development: Commit Changes

Once testing is complete and you're happy with the integration:

```bash
# 1. Stage all changes
git add -A

# 2. Review what will be committed
git status
git diff --staged

# 3. Commit with descriptive message
git commit -m "Add <datasetname> dataset integration

- Add parser in src/update/<datasetname>.go
- Add protobuf definition in src/pbuf/attr.proto
- Add configuration in conf/source.dataset.json
- Add merge logic in src/generate/mergeg.go
- Add CEL filter support in src/service/service.go
- Add tests in tests/datasets/<datasetname>/

Dataset: <DatasetName>
Source: <URL>
Entries: ~<count>
"
```

**Checklist before committing:**
- [ ] All tests pass (`python3 tests/run_tests.py <datasetname>`)
- [ ] Build succeeds (`make build`)
- [ ] Web queries return expected results
- [ ] README.md updated (if significant dataset)
- [ ] Known limitations documented in tests/datasets/<datasetname>/README.md
