package update

import (
	"biobtree/pbuf"
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type swisslipids struct {
	source      string
	d           *DataUpdate
	testLipidIDs map[string]bool // Track processed lipid IDs in test mode
}

// check provides context-aware error checking for swisslipids processor
func (s *swisslipids) check(err error, operation string) {
	checkWithContext(err, s.source, operation)
}

func (s *swisslipids) update() {
	defer s.d.wg.Done()

	// Test mode: get limit and open ID log file
	testLimit := config.GetTestLimit(s.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, s.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// SwissLipids provides multiple TSV files for complete biological context
	// Parse them in order: main lipids first, then cross-reference files

	// Initialize test mode ID tracking
	if config.IsTestMode() {
		s.testLipidIDs = make(map[string]bool)
		log.Printf("[%s] Test mode enabled: will track processed lipid IDs for filtering cross-references", s.source)
	}

	// 1. Parse main lipids.tsv - creates primary entries with attributes
	log.Printf("[%s] Processing main lipids data (lipids.tsv)", s.source)
	s.downloadAndParseTSV("lipids.tsv", func(reader io.Reader) {
		s.parseLipidsTSV(reader, testLimit, idLogFile)
	})

	// In test mode, log how many IDs were processed
	if config.IsTestMode() {
		log.Printf("[%s] Test mode: processed %d lipid IDs, will filter cross-references to these IDs only", s.source, len(s.testLipidIDs))
	}

	// 2. Parse lipids2uniprot.tsv - adds UniProt cross-references
	log.Printf("[%s] Processing lipid-protein associations (lipids2uniprot.tsv)", s.source)
	s.downloadAndParseTSV("lipids2uniprot.tsv", func(reader io.Reader) {
		s.parseLipids2UniprotTSV(reader)
	})

	// 3. Parse go.tsv - adds GO cellular location annotations
	log.Printf("[%s] Processing GO annotations (go.tsv)", s.source)
	s.downloadAndParseTSV("go.tsv", func(reader io.Reader) {
		s.parseGoTSV(reader)
	})

	// 4. Parse tissues.tsv - adds tissue/organ distribution data
	log.Printf("[%s] Processing tissue annotations (tissues.tsv)", s.source)
	s.downloadAndParseTSV("tissues.tsv", func(reader io.Reader) {
		s.parseTissuesTSV(reader)
	})

	// 5. Parse enzymes.tsv - adds Rhea reaction cross-references
	log.Printf("[%s] Processing enzyme/reaction data (enzymes.tsv)", s.source)
	s.downloadAndParseTSV("enzymes.tsv", func(reader io.Reader) {
		s.parseEnzymesTSV(reader)
	})

	// 6. Parse evidences.tsv - adds ECO evidence code cross-references
	log.Printf("[%s] Processing evidence codes (evidences.tsv)", s.source)
	s.downloadAndParseTSV("evidences.tsv", func(reader io.Reader) {
		s.parseEvidencesTSV(reader)
	})

	log.Printf("[%s] SwissLipids processing completed successfully", s.source)
}

// downloadAndParseTSV downloads a specific TSV file from SwissLipids API and parses it
func (s *swisslipids) downloadAndParseTSV(filename string, parseFunc func(io.Reader)) {
	var reader io.Reader

	if config.IsTestMode() && config.Dataconf[s.source]["path2"] != "" {
		// In test mode, try to use local file
		// For now, skip additional files in test mode unless explicitly provided
		log.Printf("[%s] Test mode: Skipping %s (only using main test file)", s.source, filename)
		return
	}

	// Build API URL for this file
	apiURL := "https://www.swisslipids.org/api/file.php?cas=download_files&file=" + filename
	log.Printf("[%s] Downloading %s from API", s.source, filename)

	resp, err := http.Get(apiURL)
	s.check(err, "downloading SwissLipids file: "+filename)
	defer resp.Body.Close()

	// Validate HTTP response
	if resp.StatusCode != 200 {
		log.Fatalf("[%s] Error: SwissLipids server returned HTTP %s for %s",
			s.source, resp.Status, filename)
	}

	// SwissLipids API may return gzipped or plain TSV (smaller files are plain)
	// Try gzip decompression first, fallback to plain text if it fails

	// Read first few bytes to check if gzipped (gzip magic number: 0x1f 0x8b)
	peekBuf := make([]byte, 2)
	n, err := io.ReadFull(resp.Body, peekBuf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		s.check(err, "reading header from: "+filename)
	}

	// Create a reader that includes the peeked bytes
	combinedReader := io.MultiReader(bytes.NewReader(peekBuf[:n]), resp.Body)

	// Check for gzip magic number (0x1f 0x8b)
	if n >= 2 && peekBuf[0] == 0x1f && peekBuf[1] == 0x8b {
		log.Printf("[%s] Decompressing gzipped TSV: %s", s.source, filename)
		gzReader, err := gzip.NewReader(combinedReader)
		s.check(err, "creating gzip reader for: "+filename)
		defer gzReader.Close()
		reader = gzReader
	} else {
		log.Printf("[%s] Reading plain TSV: %s", s.source, filename)
		reader = combinedReader
	}

	// Parse using the provided function
	parseFunc(reader)
}

// parseLipidsTSV parses the main lipids.tsv file and creates primary entries
func (s *swisslipids) parseLipidsTSV(reader io.Reader, testLimit int, idLogFile *os.File) {
	scanner := bufio.NewScanner(reader)
	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var entryCount int64
	var previous int64
	startTime := time.Now()

	// Column indices (to be determined from header)
	var colIndices map[string]int

	// First line should be header
	if !scanner.Scan() {
		log.Fatalf("[%s] Error: Empty TSV file", s.source)
	}

	header := scanner.Text()
	colIndices = s.parseHeader(header)

	// Log column structure for debugging
	log.Printf("[%s] TSV columns detected: %d columns", s.source, len(colIndices))
	if entryCount == 0 {
		log.Printf("[%s] DEBUG: Column mapping:", s.source)
		for colName, colIdx := range colIndices {
			log.Printf("[%s]   %s -> column %d", s.source, colName, colIdx)
		}
	}

	// Process data lines
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Parse and process record
		s.processRecord(line, colIndices, &entryCount, idLogFile)

		// Progress reporting
		elapsed := int64(time.Since(startTime).Seconds())
		if elapsed > previous+s.d.progInterval {
			previous = elapsed
			s.d.progChan <- &progressInfo{
				dataset: s.source,
			}
		}

		// Check test limit
		if config.IsTestMode() && testLimit > 0 && entryCount >= int64(testLimit) {
			log.Printf("[%s] Test mode: reached limit of %d entries", s.source, testLimit)
			break
		}
	}

	s.check(scanner.Err(), "scanning TSV file")
	log.Printf("[%s] Processed %d lipid entries", s.source, entryCount)
}

// parseHeader parses the TSV header and returns column name to index mapping
func (s *swisslipids) parseHeader(header string) map[string]int {
	colIndices := make(map[string]int)

	columns := strings.Split(header, "\t")
	for i, col := range columns {
		// Normalize column names (trim whitespace, lowercase for matching)
		normalizedCol := strings.TrimSpace(col)
		colIndices[normalizedCol] = i
	}

	return colIndices
}

// getColumn safely retrieves a column value by name
func (s *swisslipids) getColumn(fields []string, colIndices map[string]int, colName string) string {
	if idx, exists := colIndices[colName]; exists && idx < len(fields) {
		return strings.TrimSpace(fields[idx])
	}
	return ""
}

// processRecord processes a single lipid record from TSV
func (s *swisslipids) processRecord(line string, colIndices map[string]int, entryCount *int64, idLogFile *os.File) {
	fields := strings.Split(line, "\t")

	// Get SwissLipids ID - from API column "Lipid ID"
	slmID := s.getColumn(fields, colIndices, "Lipid ID")
	if slmID == "" {
		// Skip records without SwissLipids ID
		return
	}

	atomic.AddInt64(entryCount, 1)

	// Track ID in test mode for filtering cross-references
	if config.IsTestMode() && s.testLipidIDs != nil {
		s.testLipidIDs[slmID] = true
	}

	// Log ID in test mode
	if idLogFile != nil {
		logProcessedID(idLogFile, slmID)
	}

	// Extract fields using exact API column names
	name := s.getColumn(fields, colIndices, "Name")
	abbreviation := s.getColumn(fields, colIndices, "Abbreviation*")
	level := s.getColumn(fields, colIndices, "Level")

	// Note: "Lipid class*" is actually a parent lipid ID reference (e.g., SLM:000399814)
	// not a textual classification. Category/class hierarchy would need to be derived
	// by following the parent chain or from a separate classification file.
	// For now, we'll leave category, main_class, sub_class empty and focus on
	// the chemical descriptors and level information that ARE available.

	smiles := s.getColumn(fields, colIndices, "SMILES (pH7.3)")
	inchi := s.getColumn(fields, colIndices, "InChI (pH7.3)")
	inchiKey := s.getColumn(fields, colIndices, "InChI key (pH7.3)")
	formula := s.getColumn(fields, colIndices, "Formula (pH7.3)")
	mass := s.getColumn(fields, colIndices, "Mass (pH7.3)")
	charge := s.getColumn(fields, colIndices, "Charge (pH7.3)")

	// Create attribute object
	attr := pbuf.SwisslipidsAttr{
		Name:         name,
		Abbreviation: abbreviation,
		Category:     "", // TODO: Derive from parent chain or separate classification
		MainClass:    "", // TODO: Derive from parent chain or separate classification
		SubClass:     "", // TODO: Derive from parent chain or separate classification
		Level:        level,
		Smiles:       smiles,
		Inchi:        inchi,
		InchiKey:     inchiKey,
		Formula:      formula,
		Mass:         mass,
		Charge:       charge,
	}

	// Parse synonyms - using exact API column name "Synonyms*"
	synonymsStr := s.getColumn(fields, colIndices, "Synonyms*")

	if synonymsStr != "" {
		// Synonyms can be pipe-delimited or semicolon-delimited
		var synList []string
		if strings.Contains(synonymsStr, "|") {
			synList = strings.Split(synonymsStr, "|")
		} else if strings.Contains(synonymsStr, ";") {
			synList = strings.Split(synonymsStr, ";")
		} else {
			synList = []string{synonymsStr}
		}

		for _, syn := range synList {
			trimmedSyn := strings.TrimSpace(syn)
			if trimmedSyn != "" && trimmedSyn != "null" && trimmedSyn != "NA" {
				attr.Synonyms = append(attr.Synonyms, trimmedSyn)
			}
		}
	}

	// Marshal attributes
	b, err := ffjson.Marshal(&attr)
	s.check(err, "marshaling attributes for "+slmID)

	// Get dataset ID string from config
	var fr = config.Dataconf[s.source]["id"]

	// Debug: Log marshaled JSON for first entry
	if *entryCount == 1 {
		log.Printf("[%s] DEBUG: Marshaled JSON: %s", s.source, string(b))
		log.Printf("[%s] DEBUG: Dataset ID from config: %s", s.source, fr)
	}

	// Save entry to database
	s.d.addProp3(slmID, fr, b)

	// Add text search terms
	// textLinkID is a constant "0" defined in update.go

	// Add name as searchable
	if attr.Name != "" {
		s.d.addXref(attr.Name, textLinkID, slmID, s.source, true)
	}

	// Add abbreviation as searchable
	if attr.Abbreviation != "" {
		s.d.addXref(attr.Abbreviation, textLinkID, slmID, s.source, true)
	}

	// Add synonyms as searchable
	for _, syn := range attr.Synonyms {
		if syn != "" {
			s.d.addXref(syn, textLinkID, slmID, s.source, true)
		}
	}

	// Add InChI key as searchable (useful for structure search)
	if attr.InchiKey != "" {
		s.d.addXref(attr.InchiKey, textLinkID, slmID, s.source, true)
	}

	// Create cross-references to other databases
	// Using exact API column names

	// LIPID MAPS cross-reference (column: "LIPID MAPS")
	lmID := s.getColumn(fields, colIndices, "LIPID MAPS")
	if lmID != "" && lmID != "null" && lmID != "NA" && lmID != "-" {
		s.d.addXref(slmID, fr, lmID, "lipidmaps", false)
	}

	// ChEBI cross-reference (column: "CHEBI")
	chebiID := s.getColumn(fields, colIndices, "CHEBI")
	if chebiID != "" && chebiID != "null" && chebiID != "NA" && chebiID != "-" {
		// Ensure ChEBI ID has proper prefix
		if !strings.HasPrefix(chebiID, "CHEBI:") && !strings.HasPrefix(chebiID, "chebi:") {
			chebiID = "CHEBI:" + chebiID
		}
		s.d.addXref(slmID, fr, chebiID, "chebi", false)
	}

	// HMDB cross-reference (column: "HMDB")
	hmdbID := s.getColumn(fields, colIndices, "HMDB")
	if hmdbID != "" && hmdbID != "null" && hmdbID != "NA" && hmdbID != "-" {
		s.d.addXref(slmID, fr, hmdbID, "hmdb", false)
	}

	// Log first entry details for debugging
	if *entryCount == 1 {
		log.Printf("[%s] DEBUG: First entry details:", s.source)
		log.Printf("[%s]   ID: %s", s.source, slmID)
		log.Printf("[%s]   Name: %s", s.source, attr.Name)
		log.Printf("[%s]   Abbreviation: %s", s.source, attr.Abbreviation)
		log.Printf("[%s]   Level: %s", s.source, attr.Level)
		log.Printf("[%s]   Formula: %s", s.source, attr.Formula)
		log.Printf("[%s]   Mass: %s", s.source, attr.Mass)
	}
}

// parseLipids2UniprotTSV parses lipids2uniprot.tsv to add UniProt cross-references
// Columns: metabolite id, UniprotKB IDs, level, metabolite name, abbreviations, synonyms, ...
func (s *swisslipids) parseLipids2UniprotTSV(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var colIndices map[string]int
	var xrefCount int64

	// Parse header
	if !scanner.Scan() {
		log.Printf("[%s] Warning: Empty lipids2uniprot.tsv file", s.source)
		return
	}
	header := scanner.Text()
	colIndices = s.parseHeader(header)

	// Get dataset ID from config
	fr := config.Dataconf[s.source]["id"]

	// Process data lines
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\t")

		// Get SwissLipids ID
		slmID := s.getColumn(fields, colIndices, "metabolite id")
		if slmID == "" {
			continue
		}

		// In test mode, skip IDs that weren't processed in main lipids file
		if config.IsTestMode() && s.testLipidIDs != nil {
			if !s.testLipidIDs[slmID] {
				continue // Skip this ID - not in test set
			}
		}

		// Get UniProt IDs (can be multiple, pipe-delimited)
		uniprotIDs := s.getColumn(fields, colIndices, "UniprotKB IDs")
		if uniprotIDs == "" || uniprotIDs == "null" || uniprotIDs == "-" {
			continue
		}

		// Parse multiple UniProt IDs
		ids := strings.Split(uniprotIDs, "|")
		for _, uniprotID := range ids {
			trimmedID := strings.TrimSpace(uniprotID)
			if trimmedID != "" && trimmedID != "null" {
				s.d.addXref(slmID, fr, trimmedID, "uniprot", false)
				xrefCount++
			}
		}
	}

	s.check(scanner.Err(), "scanning lipids2uniprot.tsv")
	log.Printf("[%s] Added %d UniProt cross-references from lipids2uniprot.tsv", s.source, xrefCount)
}

