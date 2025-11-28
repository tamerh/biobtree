package update

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// BucketWriter handles writes to a single bucket file
type BucketWriter struct {
	writeChan   chan []byte
	filePath    string
	done        chan struct{}
	fileCreated bool
	lineCount   uint64 // Count lines written (no contention - single goroutine per bucket)
}

// DatasetBucketWriter manages all buckets for one dataset
type DatasetBucketWriter struct {
	datasetID string
	config    *BucketConfig
	buckets   []*BucketWriter
	outputDir string
	wg        *sync.WaitGroup
}

// HybridWriterPool routes writes to buckets OR fallback
type HybridWriterPool struct {
	bucketConfigs map[string]*BucketConfig        // datasetID → config
	bucketWriters map[string]*DatasetBucketWriter // datasetID → writer
	fallbackChan  *chan string                    // Existing kvdatachan for fallback
	outputDir     string
	wg            *sync.WaitGroup
	mu            sync.RWMutex
}

// NewHybridWriterPool creates the pool with bucket configs
func NewHybridWriterPool(configs map[string]*BucketConfig, fallbackChan *chan string, outputDir string, wg *sync.WaitGroup) *HybridWriterPool {
	pool := &HybridWriterPool{
		bucketConfigs: configs,
		bucketWriters: make(map[string]*DatasetBucketWriter),
		fallbackChan:  fallbackChan,
		outputDir:     outputDir,
		wg:            wg,
	}

	// Initialize bucket writers for ALL configured datasets
	for datasetID, cfg := range configs {
		pool.bucketWriters[datasetID] = newDatasetBucketWriter(datasetID, cfg, outputDir, wg)
	}

	return pool
}

// newDatasetBucketWriter creates bucket writer for a dataset
func newDatasetBucketWriter(datasetID string, cfg *BucketConfig, baseDir string, wg *sync.WaitGroup) *DatasetBucketWriter {
	// Use dataset name for folder (not ID) for readability
	dir := filepath.Join(baseDir, cfg.DatasetName, "buckets")
	os.MkdirAll(dir, 0755)

	w := &DatasetBucketWriter{
		datasetID: datasetID,
		config:    cfg,
		outputDir: dir,
		buckets:   make([]*BucketWriter, cfg.NumBuckets),
		wg:        wg,
	}

	// Initialize all bucket channels and writers
	for i := 0; i < cfg.NumBuckets; i++ {
		bw := &BucketWriter{
			writeChan: make(chan []byte, 10000), // Buffered channel
			filePath:  filepath.Join(dir, fmt.Sprintf("bucket_%03d.gz", i)),
			done:      make(chan struct{}),
		}
		w.buckets[i] = bw
		wg.Add(1)
		go bw.writerLoop(wg)
	}

	return w
}

// writerLoop runs in goroutine, writes data from channel to file
func (bw *BucketWriter) writerLoop(wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(bw.done)

	var f *os.File
	var gz *gzip.Writer
	var buf *bufio.Writer

	for data := range bw.writeChan {
		// Lazy file creation - only when first data arrives
		if !bw.fileCreated {
			var err error
			f, err = os.Create(bw.filePath)
			if err != nil {
				fmt.Printf("Error creating bucket file %s: %v\n", bw.filePath, err)
				continue
			}
			gz, _ = gzip.NewWriterLevel(f, gzip.BestSpeed)
			buf = bufio.NewWriterSize(gz, 65536)
			bw.fileCreated = true
		}
		buf.Write(data)
		buf.WriteByte('\n')
		bw.lineCount++
	}

	// Close file if it was created
	if bw.fileCreated && buf != nil {
		buf.Flush()
		gz.Close()
		f.Close()
	}
}

// Write routes data to appropriate bucket or fallback
// For link datasets (parent/child), routes to the parent dataset's buckets
func (p *HybridWriterPool) Write(datasetID string, entityID string, line string) {
	// Resolve link dataset to parent dataset
	resolvedID := GetLinkDatasetID(datasetID)

	p.mu.RLock()
	cfg, hasBucket := p.bucketConfigs[resolvedID]
	writer, hasWriter := p.bucketWriters[resolvedID]
	p.mu.RUnlock()

	if hasBucket && hasWriter {
		// Route to bucket
		bucket := cfg.Method(entityID, cfg.NumBuckets)
		writer.buckets[bucket].writeChan <- []byte(line)
	} else {
		// Fallback to kvdatachan
		*p.fallbackChan <- line
	}
}

// GetTotalWrites returns the total number of lines written to buckets
// Must be called after Close() to get accurate count
func (p *HybridWriterPool) GetTotalWrites() uint64 {
	var total uint64
	for _, writer := range p.bucketWriters {
		for _, bucket := range writer.buckets {
			total += bucket.lineCount
		}
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

	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.bucketConfigs[resolvedID]
	return ok
}

// Close closes all bucket channels and waits for writers to finish
func (p *HybridWriterPool) Close() {
	for _, writer := range p.bucketWriters {
		for _, bucket := range writer.buckets {
			close(bucket.writeChan)
		}
	}
	// WaitGroup.Wait() is called by the caller (update.go)
}

// GetBucketFiles returns all non-empty bucket files for merging
func (p *HybridWriterPool) GetBucketFiles() []string {
	var files []string
	for _, writer := range p.bucketWriters {
		for _, bucket := range writer.buckets {
			if bucket.fileCreated {
				files = append(files, bucket.filePath)
			}
		}
	}
	return files
}

// GetBucketWriters returns the bucket writers map (for sorting/concatenation)
func (p *HybridWriterPool) GetBucketWriters() map[string]*DatasetBucketWriter {
	return p.bucketWriters
}
