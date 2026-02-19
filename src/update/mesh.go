package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type mesh struct {
	source           string
	d                *DataUpdate
	treeToDescriptor map[string]string // Maps tree number -> descriptor ID
}

// check provides context-aware error checking for mesh processor
func (m *mesh) check(err error, operation string) {
	checkWithContext(err, m.source, operation)
}

// isMeshID checks if a string is a valid MeSH ID (D/C/Q followed by digits)
// Returns false for term names like "Anti-Bacterial Agents"
func isMeshID(s string) bool {
	if len(s) < 2 {
		return false
	}
	// MeSH IDs start with D (Descriptor), C (Supplementary), or Q (Qualifier)
	firstChar := s[0]
	if firstChar != 'D' && firstChar != 'C' && firstChar != 'Q' {
		return false
	}
	// Rest should be digits
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func (m *mesh) update() {
	defer m.d.wg.Done()

	// Test mode support
	testLimit := config.GetTestLimit(m.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, m.source+"_ids.txt")
		defer func() {
			if idLogFile != nil {
				idLogFile.Close()
			}
		}()
	}

	// Get dataset IDs
	fr := config.Dataconf[m.source]["id"]
	frparentStr := m.source + "parent"
	frchildStr := m.source + "child"
	frparent := config.Dataconf[frparentStr]["id"]
	frchild := config.Dataconf[frchildStr]["id"]

	var start time.Time
	var previous int64
	var totalDescriptors, totalSupplementary uint64

	// Parse descriptors
	stopEarly := m.parseDescriptors(fr, frparent, frchild, frparentStr, frchildStr, &start, &previous, &totalDescriptors, testLimit, idLogFile)

	// Build hierarchy cross-references after all descriptors are parsed
	// This creates meshparent and meshchild relationships based on tree numbers
	m.buildHierarchyXrefs(fr, frparent, frchild, frparentStr, frchildStr)

	// Parse supplementary concepts if path configured and not stopped early
	if !stopEarly && config.Dataconf[m.source]["pathSupplementary"] != "" {
		m.parseSupplementary(fr, &start, &previous, &totalSupplementary, testLimit, idLogFile, int(totalDescriptors))
	}

	// Completion
	total := totalDescriptors + totalSupplementary
	m.d.progChan <- &progressInfo{dataset: m.source, done: true}
	atomic.AddUint64(&m.d.totalParsedEntry, total)
}

func (m *mesh) parseDescriptors(fr, frparent, frchild string, frparentStr, frchildStr string, start *time.Time, previous *int64, total *uint64, testLimit int, idLogFile *os.File) bool {
	path := config.Dataconf[m.source]["path"]

	var reader io.Reader
	if config.Dataconf[m.source]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(path))
		m.check(err, "opening descriptors file")
		defer file.Close()
		reader = file
	} else {
		resp, err := http.Get(path)
		m.check(err, "downloading descriptors file")
		if resp.StatusCode != 200 {
			m.check(fmt.Errorf("HTTP %d: %s - file may not exist or URL may have changed", resp.StatusCode, resp.Status), "downloading descriptors file from "+path)
		}
		defer resp.Body.Close()
		reader = resp.Body
	}

	*start = time.Now()
	*previous = 0

	scanner := bufio.NewScanner(reader)
	var currentRecord map[string][]string
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Progress reporting
		if lineNum%10000 == 0 {
			elapsed := int64(time.Since(*start).Seconds())
			if elapsed > *previous+m.d.progInterval {
				*previous = elapsed
				m.d.progChan <- &progressInfo{dataset: m.source}
			}
		}

		// New record starts
		if line == "*NEWRECORD" {
			// Process previous record if exists
			if currentRecord != nil {
				attr := m.parseDescriptorRecord(currentRecord)
				if attr != nil && attr.DescriptorUi != "" {
					m.saveEntry(attr.DescriptorUi, fr, attr)
					m.saveHierarchyRelations(attr.DescriptorUi, fr, frparent, frchild, frparentStr, frchildStr, attr.TreeNumbers)
					atomic.AddUint64(total, 1)

					// Log ID in test mode
					if idLogFile != nil {
						logProcessedID(idLogFile, attr.DescriptorUi)
					}

					// Check test limit
					if config.IsTestMode() && shouldStopProcessing(testLimit, int(*total)) {
						return true // Stop early
					}
				}
			}
			currentRecord = make(map[string][]string)
			continue
		}

		// Parse field = value
		if strings.Contains(line, " = ") {
			parts := strings.SplitN(line, " = ", 2)
			if len(parts) == 2 {
				field := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				currentRecord[field] = append(currentRecord[field], value)
			}
		}
	}

	// Process last record
	if currentRecord != nil {
		attr := m.parseDescriptorRecord(currentRecord)
		if attr != nil && attr.DescriptorUi != "" {
			m.saveEntry(attr.DescriptorUi, fr, attr)
			m.saveHierarchyRelations(attr.DescriptorUi, fr, frparent, frchild, frparentStr, frchildStr, attr.TreeNumbers)
			atomic.AddUint64(total, 1)

			// Log ID in test mode
			if idLogFile != nil {
				logProcessedID(idLogFile, attr.DescriptorUi)
			}

			// Check test limit
			if config.IsTestMode() && shouldStopProcessing(testLimit, int(*total)) {
				return true // Stop early
			}
		}
	}

	m.check(scanner.Err(), "scanning descriptors file")
	return false // Did not stop early
}

