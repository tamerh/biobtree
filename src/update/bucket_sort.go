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

// KeepBucketFiles when set to true, preserves bucket files after sorting for debugging
var KeepBucketFiles = false

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
	writerKey    string
	writer       *DatasetBucketWriter
	datasetState *DatasetState
	indexDir     string
	outDir       string // For saving dataset state to main output directory
}

// concatenateResult holds the result of concatenating one dataset
type concatenateResult struct {
	datasetName string
	lines       uint64
	err         error
	skipped     bool // true if dataset was already merged (skip on recovery)
}

// ConcatenateBuckets merges all sorted bucket files for a dataset into one
// Uses k-way merge to maintain global sort order across buckets
// Reads uncompressed .txt bucket files, writes compressed .gz output
// Bucket files are preserved after concatenation for debugging
// Returns BucketStats with total lines and per-dataset breakdown
// For multi-bucket-set datasets, produces separate output files: dataset_sorted_1.gz, dataset_sorted_2.gz
// numWorkers controls parallelism (0 = use BucketSortWorkers from config)
// datasetState is used to track merge progress for crash recovery (can be nil)
// outDir is the main output directory for saving dataset state
func ConcatenateBuckets(pool *HybridWriterPool, indexDir, outDir string, chunkIdx string, datasetState *DatasetState) (*BucketStats, error) {
	return ConcatenateBucketsParallel(pool, indexDir, outDir, chunkIdx, 0, datasetState)
}

// ConcatenateBucketsParallel is like ConcatenateBuckets but with configurable parallelism
// datasetState is used to track merge progress for crash recovery - each dataset is marked
// as "merged" immediately after its merge completes, allowing recovery to skip already-merged datasets
// outDir is the main output directory for saving dataset state
func ConcatenateBucketsParallel(pool *HybridWriterPool, indexDir, outDir string, chunkIdx string, numWorkers int, datasetState *DatasetState) (*BucketStats, error) {
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
				datasetName := job.writer.config.DatasetName
				datasetID := job.writer.datasetID

				// Check if dataset is already merged (crash recovery case)
				if job.datasetState != nil && job.datasetState.GetDatasetStatus(datasetName) == StatusMerged {
					log.Printf("Skipping %s for merge (already merged from previous run)", datasetName)
					results <- concatenateResult{
						datasetName: datasetName,
						lines:       0,
						skipped:     true,
					}
					continue
				}

				// Do the actual merge
				lines, err := concatenateOneDataset(job.writerKey, job.writer, job.indexDir, chunkIdx)

				// After successful merge, mark as merged and save state immediately
				// This ensures crash recovery knows this dataset is done
				if err == nil && job.datasetState != nil {
					job.datasetState.MarkDatasetsMerged([]string{datasetName})
					if saveErr := SaveDatasetState(job.datasetState, job.outDir); saveErr != nil {
						log.Printf("Warning: failed to save merged state for %s: %v", datasetName, saveErr)
					} else {
						log.Printf("Marked %s (ID:%s) as merged", datasetName, datasetID)
					}
				}

				results <- concatenateResult{
					datasetName: datasetName,
					lines:       lines,
					err:         err,
				}
			}
		}()
	}

	// Send jobs
	for writerKey, writer := range writers {
		jobs <- concatenateJob{
			writerKey:    writerKey,
			writer:       writer,
			datasetState: datasetState,
			indexDir:     indexDir,
			outDir:       outDir,
		}
	}
	close(jobs)

	// Wait for workers and close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var firstErr error
	skippedCount := 0
	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
		}
		if result.skipped {
			skippedCount++
		}
		stats.TotalLines += result.lines
		stats.PerDataset[result.datasetName] += result.lines
	}

	if skippedCount > 0 {
		log.Printf("Skipped %d already-merged datasets", skippedCount)
	}

	if firstErr != nil {
		return stats, firstErr
	}
	return stats, nil
}

