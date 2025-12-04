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

// ConcatenateBuckets merges all sorted bucket files for a dataset into one
// Uses k-way merge to maintain global sort order across buckets
// Reads uncompressed .txt bucket files, writes compressed .gz output
// Bucket files are preserved after concatenation for debugging
// Returns total lines written across all datasets (post-deduplication count)
// For multi-bucket-set datasets, produces separate output files: dataset_sorted_1.gz, dataset_sorted_2.gz
func ConcatenateBuckets(pool *HybridWriterPool, indexDir string, chunkIdx string) (uint64, error) {
	var totalLines uint64

	for writerKey, writer := range pool.GetBucketWriters() {
		bucketFiles := []string{}
		for _, bucket := range writer.buckets {
			if bucket.fileCreated {
				bucketFiles = append(bucketFiles, bucket.filePath)
			}
		}

		if len(bucketFiles) == 0 {
			continue
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

		totalLines += linesWritten

		// Log message includes set info for multi-set
		if writer.setIndex >= 0 {
			log.Printf("K-way merged %d buckets for %s set%d (ID:%s) → %s (%d lines)",
				len(bucketFiles), writer.config.DatasetName, writer.setIndex+1, writerKey, outPath, linesWritten)
		} else {
			log.Printf("K-way merged %d buckets for %s (ID:%s) → %s (%d lines)",
				len(bucketFiles), writer.config.DatasetName, writer.datasetID, outPath, linesWritten)
		}
	}

	return totalLines, nil
}
