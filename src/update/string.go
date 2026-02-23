package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
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

// uniprotPriority returns priority score for UniProt accession (lower = better)
// P + 5 chars = 1 (classic Swiss-Prot, highest priority)
// Q + 5 chars = 2
// O + 5 chars = 3
// Everything else (A0A..., etc.) = 9 (lowest priority)
func uniprotPriority(acc string) int {
	if len(acc) != 6 {
		return 9 // Non-canonical length
	}
	switch acc[0] {
	case 'P':
		return 1
	case 'Q':
		return 2
	case 'O':
		return 3
	default:
		return 9
	}
}

// resolveCanonicalUniprot looks up a UniProt ID and resolves it to the canonical/primary accession
// For example: A0A... or secondary accessions -> P04637 (primary)
// Returns the input if lookup fails or no better match found
func (s *stringProcessor) resolveCanonicalUniprot(uniprotID string) string {
	if s.d == nil || uniprotID == "" {
		return uniprotID
	}

	// Already canonical (P + 5 chars)? No need to lookup
	if uniprotPriority(uniprotID) == 1 {
		return uniprotID
	}

	// Get UniProt dataset ID
	uniprotConfig, ok := config.Dataconf["uniprot"]
	if !ok {
		return uniprotID
	}
	uniprotDatasetID, err := strconv.ParseUint(uniprotConfig["id"], 10, 32)
	if err != nil {
		return uniprotID
	}

	// Use lookupInDataset - it searches text link entries and returns the target identifier
	entry, err := s.d.lookupInDataset(uniprotID, uint32(uniprotDatasetID))
	if err != nil {
		return uniprotID
	}

	if entry != nil && entry.Identifier != "" {
		return entry.Identifier
	}

	return uniprotID
}

// Main update entry point
func (s *stringProcessor) update(selectedTaxids []int) {
	defer s.d.wg.Done()

	s.sourceID = config.Dataconf[s.source]["id"]

	// Get string_interaction dataset ID for creating interaction entries
	var stringInteractionSourceID string
	if _, exists := config.Dataconf["string_interaction"]; exists {
		stringInteractionSourceID = config.Dataconf["string_interaction"]["id"]
	}

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
		// Step 1: Build STRING_ID ↔ UniProt mappings from aliases file
		forwardMap, reverseMap, err := s.buildAliasMap(taxid)
		if err != nil {
			log.Printf("Error building alias map for taxid %d: %v", taxid, err)
			continue
		}

		// Step 2: Load protein info (names, sizes, annotations)
		proteinInfo, err := s.loadProteinInfo(taxid)
		if err != nil {
			log.Printf("Error loading protein info for taxid %d: %v", taxid, err)
			continue
		}

		// Step 3: Process interactions
		proteins, interactions, err := s.processInteractions(taxid, forwardMap, reverseMap, proteinInfo, idLogFile, testLimit, stringInteractionSourceID)
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
	reader := bufio.NewReaderSize(br, 1024*1024) // 1MB buffer
	lineNum := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, nil, fmt.Errorf("error reading aliases file: %v", err)
		}
		if len(line) == 0 && err == io.EOF {
			break
		}

		lineNum++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip header
		if lineNum == 1 {
			if err == io.EOF {
				break
			}
			continue
		}

		// Format: string_protein_id	alias	source
		// Split by tab
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			if err == io.EOF {
				break
			}
			continue
		}

		stringID := fields[0]
		alias := fields[1]
		source := fields[2]

		// Store ALL UniProt_AC mappings in BOTH directions
		// This ensures all UniProt accessions (canonical + secondary/isoforms) get STRING entries
		if source == "UniProt_AC" {
			// Forward map: just store one UniProt AC per STRING ID
			// The actual canonical resolution happens later via biobtree lookup
			if _, hasExisting := forwardMap[stringID]; !hasExisting {
				forwardMap[stringID] = alias
			}
			// Reverse map: ALL UniProt ACs → their STRING ID
			reverseMap[alias] = stringID
		}

		if err == io.EOF {
			break
		}
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
	reader := bufio.NewReaderSize(br, 1024*1024) // 1MB buffer
	lineNum := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("error reading info file: %v", err)
		}
		if len(line) == 0 && err == io.EOF {
			break
		}

		lineNum++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip header
		if lineNum == 1 {
			if err == io.EOF {
				break
			}
			continue
		}

		// Format: string_protein_id	preferred_name	protein_size	annotation
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			if err == io.EOF {
				break
			}
			continue
		}

		stringID := fields[0]
		preferredName := fields[1]
		proteinSizeStr := fields[2]
		annotation := fields[3]

		proteinSize, parseErr := strconv.ParseInt(proteinSizeStr, 10, 32)
		if parseErr != nil {
			proteinSize = 0
		}

		infoMap[stringID] = &ProteinInfo{
			PreferredName: preferredName,
			ProteinSize:   int32(proteinSize),
			Annotation:    annotation,
		}

		if err == io.EOF {
			break
		}
	}

	return infoMap, nil
}

