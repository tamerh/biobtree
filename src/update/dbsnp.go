package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// ============================================================================
// Bucket Sort Configuration
// ============================================================================
// Instead of using kvgen → binary merge pipeline (which creates thousands of
// chunk files), dbSNP uses bucket sort based on rsID numeric ranges.
// This reduces ~2000 chunk files to 1 final sorted file.
//
// How it works:
// 1. DISTRIBUTE: Route each rsID to bucket file based on numeric range
// 2. SORT: Sort each bucket in memory (one at a time)
// 3. CONCATENATE: Join sorted buckets into single file (already in order)
// ============================================================================

const (
	// DefaultNumBuckets is the default number of buckets for rsID distribution
	// 100 buckets with max rsID ~2B = ~20M rsIDs per bucket
	// Each bucket ~2-4GB when loaded for sorting
	// Configurable via: dbsnpNumBuckets in application.param.json
	DefaultNumBuckets = 100

	// DefaultNumWorkers is the default number of workers for parallel sorting
	// Configurable via: dbsnpNumWorkers in application.param.json
	DefaultNumWorkers = 4

	// MaxRsID is the maximum rsID number (for bucket calculation)
	// Current dbSNP has rsIDs up to ~2 billion
	MaxRsID = 2000000000

	// BucketFileBufferSize is the buffer size for bucket file writers
	BucketFileBufferSize = 4 * 1024 * 1024 // 4MB
)

// bucketWriter manages distribution of dbSNP entries to bucket files
type bucketWriter struct {
	numBuckets     int
	numWorkers     int   // number of workers for parallel sorting
	bucketSize     int64 // rsIDs per bucket
	outputDir      string
	files          []*os.File
	writers        []*bufio.Writer
	mutexes        []sync.Mutex
	counts         []int64 // entries per bucket (for logging)
	sourceID       string
	mergeGateCh    *chan mergeInfo // to send final file to merge pipeline
	chunkIdx       string          // chunk index for file naming consistency
	reverseXrefs   bool            // whether to create reverse xrefs (default: false)
	lookupXrefs    bool            // whether to create lookup-based xrefs (default: false)
	d              *DataUpdate     // reference to DataUpdate for reverse xrefs via kvgen
}

// newBucketWriter creates a new bucket writer
func newBucketWriter(outputDir string, numBuckets int, numWorkers int, sourceID string, chunkIdx string, mergeGateCh *chan mergeInfo, d *DataUpdate, reverseXrefs bool, lookupXrefs bool) (*bucketWriter, error) {
	bw := &bucketWriter{
		numBuckets:   numBuckets,
		numWorkers:   numWorkers,
		bucketSize:   MaxRsID / int64(numBuckets),
		outputDir:    outputDir,
		files:        make([]*os.File, numBuckets),
		writers:      make([]*bufio.Writer, numBuckets),
		mutexes:      make([]sync.Mutex, numBuckets),
		counts:       make([]int64, numBuckets),
		sourceID:     sourceID,
		mergeGateCh:  mergeGateCh,
		chunkIdx:     chunkIdx,
		reverseXrefs: reverseXrefs,
		lookupXrefs:  lookupXrefs,
		d:            d,
	}

	// Create bucket directory
	bucketDir := filepath.Join(outputDir, "dbsnp_buckets")
	if err := os.MkdirAll(bucketDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bucket directory: %w", err)
	}

	// Create bucket files using consistent naming: dbsnp_{bucket}.{chunkIdx}.txt
	for i := 0; i < numBuckets; i++ {
		fname := filepath.Join(bucketDir, fmt.Sprintf("dbsnp_%03d.%s.txt", i, chunkIdx))
		f, err := os.Create(fname)
		if err != nil {
			// Close any files we've already opened
			for j := 0; j < i; j++ {
				bw.files[j].Close()
			}
			return nil, fmt.Errorf("failed to create bucket file %d: %w", i, err)
		}
		bw.files[i] = f
		bw.writers[i] = bufio.NewWriterSize(f, BucketFileBufferSize)
	}

	log.Printf("dbSNP: Bucket writer initialized: buckets=%d (%.0fM rsIDs each), workers=%d, reverseXrefs=%v, lookupXrefs=%v",
		numBuckets, float64(bw.bucketSize)/1e6, numWorkers, reverseXrefs, lookupXrefs)

	return bw, nil
}

