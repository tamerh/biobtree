package update

import (
	"biobtree/pbuf"
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
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

	// Cached data for efficient lookup (stored as attributes in PubchemAttr)
	cidToTitle     map[string]string   // CID → compound name (from Drug-Names)
	cidToSynonyms  map[string][]string // CID → synonyms
	cidToMeSH      map[string][]string // CID → MeSH terms
	meshToPharmActions map[string][]string // MeSH term → pharmacological actions

	// External database identifiers (extracted from synonyms, stored as attributes)
	cidToUNII    map[string]string   // CID → FDA UNII (one per compound)
	cidToDTXSID  map[string]string   // CID → EPA DSSTox ID (one per compound)
	cidToZINC    map[string][]string // CID → ZINC IDs (multiple possible)
	cidToNSC     map[string][]string // CID → NCI compound numbers (multiple possible)

	// Optional temporal/structural data (disabled by default, see loadSupplementaryMappings)
	cidToCreationDate map[string]string // CID → creation date (YYYY-MM-DD)
	cidToParent       map[string]string // CID → parent CID

	// Note: PMID and Patent data are stored as cross-references (xrefs), not attributes
	// See loadLiteratureCIDs() and loadPatentCIDs() for details

	// Cross-reference mappings
	chebiToPubChem   map[string][]string // ChEBI ID → PubChem CIDs
	hmdbToPubChem    map[string][]string // HMDB ID → PubChem CIDs
	chemblToPubChem  map[string][]string // ChEMBL ID → PubChem CIDs

	// Progress tracking
	previous  int64
	totalRead int
	totalCIDs int

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
	p.cidToUNII = make(map[string]string)
	p.cidToDTXSID = make(map[string]string)
	p.cidToZINC = make(map[string][]string)
	p.cidToNSC = make(map[string][]string)
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

	// Signal completion
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}

	log.Printf("[PubChem] Integration complete: %d biotech CIDs processed", len(p.biotechCIDs))
}

// isBiotechCID checks if a CID is in the biotech-relevant set
func (p *pubchem) isBiotechCID(cid string) bool {
	return p.biotechCIDs != nil && p.biotechCIDs[cid]
}

// preloadSureChEMBLPatents loads all SureChEMBL patent IDs from patents.json into memory for fast filtering
func (p *pubchem) preloadSureChEMBLPatents() {
	// In test mode, skip heavy patent preloading - not needed for small test runs
	if config.IsTestMode() {
		log.Printf("[PubChem] Test mode: Skipping SureChEMBL patent preloading")
		return
	}

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

	// Note: We use the in-memory biotechCIDs map (~10M entries, ~200-300MB) for filtering.
	// This is simpler and sufficient for current scale. The temp DB approach below is kept
	// commented out for future reference if we need to scale to larger datasets.
	//
	// // Step 6: Build temp biobtree database from CID file (for very large datasets)
	// log.Printf("[PubChem] Step 6: Building temp biobtree database for efficient filtering")
	// p.closeTempCIDFile() // Close file before building database
	// if err := p.buildTempBiotreeDB(); err != nil {
	//     log.Printf("[PubChem] ERROR: Failed to build temp database: %v", err)
	// } else {
	//     if err := p.openTempLookupDB(); err != nil {
	//         log.Printf("[PubChem] ERROR: Failed to open temp lookup database: %v", err)
	//     } else {
	//         log.Printf("[PubChem] Temp database ready for lookups")
	//     }
	// }
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
//
// PMID data is stored as cross-references (xrefs) rather than embedded attributes because:
// - CID-PMID.gz contains 50+ million records (311 MB compressed)
// - Storing as attributes would require loading all PMIDs into memory (~10 GB)
// - Xrefs are written directly to bucket files (disk), avoiding memory accumulation
// - Xrefs provide bidirectional queries: "compound → publications" and "publication → compounds"
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
		pmid := strings.TrimSpace(record[1])

		if cid == "" || pmid == "" {
			continue
		}

		// Mark as literature-referenced (P1)
		p.p1CIDs[cid] = true
		uniqueCIDs[cid] = true

		// Create bidirectional cross-reference: PubChem CID ↔ PubMed
		// This allows: Query compound → see publications, Query publication → see compounds
		fr := config.Dataconf["pubchem"]["id"]
		p.d.addXref(cid, fr, pmid, "pubmed", false)

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
	log.Printf("[PubChem] Created %d CID ↔ PubMed cross-references", lineCount)
}

