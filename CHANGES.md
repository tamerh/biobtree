# BiobtreeV2 Changes Log

This document tracks modifications made to biobtree for integration with the BioYoda project.

## Date: October 14, 2025

### Summary
Updated biobtree to work with modern data sources that have migrated from FTP to HTTPS protocols. Fixed parsing issues with updated data formats and adapted cluster submission scripts for SGE scheduler.

---

## 1. FTP to HTTPS Migration Fixes

### Problem
Multiple bioinformatics data sources (EBI, Ensembl) have disabled FTP protocol but kept files accessible via HTTPS. This caused connection failures during data downloads.

### Datasets Affected
- HGNC (Hugo Gene Nomenclature Committee)
- ChEMBL (Chemical Database)
- HMDB (Human Metabolome Database)
- Ensembl (all branches: main, bacteria, fungi, metazoa, plants, protists)

### Solution
Modified `update/commons.go` to add HTTPS fallback support with proper domain mapping.

**Files Modified:**
- `update/commons.go`
  - Added HTTPS support for `ftp.ebi.ac.uk`
  - Added HTTPS support for Ensembl domains with certificate mapping:
    - `ftp.ensembl.org` → `ftp.ensembl.ebi.ac.uk`
    - `ftp.ensemblgenomes.org` → `ftp.ensemblgenomes.ebi.ac.uk`
  - Falls back to FTP if HTTPS fails

**Key Code Changes:**
```go
// Try HTTPS first for EBI datasets
if strings.HasPrefix(ftpAddr, "ftp.ebi.ac.uk") {
    httpsURL := "https://ftp.ebi.ac.uk" + ftpPath + filePath
    resp, err := http.Get(httpsURL)
    // ... handle response
}

// Map Ensembl domains for certificate compatibility
if hostOnly == "ftp.ensembl.org" {
    hostOnly = "ftp.ensembl.ebi.ac.uk"
} else if hostOnly == "ftp.ensemblgenomes.org" {
    hostOnly = "ftp.ensemblgenomes.ebi.ac.uk"
}
```

---

## 2. HGNC Dataset Fix

### Problem
HGNC migrated from FTP to Google Cloud Storage (HTTPS-only).

### Solution
- Updated `conf/source.dataset.json`: Changed path from FTP to direct HTTPS URL
- Modified `update/hgnc.go`: Added HTTP(S) download support with backward FTP compatibility

**Files Modified:**
- `conf/source.dataset.json`
  ```json
  "path": "https://storage.googleapis.com/public-download-files/hgnc/json/json/hgnc_complete_set.json"
  ```
- `update/hgnc.go`: Added logic to detect and handle HTTP(S) URLs

---

## 3. ChEMBL Dataset Fixes

### Problem
1. ChEMBL directory listings required pattern matching for versioned files (e.g., `chembl_36.0_molecule.ttl.gz`)
2. FTP directory listing failed, needed HTTPS alternative
3. Data format changed from integers to floats in version 36

### Solution
Added HTTP directory listing parser with glob-to-regex pattern matching and fixed data parsing.

**Files Modified:**
- `update/chembl.go`
  - Added `getHTTPFilePath()` function for HTTPS directory listing
  - Added `getFtpPath()` to handle both HTTPS and FTP
  - Fixed regex conversion: properly escape special characters before replacing wildcards
  - Changed `highestDevelopmentPhase` parsing from `strconv.ParseInt` to `strconv.ParseFloat`

**Pattern Matching Fix:**
```go
// Correct approach: use placeholder to protect wildcards
regexPattern := strings.ReplaceAll(pattern, "*", "<<<WILDCARD>>>")
regexPattern = regexp.QuoteMeta(regexPattern)  // Escape special chars
regexPattern = strings.ReplaceAll(regexPattern, "<<<WILDCARD>>>", ".*")
regexPattern = "^" + regexPattern + "$"
```

**Data Format Fix:**
```go
// Parse as float first (ChEMBL now uses values like "4.0")
ccFloat, err := strconv.ParseFloat(triple.Obj.String(), 64)
check(err)
cc := int32(ccFloat)
```

---

