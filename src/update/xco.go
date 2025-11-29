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

type xco struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for xco processor
func (x *xco) check(err error, operation string) {
	checkWithContext(err, x.source, operation)
}

func (x *xco) update() {

	var br *bufio.Reader
	fr := config.Dataconf[x.source]["id"]
	path := config.Dataconf[x.source]["path"]
	frparentStr := x.source + "parent"
	frchildStr := x.source + "child"
	frparent := config.Dataconf[frparentStr]["id"]
	frchild := config.Dataconf[frchildStr]["id"]

	defer x.d.wg.Done()

	// Test mode support
	testLimit := config.GetTestLimit(x.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, x.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64
	var previous int64
	var start time.Time

	if config.Dataconf[x.source]["useLocalFile"] == "yes" {
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
		if elapsed > previous+x.d.progInterval {
			previous = elapsed
			x.d.progChan <- &progressInfo{dataset: x.source}
		}

		// Start of new term
		if strings.HasPrefix(line, "[Term]") {
			// Save previous term if it exists and is valid
			if inTerm && currentID != "" && !isObsolete {
				x.saveEntry(currentID, fr, &attr)
				x.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
				total++

				// Log ID in test mode
				if idLogFile != nil {
					logProcessedID(idLogFile, currentID)
				}

				// Check test limit
				if shouldStopProcessing(testLimit, int(total)) {
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
				x.saveEntry(currentID, fr, &attr)
				x.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
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

		// Parse fields - only process XCO terms
		if strings.HasPrefix(line, "id: XCO:") {
			currentID = strings.TrimPrefix(line, "id: ")
		} else if strings.HasPrefix(line, "id: ") && !strings.HasPrefix(line, "id: XCO:") {
			// Not an XCO term (imported term)
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
		} else if strings.HasPrefix(line, "is_a: XCO:") {
			// Parse parent relationship within XCO: is_a: XCO:0000000 ! experimental condition
			parentID := extractXCOParentID(line)
			if parentID != "" {
				parents = append(parents, parentID)
			}
		} else if strings.HasPrefix(line, "is_obsolete: true") {
			isObsolete = true
		}
	}

	// Save last term if file doesn't end with [Typedef]
	if inTerm && currentID != "" && !isObsolete {
		x.saveEntry(currentID, fr, &attr)
		x.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
		total++

		if idLogFile != nil {
			logProcessedID(idLogFile, currentID)
		}
	}

done:
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	x.d.progChan <- &progressInfo{dataset: x.source, done: true}
	atomic.AddUint64(&x.d.totalParsedEntry, total)
	x.d.addEntryStat(x.source, total)
}

func (x *xco) saveEntry(id string, datasetID string, attr *pbuf.OntologyAttr) {
	attr.Type = "experimental_condition"
	b, _ := ffjson.Marshal(attr)
	x.d.addProp3(id, datasetID, b)

	// Deduplicate search terms to avoid duplicate text xrefs
	searchTerms := make(map[string]bool)

	// Add term name to search terms
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
		x.d.addXref(term, textLinkID, id, x.source, true)
	}
}

// extractXCOParentID extracts the parent XCO ID from an is_a line
// Example: is_a: XCO:0000000 ! experimental condition
func extractXCOParentID(line string) string {
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

	// Validate it's an XCO ID
	if strings.HasPrefix(parentID, "XCO:") {
		return parentID
	}

	return ""
}

// saveParentChildRelations creates parent/child cross-references for hierarchical relationships
func (x *xco) saveParentChildRelations(childID string, xcoDatasetID string,
	parentDatasetID string, childDatasetID string, frparentStr string, frchildStr string, parents []string) {

	for _, parentID := range parents {
		if parentID == "" || parentID == childID {
			continue
		}

		// Create parent relationships
		// childID -> parent link
		x.d.addXref2(childID, xcoDatasetID, parentID, frparentStr)
		// parent term itself links back to parent dataset
		x.d.addXref2(parentID, parentDatasetID, parentID, x.source)

		// Create child relationships
		// parentID -> child link
		x.d.addXref2(parentID, xcoDatasetID, childID, frchildStr)
		// child term itself links back to child dataset
		x.d.addXref2(childID, childDatasetID, childID, x.source)
	}
}