// concatenateOneDataset merges bucket files for a single dataset, creating per-source output files
// This enables incremental updates - when a source dataset is updated, only its contribution files are rebuilt
// Output format:
//   - {datasetName}_sorted.{chunkIdx}.index.gz - dataset's own data (from forward/)
//   - {datasetName}_from_{source}_sorted.{chunkIdx}.index.gz - xrefs from other datasets (from from_*/)
func concatenateOneDataset(writerKey string, writer *DatasetBucketWriter, indexDir string, chunkIdx string) (uint64, error) {
	datasetName := writer.config.DatasetName
	isDerived := writer.config.IsDerived

	// Special handling for textsearch: uses same per-source approach but with different naming
	// textsearch_{source}_sorted.{chunkIdx}.index.gz
	if datasetName == "textsearch" {
		return concatenateTextsearchPerSource(indexDir, chunkIdx, isDerived)
	}

	// Get bucket files grouped by source (forward, from_entrez, from_refseq, etc.)
	filesPerSource, err := GetBucketFilesPerSource(datasetName, indexDir, isDerived)
	if err != nil {
		return 0, fmt.Errorf("getting bucket files per source for %s: %w", datasetName, err)
	}

	// Add pool's forward bucket files to the "forward" source
	// These are from the writer pool and may not be on disk yet in filesPerSource
	// Only include actual forward/ files, not from_*/ files (those come from disk scan)
	poolForwardFiles := []string{}
	for _, bucket := range writer.buckets {
		if bucket.fileCreated && strings.Contains(bucket.filePath, "/forward/") {
			poolForwardFiles = append(poolForwardFiles, bucket.filePath)
		}
	}

	if len(poolForwardFiles) > 0 {
		// Merge with any existing forward files from disk
		existingForward := filesPerSource["forward"]
		existingSet := make(map[string]bool)
		for _, f := range existingForward {
			existingSet[f] = true
		}
		for _, f := range poolForwardFiles {
			if !existingSet[f] {
				existingForward = append(existingForward, f)
			}
		}
		filesPerSource["forward"] = existingForward
	}

	if len(filesPerSource) == 0 {
		return 0, nil // No bucket files for this dataset
	}

	totalLines := uint64(0)
	sourcesProcessed := 0

	// Process each source separately, creating its own output file
	for sourceName, bucketFiles := range filesPerSource {
		if len(bucketFiles) == 0 {
			continue
		}

		// Sort bucket files before merge
		// Skip files that no longer exist (may have been processed by another concurrent worker)
		for _, f := range bucketFiles {
			if _, err := os.Stat(f); os.IsNotExist(err) {
				continue
			}
			if err := SortBucketFile(f); err != nil {
				log.Printf("Warning: error sorting bucket file %s: %v", f, err)
			}
		}

		// Sort file paths for consistent order
		sort.Strings(bucketFiles)

		// Create output file path based on source
		// - forward → {datasetName}_sorted.{chunkIdx}.index.gz
		// - from_{source} → {datasetName}_from_{source}_sorted.{chunkIdx}.index.gz
		var outPath string
		if sourceName == "forward" {
			if writer.setIndex >= 0 {
				// Multi-bucket-set: {datasetName}_sorted_{setIdx+1}.{chunkIdx}.index.gz
				outPath = filepath.Join(indexDir, fmt.Sprintf("%s_sorted_%d.%s.index.gz",
					datasetName, writer.setIndex+1, chunkIdx))
			} else {
				// Single set: {datasetName}_sorted.{chunkIdx}.index.gz
				outPath = filepath.Join(indexDir, fmt.Sprintf("%s_sorted.%s.index.gz",
					datasetName, chunkIdx))
			}
		} else {
			// from_{source} → {datasetName}_from_{source}_sorted.{chunkIdx}.index.gz
			if writer.setIndex >= 0 {
				outPath = filepath.Join(indexDir, fmt.Sprintf("%s_from_%s_sorted_%d.%s.index.gz",
					datasetName, sourceName, writer.setIndex+1, chunkIdx))
			} else {
				outPath = filepath.Join(indexDir, fmt.Sprintf("%s_from_%s_sorted.%s.index.gz",
					datasetName, sourceName, chunkIdx))
			}
		}

		outF, err := os.Create(outPath)
		if err != nil {
			return totalLines, fmt.Errorf("creating output file %s: %w", outPath, err)
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
			br.readNext()
			if !br.eof {
				readers = append(readers, br)
			} else {
				f.Close()
			}
		}

		// K-way merge
		for len(readers) > 0 {
			minKey := readers[0].curKey
			for i := 1; i < len(readers); i++ {
				if readers[i].curKey < minKey {
					minKey = readers[i].curKey
				}
			}

			i := 0
			for i < len(readers) {
				if readers[i].curKey == minKey {
					buf.WriteString(readers[i].curLine)
					linesWritten++
					readers[i].readNext()
					if readers[i].eof {
						readers[i].file.Close()
						readers = append(readers[:i], readers[i+1:]...)
						continue
					}
				}
				i++
			}
		}

		buf.Flush()
		outGz.Close()
		outF.Close()

		// Clean up bucket files
		cleanedCount := 0
		if !KeepBucketFiles {
			cleanedDirs := make(map[string]bool)
			for _, bucketFile := range bucketFiles {
				if err := os.Remove(bucketFile); err == nil {
					cleanedCount++
					cleanedDirs[filepath.Dir(bucketFile)] = true
				}
			}

			// Remove empty bucket directories
			for dir := range cleanedDirs {
				entries, err := os.ReadDir(dir)
				if err == nil && len(entries) == 0 {
					os.Remove(dir)
				}
			}
		}

		totalLines += linesWritten
		sourcesProcessed++

		if sourceName == "forward" {
			log.Printf("K-way merged %d buckets for %s (ID:%s) → %s (%d lines, cleaned %d files)",
				len(bucketFiles), datasetName, writer.datasetID, outPath, linesWritten, cleanedCount)
		} else {
			log.Printf("K-way merged %d buckets for %s from %s → %s (%d lines, cleaned %d files)",
				len(bucketFiles), datasetName, sourceName, outPath, linesWritten, cleanedCount)
		}
	}

	log.Printf("Dataset %s: processed %d sources, total %d lines", datasetName, sourcesProcessed, totalLines)
	return totalLines, nil
}