// parseGoTSV parses go.tsv to add GO cellular location annotations
// Columns: Lipid ID, Lipid name, GO ID, GO term, Taxon ID, Taxon scientific name, Evidence tag ID
func (s *swisslipids) parseGoTSV(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var colIndices map[string]int
	var xrefCount int64

	// Parse header
	if !scanner.Scan() {
		log.Printf("[%s] Warning: Empty go.tsv file", s.source)
		return
	}
	header := scanner.Text()
	colIndices = s.parseHeader(header)

	// Get dataset ID from config
	fr := config.Dataconf[s.source]["id"]

	// Process data lines
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\t")

		// Get SwissLipids ID
		slmID := s.getColumn(fields, colIndices, "Lipid ID")
		if slmID == "" {
			continue
		}

		// In test mode, skip IDs that weren't processed in main lipids file
		if config.IsTestMode() && s.testLipidIDs != nil {
			if !s.testLipidIDs[slmID] {
				continue // Skip this ID - not in test set
			}
		}

		// Get GO ID
		goID := s.getColumn(fields, colIndices, "GO ID")
		if goID == "" || goID == "null" || goID == "-" {
			continue
		}

		// Add GO cross-reference
		// TODO: GO dataset integration - for now, cross-reference created but GO dataset not yet implemented
		s.d.addXref(slmID, fr, goID, "go", false)
		xrefCount++
	}

	s.check(scanner.Err(), "scanning go.tsv")
	log.Printf("[%s] Added %d GO cross-references from go.tsv (GO dataset already available)", s.source, xrefCount)
}

