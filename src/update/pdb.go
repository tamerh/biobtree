package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type pdb struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (p *pdb) check(err error, operation string) {
	checkWithContext(err, p.source, operation)
}

// Main update entry point
func (p *pdb) update() {
	defer p.d.wg.Done()

	log.Println("PDB: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(p.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, p.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("PDB: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Phase 1: Parse entries.idx for core metadata and store attributes.
	// Chain inventory comes from SIFTS first because entries.idx has no chain
	// information (this is what left chain_count at 0 on every entry).
	siftsPath := config.Dataconf[p.source]["siftsPath"]
	var chainData *pdbChainData
	if siftsPath != "" {
		chainData = p.parseSiftsChains(siftsPath)
	}
	processedIDs := p.parseEntriesIdx(testLimit, idLogFile, chainData)

	// Phase 2: Parse SIFTS files for cross-references (only for entries we processed)
	if siftsPath != "" {
		p.parseSiftsUniprot(siftsPath, processedIDs)
		p.parseSiftsTaxonomy(siftsPath, processedIDs)
		p.parseSiftsGO(siftsPath, processedIDs)
		p.parseSiftsPfam(siftsPath, processedIDs)
		p.parseSiftsEnzyme(siftsPath, processedIDs)
		p.parseSiftsInterpro(siftsPath, processedIDs)
		p.parseSiftsPubmed(siftsPath, processedIDs)
		p.parseSiftsCath(siftsPath, processedIDs)
		p.parseSiftsScop(siftsPath, processedIDs)
		p.parseSiftsEnsembl(siftsPath, processedIDs)
	}

	log.Printf("PDB: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress handler
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
}

// parseEntriesIdx parses the main PDB entries.idx file
func (p *pdb) parseEntriesIdx(testLimit int, idLogFile *os.File, chainData *pdbChainData) map[string]bool {
	filePath := config.Dataconf[p.source]["path"]
	log.Printf("PDB: Downloading entries.idx from %s", filePath)

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(p.source, "", "", filePath)
	p.check(err, "opening entries.idx")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[p.source]["id"]
	processedIDs := make(map[string]bool)

	scanner := bufio.NewScanner(br)
	// Increase buffer for long lines (some compound descriptions are very long)
	const maxCapacity = 1024 * 1024 // 1MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	var total uint64
	var previous int64
	lineNum := 0

	// Skip header lines (first 2 lines)
	for i := 0; i < 2 && scanner.Scan(); i++ {
		lineNum++
	}

	log.Println("PDB: Parsing entries.idx...")

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}

		// Format: IDCODE\tHEADER\tACCESSION DATE\tCOMPOUND\tSOURCE\tAUTHOR LIST\tRESOLUTION\tEXPERIMENT TYPE
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}

		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		if pdbID == "" || len(pdbID) != 4 {
			continue
		}

		header := strings.TrimSpace(fields[1])
		releaseDate := strings.TrimSpace(fields[2])
		title := strings.TrimSpace(fields[3])
		sourceOrganism := strings.TrimSpace(fields[4])
		authorList := strings.TrimSpace(fields[5])
		resolution := strings.TrimSpace(fields[6])

		// Method is in column 7 if present, default to "X-RAY DIFFRACTION"
		method := "X-RAY DIFFRACTION"
		if len(fields) > 7 && strings.TrimSpace(fields[7]) != "" {
			method = strings.TrimSpace(fields[7])
		}

		// Handle resolution - "NOT" means NMR/EM without resolution
		if resolution == "NOT" {
			resolution = ""
		}

		// Parse authors
		var authors []string
		if authorList != "" {
			for _, a := range strings.Split(authorList, ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					authors = append(authors, a)
				}
			}
		}

		// Determine molecule type based on header
		moleculeType := "prot"
		headerLower := strings.ToLower(header)
		if strings.Contains(headerLower, "dna") || strings.Contains(headerLower, "rna") {
			if strings.Contains(headerLower, "protein") || strings.Contains(headerLower, "complex") {
				moleculeType = "prot-nuc"
			} else {
				moleculeType = "nuc"
			}
		}

		// Build attribute. Chain inventory and residue coverage come from
		// SIFTS; entries.idx has neither.
		var chains []string
		var mappings []*pbuf.PdbChainMapping
		if chainData != nil {
			chains = chainData.chains[pdbID]
			mappings = chainData.mappings[pdbID]
		}
		attr := &pbuf.PdbAttr{
			Method:         method,
			Resolution:     resolution,
			Title:          title,
			Header:         header,
			ReleaseDate:    releaseDate,
			SourceOrganism: sourceOrganism,
			MoleculeType:   moleculeType,
			Authors:        authors,
			Chains:         strings.Join(chains, ","),
			ChainCount:     int32(len(chains)),
			ChainMappings:  mappings,
			ChainRanges:    formatChainRanges(mappings),
		}

		// Marshal and store attributes
		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("PDB: Error marshaling attributes for %s: %v", pdbID, err)
			continue
		}

		p.d.addProp3(pdbID, sourceID, attrBytes)

		// Add text search links
		p.d.addXref(pdbID, textLinkID, pdbID, p.source, true)

		// Add title keywords for search
		if title != "" {
			p.d.addXref(title, textLinkID, pdbID, p.source, true)
		}

		processedIDs[pdbID] = true

		// Log processed ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, pdbID)
		}

		total++

		// Progress tracking
		elapsed := int64(time.Since(p.d.start).Seconds())
		if elapsed > previous+p.d.progInterval {
			previous = elapsed
			p.d.progChan <- &progressInfo{dataset: p.source, currentKBPerSec: int64(total / uint64(elapsed))}
		}

		// Check test limit
		if testLimit > 0 && int(total) >= testLimit {
			log.Printf("PDB: Test limit reached (%d entries)", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("PDB: Scanner error: %v", err)
	}

	atomic.AddUint64(&p.d.totalParsedEntry, total)
	log.Printf("PDB: Parsed %d entries from entries.idx", total)

	return processedIDs
}