// concatenateTextsearchPerSource creates separate sorted files for each source dataset
// Output format: textsearch_{source}_sorted.{chunkIdx}.index.gz
// This enables incremental updates - when a dataset is updated, only its textsearch file is rebuilt
func concatenateTextsearchPerSource(indexDir string, chunkIdx string, isDerived bool) (uint64, error) {
	// Get bucket files grouped by source
	filesPerSource, err := GetBucketFilesPerSource("textsearch", indexDir, isDerived)
	if err != nil {
		return 0, fmt.Errorf("getting textsearch bucket files per source: %w", err)
	}

	if len(filesPerSource) == 0 {
		log.Printf("No textsearch bucket files found")
		return 0, nil
	}

	totalLines := uint64(0)
	sourcesProcessed := 0

	// Process each source separately
	for sourceName, bucketFiles := range filesPerSource {
		if len(bucketFiles) == 0 {
			continue
		}

		// Sort bucket files
		// Skip files that no longer exist (may have been processed by another concurrent worker)
		for _, f := range bucketFiles {
			if _, err := os.Stat(f); os.IsNotExist(err) {
				continue
			}
			if err := SortBucketFile(f); err != nil {
				log.Printf("Warning: error sorting textsearch bucket file %s: %v", f, err)
			}
		}

		// Sort file paths for consistent order
		sort.Strings(bucketFiles)

		// Create output file: textsearch_{source}_sorted.{chunkIdx}.index.gz
		outPath := filepath.Join(indexDir, fmt.Sprintf("textsearch_%s_sorted.%s.index.gz", sourceName, chunkIdx))

		outF, err := os.Create(outPath)
		if err != nil {
			return totalLines, fmt.Errorf("creating textsearch output file %s: %w", outPath, err)
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
			br.readNext()
			if !br.eof {
				readers = append(readers, br)
			} else {
				f.Close()
			}
		}

		// K-way merge
		for len(readers) > 0 {
			minKey := readers[0].curKey
			for i := 1; i < len(readers); i++ {
				if readers[i].curKey < minKey {
					minKey = readers[i].curKey
				}
			}

			i := 0
			for i < len(readers) {
				if readers[i].curKey == minKey {
					buf.WriteString(readers[i].curLine)
					linesWritten++
					readers[i].readNext()
					if readers[i].eof {
						readers[i].file.Close()
						readers = append(readers[:i], readers[i+1:]...)
						continue
					}
				}
				i++
			}
		}

		buf.Flush()
		outGz.Close()
		outF.Close()

		// Clean up bucket files
		cleanedCount := 0
		if !KeepBucketFiles {
			cleanedDirs := make(map[string]bool)
			for _, bucketFile := range bucketFiles {
				if err := os.Remove(bucketFile); err == nil {
					cleanedCount++
					cleanedDirs[filepath.Dir(bucketFile)] = true
				}
			}

			// Remove empty bucket directories
			for dir := range cleanedDirs {
				entries, err := os.ReadDir(dir)
				if err == nil && len(entries) == 0 {
					os.Remove(dir)
				}
			}
		}

		totalLines += linesWritten
		sourcesProcessed++
		log.Printf("K-way merged %d buckets for textsearch_%s → %s (%d lines, cleaned %d files)",
			len(bucketFiles), sourceName, outPath, linesWritten, cleanedCount)
	}

	log.Printf("Textsearch: processed %d sources, total %d lines", sourcesProcessed, totalLines)
	return totalLines, nil
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

	// Clean up bucket files after successful merge to prevent accumulation across builds
	cleanedCount := 0
	if !KeepBucketFiles {
		cleanedDirs := make(map[string]bool)
		for _, bucketFile := range files {
			if err := os.Remove(bucketFile); err == nil {
				cleanedCount++
				cleanedDirs[filepath.Dir(bucketFile)] = true
			}
		}

		// Remove empty bucket directories (forward/, from_*/)
		for dir := range cleanedDirs {
			entries, err := os.ReadDir(dir)
			if err == nil && len(entries) == 0 {
				os.Remove(dir)
			}
		}
	}

	log.Printf("K-way merged %d buckets for %s → %s (%d lines, cleaned %d files)",
		len(files), datasetName, outPath, linesWritten, cleanedCount)

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
