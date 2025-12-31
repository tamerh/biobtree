package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pquerna/ffjson/ffjson"
	"github.com/tamerh/jsparser"
)

type patents struct {
	source        string
	d             *DataUpdate
	dataPath      string
	testPatentIDs map[string]bool // Track patent IDs in test mode
}

func (p *patents) update() {
	defer p.d.wg.Done()

	// Process patents metadata
	totalPatents, err := p.processPatents()
	if err != nil {
		panic(fmt.Sprintf("Error processing patents: %v", err))
	}
	fmt.Printf("Completed processing patents: %d records\n", totalPatents)

	// Process compounds
	totalCompounds, err := p.processCompounds()
	if err != nil {
		panic(fmt.Sprintf("Error processing compounds: %v", err))
	}
	fmt.Printf("Completed processing compounds: %d records\n", totalCompounds)

	// Process patent-compound mappings
	totalMappings, err := p.processMappings()
	if err != nil {
		panic(fmt.Sprintf("Error processing mappings: %v", err))
	}
	fmt.Printf("Completed processing mappings: %d records\n", totalMappings)

	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
}

// parseStringList parses Python string list representation into Go string slice
// Input: "['item1' 'item2' 'item3']" or "['item1', 'item2']"
// Output: []string{"item1", "item2", "item3"}
func parseStringList(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}

	// Remove brackets and newlines
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "[]")
	s = strings.ReplaceAll(s, "\n", " ")

	// Split by quotes and extract items
	var items []string
	parts := strings.Split(s, "'")

	for i, part := range parts {
		// Items are at odd indices (1, 3, 5, ...)
		if i%2 == 1 {
			item := strings.TrimSpace(part)
			if item != "" {
				items = append(items, item)
			}
		}
	}

	return items
}

