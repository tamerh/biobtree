package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type drugcentral struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (dc *drugcentral) check(err error, operation string) {
	checkWithContext(err, dc.source, operation)
}

// Main update entry point
func (dc *drugcentral) update() {
	defer dc.d.wg.Done()

	log.Println("DrugCentral: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(dc.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, dc.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("DrugCentral: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// First load structures file (SMILES, InChI, INN, CAS)
	structures := dc.loadStructures()
	log.Printf("DrugCentral: Loaded %d structures", len(structures))

	// Process drug-target interactions
	dc.parseAndSaveEntries(testLimit, idLogFile, structures)

	log.Printf("DrugCentral: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress tracker
	dc.d.progChan <- &progressInfo{dataset: dc.source, done: true}
}

// structureInfo holds chemical structure information for a drug
type structureInfo struct {
	smiles   string
	inchi    string
	inchiKey string
	inn      string
	casRN    string
}

// loadStructures loads the structures.smiles.tsv file and returns a map of STRUCT_ID -> structureInfo
func (dc *drugcentral) loadStructures() map[string]*structureInfo {
	structures := make(map[string]*structureInfo)

	pathStructures := config.Dataconf[dc.source]["pathStructures"]
	if pathStructures == "" {
		log.Println("DrugCentral: No pathStructures configured, skipping structure loading")
		return structures
	}

	log.Printf("DrugCentral: Loading structures from %s", pathStructures)

	// Download the structures file
	resp, err := http.Get(pathStructures)
	if err != nil {
		log.Printf("DrugCentral: Warning - could not load structures: %v", err)
		return structures
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("DrugCentral: Warning - structures server returned HTTP %s", resp.Status)
		return structures
	}

	// Parse TSV
	scanner := bufio.NewScanner(resp.Body)

	// Increase buffer size for long lines (SMILES can be very long)
	const maxCapacity = 2 * 1024 * 1024 // 2MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read header
	if !scanner.Scan() {
		log.Printf("DrugCentral: Warning - could not read structures header")
		return structures
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	log.Printf("DrugCentral: Structures header: %v", header)

	// Parse rows
	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue
		}

		row := strings.Split(line, "\t")

		// Get ID field
		id := getField(row, colMap, "ID")
		if id == "" {
			continue
		}

		structures[id] = &structureInfo{
			smiles:   getField(row, colMap, "SMILES"),
			inchi:    getField(row, colMap, "InChI"),
			inchiKey: getField(row, colMap, "InChIKey"),
			inn:      getField(row, colMap, "INN"),
			casRN:    getField(row, colMap, "CAS_RN"),
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("DrugCentral: Warning - error reading structures: %v", err)
	}

	return structures
}

// drugEntry holds aggregated data for a single drug (by STRUCT_ID)
type drugEntry struct {
	structID      string
	drugName      string
	innName       string
	casRN         string
	smiles        string
	inchi         string
	inchiKey      string
	targets       []*pbuf.DrugcentralTarget
	actionTypes   map[string]bool
	targetClasses map[string]bool
	organisms     map[string]bool
	uniprotIDs    map[string]bool // For cross-references
}

// parseAndSaveEntries processes the drug.target.interaction.tsv.gz file
func (dc *drugcentral) parseAndSaveEntries(testLimit int, idLogFile *os.File, structures map[string]*structureInfo) {
	filePath := config.Dataconf[dc.source]["path"]
	log.Printf("DrugCentral: Downloading from %s", filePath)

	sourceID := config.Dataconf[dc.source]["id"]

	// Download and decompress the gzip file
	resp, err := http.Get(filePath)
	dc.check(err, "downloading DrugCentral TSV file")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("[%s] Error: DrugCentral server returned HTTP %s from: %s",
			dc.source, resp.Status, filePath)
	}

	// Create gzip reader
	gzReader, err := gzip.NewReader(resp.Body)
	dc.check(err, "creating gzip reader")
	defer gzReader.Close()

	// Create buffered reader
	br := bufio.NewReaderSize(gzReader, fileBufSize)

	// Create scanner
	scanner := bufio.NewScanner(br)

	// Increase buffer size for long lines
	const maxCapacity = 2 * 1024 * 1024 // 2MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read header
	if !scanner.Scan() {
		dc.check(scanner.Err(), "reading DrugCentral header")
		return
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		// Remove quotes from column names
		cleanName := strings.Trim(name, "\"")
		colMap[cleanName] = i
	}

	log.Printf("DrugCentral: Found %d columns in header", len(colMap))

	// Log key columns
	keyColumns := []string{
		"DRUG_NAME", "STRUCT_ID", "TARGET_NAME", "TARGET_CLASS",
		"ACCESSION", "GENE", "SWISSPROT", "ACT_VALUE", "ACT_TYPE",
		"ACTION_TYPE", "TDL", "ORGANISM", "MOA",
	}
	for _, col := range keyColumns {
		if idx, ok := colMap[col]; ok {
			log.Printf("DrugCentral: Column '%s' at index %d", col, idx)
		}
	}

	// Aggregate entries by STRUCT_ID
	drugs := make(map[string]*drugEntry)

	var totalRowsRead int
	var previous int64

	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if line == "" {
			continue
		}

		totalRowsRead++

		// Progress logging
		if totalRowsRead%10000 == 0 {
			log.Printf("DrugCentral: Read %d rows so far (line %d)...", totalRowsRead, lineNum)
		}

		// Progress tracking
		elapsed := int64(time.Since(dc.d.start).Seconds())
		if elapsed > previous+dc.d.progInterval {
			previous = elapsed
			dc.d.progChan <- &progressInfo{dataset: dc.source, currentKBPerSec: int64(totalRowsRead / int(elapsed))}
		}

		// Split by tab
		row := strings.Split(line, "\t")

		// Get STRUCT_ID (primary key)
		structID := getFieldQuoted(row, colMap, "STRUCT_ID")
		if structID == "" {
			continue
		}

		// Get or create drug entry
		drug, exists := drugs[structID]
		if !exists {
			drugName := getFieldQuoted(row, colMap, "DRUG_NAME")

			// Get structure info if available
			var smiles, inchi, inchiKey, innName, casRN string
			if structInfo, ok := structures[structID]; ok {
				smiles = structInfo.smiles
				inchi = structInfo.inchi
				inchiKey = structInfo.inchiKey
				innName = structInfo.inn
				casRN = structInfo.casRN
			}

			drug = &drugEntry{
				structID:      structID,
				drugName:      drugName,
				innName:       innName,
				casRN:         casRN,
				smiles:        smiles,
				inchi:         inchi,
				inchiKey:      inchiKey,
				targets:       make([]*pbuf.DrugcentralTarget, 0),
				actionTypes:   make(map[string]bool),
				targetClasses: make(map[string]bool),
				organisms:     make(map[string]bool),
				uniprotIDs:    make(map[string]bool),
			}
			drugs[structID] = drug
		}

		// Parse target interaction
		target := dc.parseTarget(row, colMap)
		if target != nil {
			drug.targets = append(drug.targets, target)

			// Aggregate metadata
			if target.ActionType != "" {
				drug.actionTypes[target.ActionType] = true
			}
			if target.TargetClass != "" {
				drug.targetClasses[target.TargetClass] = true
			}
			if target.Organism != "" {
				drug.organisms[target.Organism] = true
			}

			// Collect UniProt IDs for cross-references
			if target.UniprotAccession != "" {
				// Handle pipe-separated UniProt IDs
				for _, acc := range strings.Split(target.UniprotAccession, "|") {
					acc = strings.TrimSpace(acc)
					if acc != "" {
						drug.uniprotIDs[acc] = true
					}
				}
			}
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("DrugCentral: Scanner error: %v", err)
	}

	log.Printf("DrugCentral: Total rows read: %d, Unique drugs: %d", totalRowsRead, len(drugs))

	// Save aggregated entries
	var savedEntries int
	for structID, drug := range drugs {
		// Build attribute object
		attr := dc.buildEntry(drug)

		// Save entry
		dc.saveEntry(structID, attr, sourceID, drug)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(structID + "\n")
		}

		savedEntries++

		// Test mode: check limit
		if testLimit > 0 && savedEntries >= testLimit {
			log.Printf("DrugCentral: [TEST MODE] Reached limit of %d entries, stopping", testLimit)
			break
		}
	}

	log.Printf("DrugCentral: Saved %d drug entries", savedEntries)

	// Update entry statistics
	atomic.AddUint64(&dc.d.totalParsedEntry, uint64(savedEntries))
}