// parseTissuesTSV parses tissues.tsv to add tissue/organ distribution data
// Columns: Lipid ID, Lipid name, Tissue/Cell ID, Tissue/Cell name, Taxon ID, Taxon scientific name, Evidence tag ID
func (s *swisslipids) parseTissuesTSV(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var colIndices map[string]int
	var xrefCount int64

	// Parse header
	if !scanner.Scan() {
		log.Printf("[%s] Warning: Empty tissues.tsv file", s.source)
		return
	}
	header := scanner.Text()
	colIndices = s.parseHeader(header)

	// Get dataset ID from config
	fr := config.Dataconf[s.source]["id"]

	// Process data lines
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\t")

		// Get SwissLipids ID
		slmID := s.getColumn(fields, colIndices, "Lipid ID")
		if slmID == "" {
			continue
		}

		// In test mode, skip IDs that weren't processed in main lipids file
		if config.IsTestMode() && s.testLipidIDs != nil {
			if !s.testLipidIDs[slmID] {
				continue // Skip this ID - not in test set
			}
		}

		// Get Tissue/Cell ID (Uberon)
		tissueID := s.getColumn(fields, colIndices, "Tissue/Cell ID")
		if tissueID == "" || tissueID == "null" || tissueID == "-" {
			continue
		}

		// Add tissue cross-reference
		// TODO: Uberon dataset integration - for now, cross-reference created but Uberon dataset not yet implemented
		s.d.addXref(slmID, fr, tissueID, "uberon", false)
		xrefCount++
	}

	s.check(scanner.Err(), "scanning tissues.tsv")
	log.Printf("[%s] Added %d Uberon tissue cross-references from tissues.tsv", s.source, xrefCount)
	log.Printf("[%s] TODO: Implement Uberon dataset for tissue/organ annotations", s.source)
}

