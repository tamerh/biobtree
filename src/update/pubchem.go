package update

import (
	"biobtree/db"
	"biobtree/pbuf"
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	"github.com/tamerh/jsparser"
)

// pubchem processes PubChem Compound database with biotech-focused subset
// Priorities:
//   P0: FDA-approved drugs (~3K compounds)
//   P1: Literature-referenced compounds (~8M compounds)
//   P2: Patent-associated compounds (SureChEMBL filtered, ~6M compounds)
//   P3: Bioassay-tested active compounds (~variable)
//   P4: Biologics - peptides, proteins, nucleotides (~2.5M compounds)
type pubchem struct {
	source string
	d      *DataUpdate

	// Master list of biotech-relevant CIDs (union of all sources)
	biotechCIDs map[string]bool // All biotech-relevant CIDs (10-11M target)

	// Priority tracking for biotech-relevant CIDs
	p0CIDs map[string]bool // FDA-approved drugs (~3K)
	p1CIDs map[string]bool // Literature-referenced (~8M)
	p2CIDs map[string]bool // Patent-associated (SureChEMBL filtered, ~6M)
	p3CIDs map[string]bool // Bioassay-tested active (~variable)
	p4CIDs map[string]bool // Biologics - peptides, proteins, nucleotides (~2.5M)

	// Source tracking for filtering
	surechemblPatentIDs map[string]bool // Pre-loaded SureChEMBL patent IDs (~665K)

	// Cached data for efficient lookup
	cidToTitle     map[string]string // CID → compound name (from Drug-Names)
	cidToSynonyms  map[string][]string // CID → synonyms (top 20)
	cidToMeSH      map[string][]string // CID → MeSH terms
	cidToPMIDs     map[string][]int64  // CID → PubMed IDs (top 10)
	cidToPatents   map[string][]string // CID → Patent IDs (top 10)
	cidToCreationDate map[string]string  // CID → creation date (YYYY-MM-DD)
	cidToParent    map[string]string    // CID → parent CID
	meshToPharmActions map[string][]string // MeSH term → pharmacological actions
	// NOTE: cidToActivities removed - activities are now a separate dataset

	// Cross-reference mappings
	chebiToPubChem   map[string][]string // ChEBI ID → PubChem CIDs
	hmdbToPubChem    map[string][]string // HMDB ID → PubChem CIDs
	chemblToPubChem  map[string][]string // ChEMBL ID → PubChem CIDs

	// Progress tracking
	previous  int64
	totalRead int
	totalCIDs int

	// Temp biobtree database for biotech CID filtering
	tempCIDFile       string     // File containing biotech CIDs (one per line)
	tempDBDir         string     // Temp biobtree database directory
	tempLookupEnv     db.Env     // Temp database environment (read-only)
	tempLookupDbi     db.DBI     // Temp database DBI
	hasTempLookup     bool       // Whether temp lookup database is available
	tempCIDFileHandle *os.File   // File handle for writing CIDs
}

// check provides context-aware error checking for PubChem processor
func (p *pubchem) check(err error, operation string) {
	checkWithContext(err, p.source, operation)
}

// parseFloat converts string to float64, returns 0.0 if conversion fails
func (p *pubchem) parseFloat(s string) float64 {
	if s == "" {
		return 0.0
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0
	}
	return val
}

// parseInt32 converts string to int32, returns 0 if conversion fails
func (p *pubchem) parseInt32(s string) int32 {
	if s == "" {
		return 0
	}
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(val)
}

// update is the main entry point for PubChem processing
func (p *pubchem) update() {
	defer p.d.wg.Done()

	log.Printf("[PubChem] Starting biotech-focused subset integration")
	log.Printf("[PubChem] Phase 1: Identifying biotech-relevant CIDs (P0: FDA drugs)")

	// Initialize maps
	p.biotechCIDs = make(map[string]bool)
	p.p0CIDs = make(map[string]bool)
	p.p1CIDs = make(map[string]bool)
	p.p2CIDs = make(map[string]bool)
	p.p3CIDs = make(map[string]bool)
	p.p4CIDs = make(map[string]bool)
	p.surechemblPatentIDs = make(map[string]bool)
	p.cidToTitle = make(map[string]string)
	p.cidToSynonyms = make(map[string][]string)
	p.cidToMeSH = make(map[string][]string)
	p.cidToPMIDs = make(map[string][]int64)
	p.cidToPatents = make(map[string][]string)
	p.cidToCreationDate = make(map[string]string)
	p.cidToParent = make(map[string]string)
	p.meshToPharmActions = make(map[string][]string)
	p.chebiToPubChem = make(map[string][]string)
	p.hmdbToPubChem = make(map[string][]string)
	p.chemblToPubChem = make(map[string][]string)

	// Identify biotech-relevant CIDs first, then fetch their data
	p.identifyBiotechCIDs()

	log.Printf("[PubChem] Identified %d biotech-relevant CIDs", p.totalCIDs)
	log.Printf("[PubChem]   P0 (FDA drugs): %d", len(p.p0CIDs))
	log.Printf("[PubChem]   P1 (Literature): %d", len(p.p1CIDs))
	log.Printf("[PubChem]   P2 (Patents): %d", len(p.p2CIDs))
	log.Printf("[PubChem]   P3 (Bioassay-tested): %d", len(p.p3CIDs))
	log.Printf("[PubChem]   P4 (Biologics): %d", len(p.p4CIDs))

	// Phase 2: Load supplementary data (synonyms, MeSH, PMIDs, patents, dates, parents, pharm actions, bioactivities)
	p.loadSupplementaryMappings()

	// Phase 3: Parse SDF files and create entries immediately
	log.Printf("[PubChem] Phase 3: Parsing SDF files and creating database entries")
	p.parseSDFFiles()

	// Phase 4: Create bidirectional cross-references with ChEBI, HMDB, ChEMBL
	log.Printf("[PubChem] Phase 4: Creating bidirectional cross-references")
	p.createBidirectionalXrefs()

	// Cleanup temp lookup database and files
	defer p.closeTempLookupDB()
	// TEMPORARY: Disable cleanup for debugging
	// defer p.cleanupTempFiles()

	// Signal completion
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}

	// Log completion (note: biotechCIDs map may be nil if using temp DB)
	if p.hasTempLookup {
		log.Printf("[PubChem] Integration complete: Biotech CIDs indexed in temp database")
	} else {
		log.Printf("[PubChem] Integration complete: %d biotech CIDs identified", len(p.biotechCIDs))
	}
}

// initTempCIDFile initializes a file to collect biotech CIDs (one per line)
func (p *pubchem) initTempCIDFile() error {
	outDir := config.Appconf["outDir"]
	p.tempCIDFile = filepath.Join(outDir, "temp_biotech_cids.txt")
	p.tempDBDir = filepath.Join(outDir, "temp_biotech_db")

	log.Printf("[PubChem] Initializing temp CID file: %s", p.tempCIDFile)

	// Remove existing file if present
	os.Remove(p.tempCIDFile)

	// Create file
	file, err := os.Create(p.tempCIDFile)
	if err != nil {
		return fmt.Errorf("failed to create temp CID file: %v", err)
	}

	p.tempCIDFileHandle = file
	return nil
}

// writeCIDToFile writes a biotech CID to the temp file
func (p *pubchem) writeCIDToFile(cid string) error {
	if p.tempCIDFileHandle == nil {
		return fmt.Errorf("temp CID file not initialized")
	}

	_, err := fmt.Fprintln(p.tempCIDFileHandle, cid)
	return err
}

// closeTempCIDFile closes the temp CID file
func (p *pubchem) closeTempCIDFile() error {
	if p.tempCIDFileHandle != nil {
		return p.tempCIDFileHandle.Close()
	}
	return nil
}

// buildTempBiotreeDB calls biobtree to build a database from the temp CID file
func (p *pubchem) buildTempBiotreeDB() error {
	log.Printf("[PubChem] Building temp biobtree database from %s", p.tempCIDFile)
	log.Printf("[PubChem] This may take 5-10 minutes for ~10M CIDs...")

	// Remove existing temp database
	os.RemoveAll(p.tempDBDir)

	// Build biobtree command:
	// ./biobtree -d my_data --my_data.file=temp_biotech_cids.txt --out-dir=temp_biotech_db build
	biobtreeBinary := "./biobtree"

	args := []string{
		"-d", "my_data",
		"--my_data.file=" + p.tempCIDFile,
		"--out-dir=" + p.tempDBDir,
		"--keep",
		"build",
	}

	log.Printf("[PubChem] Running: %s %s", biobtreeBinary, strings.Join(args, " "))

	// Check if CID file exists and has content
	if stat, err := os.Stat(p.tempCIDFile); err == nil {
		log.Printf("[PubChem] Temp CID file exists: %s (%d bytes)", p.tempCIDFile, stat.Size())
	} else {
		log.Printf("[PubChem] WARNING: Temp CID file not found: %v", err)
	}

	cmd := exec.Command(biobtreeBinary, args...)

	// Capture stdout and stderr
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startTime := time.Now()
	err := cmd.Run()
	elapsed := time.Since(startTime)

	// Log biobtree output
	log.Printf("[PubChem] Biobtree STDOUT:\n%s", stdout.String())
	if stderr.String() != "" {
		log.Printf("[PubChem] Biobtree STDERR:\n%s", stderr.String())
	}

	if err != nil {
		return fmt.Errorf("biobtree build failed: %v", err)
	}

	log.Printf("[PubChem] Temp database built successfully in %v", elapsed)
	return nil
}