func (m *mesh) parseSupplementary(fr string, start *time.Time, previous *int64, total *uint64, testLimit int, idLogFile *os.File, descriptorCount int) {
	path := config.Dataconf[m.source]["pathSupplementary"]

	var reader io.Reader
	if config.Dataconf[m.source]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(path))
		m.check(err, "opening supplementary file")
		defer file.Close()
		reader = file
	} else {
		resp, err := http.Get(path)
		m.check(err, "downloading supplementary file")
		if resp.StatusCode != 200 {
			m.check(fmt.Errorf("HTTP %d: %s - file may not exist or URL may have changed", resp.StatusCode, resp.Status), "downloading supplementary file from "+path)
		}
		defer resp.Body.Close()
		reader = resp.Body
	}

	scanner := bufio.NewScanner(reader)
	var currentRecord map[string][]string
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Progress reporting
		if lineNum%10000 == 0 {
			elapsed := int64(time.Since(*start).Seconds())
			if elapsed > *previous+m.d.progInterval {
				*previous = elapsed
				m.d.progChan <- &progressInfo{dataset: m.source}
			}
		}

		// New record starts
		if line == "*NEWRECORD" {
			// Process previous record if exists
			if currentRecord != nil {
				attr := m.parseSupplementalRecord(currentRecord)
				if attr != nil && attr.DescriptorUi != "" {
					m.saveEntry(attr.DescriptorUi, fr, attr)
					atomic.AddUint64(total, 1)

					// Log ID in test mode
					if idLogFile != nil {
						logProcessedID(idLogFile, attr.DescriptorUi)
					}

					// Check test limit (total includes descriptors)
					if config.IsTestMode() && shouldStopProcessing(testLimit, descriptorCount+int(*total)) {
						return // Stop early
					}
				}
			}
			currentRecord = make(map[string][]string)
			continue
		}

		// Parse field = value
		if strings.Contains(line, " = ") {
			parts := strings.SplitN(line, " = ", 2)
			if len(parts) == 2 {
				field := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				currentRecord[field] = append(currentRecord[field], value)
			}
		}
	}

	// Process last record
	if currentRecord != nil {
		attr := m.parseSupplementalRecord(currentRecord)
		if attr != nil && attr.DescriptorUi != "" {
			m.saveEntry(attr.DescriptorUi, fr, attr)
			atomic.AddUint64(total, 1)

			// Log ID in test mode
			if idLogFile != nil {
				logProcessedID(idLogFile, attr.DescriptorUi)
			}
		}
	}

	m.check(scanner.Err(), "scanning supplementary file")
}

