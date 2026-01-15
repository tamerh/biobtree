package update

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// BucketFile represents a single bucket file with mutex protection
// Files are written uncompressed during processing for speed,
// then compressed during concatenation phase
type BucketFile struct {
	filePath  string
	file      *os.File
	buf       *bufio.Writer
	mutex     sync.Mutex
	created   bool
	lineCount uint64
}

// validateBucketLine checks if a line has the minimum required format: KEY\tDB\tVALUE\tVALUEDB
// Returns true if valid, false if malformed.
// CRITICAL: All 4 fields must be non-empty to prevent "Error while converting to int16" panics
// during the generate phase when VALUEDB is empty.
func validateBucketLine(line string) bool {
	// Find first tab - KEY must be non-empty (idx > 0)
	idx1 := strings.IndexByte(line, '\t')
	if idx1 <= 0 {
		return false
	}

	// Find second tab - DB field must exist and be non-empty
	rest := line[idx1+1:]
	idx2 := strings.IndexByte(rest, '\t')
	if idx2 <= 0 {
		return false
	}

	// Find third tab - VALUE field must exist and be non-empty
	rest = rest[idx2+1:]
	idx3 := strings.IndexByte(rest, '\t')
	if idx3 <= 0 {
		return false
	}

	// VALUEDB field must be non-empty (everything after 3rd tab, before 4th tab or end)
	rest = rest[idx3+1:]
	if len(rest) == 0 {
		return false
	}
	// Check that VALUEDB is not empty (could be followed by more tabs for evidence/relationship)
	idx4 := strings.IndexByte(rest, '\t')
	if idx4 == 0 {
		return false
	}

	return true
}

// HybridWriterPool provides direct mutex-based writes to bucket files
// This avoids channel overhead by having callers write directly with per-bucket locks
type HybridWriterPool struct {
	bucketConfigs map[string]*BucketConfig // datasetID → config
	bucketFiles   map[string]*BucketFile   // "datasetID_subdir_bucketNum" → file
	outputDir     string
	poolMutex     sync.RWMutex // Mutex for lazy bucket file creation
}

// NewHybridWriterPool creates the pool with bucket configs
func NewHybridWriterPool(configs map[string]*BucketConfig, outputDir string, wg *sync.WaitGroup) *HybridWriterPool {
	return NewHybridWriterPoolWithWorkers(configs, outputDir, wg, 0)
}

// NewHybridWriterPoolWithWorkers creates the pool (numWorkers ignored - direct writes)
// All bucket files/directories are created lazily via writeToSubdir() when data is written
func NewHybridWriterPoolWithWorkers(configs map[string]*BucketConfig, outputDir string, wg *sync.WaitGroup, numWorkers int) *HybridWriterPool {
	pool := &HybridWriterPool{
		bucketConfigs: configs,
		bucketFiles:   make(map[string]*BucketFile),
		outputDir:     outputDir,
	}

	fmt.Printf("Bucket writer initialized with %d dataset configs (lazy directory creation)\n",
		len(configs))

	return pool
}

// WriteForward writes a forward xref to the source dataset's forward/ directory
// Used for source-tagged bucket files to enable incremental updates
// Directory structure: {dataset}/forward/bucket_*.txt
// For derived datasets: _derived/{dataset}/forward/bucket_*.txt
func (p *HybridWriterPool) WriteForward(sourceDatasetID, sourceDatasetName, entityID, line string) bool {
	return p.writeToSubdir(sourceDatasetID, entityID, line, "forward")
}

// WriteReverse writes a reverse xref to the target dataset's from_{source}/ directory
// Used for source-tagged bucket files to enable incremental updates
// Directory structure: {dataset}/from_{source}/bucket_*.txt
// For derived datasets: _derived/{dataset}/from_{source}/bucket_*.txt
func (p *HybridWriterPool) WriteReverse(targetDatasetID, entityID, line, sourceDatasetName string) bool {
	subdir := fmt.Sprintf("from_%s", sourceDatasetName)
	return p.writeToSubdir(targetDatasetID, entityID, line, subdir)
}

