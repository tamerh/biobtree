package update

import (
	"biobtree/pbuf"
	"bufio"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// signor parses SIGNOR (SIGnaling Network Open Resource) causal interactions
// Source: https://signor.uniroma2.it/
// Format: TSV with 28 columns including entity info, effect, mechanism, and score
// Supports: Human (9606), Mouse (10090), Rat (10116)
type signor struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (s *signor) check(err error, operation string) {
	checkWithContext(err, s.source, operation)
}

// update processes all SIGNOR TSV files (human, mouse, rat)
func (s *signor) update() {
	defer s.d.wg.Done()

	log.Println("SIGNOR: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(s.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, s.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("SIGNOR: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Get base path and organism-specific file paths
	basePath := config.Dataconf[s.source]["path"]
	pathHuman := config.Dataconf[s.source]["pathHuman"]
	pathMouse := config.Dataconf[s.source]["pathMouse"]
	pathRat := config.Dataconf[s.source]["pathRat"]

	sourceID := config.Dataconf[s.source]["id"]

	// Track total entries across all files
	var totalSaved int
	var testLimitReached bool

	// Process each organism file
	organismFiles := []struct {
		name     string
		filename string
	}{
		{"Human", pathHuman},
		{"Mouse", pathMouse},
		{"Rat", pathRat},
	}

	for _, org := range organismFiles {
		if testLimitReached {
			break
		}

		filePath := filepath.Join(basePath, org.filename)
		log.Printf("SIGNOR: Processing %s data from %s", org.name, filePath)

		saved, limitReached := s.processFile(filePath, sourceID, idLogFile, testLimit-totalSaved)
		totalSaved += saved
		testLimitReached = limitReached

		log.Printf("SIGNOR: Processed %d interactions from %s file", saved, org.name)
	}

	log.Printf("SIGNOR: Saved %d total causal interactions", totalSaved)
	log.Printf("SIGNOR: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Update entry statistics
	atomic.AddUint64(&s.d.totalParsedEntry, uint64(totalSaved))

	// Signal completion
	s.d.progChan <- &progressInfo{dataset: s.source, done: true}
}

// processFile processes a single SIGNOR TSV file
// Returns: number of saved entries and whether test limit was reached
func (s *signor) processFile(filePath, sourceID string, idLogFile *os.File, remainingLimit int) (int, bool) {
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("SIGNOR: Warning - could not open %s: %v", filePath, err)
		return 0, false
	}
	defer file.Close()

	// Create scanner for TSV format
	scanner := bufio.NewScanner(file)

	// Increase buffer size for long lines
	const maxCapacity = 1024 * 1024 // 1MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read and parse header
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			log.Printf("SIGNOR: Error reading header from %s: %v", filePath, err)
		}
		return 0, false
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	// Process entries
	var savedEntries int
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
		elapsed := int64(time.Since(s.d.start).Seconds())
		if elapsed > previous+s.d.progInterval {
			previous = elapsed
			s.d.progChan <- &progressInfo{dataset: s.source, currentKBPerSec: int64(savedEntries / int(elapsed))}
		}

		// Extract SIGNOR_ID (entry key)
		signorID := s.getField(row, colMap, "SIGNOR_ID")
		if signorID == "" {
			continue
		}

		// Extract all fields for attributes
		entityA := s.getField(row, colMap, "ENTITYA")
		typeA := s.getField(row, colMap, "TYPEA")
		idA := s.getField(row, colMap, "IDA")
		databaseA := s.getField(row, colMap, "DATABASEA")
		entityB := s.getField(row, colMap, "ENTITYB")
		typeB := s.getField(row, colMap, "TYPEB")
		idB := s.getField(row, colMap, "IDB")
		databaseB := s.getField(row, colMap, "DATABASEB")
		effect := s.getField(row, colMap, "EFFECT")
		mechanism := s.getField(row, colMap, "MECHANISM")
		residue := s.getField(row, colMap, "RESIDUE")
		sequence := s.getField(row, colMap, "SEQUENCE")
		taxIDStr := s.getField(row, colMap, "TAX_ID")
		cellData := s.getField(row, colMap, "CELL_DATA")
		tissueData := s.getField(row, colMap, "TISSUE_DATA")
		directStr := s.getField(row, colMap, "DIRECT")
		pmidStr := s.getField(row, colMap, "PMID")
		scoreStr := s.getField(row, colMap, "SCORE")

		// Parse tax_id
		taxID := 0
		if taxIDStr != "" {
			if parsed, err := strconv.Atoi(taxIDStr); err == nil {
				taxID = parsed
			}
		}

		// Parse direct flag
		direct := strings.ToUpper(directStr) == "YES"

		// Parse score
		score := 0.0
		if scoreStr != "" {
			if parsed, err := strconv.ParseFloat(scoreStr, 64); err == nil {
				score = parsed
			}
		}

		// Build attribute
		attr := &pbuf.SignorAttr{
			EntityA:    entityA,
			TypeA:      typeA,
			IdA:        idA,
			DatabaseA:  databaseA,
			EntityB:    entityB,
			TypeB:      typeB,
			IdB:        idB,
			DatabaseB:  databaseB,
			Effect:     effect,
			Mechanism:  mechanism,
			Residue:    residue,
			Sequence:   sequence,
			TaxId:      int32(taxID),
			CellData:   cellData,
			TissueData: tissueData,
			Direct:     direct,
			Score:      score,
		}

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		s.check(err, "marshaling SIGNOR attributes")

		s.d.addProp3(signorID, sourceID, attrBytes)

		// Text search indexing - make entity names and SIGNOR ID searchable
		s.d.addXref(entityA, textLinkID, signorID, s.source, true)
		s.d.addXref(entityB, textLinkID, signorID, s.source, true)
		s.d.addXref(signorID, textLinkID, signorID, s.source, true)

		// Cross-reference to external databases based on DATABASEA/DATABASEB
		// Pass taxID and score for sorting (species priority + confidence score)
		s.addDatabaseXref(databaseA, idA, signorID, sourceID, taxID, score)
		s.addDatabaseXref(databaseB, idB, signorID, sourceID, taxID, score)

		// Cross-reference to PubMed
		if pmidStr != "" && pmidStr != "Other" {
			// Handle multiple PMIDs (semicolon-separated)
			for _, pmid := range strings.Split(pmidStr, ";") {
				pmid = strings.TrimSpace(pmid)
				if pmid != "" && isNumericSignor(pmid) {
					s.d.addXref(signorID, sourceID, pmid, "pubmed", false)
				}
			}
		}

		// Cross-reference to taxonomy
		if taxID > 0 {
			s.d.addXref(signorID, sourceID, strconv.Itoa(taxID), "taxonomy", false)
		}

		// Log ID for testing
		if idLogFile != nil {
			logProcessedID(idLogFile, signorID)
		}

		savedEntries++

		// Test mode: check limit
		if remainingLimit > 0 && savedEntries >= remainingLimit {
			log.Printf("SIGNOR: [TEST MODE] Reached limit, stopping")
			return savedEntries, true
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("SIGNOR: Scanner error: %v", err)
	}

	return savedEntries, false
}

// getField retrieves a field value from the row
func (s *signor) getField(row []string, colMap map[string]int, colName string) string {
	if idx, exists := colMap[colName]; exists && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

// addDatabaseXref creates cross-references based on the source database
// taxID and score are used for sorting reverse xrefs (species priority + confidence score)
func (s *signor) addDatabaseXref(database, id, entryID, sourceID string, taxID int, score float64) {
	if id == "" || database == "" {
		return
	}

	// Normalize database name and map to biobtree dataset
	db := strings.ToUpper(database)
	var targetDataset string
	var useSorting bool

	switch db {
	case "UNIPROT":
		targetDataset = "uniprot"
		useSorting = true // Sort by species priority + score
	case "CHEBI":
		// ChEBI IDs should have "CHEBI:" prefix for consistency with ChEBI dataset
		if !strings.HasPrefix(id, "CHEBI:") {
			id = "CHEBI:" + id // Add "CHEBI:" prefix if missing
		}
		targetDataset = "chebi"
		useSorting = true // Sort by score only (no species for chemicals)
	case "PUBCHEM":
		// PubChem IDs may have "CID:" or "SID:" prefix
		if strings.HasPrefix(id, "CID:") {
			id = id[4:] // Remove "CID:" prefix
		} else if strings.HasPrefix(id, "SID:") {
			id = id[4:] // Remove "SID:" prefix (Substance ID)
		}
		targetDataset = "pubchem"
	case "DRUGBANK":
		targetDataset = "drugbank"
	case "RNACENTRAL":
		targetDataset = "rnacentral"
	case "SIGNOR":
		// Internal SIGNOR complexes/phenotypes - skip external xref
		return
	default:
		// Unknown database - skip
		return
	}

	if targetDataset == "" {
		return
	}

	if useSorting {
		// Convert score to int (0-1000 range, score is typically 0-1)
		// Use math.Round to avoid float truncation (e.g., 0.986 * 1000 = 985.999 → 985)
		scoreInt := int(math.Round(score * 1000))
		if scoreInt > 1000 {
			scoreInt = 1000
		}

		var sortLevels []string
		if targetDataset == "uniprot" {
			// UniProt: species priority + score
			taxIDStr := strconv.Itoa(taxID)
			sortLevels = []string{
				ComputeSortLevelValue(SortLevelSpeciesPriority, map[string]interface{}{"taxID": taxIDStr}),
				ComputeSortLevelValue(SortLevelInteractionScore, map[string]interface{}{"score": scoreInt}),
			}
		} else {
			// ChEBI: score only (no species for chemicals)
			sortLevels = []string{
				ComputeSortLevelValue(SortLevelInteractionScore, map[string]interface{}{"score": scoreInt}),
			}
		}
		s.d.addXrefWithSortLevels(entryID, sourceID, id, targetDataset, sortLevels)
	} else {
		s.d.addXref(entryID, sourceID, id, targetDataset, false)
	}
}

// isNumericSignor checks if a string contains only digits
func isNumericSignor(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
