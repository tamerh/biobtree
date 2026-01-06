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

// HybridWriterPool provides direct mutex-based writes to bucket files
// This avoids channel overhead by having callers write directly with per-bucket locks
type HybridWriterPool struct {
	bucketConfigs map[string]*BucketConfig // datasetID → config
	bucketFiles   map[string]*BucketFile   // "datasetID_bucketNum" → file
	bucketKeys    map[string][]string      // datasetID → pre-computed bucket keys
	bucketDirs    map[string]string        // datasetID → output directory
	outputDir     string
	poolMutex     sync.RWMutex // Mutex for lazy bucket file creation
}

// NewHybridWriterPool creates the pool with bucket configs
func NewHybridWriterPool(configs map[string]*BucketConfig, outputDir string, wg *sync.WaitGroup) *HybridWriterPool {
	return NewHybridWriterPoolWithWorkers(configs, outputDir, wg, 0)
}

// NewHybridWriterPoolWithWorkers creates the pool (numWorkers ignored - direct writes)
// Supports multi-bucket-set configs with separate bucket directories per set
func NewHybridWriterPoolWithWorkers(configs map[string]*BucketConfig, outputDir string, wg *sync.WaitGroup, numWorkers int) *HybridWriterPool {
	pool := &HybridWriterPool{
		bucketConfigs: configs,
		bucketFiles:   make(map[string]*BucketFile),
		bucketKeys:    make(map[string][]string),
		bucketDirs:    make(map[string]string),
		outputDir:     outputDir,
	}

	// Create output directories for source datasets (non-derived) only
	// Derived datasets use lazy creation via writeToSubdir()
	for datasetID, cfg := range configs {
		// Skip derived datasets - they're created lazily when receiving reverse xrefs
		if cfg.IsDerived {
			continue
		}

		if cfg.NumSets > 1 {
			// Multi-bucket-set: create buckets1_{chunkIdx}/, buckets2_{chunkIdx}/, etc.
			// Include chunkIdx in dir name to allow multiple processes with different --idx
			// Pre-compute bucket keys for all sets
			// Key format: "datasetID_setIdx_bucketNum"
			totalBuckets := 0
			for setIdx := 0; setIdx < cfg.NumSets; setIdx++ {
				dir := filepath.Join(outputDir, cfg.DatasetName, fmt.Sprintf("buckets%d_%s", setIdx+1, chunkIdx))
				os.MkdirAll(dir, 0755)

				numBuckets := cfg.NumBucketsPerSet[setIdx]
				for i := 0; i < numBuckets; i++ {
					key := fmt.Sprintf("%s_%d_%d", datasetID, setIdx, i)
					pool.bucketFiles[key] = &BucketFile{
						filePath: filepath.Join(dir, fmt.Sprintf("bucket_%03d.txt", i)),
					}
					totalBuckets++
				}
			}
			// Store first set's dir for backward compat (not used in multi-set)
			pool.bucketDirs[datasetID] = filepath.Join(outputDir, cfg.DatasetName, fmt.Sprintf("buckets1_%s", chunkIdx))
			// No single bucketKeys array for multi-set - handled in Write()
		} else {
			// Single bucket set: create buckets_{chunkIdx}/
			// Include chunkIdx in dir name to allow multiple processes with different --idx
			dir := filepath.Join(outputDir, cfg.DatasetName, fmt.Sprintf("buckets_%s", chunkIdx))
			os.MkdirAll(dir, 0755)
			pool.bucketDirs[datasetID] = dir

			// Pre-compute bucket keys for this dataset
			keys := make([]string, cfg.NumBuckets)
			for i := 0; i < cfg.NumBuckets; i++ {
				key := fmt.Sprintf("%s_%d", datasetID, i)
				keys[i] = key
				pool.bucketFiles[key] = &BucketFile{
					// Write uncompressed during processing (no .gz extension)
					filePath: filepath.Join(dir, fmt.Sprintf("bucket_%03d.txt", i)),
				}
			}
			pool.bucketKeys[datasetID] = keys
		}
	}

	fmt.Printf("Direct bucket writer initialized with %d datasets, %d total buckets\n",
		len(configs), len(pool.bucketFiles))

	return pool
}