// writeToSubdir writes data to a source-tagged subdirectory with lazy bucket file creation
// Supports both regular datasets and derived datasets (stored under _derived/)
func (p *HybridWriterPool) writeToSubdir(datasetID, entityID, line, subdir string) bool {
	// Resolve link dataset if needed
	resolvedID := datasetID
	if linkDatasetMap != nil {
		if parentID, isLink := linkDatasetMap[datasetID]; isLink {
			resolvedID = parentID
		}
	}

	// Get bucket config
	cfg, hasBucket := p.bucketConfigs[resolvedID]
	if !hasBucket {
		return false
	}

	// Compute bucket number using the bucket method
	var bucketNum int
	if cfg.NumSets > 1 && !cfg.HybridMode {
		// Multi-bucket-set: try each method, use first match
		matched := false
		for _, method := range cfg.Methods {
			bucketNum = method(entityID, cfg.NumBucketsPerSet[0])
			if bucketNum >= 0 {
				matched = true
				break
			}
		}
		if !matched {
			// No method matched - use alphabetic fallback
			bucketNum = alphabeticBucket(entityID, cfg.NumBucketsPerSet[0])
			if bucketNum < 0 {
				bucketNum = 0
			}
		}
	} else if cfg.HybridMode {
		// Hybrid mode: decode bucket from encoded value
		numBucketsPerSet := cfg.NumBucketsPerSet[0]
		encodedBucket := cfg.Method(entityID, numBucketsPerSet)
		if encodedBucket < 0 {
			// Unknown pattern - use alphabetic fallback bucket
			// This ensures ALL data goes through bucket system for incremental updates
			bucketNum = alphabeticBucket(entityID, numBucketsPerSet)
			if bucketNum < 0 {
				bucketNum = 0
			}
		} else {
			bucketNum = encodedBucket % numBucketsPerSet
		}
	} else {
		// Single bucket set
		bucketNum = cfg.Method(entityID, cfg.NumBuckets)
		if bucketNum < 0 || bucketNum >= cfg.NumBuckets {
			bucketNum = 0
		}
	}

	// Build bucket key for source-tagged subdirectory
	// Format: "datasetID_subdir_bucketNum"
	bucketKey := fmt.Sprintf("%s_%s_%d", resolvedID, subdir, bucketNum)

	// Fast path: check if bucket file already exists (read lock)
	p.poolMutex.RLock()
	bf := p.bucketFiles[bucketKey]
	p.poolMutex.RUnlock()

	// Slow path: create bucket file if not exists (write lock)
	if bf == nil {
		p.poolMutex.Lock()
		// Double-check after acquiring write lock
		bf = p.bucketFiles[bucketKey]
		if bf == nil {
			// Build directory path
			var dir string
			if cfg.IsDerived {
				// Derived datasets go under _derived/
				dir = filepath.Join(p.outputDir, "_derived", cfg.DatasetName, subdir)
			} else {
				dir = filepath.Join(p.outputDir, cfg.DatasetName, subdir)
			}

			// Create directory if needed
			if err := os.MkdirAll(dir, 0755); err != nil {
				p.poolMutex.Unlock()
				log.Printf("Error creating bucket dir %s: %v", dir, err)
				return false
			}

			// Create bucket file entry
			bf = &BucketFile{
				filePath: filepath.Join(dir, fmt.Sprintf("bucket_%03d.txt", bucketNum)),
			}
			p.bucketFiles[bucketKey] = bf
		}
		p.poolMutex.Unlock()
	}

	// Validate line format before writing
	// Log details and panic to catch bugs at source - malformed data blocks merge phase
	if !validateBucketLine(line) {
		log.Printf("ERROR: Malformed bucket line detected!")
		log.Printf("  Dataset: %s, Subdir: %s, BucketKey: %s", datasetID, subdir, bucketKey)
		log.Printf("  Line preview (first 200 chars): %.200s", line)
		panic(fmt.Sprintf("Malformed bucket line. Dataset=%s, Line=%.100s", datasetID, line))
	}

	// Write to bucket file with per-file mutex
	bf.mutex.Lock()
	defer bf.mutex.Unlock()

	// Lazy file creation
	if !bf.created {
		var err error
		bf.file, err = os.Create(bf.filePath)
		if err != nil {
			log.Printf("Error creating bucket file %s: %v", bf.filePath, err)
			return false
		}
		bf.buf = bufio.NewWriterSize(bf.file, BucketWriteBufferSize)
		bf.created = true
	}

	// Atomic write: combine line + newline to reduce partial write window
	bf.buf.WriteString(line + "\n")
	bf.lineCount++

	return true
}

// GetTotalWrites returns the total number of lines written to buckets
// Must be called after Close() to get accurate count
func (p *HybridWriterPool) GetTotalWrites() uint64 {
	var total uint64
	for _, bf := range p.bucketFiles {
		total += bf.lineCount
	}
	return total
}