// parseEnzymesTSV parses enzymes.tsv to add Rhea reaction cross-references
// Columns: SwissLipids ID, UniProtKB AC(s), Gene name, Protein taxon, Taxon scientific name, Rhea ID, Reaction text, Evidence tag ID
func (s *swisslipids) parseEnzymesTSV(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var colIndices map[string]int
	var rheaXrefCount, uniprotXrefCount int64

	// Parse header
	if !scanner.Scan() {
		log.Printf("[%s] Warning: Empty enzymes.tsv file", s.source)
		return
	}
	header := scanner.Text()
	colIndices = s.parseHeader(header)

	// Get dataset ID from config
	fr := config.Dataconf[s.source]["id"]

	// Process data lines
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\t")

		// Get SwissLipids ID
		slmID := s.getColumn(fields, colIndices, "SwissLipids ID")
		if slmID == "" {
			continue
		}

		// In test mode, skip IDs that weren't processed in main lipids file
		if config.IsTestMode() && s.testLipidIDs != nil {
			if !s.testLipidIDs[slmID] {
				continue // Skip this ID - not in test set
			}
		}

		// Get Rhea ID
		rheaID := s.getColumn(fields, colIndices, "Rhea ID")
		if rheaID != "" && rheaID != "null" && rheaID != "-" {
			// Add Rhea cross-reference
			// TODO: Rhea dataset integration - for now, cross-reference created but Rhea dataset not yet implemented
			s.d.addXref(slmID, fr, rheaID, "rhea", false)
			rheaXrefCount++
		}

		// Also get UniProt IDs (additional protein associations via enzymes)
		uniprotIDs := s.getColumn(fields, colIndices, "UniProtKB AC(s)")
		if uniprotIDs != "" && uniprotIDs != "null" && uniprotIDs != "-" {
			// Parse multiple UniProt IDs (pipe-delimited)
			ids := strings.Split(uniprotIDs, "|")
			for _, uniprotID := range ids {
				trimmedID := strings.TrimSpace(uniprotID)
				if trimmedID != "" && trimmedID != "null" {
					s.d.addXref(slmID, fr, trimmedID, "uniprot", false)
					uniprotXrefCount++
				}
			}
		}
	}

	s.check(scanner.Err(), "scanning enzymes.tsv")
	log.Printf("[%s] Added %d Rhea reaction cross-references from enzymes.tsv", s.source, rheaXrefCount)
	log.Printf("[%s] Added %d additional UniProt cross-references from enzymes.tsv", s.source, uniprotXrefCount)
	log.Printf("[%s] TODO: Implement Rhea dataset for biochemical reaction integration", s.source)
}