## 4. HMDB Dataset Fix

### Problem
HMDB numeric fields contain text annotations (e.g., "-0.467 (est)") that caused parsing failures.

### Solution
Created helper function to extract numeric part before parsing.

**Files Modified:**
- `update/hmdb.go`
  - Added `parseFloatValue()` helper function
  - Replaced all affected `strconv.ParseFloat()` calls in `getExperimentalProps()` and `getPredictedProps()`

**Code:**
```go
func parseFloatValue(val string) (float64, error) {
    val = strings.TrimSpace(val)
    parts := strings.Fields(val)  // Split by whitespace
    if len(parts) > 0 {
        return strconv.ParseFloat(parts[0], 64)  // Parse first token
    }
    return strconv.ParseFloat(val, 64)
}
```

---

## 5. Ensembl Configuration Update

### Problem
Biobtree was using outdated Ensembl version 53 configuration files.

### Solution
Enabled automatic Ensembl release checking to regenerate paths for latest version (115).

**Files Modified:**
- `conf/application.param.json`
  ```json
  "disableEnsemblReleaseCheck": "n"  // Changed from "y" to "n"
  ```
- Deleted old path files in `ensembl/` directory to trigger regeneration

---

## 6. SGE Cluster Support

### Problem
Original scripts used LSF (bsub) job scheduler, needed adaptation for SGE (qsub) cluster.

### Solution
Created SGE versions of cluster submission scripts.

**Files Created:**
- `scripts/data/all_sge.sh` - Main SGE submission script
- `scripts/data/common_sge.sh` - Common functions for SGE

**Key Differences:**
| Feature | LSF (bsub) | SGE (qsub) |
|---------|------------|------------|
| Submit command | `bsub` | `qsub` |
| Job name | `-J jobname` | `-N jobname` |
| CPUs | `-n 8` | `-pe smp 8` |
| Memory | `-M 16000 -R "rusage[mem=16000]"` | `-l h_vmem=16000M` |
| Runtime | Not set | `-l h_rt=604800` (seconds) |
| Output | `-oo file.log` | `-o file.log -j y` |
| Job check | `bjobs -P project` | `qstat -u $(whoami) \| grep jobname` |

**Usage:**
```bash
./scripts/data/all_sge.sh scc /path/to/output/dir
```

---

## 7. Debug Improvements

### Files Modified
- `update/commons.go`: Added debug logging for HTTPS requests to aid troubleshooting

```go
log.Printf("DEBUG Ensembl: Trying HTTPS URL: %s\n", httpsURL)
log.Printf("DEBUG Ensembl: HTTPS status code: %d\n", resp.StatusCode)
```

---

## Testing Results

All fixed datasets successfully tested:
- ✅ HGNC - Downloaded and parsed successfully
- ✅ ChEMBL - Pattern matching working, data parsing fixed
- ✅ HMDB - Annotated values parsed correctly
- ✅ Ensembl (taxid 9606) - Metadata regeneration and download working
- ✅ Taxonomy - Working via HTTPS

---

## Migration Notes

### For Production Deployment
1. Ensure Go version >=1.20 (for HTTP/2 support)
2. Verify firewall allows HTTPS (port 443) to:
   - ftp.ebi.ac.uk
   - ftp.ensembl.ebi.ac.uk
   - ftp.ensemblgenomes.ebi.ac.uk
   - storage.googleapis.com
3. For SGE cluster: Adjust memory/runtime limits in `all_sge.sh` based on your cluster policies
4. First run will regenerate Ensembl metadata (can take several minutes)

### Backward Compatibility
All changes maintain backward compatibility with FTP:
- HTTPS is tried first
- If HTTPS fails, falls back to FTP
- Existing local file mode (`useLocalFile: yes`) unchanged

---

## Future Improvements

1. Consider making HTTPS the primary protocol (remove FTP fallback after testing period)
2. Add retry logic with exponential backoff for network failures
3. Implement connection pooling for HTTP requests
4. Add progress bars for large downloads
5. Cache Ensembl metadata to avoid frequent regeneration

---

## Contact

For questions about these changes, contact the BioYoda development team.