func (m *mesh) parseDescriptorRecord(record map[string][]string) *pbuf.MeshAttr {
	attr := &pbuf.MeshAttr{}

	// UI (Unique Identifier)
	if ui, ok := record["UI"]; ok && len(ui) > 0 {
		attr.DescriptorUi = ui[0]
	} else {
		return nil
	}

	// MH (Main Heading / Descriptor Name)
	if mh, ok := record["MH"]; ok && len(mh) > 0 {
		// Replace tabs with spaces to avoid breaking index format
		attr.DescriptorName = strings.ReplaceAll(mh[0], "\t", " ")
	}

	// DC (Descriptor Class) - 1, 2, 3, 4
	if dc, ok := record["DC"]; ok && len(dc) > 0 {
		attr.DescriptorClass = dc[0]
	}

	// ENTRY and PRINT ENTRY (Entry Terms / Synonyms)
	// Format: "term|semtype|semtype|flags..."
	for _, entry := range record["ENTRY"] {
		if parts := strings.Split(entry, "|"); len(parts) > 0 {
			// Replace tabs with spaces to avoid breaking index format
			term := strings.ReplaceAll(parts[0], "\t", " ")
			attr.EntryTerms = append(attr.EntryTerms, term)
		}
	}
	for _, entry := range record["PRINT ENTRY"] {
		if parts := strings.Split(entry, "|"); len(parts) > 0 {
			// Replace tabs with spaces to avoid breaking index format
			term := strings.ReplaceAll(parts[0], "\t", " ")
			attr.EntryTerms = append(attr.EntryTerms, term)
		}
	}

	// MN (Tree Numbers)
	if mn, ok := record["MN"]; ok {
		attr.TreeNumbers = mn
	}

	// MS (Scope Note)
	if ms, ok := record["MS"]; ok && len(ms) > 0 {
		attr.ScopeNote = ms[0]
	}

	// AN (Annotation)
	if an, ok := record["AN"]; ok && len(an) > 0 {
		attr.Annotation = an[0]
	}

	// HN (History Note)
	if hn, ok := record["HN"]; ok && len(hn) > 0 {
		attr.HistoryNote = hn[0]
	}

	// AQ (Allowable Qualifiers) - format: "AA AD AE AG..."
	if aq, ok := record["AQ"]; ok && len(aq) > 0 {
		qualifiers := strings.Fields(aq[0])
		attr.AllowableQualifiers = qualifiers
	}

	// PA (Pharmacological Actions)
	if pa, ok := record["PA"]; ok {
		attr.PharmacologicalActions = pa
	}

	// Date fields
	if dx, ok := record["DX"]; ok && len(dx) > 0 {
		attr.DateEstablished = dx[0]
	}

	attr.IsSupplementary = false

	return attr
}

func (m *mesh) parseSupplementalRecord(record map[string][]string) *pbuf.MeshAttr {
	attr := &pbuf.MeshAttr{}

	// UI (Unique Identifier)
	if ui, ok := record["UI"]; ok && len(ui) > 0 {
		attr.DescriptorUi = ui[0]
	} else {
		return nil
	}

	// NM (Name)
	if nm, ok := record["NM"]; ok && len(nm) > 0 {
		// Replace tabs with spaces to avoid breaking index format
		attr.DescriptorName = strings.ReplaceAll(nm[0], "\t", " ")
	}

	// SY (Synonyms)
	// Format: "term|semtype|semtype|flags..."
	for _, syn := range record["SY"] {
		if parts := strings.Split(syn, "|"); len(parts) > 0 {
			// Replace tabs with spaces to avoid breaking index format
			term := strings.ReplaceAll(parts[0], "\t", " ")
			attr.EntryTerms = append(attr.EntryTerms, term)
		}
	}

	// RN (Registry Number - first one is primary)
	if rn, ok := record["RN"]; ok && len(rn) > 0 {
		attr.RegistryNumber = rn[0]
	}

	// HM (Heading Mapped To)
	if hm, ok := record["HM"]; ok && len(hm) > 0 {
		// Format: "*D000894-Anti-Inflammatory Agents" - extract descriptor UI
		hmValue := hm[0]
		if strings.Contains(hmValue, "-") {
			parts := strings.SplitN(hmValue, "-", 2)
			attr.HeadingMappedTo = strings.TrimPrefix(parts[0], "*")
		}
	}

	// NO (Note)
	if no, ok := record["NO"]; ok && len(no) > 0 {
		attr.ScopeNote = no[0]
	}

	// PA (Pharmacological Actions)
	if pa, ok := record["PA"]; ok {
		for _, action := range pa {
			// Format: "D000894-Anti-Inflammatory Agents, Non-Steroidal"
			if strings.Contains(action, "-") {
				parts := strings.SplitN(action, "-", 2)
				attr.PharmacologicalActions = append(attr.PharmacologicalActions, parts[0])
			}
		}
	}

	// Date fields
	if dx, ok := record["DX"]; ok && len(dx) > 0 {
		attr.DateEstablished = dx[0]
	}

	attr.IsSupplementary = true

	return attr
}

