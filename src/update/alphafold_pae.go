 package update

import (
	"archive/tar"
	"biobtree/pbuf"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pquerna/ffjson/ffjson"
)

// Default model organism taxonomy IDs (16 model organisms from AlphaFold)
var modelOrganismTaxIDs = []string{
	"9606", "10090", "3702", "10116", "559292", "284812", "83333", "6239",
	"39947", "7955", "7227", "44689", "237561", "243232", "3847", "4577",
}

// Regex to extract UniProt ID and version from PAE filename
// Format: AF-{UniProtID}-F{fragment}-predicted_aligned_error_v{version}.json.gz
var paeFilenameRegex = regexp.MustCompile(`AF-([A-Z0-9]+)-F(\d+)-predicted_aligned_error_v(\d+)\.json(?:\.gz)?`)

// processPAEData processes PAE (Predicted Aligned Error) data from local proteome tars
// Called by alphafoldProcessor after FTP processing completes
// Uses parallel workers for faster processing
func processPAEData(source string, sourceID string, d *DataUpdate, idLogFile *os.File, testLimit int) uint64 {
	// Get PAE data directory from config or use default
	paeDir := config.Dataconf[source]["paePath"]
	if paeDir == "" {
		paeDir = "/data/bioyoda/snapshots/raw_data/alphafold_pae"
	}

	// Check if directory exists
	if _, err := os.Stat(paeDir); os.IsNotExist(err) {
		log.Printf("PAE directory not found: %s - skipping PAE processing", paeDir)
		return 0
	}

	fmt.Printf("Processing AlphaFold PAE data from local files: %s\n", paeDir)

	// Get tax IDs from config or use default model organisms
	taxIDsStr := config.Dataconf[source]["proteomeTaxIds"]
	var taxIDs []string
	if taxIDsStr != "" {
		taxIDs = strings.Split(taxIDsStr, ",")
	} else {
		taxIDs = modelOrganismTaxIDs
	}

	// Collect all tar files across all species
	var allTarFiles []string
	for _, taxID := range taxIDs {
		taxID = strings.TrimSpace(taxID)
		if taxID == "" {
			continue
		}
		pattern := filepath.Join(paeDir, fmt.Sprintf("proteome-tax_id-%s-*_v4.tar", taxID))
		tarFiles, err := filepath.Glob(pattern)
		if err != nil {
			log.Printf("Warning: Error finding tar files for taxID %s: %v", taxID, err)
			continue
		}
		allTarFiles = append(allTarFiles, tarFiles...)
	}

	if len(allTarFiles) == 0 {
		log.Printf("No PAE tar files found in %s", paeDir)
		return 0
	}

	// Sort for consistent ordering
	sort.Strings(allTarFiles)

	// Get number of workers (default 8, configurable)
	numWorkers := 8
	if val, ok := config.Appconf["paeWorkers"]; ok {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			numWorkers = parsed
		}
	}

	fmt.Printf("Processing %d PAE tar files with %d parallel workers\n", len(allTarFiles), numWorkers)

	// Atomic counter for total processed
	var totalProcessed int64

	// Mutex for ID logging in test mode
	var idLogMutex sync.Mutex

	// Create worker pool
	var wg sync.WaitGroup
	tarChan := make(chan string, len(allTarFiles))

	// Track progress
	var filesProcessed int64

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for tarFile := range tarChan {
				// Check test limit before processing
				if testLimit > 0 && atomic.LoadInt64(&totalProcessed) >= int64(testLimit) {
					continue // Drain channel but don't process
				}

				processed, err := processProteomeTarPAEWorker(
					source, sourceID, d, tarFile, idLogFile, &idLogMutex,
					testLimit, &totalProcessed,
				)
				if err != nil {
					log.Printf("[Worker %d] Warning: Error processing %s: %v", workerID, filepath.Base(tarFile), err)
					continue
				}

				currentFiles := atomic.AddInt64(&filesProcessed, 1)
				currentTotal := atomic.LoadInt64(&totalProcessed)
				if currentFiles%10 == 0 || currentFiles == int64(len(allTarFiles)) {
					fmt.Printf("  PAE progress: %d/%d files, %d entries processed\n", currentFiles, len(allTarFiles), currentTotal)
				}

				_ = processed // Already added atomically
			}
		}(i)
	}

	// Send tar files to workers
	for _, tarFile := range allTarFiles {
		tarChan <- tarFile
	}
	close(tarChan)

	// Wait for all workers to complete
	wg.Wait()

	finalTotal := uint64(atomic.LoadInt64(&totalProcessed))
	fmt.Printf("AlphaFold PAE processing complete: %d entries processed from %d files\n", finalTotal, len(allTarFiles))

	return finalTotal
}

