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
	"sync"
)

// SortBucketFile sorts a single bucket file in place
func SortBucketFile(filePath string) error {
	// Read all lines
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return err
	}

	var lines []string
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max line
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	gz.Close()
	f.Close()

	if len(lines) == 0 {
		return nil // Empty file
	}

	// Sort lines
	sort.Strings(lines)

	// Write back (with deduplication)
	outPath := filePath + ".sorted"
	outF, err := os.Create(outPath)
	if err != nil {
		return err
	}
	outGz, _ := gzip.NewWriterLevel(outF, gzip.BestSpeed)
	buf := bufio.NewWriterSize(outGz, 65536)

	prevLine := ""
	for _, line := range lines {
		if line != prevLine { // Deduplicate
			buf.WriteString(line)
			buf.WriteByte('\n')
			prevLine = line
		}
	}

	buf.Flush()
	outGz.Close()
	outF.Close()

	// Replace original with sorted
	os.Remove(filePath)
	os.Rename(outPath, filePath)

	return nil
}

// SortAllBuckets sorts all bucket files in parallel
func SortAllBuckets(pool *HybridWriterPool, numWorkers int) error {
	files := pool.GetBucketFiles()
	if len(files) == 0 {
		return nil
	}

	fmt.Printf("Sorting %d bucket files with %d workers\n", len(files), numWorkers)

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
		return err // Return first error
	}

	fmt.Printf("Sorting complete for %d bucket files\n", len(files))
	return nil
}

// ConcatenateBuckets merges all sorted bucket files for a dataset into one
// Bucket files are preserved after concatenation for debugging
// Returns total lines written across all datasets (post-deduplication count)
func ConcatenateBuckets(pool *HybridWriterPool, indexDir string, chunkIdx string) (uint64, error) {
	var totalLines uint64

	for datasetID, writer := range pool.GetBucketWriters() {
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

		// Create output file in index directory
		// Format: {datasetName}_sorted.{chunkIdx}.index.gz
		outPath := filepath.Join(indexDir, fmt.Sprintf("%s_sorted.%s.index.gz",
			writer.config.DatasetName, chunkIdx))

		outF, err := os.Create(outPath)
		if err != nil {
			return 0, fmt.Errorf("creating output file %s: %w", outPath, err)
		}
		outGz, _ := gzip.NewWriterLevel(outF, gzip.BestSpeed)
		buf := bufio.NewWriterSize(outGz, 65536)

		linesWritten := uint64(0)

		// Concatenate all bucket files
		for _, bucketFile := range bucketFiles {
			f, err := os.Open(bucketFile)
			if err != nil {
				continue
			}
			gz, err := gzip.NewReader(f)
			if err != nil {
				f.Close()
				continue
			}

			// Copy content using io.Copy for efficiency
			reader := bufio.NewReaderSize(gz, 65536)
			for {
				line, err := reader.ReadString('\n')
				if err != nil && err != io.EOF {
					break
				}
				if len(line) > 0 {
					buf.WriteString(line)
					linesWritten++
				}
				if err == io.EOF {
					break
				}
			}

			gz.Close()
			f.Close()
		}

		buf.Flush()
		outGz.Close()
		outF.Close()

		totalLines += linesWritten

		log.Printf("Concatenated %d buckets for %s (ID:%s) → %s (%d lines)",
			len(bucketFiles), writer.config.DatasetName, datasetID, outPath, linesWritten)
	}

	return totalLines, nil
}