// openTempLookupDB opens the temp biobtree database for read-only lookups
func (p *pubchem) openTempLookupDB() error {
	log.Printf("[PubChem] Opening temp biobtree database for lookups...")

	// Check if database exists
	metaFile := filepath.Join(p.tempDBDir, "db", "db.meta.json")
	if _, err := os.Stat(metaFile); os.IsNotExist(err) {
		return fmt.Errorf("temp database meta file not found: %s", metaFile)
	}

	// Read meta file to get totalKVLine
	meta := make(map[string]interface{})
	metaData, err := ioutil.ReadFile(metaFile)
	if err != nil {
		return fmt.Errorf("failed to read meta file: %v", err)
	}

	if err := json.Unmarshal(metaData, &meta); err != nil {
		return fmt.Errorf("failed to parse meta file: %v", err)
	}

	totalKV := int64(meta["totalKVLine"].(float64))

	// Open database in read-only mode
	dbDir := filepath.Join(p.tempDBDir, "db")
	tempConf := make(map[string]string)
	tempConf["dbDir"] = dbDir
	tempConf["dbBackend"] = "lmdb"

	db1 := db.DB{}
	p.tempLookupEnv, p.tempLookupDbi = db1.OpenDBNew(false, totalKV, tempConf)
	p.hasTempLookup = true

	log.Printf("[PubChem] Temp lookup database opened successfully (%d entries)", totalKV)
	return nil
}

// isBiotechCID checks if a CID exists in the temp biobtree database
func (p *pubchem) isBiotechCID(cid string) bool {
	if !p.hasTempLookup {
		// Fallback to map if temp DB not available
		return p.biotechCIDs != nil && p.biotechCIDs[cid]
	}

	// Lookup in temp database (case-insensitive, uppercase)
	cidUpper := strings.ToUpper(cid)
	var exists bool

	err := p.tempLookupEnv.View(func(txn db.Txn) error {
		v, err := txn.Get(p.tempLookupDbi, []byte(cidUpper))
		if db.IsNotFound(err) {
			exists = false
			return nil
		}
		if err != nil {
			return err
		}
		exists = len(v) > 0
		return nil
	})

	if err != nil {
		// On error, assume not biotech
		return false
	}

	return exists
}

// closeTempLookupDB closes the temp biobtree database
func (p *pubchem) closeTempLookupDB() {
	if p.hasTempLookup {
		p.tempLookupEnv.Close()
		p.hasTempLookup = false
	}
}

// cleanupTempFiles removes temporary files and database
func (p *pubchem) cleanupTempFiles() {
	log.Printf("[PubChem] Cleaning up temporary files...")

	// Remove CID file
	if p.tempCIDFile != "" {
		os.Remove(p.tempCIDFile)
	}

	// Remove temp database
	if p.tempDBDir != "" {
		os.RemoveAll(p.tempDBDir)
	}

	log.Printf("[PubChem] Cleanup complete")
}

// preloadSureChEMBLPatents loads all SureChEMBL patent IDs from patents.json into memory for fast filtering
func (p *pubchem) preloadSureChEMBLPatents() {
	// Get patent data path from config
	patentPath, ok := config.Dataconf["patent"]["path"]
	if !ok {
		log.Printf("[PubChem] Patent dataset path not configured - skipping SureChEMBL filtering")
		return
	}

	patentsFile := filepath.Join(patentPath, "patents.json")

	// Check if file exists
	if _, err := os.Stat(patentsFile); os.IsNotExist(err) {
		log.Printf("[PubChem] SureChEMBL patents.json not found at %s - skipping patent filtering", patentsFile)
		return
	}

	log.Printf("[PubChem] Pre-loading SureChEMBL patent IDs from %s...", patentsFile)
	startTime := time.Now()

	file, err := os.Open(patentsFile)
	if err != nil {
		log.Printf("[PubChem] WARNING: Could not open %s: %v", patentsFile, err)
		return
	}
	defer file.Close()

	// Use jsparser to stream through JSON (same as patents.go)
	br := bufio.NewReader(file)
	parser := jsparser.NewJSONParser(br, "patents")

	count := 0
	for j := range parser.Stream() {
		// Extract patent_number field (same as patents.go)
		patentNumber := getString(j, "patent_number")
		if patentNumber == "" {
			continue
		}

		// Store patent ID in map for fast lookups
		p.surechemblPatentIDs[patentNumber] = true
		count++

		// Progress reporting every 100K patents
		if count%100000 == 0 {
			log.Printf("[PubChem]   Loaded %dK SureChEMBL patent IDs...", count/1000)
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("[PubChem] Pre-loaded %d SureChEMBL patent IDs in %v (%.0f MB in memory)",
		count, elapsed, float64(count*50)/1024/1024) // Rough estimate: ~50 bytes per patent ID
}

// identifyBiotechCIDs identifies biotech-relevant CIDs from various sources
func (p *pubchem) identifyBiotechCIDs() {
	// Initialize temp CID file for writing biotech CIDs
	if err := p.initTempCIDFile(); err != nil {
		log.Printf("[PubChem] ERROR: Failed to initialize temp CID file: %v", err)
		log.Printf("[PubChem] Falling back to in-memory approach")
	} else {
		defer p.closeTempCIDFile()
	}

	// Step 0: Pre-load SureChEMBL patent IDs for fast filtering
	p.preloadSureChEMBLPatents()

	// Step 1: Load FDA-approved drugs from Drug-Names.tsv.gz
	p.loadFDADrugs()

	// Step 2: Load literature-referenced compounds from CID-PMID.gz
	log.Printf("[PubChem] Step 2: Loading literature-referenced compounds")
	p.loadLiteratureCIDs()

	// Step 3: Load patent-associated compounds from CID-Patent.gz
	log.Printf("[PubChem] Step 3: Loading patent-associated compounds (filtered by SureChEMBL)")
	p.loadPatentCIDs()

	// Step 4: Load bioassay-tested compounds from bioactivities.tsv.gz
	log.Printf("[PubChem] Step 4: Loading bioassay-tested compounds")
	p.loadBioassayCIDs()

	// Step 4.5: Load biologics (peptides, proteins, nucleotides) from CID-Biologics.tsv.gz
	log.Printf("[PubChem] Step 4.5: Loading biologics (peptides/proteins/nucleotides)")
	p.loadBiologicCIDs()

	// Step 5: Merge all sources into biotechCIDs master list
	log.Printf("[PubChem] Step 5: Merging and deduplicating CID sources")
	p.mergeBiotechCIDs()

	// Step 6: Build temp biobtree database from CID file
	log.Printf("[PubChem] Step 6: Building temp biobtree database for efficient filtering")
	p.closeTempCIDFile() // Close file before building database

	if err := p.buildTempBiotreeDB(); err != nil {
		log.Printf("[PubChem] ERROR: Failed to build temp database: %v", err)
		log.Printf("[PubChem] Continuing with in-memory maps")
	} else {
		// Open temp database for lookups
		if err := p.openTempLookupDB(); err != nil {
			log.Printf("[PubChem] ERROR: Failed to open temp lookup database: %v", err)
		} else {
			log.Printf("[PubChem] Temp database ready for lookups")
			// NOTE: We'll free maps AFTER XML parsing, because we need to iterate over CIDs
			// to determine which XML files to download
		}
	}
}

// REMOVED testTempDatabaseLookups() - will test with real XML parsing instead

// testTempDatabaseLookups tests that the temp database is working correctly
func (p *pubchem) testTempDatabaseLookups() {
	log.Printf("[PubChem] Running temp database lookup tests...")

	// Get a few sample CIDs from our in-memory map (before we free it)
	testCIDs := []string{}
	count := 0
	for cid := range p.biotechCIDs {
		testCIDs = append(testCIDs, cid)
		count++
		if count >= 10 {
			break
		}
	}

	if len(testCIDs) == 0 {
		log.Printf("[PubChem] WARNING: No CIDs available for testing")
		return
	}

	// Test lookups
	passed := 0
	failed := 0
	for _, cid := range testCIDs {
		exists := p.isBiotechCID(cid)
		if exists {
			passed++
		} else {
			failed++
			log.Printf("[PubChem] ERROR: Lookup failed for CID %s (should exist)", cid)
		}
	}

	// Test a CID that definitely doesn't exist
	nonexistentCID := "999999999999"
	exists := p.isBiotechCID(nonexistentCID)
	if !exists {
		passed++
	} else {
		failed++
		log.Printf("[PubChem] ERROR: Lookup returned true for non-existent CID %s", nonexistentCID)
	}

	log.Printf("[PubChem] Temp database tests: %d passed, %d failed", passed, failed)
	if failed > 0 {
		log.Printf("[PubChem] WARNING: Temp database lookups are not working correctly!")
	} else {
		log.Printf("[PubChem] Temp database lookups working correctly ✓")
	}
}

// loadFDADrugs loads FDA-approved drug CIDs from Drug-Names.tsv.gz (Priority 0)
func (p *pubchem) loadFDADrugs() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf["pubchem"]["path"]
	drugNamesPath := config.Dataconf["pubchem"]["pathDrugNames"]
	fullPath := basePath + drugNamesPath

	log.Printf("[PubChem] Loading FDA-approved drugs from %s", fullPath)

	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, drugNamesPath)
	if err != nil {
		p.check(err, "opening Drug-Names.tsv.gz")
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Parse TSV file
	r := csv.NewReader(br)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = -1 // Variable number of fields
	r.LazyQuotes = true     // Handle malformed quotes gracefully

	lineCount := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		p.check(err, "reading Drug-Names.tsv record")

		lineCount++

		// Skip header
		if lineCount == 1 {
			continue
		}

		// Format: synonym \t Drug_name \t PubChem_CID \t URL
		if len(record) < 3 {
			continue
		}

		synonym := record[0]
		drugName := record[1]
		cid := strings.TrimSpace(record[2])

		// Skip entries with empty CID
		if cid == "" {
			continue
		}

		// Mark as P0 FDA drug
		p.p0CIDs[cid] = true

		// Store drug name as title (only if not already set)
		if _, exists := p.cidToTitle[cid]; !exists {
			p.cidToTitle[cid] = drugName
		}

		// Add text search link for synonym
		p.d.addXref(synonym, textLinkID, cid, "pubchem", true)

		// Test mode: stop early if unique CID limit reached
		if config.IsTestMode() && len(p.p0CIDs) >= config.GetTestLimit(p.source) {
			log.Printf("[PubChem] Test mode: Stopping after %d unique CIDs", len(p.p0CIDs))
			break
		}
	}

	log.Printf("[PubChem] Loaded %d FDA-approved drug CIDs from %d records", len(p.p0CIDs), lineCount)
}