// getFieldQuoted gets a field value from a row, handling quoted values
func getFieldQuoted(row []string, colMap map[string]int, colName string) string {
	if idx, ok := colMap[colName]; ok && idx < len(row) {
		return strings.Trim(row[idx], "\"")
	}
	return ""
}

// parseTarget parses a single target interaction row
func (dc *drugcentral) parseTarget(row []string, colMap map[string]int) *pbuf.DrugcentralTarget {
	targetName := getFieldQuoted(row, colMap, "TARGET_NAME")
	if targetName == "" {
		return nil
	}

	// Parse MOA column (1 = mechanism of action)
	moaStr := getFieldQuoted(row, colMap, "MOA")
	hasMOA := moaStr == "1"

	target := &pbuf.DrugcentralTarget{
		TargetName:       targetName,
		TargetClass:      getFieldQuoted(row, colMap, "TARGET_CLASS"),
		UniprotAccession: getFieldQuoted(row, colMap, "ACCESSION"),
		GeneSymbol:       getFieldQuoted(row, colMap, "GENE"),
		SwissprotEntry:   getFieldQuoted(row, colMap, "SWISSPROT"),
		ActValue:         getFieldQuoted(row, colMap, "ACT_VALUE"),
		ActUnit:          getFieldQuoted(row, colMap, "ACT_UNIT"),
		ActType:          getFieldQuoted(row, colMap, "ACT_TYPE"),
		ActComment:       getFieldQuoted(row, colMap, "ACT_COMMENT"),
		ActSource:        getFieldQuoted(row, colMap, "ACT_SOURCE"),
		ActRelation:      getFieldQuoted(row, colMap, "RELATION"),
		HasMoa:           hasMOA,
		ActionType:       getFieldQuoted(row, colMap, "ACTION_TYPE"),
		MoaSource:        getFieldQuoted(row, colMap, "MOA_SOURCE"),
		MoaSourceUrl:     getFieldQuoted(row, colMap, "MOA_SOURCE_URL"),
		Tdl:              getFieldQuoted(row, colMap, "TDL"),
		Organism:         getFieldQuoted(row, colMap, "ORGANISM"),
	}

	return target
}

