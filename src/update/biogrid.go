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

// biogrid parses BioGRID protein-protein and genetic interaction data
// Source: https://thebiogrid.org/
// Format: TAB3 (BIOGRID-ALL-LATEST.tab3.zip)
// TAB3 has 37 columns including throughput, modification, qualifications, tags, and ontology terms
type biogrid struct {
	source string
	d      *DataUpdate
}

// TAB3 column indices (0-indexed)
const (
	tab3InteractionID      = 0  // #BioGRID Interaction ID
	tab3EntrezA            = 1  // Entrez Gene Interactor A
	tab3EntrezB            = 2  // Entrez Gene Interactor B
	tab3BiogridIDA         = 3  // BioGRID ID Interactor A
	tab3BiogridIDB         = 4  // BioGRID ID Interactor B
	tab3SystematicNameA    = 5  // Systematic Name Interactor A
	tab3SystematicNameB    = 6  // Systematic Name Interactor B
	tab3SymbolA            = 7  // Official Symbol Interactor A
	tab3SymbolB            = 8  // Official Symbol Interactor B
	tab3SynonymsA          = 9  // Synonyms Interactor A
	tab3SynonymsB          = 10 // Synonyms Interactor B
	tab3ExperimentalSystem = 11 // Experimental System
	tab3ExperimentalType   = 12 // Experimental System Type (physical/genetic)
	tab3Author             = 13 // Author
	tab3PubSource          = 14 // Publication Source (e.g., "PUBMED:9006895")
	tab3OrganismIDA        = 15 // Organism ID Interactor A
	tab3OrganismIDB        = 16 // Organism ID Interactor B
	tab3Throughput         = 17 // Throughput (Low Throughput/High Throughput)
	tab3Score              = 18 // Score
	tab3Modification       = 19 // Modification (for genetic interactions)
	tab3Qualifications     = 20 // Qualifications
	tab3Tags               = 21 // Tags
	tab3SourceDatabase     = 22 // Source Database
	tab3SwissProtA         = 23 // SWISS-PROT Accessions Interactor A
	tab3TremblA            = 24 // TREMBL Accessions Interactor A
	tab3RefseqA            = 25 // REFSEQ Accessions Interactor A
	tab3SwissProtB         = 26 // SWISS-PROT Accessions Interactor B
	tab3TremblB            = 27 // TREMBL Accessions Interactor B
	tab3RefseqB            = 28 // REFSEQ Accessions Interactor B
	tab3OntologyTermIDs    = 29 // Ontology Term IDs
	tab3OntologyTermNames  = 30 // Ontology Term Names
	tab3OntologyCategories = 31 // Ontology Term Categories
	tab3OntologyQualIDs    = 32 // Ontology Term Qualifier IDs
	tab3OntologyQualNames  = 33 // Ontology Term Qualifier Names
	tab3OntologyTypes      = 34 // Ontology Term Types
	tab3OrganismNameA      = 35 // Organism Name Interactor A
	tab3OrganismNameB      = 36 // Organism Name Interactor B
)

// Helper for context-aware error checking
func (b *biogrid) check(err error, operation string) {
	checkWithContext(err, b.source, operation)
}

