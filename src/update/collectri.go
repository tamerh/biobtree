package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// collectri parses CollecTRI transcription factor-target gene interactions
// Source: https://zenodo.org/record/7773985
// Format: TSV with multiple evidence source columns
// Note: TRED source is excluded for licensing reasons
type collectri struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (c *collectri) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// update processes the CollecTRI TSV file
func (c *collectri) update() {
	defer c.d.wg.Done()

	log.Println("CollecTRI: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("CollecTRI: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Get data file
	filePath := config.Dataconf[c.source]["path"]
	log.Printf("CollecTRI: Downloading from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(c.source, "", "", filePath)
	c.check(err, "opening CollecTRI TSV file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[c.source]["id"]

	// Create scanner for TSV format
	scanner := bufio.NewScanner(br)

	// Increase buffer size for long lines
	const maxCapacity = 1024 * 1024 // 1MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read and parse header
	if !scanner.Scan() {
		c.check(scanner.Err(), "reading CollecTRI header")
		return
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	log.Printf("CollecTRI: Found %d columns in header", len(colMap))

	// Process entries
	var savedEntries int
	var skippedTRED int
	var previous int64
	lineNum := 1

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Split by tab
		row := strings.Split(line, "\t")

		// Progress tracking
		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > previous+c.d.progInterval {
			previous = elapsed
			c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: int64(savedEntries / int(elapsed))}
		}

		// Extract key fields
		tfTarget := getField(row, colMap, "TF:TG")
		if tfTarget == "" {
			continue
		}

		// Parse TF and target from the ID
		parts := strings.SplitN(tfTarget, ":", 2)
		if len(parts) != 2 {
			continue
		}
		tfGene := strings.TrimSpace(parts[0])
		targetGene := strings.TrimSpace(parts[1])

		if tfGene == "" || targetGene == "" {
			continue
		}

		// Collect sources, PMIDs, regulation, and confidence
		sources, pmids, regulation, confidence, hasTRED := c.collectEvidence(row, colMap)

		// Skip entries that ONLY have TRED as source
		if hasTRED && len(sources) == 0 {
			skippedTRED++
			continue
		}

		// Build attribute (pmids stored as xrefs, not in attributes)
		attr := &pbuf.CollecTriAttr{
			TfGene:     tfGene,
			TargetGene: targetGene,
			Sources:    sources,
			Regulation: regulation,
			Confidence: confidence,
		}

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		c.check(err, "marshaling CollecTRI attributes")

		// Use TF:TG as entry ID
		entryID := tfTarget
		c.d.addProp3(entryID, sourceID, attrBytes)

		// Text search indexing - make TF and target searchable
		c.d.addXref(tfGene, textLinkID, entryID, c.source, true)
		c.d.addXref(targetGene, textLinkID, entryID, c.source, true)
		c.d.addXref(entryID, textLinkID, entryID, c.source, true)

		// Cross-reference to HGNC and Ensembl (human genes only)
		// CollecTRI is a human TF-target database, so we go through HGNC
		// to ensure we only get human genes (not mouse/rat/zebrafish)
		// addHumanGeneXrefs creates xref to HGNC (Ensembl via HGNC→Ensembl)
		c.d.addHumanGeneXrefs(tfGene, entryID, sourceID)
		c.d.addHumanGeneXrefs(targetGene, entryID, sourceID)

		// Cross-reference to PubMed for literature citations
		for _, pmid := range pmids {
			c.d.addXref(entryID, sourceID, pmid, "pubmed", false)
		}

		// Log ID for testing
		if idLogFile != nil {
			logProcessedID(idLogFile, entryID)
		}

		savedEntries++

		// Test mode: check limit
		if testLimit > 0 && savedEntries >= testLimit {
			log.Printf("CollecTRI: [TEST MODE] Reached limit of %d entries, stopping", testLimit)
			break
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("CollecTRI: Scanner error: %v", err)
	}

	log.Printf("CollecTRI: Saved %d TF-target interactions", savedEntries)
	if skippedTRED > 0 {
		log.Printf("CollecTRI: Skipped %d entries with TRED-only evidence (excluded for licensing)", skippedTRED)
	}
	log.Printf("CollecTRI: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Update entry statistics
	atomic.AddUint64(&c.d.totalParsedEntry, uint64(savedEntries))

	// Signal completion
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}

// collectEvidence extracts sources, PMIDs, regulation, and confidence from the row
// Returns: sources (excluding TRED), pmids, regulation direction, confidence, and whether TRED was present
func (c *collectri) collectEvidence(row []string, colMap map[string]int) ([]string, []string, string, string, bool) {
	sourcesMap := make(map[string]bool)
	pmidsMap := make(map[string]bool)
	var regulation string
	var confidence string
	hasTRED := false

	// Source column mappings (present column, PMID column, optional regulation column, optional confidence column)
	sourceColumns := []struct {
		presentCol     string
		pmidCol        string
		regulationCol  string
		confidenceCol  string
		sourceNameCol  string // For TFactS which has "Source" column containing database names
	}{
		{"[ExTRI] present", "[ExTRI] PMID", "", "[ExTRI] Confidence", ""},
		{"[HTRI] present", "[HTRI] PMID", "", "[HTRI] Confidence", ""},
		{"[TRRUST] present", "[TRRUST] PMID", "[TRRUST] Regulation", "", ""},
		{"[TFactS] present", "[TFactS] PMID", "[TFactS] Sign", "[TFactS] Confidence", "[TFactS] Source"},
		{"[GOA] present", "[GOA] PMID", "[GOA] Sign", "", ""},
		{"[IntAct] present", "[IntAct] PMID", "", "", ""},
		{"[SIGNOR] present", "[SIGNOR] PMID", "[SIGNOR] Sign", "", ""},
		{"[CytReg] present", "[CytReg] PMID", "[CytReg] Activation/Repression", "", ""},
		{"[GEREDB] present", "[GEREDB] PMID", "[GEREDB] Effect", "", ""},
		{"[NTNU Curated] present", "[NTNU Curated] PMID", "[NTNU Curated] Sign", "", ""},
		{"[Pavlidis2021] present", "[Pavlidis2021] PMID", "[Pavlidis2021] Mode of action", "", ""},
		{"[DoRothEA_A] present", "[DoRothEA_A] PMID", "[DoRothEA_A] Effect", "", ""},
	}

	for _, sc := range sourceColumns {
		presentVal := getField(row, colMap, sc.presentCol)
		if presentVal == "" || strings.ToLower(presentVal) == "false" {
			continue
		}

		// Extract source name from column header
		sourceName := extractSourceName(sc.presentCol)

		// Check for TFactS Source column which may contain TRED
		if sc.sourceNameCol != "" {
			tfactsSource := getField(row, colMap, sc.sourceNameCol)
			if strings.Contains(tfactsSource, "TRED") {
				hasTRED = true
				// Don't add TFactS if it only has TRED
				// Check if there are other sources in TFactS
				otherSources := false
				for _, s := range strings.Split(tfactsSource, ";") {
					s = strings.TrimSpace(s)
					if s != "" && !strings.Contains(s, "TRED") {
						otherSources = true
						break
					}
				}
				if !otherSources {
					continue // Skip this TFactS entry
				}
			}
		}

		sourcesMap[sourceName] = true

		// Collect PMIDs (only numeric - some sources have non-PMID IDs like TRRD records)
		pmidVal := getField(row, colMap, sc.pmidCol)
		if pmidVal != "" {
			// PMIDs may be separated by semicolons or in quotes
			pmidVal = strings.Trim(pmidVal, "\"")
			for _, pmid := range strings.Split(pmidVal, ";") {
				pmid = strings.TrimSpace(pmid)
				// Only accept numeric PMIDs (filter out TRRD records like "S5269")
				if pmid != "" && pmid != "|" && isNumeric(pmid) {
					pmidsMap[pmid] = true
				}
			}
		}

		// Collect regulation if not already set
		if regulation == "" && sc.regulationCol != "" {
			regVal := getField(row, colMap, sc.regulationCol)
			if regVal != "" {
				regulation = normalizeRegulation(regVal)
			}
		}

		// Collect confidence if not already set
		if confidence == "" && sc.confidenceCol != "" {
			confVal := getField(row, colMap, sc.confidenceCol)
			if confVal != "" {
				confidence = confVal
			}
		}
	}

	// Convert maps to slices
	sources := make([]string, 0, len(sourcesMap))
	for s := range sourcesMap {
		sources = append(sources, s)
	}

	pmids := make([]string, 0, len(pmidsMap))
	for p := range pmidsMap {
		pmids = append(pmids, p)
	}

	return sources, pmids, regulation, confidence, hasTRED
}

// extractSourceName extracts the source name from a column header like "[ExTRI] present"
func extractSourceName(colHeader string) string {
	// Extract text between [ and ]
	start := strings.Index(colHeader, "[")
	end := strings.Index(colHeader, "]")
	if start >= 0 && end > start {
		return colHeader[start+1 : end]
	}
	return colHeader
}

// normalizeRegulation converts various regulation terms to a standard format
func normalizeRegulation(reg string) string {
	reg = strings.ToLower(strings.TrimSpace(reg))
	switch {
	case strings.Contains(reg, "activ") || strings.Contains(reg, "positive") || reg == "up" || reg == "+":
		return "Activation"
	case strings.Contains(reg, "repress") || strings.Contains(reg, "negative") || strings.Contains(reg, "inhibit") || reg == "down" || reg == "-":
		return "Repression"
	default:
		return "Unknown"
	}
}