// getBucketIndex returns the bucket index for a given rsID
func (bw *bucketWriter) getBucketIndex(rsID string) int {
	// Extract numeric part from rsID (e.g., "rs123456789" → 123456789)
	numStr := strings.TrimPrefix(rsID, "rs")
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		// Fallback to bucket 0 for malformed rsIDs
		return 0
	}

	bucketIdx := int(num / bw.bucketSize)
	if bucketIdx >= bw.numBuckets {
		bucketIdx = bw.numBuckets - 1
	}
	return bucketIdx
}

// writeLine writes a KV line to the appropriate bucket
// Format: KEY\tSOURCE_ID\tATTR\tSTORE_ID (same as kvgen)
func (bw *bucketWriter) writeLine(rsID string, line string) {
	bucketIdx := bw.getBucketIndex(rsID)

	bw.mutexes[bucketIdx].Lock()
	bw.writers[bucketIdx].WriteString(line)
	bw.writers[bucketIdx].WriteByte('\n')
	bw.counts[bucketIdx]++
	bw.mutexes[bucketIdx].Unlock()
}

// writeXref writes a forward cross-reference to the bucket (rsID → target)
// Format: KEY\tFROM\tVALUE\tDATASETID (same as addXref2/one-way xref)
// If reverseXrefs is enabled, also creates reverse xref via kvgen
func (bw *bucketWriter) writeXref(rsID string, fromID string, value string, valueDataset string, valueDatasetID string) {
	if len(rsID) == 0 || len(value) == 0 {
		return
	}

	key := strings.ToUpper(rsID)
	val := strings.ToUpper(value)

	// Forward xref: rsID → target (goes to bucket)
	line := key + "\t" + fromID + "\t" + val + "\t" + valueDatasetID
	bw.writeLine(rsID, line)

	// Reverse xref: target → rsID (optionally via kvgen)
	if bw.reverseXrefs {
		// Use addXref2 for one-way reverse xref via kvgen pipeline
		bw.d.addXref2(value, valueDatasetID, rsID, "dbsnp")
	}
}

// flush flushes all bucket writers
func (bw *bucketWriter) flush() {
	for i := 0; i < bw.numBuckets; i++ {
		bw.mutexes[i].Lock()
		bw.writers[i].Flush()
		bw.mutexes[i].Unlock()
	}
}

// close closes all bucket files
func (bw *bucketWriter) close() {
	for i := 0; i < bw.numBuckets; i++ {
		bw.writers[i].Flush()
		bw.files[i].Close()
	}
}

// sortedBucket holds the result of sorting a bucket
type sortedBucket struct {
	index int
	lines []string
	err   error
}