// Main update entry point
func (b *biogrid) update() {
	defer b.d.wg.Done()

	log.Println("BioGRID: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(b.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, b.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("BioGRID: [TEST MODE] Processing up to %d interactions", testLimit)
	}

	// Process PSI-MITAB file
	b.parseAndSaveInteractions(testLimit, idLogFile)

	log.Printf("BioGRID: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress tracker
	b.d.progChan <- &progressInfo{dataset: b.source, done: true}
}

// parseAndSaveInteractions processes the BioGRID TAB3 file
func (b *biogrid) parseAndSaveInteractions(testLimit int, idLogFile *os.File) {
	// Get full URL from config
	fullURL := config.Dataconf[b.source]["path"]

	if config.IsTestMode() {
		log.Printf("BioGRID: [TEST MODE] Downloading from %s (will stop after %d interactions)", fullURL, testLimit)
	} else {
		log.Printf("BioGRID: Downloading from %s", fullURL)
	}

	// Download ZIP file
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(b.source, "", "", fullURL)
	b.check(err, "opening BioGRID TAB3 ZIP file")
	defer closeReaders(gz, ftpFile, client, localFile)

	sourceID := config.Dataconf[b.source]["id"]

	// Read the entire ZIP into memory (needed for zip.NewReader)
	log.Println("BioGRID: Reading ZIP file...")
	zipData, err := io.ReadAll(br)
	b.check(err, "reading ZIP file")

	log.Printf("BioGRID: ZIP file size: %.2f MB", float64(len(zipData))/1024/1024)

	// Open ZIP
	zipReader, err := zip.NewReader(biogridReaderAtFromBytes(zipData), int64(len(zipData)))
	b.check(err, "opening ZIP archive")

	if len(zipReader.File) == 0 {
		log.Fatal("BioGRID: ZIP file is empty")
	}

	// Find the TAB3 file inside (should be a .tab3.txt or .txt file)
	var tab3File *zip.File
	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, ".tab3.txt") || strings.HasSuffix(f.Name, ".txt") {
			tab3File = f
			break
		}
	}

	if tab3File == nil {
		log.Fatal("BioGRID: No TAB3 file found in ZIP")
	}

	log.Printf("BioGRID: Found file in ZIP: %s (%.2f MB uncompressed)",
		tab3File.Name, float64(tab3File.UncompressedSize64)/1024/1024)

	// Open the TAB3 file from ZIP
	rc, err := tab3File.Open()
	b.check(err, "opening TAB3 from ZIP")
	defer rc.Close()

	// Create scanner for TAB3 format
	scanner := bufio.NewScanner(rc)

	// Increase buffer size for long lines
	const maxCapacity = 1024 * 1024 // 1MB buffer
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read and parse header
	if !scanner.Scan() {
		b.check(scanner.Err(), "reading BioGRID header")
		return
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	log.Printf("BioGRID: Found %d columns in TAB3 header (expected 37)", len(header))

	// Group interactions by BioGRID interactor ID
	// Key: BioGRID ID, Value: list of interactions for that interactor
	interactorData := make(map[string]*biogridAggregator)

	var processedCount int
	var previous int64
	var skippedLines int
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

		// In test mode, check if we've reached the limit
		if testLimit > 0 && processedCount >= testLimit {
			log.Printf("BioGRID: [TEST MODE] Reached limit of %d interactions, stopping", testLimit)
			break
		}

		// Progress reporting
		elapsed := int64(time.Since(b.d.start).Seconds())
		if elapsed > previous+b.d.progInterval {
			previous = elapsed
			b.d.progChan <- &progressInfo{dataset: b.source, currentKBPerSec: int64(processedCount / int(elapsed))}
		}

		// TAB3 format has direct columns for BioGRID IDs
		// Column 3: BioGRID ID Interactor A
		// Column 4: BioGRID ID Interactor B
		biogridA := b.getFieldByIndex(row, tab3BiogridIDA)
		biogridB := b.getFieldByIndex(row, tab3BiogridIDB)

		// Skip if either interactor doesn't have a BioGRID ID
		if biogridA == "" || biogridA == "-" || biogridB == "" || biogridB == "-" {
			skippedLines++
			continue
		}

		// In test mode, filter to only human-human interactions (taxid:9606)
		if config.IsTestMode() {
			taxidA := b.parseOrganismID(b.getFieldByIndex(row, tab3OrganismIDA))
			taxidB := b.parseOrganismID(b.getFieldByIndex(row, tab3OrganismIDB))
			if taxidA != 9606 || taxidB != 9606 {
				continue
			}
		}

		// Build interaction object
		interaction := b.buildInteraction(row, biogridA, biogridB)
		if interaction == nil {
			skippedLines++
			continue
		}

		// Add to interactor A's aggregator
		b.addToAggregator(interactorData, biogridA, interaction, row)

		// Create reverse interaction (B→A) and add to B's aggregator
		reverseInteraction := b.buildInteraction(row, biogridB, biogridA)
		if reverseInteraction != nil {
			b.addToAggregator(interactorData, biogridB, reverseInteraction, row)
		}

		processedCount++
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("BioGRID: Scanner error: %v", err)
	}

	log.Printf("BioGRID: Total rows read: %d, Interactions processed: %d", totalRowsRead, processedCount)
	log.Printf("BioGRID: Unique interactors: %d, Skipped: %d", len(interactorData), skippedLines)

	// Now save grouped interactor data
	savedInteractors := 0
	for biogridID, agg := range interactorData {
		// Save interactor
		b.saveInteractor(biogridID, agg, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(biogridID + "\n")
		}

		savedInteractors++
	}

	log.Printf("BioGRID: Saved %d unique interactors with interactions", savedInteractors)

	// Update entry statistics
	atomic.AddUint64(&b.d.totalParsedEntry, uint64(savedInteractors))
}

