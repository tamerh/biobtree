package update

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
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

// SortBucketFileInMemory sorts a single bucket file using Go's in-memory sort
// Handles both compressed (.txt.gz) and uncompressed (.txt) files
// Input and output maintain the same compression format
// Optimized: extracts keys once upfront, sorts by key only (not full line)
// WARNING: Loads entire file into memory - may OOM on very large files (use Unix sort instead)
func SortBucketFileInMemory(filePath string) error {
	// Detect if file is compressed by extension
	isCompressed := strings.HasSuffix(filePath, ".gz")

	// Open and read all lines
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}

	var reader *bufio.Reader
	var gzReader *gzip.Reader

	if isCompressed {
		gzReader, err = gzip.NewReader(f)
		if err != nil {
			f.Close()
			return fmt.Errorf("opening gzip reader for %s: %w", filePath, err)
		}
		reader = bufio.NewReaderSize(gzReader, BucketReadBufferSize)
	} else {
		reader = bufio.NewReaderSize(f, BucketReadBufferSize)
	}

	var entries []keyedLine
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

	// Close readers
	if gzReader != nil {
		gzReader.Close()
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

	// Write back in same format (with deduplication by full line)
	// Same key can appear from different datasets - that's NOT a duplicate
	// Only identical full lines are duplicates
	// Use a map to track seen lines within each key group since duplicates
	// may not be adjacent after sorting by key only
	outPath := filePath + ".sorted"
	outF, err := os.Create(outPath)
	if err != nil {
		return err
	}

	var buf *bufio.Writer
	var gzWriter *gzip.Writer

	if isCompressed {
		gzWriter, _ = gzip.NewWriterLevel(outF, gzip.BestSpeed)
		buf = bufio.NewWriterSize(gzWriter, BucketWriteBufferSize)
	} else {
		buf = bufio.NewWriterSize(outF, BucketWriteBufferSize)
	}

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
	if gzWriter != nil {
		gzWriter.Close()
	}
	outF.Close()

	// Replace original with sorted
	os.Remove(filePath)
	os.Rename(outPath, filePath)

	return nil
}

// SortBucketFileUnix sorts a bucket file using Unix sort command (external merge sort)
// Handles both compressed (.gz) and uncompressed files
// Uses bounded memory via -S flag, preventing OOM on large files
// Options are configured via UnixSortOptions (e.g., "-u -S 4G --parallel=4 -T /tmp")
// Key flags: LC_ALL=C (byte comparison), -t\t (tab delimiter), -k1,1 (sort by first field)
func SortBucketFileUnix(filePath string) error {
	isCompressed := strings.HasSuffix(filePath, ".gz")
	outPath := filePath + ".sorted"

	var cmd *exec.Cmd

	if isCompressed {
		// Pipeline: zcat input | LC_ALL=C sort ... | pigz/gzip > output
		// Using bash -c to handle the pipeline
		// UnixSortOptions contains: -u -S 4G --parallel=4 -T /tmp (or user-configured options)
		// UnixSortCompressor: "pigz" (parallel, faster) or "gzip" (standard)
		cmdStr := fmt.Sprintf(
			"zcat '%s' | LC_ALL=C sort -t$'\\t' -k1,1 %s | %s > '%s'",
			filePath,
			UnixSortOptions,
			UnixSortCompressor,
			outPath,
		)
		cmd = exec.Command("bash", "-c", cmdStr)
	} else {
		// Direct sort for uncompressed files using shell to parse UnixSortOptions
		cmdStr := fmt.Sprintf(
			"LC_ALL=C sort -t$'\\t' -k1,1 %s -o '%s' '%s'",
			UnixSortOptions,
			outPath,
			filePath,
		)
		cmd = exec.Command("bash", "-c", cmdStr)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unix sort failed for %s: %v, output: %s", filePath, err, string(output))
	}

	// Replace original with sorted
	os.Remove(filePath)
	if err := os.Rename(outPath, filePath); err != nil {
		return fmt.Errorf("renaming sorted file %s: %w", outPath, err)
	}

	return nil
}