// downloadGzippedCSV downloads a gzipped CSV file and returns a reader
func (p *pdb) downloadGzippedCSV(url string) (*bufio.Reader, io.Closer, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, err
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, nil, err
	}

	// Return a combined closer that closes both gz and resp.Body
	closer := &multiCloser{closers: []io.Closer{gz, resp.Body}}
	return bufio.NewReader(gz), closer, nil
}

type multiCloser struct {
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var firstErr error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// parseSiftsUniprot parses pdb_chain_uniprot.csv.gz for UniProt mappings
func (p *pdb) parseSiftsUniprot(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_chain_uniprot.csv.gz"
	log.Printf("PDB: Downloading UniProt mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download UniProt mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool) // Avoid duplicate xrefs

	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}

		// Format: PDB,CHAIN,SP_PRIMARY,RES_BEG,RES_END,PDB_BEG,PDB_END,SP_BEG,SP_END
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			continue
		}

		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		uniprotID := strings.TrimSpace(fields[2])

		if pdbID == "" || uniprotID == "" {
			continue
		}

		// Skip if not in our processed IDs (test mode support)
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			continue
		}

		// Avoid duplicate xrefs
		pairKey := pdbID + ":" + uniprotID
		if seenPairs[pairKey] {
			continue
		}
		seenPairs[pairKey] = true

		p.d.addXref(pdbID, sourceID, uniprotID, "uniprot", false)
		count++
	}

	log.Printf("PDB: Added %d UniProt cross-references", count)
}

// parseSiftsTaxonomy parses pdb_chain_taxonomy.csv.gz for Taxonomy mappings
func (p *pdb) parseSiftsTaxonomy(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_chain_taxonomy.csv.gz"
	log.Printf("PDB: Downloading Taxonomy mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download Taxonomy mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool)

	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}

		// Format: PDB,CHAIN,TAX_ID,SCIENTIFIC_NAME
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			continue
		}

		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		taxID := strings.TrimSpace(fields[2])

		if pdbID == "" || taxID == "" {
			continue
		}

		// Skip if not in our processed IDs
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			continue
		}

		pairKey := pdbID + ":" + taxID
		if seenPairs[pairKey] {
			continue
		}
		seenPairs[pairKey] = true

		p.d.addXref(pdbID, sourceID, taxID, "taxonomy", false)
		count++
	}

	log.Printf("PDB: Added %d Taxonomy cross-references", count)
}

