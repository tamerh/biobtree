package update

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// keyedLine holds a line with its pre-extracted key for efficient sorting
// This avoids re-extracting the key on every comparison during sort
type keyedLine struct {
	key  string // First field before tab (the sort key)
	line string // Full line
}

// extractKey extracts the first field (before tab) from a line
// This is the sort key - typically the entity ID like "RS123456789"
func extractKey(line string) string {
	if idx := strings.IndexByte(line, '\t'); idx > 0 {
		return line[:idx]
	}
	return line
}

// SortBucketFile sorts a single uncompressed bucket file in place
// Input: uncompressed .txt file
// Output: sorted, deduplicated .txt file (same path)
// Optimized: extracts keys once upfront, sorts by key only (not full line)
func SortBucketFile(filePath string) error {
	// Read all lines from uncompressed file
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}

	var entries []keyedLine
	reader := bufio.NewReaderSize(f, BucketReadBufferSize)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			break
		}
		if len(line) > 0 {
			// Remove trailing newline for sorting/dedup comparison
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			// Extract key once during read, not during every sort comparison
			entries = append(entries, keyedLine{
				key:  extractKey(line),
				line: line,
			})
		}
		if err == io.EOF {
			break
		}
	}
	f.Close()

	if len(entries) == 0 {
		return nil // Empty file
	}

	// Sort by key only - merge phase will group entries by key and dataset
	// Key is typically 10-20 bytes, full line is 200-500 bytes
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})

	// Write back uncompressed (with deduplication by full line)
	// Same key can appear from different datasets - that's NOT a duplicate
	// Only identical full lines are duplicates
	// Use a map to track seen lines within each key group since duplicates
	// may not be adjacent after sorting by key only
	outPath := filePath + ".sorted"
	outF, err := os.Create(outPath)
	if err != nil {
		return err
	}
	buf := bufio.NewWriterSize(outF, BucketWriteBufferSize)

	prevKey := ""
	seenLines := make(map[string]bool)
	for _, entry := range entries {
		// Reset seen lines when we move to a new key
		if entry.key != prevKey {
			seenLines = make(map[string]bool)
			prevKey = entry.key
		}
		// Deduplicate by full line within same key
		if !seenLines[entry.line] {
			buf.WriteString(entry.line)
			buf.WriteByte('\n')
			seenLines[entry.line] = true
		}
	}

	buf.Flush()
	outF.Close()

	// Replace original with sorted
	os.Remove(filePath)
	os.Rename(outPath, filePath)

	return nil
}