// loadLiteratureCIDs loads CIDs with literature references from CID-PMID.gz
// This identifies ~8M biotech-relevant compounds that have been published
func (p *pubchem) loadLiteratureCIDs() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	pmidPath := config.Dataconf[p.source]["pathCIDPMID"]

	log.Printf("[PubChem] Loading literature-referenced CIDs from %s", pmidPath)

	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, pmidPath)
	if err != nil {
		p.check(err, "opening CID-PMID.gz")
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Parse TSV file: CID \t PMID
	r := csv.NewReader(br)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = -1 // Variable fields (some lines may have extra columns)
	r.LazyQuotes = true

	lineCount := 0
	uniqueCIDs := make(map[string]bool)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed lines
			continue
		}

		lineCount++

		// Need at least 2 fields: CID \t PMID
		if len(record) < 2 {
			continue
		}

		// Format: CID \t PMID
		cid := strings.TrimSpace(record[0])
		// pmid := strings.TrimSpace(record[1]) // We'll use this in Phase 3

		if cid == "" {
			continue
		}

		// Mark as literature-referenced (P1)
		p.p1CIDs[cid] = true
		uniqueCIDs[cid] = true

		// Progress reporting every 10M lines
		if lineCount%10000000 == 0 {
			log.Printf("[PubChem] Processed %dM literature links, found %dM unique CIDs",
				lineCount/1000000, len(uniqueCIDs)/1000000)
		}

		// Test mode: stop early
		if config.IsTestMode() && len(uniqueCIDs) >= config.GetTestLimit(p.source)*10 {
			log.Printf("[PubChem] Test mode: Stopping after %d unique literature CIDs", len(uniqueCIDs))
			break
		}
	}

	log.Printf("[PubChem] Loaded %d literature-referenced CIDs (P1) from %d total links",
		len(p.p1CIDs), lineCount)
}

// loadPatentCIDs loads CIDs with patent associations from CID-Patent.gz
// This identifies ~6M biotech-relevant compounds that appear in patents
func (p *pubchem) loadPatentCIDs() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	patentPath := config.Dataconf[p.source]["pathCIDPatent"]

	log.Printf("[PubChem] Loading patent-associated CIDs from %s", patentPath)

	// Check if SureChEMBL patents were preloaded
	if len(p.surechemblPatentIDs) == 0 {
		log.Printf("[PubChem] WARNING: No SureChEMBL patents loaded - skipping patent filtering")
		log.Printf("[PubChem] (Would result in 38M non-biotech CIDs - not recommended)")
		return
	}
	log.Printf("[PubChem] Will filter %d SureChEMBL biotech patents", len(p.surechemblPatentIDs))

	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, patentPath)
	if err != nil {
		p.check(err, "opening CID-Patent.gz")
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Parse TSV file: CID \t Patent-ID
	r := csv.NewReader(br)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = -1 // Variable fields (some lines may have extra columns)
	r.LazyQuotes = true

	lineCount := 0
	uniqueCIDs := make(map[string]bool)
	validPatentCount := 0
	invalidPatentCount := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed lines
			continue
		}

		lineCount++

		// Need at least 2 fields: CID \t Patent-ID
		if len(record) < 2 {
			continue
		}

		// Format: CID \t Patent-ID
		cid := strings.TrimSpace(record[0])
		patentID := strings.TrimSpace(record[1])

		if cid == "" || patentID == "" {
			continue
		}

		// Filter: Only count patents that exist in preloaded SureChEMBL list (biotech-relevant)
		if !p.surechemblPatentIDs[patentID] {
			invalidPatentCount++
			continue // Skip non-biotech patents
		}
		validPatentCount++

		// Mark as patent-associated (P2)
		p.p2CIDs[cid] = true
		uniqueCIDs[cid] = true

		// Progress reporting every 10M lines (records processed)
		if lineCount%10000000 == 0 {
			log.Printf("[PubChem] Processed %dM patent records, found %dM unique CIDs (%d valid biotech patents, %d filtered)",
				lineCount/1000000, len(uniqueCIDs)/1000000, validPatentCount, invalidPatentCount)
		}

		// Test mode: stop early
		if config.IsTestMode() && len(uniqueCIDs) >= config.GetTestLimit(p.source)*10 {
			log.Printf("[PubChem] Test mode: Stopping after %d unique patent CIDs", len(uniqueCIDs))
			break
		}
	}

	log.Printf("[PubChem] Loaded %d patent-associated CIDs (P2) from %d total records (filtered by %d SureChEMBL patents)",
		len(p.p2CIDs), lineCount, len(p.surechemblPatentIDs))
	log.Printf("[PubChem] Patent filtering: %d biotech patent records matched, %d non-biotech filtered out (%.1f%% filtered)",
		validPatentCount, invalidPatentCount, 100.0*float64(invalidPatentCount)/float64(lineCount))
}