// parseEvidencesTSV parses evidences.tsv to add ECO evidence code cross-references
// Columns: Evidence ID, ECO ID, ECO definition, PMID ID, Figure legend
// This provides evidence quality tracking via ECO (Evidence & Conclusion Ontology)
func (s *swisslipids) parseEvidencesTSV(reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var colIndices map[string]int
	var ecoXrefCount int64

	// Parse header
	if !scanner.Scan() {
		log.Printf("[%s] Warning: Empty evidences.tsv file", s.source)
		return
	}
	header := scanner.Text()
	colIndices = s.parseHeader(header)

	// Get dataset ID from config
	fr := config.Dataconf[s.source]["id"]

	// Process data lines
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\t")

		// Get Evidence ID (SwissLipids internal evidence tag)
		evidenceID := s.getColumn(fields, colIndices, "Evidence ID")
		if evidenceID == "" {
			continue
		}

		// Get ECO ID
		ecoID := s.getColumn(fields, colIndices, "ECO ID")
		if ecoID == "" || ecoID == "null" || ecoID == "-" {
			continue
		}

		// Add ECO cross-reference
		// ECO dataset already exists in biobtree (ID 23)
		// Evidence tags can be referenced via their internal ID
		s.d.addXref(evidenceID, fr, ecoID, "eco", false)
		ecoXrefCount++
	}

	s.check(scanner.Err(), "scanning evidences.tsv")
	log.Printf("[%s] Added %d ECO evidence code cross-references from evidences.tsv (ECO dataset already available)", s.source, ecoXrefCount)
}