// loadPatentCIDs loads CIDs with patent associations from CID-Patent.gz
// This identifies ~6M biotech-relevant compounds that appear in patents
//
// Patent data is stored as cross-references (xrefs) rather than embedded attributes because:
// - CID-Patent.gz contains 1+ billion records (4.4 GB compressed)
// - Storing as attributes would require loading all patent IDs into memory (~50-60 GB)
// - Xrefs are written directly to bucket files (disk), avoiding memory accumulation
// - Xrefs provide bidirectional queries: "compound → patents" and "patent → compounds"
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

		// Create bidirectional cross-reference: PubChem CID ↔ Patent
		// This allows: Query compound → see patents, Query patent → see compounds
		fr := config.Dataconf["pubchem"]["id"]
		p.d.addXref(cid, fr, patentID, "patent", false)

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
	log.Printf("[PubChem] Created %d CID ↔ Patent cross-references", validPatentCount)
}

// mergeBiotechCIDs merges all biotech-relevant CID sources into master list
// Combines: FDA drugs (P0), literature, patents, and bioassays (when available)
func (p *pubchem) mergeBiotechCIDs() {
	log.Printf("[PubChem] Merging biotech-relevant CIDs from all sources")

	// Helper function to add CID to master set
	addCID := func(cid string) {
		if !p.biotechCIDs[cid] {
			p.biotechCIDs[cid] = true
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

// loadSynonyms parses CID-Synonym-filtered.gz and extracts:
// 1. Cross-references to ChEMBL, BindingDB, CAS, and SureChEMBL (patent_compound)
// 2. Human-readable synonyms for display (filtering out technical identifiers)
// File format: CID \t Synonym (one synonym per line, multiple lines per CID)
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

	// Cross-reference counters
	chemblXrefs := 0
	schemblXrefs := 0
	bindingdbXrefs := 0
	casXrefs := 0

	// External ID counters (stored as attributes, searchable)
	uniiCount := 0
	dtxsidCount := 0
	zincCount := 0
	nscCount := 0

	// Dataset ID for xrefs
	fr := config.Dataconf[p.source]["id"]

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

		// Extract cross-references from technical synonyms BEFORE filtering
		// These create bidirectional links between PubChem and other databases

		// ChEMBL molecule IDs (e.g., "CHEMBL25" for aspirin)
		// Note: SCHEMBL is SureChEMBL (patent chemistry), handled separately
		if strings.HasPrefix(synonym, "CHEMBL") && !strings.HasPrefix(synonym, "SCHEMBL") {
			if _, exists := config.Dataconf["chembl_molecule"]; exists {
				p.d.addXref(cid, fr, synonym, "chembl_molecule", false)
				p.chemblToPubChem[synonym] = append(p.chemblToPubChem[synonym], cid)
				chemblXrefs++
			}
			continue // Don't add as synonym, it's a technical ID
		}

		// SureChEMBL compound IDs (e.g., "SCHEMBL1234567")
		// Map to patent_compound by stripping SCHEMBL prefix
		if strings.HasPrefix(synonym, "SCHEMBL") {
			if _, exists := config.Dataconf["patent_compound"]; exists {
				// Extract numeric ID: SCHEMBL1234567 -> 1234567
				schemblID := strings.TrimPrefix(synonym, "SCHEMBL")
				if schemblID != "" {
					p.d.addXref(cid, fr, schemblID, "patent_compound", false)
					schemblXrefs++
				}
			}
			continue
		}

		// BindingDB monomer IDs (e.g., "BDBM50000001")
		// BindingDB uses numeric bucket method, so strip the "BDBM" prefix
		if strings.HasPrefix(synonym, "BDBM") {
			if _, exists := config.Dataconf["bindingdb"]; exists {
				bindingdbID := strings.TrimPrefix(synonym, "BDBM")
				if bindingdbID != "" {
					p.d.addXref(cid, fr, bindingdbID, "bindingdb", false)
					bindingdbXrefs++
				}
			}
			continue
		}

		// CAS Registry Numbers (e.g., "50-78-2")
		if isCASNumber(synonym) {
			if _, exists := config.Dataconf["cas"]; exists {
				p.d.addXref(cid, fr, synonym, "cas", false)
				casXrefs++
			}
			continue
		}

		// FDA UNII - Unique Ingredient Identifier (e.g., "UNII-362O9ITL9D" or just "362O9ITL9D")
		// TODO: Consider adding UNII as a full dataset for richer FDA substance data
		if strings.HasPrefix(synonym, "UNII-") {
			unii := strings.TrimPrefix(synonym, "UNII-")
			if unii != "" {
				p.cidToUNII[cid] = unii
				// Make searchable by both forms
				p.d.addXref(synonym, textLinkID, cid, "pubchem", true)
				p.d.addXref(unii, textLinkID, cid, "pubchem", true)
				uniiCount++
			}
			continue
		}

		// EPA DSSTox Substance ID (e.g., "DTXSID7020182")
		// TODO: Consider adding DTXSID as a full dataset for EPA CompTox toxicity data
		if strings.HasPrefix(synonym, "DTXSID") {
			p.cidToDTXSID[cid] = synonym
			p.d.addXref(synonym, textLinkID, cid, "pubchem", true)
			dtxsidCount++
			continue
		}

		// ZINC database IDs (e.g., "ZINC000000000001")
		if strings.HasPrefix(synonym, "ZINC") {
			p.cidToZINC[cid] = append(p.cidToZINC[cid], synonym)
			p.d.addXref(synonym, textLinkID, cid, "pubchem", true)
			zincCount++
			continue
		}

		// NCI compound numbers (e.g., "NSC123456")
		if strings.HasPrefix(synonym, "NSC") && len(synonym) > 3 {
			// Verify it's followed by digits
			rest := synonym[3:]
			isNSC := true
			for _, c := range rest {
				if c < '0' || c > '9' {
					isNSC = false
					break
				}
			}
			if isNSC {
				p.cidToNSC[cid] = append(p.cidToNSC[cid], synonym)
				p.d.addXref(synonym, textLinkID, cid, "pubchem", true)
				nscCount++
				continue
			}
		}

		// Filter out remaining technical identifiers to prefer human-readable names
		// Skip: DTXCID, RefChem, InChI keys, AKOS, NCGC, etc.
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
	if len(p.cidToSynonyms) > 0 {
		log.Printf("[PubChem]   - Average synonyms per CID: %.1f", float64(totalSynonyms)/float64(len(p.cidToSynonyms)))
	}
	log.Printf("[PubChem] Cross-references extracted from synonyms:")
	log.Printf("[PubChem]   - ChEMBL molecule xrefs: %d", chemblXrefs)
	log.Printf("[PubChem]   - SureChEMBL (patent_compound) xrefs: %d", schemblXrefs)
	log.Printf("[PubChem]   - BindingDB xrefs: %d", bindingdbXrefs)
	log.Printf("[PubChem]   - CAS xrefs: %d", casXrefs)
	log.Printf("[PubChem] External IDs extracted (stored as attributes, searchable):")
	log.Printf("[PubChem]   - FDA UNII: %d compounds", uniiCount)
	log.Printf("[PubChem]   - EPA DTXSID: %d compounds", dtxsidCount)
	log.Printf("[PubChem]   - ZINC IDs: %d total", zincCount)
	log.Printf("[PubChem]   - NCI NSC numbers: %d total", nscCount)
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

// isCASNumber checks if a string is a valid CAS Registry Number
// Format: 2-7 digits, hyphen, 2 digits, hyphen, 1 digit (e.g., "50-78-2", "7732-18-5")
func isCASNumber(s string) bool {
	// Must have exactly 2 hyphens
	if strings.Count(s, "-") != 2 {
		return false
	}

	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return false
	}

	// First part: 2-7 digits
	if len(parts[0]) < 2 || len(parts[0]) > 7 {
		return false
	}
	for _, c := range parts[0] {
		if c < '0' || c > '9' {
			return false
		}
	}

	// Second part: exactly 2 digits
	if len(parts[1]) != 2 {
		return false
	}
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return false
		}
	}

	// Third part: exactly 1 digit (check digit)
	if len(parts[2]) != 1 {
		return false
	}
	if parts[2][0] < '0' || parts[2][0] > '9' {
		return false
	}

	return true
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

	// Get worker count from config (default 4, but can be reduced via --pubchem-sdf-workers CLI flag)
	numWorkers := 4
	if val, ok := config.Appconf["pubchemSDFWorkers"]; ok {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			numWorkers = parsed
		}
	}
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

