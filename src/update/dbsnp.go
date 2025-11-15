package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
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

// parseAndSaveVCF processes the dbSNP VCF file (contains all chromosomes)
func (db *dbsnp) parseAndSaveVCF(testLimit int, idLogFile *os.File) {
	// Download from NCBI FTP (both test and production mode)
	ftpServer := config.Dataconf[db.source]["ftpUrl"]
	basePath := config.Dataconf[db.source]["path"]
	vcfFileName := "GCF_000001405.40.gz"
	filePath := basePath + vcfFileName

	if config.IsTestMode() {
		log.Printf("dbSNP: [TEST MODE] Downloading VCF from ftp://%s%s (will stop after %d SNPs)", ftpServer, filePath, testLimit)
	} else {
		log.Printf("dbSNP: Downloading VCF from ftp://%s%s", ftpServer, filePath)
	}

	// Open VCF file from FTP (getDataReaderNew already handles gzip decompression)
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(db.source, ftpServer, "", filePath)
	db.check(err, "opening VCF file")

	// Ensure cleanup
	defer closeReaders(gz, ftpFile, client, localFile)

	// Use bufio.Scanner for line-by-line reading (handles large files)
	// br is already decompressed by getDataReaderNew
	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024 // 1MB buffer for long lines
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Track statistics
	var totalLines, savedSNPs, skippedLines int64
	snpsSaved := make(map[string]bool) // Track unique rs IDs

	// Source ID for cross-references
	sourceID := config.Dataconf[db.source]["id"]

	// Progress tracking
	var previous int64

	// Parse VCF line by line
	for scanner.Scan() {
		line := scanner.Text()
		totalLines++

		// Skip header lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// In test mode, check if we've reached the limit
		if testLimit > 0 && savedSNPs >= int64(testLimit) {
			log.Printf("dbSNP: [TEST MODE] Reached limit of %d SNPs, stopping", testLimit)
			break
		}

		// Parse VCF line
		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			skippedLines++
			continue
		}

		// VCF columns: CHROM POS ID REF ALT QUAL FILTER INFO
		chrom := fields[0]
		posStr := fields[1]
		rsID := fields[2]
		refAllele := fields[3]
		altAllele := fields[4]
		infoField := fields[7]

		// In test mode, filter to only chr1 for faster testing
		// In production mode, process all chromosomes (1-22, X, Y, MT)
		if config.IsTestMode() {
			// Only process chr1 in test mode
			if chrom != "1" && chrom != "NC_000001.11" {
				continue
			}
		}
		// In production mode, no chromosome filtering - process all

		// Skip if no rs ID
		if rsID == "." || !strings.HasPrefix(rsID, "rs") {
			continue
		}

		// Skip if already saved (VCF may have duplicates)
		if snpsSaved[rsID] {
			continue
		}

		// Parse position
		pos, err := strconv.ParseInt(posStr, 10, 64)
		if err != nil {
			skippedLines++
			continue
		}

		// Parse INFO field
		infoMap := db.parseINFO(infoField)

		// Build dbSNP attribute
		attr := &pbuf.DbsnpAttr{
			RsId:       rsID,
			Chromosome: db.normalizeChromosome(chrom),
			Position:   pos,
			RefAllele:  refAllele,
			AltAllele:  altAllele,
		}

		// Extract INFO fields
		if buildID, ok := infoMap["dbSNPBuildID"]; ok {
			attr.BuildId = buildID
		}

		if af, ok := infoMap["AF"]; ok {
			if afFloat, err := strconv.ParseFloat(af, 64); err == nil {
				attr.AlleleFrequency = afFloat
			}
		}

		if geneInfo, ok := infoMap["GENEINFO"]; ok {
			// GENEINFO format: "GENE:GENEID"
			parts := strings.Split(geneInfo, ":")
			if len(parts) >= 2 {
				attr.GeneName = parts[0]
				attr.GeneId = parts[1]
			}
		}

		if varClass, ok := infoMap["VC"]; ok {
			attr.VariantClass = varClass
		}

		if clinSig, ok := infoMap["CLNSIG"]; ok {
			attr.ClinicalSignificance = clinSig
		}

		// Determine variant type
		attr.VariantType = db.determineVariantType(refAllele, altAllele)

		// Save SNP
		db.saveSNP(rsID, attr, sourceID)
		snpsSaved[rsID] = true
		savedSNPs++

		// Log ID in test mode
		if idLogFile != nil {
			fmt.Fprintln(idLogFile, rsID)
		}

		// Create cross-references
		db.createCrossReferences(rsID, sourceID, attr)

		// Progress reporting
		elapsed := int64(time.Since(db.d.start).Seconds())
		if elapsed > previous+db.d.progInterval {
			previous = elapsed
			db.d.progChan <- &progressInfo{dataset: db.source, currentKBPerSec: int64(savedSNPs / int64(elapsed))}
		}
	}

	db.check(scanner.Err(), "scanning VCF file")

	log.Printf("dbSNP: Total lines read: %d, Saved: %d SNPs, Skipped: %d",
		totalLines, savedSNPs, skippedLines)

	// Update entry statistics
	atomic.AddUint64(&db.d.totalParsedEntry, uint64(savedSNPs))
	db.d.addEntryStat(db.source, uint64(savedSNPs))
}

// parseINFO parses the VCF INFO field into a map
func (db *dbsnp) parseINFO(infoField string) map[string]string {
	infoMap := make(map[string]string)
	fields := strings.Split(infoField, ";")

	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) == 2 {
			infoMap[parts[0]] = parts[1]
		} else {
			// Flag fields (no value)
			infoMap[parts[0]] = "true"
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

// saveSNP saves a SNP record to the database
func (db *dbsnp) saveSNP(rsID string, attr *pbuf.DbsnpAttr, sourceID string) {
	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	db.check(err, fmt.Sprintf("marshaling attributes for %s", rsID))

	// Save entry (using addProp3 like gwas.go)
	db.d.addProp3(rsID, sourceID, attrBytes)
}

// createCrossReferences creates cross-references from dbSNP to other datasets
func (db *dbsnp) createCrossReferences(rsID, sourceID string, attr *pbuf.DbsnpAttr) {
	textLinkID := "0" // Text search link ID

	// SNP → Gene (via gene_id from GENEINFO)
	if attr.GeneId != "" {
		db.d.addXref(rsID, sourceID, attr.GeneId, "gene", false)
	}

	// SNP → Gene name (text search for discovery)
	if attr.GeneName != "" && len(attr.GeneName) < 100 {
		db.d.addXref(attr.GeneName, textLinkID, rsID, db.source, true)
	}

	// Future: Add bidirectional links to GWAS and ClinVar
	// These will be added when we enhance gwas.go and clinvar.go
}