// mergeBiotechCIDs merges all biotech-relevant CID sources into master list
// Combines: FDA drugs (P0), literature, patents, and bioassays (when available)
func (p *pubchem) mergeBiotechCIDs() {
	log.Printf("[PubChem] Merging biotech-relevant CIDs from all sources")

	// Helper function to add CID and write to temp file
	addCID := func(cid string) {
		if !p.biotechCIDs[cid] {
			p.biotechCIDs[cid] = true
			// Write to temp file if available
			if p.tempCIDFileHandle != nil {
				p.writeCIDToFile(cid)
			}
		}
	}

	// Start with FDA drugs (Priority 0)
	for cid := range p.p0CIDs {
		addCID(cid)
	}

	// Add literature-referenced compounds
	for cid := range p.p1CIDs {
		addCID(cid)
	}

	// Add patent-associated compounds
	for cid := range p.p2CIDs {
		addCID(cid)
	}

	// Add bioassay-tested compounds (will be populated in Phase 4)
	for cid := range p.p3CIDs {
		addCID(cid)
	}

	// Add biologics (peptides, proteins, nucleotides)
	for cid := range p.p4CIDs {
		addCID(cid)
	}

	// Calculate overlap statistics
	fdaOnly := 0
	litOnly := 0
	patentOnly := 0
	multiSource := 0

	for cid := range p.biotechCIDs {
		sources := 0
		if p.p0CIDs[cid] {
			sources++
		}
		if p.p1CIDs[cid] {
			sources++
		}
		if p.p2CIDs[cid] {
			sources++
		}

		if sources > 1 {
			multiSource++
		} else {
			if p.p0CIDs[cid] {
				fdaOnly++
			} else if p.p1CIDs[cid] {
				litOnly++
			} else if p.p2CIDs[cid] {
				patentOnly++
			}
		}
	}

	log.Printf("[PubChem] Biotech CID Merge Summary:")
	log.Printf("[PubChem]   Total unique CIDs: %d", len(p.biotechCIDs))
	log.Printf("[PubChem]   Source breakdown:")
	log.Printf("[PubChem]     - FDA drugs only: %d", fdaOnly)
	log.Printf("[PubChem]     - Literature only: %d", litOnly)
	log.Printf("[PubChem]     - Patent only: %d", patentOnly)
	log.Printf("[PubChem]     - Multi-source (overlap): %d (%.1f%%)",
		multiSource, 100.0*float64(multiSource)/float64(len(p.biotechCIDs)))
	log.Printf("[PubChem]   Source sizes:")
	log.Printf("[PubChem]     - P0 FDA drugs: %d", len(p.p0CIDs))
	log.Printf("[PubChem]     - P1 Literature CIDs: %d", len(p.p1CIDs))
	log.Printf("[PubChem]     - P2 Patent CIDs: %d", len(p.p2CIDs))
	log.Printf("[PubChem]     - P3 Bioassay CIDs: %d", len(p.p3CIDs))
	log.Printf("[PubChem]     - P4 Biologic CIDs: %d", len(p.p4CIDs))

	// Update total count
	p.totalCIDs = len(p.biotechCIDs)
}

// loadBioassayCIDs loads CIDs with bioassay data from bioactivities.tsv.gz
// This identifies compounds that have been tested in PubChem BioAssays
// Phase 1: Extract CIDs only for filtering (count AIDs per CID)
// Phase 4: Will parse full activity data (IC50, targets, etc.)
func (p *pubchem) loadBioassayCIDs() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	bioactivitiesPath := config.Dataconf[p.source]["pathBioactivities"]

	log.Printf("[PubChem] Loading bioassay-tested CIDs from %s", bioactivitiesPath)
	log.Printf("[PubChem] Note: This file is 2.97 GB compressed, ~295M records - may take 30-60 minutes")

	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, bioactivitiesPath)
	if err != nil {
		p.check(err, "opening bioactivities.tsv.gz")
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Parse TSV file: AID \t SID \t SID Group \t CID \t Activity Outcome \t ...
	r := csv.NewReader(br)
	r.Comma = '\t'
	r.Comment = '#'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1 // Variable number of fields (some records may have empty trailing fields)

	lineCount := 0
	uniqueCIDs := make(map[string]bool)
	aidSet := make(map[string]map[string]bool) // CID → set of AIDs

	// Skip header
	_, err = r.Read()
	if err != nil {
		p.check(err, "reading bioactivities header")
		return
	}

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed lines
			continue
		}

		lineCount++

		// Need at least 5 columns: AID, SID, SID Group, CID, Activity Outcome
		if len(record) < 5 {
			continue
		}

		// Format: AID \t SID \t SID Group \t CID \t Activity Outcome \t ...
		aid := strings.TrimSpace(record[0])   // Column 0: AID
		cid := strings.TrimSpace(record[3])   // Column 3: CID

		if cid == "" || aid == "" {
			continue
		}

		// Track unique CIDs and their AIDs
		uniqueCIDs[cid] = true

		// Initialize AID set for this CID if needed
		if aidSet[cid] == nil {
			aidSet[cid] = make(map[string]bool)
		}
		aidSet[cid][aid] = true

		// Progress reporting every 10M lines
		if lineCount%10000000 == 0 {
			log.Printf("[PubChem] Processed %dM bioactivity records, found %dM unique CIDs",
				lineCount/1000000, len(uniqueCIDs)/1000000)
		}

		// Test mode: stop early
		if config.IsTestMode() && len(uniqueCIDs) >= config.GetTestLimit(p.source)*10 {
			log.Printf("[PubChem] Test mode: Stopping after %d unique bioassay CIDs", len(uniqueCIDs))
			break
		}
	}

	// Mark bioassay-tested CIDs (P3)
	for cid := range aidSet {
		p.p3CIDs[cid] = true
	}

	log.Printf("[PubChem] Loaded %d bioassay-tested CIDs (P3) from %d total activity records",
		len(p.p3CIDs), lineCount)
	log.Printf("[PubChem] Average %.1f assays per compound",
		float64(lineCount)/float64(len(uniqueCIDs)))
}

// countRange counts how many CIDs fall within a range of assay counts
func countRange(counts map[int]int, min, max int) int {
	total := 0
	for assayCount, cidCount := range counts {
		if assayCount >= min && assayCount <= max {
			total += cidCount
		}
	}
	return total
}

// loadBiologicCIDs loads biologics (peptides, proteins, nucleotides) from CID-Biologics.tsv.gz
// Format: CID \t Type \t Name
// Types: 1=peptide notation, 2=helm notation, 3=nucleotide code, 5=peptide FASTA, 6=common name
func (p *pubchem) loadBiologicCIDs() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	biologicsPath := config.Dataconf[p.source]["pathCIDBiologics"]

	log.Printf("[PubChem] Loading biologics (peptides/proteins/nucleotides) from %s", biologicsPath)

	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, biologicsPath)
	if err != nil {
		p.check(err, "opening CID-Biologics.tsv.gz")
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Parse TSV file: CID \t Type \t Name
	r := csv.NewReader(br)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = -1 // Variable fields
	r.LazyQuotes = true

	lineCount := 0
	uniqueCIDs := make(map[string]bool)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed lines
			continue
		}

		lineCount++

		// Need at least 2 columns: CID and Type
		if len(record) < 2 {
			continue
		}

		cid := strings.TrimSpace(record[0])
		if cid == "" {
			continue
		}

		// Mark as P4 biologic
		if !p.p4CIDs[cid] {
			p.p4CIDs[cid] = true
			uniqueCIDs[cid] = true

			// Write to temp CID file if available
			if p.tempCIDFileHandle != nil {
				p.writeCIDToFile(cid)
			}
		}

		// Progress reporting every 1M lines
		if lineCount%1000000 == 0 {
			log.Printf("[PubChem] Processed %dM biologic records, found %dK unique CIDs",
				lineCount/1000000, len(uniqueCIDs)/1000)
		}

		// Test mode: stop early
		if config.IsTestMode() && len(uniqueCIDs) >= config.GetTestLimit(p.source)*10 {
			log.Printf("[PubChem] Test mode: Stopping after %d unique biologic CIDs", len(uniqueCIDs))
			break
		}
	}

	log.Printf("[PubChem] Loaded %d biologic CIDs (peptides/proteins/nucleotides) from %d records",
		len(uniqueCIDs), lineCount)
}

// loadSynonyms parses CID-Synonym-filtered.gz and stores top 20 synonyms per biotech CID
// File format: CID \t Synonym (one synonym per line, multiple lines per CID)
// Filters out technical identifiers to prefer human-readable names
func (p *pubchem) loadSynonyms() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	synonymPath := config.Dataconf[p.source]["pathCIDSynonym"]

	log.Printf("[PubChem] Loading synonyms from %s (929 MB compressed)", synonymPath)
	log.Printf("[PubChem] Note: This file contains millions of synonyms - may take 10-20 minutes")

	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, synonymPath)
	if err != nil {
		p.check(err, "opening CID-Synonym-filtered.gz")
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Use bufio.Reader instead of Scanner for better handling of large files
	// 4MB buffer for better performance on large files
	reader := bufio.NewReaderSize(br, 4*1024*1024)

	lineCount := 0
	processedCIDs := make(map[string]bool)

	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			if err == io.EOF {
				break // End of file reached successfully
			}
			log.Printf("[PubChem] Error reading synonyms: %v", err)
			break
		}

		line = strings.TrimSpace(line)

		// Skip comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Skip empty lines
		if line == "" {
			continue
		}

		lineCount++

		// Split by tab
		record := strings.Split(line, "\t")

		// Need exactly 2 fields: CID \t Synonym
		if len(record) < 2 {
			continue
		}

		// Format: CID \t Synonym
		cid := strings.TrimSpace(record[0])
		synonym := strings.TrimSpace(record[1])

		if cid == "" || synonym == "" {
			continue
		}

		// Only process biotech-relevant CIDs
		if !p.biotechCIDs[cid] {
			continue
		}

		// Track which CIDs we've seen
		processedCIDs[cid] = true

		// Filter out technical identifiers to prefer human-readable names
		// Skip: CAS numbers, DTXSID, UNII, RefChem, InChI keys, etc.
		if isTechnicalID(synonym) {
			continue
		}

		// Add synonym to list
		p.cidToSynonyms[cid] = append(p.cidToSynonyms[cid], synonym)

		// Progress reporting every 10M lines
		if lineCount%10000000 == 0 {
			log.Printf("[PubChem] Processed %dM synonym records, loaded synonyms for %dK CIDs",
				lineCount/1000000, len(processedCIDs)/1000)
		}

		// Test mode: stop early
		if config.IsTestMode() && len(processedCIDs) >= config.GetTestLimit(p.source) {
			log.Printf("[PubChem] Test mode: Stopping after loading synonyms for %d CIDs", len(processedCIDs))
			break
		}
	}

	// Calculate statistics
	totalSynonyms := 0
	for _, syns := range p.cidToSynonyms {
		totalSynonyms += len(syns)
	}

	log.Printf("[PubChem] Synonym loading complete:")
	log.Printf("[PubChem]   - Total records processed: %d", lineCount)
	log.Printf("[PubChem]   - CIDs with synonyms: %d", len(p.cidToSynonyms))
	log.Printf("[PubChem]   - Total synonyms stored: %d", totalSynonyms)
	log.Printf("[PubChem]   - Average synonyms per CID: %.1f", float64(totalSynonyms)/float64(len(p.cidToSynonyms)))
}