// parseSDFFile downloads and parses a single SDF file with retry mechanism
// Implements retry mechanism (configurable via pubchemRetryCount/pubchemRetryWaitMinutes) for network/corruption errors
func (p *pubchem) parseSDFFile(sdfFile string) {
	// Get retry configuration from application.param.json
	maxRetries := 2 // default
	retryWaitMinutes := 2 // default
	if val, ok := config.Appconf["pubchemRetryCount"]; ok {
		if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
			maxRetries = parsed
		}
	}
	if val, ok := config.Appconf["pubchemRetryWaitMinutes"]; ok {
		if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
			retryWaitMinutes = parsed
		}
	}

	ftpServer := config.Dataconf[p.source]["ftpUrl"]
	basePath := config.Dataconf[p.source]["path"]
	sdfPath := config.Dataconf[p.source]["pathSDF"]

	var lastError error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[PubChem] Retry attempt %d/%d for %s - waiting %d minutes before retry...", attempt, maxRetries, sdfFile, retryWaitMinutes)
			time.Sleep(time.Duration(retryWaitMinutes) * time.Minute)
			log.Printf("[PubChem] Retrying download and processing of %s...", sdfFile)
		}

		err := p.processSDFFile(sdfFile, ftpServer, basePath, sdfPath)
		if err == nil {
			return // Success
		}

		lastError = err
		log.Printf("[PubChem] Attempt %d failed for %s: %v", attempt+1, sdfFile, err)
	}

	// All retries exhausted - panic
	log.Panicf("[PubChem] FATAL: All %d retry attempts failed for %s. Last error: %v", maxRetries+1, sdfFile, lastError)
}