// SortBucketFile sorts a bucket file using the configured method
// If useUnixSort is true, uses Unix sort command (external merge sort with bounded memory)
// Otherwise uses Go's in-memory sort (faster but may OOM on large files)
func SortBucketFile(filePath string, useUnixSort bool) error {
	if useUnixSort {
		return SortBucketFileUnix(filePath)
	}
	return SortBucketFileInMemory(filePath)
}

// sortJob holds info for sorting a single bucket file
type sortJob struct {
	filePath    string
	useUnixSort bool
}

// SortAllBuckets sorts all bucket files in parallel
// Uses BucketSortWorkers from config if numWorkers is 0
// Skips datasets with SkipBucketSort=true in their config
// Uses global BucketSortMethod setting for sort method selection
func SortAllBuckets(pool *HybridWriterPool, numWorkers int) error {
	if numWorkers <= 0 {
		numWorkers = BucketSortWorkers
	}

	// Use global sort method (fixes reverse xref inheritance bug)
	useUnixSort := BucketSortMethod == "unix"

	// Get files to sort, filtering out datasets with skipBucketSort
	var filesToSort []sortJob
	var skippedDatasets []string

	for datasetID, writer := range pool.GetBucketWriters() {
		if writer.config.SkipBucketSort {
			skippedDatasets = append(skippedDatasets, writer.config.DatasetName)
			continue
		}
		for _, bucket := range writer.buckets {
			if bucket.fileCreated {
				filesToSort = append(filesToSort, sortJob{
					filePath:    bucket.filePath,
					useUnixSort: useUnixSort,
				})
			}
		}
		_ = datasetID // suppress unused warning
	}

	if len(skippedDatasets) > 0 {
		log.Printf("Skipping sort for datasets with skipBucketSort=true: %v", skippedDatasets)
	}
	if useUnixSort {
		log.Printf("Using Unix sort (bounded memory) for all bucket files")
	}

	if len(filesToSort) == 0 {
		return nil
	}

	fmt.Printf("Sorting %d bucket files with %d workers\n", len(filesToSort), numWorkers)

	// Create job channel
	jobs := make(chan sortJob, len(filesToSort))
	for _, job := range filesToSort {
		jobs <- job
	}
	close(jobs)

	// Worker pool
	var wg sync.WaitGroup
	errors := make(chan error, len(filesToSort))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if err := SortBucketFile(job.filePath, job.useUnixSort); err != nil {
					errors <- fmt.Errorf("sorting %s: %w", job.filePath, err)
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
// Supports both compressed (.txt.gz) and uncompressed (.txt) files
type bucketReader struct {
	file     *os.File
	gzReader *gzip.Reader  // nil if uncompressed
	reader   *bufio.Reader
	curLine  string // Current line (including newline)
	curKey   string // Current key (first field)
	eof      bool
}

// newBucketReader creates a bucket reader that handles both compressed and uncompressed files
// Detects compression by file extension (.txt.gz vs .txt)
func newBucketReader(filePath string) (*bucketReader, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	br := &bucketReader{file: f}

	if strings.HasSuffix(filePath, ".gz") {
		br.gzReader, err = gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("opening gzip reader for %s: %w", filePath, err)
		}
		br.reader = bufio.NewReaderSize(br.gzReader, BucketReadBufferSize)
	} else {
		br.reader = bufio.NewReaderSize(f, BucketReadBufferSize)
	}

	br.readNext()
	return br, nil
}

// close properly closes all readers (gzip if compressed, then file)
func (br *bucketReader) close() {
	if br.gzReader != nil {
		br.gzReader.Close()
	}
	br.file.Close()
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
	TotalLines       uint64                       // Total lines written across all datasets
	PerDataset       map[string]uint64            // Lines written per dataset name (total)
	PerDatasetSource map[string]map[string]uint64 // Lines per dataset per source (dataset -> source -> lines)
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
	datasetName   string
	lines         uint64
	perSourceLines map[string]uint64 // Lines per source (forward, uniprot, ensembl, etc.)
	err           error
	skipped       bool // true if dataset was already merged (skip on recovery)
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
		PerDataset:       make(map[string]uint64),
		PerDatasetSource: make(map[string]map[string]uint64),
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
				// Even if merged, we still need to check for new from_* sources
				// that may have been added by datasets processed in later batches
				isAlreadyMerged := job.datasetState != nil && job.datasetState.GetDatasetStatus(datasetName) == StatusMerged

				// Do the actual merge (will skip sources that already have output files)
				lines, perSourceLines, err := concatenateOneDatasetIncremental(job.writerKey, job.writer, job.indexDir, chunkIdx, isAlreadyMerged)

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
					datasetName:    datasetName,
					lines:          lines,
					perSourceLines: perSourceLines,
					err:            err,
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

		// Store per-source breakdown
		if result.perSourceLines != nil && len(result.perSourceLines) > 0 {
			if stats.PerDatasetSource[result.datasetName] == nil {
				stats.PerDatasetSource[result.datasetName] = make(map[string]uint64)
			}
			for source, lines := range result.perSourceLines {
				stats.PerDatasetSource[result.datasetName][source] += lines
			}
		}
	}

	if skippedCount > 0 {
		log.Printf("Skipped %d already-merged datasets", skippedCount)
	}

	if firstErr != nil {
		return stats, firstErr
	}
	return stats, nil
}

