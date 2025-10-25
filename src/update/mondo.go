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

type mondo struct {
	source string
	d      *DataUpdate
}

func (m *mondo) update() {

	var br *bufio.Reader
	fr := config.Dataconf[m.source]["id"]
	path := config.Dataconf[m.source]["path"]

	defer m.d.wg.Done()

	var total uint64
	var previous int64
	var start time.Time

	if config.Dataconf[m.source]["useLocalFile"] == "yes" {
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
	inTerm := false
	isObsolete := false

	start = time.Now()
	previous = 0

	for scanner.Scan() {
		line := scanner.Text()

		// Progress reporting (simplified - OBO parsing is fast)
		elapsed := int64(time.Since(start).Seconds())
		if elapsed > previous+m.d.progInterval {
			previous = elapsed
			m.d.progChan <- &progressInfo{dataset: m.source}
		}

		// Start of new term
		if strings.HasPrefix(line, "[Term]") {
			// Save previous term if it exists and is valid
			if inTerm && currentID != "" && !isObsolete {
				m.saveEntry(currentID, fr, &attr)
				total++
			}

			// Reset for new term
			inTerm = true
			isObsolete = false
			currentID = ""
			attr = pbuf.OntologyAttr{
				Synonyms: []string{},
			}
			continue
		}

		// Skip if not in a term block
		if !inTerm {
			continue
		}

		// Parse fields
		if strings.HasPrefix(line, "id: MONDO:") {
			currentID = strings.TrimPrefix(line, "id: ")
		} else if strings.HasPrefix(line, "name: ") {
			attr.Name = strings.TrimPrefix(line, "name: ")
		} else if strings.HasPrefix(line, "synonym: ") {
			// Parse synonym line: synonym: "text" EXACT [refs]
			synonym := extractSynonymText(line)
			if synonym != "" {
				attr.Synonyms = append(attr.Synonyms, synonym)
			}
		} else if strings.HasPrefix(line, "xref: ") {
			// Parse xref line: xref: DATABASE:ID {props}
			m.parseXref(line, currentID, fr)
		} else if strings.HasPrefix(line, "is_obsolete: true") {
			isObsolete = true
		}
	}

	// Save last term
	if inTerm && currentID != "" && !isObsolete {
		m.saveEntry(currentID, fr, &attr)
		total++
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	m.d.progChan <- &progressInfo{dataset: m.source, done: true}
	atomic.AddUint64(&m.d.totalParsedEntry, total)
	m.d.addEntryStat(m.source, total)
}

func (m *mondo) saveEntry(id string, datasetID string, attr *pbuf.OntologyAttr) {
	attr.Type = "disease"
	b, _ := ffjson.Marshal(attr)
	m.d.addProp3(id, datasetID, b)

	// Create text search xrefs for disease name
	if attr.Name != "" {
		m.d.addXref(attr.Name, textLinkID, id, m.source, true)
	}

	// Create text search xrefs for all synonyms
	for _, synonym := range attr.Synonyms {
		if synonym != "" {
			m.d.addXref(synonym, textLinkID, id, m.source, true)
		}
	}
}

// extractSynonymText extracts the synonym text from a line like:
// synonym: "adrenal cortical hypofunction" EXACT [DOID:10493, NCIT:C26691]
func extractSynonymText(line string) string {
	line = strings.TrimPrefix(line, "synonym: ")
	if len(line) < 2 || line[0] != '"' {
		return ""
	}

	// Find closing quote
	endQuote := strings.Index(line[1:], "\"")
	if endQuote == -1 {
		return ""
	}

	return line[1 : endQuote+1]
}

// parseXref parses xref lines and creates cross-references
// Example: xref: DOID:10493 {source="MONDO:equivalentTo"}
func (m *mondo) parseXref(line string, mondoID string, mondoDatasetID string) {
	line = strings.TrimPrefix(line, "xref: ")

	// Extract the xref ID (before space or brace)
	spaceIdx := strings.Index(line, " ")
	braceIdx := strings.Index(line, "{")

	endIdx := len(line)
	if spaceIdx != -1 && (braceIdx == -1 || spaceIdx < braceIdx) {
		endIdx = spaceIdx
	} else if braceIdx != -1 {
		endIdx = braceIdx
	}

	xrefID := strings.TrimSpace(line[:endIdx])
	if xrefID == "" {
		return
	}

	// Map known databases to biobtree dataset IDs
	// We can expand this list as needed
	var targetDatasetID string
	var targetID string

	if strings.HasPrefix(xrefID, "DOID:") {
		// Disease Ontology - not currently in biobtree
		return
	} else if strings.HasPrefix(xrefID, "OMIM:") || strings.HasPrefix(xrefID, "OMIMPS:") {
		// OMIM - not currently in biobtree
		return
	} else if strings.HasPrefix(xrefID, "Orphanet:") {
		// Orphanet - not currently in biobtree
		return
	} else if strings.HasPrefix(xrefID, "MESH:") {
		// MeSH - not currently in biobtree
		return
	} else if strings.HasPrefix(xrefID, "NCIT:") {
		// NCI Thesaurus - not currently in biobtree
		return
	} else if strings.HasPrefix(xrefID, "EFO:") {
		// EFO is dataset 22 in biobtree
		targetDatasetID = "22"
		targetID = xrefID
	} else if strings.HasPrefix(xrefID, "UMLS:") {
		// UMLS - not currently in biobtree
		return
	} else if strings.HasPrefix(xrefID, "ICD") {
		// ICD codes - not currently in biobtree
		return
	} else {
		// Unknown xref type, skip for now
		return
	}

	// Create bidirectional cross-reference if we found a mapping
	if targetDatasetID != "" && targetID != "" {
		m.d.addXref(mondoID, mondoDatasetID, targetID, targetDatasetID, false)
		m.d.addXref(targetID, targetDatasetID, mondoID, mondoDatasetID, false)
	}
}
