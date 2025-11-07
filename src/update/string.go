package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pquerna/ffjson/ffjson"
)

type stringProcessor struct {
	source       string
	sourceID     string
	d            *DataUpdate
	scoreThreshold int
}

// Main update entry point
func (s *stringProcessor) update(selectedTaxids []int) {
	defer s.d.wg.Done()

	s.sourceID = config.Dataconf[s.source]["id"]

	// Get score threshold from config (default: 400)
	scoreThresholdStr, ok := config.Dataconf[s.source]["scoreThreshold"]
	if !ok {
		s.scoreThreshold = 400
	} else {
		threshold, err := strconv.Atoi(scoreThresholdStr)
		if err != nil {
			log.Printf("Warning: Invalid scoreThreshold in config, using default 400")
			s.scoreThreshold = 400
		} else {
			s.scoreThreshold = threshold
		}
	}

	fmt.Printf("STRING score threshold: %d\n", s.scoreThreshold)

	// Test mode support
	testLimit := config.GetTestLimit(s.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, s.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var totalProteins uint64
	var totalInteractions uint64

	// Process each selected organism
	if len(selectedTaxids) == 0 {
		log.Fatal("No taxonomies selected for STRING. Use --tax flag to specify organisms (e.g., --tax 9606)")
	}

	for _, taxid := range selectedTaxids {
		// fmt.Printf("Processing STRING data for taxid %d...\n", taxid)

		// Step 1: Build STRING_ID ↔ UniProt mappings from aliases file
		forwardMap, reverseMap, err := s.buildAliasMap(taxid)
		if err != nil {
			log.Printf("Error building alias map for taxid %d: %v", taxid, err)
			continue
		}
		// fmt.Printf("  Loaded %d STRING ID → UniProt mappings\n", len(forwardMap))

		// Step 2: Load protein info (names, sizes, annotations)
		proteinInfo, err := s.loadProteinInfo(taxid)
		if err != nil {
			log.Printf("Error loading protein info for taxid %d: %v", taxid, err)
			continue
		}
		// fmt.Printf("  Loaded %d protein annotations\n", len(proteinInfo))

		// Step 3: Process interactions
		proteins, interactions, err := s.processInteractions(taxid, forwardMap, reverseMap, proteinInfo, idLogFile, testLimit)
		if err != nil {
			log.Printf("Error processing interactions for taxid %d: %v", taxid, err)
			continue
		}

		totalProteins += proteins
		totalInteractions += interactions
		fmt.Printf("  Processed taxid %d: %d proteins with %d interactions\n", taxid, proteins, interactions)
	}

	fmt.Printf("STRING processing complete: %d proteins, %d interactions across %d organisms\n",
		totalProteins, totalInteractions, len(selectedTaxids))

	atomic.AddUint64(&s.d.totalParsedEntry, totalProteins)
	s.d.addEntryStat(s.source, totalProteins)
	s.d.progChan <- &progressInfo{dataset: s.source, done: true}
}

// Build STRING_ID ↔ UniProt mappings from aliases file
// Returns: forwardMap (STRING_ID → UniProt_AC), reverseMap (UniProt_AC → STRING_ID)
func (s *stringProcessor) buildAliasMap(taxid int) (map[string]string, map[string]string, error) {
	var filePath string

	// Check if using local files (test mode) or remote download
	if _, ok := config.Dataconf[s.source]["useLocalFile"]; ok && config.Dataconf[s.source]["useLocalFile"] == "yes" {
		// Use local test files
		basePath := config.Dataconf[s.source]["path"]
		filePath = fmt.Sprintf("%s%d.protein.aliases.txt", basePath, taxid)
	} else {
		// Use remote download URL
		filePath = strings.Replace(config.Dataconf[s.source]["downloadUrlAliases"], "{taxid}", strconv.Itoa(taxid), -1)
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(s.source, "", "", filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open aliases file: %v", err)
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	forwardMap := make(map[string]string) // STRING_ID → UniProt_AC (for interactions)
	reverseMap := make(map[string]string) // UniProt_AC → STRING_ID (for complete coverage)
	scanner := bufio.NewScanner(br)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip header
		if lineNum == 1 {
			continue
		}

		// Format: string_protein_id	alias	source
		// Split by tab
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		stringID := fields[0]
		alias := fields[1]
		source := fields[2]

		// Store ALL UniProt_AC mappings in BOTH directions
		// This ensures all UniProt accessions (canonical + secondary/isoforms) get STRING entries
		if source == "UniProt_AC" {
			// Forward map: use last one for interactions (not critical which)
			forwardMap[stringID] = alias
			// Reverse map: ALL UniProt ACs → their STRING ID
			reverseMap[alias] = stringID
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error reading aliases file: %v", err)
	}

	fmt.Printf("DEBUG: Built forwardMap with %d STRING IDs, reverseMap with %d UniProt ACs\n", len(forwardMap), len(reverseMap))

	// Return both maps
	return forwardMap, reverseMap, nil
}

// Load protein info (preferred names, sizes, annotations)
type ProteinInfo struct {
	PreferredName string
	ProteinSize   int32
	Annotation    string
}

func (s *stringProcessor) loadProteinInfo(taxid int) (map[string]*ProteinInfo, error) {
	var filePath string

	// Check if using local files (test mode) or remote download
	if _, ok := config.Dataconf[s.source]["useLocalFile"]; ok && config.Dataconf[s.source]["useLocalFile"] == "yes" {
		// Use local test files
		basePath := config.Dataconf[s.source]["path"]
		filePath = fmt.Sprintf("%s%d.protein.info.txt", basePath, taxid)
	} else {
		// Use remote download URL
		filePath = strings.Replace(config.Dataconf[s.source]["downloadUrlInfo"], "{taxid}", strconv.Itoa(taxid), -1)
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(s.source, "", "", filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open info file: %v", err)
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	infoMap := make(map[string]*ProteinInfo)
	scanner := bufio.NewScanner(br)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip header
		if lineNum == 1 {
			continue
		}

		// Format: string_protein_id	preferred_name	protein_size	annotation
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}

		stringID := fields[0]
		preferredName := fields[1]
		proteinSizeStr := fields[2]
		annotation := fields[3]

		proteinSize, err := strconv.ParseInt(proteinSizeStr, 10, 32)
		if err != nil {
			proteinSize = 0
		}

		infoMap[stringID] = &ProteinInfo{
			PreferredName: preferredName,
			ProteinSize:   int32(proteinSize),
			Annotation:    annotation,
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading info file: %v", err)
	}

	return infoMap, nil
}

// Process interactions from links.detailed file
func (s *stringProcessor) processInteractions(taxid int, forwardMap map[string]string, reverseMap map[string]string,
	proteinInfo map[string]*ProteinInfo, idLogFile *os.File, testLimit int) (uint64, uint64, error) {

	var filePath string

	// Check if using local files (test mode) or remote download
	if _, ok := config.Dataconf[s.source]["useLocalFile"]; ok && config.Dataconf[s.source]["useLocalFile"] == "yes" {
		// Use local test files
		basePath := config.Dataconf[s.source]["path"]
		filePath = fmt.Sprintf("%s%d.protein.links.detailed.txt", basePath, taxid)
	} else {
		// Use remote download URL
		filePath = strings.Replace(config.Dataconf[s.source]["downloadUrlLinks"], "{taxid}", strconv.Itoa(taxid), -1)
	}

	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(s.source, "", "", filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open links file: %v", err)
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	// Track protein interactions by STRING ID (not UniProt AC)
	// This allows all UniProt ACs mapping to same STRING ID to share interactions
	stringInteractions := make(map[string][]*pbuf.StringInteraction)

	scanner := bufio.NewScanner(br)
	// Increase buffer size for large lines
	const maxCapacity = 1024 * 1024 // 1MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0
	var totalRead int
	var previous int64

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip header
		if lineNum == 1 {
			continue
		}

		// Format: protein1 protein2 neighborhood fusion cooccurence coexpression experimental database textmining combined_score
		// Space-separated (NOT tab-separated!)
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		protein1 := fields[0]
		protein2 := fields[1]

		// Parse evidence channels
		experimental, _ := strconv.Atoi(fields[5])
		database, _ := strconv.Atoi(fields[6])
		textmining, _ := strconv.Atoi(fields[7])
		coexpression, _ := strconv.Atoi(fields[4])
		combinedScore, _ := strconv.Atoi(fields[8])

		// Filter by score threshold
		if combinedScore < s.scoreThreshold {
			continue
		}

		// Map STRING IDs to UniProt ACs using forwardMap
		uniprot1, ok1 := forwardMap[protein1]
		uniprot2, ok2 := forwardMap[protein2]

		// Skip if either protein doesn't have UniProt mapping
		if !ok1 || !ok2 {
			continue
		}

		// Create bidirectional interactions
		// Store by STRING ID (not UniProt AC) so ALL UniProt ACs for same STRING ID get interactions
		// Protein1 → Protein2
		interaction12 := &pbuf.StringInteraction{
			Partner:          uniprot2,
			Score:            int32(combinedScore),
			HasExperimental:  experimental > 0,
			HasDatabase:      database > 0,
			HasTextmining:    textmining > 0,
			HasCoexpression:  coexpression > 0,
		}
		stringInteractions[protein1] = append(stringInteractions[protein1], interaction12)

		// Protein2 → Protein1 (bidirectional)
		interaction21 := &pbuf.StringInteraction{
			Partner:          uniprot1,
			Score:            int32(combinedScore),
			HasExperimental:  experimental > 0,
			HasDatabase:      database > 0,
			HasTextmining:    textmining > 0,
			HasCoexpression:  coexpression > 0,
		}
		stringInteractions[protein2] = append(stringInteractions[protein2], interaction21)

		totalRead += len(line)

		// In test mode, stop reading once we have enough interactions
		// Note: Total protein count will be determined later from reverseMap
		if testLimit > 0 && len(stringInteractions) >= testLimit {
			// fmt.Printf("  [TEST MODE] Reached limit of %d proteins with interactions, stopping interaction processing\n", testLimit)
			break
		}

		// Progress reporting
		elapsed := int64(time.Since(s.d.start).Seconds())
		if elapsed > previous+s.d.progInterval {
			kbytesPerSecond := int64(totalRead) / elapsed / 1024
			previous = elapsed
			s.d.progChan <- &progressInfo{dataset: s.source, currentKBPerSec: kbytesPerSecond}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("error reading links file: %v", err)
	}

	// Build protein data keyed by STRING ID (primary identifier)
	// This eliminates duplication - each interaction list stored only once
	fmt.Printf("DEBUG: Total STRING IDs with interactions: %d\n", len(stringInteractions))
	fmt.Printf("DEBUG: Total STRING IDs with protein info: %d\n", len(proteinInfo))

	// Debug: Check overlap between stringInteractions and proteinInfo
	overlapCount := 0
	sampleWithInteractions := []string{}
	for stringID := range stringInteractions {
		if len(sampleWithInteractions) < 5 {
			sampleWithInteractions = append(sampleWithInteractions, stringID)
		}
		if _, exists := proteinInfo[stringID]; exists {
			overlapCount++
		}
	}
	fmt.Printf("DEBUG: Overlap (proteins with both interactions and info): %d\n", overlapCount)
	fmt.Printf("DEBUG: Sample STRING IDs with interactions: %v\n", sampleWithInteractions)

	// Store all protein data (with or without interactions)
	var totalProteins uint64
	var totalInteractions uint64
	var processedIDs int

	// Helper function to process a protein
	processProtein := func(stringID string, info *ProteinInfo) bool {
		// Get interactions for this STRING ID (may be empty/nil)
		interactions := stringInteractions[stringID]

		// Create STRING attribute
		attr := pbuf.StringAttr{
			StringId:      stringID,
			OrganismTaxid: int32(taxid),
			PreferredName: info.PreferredName,
			ProteinSize:   info.ProteinSize,
			Annotation:    info.Annotation,
			Interactions:  interactions,
		}

		// Marshal STRING attributes
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("Error marshaling STRING attr for %s: %v", stringID, err)
			return false // Don't count this protein
		}

		// Store attributes on STRING ID (primary identifier for STRING dataset)
		s.d.addProp3(stringID, s.sourceID, b)

		// Create keywords: ALL UniProt ACs → STRING entry (for search/cross-reference)
		// This allows queries by any UniProt AC to find the STRING entry
		for uniprotAC, sid := range reverseMap {
			if sid == stringID {
				// isLink=true means /ws/?i=UniProt_AC will find and return the STRING entry
				s.d.addXref(uniprotAC, textLinkID, stringID, s.source, true)

				// Bidirectional xref: STRING ID → UniProt AC (for mapping queries)
				// This allows STRING >> uniprot mapping to work
				s.d.addXref(stringID, s.sourceID, uniprotAC, "uniprot", false)
			}
		}

		// Log STRING ID in test mode (primary key where attributes are stored)
		if idLogFile != nil {
			logProcessedID(idLogFile, stringID)
		}

		totalProteins++
		totalInteractions += uint64(len(interactions))
		return true
	}

	// In test mode, prioritize proteins with interactions
	if testLimit > 0 {
		// First, process proteins WITH interactions
		for stringID := range stringInteractions {
			if info, exists := proteinInfo[stringID]; exists {
				if processProtein(stringID, info) {
					processedIDs++
					if shouldStopProcessing(testLimit, processedIDs) {
						fmt.Printf("DEBUG: Stopped after processing %d proteins with interactions\n", processedIDs)
						return totalProteins, totalInteractions, nil
					}
				}
			}
		}

		// Then fill out with proteins WITHOUT interactions (up to test limit)
		for stringID, info := range proteinInfo {
			// Skip if already processed (has interactions)
			if _, hasInteractions := stringInteractions[stringID]; hasInteractions {
				continue
			}

			if processProtein(stringID, info) {
				processedIDs++
				if shouldStopProcessing(testLimit, processedIDs) {
					fmt.Printf("DEBUG: Stopped after processing %d total proteins (%d with interactions)\n",
						processedIDs, len(stringInteractions))
					return totalProteins, totalInteractions, nil
				}
			}
		}
	} else {
		// Production mode: process all proteins
		for stringID, info := range proteinInfo {
			processProtein(stringID, info)
		}
	}

	return totalProteins, totalInteractions, nil
}

// Helper struct for protein data
type StringProteinData struct {
	StringID      string
	OrganismTaxid int32
	PreferredName string
	ProteinSize   int32
	Annotation    string
}

// Helper to close readers
func closeReaders(gz *gzip.Reader, ftpFile *ftp.Response, client *ftp.ServerConn, localFile *os.File) {
	// Close gzip reader (may be nil for uncompressed files)
	if gz != nil {
		gz.Close()
	}
	// Close FTP file (may be nil for local files)
	if ftpFile != nil {
		ftpFile.Close()
	}
	// Close FTP client (may be nil for local files)
	if client != nil {
		client.Quit()
	}
	// Close local file
	if localFile != nil {
		localFile.Close()
	}
}