// processSDFFile handles the actual SDF file processing
// Returns error if processing fails (for retry mechanism)
func (p *pubchem) processSDFFile(sdfFile, ftpServer, basePath, sdfPath string) error {
	log.Printf("[PubChem] Parsing %s", sdfFile)

	// Download and open SDF file
	br, gz, _, _, localFile, _, err := getDataReaderNew(p.source, ftpServer, basePath, sdfPath+sdfFile)
	if err != nil {
		return fmt.Errorf("could not load %s: %v", sdfFile, err)
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
			// This catches the "flate: corrupt input" and network errors
			return fmt.Errorf("error reading stream: %v", err)
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
	return nil // Success
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

	// Get synonyms for this CID (loaded in Phase 2)
	var synonyms []string
	if syns, hasSynonyms := p.cidToSynonyms[cid]; hasSynonyms {
		synonyms = syns
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
		Synonyms:            synonyms,

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

		// External Database Identifiers (extracted from synonyms)
		Unii:    p.cidToUNII[cid],
		Dtxsid:  p.cidToDTXSID[cid],
		ZincIds: p.cidToZINC[cid],
		NscIds:  p.cidToNSC[cid],
	}

	// Marshal and store entry (send to biobtree processing channel)
	// Channels are thread-safe, multiple workers can write concurrently
	b, _ := ffjson.Marshal(attr)
	p.d.addProp3(cid, fr, b)

	// Add text search link for InChI Key (channel is thread-safe)
	if inchiKey != "" {
		p.d.addXref(inchiKey, textLinkID, cid, "pubchem", true)
	}

	// Add text search links for synonyms (makes synonyms searchable)
	for _, synonym := range synonyms {
		if synonym != "" {
			p.d.addXref(synonym, textLinkID, cid, "pubchem", true)
		}
	}

	// Create cross-references to MeSH terms
	// MeSH terms from PubChem are term names (e.g., "Aspirin"), not descriptor UIDs (e.g., "D001241")
	// Use addXrefViaKeyword to lookup the MeSH term name and find the actual MeSH descriptor entry
	for _, meshTerm := range meshTerms {
		if meshTerm != "" {
			p.d.addXrefViaKeyword(meshTerm, "mesh", cid, p.source, fr, false)
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

// loadSupplementaryMappings loads synonym and MeSH mappings for biotech-relevant CIDs
// These are stored as attributes in PubchemAttr entries during SDF parsing
//
// Note: PMID and Patent data are handled via cross-references (xrefs) in Phase 1,
// not as embedded attributes. See loadLiteratureCIDs() and loadPatentCIDs() for details.
func (p *pubchem) loadSupplementaryMappings() {
	log.Printf("[PubChem] Phase 2: Loading supplementary nomenclature data")

	// Load Synonyms - stored as attributes in PubchemAttr
	p.loadSynonyms()

	// Load MeSH terms (medical classifications) - stored as attributes
	p.loadMeSHTerms()

	// Load MeSH → Pharmacological Actions mapping
	p.loadMeSHPharmActions()

	log.Printf("[PubChem] Phase 2 complete: Synonyms and MeSH data loaded")
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