func (p *patents) processPatents() (int, error) {
	patentsFile := filepath.Join(p.dataPath, "patents.json")
	file, err := os.Open(patentsFile)
	if err != nil {
		return 0, fmt.Errorf("failed to open patents file: %w", err)
	}
	defer file.Close()

	fr := config.Dataconf[p.source]["id"]

	// Test mode setup
	testLimit := config.GetTestLimit(p.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		p.testPatentIDs = make(map[string]bool)
		idLogFile = openIDLogFile(config.TestRefDir, p.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	// Use jsparser for streaming JSON
	br := bufio.NewReader(file)
	parser := jsparser.NewJSONParser(br, "patents")

	count := 0
	var previous int64

	for j := range parser.Stream() {
		count++

		elapsed := int64(time.Since(p.d.start).Seconds())
		if elapsed > previous+p.d.progInterval {
			kbytesPerSecond := int64(parser.TotalReadSize) / elapsed / 1024
			previous = elapsed
			p.d.progChan <- &progressInfo{dataset: p.source, currentKBPerSec: kbytesPerSecond}
		}

		// Extract fields from JSON object
		patentNumber := getString(j, "patent_number")
		if patentNumber == "" {
			continue
		}

		// Test mode: Track patent IDs and check limit
		if config.IsTestMode() {
			if _, exists := p.testPatentIDs[patentNumber]; !exists {
				p.testPatentIDs[patentNumber] = true
				if idLogFile != nil {
					logProcessedID(idLogFile, patentNumber)
				}
			}
		}

		title := getString(j, "title")
		country := getString(j, "country")
		pubDate := getString(j, "publication_date")
		familyIDStr := getString(j, "family_id")

		cpcStr := getString(j, "cpc")
		ipcrStr := getString(j, "ipcr")
		ipcStr := getString(j, "ipc")
		eclaStr := getString(j, "ecla")
		assigneeStr := getString(j, "asignee")

		// Parse array fields
		cpcList := parseStringList(cpcStr)
		ipcrList := parseStringList(ipcrStr)
		ipcList := parseStringList(ipcStr)
		eclaList := parseStringList(eclaStr)
		asigneeList := parseStringList(assigneeStr)

		// Store patent attributes
		attr := pbuf.PatentAttr{
			Title:           title,
			Country:         country,
			PublicationDate: pubDate,
			FamilyId:        familyIDStr,
			Cpc:             cpcList,
			Ipcr:            ipcrList,
			Ipc:             ipcList,
			Ecla:            eclaList,
			Asignee:         asigneeList,
		}

		b, _ := ffjson.Marshal(attr)
		p.d.addProp3(patentNumber, fr, b)

		// Patent ↔ Patent Family
		if familyIDStr != "" && familyIDStr != "0" {
			p.d.addXref(patentNumber, fr, familyIDStr, "patent_family", false)
		}

		// Patent ↔ IPC codes
		for _, ipc := range ipcList {
			ipcClean := strings.TrimSpace(ipc)
			if ipcClean != "" {
				p.d.addXref(patentNumber, fr, ipcClean, "ipc", false)
			}
		}

		// Patent ↔ CPC codes
		for _, cpc := range cpcList {
			cpcClean := strings.TrimSpace(cpc)
			if cpcClean != "" {
				p.d.addXref(patentNumber, fr, cpcClean, "cpc", false)
			}
		}

		// Patent ↔ Assignees (with normalization)
		for _, assignee := range asigneeList {
			normalized := normalizeCompanyName(assignee)
			if normalized != "" {
				p.d.addXref(patentNumber, fr, normalized, "assignee", false)
			}
		}

		// Test mode: Check if we've reached the limit
		if shouldStopProcessing(testLimit, len(p.testPatentIDs)) {
			break
		}
	}

	return count, nil
}

func (p *patents) processCompounds() (int, error) {
	compoundsFile := filepath.Join(p.dataPath, "compounds.json")
	file, err := os.Open(compoundsFile)
	if err != nil {
		return 0, fmt.Errorf("failed to open compounds file: %w", err)
	}
	defer file.Close()

	// Check if lookup database is available for ChEMBL mapping
	if !p.d.hasLookupDB {
		fmt.Println("Warning: No lookup database available, patent_compound linkdataset disabled")
		fmt.Println("         Patent compounds will not be linked to ChEMBL molecules")
		return 0, nil
	}

	// Get dataset IDs
	patentCompoundDatasetID := config.Dataconf["patent_compound"]["id"]
	// Test mode setup
	testLimit := config.GetTestLimit(p.source)
	processedCompounds := make(map[string]bool) // Track unique compound IDs processed

	// Stats tracking
	totalProcessed := 0
	totalLinkedToChEMBL := 0
	totalWithKeywords := 0

	// Use jsparser for streaming JSON
	br := bufio.NewReader(file)
	parser := jsparser.NewJSONParser(br, "compounds")

	count := 0
	var previous int64

	for j := range parser.Stream() {
		count++

		elapsed := int64(time.Since(p.d.start).Seconds())
		if elapsed > previous+p.d.progInterval {
			kbytesPerSecond := int64(parser.TotalReadSize) / elapsed / 1024
			previous = elapsed
			p.d.progChan <- &progressInfo{dataset: p.source, currentKBPerSec: kbytesPerSecond}
		}

		compoundID := getString(j, "id")
		if compoundID == "" || compoundID == "0" {
			continue
		}

		inchiKey := getString(j, "inchi_key")
		smiles := getString(j, "smiles")

		// Build target datasets map for compound lookup
		// Only include datasets that are configured in this build
		targetDatasets := make(map[string]uint32)
		for _, ds := range []string{"chembl_molecule", "pubchem", "chebi", "hmdb"} {
			if _, exists := config.Dataconf[ds]; exists {
				targetDatasets[ds] = config.DataconfIDStringToInt[ds]
			}
		}

		// Lookup compounds in all target datasets by InChI key or SMILES
		var matches []compoundMatch
		if inchiKey != "" {
			matches = p.lookupCompoundsInDatasets(inchiKey, targetDatasets)
		}
		if len(matches) == 0 && smiles != "" {
			matches = p.lookupCompoundsInDatasets(smiles, targetDatasets)
		}

		// Create xrefs for all matched chemical databases
		if len(matches) > 0 {
			for _, match := range matches {
				// Forward xref: patent_compound → chemical database
				p.d.addXref2(compoundID, patentCompoundDatasetID, match.identifier, match.datasetName)

				// Reverse xref: chemical database → patent_compound
				// This enables queries like: chembl_molecule >> patent_compound >> patent
				p.d.addXref(match.identifier, match.datasetName, compoundID, "patent_compound", false)
			}
			totalLinkedToChEMBL++ // TODO: rename to totalLinkedToChemDB

			// Create keyword xrefs for InChI/SMILES → patent_compound
			if inchiKey != "" {
				p.d.addXref(inchiKey, textLinkID, compoundID, "patent_compound", true)
				totalWithKeywords++
			}
			if smiles != "" {
				p.d.addXref(smiles, textLinkID, compoundID, "patent_compound", true)
			}
		}

		totalProcessed++

		// Test mode: Track processed compounds
		if config.IsTestMode() {
			if _, exists := processedCompounds[compoundID]; !exists {
				processedCompounds[compoundID] = true
			}

			// Check if we've reached the test limit
			if shouldStopProcessing(testLimit, len(processedCompounds)) {
				break
			}
		}
	}

	fmt.Printf("Patent compounds processed: %d total, %d linked to ChEMBL, %d with keywords\n",
		totalProcessed, totalLinkedToChEMBL, totalWithKeywords)

	return count, nil
}

// compoundMatch holds a matched compound ID and its dataset
type compoundMatch struct {
	datasetName string
	identifier  string
}

// lookupCompoundsInDatasets searches for compound IDs across multiple chemical databases
// Returns matches for all found datasets
func (p *patents) lookupCompoundsInDatasets(identifier string, targetDatasets map[string]uint32) []compoundMatch {
	if identifier == "" {
		return nil
	}

	result, err := p.d.lookup(identifier)
	if err != nil || result == nil || len(result.Results) == 0 {
		return nil
	}

	var matches []compoundMatch
	found := make(map[string]bool) // Prevent duplicates

	for _, xref := range result.Results {
		if xref.IsLink {
			for _, entry := range xref.Entries {
				for datasetName, datasetID := range targetDatasets {
					if entry.Dataset == datasetID && !found[datasetName] {
						matches = append(matches, compoundMatch{datasetName, entry.Identifier})
						found[datasetName] = true
					}
				}
			}
		} else {
			for datasetName, datasetID := range targetDatasets {
				if xref.Dataset == datasetID && !found[datasetName] {
					matches = append(matches, compoundMatch{datasetName, xref.Identifier})
					found[datasetName] = true
				}
			}
		}
	}

	return matches
}

func (p *patents) processMappings() (int, error) {
	mappingFile := filepath.Join(p.dataPath, "mapping.json")
	file, err := os.Open(mappingFile)
	if err != nil {
		return 0, fmt.Errorf("failed to open mapping file: %w", err)
	}
	defer file.Close()

	fr := config.Dataconf[p.source]["id"]

	// We need to map patent IDs (strings) back to patent numbers
	// Build a map from patent.id → patent.patent_number
	patentIDMap, err := p.buildPatentIDMap()
	if err != nil {
		return 0, fmt.Errorf("failed to build patent ID map: %w", err)
	}

	// Test mode setup
	testLimit := config.GetTestLimit(p.source)
	processedMappings := 0

	// Use jsparser for streaming JSON
	br := bufio.NewReader(file)
	parser := jsparser.NewJSONParser(br, "mappings")

	count := 0
	var previous int64

	for j := range parser.Stream() {
		count++

		elapsed := int64(time.Since(p.d.start).Seconds())
		if elapsed > previous+p.d.progInterval {
			kbytesPerSecond := int64(parser.TotalReadSize) / elapsed / 1024
			previous = elapsed
			p.d.progChan <- &progressInfo{dataset: p.source, currentKBPerSec: kbytesPerSecond}
		}

		patentID := getString(j, "patent_id")
		compoundID := getString(j, "compound_id")

		if patentID == "" || patentID == "0" || compoundID == "" || compoundID == "0" {
			continue
		}

		// Get patent number from patent ID
		patentNumber, ok := patentIDMap[patentID]
		if !ok {
			// Patent not in our dataset (or filtered out in test mode)
			continue
		}

		// Patent ↔ Patent Compound (bidirectional)
		p.d.addXref(patentNumber, fr, compoundID, "patent_compound", false)

		// Test mode: Count processed mappings and check limit
		if config.IsTestMode() {
			processedMappings++
			// Use a reasonable multiplier (e.g., 3x) to get enough mappings
			// for the limited patents/compounds we're testing
			if shouldStopProcessing(testLimit*3, processedMappings) {
				break
			}
		}
	}

	return count, nil
}

func (p *patents) buildPatentIDMap() (map[string]string, error) {
	patentsFile := filepath.Join(p.dataPath, "patents.json")
	file, err := os.Open(patentsFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	idMap := make(map[string]string)

	// Use jsparser for streaming JSON
	br := bufio.NewReader(file)
	parser := jsparser.NewJSONParser(br, "patents")

	var previous int64
	for j := range parser.Stream() {
		elapsed := int64(time.Since(p.d.start).Seconds())
		if elapsed > previous+p.d.progInterval {
			kbytesPerSecond := int64(parser.TotalReadSize) / elapsed / 1024
			previous = elapsed
			p.d.progChan <- &progressInfo{dataset: p.source, currentKBPerSec: kbytesPerSecond}
		}

		id := getString(j, "id")
		patentNumber := getString(j, "patent_number")

		if id != "" && id != "0" && patentNumber != "" {
			// Test mode: Only include patents we processed
			if config.IsTestMode() {
				if p.testPatentIDs != nil && p.testPatentIDs[patentNumber] {
					idMap[id] = patentNumber
				}
			} else {
				idMap[id] = patentNumber
			}
		}
	}

	return idMap, nil
}

// Helper functions to extract values from jsparser.JSON
func getString(j *jsparser.JSON, key string) string {
	if j.ObjectVals[key] != nil {
		switch v := j.ObjectVals[key].(type) {
		case string:
			return v
		case float64:
			// Convert numbers to strings
			return strconv.FormatFloat(v, 'f', 0, 64)
		case int64:
			return strconv.FormatInt(v, 10)
		case int:
			return strconv.Itoa(v)
		}
	}
	return ""
}

func normalizeCompanyName(name string) string {
	// Normalize company names for consistent xrefs
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	// Convert to uppercase
	name = strings.ToUpper(name)

	// Remove country codes in parentheses: (US), (GB), (DE), etc.
	name = strings.ReplaceAll(name, " (US)", "")
	name = strings.ReplaceAll(name, " (GB)", "")
	name = strings.ReplaceAll(name, " (DE)", "")
	name = strings.ReplaceAll(name, " (FR)", "")
	name = strings.ReplaceAll(name, " (JP)", "")
	name = strings.ReplaceAll(name, " (CN)", "")
	name = strings.ReplaceAll(name, " (CH)", "")
	name = strings.ReplaceAll(name, " (NL)", "")
	name = strings.ReplaceAll(name, " (DK)", "")
	name = strings.ReplaceAll(name, " (SE)", "")
	name = strings.ReplaceAll(name, " (BM)", "")

	// Remove legal suffixes
	suffixes := []string{
		" INC.", " INC", " INCORPORATED",
		" LTD.", " LTD", " LIMITED",
		" LLC", " LLC.",
		" GMBH", " GMBH.",
		" AG", " AG.",
		" SA", " SA.",
		" NV", " NV.",
		" CO.", " CO", " COMPANY",
		" CORPORATION", " CORP.", " CORP",
		" AND COMPANY",
	}

	for _, suffix := range suffixes {
		name = strings.TrimSuffix(name, suffix)
	}

	// Handle "E. I." → "EI", "E.I." → "EI"
	name = strings.ReplaceAll(name, "E. I. ", "EI ")
	name = strings.ReplaceAll(name, "E.I. ", "EI ")

	// Clean up multiple spaces
	name = strings.Join(strings.Fields(name), " ")

	return strings.TrimSpace(name)
}