// parseSiftsGO parses pdb_chain_go.csv.gz for GO mappings
func (p *pdb) parseSiftsGO(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_chain_go.csv.gz"
	log.Printf("PDB: Downloading GO mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download GO mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool)

	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}

		// Format: PDB,CHAIN,SP_PRIMARY,WITH_STRING,EVIDENCE,GO_ID
		fields := strings.Split(line, ",")
		if len(fields) < 6 {
			continue
		}

		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		goID := strings.TrimSpace(fields[5])
		evidence := strings.TrimSpace(fields[4])

		if pdbID == "" || goID == "" {
			continue
		}

		// Only process valid GO IDs (format: GO:XXXXXXX)
		if !strings.HasPrefix(goID, "GO:") {
			continue
		}

		// Skip if not in our processed IDs
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			continue
		}

		pairKey := pdbID + ":" + goID
		if seenPairs[pairKey] {
			continue
		}
		seenPairs[pairKey] = true

		// Use addXrefWithEvidence to include GO evidence code
		p.d.addXrefWithEvidence(pdbID, sourceID, goID, "go", false, evidence)
		count++
	}

	log.Printf("PDB: Added %d GO cross-references", count)
}

// parseSiftsPfam parses pdb_chain_pfam.csv.gz for Pfam mappings
func (p *pdb) parseSiftsPfam(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_chain_pfam.csv.gz"
	log.Printf("PDB: Downloading Pfam mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download Pfam mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool)

	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}

		// Format: PDB,CHAIN,SP_PRIMARY,PFAM_ID,COVERAGE
		fields := strings.Split(line, ",")
		if len(fields) < 4 {
			continue
		}

		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		pfamID := strings.TrimSpace(fields[3])

		if pdbID == "" || pfamID == "" {
			continue
		}

		// Skip if not in our processed IDs
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			continue
		}

		pairKey := pdbID + ":" + pfamID
		if seenPairs[pairKey] {
			continue
		}
		seenPairs[pairKey] = true

		p.d.addXref(pdbID, sourceID, pfamID, "pfam", false)
		count++
	}

	log.Printf("PDB: Added %d Pfam cross-references", count)
}

// parseSiftsEnzyme parses pdb_chain_enzyme.csv.gz for EC number mappings
func (p *pdb) parseSiftsEnzyme(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_chain_enzyme.csv.gz"
	log.Printf("PDB: Downloading Enzyme mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download Enzyme mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool)

	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}

		// Format: PDB,CHAIN,ACCESSION,EC_NUMBER
		fields := strings.Split(line, ",")
		if len(fields) < 4 {
			continue
		}

		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		ecNumber := strings.TrimSpace(fields[3])

		if pdbID == "" || ecNumber == "" {
			continue
		}

		// Skip if not in our processed IDs
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			continue
		}

		pairKey := pdbID + ":" + ecNumber
		if seenPairs[pairKey] {
			continue
		}
		seenPairs[pairKey] = true

		// Link to ec dataset (enzyme classification)
		p.d.addXref(pdbID, sourceID, ecNumber, "ec", false)
		count++
	}

	log.Printf("PDB: Added %d Enzyme cross-references", count)
}

// parseSiftsInterpro parses pdb_chain_interpro.csv.gz for InterPro mappings
func (p *pdb) parseSiftsInterpro(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_chain_interpro.csv.gz"
	log.Printf("PDB: Downloading InterPro mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download InterPro mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool)

	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}

		// Format: PDB,CHAIN,INTERPRO_ID
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			continue
		}

		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		interproID := strings.TrimSpace(fields[2])

		if pdbID == "" || interproID == "" {
			continue
		}

		// Skip if not in our processed IDs
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			continue
		}

		pairKey := pdbID + ":" + interproID
		if seenPairs[pairKey] {
			continue
		}
		seenPairs[pairKey] = true

		p.d.addXref(pdbID, sourceID, interproID, "interpro", false)
		count++
	}

	log.Printf("PDB: Added %d InterPro cross-references", count)
}

// parseSiftsPubmed parses pdb_pubmed.csv.gz for PubMed mappings
func (p *pdb) parseSiftsPubmed(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_pubmed.csv.gz"
	log.Printf("PDB: Downloading PubMed mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download PubMed mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool)

	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}

		// Format: PDB,ORDINAL,PUBMED_ID
		fields := strings.Split(line, ",")
		if len(fields) < 3 {
			continue
		}

		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		pubmedID := strings.TrimSpace(fields[2])

		if pdbID == "" || pubmedID == "" {
			continue
		}

		// Skip if not in our processed IDs
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			continue
		}

		pairKey := pdbID + ":" + pubmedID
		if seenPairs[pairKey] {
			continue
		}
		seenPairs[pairKey] = true

		p.d.addXref(pdbID, sourceID, pubmedID, "pubmed", false)
		count++
	}

	log.Printf("PDB: Added %d PubMed cross-references", count)
}