// SortAllBuckets sorts all bucket files in parallel
// Uses BucketSortWorkers from config if numWorkers is 0
// Skips datasets with SkipBucketSort=true in their config
func SortAllBuckets(pool *HybridWriterPool, numWorkers int) error {
	if numWorkers <= 0 {
		numWorkers = BucketSortWorkers
	}

	// Get files to sort, filtering out datasets with skipBucketSort
	var filesToSort []string
	var skippedDatasets []string

	for datasetID, writer := range pool.GetBucketWriters() {
		if writer.config.SkipBucketSort {
			skippedDatasets = append(skippedDatasets, writer.config.DatasetName)
			continue
		}
		for _, bucket := range writer.buckets {
			if bucket.fileCreated {
				filesToSort = append(filesToSort, bucket.filePath)
			}
		}
		_ = datasetID // suppress unused warning
	}

	if len(skippedDatasets) > 0 {
		log.Printf("Skipping sort for datasets with skipBucketSort=true: %v", skippedDatasets)
	}

	if len(filesToSort) == 0 {
		return nil
	}

	fmt.Printf("Sorting %d bucket files with %d workers\n", len(filesToSort), numWorkers)

	// Create job channel
	jobs := make(chan string, len(filesToSort))
	for _, f := range filesToSort {
		jobs <- f
	}
	close(jobs)

	// Worker pool
	var wg sync.WaitGroup
	errors := make(chan error, len(filesToSort))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range jobs {
				if err := SortBucketFile(filePath); err != nil {
					errors <- fmt.Errorf("sorting %s: %w", filePath, err)
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		return err // Return first error
	}

	fmt.Printf("Sorting complete for %d bucket files\n", len(filesToSort))
	return nil
}

// bucketReader holds state for reading from a single sorted bucket file during k-way merge
type bucketReader struct {
	file    *os.File
	reader  *bufio.Reader
	curLine string // Current line (including newline)
	curKey  string // Current key (first field)
	eof     bool
}

// readNext reads the next line from the bucket file
func (br *bucketReader) readNext() {
	line, err := br.reader.ReadString('\n')
	if err != nil {
		br.eof = true
		br.curLine = ""
		br.curKey = ""
		return
	}
	br.curLine = line
	// Extract key (first field before tab)
	if idx := strings.IndexByte(line, '\t'); idx > 0 {
		br.curKey = line[:idx]
	} else {
		br.curKey = strings.TrimRight(line, "\n")
	}
}

// BucketStats holds statistics for bucket concatenation
type BucketStats struct {
	TotalLines uint64            // Total lines written across all datasets
	PerDataset map[string]uint64 // Lines written per dataset name
}

// concatenateJob holds the info needed to concatenate one dataset
type concatenateJob struct {
	writerKey string
	writer    *DatasetBucketWriter
}

// concatenateResult holds the result of concatenating one dataset
type concatenateResult struct {
	datasetName string
	lines       uint64
	err         error
}

// ConcatenateBuckets merges all sorted bucket files for a dataset into one
// Uses k-way merge to maintain global sort order across buckets
// Reads uncompressed .txt bucket files, writes compressed .gz output
// Bucket files are preserved after concatenation for debugging
// Returns BucketStats with total lines and per-dataset breakdown
// For multi-bucket-set datasets, produces separate output files: dataset_sorted_1.gz, dataset_sorted_2.gz
// numWorkers controls parallelism (0 = use BucketSortWorkers from config)
func ConcatenateBuckets(pool *HybridWriterPool, indexDir string, chunkIdx string) (*BucketStats, error) {
	return ConcatenateBucketsParallel(pool, indexDir, chunkIdx, 0)
}

// ConcatenateBucketsParallel is like ConcatenateBuckets but with configurable parallelism
func ConcatenateBucketsParallel(pool *HybridWriterPool, indexDir string, chunkIdx string, numWorkers int) (*BucketStats, error) {
	if numWorkers <= 0 {
		numWorkers = BucketConcatWorkers // Use concat-specific workers (higher default, I/O bound)
	}

	stats := &BucketStats{
		PerDataset: make(map[string]uint64),
	}

	// Collect all jobs
	writers := pool.GetBucketWriters()
	log.Printf("Concatenating %d datasets with %d workers", len(writers), numWorkers)
	jobs := make(chan concatenateJob, len(writers))
	results := make(chan concatenateResult, len(writers))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				lines, err := concatenateOneDataset(job.writerKey, job.writer, indexDir, chunkIdx)
				results <- concatenateResult{
					datasetName: job.writer.config.DatasetName,
					lines:       lines,
					err:         err,
				}
			}
		}()
	}

	// Send jobs
	for writerKey, writer := range writers {
		jobs <- concatenateJob{writerKey: writerKey, writer: writer}
	}
	close(jobs)

	// Wait for workers and close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var firstErr error
	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
		}
		stats.TotalLines += result.lines
		stats.PerDataset[result.datasetName] += result.lines
	}

	if firstErr != nil {
		return stats, firstErr
	}
	return stats, nil
}