// isTechnicalID checks if a synonym is a technical identifier (CAS, DTXSID, etc.)
// We filter these out to prefer human-readable names
func isTechnicalID(s string) bool {
	// CAS numbers: digits with hyphens, e.g., "14992-62-2"
	if len(s) > 0 && (s[0] >= '0' && s[0] <= '9') && strings.Contains(s, "-") {
		return true
	}

	// Technical prefixes
	technicalPrefixes := []string{
		"DTXSID", "DTXCID", "UNII-", "RefChem:", "CHEMBL", "ZINC",
		"SCHEMBL", "BDBM", "AKOS", "NCGC", "CCRIS", "EINECS",
		"bmse", "CTK", "HMS", "MFCD", "NSC",
	}

	for _, prefix := range technicalPrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}

	// InChI keys: 27 character uppercase with hyphens
	if len(s) == 27 && strings.Count(s, "-") == 2 {
		return true
	}

	return false
}

// loadPMIDDetails parses CID-PMID.gz and stores top 10 PMID values per biotech CID
// Note: Phase 1 already counted PMIDs per CID; this phase stores the actual PMID values
func (p *pubchem) loadPMIDDetails() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	pmidPath := config.Dataconf[p.source]["pathCIDPMID"]

	log.Printf("[PubChem] Loading PMID details from %s (311 MB compressed)", pmidPath)
	log.Printf("[PubChem] Storing top 10 PMIDs per biotech-relevant CID")

	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, pmidPath)
	if err != nil {
		p.check(err, "opening CID-PMID.gz for details")
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Parse TSV file: CID \t PMID
	r := csv.NewReader(br)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = 2
	r.LazyQuotes = true

	lineCount := 0
	processedCIDs := make(map[string]bool)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed lines
			continue
		}

		lineCount++

		// Format: CID \t PMID
		cid := strings.TrimSpace(record[0])
		pmidStr := strings.TrimSpace(record[1])

		if cid == "" || pmidStr == "" {
			continue
		}

		// Only process biotech-relevant CIDs
		if !p.biotechCIDs[cid] {
			continue
		}

		// Track which CIDs we've seen
		processedCIDs[cid] = true

		// Parse PMID as int64
		pmid, err := strconv.ParseInt(pmidStr, 10, 64)
		if err != nil {
			continue
		}

		// Add PMID to list
		p.cidToPMIDs[cid] = append(p.cidToPMIDs[cid], pmid)

		// Progress reporting every 10M lines
		if lineCount%10000000 == 0 {
			log.Printf("[PubChem] Processed %dM PMID records, loaded PMIDs for %dK CIDs",
				lineCount/1000000, len(processedCIDs)/1000)
		}

		// Test mode: stop early
		if config.IsTestMode() && len(processedCIDs) >= config.GetTestLimit(p.source) {
			log.Printf("[PubChem] Test mode: Stopping after loading PMIDs for %d CIDs", len(processedCIDs))
			break
		}
	}

	// Calculate statistics
	totalPMIDs := 0
	for _, pmids := range p.cidToPMIDs {
		totalPMIDs += len(pmids)
	}

	log.Printf("[PubChem] PMID loading complete:")
	log.Printf("[PubChem]   - Total records processed: %d", lineCount)
	log.Printf("[PubChem]   - CIDs with PMIDs: %d", len(p.cidToPMIDs))
	log.Printf("[PubChem]   - Total PMIDs stored: %d", totalPMIDs)
	log.Printf("[PubChem]   - Average PMIDs per CID: %.1f", float64(totalPMIDs)/float64(len(p.cidToPMIDs)))
}

// loadPatentDetails parses CID-Patent.gz and stores top 10 Patent IDs per biotech CID
// Note: Phase 1 already counted patents per CID; this phase stores the actual patent IDs
func (p *pubchem) loadPatentDetails() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	patentPath := config.Dataconf[p.source]["pathCIDPatent"]

	log.Printf("[PubChem] Loading patent details from %s (4.4 GB compressed)", patentPath)
	log.Printf("[PubChem] Storing top 10 patent IDs per biotech-relevant CID")

	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, patentPath)
	if err != nil {
		p.check(err, "opening CID-Patent.gz for details")
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Parse TSV file: CID \t Patent-ID
	r := csv.NewReader(br)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = 2
	r.LazyQuotes = true

	lineCount := 0
	processedCIDs := make(map[string]bool)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed lines
			continue
		}

		lineCount++

		// Format: CID \t Patent-ID
		cid := strings.TrimSpace(record[0])
		patentID := strings.TrimSpace(record[1])

		if cid == "" || patentID == "" {
			continue
		}

		// Only process biotech-relevant CIDs
		if !p.biotechCIDs[cid] {
			continue
		}

		// Track which CIDs we've seen
		processedCIDs[cid] = true

		// Add patent ID to list
		p.cidToPatents[cid] = append(p.cidToPatents[cid], patentID)

		// Progress reporting every 10M lines
		if lineCount%10000000 == 0 {
			log.Printf("[PubChem] Processed %dM patent records, loaded patents for %dK CIDs",
				lineCount/1000000, len(processedCIDs)/1000)
		}

		// Test mode: stop early
		if config.IsTestMode() && len(processedCIDs) >= config.GetTestLimit(p.source) {
			log.Printf("[PubChem] Test mode: Stopping after loading patents for %d CIDs", len(processedCIDs))
			break
		}
	}

	// Calculate statistics
	totalPatents := 0
	for _, patents := range p.cidToPatents {
		totalPatents += len(patents)
	}

	log.Printf("[PubChem] Patent loading complete:")
	log.Printf("[PubChem]   - Total records processed: %d", lineCount)
	log.Printf("[PubChem]   - CIDs with patents: %d", len(p.cidToPatents))
	log.Printf("[PubChem]   - Total patents stored: %d", totalPatents)
	log.Printf("[PubChem]   - Average patents per CID: %.1f", float64(totalPatents)/float64(len(p.cidToPatents)))
}

// loadMeSHTerms parses CID-MeSH and stores MeSH terms per biotech CID
// MeSH (Medical Subject Headings) provides medical/biological classifications
func (p *pubchem) loadMeSHTerms() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	meshPath := config.Dataconf[p.source]["pathCIDMeSH"]

	log.Printf("[PubChem] Loading MeSH terms from %s (2.9 MB compressed)", meshPath)

	// Download and open file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, meshPath)
	if err != nil {
		p.check(err, "opening CID-MeSH")
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Parse TSV file: CID \t MeSH Term
	r := csv.NewReader(br)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = -1 // Variable fields
	r.LazyQuotes = true

	lineCount := 0
	processedCIDs := make(map[string]bool)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed lines
			continue
		}

		lineCount++

		// Need at least 2 fields
		if len(record) < 2 {
			continue
		}

		// Format: CID \t MeSH Term
		cid := strings.TrimSpace(record[0])
		meshTerm := strings.TrimSpace(record[1])

		if cid == "" || meshTerm == "" {
			continue
		}

		// Only process biotech-relevant CIDs
		if !p.biotechCIDs[cid] {
			continue
		}

		// Track which CIDs we've seen
		processedCIDs[cid] = true

		// Add MeSH term to list
		p.cidToMeSH[cid] = append(p.cidToMeSH[cid], meshTerm)

		// Test mode: stop early
		if config.IsTestMode() && len(processedCIDs) >= config.GetTestLimit(p.source) {
			log.Printf("[PubChem] Test mode: Stopping after loading MeSH for %d CIDs", len(processedCIDs))
			break
		}
	}

	// Calculate statistics
	totalMeSH := 0
	for _, terms := range p.cidToMeSH {
		totalMeSH += len(terms)
	}

	log.Printf("[PubChem] MeSH loading complete:")
	log.Printf("[PubChem]   - Total records processed: %d", lineCount)
	log.Printf("[PubChem]   - CIDs with MeSH terms: %d", len(p.cidToMeSH))
	log.Printf("[PubChem]   - Total MeSH terms stored: %d", totalMeSH)
	if len(p.cidToMeSH) > 0 {
		log.Printf("[PubChem]   - Average MeSH terms per CID: %.1f", float64(totalMeSH)/float64(len(p.cidToMeSH)))
	}
}