// biogridAggregator holds aggregated data for a single BioGRID interactor
type biogridAggregator struct {
	interactions        []*pbuf.BiogridInteraction
	uniquePartners      map[string]bool
	physicalCount       int
	geneticCount        int
	organisms           map[int32]bool
	experimentalSystems map[string]bool
	pubmedIDs           map[string]bool
	entrezID            string   // The Entrez Gene ID for this interactor
	symbol              string   // The gene symbol for this interactor
	uniprotIDs          []string // UniProt accessions (e.g., P45985)
	refseqIDs           []string // RefSeq IDs (e.g., NP_003001)
}

// addToAggregator adds an interaction to the aggregator for a given interactor
func (b *biogrid) addToAggregator(data map[string]*biogridAggregator, biogridID string, interaction *pbuf.BiogridInteraction, row []string) {
	agg, exists := data[biogridID]
	if !exists {
		agg = &biogridAggregator{
			interactions:        make([]*pbuf.BiogridInteraction, 0),
			uniquePartners:      make(map[string]bool),
			organisms:           make(map[int32]bool),
			experimentalSystems: make(map[string]bool),
			pubmedIDs:           make(map[string]bool),
		}
		// TAB3 format has direct columns for Entrez ID and Symbol
		// Column 1: Entrez Gene Interactor A
		entrezA := b.getFieldByIndex(row, tab3EntrezA)
		if entrezA != "-" {
			agg.entrezID = entrezA
		}

		// Column 7: Official Symbol Interactor A
		symbolA := b.getFieldByIndex(row, tab3SymbolA)
		if symbolA != "-" {
			agg.symbol = symbolA
		}

		// TAB3 format has direct columns for UniProt and RefSeq IDs
		// Column 23: SWISS-PROT Accessions Interactor A
		// Column 24: TREMBL Accessions Interactor A
		// Column 25: REFSEQ Accessions Interactor A
		agg.uniprotIDs = b.parseDelimitedIDs(b.getFieldByIndex(row, tab3SwissProtA))
		tremblIDs := b.parseDelimitedIDs(b.getFieldByIndex(row, tab3TremblA))
		agg.uniprotIDs = append(agg.uniprotIDs, tremblIDs...)
		agg.refseqIDs = b.parseDelimitedIDs(b.getFieldByIndex(row, tab3RefseqA))

		data[biogridID] = agg
	}

	agg.interactions = append(agg.interactions, interaction)
	agg.uniquePartners[interaction.PartnerBiogridId] = true

	if interaction.ExperimentalSystemType == "physical" {
		agg.physicalCount++
	} else if interaction.ExperimentalSystemType == "genetic" {
		agg.geneticCount++
	}

	if interaction.OrganismA != 0 {
		agg.organisms[interaction.OrganismA] = true
	}
	if interaction.OrganismB != 0 {
		agg.organisms[interaction.OrganismB] = true
	}

	if interaction.ExperimentalSystem != "" {
		agg.experimentalSystems[interaction.ExperimentalSystem] = true
	}

	if interaction.PubmedId != "" {
		agg.pubmedIDs[interaction.PubmedId] = true
	}
}

