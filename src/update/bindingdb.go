package update

import (
	"archive/zip"
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
}

// parseAndSaveEntries processes the BindingDB TSV ZIP file
func (b *bindingdb) parseAndSaveEntries(testLimit int, idLogFile *os.File) {
	// Build file path
	filePath := config.Dataconf[b.source]["path"]
	log.Printf("BindingDB: Downloading from %s", filePath)

	// Open ZIP file via HTTP
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(b.source, b.d.ebiFtp, b.d.ebiFtpPath, filePath)
	b.check(err, "opening BindingDB ZIP file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[b.source]["id"]

	// The file is a ZIP, need to extract the TSV inside
	log.Println("BindingDB: Reading ZIP file...")
	zipData, err := io.ReadAll(br)
	b.check(err, "reading ZIP file")

	log.Printf("BindingDB: ZIP file size: %.2f MB", float64(len(zipData))/1024/1024)

	// Open ZIP
	zipReader, err := zip.NewReader(readerAtFromBytes(zipData), int64(len(zipData)))
	b.check(err, "opening ZIP archive")

	if len(zipReader.File) == 0 {
		log.Fatal("BindingDB: ZIP file is empty")
	}

	// Find the TSV file inside
	var tsvFile *zip.File
	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, ".tsv") || strings.HasSuffix(f.Name, ".txt") {
			tsvFile = f
			break
		}
	}

	if tsvFile == nil {
		log.Fatal("BindingDB: No TSV file found in ZIP")
	}

	log.Printf("BindingDB: Found file in ZIP: %s (%.2f MB uncompressed)",
		tsvFile.Name, float64(tsvFile.UncompressedSize64)/1024/1024)

	// Open the TSV file from ZIP
	rc, err := tsvFile.Open()
	b.check(err, "opening TSV from ZIP")
	defer rc.Close()

	// Create scanner for TSV format
	scanner := bufio.NewScanner(rc)

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

	var savedEntries int
	var previous int64
	var skippedEmptyID int
	var totalRowsRead int

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

		// Progress logging every 50000 rows
		if totalRowsRead%50000 == 0 {
			log.Printf("BindingDB: Read %d rows so far (line %d)...", totalRowsRead, lineNum)
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
		attr := b.buildEntry(row, colMap, bindingdbID)
		if attr == nil {
			continue
		}

		// Save entry
		b.saveEntry(bindingdbID, attr, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(bindingdbID + "\n")
		}

		savedEntries++

		// Test mode: check limit
		if testLimit > 0 && savedEntries >= testLimit {
			log.Printf("BindingDB: [TEST MODE] Reached limit of %d entries, stopping", testLimit)
			break
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("BindingDB: Scanner error: %v", err)
	}

	log.Printf("BindingDB: Total rows read: %d, Saved: %d entries", totalRowsRead, savedEntries)
	log.Printf("BindingDB: Skipped - empty ID: %d", skippedEmptyID)

	// Update entry statistics
	atomic.AddUint64(&b.d.totalParsedEntry, uint64(savedEntries))
}

// buildEntry creates a BindingDB entry from row
func (b *bindingdb) buildEntry(row []string, colMap map[string]int, bindingdbID string) *pbuf.BindingdbAttr {
	// Extract UniProt IDs (can be multiple, pipe-separated)
	uniprotField := getField(row, colMap, "UniProt (SwissProt) Primary ID of Target Chain")
	var uniprotIDs []string
	if uniprotField != "" {
		for _, id := range strings.Split(uniprotField, "|") {
			id = strings.TrimSpace(id)
			if id != "" {
				uniprotIDs = append(uniprotIDs, id)
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

	// Build attribute object
	attr := &pbuf.BindingdbAttr{
		BindingdbId:          bindingdbID,
		LigandName:           getField(row, colMap, "Ligand Name"),
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

	return attr
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
func (b *bindingdb) saveEntry(bindingdbID string, attr *pbuf.BindingdbAttr, sourceID string) {
	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	b.check(err, fmt.Sprintf("marshaling BindingDB attributes for %s", bindingdbID))

	// Save entry with unique key
	b.d.addProp3(bindingdbID, sourceID, attrBytes)

	// Create cross-references
	b.createCrossReferences(bindingdbID, attr, sourceID)
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

	// Bidirectional cross-reference: BindingDB ↔ ChEMBL
	if _, exists := config.Dataconf["chembl_molecule"]; exists {
		for _, chemblID := range attr.ChemblIds {
			if chemblID != "" {
				b.d.addXref(bindingdbID, sourceID, chemblID, "chembl_molecule", false)
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
