package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"io"
	"log"
	"net/http"
	"os"
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

	// Phase 1: Parse entries.idx for core metadata and store attributes
	processedIDs := p.parseEntriesIdx(testLimit, idLogFile)

	// Phase 2: Parse SIFTS files for cross-references (only for entries we processed)
	siftsPath := config.Dataconf[p.source]["siftsPath"]
	if siftsPath != "" {
		p.parseSiftsUniprot(siftsPath, processedIDs)
		p.parseSiftsTaxonomy(siftsPath, processedIDs)
		p.parseSiftsGO(siftsPath, processedIDs)
		p.parseSiftsPfam(siftsPath, processedIDs)
		p.parseSiftsEnzyme(siftsPath, processedIDs)
		p.parseSiftsInterpro(siftsPath, processedIDs)
		p.parseSiftsPubmed(siftsPath, processedIDs)
		p.parseSiftsCath(siftsPath, processedIDs)
	}

	log.Printf("PDB: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress handler
	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
}

// parseEntriesIdx parses the main PDB entries.idx file
func (p *pdb) parseEntriesIdx(testLimit int, idLogFile *os.File) map[string]bool {
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

		// Build attribute
		attr := &pbuf.PdbAttr{
			Method:         method,
			Resolution:     resolution,
			Title:          title,
			Header:         header,
			ReleaseDate:    releaseDate,
			SourceOrganism: sourceOrganism,
			MoleculeType:   moleculeType,
			Authors:        authors,
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