// getFieldByIndex gets field at a specific index safely
func (b *biogrid) getFieldByIndex(row []string, idx int) string {
	if idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

// parseOrganismID parses organism taxonomy ID from TAB3 column
// TAB3 format has direct numeric taxonomy IDs (e.g., "9606")
func (b *biogrid) parseOrganismID(field string) int32 {
	if field == "" || field == "-" {
		return 0
	}
	taxid, err := strconv.ParseInt(field, 10, 32)
	if err != nil {
		return 0
	}
	return int32(taxid)
}

// parseDelimitedIDs parses pipe-delimited IDs from TAB3 columns
// Used for UniProt, RefSeq accessions (e.g., "P45985|Q12345" or "NP_003001|NP_001268364")
func (b *biogrid) parseDelimitedIDs(field string) []string {
	if field == "" || field == "-" {
		return nil
	}
	parts := strings.Split(field, "|")
	result := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && p != "-" && !seen[p] {
			result = append(result, p)
			seen[p] = true
		}
	}
	return result
}

// extractBiogridID extracts BioGRID ID from the ID field (legacy MITAB support)
// Format: biogrid:103|entrez gene/locuslink:6416
func (b *biogrid) extractBiogridID(idField string) string {
	// Split by pipe to handle multiple IDs
	ids := strings.Split(idField, "|")

	// Look for biogrid: entries
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "biogrid:") {
			return strings.TrimPrefix(id, "biogrid:")
		}
	}

	return ""
}

// extractEntrezID extracts Entrez Gene ID from the ID field
// Format: biogrid:103|entrez gene/locuslink:6416
// Note: Alt ID fields (columns 2-3) may contain gene SYMBOLS with the same prefix
// (e.g., "entrez gene/locuslink:MAP2K4"), so we validate the value is numeric
func (b *biogrid) extractEntrezID(idField string) string {
	// Split by pipe to handle multiple IDs
	ids := strings.Split(idField, "|")

	// Look for entrez gene/locuslink: entries
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "entrez gene/locuslink:") {
			value := strings.TrimPrefix(id, "entrez gene/locuslink:")
			// Validate that the value is numeric (actual Entrez ID, not a gene symbol)
			if len(value) > 0 && isNumericString(value) {
				return value
			}
		}
	}

	return ""
}

// isNumericString checks if a string contains only digits
func isNumericString(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// extractUniProtIDs extracts UniProt accessions from the Alt IDs field
// Format: biogrid:112315|uniprot/swiss-prot:P45985|refseq:NP_003001
func (b *biogrid) extractUniProtIDs(altIDsField string) []string {
	var uniprotIDs []string
	seen := make(map[string]bool)

	// Split by pipe to handle multiple IDs
	ids := strings.Split(altIDsField, "|")

	for _, id := range ids {
		id = strings.TrimSpace(id)
		// Match uniprot/swiss-prot: or uniprot/trembl: prefixes
		if strings.HasPrefix(id, "uniprot/swiss-prot:") {
			acc := strings.TrimPrefix(id, "uniprot/swiss-prot:")
			if acc != "" && !seen[acc] {
				uniprotIDs = append(uniprotIDs, acc)
				seen[acc] = true
			}
		} else if strings.HasPrefix(id, "uniprot/trembl:") {
			acc := strings.TrimPrefix(id, "uniprot/trembl:")
			if acc != "" && !seen[acc] {
				uniprotIDs = append(uniprotIDs, acc)
				seen[acc] = true
			}
		}
	}

	return uniprotIDs
}

// extractRefSeqIDs extracts RefSeq IDs from the Alt IDs field
// Format: biogrid:112315|refseq:NP_003001|refseq:NP_001268364
func (b *biogrid) extractRefSeqIDs(altIDsField string) []string {
	var refseqIDs []string
	seen := make(map[string]bool)

	// Split by pipe to handle multiple IDs
	ids := strings.Split(altIDsField, "|")

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "refseq:") {
			refID := strings.TrimPrefix(id, "refseq:")
			if refID != "" && !seen[refID] {
				refseqIDs = append(refseqIDs, refID)
				seen[refID] = true
			}
		}
	}

	return refseqIDs
}

