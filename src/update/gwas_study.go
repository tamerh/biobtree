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

type gwasStudy struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (g *gwasStudy) check(err error, operation string) {
	checkWithContext(err, g.source, operation)
}

// Main update entry point
func (g *gwasStudy) update() {
	defer g.d.wg.Done()

	log.Println("GWAS Catalog: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(g.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, g.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("GWAS Catalog: [TEST MODE] Processing up to %d studies", testLimit)
	}

	// Process GWAS Catalog studies TSV file
	g.parseAndSaveStudies(testLimit, idLogFile)

	log.Printf("GWAS Catalog: Processing complete (%.2fs)", time.Since(startTime).Seconds())
}

// parseAndSaveStudies processes the GWAS Catalog studies TSV file
func (g *gwasStudy) parseAndSaveStudies(testLimit int, idLogFile *os.File) {
	// Build file path
	filePath := config.Dataconf[g.source]["path"]
	log.Printf("GWAS Catalog: Downloading via EBI FTP from %s", filePath)

	// Open file via EBI FTP (like uniprot does)
	// Path in config is now a full FTP URL
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(g.source, "", "", filePath)
	g.check(err, "opening GWAS Catalog TSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[g.source]["id"]

	// Create scanner for TSV format (like rhea does)
	scanner := bufio.NewScanner(br)

	// Read and parse header
	if !scanner.Scan() {
		g.check(scanner.Err(), "reading GWAS Catalog header")
		return
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	var processedCount int
	var previous int64
	var skippedEmptyAccession int
	var skippedNonGCST int
	var totalMappedTraits int
	var maxMappedTraitsInStudy int
	var totalRowsRead int
	var bytesRead int64

	// Process each row (one study per row) - using Scanner like rhea does
	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		totalRowsRead++

		// Split by tab
		row := strings.Split(line, "\t")

		// Estimate bytes read (approximate)
		bytesRead += int64(len(line) + 1) // +1 for newline

		// Validate field count (should be 25)
		if len(row) != 25 {
			log.Printf("GWAS Catalog: Skipping line %d with %d fields (expected 25)", lineNum, len(row))
			continue
		}

		// Log progress every 1000 rows
		if totalRowsRead%1000 == 0 {
			log.Printf("GWAS Catalog: Read %d rows so far (line %d)...", totalRowsRead, lineNum)
		}

		// Progress tracking
		elapsed := int64(time.Since(g.d.start).Seconds())
		if elapsed > previous+g.d.progInterval {
			previous = elapsed
			g.d.progChan <- &progressInfo{dataset: g.source, currentKBPerSec: int64(processedCount / int(elapsed))}
		}

		// Extract study accession (primary key)
		studyAccession := getField(row, colMap, "STUDY ACCESSION")
		if studyAccession == "" {
			skippedEmptyAccession++
			continue
		}

		// Skip non-GCST study accessions (data quality issues in source file)
		if !strings.HasPrefix(studyAccession, "GCST") {
			skippedNonGCST++
			continue
		}

		// Track mapped traits statistics
		mappedTraitsStr := getField(row, colMap, "MAPPED_TRAIT")
		if mappedTraitsStr != "" {
			traits := splitAndClean(mappedTraitsStr, ",")
			totalMappedTraits += len(traits)
			if len(traits) > maxMappedTraitsInStudy {
				maxMappedTraitsInStudy = len(traits)
			}
		}

		// Build and save study
		g.saveStudy(row, colMap, studyAccession, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(studyAccession + "\n")
		}

		processedCount++

		// Test mode: check limit
		if testLimit > 0 && processedCount >= testLimit {
			log.Printf("GWAS Catalog: [TEST MODE] Reached limit of %d studies, stopping", testLimit)
			break
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("GWAS Catalog: Scanner error: %v", err)
	}

	log.Printf("GWAS Catalog: Total rows read: %d, Saved: %d studies", totalRowsRead, processedCount)
	log.Printf("GWAS Catalog: Skipped - Empty accession: %d, Non-GCST accession: %d", skippedEmptyAccession, skippedNonGCST)
	if processedCount > 0 {
		avgTraits := float64(totalMappedTraits) / float64(processedCount)
		log.Printf("GWAS Catalog: Mapped traits - Total: %d, Avg per study: %.1f, Max in one study: %d",
			totalMappedTraits, avgTraits, maxMappedTraitsInStudy)
	}

	// Update entry statistics
	atomic.AddUint64(&g.d.totalParsedEntry, uint64(processedCount))
}

// saveStudy creates and saves a single GWAS study entry
func (g *gwasStudy) saveStudy(row []string, colMap map[string]int, studyAccession, sourceID string) {
	// Extract trait URIs and parse EFO IDs
	traitURIs := getField(row, colMap, "MAPPED_TRAIT_URI")
	var efoIDs []string
	if traitURIs != "" {
		for _, uri := range splitAndClean(traitURIs, ",") {
			efoID := extractEFOID(uri)
			if efoID != "" {
				efoIDs = append(efoIDs, efoID)
			}
		}
	}

	// Extract mapped traits
	mappedTraits := splitAndClean(getField(row, colMap, "MAPPED_TRAIT"), ",")

	// Parse association count
	var assocCount int32
	if countStr := getField(row, colMap, "ASSOCIATION COUNT"); countStr != "" {
		if count, err := strconv.Atoi(countStr); err == nil {
			assocCount = int32(count)
		}
	}

	// Build attribute object
	attr := &pbuf.GwasStudyAttr{
		StudyAccession:        studyAccession,
		PubmedId:              getField(row, colMap, "PUBMED ID"),
		FirstAuthor:           getField(row, colMap, "FIRST AUTHOR"),
		PublicationDate:       getField(row, colMap, "DATE"),
		Journal:               getField(row, colMap, "JOURNAL"),
		Study:                 getField(row, colMap, "STUDY"),
		DiseaseTrait:          getField(row, colMap, "DISEASE/TRAIT"),
		InitialSampleSize:     getField(row, colMap, "INITIAL SAMPLE SIZE"),
		ReplicationSampleSize: getField(row, colMap, "REPLICATION SAMPLE SIZE"),
		Platform:              getField(row, colMap, "PLATFORM [SNPS PASSING QC]"),
		AssociationCount:      assocCount,
		TraitEfos:             efoIDs,
		ReportedGenes:         []string{}, // Not available in studies format
		MappedGenes:           []string{}, // Not available in studies format
		TopAssociations:       []*pbuf.GwasAssociation{}, // Not available in studies format
	}

	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	g.check(err, fmt.Sprintf("marshaling GWAS attributes for %s", studyAccession))

	// Save entry
	g.d.addProp3(studyAccession, sourceID, attrBytes)

	// Create cross-references
	g.createCrossReferences(studyAccession, sourceID, attr, mappedTraits)
}

// createCrossReferences builds all cross-references for a GWAS study
func (g *gwasStudy) createCrossReferences(studyAccession, sourceID string, attr *pbuf.GwasStudyAttr, mappedTraits []string) {
	// Text search: study accession
	g.d.addXref(studyAccession, textLinkID, studyAccession, g.source, true)

	// Text search: disease/trait (only if not too long to avoid buffer overflow)
	if attr.DiseaseTrait != "" && len(attr.DiseaseTrait) < 200 {
		g.d.addXref(attr.DiseaseTrait, textLinkID, studyAccession, g.source, true)
	}

	// Text search: mapped traits (limit to first 5 to avoid too many xrefs)
	maxTraits := 5
	for i, trait := range mappedTraits {
		if i >= maxTraits {
			break
		}
		if trait != "" && len(trait) < 150 {
			g.d.addXref(trait, textLinkID, studyAccession, g.source, true)
		}
	}

	// Cross-reference: PubMed (as text link)
	if attr.PubmedId != "" {
		g.d.addXref("PMID:"+attr.PubmedId, textLinkID, studyAccession, g.source, true)
	}

	// Cross-reference: ontology traits (EFO, MONDO, HP, OBA, etc.)
	// GWAS Catalog includes traits from multiple ontologies, route each to correct dataset
	for _, traitID := range attr.TraitEfos {
		if traitID == "" {
			continue
		}
		// Route to correct dataset based on prefix
		targetDataset := getOntologyDataset(traitID)
		if targetDataset != "" {
			if _, exists := config.Dataconf[targetDataset]; exists {
				g.d.addXref(studyAccession, sourceID, traitID, targetDataset, false)
			}
		}
	}
}

// Helper: get field from row by column name
func getField(row []string, colMap map[string]int, colName string) string {
	if idx, exists := colMap[colName]; exists && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

// Helper: get first N characters from row at index
func getFirstN(row []string, idx int, n int) string {
	if idx >= len(row) {
		return "<missing>"
	}
	val := row[idx]
	if len(val) > n {
		return val[:n] + "..."
	}
	return val
}

// Helper: split and clean string by multiple delimiters
func splitAndClean(s string, delimiters string) []string {
	if s == "" {
		return []string{}
	}

	// Replace all delimiters with a single delimiter
	for _, delim := range delimiters {
		s = strings.ReplaceAll(s, string(delim), ",")
	}

	parts := strings.Split(s, ",")
	var cleaned []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

// Helper: extract ontology ID from URI
// Only extracts valid ontology IDs (PREFIX_NNNNN format)
func extractEFOID(uri string) string {
	// http://www.ebi.ac.uk/efo/EFO_0000400 -> EFO:0000400
	// http://purl.obolibrary.org/obo/MONDO_0005148 -> MONDO:0005148
	// Skip malformed URLs like http://...oc/exp.php?expert=37553
	parts := strings.Split(uri, "/")
	if len(parts) == 0 {
		return ""
	}
	lastPart := parts[len(parts)-1]

	// Must contain underscore to be a valid ontology ID (PREFIX_NNNNN)
	if !strings.Contains(lastPart, "_") {
		return ""
	}

	// Skip URLs with query parameters or file extensions
	if strings.Contains(lastPart, "?") || strings.Contains(lastPart, ".") {
		return ""
	}

	// Convert underscore to colon
	id := strings.ReplaceAll(lastPart, "_", ":")

	// Validate format: should start with letters followed by colon
	colonIdx := strings.Index(id, ":")
	if colonIdx <= 0 || colonIdx >= len(id)-1 {
		return ""
	}

	// Prefix should be all uppercase letters
	prefix := id[:colonIdx]
	for _, c := range prefix {
		if c < 'A' || c > 'Z' {
			return ""
		}
	}

	return id
}