// parseSiftsCath parses pdb_chain_cath_uniprot.csv.gz for CATH mappings
func (p *pdb) parseSiftsCath(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_chain_cath_uniprot.csv.gz"
	log.Printf("PDB: Downloading CATH mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download CATH mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool)

	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}

		// Format: PDB,CHAIN,SP_PRIMARY,CATH_ID
		fields := strings.Split(line, ",")
		if len(fields) < 4 {
			continue
		}

		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		cathID := strings.TrimSpace(fields[3])

		if pdbID == "" || cathID == "" {
			continue
		}

		// Skip if not in our processed IDs
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			continue
		}

		pairKey := pdbID + ":" + cathID
		if seenPairs[pairKey] {
			continue
		}
		seenPairs[pairKey] = true

		p.d.addXref(pdbID, sourceID, cathID, "cath", false)
		count++
	}

	log.Printf("PDB: Added %d CATH cross-references", count)
}

// Utility function to convert string to int with default
func pdbStringToInt(s string, defaultVal int32) int32 {
	if s == "" {
		return defaultVal
	}
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return defaultVal
	}
	return int32(val)
}

// pdbChainData holds the per-entry polymer chain inventory and the SIFTS
// chain -> UniProt residue mappings used for coverage reporting.
type pdbChainData struct {
	chains   map[string][]string
	mappings map[string][]*pbuf.PdbChainMapping
}

// parseSiftsChains builds a PDB -> polymer chain inventory and per-chain
// UniProt residue coverage from SIFTS. entries.idx carries no chain
// information, so this is the only source for chains/chain_count.
//
// uniprot_segments_observed gives the observed (modelled) segments, so gaps are
// preserved. pdb_chain_uniprot supplies the full mapped span and acts as a
// fallback for chains that have no observed segments. pdb_chain_taxonomy is
// unioned in so structures without a UniProt mapping still get their chains.
func (p *pdb) parseSiftsChains(basePath string) *pdbChainData {
	chainSets := make(map[string]map[string]bool)
	overall := make(map[string][]*pbuf.PdbChainMapping)
	observed := make(map[string][]*pbuf.PdbChainMapping)

	for _, file := range []string{"pdb_chain_uniprot.csv.gz", "pdb_chain_taxonomy.csv.gz", "uniprot_segments_observed.csv.gz"} {
		url := basePath + file
		log.Printf("PDB: Downloading chain inventory from %s", url)

		br, closer, err := p.downloadGzippedCSV(url)
		if err != nil {
			log.Printf("PDB: Warning - could not download chain inventory %s: %v", file, err)
			continue
		}

		var scanErr error
		switch file {
		case "pdb_chain_uniprot.csv.gz":
			scanErr = scanPdbChainMappings(br, chainSets, overall)
		case "uniprot_segments_observed.csv.gz":
			scanErr = scanPdbChainMappings(br, chainSets, observed)
		default:
			scanErr = scanChainInventory(br, chainSets)
		}
		if scanErr != nil {
			log.Printf("PDB: Warning - error reading chain inventory %s: %v", file, scanErr)
		}
		closer.Close()
	}

	chains := make(map[string][]string, len(chainSets))
	for pdbID, set := range chainSets {
		list := make([]string, 0, len(set))
		for chain := range set {
			list = append(list, chain)
		}
		sort.Strings(list)
		chains[pdbID] = list
	}

	mappings := mergePdbChainMappings(overall, observed)
	log.Printf("PDB: Chain inventory built for %d entries (%d with residue mappings)", len(chains), len(mappings))
	return &pdbChainData{chains: chains, mappings: mappings}
}

// scanChainInventory reads a SIFTS chain CSV (PDB,CHAIN,...) into sets.
func scanChainInventory(r io.Reader, sets map[string]map[string]bool) error {
	scanner := bufio.NewScanner(r)
	const maxCapacity = 1024 * 1024
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}
		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		chain := strings.TrimSpace(fields[1])
		if pdbID == "" || chain == "" {
			continue
		}
		addChain(sets, pdbID, chain)
	}
	return scanner.Err()
}