// extractSymbol extracts gene symbol from alias field
// Format: entrez gene/locuslink:MAP2K4(gene name)|biogrid:MAP2K4
func (b *biogrid) extractSymbol(aliasField string) string {
	// Split by pipe
	aliases := strings.Split(aliasField, "|")

	// Look for entries with "(gene name)" or "(official symbol)"
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if strings.Contains(alias, "(gene name)") || strings.Contains(alias, "(official symbol)") {
			// Extract the symbol: "entrez gene/locuslink:MAP2K4(gene name)" → "MAP2K4"
			if idx := strings.Index(alias, ":"); idx > 0 {
				symbol := alias[idx+1:]
				if parenIdx := strings.Index(symbol, "("); parenIdx > 0 {
					return symbol[:parenIdx]
				}
			}
		}
	}

	// Fallback: look for biogrid: symbol
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if strings.HasPrefix(alias, "biogrid:") {
			symbol := strings.TrimPrefix(alias, "biogrid:")
			// Remove any parenthetical suffixes
			if parenIdx := strings.Index(symbol, "("); parenIdx > 0 {
				return symbol[:parenIdx]
			}
			return symbol
		}
	}

	return ""
}

// extractTaxid extracts NCBI taxonomy ID
// Format: taxid:9606(Homo sapiens)
func (b *biogrid) extractTaxid(taxidField string) int32 {
	// Split by pipe (in case there are multiple)
	parts := strings.Split(taxidField, "|")
	if len(parts) == 0 {
		return 0
	}

	// Parse first taxid
	taxidStr := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(taxidStr, "taxid:") {
		return 0
	}

	// Extract number: "taxid:9606(Homo sapiens)" → "9606"
	taxidStr = strings.TrimPrefix(taxidStr, "taxid:")
	if idx := strings.Index(taxidStr, "("); idx > 0 {
		taxidStr = taxidStr[:idx]
	}

	taxid, err := strconv.ParseInt(taxidStr, 10, 32)
	if err != nil {
		return 0
	}

	return int32(taxid)
}

// extractPubMedID extracts PubMed ID
// Format: pubmed:10542231
func (b *biogrid) extractPubMedID(pubField string) string {
	// Split by pipe
	ids := strings.Split(pubField, "|")

	// Look for pubmed: entries
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "pubmed:") {
			return strings.TrimPrefix(id, "pubmed:")
		}
	}

	return ""
}

// extractPSIMITerm extracts PSI-MI term name
// Format: psi-mi:"MI:0401"(biochemical) or psi-mi:"MI:0018"(two hybrid)
func (b *biogrid) extractPSIMITerm(field string) string {
	if field == "" || field == "-" {
		return ""
	}

	// Extract the term name from parentheses: psi-mi:"MI:0401"(biochemical) → "biochemical"
	if idx := strings.LastIndex(field, "("); idx > 0 {
		if endIdx := strings.Index(field[idx:], ")"); endIdx > 0 {
			return field[idx+1 : idx+endIdx]
		}
	}

	return field
}

// extractInteractionType determines if interaction is physical or genetic
// Physical: psi-mi:"MI:0407"(direct interaction), psi-mi:"MI:0915"(physical association)
// Genetic: psi-mi:"MI:0794"(synthetic genetic interaction), psi-mi:"MI:0799"(phenotypic effect)
func (b *biogrid) extractInteractionType(field string) string {
	if field == "" || field == "-" {
		return ""
	}

	lower := strings.ToLower(field)
	if strings.Contains(lower, "genetic") || strings.Contains(lower, "phenotyp") ||
		strings.Contains(lower, "suppression") || strings.Contains(lower, "synthetic") ||
		strings.Contains(lower, "dosage") {
		return "genetic"
	}

	// Default to physical for all other interaction types
	return "physical"
}