// concatenateOneDatasetIncremental merges bucket files for a dataset, skipping sources that already have output files
// This handles the case where a dataset was merged in an earlier batch but received new reverse mappings
// from datasets processed in later batches (e.g., GO merged first, then UniProt adds go/from_uniprot/)
// isAlreadyMerged: if true, skip sources that already have output files (incremental mode)
// Returns total lines, per-source breakdown, and error
func concatenateOneDatasetIncremental(writerKey string, writer *DatasetBucketWriter, indexDir string, chunkIdx string, isAlreadyMerged bool) (uint64, map[string]uint64, error) {
	datasetName := writer.config.DatasetName
	isDerived := writer.config.IsDerived

	// Special handling for textsearch: uses same per-source approach but with different naming
	if datasetName == "textsearch" {
		lines, err := concatenateTextsearchPerSource(indexDir, chunkIdx, isDerived)
		// Textsearch doesn't need per-source tracking in dataset state
		return lines, nil, err
	}

	// Get bucket files grouped by source (forward, from_entrez, from_refseq, etc.)
	filesPerSource, err := GetBucketFilesPerSource(datasetName, indexDir, isDerived)
	if err != nil {
		return 0, nil, fmt.Errorf("getting bucket files per source for %s: %w", datasetName, err)
	}

	// Add pool's forward bucket files to the "forward" source
	poolForwardFiles := []string{}
	for _, bucket := range writer.buckets {
		if bucket.fileCreated && strings.Contains(bucket.filePath, "/forward/") {
			poolForwardFiles = append(poolForwardFiles, bucket.filePath)
		}
	}

	if len(poolForwardFiles) > 0 {
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
		if isAlreadyMerged {
			log.Printf("Dataset %s: already merged, no new sources to process", datasetName)
		}
		return 0, nil, nil
	}

	// For already-merged datasets, filter out sources that already have output files
	if isAlreadyMerged {
		newSources := make(map[string][]string)
		for sourceName, bucketFiles := range filesPerSource {
			// Check if any matching output file exists (with any chunkIdx)
			pattern := getSourceOutputPattern(datasetName, sourceName, indexDir, writer.setIndex)
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				// Output already exists, skip this source
				continue
			}
			newSources[sourceName] = bucketFiles
		}

		if len(newSources) == 0 {
			log.Printf("Dataset %s: already merged, no new reverse sources found", datasetName)
			return 0, nil, nil
		}

		log.Printf("Dataset %s: already merged but found %d new reverse sources to merge", datasetName, len(newSources))
		filesPerSource = newSources
	}

	// Delegate to the core merge logic
	return concatenateSourceFiles(datasetName, writer, filesPerSource, indexDir, chunkIdx)
}