// sortAndConcatenate sorts each bucket using parallel workers and concatenates into final output
// Returns the path to the final sorted file
func (bw *bucketWriter) sortAndConcatenate() (string, error) {
	bucketDir := filepath.Join(bw.outputDir, "dbsnp_buckets")
	// Use consistent naming with .index.gz like other merge files
	finalFile := filepath.Join(bw.outputDir, fmt.Sprintf("dbsnp_sorted.%s.index.gz", bw.chunkIdx))

	// Create final output file (gzipped)
	outFile, err := os.Create(finalFile)
	if err != nil {
		return "", fmt.Errorf("failed to create final output file: %w", err)
	}
	defer outFile.Close()

	gzWriter, err := gzip.NewWriterLevel(outFile, gzip.BestSpeed)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gzWriter.Close()

	bufWriter := bufio.NewWriterSize(gzWriter, BucketFileBufferSize)

	var totalLines int64
	startTime := time.Now()

	// Build list of non-empty buckets
	var nonEmptyBuckets []int
	for i := 0; i < bw.numBuckets; i++ {
		if bw.counts[i] > 0 {
			nonEmptyBuckets = append(nonEmptyBuckets, i)
		} else {
			// Remove empty bucket file
			bucketFile := filepath.Join(bucketDir, fmt.Sprintf("dbsnp_%03d.%s.txt", i, bw.chunkIdx))
			os.Remove(bucketFile)
		}
	}

	log.Printf("dbSNP: Sorting %d non-empty buckets with %d workers", len(nonEmptyBuckets), bw.numWorkers)

	// Process buckets in parallel batches, but write in order
	// We use a sliding window approach: sort numWorkers buckets ahead
	resultChan := make(chan sortedBucket, bw.numWorkers)
	pendingResults := make(map[int]sortedBucket) // buffer for out-of-order results
	nextToWrite := 0                              // next bucket index to write (in nonEmptyBuckets order)

	// Start initial batch of workers
	inFlight := 0
	nextToSort := 0

	// Helper to start a sort job
	startSortJob := func(bucketListIdx int) {
		bucketIdx := nonEmptyBuckets[bucketListIdx]
		go func(listIdx, bIdx int) {
			bucketFile := filepath.Join(bucketDir, fmt.Sprintf("dbsnp_%03d.%s.txt", bIdx, bw.chunkIdx))
			lines, err := bw.sortBucket(bucketFile)
			resultChan <- sortedBucket{index: listIdx, lines: lines, err: err}
			// Delete bucket file after sorting
			os.Remove(bucketFile)
		}(bucketListIdx, bucketIdx)
		inFlight++
	}

	// Start initial workers
	for nextToSort < len(nonEmptyBuckets) && nextToSort < bw.numWorkers {
		startSortJob(nextToSort)
		nextToSort++
	}

	// Process results and keep workers busy
	for nextToWrite < len(nonEmptyBuckets) {
		result := <-resultChan
		inFlight--

		if result.err != nil {
			return "", fmt.Errorf("failed to sort bucket %d: %w", nonEmptyBuckets[result.index], result.err)
		}

		// Buffer result if not next in order
		if result.index != nextToWrite {
			pendingResults[result.index] = result
		} else {
			// Write this result and any buffered sequential results
			for {
				var toWrite sortedBucket
				if result.index == nextToWrite {
					toWrite = result
				} else if pending, ok := pendingResults[nextToWrite]; ok {
					toWrite = pending
					delete(pendingResults, nextToWrite)
				} else {
					break
				}

				// Write sorted lines to final output
				var prevLine string
				for _, line := range toWrite.lines {
					if line != prevLine {
						bufWriter.WriteString(line)
						bufWriter.WriteByte('\n')
						totalLines++
						prevLine = line
					}
				}

				if nextToWrite%10 == 0 || nextToWrite == len(nonEmptyBuckets)-1 {
					log.Printf("dbSNP: Written bucket %d/%d (%d lines)",
						nextToWrite+1, len(nonEmptyBuckets), len(toWrite.lines))
				}

				nextToWrite++
				result.index = -1 // mark as processed
			}
		}

		// Start next sort job if available
		if nextToSort < len(nonEmptyBuckets) {
			startSortJob(nextToSort)
			nextToSort++
		}
	}

	bufWriter.Flush()

	// Remove bucket directory
	os.Remove(bucketDir)

	elapsed := time.Since(startTime)
	log.Printf("dbSNP: Sort and concatenate complete: %d total lines in %.1fs",
		totalLines, elapsed.Seconds())

	return finalFile, nil
}

