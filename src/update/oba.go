package update

import (
	"biobtree/pbuf"
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type oba struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for oba processor
func (o *oba) check(err error, operation string) {
	checkWithContext(err, o.source, operation)
}

func (o *oba) update() {

	var br *bufio.Reader
	fr := config.Dataconf[o.source]["id"]
	path := config.Dataconf[o.source]["path"]
	frparentStr := o.source + "parent"
	frchildStr := o.source + "child"
	frparent := config.Dataconf[frparentStr]["id"]
	frchild := config.Dataconf[frchildStr]["id"]

	defer o.d.wg.Done()

	// Test mode support
	testLimit := config.GetTestLimit(o.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, o.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64
	var previous int64
	var start time.Time

	if config.Dataconf[o.source]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(path))
		check(err)
		br = bufio.NewReaderSize(file, fileBufSize)
		defer file.Close()
	} else {
		resp, err := http.Get(path)
		check(err)
		br = bufio.NewReaderSize(resp.Body, fileBufSize)
		defer resp.Body.Close()
	}

	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer for long lines

	var currentID string
	var attr pbuf.OntologyAttr
	var parents []string
	inTerm := false
	isObsolete := false

	start = time.Now()
	previous = 0

	for scanner.Scan() {
		line := scanner.Text()

		// Progress reporting
		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+o.d.progInterval {
			previous = elapsed
			o.d.progChan <- &progressInfo{dataset: o.source}
		}

		// Start of new term
		if strings.HasPrefix(line, "[Term]") {
			// Save previous term if it exists and is valid
			if inTerm && currentID != "" && !isObsolete {
				o.saveEntry(currentID, fr, &attr)
				o.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
				total++

				// Log ID in test mode
				if idLogFile != nil {
					logProcessedID(idLogFile, currentID)
				}

				// Check test limit
				if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
					goto done
				}
			}

			// Reset for new term
			inTerm = true
			isObsolete = false
			currentID = ""
			parents = []string{}
			attr = pbuf.OntologyAttr{
				Synonyms: []string{},
			}
			continue
		}

		// End of terms section (Typedef section starts)
		if strings.HasPrefix(line, "[Typedef]") {
			// Save last term before Typedef section
			if inTerm && currentID != "" && !isObsolete {
				o.saveEntry(currentID, fr, &attr)
				o.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
				total++

				if idLogFile != nil {
					logProcessedID(idLogFile, currentID)
				}
			}
			break // Stop processing after terms
		}

		// Skip if not in a term block
		if !inTerm {
			continue
		}

		// Parse fields - only process OBA terms
		if strings.HasPrefix(line, "id: OBA:") {
			currentID = strings.TrimPrefix(line, "id: ")
		} else if strings.HasPrefix(line, "id: ") && !strings.HasPrefix(line, "id: OBA:") {
			// Not an OBA term (imported term like PATO, CHEBI, etc.)
			inTerm = false
			continue
		} else if strings.HasPrefix(line, "name: ") {
			attr.Name = strings.TrimPrefix(line, "name: ")
		} else if strings.HasPrefix(line, "synonym: ") {
			// Parse synonym line: synonym: "text" EXACT [refs]
			synonym := extractSynonymText(line)
			if synonym != "" {
				attr.Synonyms = append(attr.Synonyms, synonym)
			}
		} else if strings.HasPrefix(line, "is_a: OBA:") {
			// Parse parent relationship within OBA: is_a: OBA:0000001 ! biological attribute
			parentID := extractOBAParentID(line)
			if parentID != "" {
				parents = append(parents, parentID)
			}
		} else if strings.HasPrefix(line, "relationship: ") {
			// Parse relationships to other ontologies and create xrefs
			// e.g., relationship: RO:0000052 GO:0005884 ! characteristic of actin filament
			o.parseRelationship(line, currentID, fr)
		} else if strings.HasPrefix(line, "xref: PMID:") {
			// Parse PMID cross-references
			pmid := extractPMID(line)
			if pmid != "" {
				// Create cross-reference to literature
				o.d.addXref(currentID, fr, pmid, "literature_mappings", false)
			}
		} else if strings.HasPrefix(line, "is_obsolete: true") {
			isObsolete = true
		}
	}

	// Save last term if file doesn't end with [Typedef]
	if inTerm && currentID != "" && !isObsolete {
		o.saveEntry(currentID, fr, &attr)
		o.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
		total++

		if idLogFile != nil {
			logProcessedID(idLogFile, currentID)
		}
	}