// buildEntry creates a DrugCentral attribute from aggregated drug entry
func (dc *drugcentral) buildEntry(drug *drugEntry) *pbuf.DrugcentralAttr {
	// Convert maps to slices
	actionTypes := make([]string, 0, len(drug.actionTypes))
	for k := range drug.actionTypes {
		actionTypes = append(actionTypes, k)
	}

	targetClasses := make([]string, 0, len(drug.targetClasses))
	for k := range drug.targetClasses {
		targetClasses = append(targetClasses, k)
	}

	organisms := make([]string, 0, len(drug.organisms))
	for k := range drug.organisms {
		organisms = append(organisms, k)
	}

	return &pbuf.DrugcentralAttr{
		StructId:      drug.structID,
		DrugName:      drug.drugName,
		InnName:       drug.innName,
		CasRn:         drug.casRN,
		Smiles:        drug.smiles,
		Inchi:         drug.inchi,
		InchiKey:      drug.inchiKey,
		Targets:       drug.targets,
		TargetCount:   int32(len(drug.targets)),
		ActionTypes:   actionTypes,
		TargetClasses: targetClasses,
		Organisms:     organisms,
	}
}

// saveEntry creates and saves a DrugCentral entry
func (dc *drugcentral) saveEntry(structID string, attr *pbuf.DrugcentralAttr, sourceID string, drug *drugEntry) {
	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	dc.check(err, fmt.Sprintf("marshaling DrugCentral attributes for %s", structID))

	// Save entry with unique key
	dc.d.addProp3(structID, sourceID, attrBytes)

	// Create cross-references
	dc.createCrossReferences(structID, attr, sourceID, drug)
}

// createCrossReferences builds all cross-references for a DrugCentral entry
func (dc *drugcentral) createCrossReferences(structID string, attr *pbuf.DrugcentralAttr, sourceID string, drug *drugEntry) {
	// Text search: STRUCT_ID searchable
	dc.d.addXref(structID, textLinkID, structID, dc.source, true)

	// Text search: drug name searchable
	if attr.DrugName != "" {
		dc.d.addXref(attr.DrugName, textLinkID, structID, dc.source, true)
	}

	// Text search: INN name searchable
	if attr.InnName != "" && attr.InnName != attr.DrugName {
		dc.d.addXref(attr.InnName, textLinkID, structID, dc.source, true)
	}

	// Text search: CAS RN searchable
	if attr.CasRn != "" {
		dc.d.addXref(attr.CasRn, textLinkID, structID, dc.source, true)
	}

	// Text search: InChI Key searchable (useful for structure search)
	if attr.InchiKey != "" {
		dc.d.addXref(attr.InchiKey, textLinkID, structID, dc.source, true)
	}

	// Bidirectional cross-reference: DrugCentral ↔ UniProt
	if _, exists := config.Dataconf["uniprot"]; exists {
		for uniprotID := range drug.uniprotIDs {
			if uniprotID != "" {
				dc.d.addXref(structID, sourceID, uniprotID, "uniprot", false)
			}
		}
	}

	// Text search: target names
	for _, target := range attr.Targets {
		if target.TargetName != "" {
			dc.d.addXref(target.TargetName, textLinkID, structID, dc.source, true)
		}
		// Text search: gene symbols
		if target.GeneSymbol != "" {
			dc.d.addXref(target.GeneSymbol, textLinkID, structID, dc.source, true)
		}
	}
}