// buildInteraction creates an interaction record from a TAB3 row
func (b *biogrid) buildInteraction(row []string, biogridA, biogridB string) *pbuf.BiogridInteraction {
	// TAB3 format columns (37 columns total):
	// 0: #BioGRID Interaction ID
	// 1-2: Entrez Gene IDs (A, B)
	// 3-4: BioGRID IDs (A, B)
	// 5-6: Systematic Names (A, B)
	// 7-8: Official Symbols (A, B)
	// 9-10: Synonyms (A, B)
	// 11: Experimental System (e.g., "Two-hybrid")
	// 12: Experimental System Type ("physical" or "genetic")
	// 13: Author
	// 14: Publication Source (e.g., "PUBMED:9006895")
	// 15-16: Organism IDs (A, B)
	// 17: Throughput ("Low Throughput" or "High Throughput")
	// 18: Score
	// 19: Modification (for genetic interactions)
	// 20: Qualifications
	// 21: Tags
	// 22: Source Database
	// 23-28: UniProt/RefSeq accessions (A, B)
	// 29-34: Ontology terms
	// 35-36: Organism Names (A, B)

	// Extract interaction ID (column 0)
	interactionID := b.getFieldByIndex(row, tab3InteractionID)

	// Extract partner Entrez ID (column 2 for interactor B)
	partnerEntrezID := b.getFieldByIndex(row, tab3EntrezB)
	if partnerEntrezID == "-" {
		partnerEntrezID = ""
	}

	// Extract partner symbol (column 8 for interactor B)
	partnerSymbol := b.getFieldByIndex(row, tab3SymbolB)
	if partnerSymbol == "-" {
		partnerSymbol = ""
	}

	// Extract partner systematic name (column 6 for interactor B)
	partnerSystematicName := b.getFieldByIndex(row, tab3SystematicNameB)
	if partnerSystematicName == "-" {
		partnerSystematicName = ""
	}

	// Extract experimental system (column 11)
	experimentalSystem := b.getFieldByIndex(row, tab3ExperimentalSystem)

	// Extract experimental system type (column 12)
	experimentalSystemType := b.getFieldByIndex(row, tab3ExperimentalType)

	// Extract taxonomy IDs (columns 15-16)
	taxidA := b.parseOrganismID(b.getFieldByIndex(row, tab3OrganismIDA))
	taxidB := b.parseOrganismID(b.getFieldByIndex(row, tab3OrganismIDB))

	// Extract publication info (column 14)
	pubSource := b.getFieldByIndex(row, tab3PubSource)
	pubmedID := ""
	if strings.HasPrefix(pubSource, "PUBMED:") {
		pubmedID = strings.TrimPrefix(pubSource, "PUBMED:")
	}
	author := b.getFieldByIndex(row, tab3Author)

	// Extract score (column 18)
	score := b.getFieldByIndex(row, tab3Score)
	if score == "-" {
		score = ""
	}

	// Extract source database (column 22)
	sourceDB := b.getFieldByIndex(row, tab3SourceDatabase)

	// *** NEW TAB3 FIELDS ***

	// Extract throughput (column 17) - "Low Throughput" or "High Throughput"
	throughput := b.getFieldByIndex(row, tab3Throughput)
	if throughput == "-" {
		throughput = ""
	}

	// Extract modification (column 19) - for genetic interactions (e.g., "Synthetic Lethality")
	modification := b.getFieldByIndex(row, tab3Modification)
	if modification == "-" {
		modification = ""
	}

	// Extract qualifications (column 20)
	qualifications := b.getFieldByIndex(row, tab3Qualifications)
	if qualifications == "-" {
		qualifications = ""
	}

	// Extract tags (column 21)
	tags := b.getFieldByIndex(row, tab3Tags)
	if tags == "-" {
		tags = ""
	}

	// Extract ontology term IDs (column 29) - for phenotype
	ontologyTermID := b.getFieldByIndex(row, tab3OntologyTermIDs)
	if ontologyTermID == "-" {
		ontologyTermID = ""
	}

	// Extract ontology term names (column 30) - for phenotype description
	phenotype := b.getFieldByIndex(row, tab3OntologyTermNames)
	if phenotype == "-" {
		phenotype = ""
	}

	return &pbuf.BiogridInteraction{
		InteractionId:          interactionID,
		PartnerBiogridId:       biogridB,
		PartnerEntrezId:        partnerEntrezID,
		PartnerSymbol:          partnerSymbol,
		PartnerSystematicName:  partnerSystematicName,
		ExperimentalSystem:     experimentalSystem,
		ExperimentalSystemType: experimentalSystemType,
		OrganismA:              taxidA,
		OrganismB:              taxidB,
		PubmedId:               pubmedID,
		Author:                 author,
		Score:                  score,
		SourceDatabase:         sourceDB,
		// New TAB3 fields
		Throughput:     throughput,
		Modification:   modification,
		Qualifications: qualifications,
		Tags:           tags,
		OntologyTermId: ontologyTermID,
		Phenotype:      phenotype,
	}
}

