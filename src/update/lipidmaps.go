package update

import (
	"archive/zip"
	"biobtree/pbuf"
	"bufio"
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type lipidmaps struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for lipidmaps processor
func (l *lipidmaps) check(err error, operation string) {
	checkWithContext(err, l.source, operation)
}

func (l *lipidmaps) update() {
	defer l.d.wg.Done()

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(l.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, l.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Get download URL
	var zipBytes []byte
	var err error

	if config.IsTestMode() && config.Dataconf[l.source]["path2"] != "" {
		// Use local test file in test mode
		testFilePath := config.Dataconf[l.source]["path2"]
		zipBytes, err = os.ReadFile(testFilePath)
		l.check(err, "reading local LIPID MAPS test file: "+testFilePath)
		log.Printf("[%s] Using local test file: %s", l.source, testFilePath)
	} else {
		// Download from remote URL
		lipidmapsURL := config.Dataconf[l.source]["path"]
		log.Printf("[%s] Downloading LIPID MAPS SDF from: %s", l.source, lipidmapsURL)

		resp, err := http.Get(lipidmapsURL)
		l.check(err, "downloading LIPID MAPS ZIP file from: "+lipidmapsURL)
		defer resp.Body.Close()

		// Validate HTTP response
		if resp.StatusCode != 200 {
			log.Fatalf("[%s] Error: LIPID MAPS server returned HTTP %s (expected 200 OK) from: %s",
				l.source, resp.Status, lipidmapsURL)
		}

		// Read ZIP file into memory (only ~20MB, acceptable)
		zipBytes, err = io.ReadAll(resp.Body)
		l.check(err, "reading LIPID MAPS ZIP file content")
		log.Printf("[%s] Downloaded %d bytes", l.source, len(zipBytes))
	}

	// Open ZIP archive
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	l.check(err, "opening LIPID MAPS ZIP archive")

	// Find and parse SDF file inside ZIP
	var sdfFound bool
	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, ".sdf") {
			sdfFound = true
			log.Printf("[%s] Found SDF file in ZIP: %s", l.source, file.Name)

			sdfFile, err := file.Open()
			l.check(err, "opening SDF file: "+file.Name)
			defer sdfFile.Close()

			// Parse SDF file
			l.parseSDF(sdfFile, testLimit, idLogFile)
			break
		}
	}

	if !sdfFound {
		log.Fatalf("[%s] Error: No .sdf file found in ZIP archive", l.source)
	}

	log.Printf("[%s] LIPID MAPS processing completed successfully", l.source)
}

// parseSDF parses the SDF format and processes lipid records
func (l *lipidmaps) parseSDF(reader io.Reader, testLimit int, idLogFile *os.File) {
	scanner := bufio.NewScanner(reader)
	// Increase buffer size for large records
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var currentRecord strings.Builder
	var entryCount int64
	var previous int64
	startTime := time.Now()

	for scanner.Scan() {
		line := scanner.Text()

		// Record delimiter: $$$$ marks end of a record
		if line == "$$$$" {
			// Process complete record
			record := currentRecord.String()
			if len(record) > 0 {
				dataItems := l.extractDataItems(record)
				l.processRecord(dataItems, &entryCount, idLogFile)

				// Progress reporting (simplified - just track time)
				elapsed := int64(time.Since(startTime).Seconds())
				if elapsed > previous+l.d.progInterval {
					previous = elapsed
					// Note: currentKBPerSec not available for SDF parsing
					l.d.progChan <- &progressInfo{
						dataset: l.source,
					}
				}

				// Check test limit
				if config.IsTestMode() && testLimit > 0 && entryCount >= int64(testLimit) {
					log.Printf("[%s] Test mode: reached limit of %d entries", l.source, testLimit)
					break
				}
			}

			// Reset for next record
			currentRecord.Reset()
			continue
		}

		// Accumulate record lines
		currentRecord.WriteString(line + "\n")
	}

	l.check(scanner.Err(), "scanning SDF file")
	log.Printf("[%s] Processed %d lipid entries", l.source, entryCount)
}

// extractDataItems extracts key-value pairs from SDF data items
// SDF format: > <FIELD_NAME> followed by value on next lines
func (l *lipidmaps) extractDataItems(record string) map[string]string {
	dataItems := make(map[string]string)

	lines := strings.Split(record, "\n")
	var currentField string
	var currentValue strings.Builder
	inMolBlock := true

	for i, line := range lines {
		// Record delimiter - stop parsing
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "$$$$" {
			break
		}

		// End of MOL block (starts at "M  END")
		if line == "M  END" {
			inMolBlock = false
			continue
		}

		// Skip MOL block lines
		if inMolBlock {
			continue
		}

		// Field name line: > <FIELD_NAME>
		if strings.HasPrefix(line, "> <") && strings.HasSuffix(line, ">") {
			// Save previous field if exists
			if currentField != "" {
				dataItems[currentField] = strings.TrimSpace(currentValue.String())
			}

			// Extract new field name
			// Format: "> <FIELD_NAME>"
			fieldLine := strings.TrimPrefix(line, "> <")
			fieldLine = strings.TrimSuffix(fieldLine, ">")
			currentField = fieldLine
			currentValue.Reset()

			// Next line contains the value (might be empty)
			if i+1 < len(lines) {
				// Value starts on next line
			}
		} else if currentField != "" && trimmedLine != "" && !strings.HasPrefix(line, "> <") {
			// Value line for current field
			if currentValue.Len() > 0 {
				currentValue.WriteString("; ") // Multi-line values separated by semicolon
			}
			currentValue.WriteString(trimmedLine)
		}
	}

	// Save last field
	if currentField != "" {
		dataItems[currentField] = strings.TrimSpace(currentValue.String())
	}

	return dataItems
}