// sortBucket reads a bucket file, sorts it in memory, and returns sorted lines
func (bw *bucketWriter) sortBucket(bucketFile string) ([]string, error) {
	// Read all lines from bucket file
	f, err := os.Open(bucketFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	// Increase scanner buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // 10MB max line size

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Sort lines (by first field which is the key)
	sort.Strings(lines)

	return lines, nil
}

// finalize sorts buckets, concatenates, and sends to merge pipeline
func (bw *bucketWriter) finalize() error {
	// Close all bucket files first
	bw.close()

	// Log bucket distribution
	var totalEntries int64
	nonEmptyBuckets := 0
	for i := 0; i < bw.numBuckets; i++ {
		totalEntries += bw.counts[i]
		if bw.counts[i] > 0 {
			nonEmptyBuckets++
		}
	}
	log.Printf("dbSNP: Bucket distribution: %d entries across %d non-empty buckets",
		totalEntries, nonEmptyBuckets)

	// Sort and concatenate
	finalFile, err := bw.sortAndConcatenate()
	if err != nil {
		return err
	}

	// Send final file to merge pipeline
	*bw.mergeGateCh <- mergeInfo{
		fname: finalFile,
		level: 1, // Will be picked up by merge pipeline
	}

	log.Printf("dbSNP: Final sorted file sent to merge pipeline: %s", finalFile)

	return nil
}

type dbsnp struct {
	source       string
	d            *DataUpdate
	bucketWriter *bucketWriter
}

// Helper for context-aware error checking
func (db *dbsnp) check(err error, operation string) {
	checkWithContext(err, db.source, operation)
}

// Main update entry point
func (db *dbsnp) update() {
	defer db.d.wg.Done()

	log.Println("dbSNP: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(db.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, db.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("dbSNP: [TEST MODE] Processing up to %d SNPs", testLimit)
	}

	// Initialize bucket writer for optimized sorting
	// Uses bucket sort based on rsID ranges instead of kvgen pipeline
	// All config options are read from application.param.json with dbsnp prefix
	sourceID := config.Dataconf[db.source]["id"]

	// dbsnpNumBuckets: Number of buckets for rsID distribution (default: 100)
	numBuckets := DefaultNumBuckets
	if bucketStr, ok := config.Appconf["dbsnpNumBuckets"]; ok {
		if n, err := strconv.Atoi(bucketStr); err == nil && n > 0 {
			numBuckets = n
		}
	}

	// dbsnpNumWorkers: Number of parallel workers for sorting (default: 4)
	numWorkers := DefaultNumWorkers
	if workerStr, ok := config.Appconf["dbsnpNumWorkers"]; ok {
		if n, err := strconv.Atoi(workerStr); err == nil && n > 0 {
			numWorkers = n
		}
	}

	// dbsnpReverseXrefs: Create reverse xrefs gene→rsID via kvgen (default: no)
	reverseXrefs := false
	if reverseStr, ok := config.Appconf["dbsnpReverseXrefs"]; ok {
		reverseXrefs = (reverseStr == "yes" || reverseStr == "true" || reverseStr == "y")
	}

	// dbsnpLookupXrefs: Create lookup-based xrefs via Ensembl lookup (default: no)
	lookupXrefs := false
	if lookupStr, ok := config.Appconf["dbsnpLookupXrefs"]; ok {
		lookupXrefs = (lookupStr == "yes" || lookupStr == "true" || lookupStr == "y")
	}

	bw, err := newBucketWriter(config.Appconf["indexDir"], numBuckets, numWorkers, sourceID, chunkIdx, db.d.mergeGateCh, db.d, reverseXrefs, lookupXrefs)
	if err != nil {
		log.Printf("dbSNP: ERROR - Failed to initialize bucket writer: %v", err)
		return
	}
	db.bucketWriter = bw

	// Process VCF file containing all human chromosomes
	// In test mode, only chr1 is processed due to filtering in parseAndSaveVCF
	// In production mode, all chromosomes (1-22, X, Y, MT) are processed
	db.parseAndSaveVCF(testLimit, idLogFile)

	// Finalize bucket writer - sort buckets and concatenate
	log.Println("dbSNP: All chromosomes processed. Starting bucket sort phase...")
	sortStartTime := time.Now()
	if err := db.bucketWriter.finalize(); err != nil {
		log.Printf("dbSNP: ERROR - Failed to finalize bucket writer: %v", err)
		return
	}
	log.Printf("dbSNP: Bucket sort phase complete (%.2fs)", time.Since(sortStartTime).Seconds())

	log.Printf("dbSNP: Processing complete (%.2fs)", time.Since(startTime).Seconds())
}

// getVCFUrl returns the HTTPS URL for the dbSNP VCF file
func (db *dbsnp) getVCFUrl() string {
	basePath := config.Dataconf[db.source]["path"]
	vcfFileName := "GCF_000001405.40.gz"

	// Check if path is already a full URL
	if strings.HasPrefix(basePath, "https://") || strings.HasPrefix(basePath, "http://") {
		return basePath + vcfFileName
	}

	// For local file mode, construct the local path
	if _, ok := config.Dataconf[db.source]["useLocalFile"]; ok && config.Dataconf[db.source]["useLocalFile"] == "yes" {
		return filepath.Join(basePath, vcfFileName)
	}

	// Default to NCBI HTTPS URL
	return "https://ftp.ncbi.nlm.nih.gov/snp/latest_release/VCF/" + vcfFileName
}

// getChromosomes returns the list of all chromosomes/contigs from the VCF file
// Uses tabix -l to dynamically get all available sequences
// This includes main chromosomes (NC_*) and contigs (NT_*, NW_*)
func (db *dbsnp) getChromosomes(vcfURL string) []string {
	// Use tabix -l to list all chromosomes/contigs in the VCF
	cmd := exec.Command("tabix", "-l", vcfURL)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("dbSNP: Warning - failed to get chromosome list from tabix: %v", err)
		log.Printf("dbSNP: Falling back to main chromosomes only")
		// Fallback to main chromosomes if tabix -l fails
		return []string{
			"NC_000001.11", "NC_000002.12", "NC_000003.12", "NC_000004.12",
			"NC_000005.10", "NC_000006.12", "NC_000007.14", "NC_000008.11",
			"NC_000009.12", "NC_000010.11", "NC_000011.10", "NC_000012.12",
			"NC_000013.11", "NC_000014.9", "NC_000015.10", "NC_000016.10",
			"NC_000017.11", "NC_000018.10", "NC_000019.10", "NC_000020.11",
			"NC_000021.9", "NC_000022.11", "NC_000023.11", "NC_000024.10",
			"NC_012920.1",
		}
	}

	// Parse output - one chromosome/contig per line
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	chromosomes := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			chromosomes = append(chromosomes, line)
		}
	}

	// Count main chromosomes vs contigs for logging
	mainCount := 0
	contigCount := 0
	for _, c := range chromosomes {
		if strings.HasPrefix(c, "NC_") {
			mainCount++
		} else {
			contigCount++
		}
	}
	log.Printf("dbSNP: Found %d sequences (%d main chromosomes, %d contigs)",
		len(chromosomes), mainCount, contigCount)

	return chromosomes
}