// Process interactions from links.detailed file
func (s *stringProcessor) processInteractions(taxid int, forwardMap map[string]string, reverseMap map[string]string,
	proteinInfo map[string]*ProteinInfo, idLogFile *os.File, testLimit int, stringInteractionSourceID string) (uint64, uint64, error) {

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

	// Track interaction counts by STRING ID (not UniProt AC)
	// Actual interactions are stored as separate string_interaction entries
	stringInteractionCounts := make(map[string]int32)

	reader := bufio.NewReaderSize(br, 1024*1024) // 1MB buffer

	lineNum := 0
	var totalRead int
	var previous int64

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return 0, 0, fmt.Errorf("error reading links file: %v", err)
		}
		if len(line) == 0 && err == io.EOF {
			break
		}

		lineNum++
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		// Skip header
		if lineNum == 1 {
			if err == io.EOF {
				break
			}
			continue
		}

		// Format: protein1 protein2 neighborhood fusion cooccurence coexpression experimental database textmining combined_score
		// Space-separated (NOT tab-separated!)
		fields := strings.Fields(line)
		if len(fields) < 10 {
			if err == io.EOF {
				break
			}
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
			if err == io.EOF {
				break
			}
			continue
		}

		// Map STRING IDs to UniProt ACs using forwardMap
		uniprot1, ok1 := forwardMap[protein1]
		uniprot2, ok2 := forwardMap[protein2]

		// Skip if either protein doesn't have UniProt mapping
		if !ok1 || !ok2 {
			if err == io.EOF {
				break
			}
			continue
		}

		// Resolve to canonical UniProt IDs using biobtree lookup
		// This converts secondary/non-canonical accessions (A0A..., Q...) to primary (P...)
		uniprot1 = s.resolveCanonicalUniprot(uniprot1)
		uniprot2 = s.resolveCanonicalUniprot(uniprot2)

		// Create bidirectional interactions as separate string_interaction entries
		// Store by STRING ID (not UniProt AC) so ALL UniProt ACs for same STRING ID get interactions
		// ID format: {STRING_ID}_{INVERTED_SCORE}_{UNIPROT_PARTNER}
		// Inverted score (1000-score) ensures highest scores sort first lexicographically

		// Protein1 → Protein2
		invertedScore := fmt.Sprintf("%04d", 1000-combinedScore)
		interactionID12 := protein1 + "_" + invertedScore + "_" + uniprot2
		interaction12 := &pbuf.StringInteractionAttr{
			ProteinA:        protein1,
			ProteinB:        protein2,
			UniprotA:        uniprot1,
			UniprotB:        uniprot2,
			Score:           int32(combinedScore),
			HasExperimental: experimental > 0,
			HasDatabase:     database > 0,
			HasTextmining:   textmining > 0,
			HasCoexpression: coexpression > 0,
		}

		// Compute sort levels for interaction score (higher scores first)
		scoreSortLevels := []string{
			ComputeSortLevelValue(SortLevelInteractionScore, map[string]interface{}{"score": combinedScore}),
		}

		// Marshal and store interaction entry
		b12, errMarshal := ffjson.Marshal(interaction12)
		if errMarshal == nil && stringInteractionSourceID != "" {
			s.d.addProp3(interactionID12, stringInteractionSourceID, b12)
			// Xref: string -> string_interaction
			s.d.addXref(protein1, s.sourceID, interactionID12, "string_interaction", false)
			// Xref: string_interaction -> uniprot (BOTH proteins) - sorted by score
			// This enables queries like: uniprot >> string_interaction
			s.d.addXrefWithSortLevels(interactionID12, stringInteractionSourceID, uniprot1, "uniprot", scoreSortLevels)
			s.d.addXrefWithSortLevels(interactionID12, stringInteractionSourceID, uniprot2, "uniprot", scoreSortLevels)
		}
		stringInteractionCounts[protein1]++

		// Protein2 → Protein1 (bidirectional)
		interactionID21 := protein2 + "_" + invertedScore + "_" + uniprot1
		interaction21 := &pbuf.StringInteractionAttr{
			ProteinA:        protein2,
			ProteinB:        protein1,
			UniprotA:        uniprot2,
			UniprotB:        uniprot1,
			Score:           int32(combinedScore),
			HasExperimental: experimental > 0,
			HasDatabase:     database > 0,
			HasTextmining:   textmining > 0,
			HasCoexpression: coexpression > 0,
		}

		// Marshal and store interaction entry
		b21, errMarshal := ffjson.Marshal(interaction21)
		if errMarshal == nil && stringInteractionSourceID != "" {
			s.d.addProp3(interactionID21, stringInteractionSourceID, b21)
			// Xref: string -> string_interaction
			s.d.addXref(protein2, s.sourceID, interactionID21, "string_interaction", false)
			// Xref: string_interaction -> uniprot (BOTH proteins) - sorted by score
			s.d.addXrefWithSortLevels(interactionID21, stringInteractionSourceID, uniprot1, "uniprot", scoreSortLevels)
			s.d.addXrefWithSortLevels(interactionID21, stringInteractionSourceID, uniprot2, "uniprot", scoreSortLevels)
		}
		stringInteractionCounts[protein2]++

		totalRead += len(line)

		// In test mode, stop reading once we have enough interactions
		// Note: Total protein count will be determined later from reverseMap
		if testLimit > 0 && len(stringInteractionCounts) >= testLimit {
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

		if err == io.EOF {
			break
		}
	}

	// Build protein data keyed by STRING ID (primary identifier)
	// Interactions are already stored as separate string_interaction entries
	fmt.Printf("DEBUG: Total STRING IDs with interactions: %d\n", len(stringInteractionCounts))
	fmt.Printf("DEBUG: Total STRING IDs with protein info: %d\n", len(proteinInfo))

	// Debug: Check overlap between stringInteractionCounts and proteinInfo
	overlapCount := 0
	sampleWithInteractions := []string{}
	for stringID := range stringInteractionCounts {
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
		// Get interaction count for this STRING ID (may be 0)
		interactionCount := stringInteractionCounts[stringID]

		// Create STRING attribute (interactions stored separately in string_interaction dataset)
		attr := pbuf.StringAttr{
			StringId:         stringID,
			OrganismTaxid:    int32(taxid),
			PreferredName:    info.PreferredName,
			ProteinSize:      info.ProteinSize,
			Annotation:       info.Annotation,
			InteractionCount: interactionCount,
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
		totalInteractions += uint64(interactionCount)
		return true
	}

	// In test mode, prioritize proteins with interactions
	if testLimit > 0 {
		// First, process proteins WITH interactions
		for stringID := range stringInteractionCounts {
			if info, exists := proteinInfo[stringID]; exists {
				if processProtein(stringID, info) {
					processedIDs++
					if config.IsTestMode() && shouldStopProcessing(testLimit, processedIDs) {
						fmt.Printf("DEBUG: Stopped after processing %d proteins with interactions\n", processedIDs)
						return totalProteins, totalInteractions, nil
					}
				}
			}
		}

		// Then fill out with proteins WITHOUT interactions (up to test limit)
		for stringID, info := range proteinInfo {
			// Skip if already processed (has interactions)
			if _, hasInteractions := stringInteractionCounts[stringID]; hasInteractions {
				continue
			}

			if processProtein(stringID, info) {
				processedIDs++
				if config.IsTestMode() && shouldStopProcessing(testLimit, processedIDs) {
					fmt.Printf("DEBUG: Stopped after processing %d total proteins (%d with interactions)\n",
						processedIDs, len(stringInteractionCounts))
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