// processRecord processes a single lipid record
func (l *lipidmaps) processRecord(dataItems map[string]string, entryCount *int64, idLogFile *os.File) {
	lmID := dataItems["LM_ID"]
	if lmID == "" {
		// Skip records without LM_ID
		return
	}

	atomic.AddInt64(entryCount, 1)

	// Debug: Log field names for first entry to verify SDF structure
	if *entryCount == 1 {
		log.Printf("[%s] DEBUG: First entry fields:", l.source)
		for fieldName := range dataItems {
			log.Printf("[%s]   - %s", l.source, fieldName)
		}
	}

	// Log ID in test mode
	if idLogFile != nil {
		logProcessedID(idLogFile, lmID)
	}

	// Create attribute object
	// Note: Try both "NAME" and "COMMON_NAME" as SDF might use either
	commonName := dataItems["COMMON_NAME"]
	if commonName == "" {
		commonName = dataItems["NAME"]
	}

	attr := pbuf.LipidmapsAttr{
		Name:           commonName,
		SystematicName: dataItems["SYSTEMATIC_NAME"],
		Abbreviation:   dataItems["ABBREVIATION"],
		Category:       dataItems["CATEGORY"],
		MainClass:      dataItems["MAIN_CLASS"],
		SubClass:       dataItems["SUB_CLASS"],
		ClassLevel4:    dataItems["CLASS_LEVEL4"],
		ExactMass:      dataItems["EXACT_MASS"],
		Formula:        dataItems["FORMULA"],
		Inchi:          dataItems["INCHI"],
		InchiKey:       dataItems["INCHI_KEY"],
		Smiles:         dataItems["SMILES"],
	}

	// Parse synonyms (semicolon-delimited in SDF)
	if synonyms := dataItems["SYNONYMS"]; synonyms != "" {
		// Synonyms can be delimited by "; " or "; "
		synList := strings.Split(synonyms, "; ")
		for _, syn := range synList {
			trimmedSyn := strings.TrimSpace(syn)
			if trimmedSyn != "" {
				attr.Synonyms = append(attr.Synonyms, trimmedSyn)
			}
		}
	}

	// Marshal attributes
	b, err := ffjson.Marshal(&attr)
	l.check(err, "marshaling attributes for "+lmID)

	// Get dataset ID string from config
	var fr = config.Dataconf[l.source]["id"]

	// Save entry to database
	// IMPORTANT: Second parameter is dataset ID string from config
	l.d.addProp3(lmID, fr, b)

	// Add text search terms
	// textLinkID is a constant "0" defined in update.go
	// Note: Do NOT add LM_ID to itself - that creates duplicates
	// The LM_ID is already searchable via the main entry

	// Add common name as searchable
	if attr.Name != "" {
		l.d.addXref(attr.Name, textLinkID, lmID, l.source, true)
	}

	// Add systematic name as searchable
	if attr.SystematicName != "" {
		l.d.addXref(attr.SystematicName, textLinkID, lmID, l.source, true)
	}

	// Add abbreviation as searchable
	if attr.Abbreviation != "" {
		l.d.addXref(attr.Abbreviation, textLinkID, lmID, l.source, true)
	}

	// Add synonyms as searchable
	for _, syn := range attr.Synonyms {
		if syn != "" {
			l.d.addXref(syn, textLinkID, lmID, l.source, true)
		}
	}

	// Add InChI key as searchable (useful for structure search)
	if attr.InchiKey != "" {
		l.d.addXref(attr.InchiKey, textLinkID, lmID, l.source, true)
	}

	// Create cross-references to other databases
	// IMPORTANT: Parameters are (fromID, fromDatasetID, toID, toDatasetName, isTextSearch)
	// Second parameter must be dataset ID string from config, fourth parameter must be dataset name (string)

	// ChEBI cross-reference
	if chebiID := dataItems["CHEBI_ID"]; chebiID != "" {
		// Ensure ChEBI ID has proper prefix
		if !strings.HasPrefix(chebiID, "CHEBI:") && !strings.HasPrefix(chebiID, "chebi:") {
			chebiID = "CHEBI:" + chebiID
		}
		l.d.addXref(lmID, fr, chebiID, "chebi", false)
	}

	// HMDB cross-reference
	if hmdbID := dataItems["HMDB_ID"]; hmdbID != "" {
		// Validate HMDB ID format (should start with HMDB followed by digits)
		if !strings.HasPrefix(hmdbID, "HMDB") {
			log.Printf("[%s] Non-standard HMDB_ID: '%s' for lipid: %s", l.source, hmdbID, lmID)
		} else {
			l.d.addXref(lmID, fr, hmdbID, "hmdb", false)
		}
	}

	// KEGG cross-reference
	if keggID := dataItems["KEGG_ID"]; keggID != "" {
		l.d.addXref(lmID, fr, keggID, "kegg", false)
	}

	// PubChem cross-reference
	if pubchemID := dataItems["PUBCHEM_CID"]; pubchemID != "" {
		l.d.addXref(lmID, fr, pubchemID, "pubchem", false)
	}

	// SwissLipids cross-reference (for future SwissLipids integration)
	if swissID := dataItems["SWISSLIPIDS_ID"]; swissID != "" {
		l.d.addXref(lmID, fr, swissID, "swisslipids", false)
	}

	// LipidBank cross-reference
	if lipidBankID := dataItems["LIPIDBANK_ID"]; lipidBankID != "" {
		l.d.addXref(lmID, fr, lipidBankID, "lipidbank", false)
	}

	// PlantFA cross-reference
	if plantFAID := dataItems["PLANTFA_ID"]; plantFAID != "" {
		l.d.addXref(lmID, fr, plantFAID, "plantfa", false)
	}
}
