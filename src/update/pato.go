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

type pato struct {
	source string
	d      *DataUpdate
}

// check provides context-aware error checking for pato processor
func (p *pato) check(err error, operation string) {
	checkWithContext(err, p.source, operation)
}

func (p *pato) update() {

	var br *bufio.Reader
	fr := config.Dataconf[p.source]["id"]
	path := config.Dataconf[p.source]["path"]
	frparentStr := p.source + "parent"
	frchildStr := p.source + "child"
	frparent := config.Dataconf[frparentStr]["id"]
	frchild := config.Dataconf[frchildStr]["id"]

	defer p.d.wg.Done()

	// Test mode support
	testLimit := config.GetTestLimit(p.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, p.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64
	var previous int64
	var start time.Time

	if config.Dataconf[p.source]["useLocalFile"] == "yes" {
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
		if elapsed > previous+p.d.progInterval {
			previous = elapsed
			p.d.progChan <- &progressInfo{dataset: p.source}
		}

		// Start of new term
		if strings.HasPrefix(line, "[Term]") {
			// Save previous term if it exists and is valid
			if inTerm && currentID != "" && !isObsolete {
				p.saveEntry(currentID, fr, &attr)
				p.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
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
				p.saveEntry(currentID, fr, &attr)
				p.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
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

		// Parse fields - only process PATO terms
		if strings.HasPrefix(line, "id: PATO:") {
			currentID = strings.TrimPrefix(line, "id: ")
		} else if strings.HasPrefix(line, "id: ") && !strings.HasPrefix(line, "id: PATO:") {
			// Not a PATO term (imported term)
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
		} else if strings.HasPrefix(line, "is_a: PATO:") {
			// Parse parent relationship within PATO: is_a: PATO:0000001 ! quality
			parentID := extractPATOParentID(line)
			if parentID != "" {
				parents = append(parents, parentID)
			}
		} else if strings.HasPrefix(line, "is_obsolete: true") {
			isObsolete = true
		}
	}

	// Save last term if file doesn't end with [Typedef]
	if inTerm && currentID != "" && !isObsolete {
		p.saveEntry(currentID, fr, &attr)
		p.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
		total++

		if idLogFile != nil {
			logProcessedID(idLogFile, currentID)
		}
	}

done:
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	p.d.progChan <- &progressInfo{dataset: p.source, done: true}
	atomic.AddUint64(&p.d.totalParsedEntry, total)
}

func (p *pato) saveEntry(id string, datasetID string, attr *pbuf.OntologyAttr) {
	attr.Type = "quality"
	b, _ := ffjson.Marshal(attr)
	p.d.addProp3(id, datasetID, b)

	// Deduplicate search terms to avoid duplicate text xrefs
	searchTerms := make(map[string]bool)

	// Add quality name to search terms
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
		p.d.addXref(term, textLinkID, id, p.source, true)
	}
}

// extractPATOParentID extracts the parent PATO ID from an is_a line
// Example: is_a: PATO:0000001 ! quality
func extractPATOParentID(line string) string {
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

	// Validate it's a PATO ID
	if strings.HasPrefix(parentID, "PATO:") {
		return parentID
	}

	return ""
}

// saveParentChildRelations creates parent/child cross-references for hierarchical relationships
func (p *pato) saveParentChildRelations(childID string, patoDatasetID string,
	parentDatasetID string, childDatasetID string, frparentStr string, frchildStr string, parents []string) {

	for _, parentID := range parents {
		if parentID == "" || parentID == childID {
			continue
		}

		// Create parent relationships
		// childID -> parent link
		p.d.addXref2(childID, patoDatasetID, parentID, frparentStr)
		// parent term itself links back to parent dataset
		p.d.addXref2(parentID, parentDatasetID, parentID, p.source)

		// Create child relationships
		// parentID -> child link
		p.d.addXref2(parentID, patoDatasetID, childID, frchildStr)
		// child term itself links back to child dataset
		p.d.addXref2(childID, childDatasetID, childID, p.source)
	}
}