// determineRequiredSDFFiles analyzes biotechCIDs and returns list of SDF files needed
// SDF files are organized as: Compound_[start]_[end].sdf.gz (500K CIDs per file)
// Returns: slice of file names and map of fileName → CID count
func (p *pubchem) determineRequiredSDFFiles() ([]string, map[string]int) {
	log.Printf("[PubChem] Analyzing CID distribution to determine required SDF files")

	// Count CIDs per 500K bucket
	const bucketSize = 500000
	bucketCounts := make(map[int]int) // bucket number → CID count

	for cidStr := range p.biotechCIDs {
		cid, err := strconv.Atoi(cidStr)
		if err != nil {
			continue
		}

		// Calculate which bucket this CID belongs to (0-indexed)
		// CID 1-500000 → bucket 0
		// CID 500001-1000000 → bucket 1
		bucketNum := cid / bucketSize
		bucketCounts[bucketNum]++
	}

	log.Printf("[PubChem] CIDs span across %d different 500K-range SDF files", len(bucketCounts))

	// Generate SDF file names for buckets that have CIDs
	var sdfFiles []string
	fileCIDCounts := make(map[string]int)

	for bucketNum, count := range bucketCounts {
		// Calculate start and end CIDs for this bucket
		// Bucket 0: 000000001_000500000
		// Bucket 1: 000500001_001000000
		start := bucketNum*bucketSize + 1
		end := (bucketNum + 1) * bucketSize

		// Format with leading zeros (9 digits)
		fileName := fmt.Sprintf("Compound_%09d_%09d.sdf.gz", start, end)
		sdfFiles = append(sdfFiles, fileName)
		fileCIDCounts[fileName] = count
	}

	// Sort files by bucket number for sequential processing
	sort.Slice(sdfFiles, func(i, j int) bool {
		return sdfFiles[i] < sdfFiles[j]
	})

	log.Printf("[PubChem] SDF File Requirements:")
	log.Printf("[PubChem]   - Total SDF files needed: %d (out of 356 available)", len(sdfFiles))
	log.Printf("[PubChem]   - Estimated download savings: %.1f%% (skipping %d files)",
		100.0*float64(356-len(sdfFiles))/356.0, 356-len(sdfFiles))

	// Calculate statistics
	totalCIDs := 0
	minCIDsPerFile := 1000000
	maxCIDsPerFile := 0

	for _, count := range fileCIDCounts {
		totalCIDs += count
		if count < minCIDsPerFile {
			minCIDsPerFile = count
		}
		if count > maxCIDsPerFile {
			maxCIDsPerFile = count
		}
	}

	avgCIDsPerFile := float64(totalCIDs) / float64(len(sdfFiles))

	log.Printf("[PubChem]   - Total biotech CIDs to extract: %d", totalCIDs)
	log.Printf("[PubChem]   - Average CIDs per file: %.0f", avgCIDsPerFile)
	log.Printf("[PubChem]   - Min/Max CIDs per file: %d / %d", minCIDsPerFile, maxCIDsPerFile)

	// Show first few files for verification
	log.Printf("[PubChem] First 5 SDF files to download:")
	for i := 0; i < 5 && i < len(sdfFiles); i++ {
		log.Printf("[PubChem]   - %s (%d CIDs)", sdfFiles[i], fileCIDCounts[sdfFiles[i]])
	}

	return sdfFiles, fileCIDCounts
}

// parseSDFFiles downloads and parses SDF files containing target CIDs
// SDF files are organized in 500K CID ranges (e.g., Compound_000000001_000500000.sdf.gz)
func (p *pubchem) parseSDFFiles() {
	// Determine which SDF files we need based on CID ranges
	sdfFiles, _ := p.determineRequiredSDFFiles()

	if len(sdfFiles) == 0 {
		log.Printf("[PubChem] No SDF files to process")
		return
	}

	numWorkers := 4 // Multiple workers for parallel processing
	log.Printf("[PubChem] Processing %d SDF files with %d parallel workers", len(sdfFiles), numWorkers)

	// Create worker pool - multiple SDF parsers sending to same biobtree channel
	var wg sync.WaitGroup
	fileChan := make(chan string, len(sdfFiles))

	// Track progress
	var filesProcessed int
	var progressMutex sync.Mutex

	// Start workers - each parses SDF and sends entries to biobtree channel
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for sdfFile := range fileChan {
				progressMutex.Lock()
				filesProcessed++
				currentFile := filesProcessed
				progressMutex.Unlock()

				log.Printf("[Worker %d] Processing file %d/%d: %s", workerID, currentFile, len(sdfFiles), sdfFile)
				p.parseSDFFile(sdfFile)
			}
		}(i)
	}

	// Send files to workers
	for _, sdfFile := range sdfFiles {
		fileChan <- sdfFile
	}
	close(fileChan)

	// Wait for all workers to complete
	wg.Wait()

	log.Printf("[PubChem] SDF parsing complete: processed %d files", len(sdfFiles))
}