// WriteXref writes forward xref and optionally reverse xref using source-tagged directories
// Deprecated: Use WriteForward and WriteReverse directly for better control
func (p *HybridWriterPool) WriteXref(
	sourceDataset string, sourceID string, forwardLine string,
	targetDataset string, targetID string, reverseLine string,
) {
	// Get source dataset name for directory naming
	sourceDatasetName := GetDatasetName(sourceDataset)
	if sourceDatasetName == "" {
		sourceDatasetName = "unknown"
	}

	// Forward xref → {source}/forward/
	p.WriteForward(sourceDataset, sourceDatasetName, sourceID, forwardLine)

	// Reverse xref → {target}/from_{source}/ (if provided)
	if targetDataset != "" && reverseLine != "" {
		p.WriteReverse(targetDataset, targetID, reverseLine, sourceDatasetName)
	}
}

// HasBucketConfig returns true if dataset has bucket configuration
// Also returns true for link datasets whose parent has bucket config
func (p *HybridWriterPool) HasBucketConfig(datasetID string) bool {
	// Resolve link dataset to parent dataset
	resolvedID := GetLinkDatasetID(datasetID)
	_, ok := p.bucketConfigs[resolvedID]
	return ok
}

// Close flushes and closes all bucket files
// Must acquire mutex for each file to prevent race with concurrent writes
func (p *HybridWriterPool) Close() {
	// Close all bucket files (uncompressed)
	for _, bf := range p.bucketFiles {
		bf.mutex.Lock()
		if bf.created && bf.buf != nil {
			bf.buf.Flush()
			bf.file.Close()
		}
		bf.mutex.Unlock()
	}
}

// GetBucketFiles returns all non-empty bucket files for merging
func (p *HybridWriterPool) GetBucketFiles() []string {
	var files []string
	for _, bf := range p.bucketFiles {
		if bf.created {
			files = append(files, bf.filePath)
		}
	}
	return files
}

// GetBucketWriters returns bucket info for sorting/concatenation
// Returns a compatible structure for existing sort code
// For multi-bucket-set configs, returns separate entries for each set
// IMPORTANT: This now includes ALL bucket files (both old buckets_{chunkIdx}/ and new forward/, from_*/)
func (p *HybridWriterPool) GetBucketWriters() map[string]*DatasetBucketWriter {
	result := make(map[string]*DatasetBucketWriter)

	// Collect all created bucket files by dataset
	// Key format from writeToSubdir: "{datasetID}_{subdir}_{bucketNum}"
	// Key format from initialization: "{datasetID}_{bucketNum}" or "{datasetID}_{setIdx}_{bucketNum}"
	datasetFiles := make(map[string][]*BucketFile)
	for key, bf := range p.bucketFiles {
		if !bf.created {
			continue // Skip files that were never written to
		}
		// Extract datasetID from key (first part before underscore)
		parts := strings.SplitN(key, "_", 2)
		if len(parts) < 1 {
			continue
		}
		datasetID := parts[0]
		datasetFiles[datasetID] = append(datasetFiles[datasetID], bf)
	}

	// Build result from collected files
	for datasetID, files := range datasetFiles {
		cfg, hasCfg := p.bucketConfigs[datasetID]
		if !hasCfg {
			// Try resolving via link dataset map
			if parentID := GetLinkDatasetID(datasetID); parentID != datasetID {
				cfg, hasCfg = p.bucketConfigs[parentID]
			}
		}
		if !hasCfg {
			continue
		}

		// Convert to BucketWriter array
		buckets := make([]*BucketWriter, len(files))
		for i, bf := range files {
			buckets[i] = &BucketWriter{
				filePath:    bf.filePath,
				fileCreated: bf.created,
				lineCount:   bf.lineCount,
			}
		}

		result[datasetID] = &DatasetBucketWriter{
			datasetID: datasetID,
			config:    cfg,
			outputDir: p.outputDir,
			buckets:   buckets,
			setIndex:  -1,
		}
	}

	return result
}

// DatasetBucketWriter is kept for compatibility with sorting code
type DatasetBucketWriter struct {
	datasetID string
	config    *BucketConfig
	buckets   []*BucketWriter
	outputDir string
	wg        *sync.WaitGroup
	setIndex  int // -1 for single set, 0+ for multi-set index
}

// BucketWriter is kept for compatibility with sorting code
type BucketWriter struct {
	filePath    string
	fileCreated bool
	lineCount   uint64
}

// MarkExistingFilesCreated scans bucket directories and marks files that exist as created
// Used for resume-from-sort mode where bucket files already exist from a previous run
func (p *HybridWriterPool) MarkExistingFilesCreated() int {
	count := 0
	for _, bf := range p.bucketFiles {
		if _, err := os.Stat(bf.filePath); err == nil {
			bf.created = true
			count++
		}
	}
	log.Printf("Resume mode: found %d existing bucket files", count)
	return count
}
