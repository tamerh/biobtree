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

type hpo struct {
	source string
	d      *DataUpdate
}

func (h *hpo) update() {

	var br *bufio.Reader
	fr := config.Dataconf[h.source]["id"]
	path := config.Dataconf[h.source]["path"]
	frparentStr := h.source + "parent"
	frchildStr := h.source + "child"
	frparent := config.Dataconf[frparentStr]["id"]
	frchild := config.Dataconf[frchildStr]["id"]

	defer h.d.wg.Done()

	// Test mode support
	testLimit := config.GetTestLimit(h.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, h.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var total uint64
	var previous int64
	var start time.Time

	// Phase 1: Parse hp.obo ontology file
	if config.Dataconf[h.source]["useLocalFile"] == "yes" {
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
		if elapsed > previous+h.d.progInterval {
			previous = elapsed
			h.d.progChan <- &progressInfo{dataset: h.source}
		}

		// Start of new term
		if strings.HasPrefix(line, "[Term]") {
			// Save previous term if it exists and is valid
			if inTerm && currentID != "" && !isObsolete {
				h.saveEntry(currentID, fr, &attr)
				h.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
				total++

				// Log ID in test mode
				if idLogFile != nil {
					logProcessedID(idLogFile, currentID)
				}

				// Check test limit
				if shouldStopProcessing(testLimit, int(total)) {
					goto phase2
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

		// Skip if not in a term block
		if !inTerm {
			continue
		}

		// Parse fields
		if strings.HasPrefix(line, "id: HP:") {
			currentID = strings.TrimPrefix(line, "id: ")
		} else if strings.HasPrefix(line, "name: ") {
			attr.Name = strings.TrimPrefix(line, "name: ")
		} else if strings.HasPrefix(line, "synonym: ") {
			// Parse synonym line: synonym: "text" EXACT [refs]
			synonym := extractSynonymText(line)
			if synonym != "" {
				attr.Synonyms = append(attr.Synonyms, synonym)
			}
		} else if strings.HasPrefix(line, "is_a: HP:") {
			// Parse parent relationship: is_a: HP:0000001 ! All
			parentID := extractHPOParentID(line)
			if parentID != "" {
				parents = append(parents, parentID)
			}
		} else if strings.HasPrefix(line, "is_obsolete: true") {
			isObsolete = true
		}
	}

	// Save last term
	if inTerm && currentID != "" && !isObsolete {
		h.saveEntry(currentID, fr, &attr)
		h.saveParentChildRelations(currentID, fr, frparent, frchild, frparentStr, frchildStr, parents)
		total++

		// Log ID in test mode
		if idLogFile != nil {
			logProcessedID(idLogFile, currentID)
		}
	}

phase2:
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	// Phase 2: Parse genes_to_phenotype.txt for gene-phenotype associations
	genePath := config.Dataconf[h.source]["genes_to_phenotype_path"]
	if genePath != "" {
		h.parseGeneToPhenotype(genePath)
	}
	h.d.progChan <- &progressInfo{dataset: h.source, done: true}
	atomic.AddUint64(&h.d.totalParsedEntry, total)
	h.d.addEntryStat(h.source, total)
}

func (h *hpo) saveEntry(id string, datasetID string, attr *pbuf.OntologyAttr) {
	attr.Type = "phenotype"
	b, _ := ffjson.Marshal(attr)
	h.d.addProp3(id, datasetID, b)

	// Deduplicate search terms to avoid duplicate text xrefs
	searchTerms := make(map[string]bool)

	// Add phenotype name to search terms
	if attr.Name != "" {
		searchTerms[attr.Name] = true
	}

	// Add all synonyms to search terms (will automatically deduplicate)
	for _, synonym := range attr.Synonyms {
		if synonym != "" {
			searchTerms[synonym] = true
		}
	}

	// Create text search xrefs for all unique terms
	for term := range searchTerms {
		h.d.addXref(term, textLinkID, id, h.source, true)
	}
}


// extractHPOParentID extracts the parent HP ID from an is_a line
// Example: is_a: HP:0000001 ! All
func extractHPOParentID(line string) string {
	line = strings.TrimPrefix(line, "is_a: ")

	// Find the space or exclamation mark (whichever comes first)
	endIdx := len(line)

	spaceIdx := strings.Index(line, " ")
	exclamIdx := strings.Index(line, "!")

	// Find the minimum valid index
	if spaceIdx != -1 && spaceIdx < endIdx {
		endIdx = spaceIdx
	}
	if exclamIdx != -1 && exclamIdx < endIdx {
		endIdx = exclamIdx
	}

	parentID := strings.TrimSpace(line[:endIdx])

	// Validate it's an HP ID
	if strings.HasPrefix(parentID, "HP:") {
		return parentID
	}

	return ""
}

// saveParentChildRelations creates parent/child cross-references for hierarchical relationships
func (h *hpo) saveParentChildRelations(childID string, hpoDatasetID string,
	parentDatasetID string, childDatasetID string, frparentStr string, frchildStr string, parents []string) {

	for _, parentID := range parents {
		if parentID == "" || parentID == childID {
			continue
		}

		// Create parent relationships
		// childID -> parent link
		h.d.addXref2(childID, hpoDatasetID, parentID, frparentStr)
		// parent term itself links back to parent dataset
		h.d.addXref2(parentID, parentDatasetID, parentID, h.source)

		// Create child relationships
		// parentID -> child link
		h.d.addXref2(parentID, hpoDatasetID, childID, frchildStr)
		// child term itself links back to child dataset
		h.d.addXref2(childID, childDatasetID, childID, h.source)
	}
}

// parseGeneToPhenotype parses genes_to_phenotype.txt file
// Format: ncbi_gene_id\tgene_symbol\thpo_id\thpo_name\tfrequency\tdisease_id
func (h *hpo) parseGeneToPhenotype(path string) {
	var br *bufio.Reader

	// Get dataset IDs for cross-references
	hpoDatasetID := config.Dataconf[h.source]["id"]
	hgncDatasetID := config.Dataconf["hgnc"]["id"]

	if config.Dataconf[h.source]["useLocalFile"] == "yes" {
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
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	lineNum := 0
	associationCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// Skip header
		if lineNum == 1 {
			continue
		}

		// Parse tab-delimited line
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}

		geneSymbol := fields[1]
		hpoID := fields[2]

		if geneSymbol == "" || hpoID == "" {
			continue
		}

		// Create bidirectional cross-reference: Gene ↔ HPO term
		// addXref expects: (key, fromDatasetID, value, toDatasetName, isLink)

		// Gene symbol → HPO term
		h.d.addXref(geneSymbol, hgncDatasetID, hpoID, h.source, false)

		// HPO term → Gene symbol
		h.d.addXref(hpoID, hpoDatasetID, geneSymbol, "hgnc", false)

		associationCount++
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}
}