// getSourceOutputPath returns the expected output file path for a source
func getSourceOutputPath(datasetName, sourceName, indexDir, chunkIdx string, setIndex int) string {
	if sourceName == "forward" {
		if setIndex >= 0 {
			return filepath.Join(indexDir, fmt.Sprintf("%s_sorted_%d.%s.index.gz", datasetName, setIndex+1, chunkIdx))
		}
		return filepath.Join(indexDir, fmt.Sprintf("%s_sorted.%s.index.gz", datasetName, chunkIdx))
	}
	// from_{source}
	if setIndex >= 0 {
		return filepath.Join(indexDir, fmt.Sprintf("%s_from_%s_sorted_%d.%s.index.gz", datasetName, sourceName, setIndex+1, chunkIdx))
	}
	return filepath.Join(indexDir, fmt.Sprintf("%s_from_%s_sorted.%s.index.gz", datasetName, sourceName, chunkIdx))
}

// getSourceOutputPattern returns a glob pattern to find existing output files for a source (any chunkIdx)
func getSourceOutputPattern(datasetName, sourceName, indexDir string, setIndex int) string {
	if sourceName == "forward" {
		if setIndex >= 0 {
			return filepath.Join(indexDir, fmt.Sprintf("%s_sorted_%d.*.index.gz", datasetName, setIndex+1))
		}
		return filepath.Join(indexDir, fmt.Sprintf("%s_sorted.*.index.gz", datasetName))
	}
	// from_{source}
	if setIndex >= 0 {
		return filepath.Join(indexDir, fmt.Sprintf("%s_from_%s_sorted_%d.*.index.gz", datasetName, sourceName, setIndex+1))
	}
	return filepath.Join(indexDir, fmt.Sprintf("%s_from_%s_sorted.*.index.gz", datasetName, sourceName))
}