// saveInteractor saves an interactor with all its interactions
func (b *biogrid) saveInteractor(biogridID string, agg *biogridAggregator, sourceID string) {
	// Convert maps to slices
	organisms := make([]int32, 0, len(agg.organisms))
	for taxid := range agg.organisms {
		organisms = append(organisms, taxid)
	}

	experimentalSystems := make([]string, 0, len(agg.experimentalSystems))
	for sys := range agg.experimentalSystems {
		experimentalSystems = append(experimentalSystems, sys)
	}

	pubmedIDs := make([]string, 0, len(agg.pubmedIDs))
	for pmid := range agg.pubmedIDs {
		pubmedIDs = append(pubmedIDs, pmid)
	}

	attr := &pbuf.BiogridAttr{
		BiogridId:           biogridID,
		Interactions:        agg.interactions,
		InteractionCount:    int32(len(agg.interactions)),
		UniquePartners:      int32(len(agg.uniquePartners)),
		PhysicalCount:       int32(agg.physicalCount),
		GeneticCount:        int32(agg.geneticCount),
		Organisms:           organisms,
		ExperimentalSystems: experimentalSystems,
		PubmedIds:           pubmedIDs,
	}

	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	b.check(err, fmt.Sprintf("marshaling BioGRID attributes for %s", biogridID))

	// Save entry
	b.d.addProp3(biogridID, sourceID, attrBytes)

	// Create cross-references
	b.createCrossReferences(biogridID, agg, sourceID)
}

// createCrossReferences creates cross-references from interactors to other entities
func (b *biogrid) createCrossReferences(biogridID string, agg *biogridAggregator, sourceID string) {
	textLinkID := "0" // Text search link ID

	// BioGRID ID → BioGRID (text search)
	b.d.addXref(biogridID, textLinkID, biogridID, b.source, true)

	// Gene symbol → BioGRID (text search)
	if agg.symbol != "" && len(agg.symbol) < 50 {
		b.d.addXref(agg.symbol, textLinkID, biogridID, b.source, true)
	}

	// BioGRID ↔ Entrez Gene (addXref creates bidirectional mapping automatically)
	if agg.entrezID != "" {
		b.d.addXref(biogridID, sourceID, agg.entrezID, "entrez", false)
	}

	// BioGRID ↔ UniProt (bidirectional)
	for _, uniprotID := range agg.uniprotIDs {
		if uniprotID != "" {
			b.d.addXref(biogridID, sourceID, uniprotID, "uniprot", false)
		}
	}

	// BioGRID ↔ RefSeq (bidirectional)
	for _, refseqID := range agg.refseqIDs {
		if refseqID != "" {
			b.d.addXref(biogridID, sourceID, refseqID, "refseq", false)
		}
	}

	// BioGRID → Taxonomy (for primary organism)
	for taxid := range agg.organisms {
		if taxid > 0 {
			b.d.addXref(biogridID, sourceID, strconv.Itoa(int(taxid)), "taxonomy", false)
		}
	}

	// BioGRID → PubMed (limit to first 10 to avoid excessive xrefs)
	pmCount := 0
	for pmid := range agg.pubmedIDs {
		if pmid != "" && pmCount < 10 {
			// Validate PubMed ID is numeric
			if len(pmid) > 0 && pmid[0] >= '0' && pmid[0] <= '9' {
				b.d.addXref(biogridID, sourceID, pmid, "pubmed", false)
				pmCount++
			}
		}
	}

	// BioGRID → partner BioGRID entries (interaction partners)
	partnerCount := 0
	for partnerID := range agg.uniquePartners {
		if partnerID != "" && partnerCount < 100 {
			b.d.addXref(biogridID, sourceID, partnerID, b.source, false)
			partnerCount++
		}
	}
}

// Helper: readerAtFromBytes creates a ReaderAt from byte slice (needed for zip.NewReader)
func biogridReaderAtFromBytes(b []byte) io.ReaderAt {
	return &biogridBytesReaderAt{b: b}
}

type biogridBytesReaderAt struct {
	b []byte
}

func (r *biogridBytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n = copy(p, r.b[off:])
	if n < len(p) {
		err = io.EOF
	}
	return
}