// parseAndSaveVCF processes the dbSNP VCF file using parallel tabix streams
// Each worker processes a different chromosome via tabix remote streaming
func (db *dbsnp) parseAndSaveVCF(testLimit int, idLogFile *os.File) {
	vcfURL := db.getVCFUrl()

	// In test mode, only process chr1
	var chromosomes []string
	if config.IsTestMode() {
		chromosomes = []string{"NC_000001.11"}
		log.Printf("dbSNP: [TEST MODE] Processing only chr1, limit %d SNPs", testLimit)
	} else {
		// Get all chromosomes and contigs from tabix
		chromosomes = db.getChromosomes(vcfURL)
	}

	// Use dbsnpNumWorkers for VCF parsing workers (same as sorting workers)
	numWorkers := DefaultNumWorkers
	if workerStr, ok := config.Appconf["dbsnpNumWorkers"]; ok {
		if n, err := strconv.Atoi(workerStr); err == nil && n > 0 {
			numWorkers = n
		}
	}
	log.Printf("dbSNP: Processing %d chromosomes with %d parallel workers via tabix", len(chromosomes), numWorkers)
	log.Printf("dbSNP: Remote VCF URL: %s", vcfURL)

	// Shared counters (atomic for thread safety)
	var totalSavedSNPs int64
	var totalSkippedLines int64

	// Source ID for cross-references
	sourceID := config.Dataconf[db.source]["id"]

	// Create worker pool
	var wg sync.WaitGroup
	chromChan := make(chan string, len(chromosomes))

	// ID log file mutex (for test mode)
	var idLogMutex sync.Mutex

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for chrom := range chromChan {
				savedCount, skippedCount := db.processChromosome(
					workerID, vcfURL, chrom, sourceID, testLimit,
					&totalSavedSNPs, idLogFile, &idLogMutex,
				)

				atomic.AddInt64(&totalSavedSNPs, savedCount)
				atomic.AddInt64(&totalSkippedLines, skippedCount)

				// Check if we've hit the test limit
				if testLimit > 0 && atomic.LoadInt64(&totalSavedSNPs) >= int64(testLimit) {
					log.Printf("dbSNP: [TEST MODE] Reached limit of %d SNPs", testLimit)
					return
				}
			}
		}(i)
	}

	// Send chromosomes to workers
	for _, chrom := range chromosomes {
		// Check if we've hit the test limit before sending more work
		if testLimit > 0 && atomic.LoadInt64(&totalSavedSNPs) >= int64(testLimit) {
			break
		}
		chromChan <- chrom
	}
	close(chromChan)

	// Wait for all workers to complete
	wg.Wait()

	log.Printf("dbSNP: Total saved: %d SNPs, Skipped: %d lines",
		totalSavedSNPs, totalSkippedLines)

	// Update entry statistics
	atomic.AddUint64(&db.d.totalParsedEntry, uint64(totalSavedSNPs))
	db.d.addEntryStat(db.source, uint64(totalSavedSNPs))
}