// concatenateOneDataset merges all bucket files for a single dataset
func concatenateOneDataset(writerKey string, writer *DatasetBucketWriter, indexDir string, chunkIdx string) (uint64, error) {
	// Get bucket files from the pool (forward/ directory)
	bucketFiles := []string{}
	for _, bucket := range writer.buckets {
		if bucket.fileCreated {
			bucketFiles = append(bucketFiles, bucket.filePath)
		}
	}

	// Also include bucket files from from_*/ directories (cross-references from other datasets)
	// These are written by other datasets in the same run but belong to this dataset
	datasetName := writer.config.DatasetName
	isDerived := writer.config.IsDerived
	fromFiles, err := GetAllBucketFiles(datasetName, indexDir, isDerived)
	if err == nil {
		// Add from_*/ files that aren't already in bucketFiles (avoid duplicates)
		forwardSet := make(map[string]bool)
		for _, f := range bucketFiles {
			forwardSet[f] = true
		}
		for _, f := range fromFiles {
			if !forwardSet[f] {
				// Sort from_*/ files before including them
				if err := SortBucketFile(f); err != nil {
					log.Printf("Warning: error sorting from_*/ bucket file %s: %v", f, err)
					continue
				}
				bucketFiles = append(bucketFiles, f)
			}
		}
	}

	if len(bucketFiles) == 0 {
		return 0, nil // No bucket files for this dataset
	}

	// Sort bucket files by name to ensure consistent order
	sort.Strings(bucketFiles)

	// Create compressed output file in index directory
	// Format depends on whether this is a multi-set or single-set config
	var outPath string
	if writer.setIndex >= 0 {
		// Multi-bucket-set: {datasetName}_sorted_{setIdx+1}.{chunkIdx}.index.gz
		outPath = filepath.Join(indexDir, fmt.Sprintf("%s_sorted_%d.%s.index.gz",
			writer.config.DatasetName, writer.setIndex+1, chunkIdx))
	} else {
		// Single set: {datasetName}_sorted.{chunkIdx}.index.gz
		outPath = filepath.Join(indexDir, fmt.Sprintf("%s_sorted.%s.index.gz",
			writer.config.DatasetName, chunkIdx))
	}

	outF, err := os.Create(outPath)
	if err != nil {
		return 0, fmt.Errorf("creating output file %s: %w", outPath, err)
	}
	outGz, _ := gzip.NewWriterLevel(outF, gzip.BestSpeed)
	buf := bufio.NewWriterSize(outGz, BucketWriteBufferSize)

	linesWritten := uint64(0)

	// Initialize bucket readers for k-way merge
	readers := make([]*bucketReader, 0, len(bucketFiles))
	for _, bucketFile := range bucketFiles {
		f, err := os.Open(bucketFile)
		if err != nil {
			continue
		}
		br := &bucketReader{
			file:   f,
			reader: bufio.NewReaderSize(f, BucketReadBufferSize),
		}
		br.readNext() // Read first line
		if !br.eof {
			readers = append(readers, br)
		} else {
			f.Close()
		}
	}

	// K-way merge: repeatedly find minimum key and write all lines with that key
	for len(readers) > 0 {
		// Find minimum key among all readers
		minKey := readers[0].curKey
		for i := 1; i < len(readers); i++ {
			if readers[i].curKey < minKey {
				minKey = readers[i].curKey
			}
		}

		// Write all lines with the minimum key (from all readers that have it)
		// and advance those readers
		i := 0
		for i < len(readers) {
			if readers[i].curKey == minKey {
				buf.WriteString(readers[i].curLine)
				linesWritten++
				readers[i].readNext()
				if readers[i].eof {
					// Remove exhausted reader
					readers[i].file.Close()
					readers = append(readers[:i], readers[i+1:]...)
					// Don't increment i since we removed an element
					continue
				}
			}
			i++
		}
	}

	buf.Flush()
	outGz.Close()
	outF.Close()

	// Log message includes set info for multi-set
	if writer.setIndex >= 0 {
		log.Printf("K-way merged %d buckets for %s set%d (ID:%s) → %s (%d lines)",
			len(bucketFiles), writer.config.DatasetName, writer.setIndex+1, writerKey, outPath, linesWritten)
	} else {
		log.Printf("K-way merged %d buckets for %s (ID:%s) → %s (%d lines)",
			len(bucketFiles), writer.config.DatasetName, writer.datasetID, outPath, linesWritten)
	}

	return linesWritten, nil
}