// concatenateSourceFiles performs the actual k-way merge of bucket files for each source
// Returns total lines and per-source breakdown (sourceName -> lineCount)
func concatenateSourceFiles(datasetName string, writer *DatasetBucketWriter, filesPerSource map[string][]string, indexDir string, chunkIdx string) (uint64, map[string]uint64, error) {
	totalLines := uint64(0)
	perSourceLines := make(map[string]uint64)
	sourcesProcessed := 0

	// Process each source separately, creating its own output file
	for sourceName, bucketFiles := range filesPerSource {
		if len(bucketFiles) == 0 {
			continue
		}

		// Sort bucket files before merge
		for _, f := range bucketFiles {
			if _, err := os.Stat(f); os.IsNotExist(err) {
				continue
			}
			if err := SortBucketFile(f, writer.config.UseUnixSort); err != nil {
				log.Printf("Warning: error sorting bucket file %s: %v", f, err)
			}
		}

		sort.Strings(bucketFiles)

		outPath := getSourceOutputPath(datasetName, sourceName, indexDir, chunkIdx, writer.setIndex)

		outF, err := os.Create(outPath)
		if err != nil {
			return totalLines, perSourceLines, fmt.Errorf("creating output file %s: %w", outPath, err)
		}
		outGz, _ := gzip.NewWriterLevel(outF, gzip.BestSpeed)
		buf := bufio.NewWriterSize(outGz, BucketWriteBufferSize)

		linesWritten := uint64(0)

		// Initialize bucket readers for k-way merge (handles both compressed and uncompressed)
		readers := make([]*bucketReader, 0, len(bucketFiles))
		for _, bucketFile := range bucketFiles {
			br, err := newBucketReader(bucketFile)
			if err != nil {
				log.Printf("Warning: cannot open bucket file %s: %v", bucketFile, err)
				continue
			}
			if !br.eof {
				readers = append(readers, br)
			} else {
				br.close()
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
						readers[i].close()
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

		// Clean up bucket files after successful merge (unless keepBucketFiles is set)
		cleanedCount := 0
		if !KeepBucketFiles {
			for _, f := range bucketFiles {
				if err := os.Remove(f); err == nil {
					cleanedCount++
				}
			}
		}

		sourcesProcessed++
		totalLines += linesWritten
		perSourceLines[sourceName] = linesWritten

		if sourceName == "forward" {
			log.Printf("K-way merged %d buckets for %s (ID:%s) → %s (%d lines, cleaned %d files)",
				len(bucketFiles), datasetName, writer.datasetID, outPath, linesWritten, cleanedCount)
		} else {
			log.Printf("K-way merged %d buckets for %s from %s → %s (%d lines, cleaned %d files)",
				len(bucketFiles), datasetName, sourceName, outPath, linesWritten, cleanedCount)
		}
	}

	log.Printf("Dataset %s: processed %d sources, total %d lines", datasetName, sourcesProcessed, totalLines)
	return totalLines, perSourceLines, nil
}

// concatenateOneDataset merges bucket files for a single dataset, creating per-source output files
// This enables incremental updates - when a source dataset is updated, only its contribution files are rebuilt
// Output format:
//   - {datasetName}_sorted.{chunkIdx}.index.gz - dataset's own data (from forward/)
//   - {datasetName}_from_{source}_sorted.{chunkIdx}.index.gz - xrefs from other datasets (from from_*/)
// Returns total lines, per-source breakdown, and error
func concatenateOneDataset(writerKey string, writer *DatasetBucketWriter, indexDir string, chunkIdx string) (uint64, map[string]uint64, error) {
	// Delegate to incremental version with isAlreadyMerged=false (process all sources)
	return concatenateOneDatasetIncremental(writerKey, writer, indexDir, chunkIdx, false)
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
			if err := SortBucketFile(f, false); err != nil { // textsearch uses Go sort
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

		// Initialize bucket readers for k-way merge (handles both compressed and uncompressed)
		readers := make([]*bucketReader, 0, len(bucketFiles))
		for _, bucketFile := range bucketFiles {
			br, err := newBucketReader(bucketFile)
			if err != nil {
				log.Printf("Warning: cannot open bucket file %s: %v", bucketFile, err)
				continue
			}
			if !br.eof {
				readers = append(readers, br)
			} else {
				br.close()
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
						readers[i].close()
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
// If useUnixSort is true, uses Unix sort command (bounded memory)
func SortBucketsForDataset(datasetName string, indexDir string, isDerived bool, numWorkers int, useUnixSort bool) error {
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

	sortMethod := "Go in-memory"
	if useUnixSort {
		sortMethod = "Unix external"
	}
	log.Printf("Sorting %d bucket files for %s with %d workers (%s sort)", len(files), datasetName, numWorkers, sortMethod)

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
				if err := SortBucketFile(filePath, useUnixSort); err != nil {
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

	// Initialize bucket readers for k-way merge (handles both compressed and uncompressed)
	readers := make([]*bucketReader, 0, len(files))
	for _, bucketFile := range files {
		br, err := newBucketReader(bucketFile)
		if err != nil {
			log.Printf("Warning: cannot open bucket file %s: %v", bucketFile, err)
			continue
		}
		if !br.eof {
			readers = append(readers, br)
		} else {
			br.close()
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
					readers[i].close()
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
		PerDataset:       make(map[string]uint64),
		PerDatasetSource: make(map[string]map[string]uint64),
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
			if err := SortBucketsForDataset(datasetName, indexDir, isDerived, numWorkers, cfg.UseUnixSort); err != nil {
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
