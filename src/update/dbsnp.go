package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	// "encoding/json" // Fallback: use standard json if ffjson causes SIGBUS errors
)

type dbsnp struct {
	source string
	d      *DataUpdate
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

	// Process VCF file containing all human chromosomes
	// In test mode, only chr1 is processed due to filtering in parseAndSaveVCF
	// In production mode, all chromosomes (1-22, X, Y, MT) are processed
	db.parseAndSaveVCF(testLimit, idLogFile)

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

	numWorkers := 4
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

		// Save SNP using standard addProp3 (routes to bucket system via HybridWriterPool)
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
		"dbSNPBuildID":   true,
		"AF":             true,
		"GENEINFO":       true,
		"PSEUDOGENEINFO": true,
		"CLNSIG":         true,
		"VC":             true,
		"SAO":            true, // Variant Allele Origin
		"COMMON":         true, // Common SNP flag
		"NSF":            true, // Non-synonymous frameshift
		"NSM":            true, // Non-synonymous missense
		"NSN":            true, // Non-synonymous nonsense
		"SYN":            true, // Synonymous
		"U3":             true, // In 3' UTR
		"U5":             true, // In 5' UTR
		"ASS":            true, // Acceptor splice site
		"DSS":            true, // Donor splice site
		"INT":            true, // Intronic
		"R3":             true, // In 3' gene region
		"R5":             true, // In 5' gene region
		"SSR":            true, // Suspect Reason Codes
		"PM":             true, // Has associated publication
		"PUB":            true, // RefSNP mentioned in publication
		"GNO":            true, // Genotypes available
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

// saveSNP saves a SNP record using standard addProp3
// The bucket system (HybridWriterPool) automatically routes dbSNP data
// to bucket files based on rsID (configured in source.dataset.json)
func (db *dbsnp) saveSNP(rsID string, attr *pbuf.DbsnpAttr, sourceID string) {
	// Marshal attributes
	// Note: If you encounter SIGBUS errors during concurrent processing, this could be:
	// 1. Server/filesystem issues (most likely) - try running on different node
	// 2. ffjson buffer pool thread-safety issue (unlikely but possible)
	// To test #2, uncomment the json.Marshal line below and comment out ffjson.Marshal
	attrBytes, err := ffjson.Marshal(attr)
	// attrBytes, err := json.Marshal(attr) // Fallback: standard json (slower but thread-safe)
	db.check(err, fmt.Sprintf("marshaling attributes for %s", rsID))

	// Save entry using standard addProp3
	// HybridWriterPool routes this to bucket files (bucketMethod: "rsid", numBuckets: 100)
	db.d.addProp3(rsID, sourceID, attrBytes)
}

// createCrossReferences creates cross-references from dbSNP to other datasets
func (db *dbsnp) createCrossReferences(rsID, sourceID string, attr *pbuf.DbsnpAttr) {
	// SNP → Gene (via gene_ids from GENEINFO) - ALL genes
	for _, geneID := range attr.GeneIds {
		if geneID != "" {
			db.d.addXref(rsID, sourceID, geneID, "gene", false)
		}
	}

	// Gene names → SNP cross-reference via Ensembl lookup
	// Handles paralogs by filtering using chromosome and HGNC preference
	// Search "BRCA1" returns Ensembl entry (with embedded HGNC data), then "BRCA1 >> dbsnp" returns all SNPs
	// No limit - biobtree is deterministic, we show all genes or none
	for _, geneName := range attr.GeneNames {
		if geneName != "" && len(geneName) < 100 {
			db.d.addXrefViaGeneSymbol(geneName, attr.Chromosome, rsID, db.source, sourceID)
		}
	}

	// Pseudogene names → SNP cross-reference via Ensembl lookup
	// Same pattern as genes, but for pseudogenes
	for _, pseudogeneName := range attr.PseudogeneNames {
		if pseudogeneName != "" && len(pseudogeneName) < 100 {
			db.d.addXrefViaGeneSymbol(pseudogeneName, attr.Chromosome, rsID, db.source, sourceID)
		}
	}
}