// SortBucketsForDataset sorts all bucket files for a dataset using the new directory structure
// Discovers files from forward/, from_*/ subdirectories and _derived/ if applicable
// This is used for incremental updates where we don't have a HybridWriterPool
func SortBucketsForDataset(datasetName string, indexDir string, isDerived bool, numWorkers int) error {
	if numWorkers <= 0 {
		numWorkers = BucketSortWorkers
	}

	// Get all bucket files for this dataset
	files, err := GetAllBucketFiles(datasetName, indexDir, isDerived)
	if err != nil {
		return fmt.Errorf("getting bucket files for %s: %w", datasetName, err)
	}

	if len(files) == 0 {
		log.Printf("No bucket files found for %s", datasetName)
		return nil
	}

	log.Printf("Sorting %d bucket files for %s with %d workers", len(files), datasetName, numWorkers)

	// Create job channel
	jobs := make(chan string, len(files))
	for _, f := range files {
		jobs <- f
	}
	close(jobs)

	// Worker pool
	var wg sync.WaitGroup
	errors := make(chan error, len(files))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range jobs {
				if err := SortBucketFile(filePath); err != nil {
					errors <- fmt.Errorf("sorting %s: %w", filePath, err)
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		return err
	}

	log.Printf("Sorting complete for %d bucket files of %s", len(files), datasetName)
	return nil
}

// ConcatenateBucketsForDataset merges all sorted bucket files for a dataset into one
// Uses the new directory structure with forward/, from_*/ subdirectories
// All subdirectories are merged into a single output file
func ConcatenateBucketsForDataset(datasetName string, indexDir string, isDerived bool, chunkIdx string) (uint64, error) {
	// Get all bucket files for this dataset
	files, err := GetAllBucketFiles(datasetName, indexDir, isDerived)
	if err != nil {
		return 0, fmt.Errorf("getting bucket files for %s: %w", datasetName, err)
	}

	if len(files) == 0 {
		log.Printf("No bucket files found for %s", datasetName)
		return 0, nil
	}

	// Sort bucket files by name to ensure consistent order
	sort.Strings(files)

	// Create compressed output file in index directory
	outPath := filepath.Join(indexDir, fmt.Sprintf("%s_sorted.%s.index.gz", datasetName, chunkIdx))

	outF, err := os.Create(outPath)
	if err != nil {
		return 0, fmt.Errorf("creating output file %s: %w", outPath, err)
	}
	outGz, _ := gzip.NewWriterLevel(outF, gzip.BestSpeed)
	buf := bufio.NewWriterSize(outGz, BucketWriteBufferSize)

	linesWritten := uint64(0)

	// Initialize bucket readers for k-way merge
	readers := make([]*bucketReader, 0, len(files))
	for _, bucketFile := range files {
		f, err := os.Open(bucketFile)
		if err != nil {
			continue
		}
		br := &bucketReader{
			file:   f,
			reader: bufio.NewReaderSize(f, BucketReadBufferSize),
		}
		br.readNext() // Read first line
		if !br.eof {
			readers = append(readers, br)
		} else {
			f.Close()
		}
	}

	// K-way merge: repeatedly find minimum key and write all lines with that key
	for len(readers) > 0 {
		// Find minimum key among all readers
		minKey := readers[0].curKey
		for i := 1; i < len(readers); i++ {
			if readers[i].curKey < minKey {
				minKey = readers[i].curKey
			}
		}

		// Write all lines with the minimum key (from all readers that have it)
		// and advance those readers
		i := 0
		for i < len(readers) {
			if readers[i].curKey == minKey {
				buf.WriteString(readers[i].curLine)
				linesWritten++
				readers[i].readNext()
				if readers[i].eof {
					// Remove exhausted reader
					readers[i].file.Close()
					readers = append(readers[:i], readers[i+1:]...)
					// Don't increment i since we removed an element
					continue
				}
			}
			i++
		}
	}

	buf.Flush()
	outGz.Close()
	outF.Close()

	log.Printf("K-way merged %d buckets for %s → %s (%d lines)",
		len(files), datasetName, outPath, linesWritten)

	return linesWritten, nil
}

// SortAndConcatenateAllDatasets sorts and concatenates bucket files for all datasets
// Uses the new directory structure with forward/, from_*/ subdirectories
// Processes both regular and derived datasets
func SortAndConcatenateAllDatasets(configs map[string]*BucketConfig, indexDir string, chunkIdx string, numWorkers int) (*BucketStats, error) {
	stats := &BucketStats{
		PerDataset: make(map[string]uint64),
	}

	if numWorkers <= 0 {
		numWorkers = BucketSortWorkers
	}

	// Process each dataset
	for _, cfg := range configs {
		datasetName := cfg.DatasetName
		isDerived := cfg.IsDerived

		// Sort bucket files
		if !cfg.SkipBucketSort {
			if err := SortBucketsForDataset(datasetName, indexDir, isDerived, numWorkers); err != nil {
				return nil, fmt.Errorf("sorting %s: %w", datasetName, err)
			}
		}

		// Concatenate bucket files
		lines, err := ConcatenateBucketsForDataset(datasetName, indexDir, isDerived, chunkIdx)
		if err != nil {
			return nil, fmt.Errorf("concatenating %s: %w", datasetName, err)
		}

		stats.TotalLines += lines
		stats.PerDataset[datasetName] = lines
	}

	return stats, nil
}

// DiscoverDatasetsWithBuckets scans the index directory and returns datasets that have bucket files
// This is useful for incremental updates where we don't have a HybridWriterPool
func DiscoverDatasetsWithBuckets(indexDir string) ([]string, error) {
	var datasets []string
	seen := make(map[string]bool)

	// Scan regular dataset directories
	entries, err := os.ReadDir(indexDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "_derived" {
			continue
		}

		// Check if this dataset has forward/ or from_*/ subdirectories
		datasetDir := filepath.Join(indexDir, entry.Name())
		hasBuckets := false

		// Check for forward/
		forwardDir := filepath.Join(datasetDir, "forward")
		if _, err := os.Stat(forwardDir); err == nil {
			hasBuckets = true
		}

		// Check for from_*/
		if !hasBuckets {
			fromDirs, _ := filepath.Glob(filepath.Join(datasetDir, "from_*"))
			if len(fromDirs) > 0 {
				hasBuckets = true
			}
		}

		if hasBuckets && !seen[entry.Name()] {
			datasets = append(datasets, entry.Name())
			seen[entry.Name()] = true
		}
	}

	// Scan _derived/ directory
	derivedDir := filepath.Join(indexDir, "_derived")
	if _, err := os.Stat(derivedDir); err == nil {
		derivedEntries, err := os.ReadDir(derivedDir)
		if err == nil {
			for _, entry := range derivedEntries {
				if !entry.IsDir() {
					continue
				}

				// Derived datasets always have buckets if the directory exists
				if !seen[entry.Name()] {
					datasets = append(datasets, entry.Name())
					seen[entry.Name()] = true
				}
			}
		}
	}

	return datasets, nil
}