// processChromosome processes a single chromosome using tabix
// Returns (savedCount, skippedCount)
func (db *dbsnp) processChromosome(
	workerID int,
	vcfURL string,
	chrom string,
	sourceID string,
	testLimit int,
	globalSavedCount *int64,
	idLogFile *os.File,
	idLogMutex *sync.Mutex,
) (int64, int64) {

	log.Printf("[Worker %d] Starting chromosome %s", workerID, chrom)
	startTime := time.Now()

	// Run tabix to stream chromosome data
	// tabix automatically fetches the .tbi index from vcfURL.tbi
	cmd := exec.Command("tabix", vcfURL, chrom)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[Worker %d] Error creating pipe for %s: %v", workerID, chrom, err)
		return 0, 0
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[Worker %d] Error starting tabix for %s: %v", workerID, chrom, err)
		return 0, 0
	}

	// Read tabix output
	reader := bufio.NewReaderSize(stdout, 4*1024*1024) // 4MB buffer

	var savedSNPs, skippedLines int64
	lastProgress := time.Now() // Initialize to now to avoid immediate logging

	for {
		// Check global limit
		if testLimit > 0 && atomic.LoadInt64(globalSavedCount)+savedSNPs >= int64(testLimit) {
			cmd.Process.Kill() // Kill tabix process when limit reached
			break
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if len(line) > 0 {
					// Process last line
				} else {
					break
				}
			} else {
				log.Printf("[Worker %d] Error reading from tabix for %s: %v", workerID, chrom, err)
				break
			}
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse VCF line
		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			skippedLines++
			continue
		}

		// VCF columns: CHROM POS ID REF ALT QUAL FILTER INFO
		chromField := fields[0]
		posStr := fields[1]
		rsID := fields[2]
		refAllele := fields[3]
		altAllele := fields[4]
		infoField := fields[7]

		// Skip if no rs ID
		if rsID == "." || !strings.HasPrefix(rsID, "rs") {
			continue
		}

		// Parse position
		pos, parseErr := strconv.ParseInt(posStr, 10, 64)
		if parseErr != nil {
			skippedLines++
			continue
		}

		// Parse INFO field
		infoMap := db.parseINFO(infoField)

		// Build dbSNP attribute
		attr := &pbuf.DbsnpAttr{
			RsId:       rsID,
			Chromosome: db.normalizeChromosome(chromField),
			Position:   pos,
			RefAllele:  refAllele,
			AltAllele:  altAllele,
		}

		// Extract INFO fields
		if buildID, ok := infoMap["dbSNPBuildID"]; ok {
			attr.BuildId = buildID
		}

		if af, ok := infoMap["AF"]; ok {
			if afFloat, parseErr := strconv.ParseFloat(af, 64); parseErr == nil {
				attr.AlleleFrequency = afFloat
			}
		}

		// Parse GENEINFO
		if geneInfo, ok := infoMap["GENEINFO"]; ok {
			genePairs := strings.Split(geneInfo, "|")
			for _, pair := range genePairs {
				parts := strings.Split(pair, ":")
				if len(parts) >= 2 {
					attr.GeneNames = append(attr.GeneNames, parts[0])
					attr.GeneIds = append(attr.GeneIds, parts[1])
				}
			}
		}

		// Parse PSEUDOGENEINFO
		if pseudogeneInfo, ok := infoMap["PSEUDOGENEINFO"]; ok {
			pseudogenePairs := strings.Split(pseudogeneInfo, "|")
			for _, pair := range pseudogenePairs {
				parts := strings.Split(pair, ":")
				if len(parts) >= 2 {
					attr.PseudogeneNames = append(attr.PseudogeneNames, parts[0])
					attr.PseudogeneIds = append(attr.PseudogeneIds, parts[1])
				}
			}
		}

		if varClass, ok := infoMap["VC"]; ok {
			attr.VariantClass = varClass
		}

		if clinSig, ok := infoMap["CLNSIG"]; ok {
			attr.ClinicalSignificance = clinSig
		}

		// Parse SAO (Variant Allele Origin)
		if sao, ok := infoMap["SAO"]; ok {
			if saoInt, parseErr := strconv.ParseInt(sao, 10, 32); parseErr == nil {
				attr.Sao = int32(saoInt)
			}
		}

		// Parse flags
		if _, ok := infoMap["COMMON"]; ok {
			attr.IsCommon = true
		}
		if _, ok := infoMap["NSF"]; ok {
			attr.Nsf = true
		}
		if _, ok := infoMap["NSM"]; ok {
			attr.Nsm = true
		}
		if _, ok := infoMap["NSN"]; ok {
			attr.Nsn = true
		}
		if _, ok := infoMap["SYN"]; ok {
			attr.Syn = true
		}
		if _, ok := infoMap["U3"]; ok {
			attr.U3 = true
		}
		if _, ok := infoMap["U5"]; ok {
			attr.U5 = true
		}
		if _, ok := infoMap["ASS"]; ok {
			attr.Ass = true
		}
		if _, ok := infoMap["DSS"]; ok {
			attr.Dss = true
		}
		if _, ok := infoMap["INT"]; ok {
			attr.Intron = true
		}
		if _, ok := infoMap["R3"]; ok {
			attr.R3 = true
		}
		if _, ok := infoMap["R5"]; ok {
			attr.R5 = true
		}
		if ssr, ok := infoMap["SSR"]; ok {
			if ssrInt, parseErr := strconv.ParseInt(ssr, 10, 32); parseErr == nil {
				attr.Ssr = int32(ssrInt)
			}
		}
		if _, ok := infoMap["PM"]; ok {
			attr.HasPublication = true
		}
		if _, ok := infoMap["PUB"]; ok {
			attr.HasPubmedRef = true
		}
		if _, ok := infoMap["GNO"]; ok {
			attr.HasGenotypes = true
		}

		// Determine variant type
		attr.VariantType = db.determineVariantType(refAllele, altAllele)

		// Save SNP
		db.saveSNP(rsID, attr, sourceID)
		savedSNPs++

		// Log ID in test mode (thread-safe)
		if idLogFile != nil {
			idLogMutex.Lock()
			fmt.Fprintln(idLogFile, rsID)
			idLogMutex.Unlock()
		}

		// Create cross-references
		db.createCrossReferences(rsID, sourceID, attr)

		// Progress reporting (per worker, every 30 seconds)
		if time.Since(lastProgress) > 30*time.Second {
			lastProgress = time.Now()
			log.Printf("[Worker %d] %s: %d SNPs processed", workerID, chrom, savedSNPs)
		}

		if err == io.EOF {
			break
		}
	}

	// Wait for tabix to finish
	cmd.Wait()

	elapsed := time.Since(startTime)
	rate := float64(savedSNPs) / elapsed.Seconds()
	log.Printf("[Worker %d] Completed %s: %d SNPs in %.1fs (%.0f SNPs/s)",
		workerID, chrom, savedSNPs, elapsed.Seconds(), rate)

	return savedSNPs, skippedLines
}