// scanPdbChainMappings reads a SIFTS CSV with the shared
// PDB,CHAIN,SP_PRIMARY,RES_BEG,RES_END,PDB_BEG,PDB_END,SP_BEG,SP_END layout
// (pdb_chain_uniprot and uniprot_segments_observed) into chain sets and
// per-entry residue mappings.
func scanPdbChainMappings(r io.Reader, sets map[string]map[string]bool, out map[string][]*pbuf.PdbChainMapping) error {
	scanner := bufio.NewScanner(r)
	const maxCapacity = 1024 * 1024
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 9 {
			continue
		}
		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		chain := strings.TrimSpace(fields[1])
		uniprot := strings.TrimSpace(fields[2])
		if pdbID == "" || chain == "" {
			continue
		}
		addChain(sets, pdbID, chain)
		if uniprot == "" {
			continue
		}
		spBeg := pdbStringToInt(strings.TrimSpace(fields[7]), 0)
		spEnd := pdbStringToInt(strings.TrimSpace(fields[8]), 0)
		if spBeg <= 0 || spEnd < spBeg {
			continue
		}
		out[pdbID] = append(out[pdbID], &pbuf.PdbChainMapping{
			Chain:        chain,
			Uniprot:      uniprot,
			UniprotStart: spBeg,
			UniprotEnd:   spEnd,
			PdbStart:     pdbStringToInt(strings.TrimSpace(fields[5]), 0),
			PdbEnd:       pdbStringToInt(strings.TrimSpace(fields[6]), 0),
		})
	}
	return scanner.Err()
}

func addChain(sets map[string]map[string]bool, pdbID, chain string) {
	if sets[pdbID] == nil {
		sets[pdbID] = make(map[string]bool)
	}
	sets[pdbID][chain] = true
}

// mergePdbChainMappings prefers observed segments and falls back to the full
// pdb_chain_uniprot span for any (chain, UniProt) pair without observed data.
func mergePdbChainMappings(overall, observed map[string][]*pbuf.PdbChainMapping) map[string][]*pbuf.PdbChainMapping {
	out := make(map[string][]*pbuf.PdbChainMapping, len(overall)+len(observed))
	for pdbID, list := range overall {
		out[pdbID] = append(out[pdbID], list...)
	}
	for pdbID, list := range observed {
		seen := make(map[string]bool, len(list))
		for _, m := range list {
			seen[m.Chain+"|"+m.Uniprot] = true
		}
		filtered := out[pdbID][:0]
		for _, m := range out[pdbID] {
			if !seen[m.Chain+"|"+m.Uniprot] {
				filtered = append(filtered, m)
			}
		}
		out[pdbID] = append(filtered, list...)
	}
	for pdbID, list := range out {
		sort.Slice(list, func(i, j int) bool {
			if list[i].Chain != list[j].Chain {
				return list[i].Chain < list[j].Chain
			}
			if list[i].Uniprot != list[j].Uniprot {
				return list[i].Uniprot < list[j].Uniprot
			}
			return list[i].UniprotStart < list[j].UniprotStart
		})
		out[pdbID] = list
	}
	return out
}

// formatChainRanges collapses per-chain mappings into the compact UniProt-style
// form "A/B/C/D=1-390". The span is the min/max observed residue per
// (chain, UniProt). When the structure maps to more than one protein each
// group is prefixed, e.g. "Q8TDI8:A=86-137;O75838:B=2-187".
func formatChainRanges(mappings []*pbuf.PdbChainMapping) string {
	if len(mappings) == 0 {
		return ""
	}

	type chainKey struct {
		chain   string
		uniprot string
	}
	type span struct {
		start int32
		end   int32
	}
	spans := make(map[chainKey]span)
	uniprots := make(map[string]bool)
	for _, m := range mappings {
		if m == nil || m.Chain == "" || m.Uniprot == "" || m.UniprotStart <= 0 || m.UniprotEnd < m.UniprotStart {
			continue
		}
		uniprots[m.Uniprot] = true
		k := chainKey{chain: m.Chain, uniprot: m.Uniprot}
		s, ok := spans[k]
		if !ok {
			spans[k] = span{start: m.UniprotStart, end: m.UniprotEnd}
			continue
		}
		if m.UniprotStart < s.start {
			s.start = m.UniprotStart
		}
		if m.UniprotEnd > s.end {
			s.end = m.UniprotEnd
		}
		spans[k] = s
	}
	if len(spans) == 0 {
		return ""
	}

	type groupKey struct {
		uniprot string
		start   int32
		end     int32
	}
	groups := make(map[groupKey][]string)
	for k, s := range spans {
		g := groupKey{uniprot: k.uniprot, start: s.start, end: s.end}
		groups[g] = append(groups[g], k.chain)
	}

	keys := make([]groupKey, 0, len(groups))
	for g := range groups {
		sort.Strings(groups[g])
		keys = append(keys, g)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].uniprot != keys[j].uniprot {
			return keys[i].uniprot < keys[j].uniprot
		}
		// Order by first chain so "A=3-187;B=2-187" reads naturally even when
		// the spans differ by a residue.
		ci, cj := groups[keys[i]][0], groups[keys[j]][0]
		if ci != cj {
			return ci < cj
		}
		if keys[i].start != keys[j].start {
			return keys[i].start < keys[j].start
		}
		return keys[i].end < keys[j].end
	})

	multi := len(uniprots) > 1
	parts := make([]string, 0, len(keys))
	for _, g := range keys {
		chains := groups[g]
		part := strings.Join(chains, "/") + "=" + strconv.Itoa(int(g.start)) + "-" + strconv.Itoa(int(g.end))
		if multi {
			part = g.uniprot + ":" + part
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ";")
}

