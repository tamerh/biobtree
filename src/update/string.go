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

	// fmt.Printf("STRING score threshold: %d\n", s.scoreThreshold)

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

		// Step 1: Build STRING_ID → UniProt mapping from aliases file
		aliasMap, err := s.buildAliasMap(taxid)
		if err != nil {
			log.Printf("Error building alias map for taxid %d: %v", taxid, err)
			continue
		}
		// fmt.Printf("  Loaded %d STRING ID → UniProt mappings\n", len(aliasMap))

		// Step 2: Load protein info (names, sizes, annotations)
		proteinInfo, err := s.loadProteinInfo(taxid)
		if err != nil {
			log.Printf("Error loading protein info for taxid %d: %v", taxid, err)
			continue
		}
		// fmt.Printf("  Loaded %d protein annotations\n", len(proteinInfo))

		// Step 3: Process interactions
		proteins, interactions, err := s.processInteractions(taxid, aliasMap, proteinInfo, idLogFile, testLimit)
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

// Build STRING_ID → UniProt mapping from aliases file
func (s *stringProcessor) buildAliasMap(taxid int) (map[string]string, error) {
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
		return nil, fmt.Errorf("failed to open aliases file: %v", err)
	}
	defer closeReaders(gz, ftpFile, client, localFile)

	aliasMap := make(map[string]string)
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

		// Only map UniProt_AC entries to STRING IDs
		// Use the first UniProt AC we encounter (primary mapping)
		if source == "UniProt_AC" {
			if _, exists := aliasMap[stringID]; !exists {
				aliasMap[stringID] = alias // alias is the UniProt AC
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading aliases file: %v", err)
	}

	return aliasMap, nil
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
func (s *stringProcessor) processInteractions(taxid int, aliasMap map[string]string,
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

	// Track proteins we've processed and their interactions
	proteinInteractions := make(map[string][]*pbuf.StringInteraction)
	proteinData := make(map[string]*StringProteinData)

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

		// Map STRING IDs to UniProt ACs
		uniprot1, ok1 := aliasMap[protein1]
		uniprot2, ok2 := aliasMap[protein2]

		// Skip if either protein doesn't have UniProt mapping
		if !ok1 || !ok2 {
			continue
		}

		// Create bidirectional interactions
		// Protein1 → Protein2
		interaction12 := &pbuf.StringInteraction{
			Partner:          uniprot2,
			Score:            int32(combinedScore),
			HasExperimental:  experimental > 0,
			HasDatabase:      database > 0,
			HasTextmining:    textmining > 0,
			HasCoexpression:  coexpression > 0,
		}
		proteinInteractions[uniprot1] = append(proteinInteractions[uniprot1], interaction12)

		// Protein2 → Protein1 (bidirectional)
		interaction21 := &pbuf.StringInteraction{
			Partner:          uniprot1,
			Score:            int32(combinedScore),
			HasExperimental:  experimental > 0,
			HasDatabase:      database > 0,
			HasTextmining:    textmining > 0,
			HasCoexpression:  coexpression > 0,
		}
		proteinInteractions[uniprot2] = append(proteinInteractions[uniprot2], interaction21)

		// Store protein data for both proteins
		if _, exists := proteinData[uniprot1]; !exists {
			if info, ok := proteinInfo[protein1]; ok {
				proteinData[uniprot1] = &StringProteinData{
					StringID:      protein1,
					OrganismTaxid: int32(taxid),
					PreferredName: info.PreferredName,
					ProteinSize:   info.ProteinSize,
					Annotation:    info.Annotation,
				}
			}
		}
		if _, exists := proteinData[uniprot2]; !exists {
			if info, ok := proteinInfo[protein2]; ok {
				proteinData[uniprot2] = &StringProteinData{
					StringID:      protein2,
					OrganismTaxid: int32(taxid),
					PreferredName: info.PreferredName,
					ProteinSize:   info.ProteinSize,
					Annotation:    info.Annotation,
				}
			}
		}

		totalRead += len(line)

		// In test mode, stop reading once we have enough unique proteins
		if testLimit > 0 && len(proteinData) >= testLimit {
			// fmt.Printf("  [TEST MODE] Reached limit of %d proteins, stopping interaction processing\n", testLimit)
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

	// Store all protein data with interactions
	var totalProteins uint64
	var totalInteractions uint64
	var processedIDs int

	for uniprotID, interactions := range proteinInteractions {
		data, ok := proteinData[uniprotID]
		if !ok {
			continue
		}

		// Create STRING attribute
		attr := pbuf.StringAttr{
			StringId:      data.StringID,
			OrganismTaxid: data.OrganismTaxid,
			PreferredName: data.PreferredName,
			ProteinSize:   data.ProteinSize,
			Annotation:    data.Annotation,
			Interactions:  interactions,
		}

		// Marshal STRING attributes
		b, err := ffjson.Marshal(&attr)
		if err != nil {
			log.Printf("Error marshaling STRING attr for %s: %v", uniprotID, err)
			continue
		}

		// Store attributes on UniProt ID (primary identifier for STRING dataset)
		s.d.addProp3(uniprotID, s.sourceID, b)

		// Create keyword: STRING ID → UniProt entry (for search endpoint)
		// isLink=true means /ws/?i=STRING_ID will find and return the UniProt entry
		// Note: /ws/entry/ requires actual identifier (UniProt ID), not keyword
		s.d.addXref(data.StringID, textLinkID, uniprotID, s.source, true)

		// Log UniProt ID in test mode (primary key where attributes are stored)
		if idLogFile != nil {
			logProcessedID(idLogFile, uniprotID)
			processedIDs++
			if shouldStopProcessing(testLimit, processedIDs) {
				break
			}
		}

		totalProteins++
		totalInteractions += uint64(len(interactions))
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