// parseINFO parses the VCF INFO field into a map
// OPTIMIZED: Uses streaming parse to avoid allocating full split array in memory
// This handles large INFO fields (even MB-sized) without memory explosion
func (db *dbsnp) parseINFO(infoField string) map[string]string {
	infoMap := make(map[string]string, 8) // Pre-allocate for typical number of fields we need

	// Fields we actually care about (for targeted extraction)
	// Only parse what we'll use to save memory
	targetFields := map[string]bool{
		"dbSNPBuildID":    true,
		"AF":              true,
		"GENEINFO":        true,
		"PSEUDOGENEINFO":  true,
		"CLNSIG":          true,
		"VC":              true,
		"SAO":             true,  // Variant Allele Origin
		"COMMON":          true,  // Common SNP flag
		"NSF":             true,  // Non-synonymous frameshift
		"NSM":             true,  // Non-synonymous missense
		"NSN":             true,  // Non-synonymous nonsense
		"SYN":             true,  // Synonymous
		"U3":              true,  // In 3' UTR
		"U5":              true,  // In 5' UTR
		"ASS":             true,  // Acceptor splice site
		"DSS":             true,  // Donor splice site
		"INT":             true,  // Intronic
		"R3":              true,  // In 3' gene region
		"R5":              true,  // In 5' gene region
		"SSR":             true,  // Suspect Reason Codes
		"PM":              true,  // Has associated publication
		"PUB":             true,  // RefSNP mentioned in publication
		"GNO":             true,  // Genotypes available
	}

	// Manual parsing to avoid strings.Split() which allocates entire array
	// This streams through the string and only extracts what we need
	// Memory usage: O(fields_we_need) instead of O(total_fields)
	start := 0
	for i := 0; i < len(infoField); i++ {
		if infoField[i] == ';' || i == len(infoField)-1 {
			end := i
			if i == len(infoField)-1 {
				end = i + 1
			}

			field := infoField[start:end]
			start = i + 1

			// Find the '=' separator (using IndexByte is faster than Split)
			eqIdx := strings.IndexByte(field, '=')
			if eqIdx == -1 {
				// Flag field (no value)
				if targetFields[field] {
					infoMap[field] = "true"
				}
			} else {
				key := field[:eqIdx]
				// Only extract fields we need - this is the key optimization
				if targetFields[key] {
					value := field[eqIdx+1:]
					infoMap[key] = value
				}
			}
		}
	}

	 

	return infoMap
}

