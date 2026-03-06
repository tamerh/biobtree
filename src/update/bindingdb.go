package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/krolaw/zipstream"
	"github.com/pquerna/ffjson/ffjson"
)

type bindingdb struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (b *bindingdb) check(err error, operation string) {
	checkWithContext(err, b.source, operation)
}

// Main update entry point
func (b *bindingdb) update() {
	defer b.d.wg.Done()

	log.Println("BindingDB: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(b.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, b.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("BindingDB: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Process BindingDB TSV file
	b.parseAndSaveEntries(testLimit, idLogFile)

	log.Printf("BindingDB: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress handler so status is updated from "processing" to "processed"
	b.d.progChan <- &progressInfo{dataset: b.source, done: true}
}

// parseAndSaveEntries processes the BindingDB TSV ZIP file using streaming
func (b *bindingdb) parseAndSaveEntries(testLimit int, idLogFile *os.File) {
	// Build file path
	filePath := config.Dataconf[b.source]["path"]
	log.Printf("BindingDB: Downloading from %s", filePath)

	sourceID := config.Dataconf[b.source]["id"]

	// Use zipstream for streaming ZIP processing (like HMDB)
	var zipReader io.Reader
	var readerCloser io.Closer // Track reader for proper cleanup (local file or HTTP response)

	// Check if using local file
	if config.Dataconf[b.source]["useLocalFile"] == "yes" {
		localFile, err := os.Open(filePath)
		b.check(err, "opening local BindingDB file: "+filePath)
		// Don't defer close here - we need to handle it after the zipstream finishes
		zipReader = localFile
		readerCloser = localFile
	} else {
		// Download from remote URL
		resp, err := http.Get(filePath)
		b.check(err, "downloading BindingDB ZIP file")
		// Don't defer close here - the goroutine will close it after reading completes

		if resp.StatusCode != 200 {
			log.Fatalf("[%s] Error: BindingDB server returned HTTP %s from: %s",
				b.source, resp.Status, filePath)
		}

		zipReader = resp.Body
		readerCloser = resp.Body
	}

	// Use zipstream for streaming decompression
	log.Println("BindingDB: Streaming ZIP file...")
	zips := zipstream.NewReader(zipReader)

	// Get first (and only) entry in ZIP
	_, err := zips.Next()
	if err != nil {
		log.Printf("[bindingdb] ERROR: Failed to read ZIP stream")
		log.Printf("[bindingdb] This may happen if the download was interrupted")
		b.check(err, "reading first entry from BindingDB ZIP stream")
	}

	// Create buffered reader for TSV parsing
	br := bufio.NewReaderSize(zips, fileBufSize)

	// Create scanner for TSV format
	scanner := bufio.NewScanner(br)

	// Increase buffer size for long lines (SMILES strings can be very long)
	const maxCapacity = 2 * 1024 * 1024 // 2MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read and parse header
	if !scanner.Scan() {
		b.check(scanner.Err(), "reading BindingDB header")
		return
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	log.Printf("BindingDB: Found %d columns in header", len(colMap))

	// Log key columns for debugging
	keyColumns := []string{
		"BindingDB MonomerID",
		"Ligand SMILES",
		"Ligand InChI",
		"Ligand InChI Key",
		"Target Name",
		"Target Source Organism According to Curator or DataSource",
		"Ki (nM)",
		"IC50 (nM)",
		"Kd (nM)",
		"EC50 (nM)",
		"UniProt (SwissProt) Primary ID of Target Chain",
		"PubChem CID",
		"ChEMBL ID of Ligand",
		"ChEBI ID of Ligand",
	}
	for _, col := range keyColumns {
		if idx, ok := colMap[col]; ok {
			log.Printf("BindingDB: Column '%s' at index %d", col, idx)
		}
	}

	// Stream lines through a channel (same pattern as HMDB XML parser)
	// This allows proper cleanup when stopping early
	type lineData struct {
		lineNum int
		text    string
	}
	stream := make(chan lineData, 100)

	// Goroutine reads from scanner and sends to channel
	// Closes reader after all reads complete (including zipstream's data descriptor)
	go func() {
		lineNum := 1
		for scanner.Scan() {
			lineNum++
			stream <- lineData{lineNum: lineNum, text: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			log.Printf("BindingDB: Scanner error: %v", err)
		}
		// Drain any remaining zipstream data (data descriptor) before closing
		io.Copy(io.Discard, zips)
		if readerCloser != nil {
			readerCloser.Close()
		}
		close(stream)
	}()

	var savedEntries int
	var previous int64
	var skippedEmptyID int
	var totalRowsRead int
	var stoppedEarly bool

	for ld := range stream {
		line := ld.text

		// Skip empty lines
		if line == "" {
			continue
		}

		totalRowsRead++

		// Split by tab
		row := strings.Split(line, "\t")

		// Progress logging every 50000 rows
		if totalRowsRead%50000 == 0 {
			log.Printf("BindingDB: Read %d rows so far (line %d)...", totalRowsRead, ld.lineNum)
		}

		// Progress tracking
		elapsed := int64(time.Since(b.d.start).Seconds())
		if elapsed > previous+b.d.progInterval {
			previous = elapsed
			b.d.progChan <- &progressInfo{dataset: b.source, currentKBPerSec: int64(savedEntries / int(elapsed))}
		}

		// Extract BindingDB Monomer ID (primary key)
		bindingdbID := getField(row, colMap, "BindingDB MonomerID")
		if bindingdbID == "" {
			skippedEmptyID++
			continue
		}

		// Build entry
		attr, drugbankIDs := b.buildEntry(row, colMap, bindingdbID)
		if attr == nil {
			continue
		}

		// Save entry
		b.saveEntry(bindingdbID, attr, sourceID, drugbankIDs)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(bindingdbID + "\n")
		}

		savedEntries++

		// Test mode: check limit
		if testLimit > 0 && savedEntries >= testLimit {
			log.Printf("BindingDB: [TEST MODE] Reached limit of %d entries, stopping", testLimit)
			stoppedEarly = true
			break
		}
	}

	// If stopped early, drain remaining stream in background
	// This lets the goroutine finish reading and properly close the file
	// (same pattern as HMDB XML parser)
	if stoppedEarly {
		go func() {
			for range stream {
				// Discard remaining entries
			}
		}()
	}
	// On normal completion, the loop exits when channel closes,
	// and the goroutine has already handled file cleanup

	log.Printf("BindingDB: Total rows read: %d, Saved: %d entries", totalRowsRead, savedEntries)
	log.Printf("BindingDB: Skipped - empty ID: %d", skippedEmptyID)

	// Update entry statistics
	atomic.AddUint64(&b.d.totalParsedEntry, uint64(savedEntries))
}

// buildEntry creates a BindingDB entry from row
// Returns attr and drugbankIDs (for xref creation, not stored in attr)
func (b *bindingdb) buildEntry(row []string, colMap map[string]int, bindingdbID string) (*pbuf.BindingdbAttr, []string) {
	// Extract UniProt IDs from all target chains (up to 50 chains supported)
	// BindingDB TSV has numbered columns: "UniProt (SwissProt) Primary ID of Target Chain 1", etc.
	var uniprotIDs []string
	seenIDs := make(map[string]bool)

	// Collect UniProt IDs from all chain columns (SwissProt and TrEMBL)
	for i := 1; i <= 50; i++ {
		// SwissProt IDs
		swissprotCol := fmt.Sprintf("UniProt (SwissProt) Primary ID of Target Chain %d", i)
		if field := getField(row, colMap, swissprotCol); field != "" {
			for _, id := range strings.Split(field, "|") {
				id = strings.TrimSpace(id)
				if id != "" && !seenIDs[id] {
					uniprotIDs = append(uniprotIDs, id)
					seenIDs[id] = true
				}
			}
		}
		// TrEMBL IDs (for proteins not in SwissProt)
		tremblCol := fmt.Sprintf("UniProt (TrEMBL) Primary ID of Target Chain %d", i)
		if field := getField(row, colMap, tremblCol); field != "" {
			for _, id := range strings.Split(field, "|") {
				id = strings.TrimSpace(id)
				if id != "" && !seenIDs[id] {
					uniprotIDs = append(uniprotIDs, id)
					seenIDs[id] = true
				}
			}
		}
	}

	// Extract PubChem CIDs (can be multiple, pipe-separated)
	pubchemField := getField(row, colMap, "PubChem CID")
	var pubchemCIDs []string
	if pubchemField != "" {
		for _, id := range strings.Split(pubchemField, "|") {
			id = strings.TrimSpace(id)
			if id != "" {
				pubchemCIDs = append(pubchemCIDs, id)
			}
		}
	}

	// Extract ChEMBL IDs
	chemblField := getField(row, colMap, "ChEMBL ID of Ligand")
	var chemblIDs []string
	if chemblField != "" {
		for _, id := range strings.Split(chemblField, "|") {
			id = strings.TrimSpace(id)
			if id != "" {
				chemblIDs = append(chemblIDs, id)
			}
		}
	}

	// Extract ChEBI IDs
	chebiField := getField(row, colMap, "ChEBI ID of Ligand")
	var chebiIDs []string
	if chebiField != "" {
		for _, id := range strings.Split(chebiField, "|") {
			id = strings.TrimSpace(id)
			if id != "" {
				// Ensure CHEBI: prefix
				if !strings.HasPrefix(strings.ToUpper(id), "CHEBI:") {
					id = "CHEBI:" + id
				}
				chebiIDs = append(chebiIDs, id)
			}
		}
	}

	// Extract DrugBank IDs
	drugbankField := getField(row, colMap, "DrugBank ID of Ligand")
	var drugbankIDs []string
	if drugbankField != "" {
		for _, id := range strings.Split(drugbankField, "|") {
			id = strings.TrimSpace(id)
			if id != "" {
				drugbankIDs = append(drugbankIDs, id)
			}
		}
	}

	// Build attribute object
	attr := &pbuf.BindingdbAttr{
		BindingdbId:          bindingdbID,
		LigandName:           getField(row, colMap, "BindingDB Ligand Name"),
		LigandSmiles:         getField(row, colMap, "Ligand SMILES"),
		LigandInchi:          getField(row, colMap, "Ligand InChI"),
		LigandInchiKey:       getField(row, colMap, "Ligand InChI Key"),
		TargetName:           getField(row, colMap, "Target Name"),
		TargetSourceOrganism: getField(row, colMap, "Target Source Organism According to Curator or DataSource"),
		Ki:                   formatAffinityValue(getField(row, colMap, "Ki (nM)")),
		Ic50:                 formatAffinityValue(getField(row, colMap, "IC50 (nM)")),
		Kd:                   formatAffinityValue(getField(row, colMap, "Kd (nM)")),
		Ec50:                 formatAffinityValue(getField(row, colMap, "EC50 (nM)")),
		Kon:                  getField(row, colMap, "kon (M-1-s-1)"),
		Koff:                 getField(row, colMap, "koff (s-1)"),
		Ph:                   getField(row, colMap, "pH"),
		TempC:                getField(row, colMap, "Temp (C)"),
		UniprotIds:           uniprotIDs,
		PubchemCids:          pubchemCIDs,
		ChemblIds:            chemblIDs,
		ChebiIds:             chebiIDs,
		Doi:                  getField(row, colMap, "Article DOI"),
		Pmid:                 getField(row, colMap, "PMID"),
		PatentNumber:         getField(row, colMap, "Patent Number"),
		Institution:          getField(row, colMap, "Institution"),
		CurationDate:         getField(row, colMap, "Curation/DataSource"),
	}

	return attr, drugbankIDs
}

// formatAffinityValue formats binding affinity values with units
func formatAffinityValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	// The values already come with units in some cases, or are just numbers (in nM)
	// If it's just a number, we can optionally append " nM"
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value + " nM"
	}
	return value
}

// saveEntry creates and saves a BindingDB entry
func (b *bindingdb) saveEntry(bindingdbID string, attr *pbuf.BindingdbAttr, sourceID string, drugbankIDs []string) {
	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	b.check(err, fmt.Sprintf("marshaling BindingDB attributes for %s", bindingdbID))

	// Save entry with unique key
	b.d.addProp3(bindingdbID, sourceID, attrBytes)

	// Create cross-references
	b.createCrossReferences(bindingdbID, attr, sourceID)

	// Bidirectional cross-reference: BindingDB ↔ DrugBank (not stored in attr)
	if _, exists := config.Dataconf["drugbank"]; exists {
		for _, drugbankID := range drugbankIDs {
			if drugbankID != "" {
				b.d.addXref(bindingdbID, sourceID, drugbankID, "drugbank", false)
			}
		}
	}
}

// createCrossReferences builds all cross-references for a BindingDB entry
func (b *bindingdb) createCrossReferences(bindingdbID string, attr *pbuf.BindingdbAttr, sourceID string) {
	// Text search: BindingDB ID searchable
	b.d.addXref(bindingdbID, textLinkID, bindingdbID, b.source, true)

	// Text search: ligand name searchable
	if attr.LigandName != "" {
		b.d.addXref(attr.LigandName, textLinkID, bindingdbID, b.source, true)
	}

	// Text search: target name searchable
	if attr.TargetName != "" {
		b.d.addXref(attr.TargetName, textLinkID, bindingdbID, b.source, true)
	}

	// Text search: InChI Key searchable (useful for structure search)
	if attr.LigandInchiKey != "" {
		b.d.addXref(attr.LigandInchiKey, textLinkID, bindingdbID, b.source, true)
	}

	// Bidirectional cross-reference: BindingDB ↔ UniProt
	if _, exists := config.Dataconf["uniprot"]; exists {
		for _, uniprotID := range attr.UniprotIds {
			if uniprotID != "" {
				b.d.addXref(bindingdbID, sourceID, uniprotID, "uniprot", false)
			}
		}
	}

	// Bidirectional cross-reference: BindingDB ↔ PubChem
	if _, exists := config.Dataconf["pubchem"]; exists {
		for _, pubchemCID := range attr.PubchemCids {
			if pubchemCID != "" {
				b.d.addXref(bindingdbID, sourceID, pubchemCID, "pubchem", false)
			}
		}
	}

	// Bidirectional cross-reference: BindingDB ↔ ChEMBL / SureChEMBL
	// Note: BindingDB's "ChEMBL ID of Ligand" field contains both:
	// - ChEMBL IDs (CHEMBL...) → link to chembl_molecule
	// - SureChEMBL IDs (SCHEMBL...) → link to patent_compound (numeric part = SureChEMBL compound ID)
	for _, chemblID := range attr.ChemblIds {
		if chemblID == "" {
			continue
		}
		if strings.HasPrefix(chemblID, "CHEMBL") {
			if _, exists := config.Dataconf["chembl_molecule"]; exists {
				b.d.addXref(bindingdbID, sourceID, chemblID, "chembl_molecule", false)
			}
		} else if strings.HasPrefix(chemblID, "SCHEMBL") {
			// SureChEMBL ID format: SCHEMBL<numeric_id> where numeric_id = patent_compound.id
			if _, exists := config.Dataconf["patent_compound"]; exists {
				numericID := strings.TrimPrefix(chemblID, "SCHEMBL")
				b.d.addXref(bindingdbID, sourceID, numericID, "patent_compound", false)
			}
		}
	}

	// Bidirectional cross-reference: BindingDB ↔ ChEBI
	if _, exists := config.Dataconf["chebi"]; exists {
		for _, chebiID := range attr.ChebiIds {
			if chebiID != "" {
				b.d.addXref(bindingdbID, sourceID, chebiID, "chebi", false)
			}
		}
	}

	// Cross-reference to PubMed if PMID exists
	if attr.Pmid != "" {
		// Text search: PMID searchable
		b.d.addXref(attr.Pmid, textLinkID, bindingdbID, b.source, true)
	}
}
