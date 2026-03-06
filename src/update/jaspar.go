package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// jaspar parses JASPAR transcription factor binding profile data
// Source: https://jaspar.elixir.no/
// Format: TSV metadata files (CORE and UNVALIDATED collections)
type jaspar struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (j *jaspar) check(err error, operation string) {
	checkWithContext(err, j.source, operation)
}

// update processes the JASPAR TSV metadata files
func (j *jaspar) update() {
	defer j.d.wg.Done()

	log.Println("JASPAR: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(j.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, j.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("JASPAR: [TEST MODE] Processing up to %d entries", testLimit)
	}

	sourceID := config.Dataconf[j.source]["id"]

	var totalSaved int
	var previous int64

	// Process CORE collection
	corePath := config.Dataconf[j.source]["path"]
	log.Printf("JASPAR: Processing CORE collection from %s", corePath)
	coreSaved := j.processCollection(corePath, "CORE", sourceID, textLinkID, testLimit, idLogFile, &previous)
	totalSaved += coreSaved
	log.Printf("JASPAR: Saved %d CORE entries", coreSaved)

	// Process UNVALIDATED collection (if not in test mode limit reached)
	if testLimit == 0 || totalSaved < testLimit {
		unvalidatedPath := config.Dataconf[j.source]["pathUnvalidated"]
		if unvalidatedPath != "" {
			log.Printf("JASPAR: Processing UNVALIDATED collection from %s", unvalidatedPath)
			remainingLimit := 0
			if testLimit > 0 {
				remainingLimit = testLimit - totalSaved
			}
			unvalidatedSaved := j.processCollection(unvalidatedPath, "UNVALIDATED", sourceID, textLinkID, remainingLimit, idLogFile, &previous)
			totalSaved += unvalidatedSaved
			log.Printf("JASPAR: Saved %d UNVALIDATED entries", unvalidatedSaved)
		}
	}

	log.Printf("JASPAR: Total saved %d TF binding profiles", totalSaved)
	log.Printf("JASPAR: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Update entry statistics
	atomic.AddUint64(&j.d.totalParsedEntry, uint64(totalSaved))

	// Signal completion
	j.d.progChan <- &progressInfo{dataset: j.source, done: true}
}

// processCollection processes a single JASPAR TSV collection file
func (j *jaspar) processCollection(filePath, collection string, sourceID, textLinkID string, testLimit int, idLogFile *os.File, previous *int64) int {
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(j.source, "", "", filePath)
	if err != nil {
		log.Printf("JASPAR: Error opening %s file: %v", collection, err)
		return 0
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	scanner := bufio.NewScanner(br)

	// Increase buffer size for long lines
	const maxCapacity = 1024 * 1024 // 1MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Skip header line
	if !scanner.Scan() {
		j.check(scanner.Err(), "reading JASPAR header")
		return 0
	}

	var savedEntries int

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Progress tracking
		elapsed := int64(time.Since(j.d.start).Seconds())
		if elapsed > *previous+j.d.progInterval {
			*previous = elapsed
			j.d.progChan <- &progressInfo{dataset: j.source, currentKBPerSec: int64(savedEntries)}
		}

		// Split by tab
		// TSV columns: collection, tax_group, matrix_id, base_id, version, name, class, family, uniprot_ids, validation, comment, source, type, tax_id, species
		row := strings.Split(line, "\t")
		if len(row) < 15 {
			continue
		}

		// Extract fields (0-indexed)
		matrixID := strings.TrimSpace(row[2])
		if matrixID == "" {
			continue
		}

		versionStr := strings.TrimSpace(row[4])
		version := 0
		if v, err := strconv.Atoi(versionStr); err == nil {
			version = v
		}

		name := strings.TrimSpace(row[5])
		class := strings.TrimSpace(row[6])
		family := strings.TrimSpace(row[7])
		uniprotIDs := strings.TrimSpace(row[8])
		pubmedID := strings.TrimSpace(row[9])   // validation column = PubMed ID
		expType := strings.TrimSpace(row[12])   // type column
		taxID := strings.TrimSpace(row[13])
		species := strings.TrimSpace(row[14])
		taxGroup := strings.TrimSpace(row[1])

		// Build attribute
		attr := &pbuf.JasparAttr{
			MatrixId:   matrixID,
			Name:       name,
			Collection: collection,
			Class:      class,
			Family:     family,
			TaxGroup:   taxGroup,
			Type:       expType,
			Species:    species,
			Version:    int32(version),
		}

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		j.check(err, "marshaling JASPAR attributes")

		j.d.addProp3(matrixID, sourceID, attrBytes)

		// Text search indexing
		j.d.addXref(matrixID, textLinkID, matrixID, j.source, true)

		// Index TF name for text search (e.g., "RUNX1", "Arnt")
		if name != "" {
			j.d.addXref(name, textLinkID, matrixID, j.source, true)

			// Handle heterodimers like "MAX::MYC" or "Ahr::Arnt"
			if strings.Contains(name, "::") {
				parts := strings.Split(name, "::")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part != "" && part != name {
						j.d.addXref(part, textLinkID, matrixID, j.source, true)
					}
				}
			}
		}

		// Cross-reference to UniProt (may have multiple IDs like "P30561::P53762")
		if uniprotIDs != "" {
			ids := strings.Split(uniprotIDs, "::")
			for _, uid := range ids {
				uid = strings.TrimSpace(uid)
				if uid != "" {
					j.d.addXref(matrixID, sourceID, uid, "uniprot", false)
				}
			}
		}

		// Cross-reference to PubMed (validation column contains PubMed ID)
		if pubmedID != "" && isNumeric(pubmedID) {
			j.d.addXref(matrixID, sourceID, pubmedID, "pubmed", false)
		}

		// Cross-reference to Taxonomy
		if taxID != "" && isNumeric(taxID) {
			j.d.addXref(matrixID, sourceID, taxID, "taxonomy", false)
		}

		// Log ID for testing
		if idLogFile != nil {
			logProcessedID(idLogFile, matrixID)
		}

		savedEntries++

		// Test mode: check limit
		if testLimit > 0 && savedEntries >= testLimit {
			log.Printf("JASPAR: [TEST MODE] Reached limit of %d entries for %s, stopping", testLimit, collection)
			break
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("JASPAR: Scanner error for %s: %v", collection, err)
	}

	return savedEntries
}