// normalizeChromosome converts RefSeq accession to simple chromosome name
func (db *dbsnp) normalizeChromosome(chrom string) string {
	// RefSeq format: NC_000001.11 → 1
	if strings.HasPrefix(chrom, "NC_") {
		parts := strings.Split(chrom, ".")
		if len(parts) > 0 {
			accNum := strings.TrimPrefix(parts[0], "NC_")
			// NC_000001 → 1, NC_000023 → X, NC_000024 → Y, NC_012920 → MT
			switch accNum {
			case "000023":
				return "X"
			case "000024":
				return "Y"
			case "012920":
				return "MT"
			default:
				// Remove leading zeros
				chrNum, _ := strconv.Atoi(accNum)
				return strconv.Itoa(chrNum)
			}
		}
	}
	return chrom
}

// determineVariantType determines the variant type based on alleles
func (db *dbsnp) determineVariantType(ref, alt string) string {
	refLen := len(ref)
	altLen := len(alt)

	if refLen == 1 && altLen == 1 {
		return "SNV" // Single Nucleotide Variant
	} else if refLen > altLen {
		return "deletion"
	} else if refLen < altLen {
		return "insertion"
	} else if refLen == altLen && refLen > 1 {
		return "MNV" // Multi-Nucleotide Variant
	}

	return "complex"
}

// saveSNP saves a SNP record to the bucket writer
func (db *dbsnp) saveSNP(rsID string, attr *pbuf.DbsnpAttr, sourceID string) {
	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	db.check(err, fmt.Sprintf("marshaling attributes for %s", rsID))

	// Format: KEY\tSOURCE_ID\tATTR\tSTORE_ID (same as kvgen/addProp3)
	// textStoreID = "-1" for property entries
	key := strings.ToUpper(rsID)
	line := key + "\t" + sourceID + "\t" + string(attrBytes) + "\t-1"

	// Write to bucket (instead of kvdatachan → kvgen pipeline)
	db.bucketWriter.writeLine(rsID, line)
}

// createCrossReferences creates cross-references from dbSNP to other datasets
// Forward xrefs (rsID → target) go to bucket, reverse xrefs (target → rsID) optionally via kvgen
func (db *dbsnp) createCrossReferences(rsID, sourceID string, attr *pbuf.DbsnpAttr) {
	// SNP → Gene (via gene_ids from GENEINFO) - ALL genes
	// Forward xref goes to bucket, reverse xref optional via kvgen
	if geneDatasetID, ok := config.Dataconf["gene"]["id"]; ok {
		for _, geneID := range attr.GeneIds {
			if geneID != "" {
				db.bucketWriter.writeXref(rsID, sourceID, geneID, "gene", geneDatasetID)
			}
		}
	}

	// Gene names → SNP cross-reference via Ensembl lookup
	// This is more complex (lookup-based), uses kvgen pipeline
	// Only enabled if dbsnpLookupXrefs is true
	if db.bucketWriter.lookupXrefs {
		for _, geneName := range attr.GeneNames {
			if geneName != "" && len(geneName) < 100 {
				db.d.addXrefViaGeneSymbol(geneName, attr.Chromosome, rsID, db.source, sourceID)
			}
		}

		// Pseudogene names → SNP cross-reference via Ensembl lookup
		for _, pseudogeneName := range attr.PseudogeneNames {
			if pseudogeneName != "" && len(pseudogeneName) < 100 {
				db.d.addXrefViaGeneSymbol(pseudogeneName, attr.Chromosome, rsID, db.source, sourceID)
			}
		}
	}
}
