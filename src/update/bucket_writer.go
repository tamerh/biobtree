package update

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	fallbackChan  *chan string             // Existing kvdatachan for fallback
	outputDir     string
}

// NewHybridWriterPool creates the pool with bucket configs
func NewHybridWriterPool(configs map[string]*BucketConfig, fallbackChan *chan string, outputDir string, wg *sync.WaitGroup) *HybridWriterPool {
	return NewHybridWriterPoolWithWorkers(configs, fallbackChan, outputDir, wg, 0)
}

// NewHybridWriterPoolWithWorkers creates the pool (numWorkers ignored - direct writes)
// Supports multi-bucket-set configs with separate bucket directories per set
func NewHybridWriterPoolWithWorkers(configs map[string]*BucketConfig, fallbackChan *chan string, outputDir string, wg *sync.WaitGroup, numWorkers int) *HybridWriterPool {
	pool := &HybridWriterPool{
		bucketConfigs: configs,
		bucketFiles:   make(map[string]*BucketFile),
		bucketKeys:    make(map[string][]string),
		bucketDirs:    make(map[string]string),
		fallbackChan:  fallbackChan,
		outputDir:     outputDir,
	}

	// Create output directories for all configured datasets
	for datasetID, cfg := range configs {
		if cfg.NumSets > 1 {
			// Multi-bucket-set: create buckets1/, buckets2/, etc.
			// Pre-compute bucket keys for all sets
			// Key format: "datasetID_setIdx_bucketNum"
			totalBuckets := 0
			for setIdx := 0; setIdx < cfg.NumSets; setIdx++ {
				dir := filepath.Join(outputDir, cfg.DatasetName, fmt.Sprintf("buckets%d", setIdx+1))
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
			pool.bucketDirs[datasetID] = filepath.Join(outputDir, cfg.DatasetName, "buckets1")
			// No single bucketKeys array for multi-set - handled in Write()
		} else {
			// Single bucket set: create buckets/
			dir := filepath.Join(outputDir, cfg.DatasetName, "buckets")
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

// GetTotalWrites returns the total number of lines written to buckets
// Must be called after Close() to get accurate count
func (p *HybridWriterPool) GetTotalWrites() uint64 {
	var total uint64
	for _, bf := range p.bucketFiles {
		total += bf.lineCount
	}
	return total
}

// WriteXref writes forward xref and optionally reverse xref
func (p *HybridWriterPool) WriteXref(
	sourceDataset string, sourceID string, forwardLine string,
	targetDataset string, targetID string, reverseLine string,
) {
	// Forward xref
	p.Write(sourceDataset, sourceID, forwardLine)

	// Reverse xref (if provided)
	if targetDataset != "" && reverseLine != "" {
		p.Write(targetDataset, targetID, reverseLine)
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
func (p *HybridWriterPool) Close() {
	// Close all bucket files (uncompressed)
	for _, bf := range p.bucketFiles {
		if bf.created && bf.buf != nil {
			bf.buf.Flush()
			bf.file.Close()
		}
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
func (p *HybridWriterPool) GetBucketWriters() map[string]*DatasetBucketWriter {
	result := make(map[string]*DatasetBucketWriter)

	for datasetID, cfg := range p.bucketConfigs {
		if cfg.NumSets > 1 {
			// Multi-bucket-set: create separate entries for each set
			for setIdx := 0; setIdx < cfg.NumSets; setIdx++ {
				dir := filepath.Join(p.outputDir, cfg.DatasetName, fmt.Sprintf("buckets%d", setIdx+1))
				numBuckets := cfg.NumBucketsPerSet[setIdx]

				// Create unique key for this set
				setKey := fmt.Sprintf("%s_set%d", datasetID, setIdx)

				dbw := &DatasetBucketWriter{
					datasetID: datasetID,
					config:    cfg,
					outputDir: dir,
					buckets:   make([]*BucketWriter, numBuckets),
					setIndex:  setIdx,
				}

				for i := 0; i < numBuckets; i++ {
					key := fmt.Sprintf("%s_%d_%d", datasetID, setIdx, i)
					bf := p.bucketFiles[key]
					dbw.buckets[i] = &BucketWriter{
						filePath:    bf.filePath,
						fileCreated: bf.created,
						lineCount:   bf.lineCount,
					}
				}

				result[setKey] = dbw
			}
		} else {
			// Single bucket set
			dir := p.bucketDirs[datasetID]
			if dir == "" {
				continue
			}

			keys := p.bucketKeys[datasetID]
			dbw := &DatasetBucketWriter{
				datasetID: datasetID,
				config:    cfg,
				outputDir: dir,
				buckets:   make([]*BucketWriter, cfg.NumBuckets),
				setIndex:  -1, // Single set indicator
			}

			for i := 0; i < cfg.NumBuckets; i++ {
				bf := p.bucketFiles[keys[i]] // Use pre-computed key
				dbw.buckets[i] = &BucketWriter{
					filePath:    bf.filePath,
					fileCreated: bf.created,
					lineCount:   bf.lineCount,
				}
			}

			result[datasetID] = dbw
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