// parseSiftsScop parses pdb_chain_scop_uniprot.csv.gz for SCOP domain mappings.
// Columns: PDB,CHAIN,SP_PRIMARY,SUNID,SCOP_ID
func (p *pdb) parseSiftsScop(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_chain_scop_uniprot.csv.gz"
	log.Printf("PDB: Downloading SCOP mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download SCOP mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool)

	err = scanScopMappings(br, func(pdbID, scopID string) {
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			return
		}
		pairKey := pdbID + ":" + scopID
		if seenPairs[pairKey] {
			return
		}
		seenPairs[pairKey] = true
		p.d.addXref(pdbID, sourceID, scopID, "scop", false)
		count++
	})
	if err != nil {
		log.Printf("PDB: Warning - error reading SCOP mappings: %v", err)
	}

	log.Printf("PDB: Added %d SCOP cross-references", count)
}

// scanScopMappings reads pdb_chain_scop_uniprot.csv.gz rows
// (PDB,CHAIN,SP_PRIMARY,SUNID,SCOP_ID) and emits the SCOP domain id.
func scanScopMappings(r io.Reader, emit func(pdbID, scopID string)) error {
	scanner := bufio.NewScanner(r)
	const maxCapacity = 1024 * 1024
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 5 {
			continue
		}
		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		scopID := strings.TrimSpace(fields[4])
		if pdbID == "" || scopID == "" {
			continue
		}
		emit(pdbID, scopID)
	}
	return scanner.Err()
}

// parseSiftsEnsembl parses pdb_chain_ensembl.csv.gz for Ensembl gene mappings.
// Columns: PDB,CHAIN,SP_PRIMARY,GENE_ID,TRANSCRIPT_ID,TRANSLATION_ID,EXON_ID
func (p *pdb) parseSiftsEnsembl(basePath string, processedIDs map[string]bool) {
	url := basePath + "pdb_chain_ensembl.csv.gz"
	log.Printf("PDB: Downloading Ensembl mappings from %s", url)

	br, closer, err := p.downloadGzippedCSV(url)
	if err != nil {
		log.Printf("PDB: Warning - could not download Ensembl mappings: %v", err)
		return
	}
	defer closer.Close()

	sourceID := config.Dataconf[p.source]["id"]
	var count int
	seenPairs := make(map[string]bool)

	err = scanEnsemblMappings(br, func(pdbID, ensemblID string) {
		if len(processedIDs) > 0 && !processedIDs[pdbID] {
			return
		}
		pairKey := pdbID + ":" + ensemblID
		if seenPairs[pairKey] {
			return
		}
		seenPairs[pairKey] = true
		p.d.addXref(pdbID, sourceID, ensemblID, "ensembl", false)
		count++
	})
	if err != nil {
		log.Printf("PDB: Warning - error reading Ensembl mappings: %v", err)
	}

	log.Printf("PDB: Added %d Ensembl cross-references", count)
}

// scanEnsemblMappings reads pdb_chain_ensembl.csv.gz rows
// (PDB,CHAIN,SP_PRIMARY,GENE_ID,TRANSCRIPT_ID,TRANSLATION_ID,EXON_ID)
// and emits the Ensembl gene id.
func scanEnsemblMappings(r io.Reader, emit func(pdbID, ensemblID string)) error {
	scanner := bufio.NewScanner(r)
	const maxCapacity = 1024 * 1024
	scanner.Buffer(make([]byte, maxCapacity), maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "PDB,") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 4 {
			continue
		}
		pdbID := strings.ToUpper(strings.TrimSpace(fields[0]))
		ensemblID := strings.TrimSpace(fields[3])
		if pdbID == "" || ensemblID == "" {
			continue
		}
		emit(pdbID, ensemblID)
	}
	return scanner.Err()
}