// processProteomeTarPAEWorker processes a single proteome tar file (worker-safe)
func processProteomeTarPAEWorker(source string, sourceID string, d *DataUpdate, tarPath string,
	idLogFile *os.File, idLogMutex *sync.Mutex, testLimit int, totalProcessed *int64) (uint64, error) {

	// Open tar file
	file, err := os.Open(tarPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open tar file: %v", err)
	}
	defer file.Close()

	// Create buffered tar reader (larger buffer for better I/O)
	br := bufio.NewReaderSize(file, 256*1024)
	tarReader := tar.NewReader(br)

	var localProcessed uint64

	// Iterate through tar entries
	for {
		// Check test limit
		if testLimit > 0 && atomic.LoadInt64(totalProcessed) >= int64(testLimit) {
			break
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return localProcessed, fmt.Errorf("error reading tar: %v", err)
		}

		// Only process PAE JSON files (gzipped)
		if !strings.Contains(header.Name, "predicted_aligned_error") {
			continue
		}
		if !strings.HasSuffix(header.Name, ".json.gz") && !strings.HasSuffix(header.Name, ".json") {
			continue
		}

		// Extract UniProt ID, fragment number, and version from filename
		uniprotID, fragment, version := extractPAEInfo(header.Name)
		if uniprotID == "" {
			continue
		}

		// Parse PAE JSON using streaming (handle gzipped files)
		var metrics *paeMetrics
		if strings.HasSuffix(header.Name, ".gz") {
			metrics, err = parsePAEJSONGzipped(tarReader, header.Size)
		} else {
			metrics, err = parsePAEJSON(tarReader, header.Size)
		}
		if err != nil {
			// Don't log every error - too noisy with parallel workers
			continue
		}

		// Create AlphaFold attribute with PAE data
		attr := pbuf.AlphaFoldAttr{
			MaxPae:               metrics.MaxPae,
			MeanPae:              metrics.MeanPae,
			FractionPaeConfident: metrics.FractionConfident,
			Version:              int32(version),
			FragmentNumber:       int32(fragment),
		}

		// Marshal and store on UniProt ID
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			continue
		}

		// addProp3 is thread-safe (uses internal locking)
		d.addProp3(uniprotID, sourceID, b)

		localProcessed++
		atomic.AddInt64(totalProcessed, 1)

		// Test mode: log ID with mutex protection
		if idLogFile != nil {
			idLogMutex.Lock()
			logProcessedID(idLogFile, uniprotID)
			idLogMutex.Unlock()
		}
	}

	return localProcessed, nil
}

// extractPAEInfo extracts UniProt ID, fragment, and version from PAE filename
// Format: AF-{UniProtID}-F{fragment}-predicted_aligned_error_v{version}.json.gz
func extractPAEInfo(filename string) (uniprotID string, fragment int, version int) {
	// Remove path prefix if present
	if idx := strings.LastIndex(filename, "/"); idx != -1 {
		filename = filename[idx+1:]
	}

	matches := paeFilenameRegex.FindStringSubmatch(filename)
	if len(matches) != 4 {
		return "", 0, 0
	}

	uniprotID = matches[1]
	fragment, _ = strconv.Atoi(matches[2])
	version, _ = strconv.Atoi(matches[3])

	return uniprotID, fragment, version
}

// paeMetrics holds the streaming-calculated PAE metrics
type paeMetrics struct {
	MaxPae           float64
	MeanPae          float64
	FractionConfident float64
}