func (m *mesh) saveEntry(id string, fr string, attr *pbuf.MeshAttr) {
	// Serialize attributes to JSON
	b, err := ffjson.Marshal(attr)
	m.check(err, "marshaling MeSH attributes")

	// Save main entry
	m.d.addProp3(id, fr, b)

	// Index descriptor name for text search
	if attr.DescriptorName != "" {
		m.d.addXref(attr.DescriptorName, textLinkID, id, m.source, true)
	}

	// Index all entry terms (synonyms) for text search
	for _, term := range attr.EntryTerms {
		if term != "" {
			m.d.addXref(term, textLinkID, id, m.source, true)
		}
	}

	// Pull synonyms from linked MONDO entries via lookup service
	// This makes MeSH entries searchable via MONDO synonyms
	// e.g., searching "hypercalcemia" finds C562390 via MONDO:0043455's synonym
	m.pullMondoSynonyms(id)

	// Create cross-references for pharmacological actions
	// Link from this descriptor to its pharmacological action descriptors
	// Only create xrefs for valid MeSH IDs (D/C/Q followed by digits)
	// Descriptor records may have term names instead of IDs - skip those
	for _, action := range attr.PharmacologicalActions {
		if action != "" && isMeshID(action) {
			m.d.addXref(id, fr, action, m.source, false)
		}
	}
}

// pullMondoSynonyms looks up MONDO entries that link to this MeSH ID
// and adds their synonyms as text search terms for the MeSH entry
func (m *mesh) pullMondoSynonyms(meshID string) {
	// Check if lookup service is available and MONDO is configured
	if m.d.lookupService == nil {
		return
	}
	if _, exists := config.Dataconf["mondo"]; !exists {
		return
	}

	// Use MapFilter to find MONDO entries that link to this MeSH ID
	// Query: meshID >>mesh>>mondo - finds MONDO entries via MeSH xrefs
	result, err := m.d.lookupService.MapFilter([]string{meshID}, ">>mesh>>mondo", "")
	if err != nil {
		log.Printf("MeSH pullMondoSynonyms: MapFilter error for %s: %v", meshID, err)
		return
	}
	if result == nil || len(result.Results) == 0 {
		return
	}

	mondoDatasetID := config.DataconfIDStringToInt["mondo"]

	// Process each mapping result
	for _, mapResult := range result.Results {
		// Get MONDO targets from this mapping
		for _, target := range mapResult.Targets {
			if target.Dataset != mondoDatasetID || target.Identifier == "" {
				continue
			}

			mondoID := target.Identifier
			log.Printf("MeSH pullMondoSynonyms: found MONDO %s for MeSH %s", mondoID, meshID)

			// Extract ontology attributes directly from target (contains synonyms)
			ontologyAttr := target.GetOntology()
			if ontologyAttr == nil {
				// Try getting full entry if attributes not in target
				mondoEntry, err := m.d.lookupFullEntry(mondoID, mondoDatasetID)
				if err != nil || mondoEntry == nil {
					continue
				}
				ontologyAttr = mondoEntry.GetOntology()
				if ontologyAttr == nil {
					continue
				}
			}

			log.Printf("MeSH pullMondoSynonyms: MONDO %s has name=%s, %d synonyms", mondoID, ontologyAttr.Name, len(ontologyAttr.Synonyms))

			// Collect all phrases (name + synonyms)
			allPhrases := []string{}
			if ontologyAttr.Name != "" {
				allPhrases = append(allPhrases, ontologyAttr.Name)
			}
			allPhrases = append(allPhrases, ontologyAttr.Synonyms...)

			// Add full phrases as text search terms
			for _, phrase := range allPhrases {
				if phrase != "" {
					m.d.addXref(phrase, textLinkID, meshID, m.source, true)
				}
			}

			// Add individual significant words for partial matching
			// This allows searching "hypercalcemia" to find entries with "hypercalcemia of malignancy"
			for _, phrase := range allPhrases {
				for _, word := range strings.Fields(phrase) {
					// Clean word of punctuation
					word = strings.Trim(word, ",.;:'\"()-")
					// Only add words with 4+ characters that aren't common stop words
					if len(word) >= 4 && !isMeshStopWord(word) {
						m.d.addXref(word, textLinkID, meshID, m.source, true)
					}
				}
			}
		}
	}
}