// Write routes data to appropriate bucket or fallback
// For link datasets (parent/child), routes to the parent dataset's buckets
// Direct write with per-bucket mutex - no channel overhead
// Supports multi-bucket-set configs: tries methods in order, uses first matching set
//
// Returns:
//   - true: data was written to bucket successfully
//   - false: data should go to kvdatachan fallback (no bucket config, or hybrid mode fallback)
func (p *HybridWriterPool) Write(datasetID string, entityID string, line string) bool {
	// Fast path: check original datasetID first (most common case)
	cfg, hasBucket := p.bucketConfigs[datasetID]
	keys := p.bucketKeys[datasetID]
	resolvedDatasetID := datasetID

	// Slow path: resolve link dataset if original not found
	if !hasBucket {
		if linkDatasetMap != nil {
			if parentID, isLink := linkDatasetMap[datasetID]; isLink {
				cfg, hasBucket = p.bucketConfigs[parentID]
				keys = p.bucketKeys[parentID]
				resolvedDatasetID = parentID
			}
		}
	}

	if hasBucket {
		var bucketKey string

		if cfg.NumSets > 1 && !cfg.HybridMode {
			// Multi-bucket-set (non-hybrid): try each method in order, use first match
			for setIdx, method := range cfg.Methods {
				bucketNum := method(entityID, cfg.NumBucketsPerSet[setIdx])
				if bucketNum >= 0 {
					// Method matched - use this bucket set
					bucketKey = fmt.Sprintf("%s_%d_%d", resolvedDatasetID, setIdx, bucketNum)
					break
				}
			}
			if bucketKey == "" {
				// No method matched - this shouldn't happen if last method is a catch-all
				log.Printf("WARNING: No bucket method matched for dataset=%s id='%s' - using fallback",
					cfg.DatasetName, entityID)
				return false
			}
		} else if cfg.HybridMode {
			// Hybrid mode: bucket method returns encoded (setIndex * numBucketsPerSet + bucket)
			// or -1 for fallback
			numBucketsPerSet := cfg.NumBucketsPerSet[0] // All sets have same bucket count in hybrid
			encodedBucket := cfg.Method(entityID, numBucketsPerSet)

			if encodedBucket < 0 {
				// Fallback to kvdatachan for unrecognized patterns
				return false
			}

			// Decode: setIndex = encodedBucket / numBucketsPerSet, bucket = encodedBucket % numBucketsPerSet
			setIdx := encodedBucket / numBucketsPerSet
			bucket := encodedBucket % numBucketsPerSet

			// Safety check
			if setIdx >= cfg.NumSets {
				log.Printf("WARNING: Hybrid bucket setIdx=%d >= NumSets=%d for dataset=%s id='%s' - using fallback",
					setIdx, cfg.NumSets, cfg.DatasetName, entityID)
				return false
			}

			bucketKey = fmt.Sprintf("%s_%d_%d", resolvedDatasetID, setIdx, bucket)
		} else if keys != nil {
			// Single bucket set: use pre-computed keys
			bucketNum := cfg.Method(entityID, cfg.NumBuckets)

			// Safety check: bucket number must be within range
			if bucketNum < 0 || bucketNum >= cfg.NumBuckets {
				log.Printf("WARNING: Bucket method returned %d but numBuckets=%d for dataset=%s id='%s' - using bucket 0",
					bucketNum, cfg.NumBuckets, cfg.DatasetName, entityID)
				bucketNum = 0
			}

			bucketKey = keys[bucketNum]
		} else {
			// No keys available - fallback
			return false
		}

		// Get bucket file and write directly with mutex
		bf := p.bucketFiles[bucketKey]
		if bf == nil {
			log.Printf("WARNING: No bucket file for key=%s dataset=%s id='%s' - using fallback",
				bucketKey, cfg.DatasetName, entityID)
			return false
		}

		bf.mutex.Lock()

		// Lazy file creation
		if !bf.created {
			var err error
			bf.file, err = os.Create(bf.filePath)
			if err != nil {
				bf.mutex.Unlock()
				fmt.Printf("Error creating bucket file %s: %v\n", bf.filePath, err)
				return false
			}
			bf.buf = bufio.NewWriterSize(bf.file, BucketWriteBufferSize)
			bf.created = true
		}

		// Write line directly
		bf.buf.WriteString(line)
		bf.buf.WriteByte('\n')
		bf.lineCount++

		bf.mutex.Unlock()
		return true
	}

	// No bucket config - fallback
	return false
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
			// Unknown pattern - use alphabetic fallback bucket instead of kvdatachan
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

	// Write line
	bf.buf.WriteString(line)
	bf.buf.WriteByte('\n')
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