// parsePAEJSONGzipped parses a gzipped PAE JSON file using streaming
func parsePAEJSONGzipped(reader io.Reader, size int64) (*paeMetrics, error) {
	// Read the gzipped content
	data := make([]byte, size)
	_, err := io.ReadFull(reader, data)
	if err != nil {
		return nil, fmt.Errorf("failed to read gzipped PAE JSON: %v", err)
	}

	// Create gzip reader from bytes
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	// Stream parse the JSON
	return streamParsePAEJSON(gzReader)
}

// parsePAEJSON parses an uncompressed PAE JSON file using streaming
func parsePAEJSON(reader io.Reader, size int64) (*paeMetrics, error) {
	// Read the JSON content
	data := make([]byte, size)
	_, err := io.ReadFull(reader, data)
	if err != nil {
		return nil, fmt.Errorf("failed to read PAE JSON: %v", err)
	}

	return streamParsePAEJSON(bytes.NewReader(data))
}

// streamParsePAEJSON uses streaming JSON decoder to calculate metrics without loading full matrix
func streamParsePAEJSON(reader io.Reader) (*paeMetrics, error) {
	decoder := json.NewDecoder(reader)

	var maxPae float64
	var sum float64
	var count, confidentCount int64
	foundMatrix := false

	// Read first token - could be '[' (array format) or '{' (object format)
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to read first token: %v", err)
	}

	// If array format, skip to first object
	if delim, ok := token.(json.Delim); ok && delim == '[' {
		token, err = decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to read object start: %v", err)
		}
	}

	// Now we should be at '{' of the object
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected object start, got %v", token)
	}

	// Process object fields
	for decoder.More() {
		// Read field name
		token, err = decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to read field name: %v", err)
		}

		fieldName, ok := token.(string)
		if !ok {
			continue
		}

		switch fieldName {
		case "max_predicted_aligned_error":
			// Read the scalar value
			var val float64
			if err := decoder.Decode(&val); err != nil {
				return nil, fmt.Errorf("failed to decode max_pae: %v", err)
			}
			maxPae = val

		case "predicted_aligned_error":
			// Stream through the 2D matrix without storing it
			foundMatrix = true
			sum, count, confidentCount, err = streamMatrixMetrics(decoder)
			if err != nil {
				return nil, fmt.Errorf("failed to stream matrix: %v", err)
			}

		default:
			// Skip unknown fields
			var skip json.RawMessage
			if err := decoder.Decode(&skip); err != nil {
				return nil, fmt.Errorf("failed to skip field %s: %v", fieldName, err)
			}
		}
	}

	if !foundMatrix || count == 0 {
		return nil, fmt.Errorf("empty PAE matrix")
	}

	return &paeMetrics{
		MaxPae:            maxPae,
		MeanPae:           sum / float64(count),
		FractionConfident: float64(confidentCount) / float64(count),
	}, nil
}

// streamMatrixMetrics streams through a 2D array calculating metrics on-the-fly
func streamMatrixMetrics(decoder *json.Decoder) (sum float64, count, confidentCount int64, err error) {
	// Read opening '[' of outer array
	token, err := decoder.Token()
	if err != nil {
		return 0, 0, 0, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return 0, 0, 0, fmt.Errorf("expected '[' for matrix, got %v", token)
	}

	// Process each row
	for decoder.More() {
		// Read opening '[' of row
		token, err = decoder.Token()
		if err != nil {
			return 0, 0, 0, err
		}
		if delim, ok := token.(json.Delim); !ok || delim != '[' {
			return 0, 0, 0, fmt.Errorf("expected '[' for row, got %v", token)
		}

		// Process each value in row
		for decoder.More() {
			token, err = decoder.Token()
			if err != nil {
				return 0, 0, 0, err
			}

			// Convert to float64
			var val float64
			switch v := token.(type) {
			case float64:
				val = v
			case json.Number:
				val, _ = v.Float64()
			default:
				continue
			}

			sum += val
			count++
			if val < 5.0 {
				confidentCount++
			}
		}

		// Read closing ']' of row
		token, err = decoder.Token()
		if err != nil {
			return 0, 0, 0, err
		}
	}

	// Read closing ']' of outer array
	_, err = decoder.Token()
	if err != nil {
		return 0, 0, 0, err
	}

	return sum, count, confidentCount, nil
}