// parseSDFFile downloads and parses a single SDF file
func (p *pubchem) parseSDFFile(sdfFile string) {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	sdfPath := config.Dataconf[p.source]["pathSDF"]

	log.Printf("[PubChem] Parsing %s", sdfFile)

	// Download and open SDF file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, sdfPath+sdfFile)
	if err != nil {
		log.Printf("[PubChem] Warning: Could not load %s: %v", sdfFile, err)
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	// Use bufio.Reader instead of Scanner for better handling of large files
	// 4MB buffer for better performance
	reader := bufio.NewReaderSize(br, 4*1024*1024)

	matchCount := 0
	scannedCount := 0

	// Parse SDF records (each record ends with $$$$)
	var currentRecord strings.Builder
	var inRecord bool

	for {
		line, err := reader.ReadString('\n')

		if err != nil {
			if err == io.EOF {
				// Process final record if exists
				if inRecord && currentRecord.Len() > 0 {
					p.parseSDFRecord(currentRecord.String(), &matchCount, &scannedCount)
				}
				break
			}
			log.Printf("[PubChem] Error reading %s: %v", sdfFile, err)
			break
		}

		// Check for record delimiter
		if strings.TrimSpace(line) == "$$$$" {
			if inRecord {
				// Process the complete record
				p.parseSDFRecord(currentRecord.String(), &matchCount, &scannedCount)
				currentRecord.Reset()
				inRecord = false
			}
			continue
		}

		// Add line to current record
		if !inRecord {
			inRecord = true
		}
		currentRecord.WriteString(line)
	}

	log.Printf("[PubChem] Processed %s: found %d target CIDs (scanned %d compounds)", sdfFile, matchCount, scannedCount)
}

// parseSDFRecord parses a single SDF record and extracts molecular data
func (p *pubchem) parseSDFRecord(record string, matchCount *int, scannedCount *int) {
	*scannedCount++

	// Extract CID from property section
	// Format: > <PUBCHEM_COMPOUND_CID>\n12345\n
	cid := p.extractSDFProperty(record, "PUBCHEM_COMPOUND_CID")
	if cid == "" {
		return
	}

	// Only process biotech-relevant CIDs
	if !p.isBiotechRelevant(cid) {
		return
	}

	*matchCount++

	// Extract molecular properties from SDF
	smiles := p.extractSDFProperty(record, "PUBCHEM_SMILES")
	inchiKey := p.extractSDFProperty(record, "PUBCHEM_IUPAC_INCHIKEY")
	iupacName := p.extractSDFProperty(record, "PUBCHEM_IUPAC_NAME")

	molecularFormula := p.extractSDFProperty(record, "PUBCHEM_MOLECULAR_FORMULA")
	molecularWeight := p.extractSDFProperty(record, "PUBCHEM_MOLECULAR_WEIGHT")
	exactMass := p.extractSDFProperty(record, "PUBCHEM_EXACT_MASS")
	monoisotopicWeight := p.extractSDFProperty(record, "PUBCHEM_MONOISOTOPIC_WEIGHT")

	xlogp := p.extractSDFProperty(record, "PUBCHEM_XLOGP3")
	hbondDonors := p.extractSDFProperty(record, "PUBCHEM_CACTVS_HBOND_DONOR")
	hbondAcceptors := p.extractSDFProperty(record, "PUBCHEM_CACTVS_HBOND_ACCEPTOR")
	rotatableBonds := p.extractSDFProperty(record, "PUBCHEM_CACTVS_ROTATABLE_BOND")
	tpsa := p.extractSDFProperty(record, "PUBCHEM_CACTVS_TPSA")

	// Determine compound type based on priority
	compoundType := "bioactive" // default
	if p.p0CIDs[cid] {
		compoundType = "drug" // FDA-approved drugs
	} else if p.p4CIDs[cid] {
		compoundType = "biologic" // Peptides, proteins, nucleotides
	} else if p.p1CIDs[cid] {
		compoundType = "literature"
	} else if p.p2CIDs[cid] {
		compoundType = "patent"
	}

	// Get supplementary data for this CID
	// DISABLED: Temporal and parent data (saves ~1.2 GB memory)
	// creationDate := p.cidToCreationDate[cid]
	// parentCID := p.cidToParent[cid]

	// Get MeSH terms for this compound
	var meshTerms []string
	if terms, hasMeSH := p.cidToMeSH[cid]; hasMeSH {
		meshTerms = terms
	}

	// Compute pharmacological actions from MeSH terms
	var pharmActions []string
	if len(meshTerms) > 0 {
		pharmActionMap := make(map[string]bool) // dedup
		for _, meshTerm := range meshTerms {
			if actions, found := p.meshToPharmActions[meshTerm]; found {
				for _, action := range actions {
					pharmActionMap[action] = true
				}
			}
		}
		// Convert map to slice
		for action := range pharmActionMap {
			pharmActions = append(pharmActions, action)
		}
	}

	// Create PubChem entry with SDF data
	fr := config.Dataconf["pubchem"]["id"]
	attr := pbuf.PubchemAttr{
		// Core Identifiers
		Cid:                 cid,
		InchiKey:            inchiKey,
		Smiles:              smiles,
		IupacName:           iupacName,
		Title:               iupacName, // Use IUPAC name as title

		// Classifications
		CompoundType:        compoundType,
		IsFdaApproved:       p.p0CIDs[cid],
		HasLiterature:       p.p1CIDs[cid],
		HasPatents:          p.p2CIDs[cid],

		// Molecular Properties
		MolecularFormula:    molecularFormula,
		MolecularWeight:     p.parseFloat(molecularWeight),
		ExactMass:           p.parseFloat(exactMass),
		MonoisotopicWeight:  p.parseFloat(monoisotopicWeight),
		Xlogp:               p.parseFloat(xlogp),
		HydrogenBondDonors:  p.parseInt32(hbondDonors),
		HydrogenBondAcceptors: p.parseInt32(hbondAcceptors),
		RotatableBonds:      p.parseInt32(rotatableBonds),
		Tpsa:                p.parseFloat(tpsa),

		// Medical/Biological Classifications
		MeshTerms:           meshTerms,

		// Metadata
		// DISABLED: Temporal and parent data (saves ~1.2 GB memory)
		// CreationDate:         creationDate,
		// ParentCid:            parentCID,
		PharmacologicalActions: pharmActions,
	}

	// Marshal and store entry (send to biobtree processing channel)
	// Channels are thread-safe, multiple workers can write concurrently
	b, _ := ffjson.Marshal(attr)
	p.d.addProp3(cid, fr, b)

	// Add text search link for InChI Key (channel is thread-safe)
	if inchiKey != "" {
		p.d.addXref(inchiKey, textLinkID, cid, "pubchem", true)
	}

	// Create cross-references to MeSH terms
	for _, meshTerm := range meshTerms {
		if meshTerm != "" {
			p.d.addXref(cid, fr, meshTerm, "mesh", false)
		}
	}
}

// extractSDFProperty extracts a property value from SDF format
// Format: > <PROPERTY_NAME>\nvalue\n
func (p *pubchem) extractSDFProperty(record string, propertyName string) string {
	// Look for property tag: > <PROPERTY_NAME>
	tag := "> <" + propertyName + ">"

	lines := strings.Split(record, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == tag {
			// Value is on the next line
			if i+1 < len(lines) {
				value := strings.TrimSpace(lines[i+1])
				// Empty line indicates end of property
				if value != "" {
					return value
				}
			}
			break
		}
	}
	return ""
}

// loadSupplementaryMappings loads synonym, MeSH, and PMID mappings
// (Core molecular data comes from XML files)
func (p *pubchem) loadSupplementaryMappings() {
	log.Printf("[PubChem] Phase 3: Loading supplementary nomenclature data")

	// Load Synonyms (top 20 per CID, filtered for readability)
	p.loadSynonyms()

	// Load PubMed IDs (top 10 per CID)
	p.loadPMIDDetails()

	// Load Patent IDs (top 10 per CID)
	p.loadPatentDetails()

	// Load MeSH terms (medical classifications)
	p.loadMeSHTerms()

	// Load MeSH → Pharmacological Actions mapping
	p.loadMeSHPharmActions()

	// DISABLED: Load creation dates (saves ~620 MB memory for 10.7M CIDs)
	// TODO: Re-enable if users need temporal queries (when compound was added to PubChem)
	// p.loadCIDDate()

	// DISABLED: Load parent compound relationships (saves ~500 MB memory for 10.7M CIDs)
	// TODO: Re-enable if users need to group compounds by parent structure (salts, hydrates, etc.)
	// p.loadCIDParent()

	log.Printf("[PubChem] Phase 2 complete: All nomenclature data loaded")

	// NOTE: BioAssay activities are now a separate dataset (pubchem_activity)
	// This eliminates memory accumulation during compound loading
}

// loadCIDSynonyms loads CID → Synonyms mapping (filtered to top 20)
func (p *pubchem) loadCIDSynonyms() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf["pubchem"]["path"]
	synonymPath := config.Dataconf["pubchem"]["pathCIDSynonym"]

	log.Printf("[PubChem] Loading CID-Synonym mappings")

	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, synonymPath)
	if err != nil {
		log.Printf("[PubChem] Warning: Could not load CID-Synonym: %v", err)
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.FieldsPerRecord = -1

	currentCID := ""
	synonyms := []string{}
	matchCount := 0
	targetCount := p.totalCIDs

	for {
		record, err := r.Read()
		if err == io.EOF {
			// Store last CID's synonyms
			if currentCID != "" && p.isBiotechRelevant(currentCID) && len(synonyms) > 0 {
				p.cidToSynonyms[currentCID] = synonyms
				matchCount++
			}
			break
		}
		if err != nil {
			continue
		}

		if len(record) < 2 {
			continue
		}

		cid := record[0]
		synonym := record[1]

		// If new CID, store previous CID's synonyms
		if cid != currentCID {
			if currentCID != "" && p.isBiotechRelevant(currentCID) && len(synonyms) > 0 {
				p.cidToSynonyms[currentCID] = synonyms
				matchCount++

				// Early exit when all target CIDs found
				if matchCount >= targetCount {
					log.Printf("[PubChem] Found all %d target CIDs, stopping scan early", matchCount)
					break
				}
			}
			currentCID = cid
			synonyms = []string{}
		}

		// Accumulate synonyms for current CID
		if p.isBiotechRelevant(cid) && len(synonyms) < 20 {
			synonyms = append(synonyms, synonym)

			// Add text search link for synonym
			p.d.addXref(synonym, textLinkID, cid, "pubchem", true)
		}
	}

	log.Printf("[PubChem] Loaded synonyms for %d biotech-relevant CIDs", matchCount)
}

// loadCIDMeSH loads CID → MeSH terms mapping
func (p *pubchem) loadCIDMeSH() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf["pubchem"]["path"]
	meshPath := config.Dataconf["pubchem"]["pathCIDMeSH"]

	log.Printf("[PubChem] Loading CID-MeSH mappings")

	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, meshPath)
	if err != nil {
		log.Printf("[PubChem] Warning: Could not load CID-MeSH: %v", err)
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.FieldsPerRecord = -1

	currentCID := ""
	meshTerms := []string{}
	matchCount := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			// Store last CID's MeSH terms
			if currentCID != "" && p.isBiotechRelevant(currentCID) && len(meshTerms) > 0 {
				p.cidToMeSH[currentCID] = meshTerms
				matchCount++
			}
			break
		}
		if err != nil {
			continue
		}

		if len(record) < 2 {
			continue
		}

		cid := record[0]
		meshTerm := record[1]

		// If new CID, store previous CID's MeSH terms
		if cid != currentCID {
			if currentCID != "" && p.isBiotechRelevant(currentCID) && len(meshTerms) > 0 {
				p.cidToMeSH[currentCID] = meshTerms
				matchCount++
			}
			currentCID = cid
			meshTerms = []string{}
		}

		// Accumulate MeSH terms for current CID (limit to 10)
		if p.isBiotechRelevant(cid) && len(meshTerms) < 10 {
			meshTerms = append(meshTerms, meshTerm)
		}
	}

	log.Printf("[PubChem] Loaded MeSH terms for %d biotech-relevant CIDs", matchCount)
}