done:
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	o.d.progChan <- &progressInfo{dataset: o.source, done: true}
	atomic.AddUint64(&o.d.totalParsedEntry, total)
}

func (o *oba) saveEntry(id string, datasetID string, attr *pbuf.OntologyAttr) {
	attr.Type = "biological_attribute"
	b, _ := ffjson.Marshal(attr)
	o.d.addProp3(id, datasetID, b)

	// Deduplicate search terms to avoid duplicate text xrefs
	searchTerms := make(map[string]bool)

	// Add attribute name to search terms
	if attr.Name != "" {
		searchTerms[attr.Name] = true
	}

	// Add all synonyms to search terms
	for _, synonym := range attr.Synonyms {
		if synonym != "" {
			searchTerms[synonym] = true
		}
	}

	// Create text search xrefs for all unique terms
	for term := range searchTerms {
		o.d.addXref(term, textLinkID, id, o.source, true)
	}
}

// extractOBAParentID extracts the parent OBA ID from an is_a line
// Example: is_a: OBA:0000040 ! calcium ion concentration
func extractOBAParentID(line string) string {
	line = strings.TrimPrefix(line, "is_a: ")

	// Find the space, exclamation mark, or opening brace (whichever comes first)
	endIdx := len(line)

	spaceIdx := strings.Index(line, " ")
	braceIdx := strings.Index(line, "{")
	exclamIdx := strings.Index(line, "!")

	if spaceIdx != -1 && spaceIdx < endIdx {
		endIdx = spaceIdx
	}
	if braceIdx != -1 && braceIdx < endIdx {
		endIdx = braceIdx
	}
	if exclamIdx != -1 && exclamIdx < endIdx {
		endIdx = exclamIdx
	}

	parentID := strings.TrimSpace(line[:endIdx])

	// Validate it's an OBA ID
	if strings.HasPrefix(parentID, "OBA:") {
		return parentID
	}

	return ""
}

// extractPMID extracts PMID from xref line
// Example: xref: PMID:12345678 → returns "12345678" (numeric only)
func extractPMID(line string) string {
	line = strings.TrimPrefix(line, "xref: PMID:")

	endIdx := len(line)
	spaceIdx := strings.Index(line, " ")
	if spaceIdx != -1 {
		endIdx = spaceIdx
	}

	pmid := strings.TrimSpace(line[:endIdx])
	return pmid // Return numeric ID only, without PMID: prefix
}

// parseRelationship parses relationship lines and creates bidirectional cross-references
// Example: relationship: RO:0000052 GO:0005884 ! characteristic of actin filament
// Example: relationship: RO:0000052 UBERON:0001054 ! characteristic of brain
func (o *oba) parseRelationship(line string, obaID string, obaDatasetID string) {
	line = strings.TrimPrefix(line, "relationship: ")

	// Split by space to get parts
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return
	}

	// The target is the second part (after the relationship type)
	target := parts[1]

	var targetDataset string
	var targetID string

	switch {
	case strings.HasPrefix(target, "GO:"):
		targetDataset = "go"
		targetID = target
	case strings.HasPrefix(target, "UBERON:"):
		targetDataset = "uberon"
		targetID = target
	case strings.HasPrefix(target, "CL:"):
		targetDataset = "cl"
		targetID = target
	case strings.HasPrefix(target, "CHEBI:"):
		targetDataset = "chebi"
		targetID = target
	default:
		return
	}

	if targetDataset != "" && targetID != "" {
		// Create cross-reference: OBA → target dataset
		// addXref creates both forward and reverse automatically
		o.d.addXref(obaID, obaDatasetID, targetID, targetDataset, false)
	}
}

// saveParentChildRelations creates parent/child cross-references for hierarchical relationships
func (o *oba) saveParentChildRelations(childID string, obaDatasetID string,
	parentDatasetID string, childDatasetID string, frparentStr string, frchildStr string, parents []string) {

	for _, parentID := range parents {
		if parentID == "" || parentID == childID {
			continue
		}

		// Create parent relationships
		// childID -> parent link
		o.d.addXref2(childID, obaDatasetID, parentID, frparentStr)
		// parent term itself links back to parent dataset
		o.d.addXref2(parentID, parentDatasetID, parentID, o.source)

		// Create child relationships
		// parentID -> child link
		o.d.addXref2(parentID, obaDatasetID, childID, frchildStr)
		// child term itself links back to child dataset
		o.d.addXref2(childID, childDatasetID, childID, o.source)
	}
}