// isMeshStopWord returns true for common medical terms that should not be indexed alone
func isMeshStopWord(word string) bool {
	word = strings.ToLower(word)
	stopWords := map[string]bool{
		// Disease type words
		"disease": true, "disorder": true, "syndrome": true, "condition": true,
		"type": true, "form": true, "variant": true,
		// Common medical terms
		"with": true, "without": true, "from": true, "that": true,
		"associated": true, "related": true, "induced": true,
		"chronic": true, "acute": true, "congenital": true,
		"familial": true, "hereditary": true, "inherited": true,
	}
	return stopWords[word]
}

func (m *mesh) saveHierarchyRelations(id string, fr, frparent, frchild string, frparentStr, frchildStr string, treeNumbers []string) {
	// Build tree number -> descriptor ID mapping
	// This will be used later to create parent-child xrefs
	for _, treeNum := range treeNumbers {
		if treeNum != "" {
			m.treeToDescriptor[treeNum] = id
		}
	}
}

// buildHierarchyXrefs creates parent-child cross-references based on tree number hierarchy
// Tree numbers follow a hierarchical pattern: A01.236.249 is a child of A01.236
// This function should be called after all descriptors are parsed
// Uses same pattern as ontology.go for consistency
func (m *mesh) buildHierarchyXrefs(fr, frparent, frchild, frparentStr, frchildStr string) {
	// Iterate through all tree numbers and create hierarchy xrefs
	for treeNum, childID := range m.treeToDescriptor {
		// Find parent tree number by removing the last segment
		// e.g., "D02.355.291.933.125" -> "D02.355.291.933"
		lastDot := strings.LastIndex(treeNum, ".")
		if lastDot > 0 {
			parentTreeNum := treeNum[:lastDot]

			// Look up the parent descriptor ID
			if parentID, exists := m.treeToDescriptor[parentTreeNum]; exists {
				// Skip if child and parent are the same (shouldn't happen but safety check)
				if childID == parentID {
					continue
				}

				// Create xrefs following the ontology.go pattern:
				// 1. Child -> Parent in mesh dataset, target in meshparent
				m.d.addXref2Bucketed(childID, fr, parentID, frparentStr, fr)
				// 2. Parent entry in meshparent dataset pointing back to mesh
				m.d.addXref2Bucketed(parentID, frparent, parentID, m.source, fr)

				// 3. Parent -> Child in mesh dataset, target in meshchild
				m.d.addXref2Bucketed(parentID, fr, childID, frchildStr, fr)
				// 4. Child entry in meshchild dataset pointing back to mesh
				m.d.addXref2Bucketed(childID, frchild, childID, m.source, fr)
			}
		}
	}
}
