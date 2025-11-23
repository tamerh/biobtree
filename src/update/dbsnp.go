package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
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
	// Download from NCBI (supports both FTP and HTTPS)
	ftpServer := config.Dataconf[db.source]["ftpUrl"]
	basePath := config.Dataconf[db.source]["path"]
	vcfFileName := "GCF_000001405.40.gz"
	filePath := basePath + vcfFileName

	// Log the download URL (detect HTTPS vs FTP)
	downloadURL := filePath
	if ftpServer != "" && !strings.HasPrefix(filePath, "http") {
		downloadURL = "ftp://" + ftpServer + filePath
	}

	if config.IsTestMode() {
		log.Printf("dbSNP: [TEST MODE] Downloading VCF from %s (will stop after %d SNPs)", downloadURL, testLimit)
	} else {
		log.Printf("dbSNP: Downloading VCF from %s", downloadURL)
	}

	// Open VCF file (getDataReaderNew handles both FTP and HTTPS, and gzip decompression)
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(db.source, ftpServer, "", filePath)
	db.check(err, "opening VCF file")

	// Ensure cleanup
	defer closeReaders(gz, ftpFile, client, localFile)

	// Use bufio.Reader with ReadString for robust line reading
	// Scanner has buffer limitations; ReadString is more reliable for massive ETL
	// br is already decompressed by getDataReaderNew
	reader := bufio.NewReaderSize(br, 4*1024*1024) // 4MB buffer

	// Track statistics
	var totalLines, savedSNPs, skippedLines int64
	// NOTE: We don't need a deduplication map because NCBI dbSNP VCF files
	// are already deduplicated at source. Tracking millions of rs IDs
	// would consume 50-100GB of memory unnecessarily.

	// Source ID for cross-references
	sourceID := config.Dataconf[db.source]["id"]

	// Progress tracking
	var previous int64

	// Parse VCF line by line
	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			if err == io.EOF {
				// Process last line if it exists (file may not end with newline)
				if len(line) > 0 {
					totalLines++
					// Process this last line below
				} else {
					break // End of file reached successfully
				}
			} else {
				// Actual error occurred
				db.check(err, "reading VCF line")
				break
			}
		}

		totalLines++
		line = strings.TrimSpace(line)

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

		// NOTE: No deduplication check needed - NCBI VCF files are pre-deduplicated
		// If duplicates exist, addProp3() will overwrite (upsert behavior)

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

		// Parse GENEINFO: Format is "GENE1:GENEID1|GENE2:GENEID2|..."
		// Example: "WASH7P:653635|DDX11L1:100287102"
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

		// Parse PSEUDOGENEINFO: Same format as GENEINFO
		// Example: "DDX11L1:100287102"
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
			if saoInt, err := strconv.ParseInt(sao, 10, 32); err == nil {
				attr.Sao = int32(saoInt)
			}
		}

		// Parse COMMON flag
		if _, ok := infoMap["COMMON"]; ok {
			attr.IsCommon = true
		}

		// Parse functional impact flags (coding region effects)
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

		// Parse UTR and splice site flags
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

		// Parse gene region flags
		if _, ok := infoMap["INT"]; ok {
			attr.Intron = true
		}
		if _, ok := infoMap["R3"]; ok {
			attr.R3 = true
		}
		if _, ok := infoMap["R5"]; ok {
			attr.R5 = true
		}

		// Parse quality and evidence indicators
		if ssr, ok := infoMap["SSR"]; ok {
			if ssrInt, err := strconv.ParseInt(ssr, 10, 32); err == nil {
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

		// Save SNP (streaming - no accumulation in memory)
		db.saveSNP(rsID, attr, sourceID)
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

		// Check if we hit EOF after processing the last line
		if err == io.EOF {
			break
		}
	}

	log.Printf("dbSNP: Total lines read: %d, Saved: %d SNPs, Skipped: %d",
		totalLines, savedSNPs, skippedLines)

	// Update entry statistics
	atomic.AddUint64(&db.d.totalParsedEntry, uint64(savedSNPs))
	db.d.addEntryStat(db.source, uint64(savedSNPs))
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

	// Future: Add bidirectional links to GWAS and ClinVar
	// These will be added when we enhance gwas.go and clinvar.go
}