// loadMeSHPharmActions loads MeSH term → pharmacological actions mapping
func (p *pubchem) loadMeSHPharmActions() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf["pubchem"]["path"]
	meshPharmPath := config.Dataconf["pubchem"]["pathMeSHPharm"]

	log.Printf("[PubChem] Loading MeSH-Pharm (pharmacological actions)")

	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, meshPharmPath)
	if err != nil {
		log.Printf("[PubChem] Warning: Could not load MeSH-Pharm: %v", err)
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	lineCount := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if len(record) < 2 {
			continue
		}

		meshTerm := record[0]
		pharmActions := record[1:] // All remaining columns are pharmacological actions

		if len(pharmActions) > 0 {
			p.meshToPharmActions[meshTerm] = pharmActions
			lineCount++
		}
	}

	log.Printf("[PubChem] Loaded pharmacological actions for %d MeSH terms", lineCount)
}

// loadCIDDate loads CID → creation date mapping
func (p *pubchem) loadCIDDate() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf["pubchem"]["path"]
	datePath := config.Dataconf["pubchem"]["pathCIDDate"]

	log.Printf("[PubChem] Loading CID-Date (creation dates)")

	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, datePath)
	if err != nil {
		log.Printf("[PubChem] Warning: Could not load CID-Date: %v", err)
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	lineCount := 0
	matchCount := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		lineCount++

		// Progress tracking every 10M records
		if lineCount%10000000 == 0 {
			log.Printf("[PubChem] Processed %dM date records, matched %d biotech CIDs",
				lineCount/1000000, matchCount)
		}

		if len(record) < 2 {
			continue
		}

		cid := record[0]
		date := record[1]

		if p.isBiotechRelevant(cid) {
			p.cidToCreationDate[cid] = date
			matchCount++
		}
	}

	log.Printf("[PubChem] Loaded creation dates for %d biotech-relevant CIDs", matchCount)
}

// loadCIDParent loads CID → parent CID mapping
func (p *pubchem) loadCIDParent() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf["pubchem"]["path"]
	parentPath := config.Dataconf["pubchem"]["pathCIDParent"]

	log.Printf("[PubChem] Loading CID-Parent (parent compound relationships)")

	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, parentPath)
	if err != nil {
		log.Printf("[PubChem] Warning: Could not load CID-Parent: %v", err)
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	lineCount := 0
	matchCount := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		lineCount++

		// Progress tracking every 10M records
		if lineCount%10000000 == 0 {
			log.Printf("[PubChem] Processed %dM parent records, matched %d biotech CIDs",
				lineCount/1000000, matchCount)
		}

		if len(record) < 2 {
			continue
		}

		cid := record[0]
		parentCID := record[1]

		// Only store if CID != parent (skip self-parents)
		if p.isBiotechRelevant(cid) && cid != parentCID {
			p.cidToParent[cid] = parentCID
			matchCount++
		}
	}

	log.Printf("[PubChem] Loaded parent relationships for %d biotech-relevant CIDs", matchCount)
}

// loadCIDPMID loads CID → PubMed IDs mapping (top 10)
func (p *pubchem) loadCIDPMID() {
	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf["pubchem"]["path"]
	pmidPath := config.Dataconf["pubchem"]["pathCIDPMID"]

	log.Printf("[PubChem] Loading CID-PMID mappings")

	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, pmidPath)
	if err != nil {
		log.Printf("[PubChem] Warning: Could not load CID-PMID: %v", err)
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.FieldsPerRecord = -1

	lineCount := 0
	currentCID := ""
	pmids := []int64{}
	matchCount := 0

	for {
		record, err := r.Read()
		if err == io.EOF {
			// Store last CID's PMIDs
			if currentCID != "" && p.isBiotechRelevant(currentCID) && len(pmids) > 0 {
				p.cidToPMIDs[currentCID] = pmids
				matchCount++
			}
			break
		}
		if err != nil {
			continue
		}

		lineCount++

		// Progress tracking every 10M records
		if lineCount%10000000 == 0 {
			log.Printf("[PubChem] Processed %dM PMID records, matched %d biotech CIDs",
				lineCount/1000000, matchCount)
		}

		if len(record) < 2 {
			continue
		}

		cid := record[0]
		pmidStr := record[1]
		pmid, err := strconv.ParseInt(pmidStr, 10, 64)
		if err != nil {
			continue
		}

		// If new CID, store previous CID's PMIDs
		if cid != currentCID {
			if currentCID != "" && p.isBiotechRelevant(currentCID) && len(pmids) > 0 {
				p.cidToPMIDs[currentCID] = pmids
				matchCount++
			}
			currentCID = cid
			pmids = []int64{}
		}

		// Accumulate PMIDs for current CID (top 10)
		if p.isBiotechRelevant(cid) && len(pmids) < 10 {
			pmids = append(pmids, pmid)
		}
	}

	log.Printf("[PubChem] Loaded PMIDs for %d biotech-relevant CIDs", matchCount)
}

// createEntries creates database entries for all biotech-relevant compounds

// createBidirectionalXrefs creates bidirectional cross-references with ChEBI, HMDB, ChEMBL
func (p *pubchem) createBidirectionalXrefs() {
	// Step 1: Parse ChEBI database_accession.tsv for PubChem CIDs
	p.parseChEBIAccessions()

	// Step 2: Parse HMDB XML for pubchem_compound_id field
	// TODO: Implement HMDB cross-reference parsing
	// p.parseHMDBPubChemIDs()

	// Step 3: Parse ChEMBL for PubChem cross-references
	// TODO: Implement ChEMBL cross-reference parsing
	// p.parseChEMBLPubChemIDs()

	log.Printf("[PubChem] Bidirectional cross-references created")
	log.Printf("[PubChem]   ChEBI: %d mappings", len(p.chebiToPubChem))
	log.Printf("[PubChem]   HMDB: %d mappings", len(p.hmdbToPubChem))
	log.Printf("[PubChem]   ChEMBL: %d mappings", len(p.chemblToPubChem))
}

// parseChEBIAccessions parses ChEBI database_accession.tsv for PubChem cross-references
func (p *pubchem) parseChEBIAccessions() {
	// Check if ChEBI dataset is configured
	if _, exists := config.Dataconf["chebi"]; !exists {
		log.Printf("[PubChem] ChEBI dataset not configured, skipping cross-reference")
		return
	}

	chebiPath := config.Dataconf["chebi"]["path"]
	log.Printf("[PubChem] Parsing ChEBI database_accession.tsv for PubChem cross-references")

	br, gz, _, _, localFile, _, err := getDataReaderNew("chebi", p.d.ebiFtp, p.d.ebiFtpPath, chebiPath+"database_accession.tsv.gz")
	if err != nil {
		log.Printf("[PubChem] Warning: Could not load ChEBI database_accession.tsv: %v", err)
		return
	}
	defer func() {
		if gz != nil {
			gz.Close()
		}
		if localFile != nil {
			localFile.Close()
		}
	}()

	r := csv.NewReader(br)
	r.Comma = '\t'
	r.Comment = '#'
	r.FieldsPerRecord = -1

	matchCount := 0
	fr := config.Dataconf["pubchem"]["id"]

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if len(record) < 4 {
			continue
		}

		// Format: ID \t COMPOUND_ID \t ACCESSION_NUMBER \t SOURCE \t TYPE
		chebiID := record[1]      // ChEBI ID
		pubchemCID := record[2]   // PubChem CID
		sourceDB := record[3]     // Source database

		// Only process PubChem entries
		if sourceDB != "PubChem" && sourceDB != "pubchem" {
			continue
		}

		// Only create cross-references for biotech-relevant CIDs
		if !p.isBiotechRelevant(pubchemCID) {
			continue
		}

		// Create bidirectional links
		// PubChem → ChEBI
		p.d.addXref(pubchemCID, fr, chebiID, "chebi", false)
		// ChEBI → PubChem (will be created by ChEBI parser)

		// Track mapping
		p.chebiToPubChem[chebiID] = append(p.chebiToPubChem[chebiID], pubchemCID)
		matchCount++
	}

	log.Printf("[PubChem] Created %d PubChem ↔ ChEBI cross-references", matchCount)
}

// isBiotechRelevant checks if a CID is in any biotech-relevant priority
// Uses temp database lookup for efficiency
func (p *pubchem) isBiotechRelevant(cid string) bool {
	return p.isBiotechCID(cid)
}
