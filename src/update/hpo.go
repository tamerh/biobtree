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

// check provides context-aware error checking for hpo processor
func (h *hpo) check(err error, operation string) {
	checkWithContext(err, h.source, operation)
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
	var attr pbuf.HPOAttr
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
				if config.IsTestMode() && shouldStopProcessing(testLimit, int(total)) {
					goto phase2
				}
			}

			// Reset for new term
			inTerm = true
			isObsolete = false
			currentID = ""
			parents = []string{}
			attr = pbuf.HPOAttr{
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

	// Phase 3: Parse phenotype.hpoa for disease-phenotype associations
	// This provides comprehensive HPO ↔ OMIM/Orphanet/MONDO mappings (~280K annotations)
	annotationsPath := config.Dataconf[h.source]["phenotype_annotations_path"]
	if annotationsPath != "" {
		h.parsePhenotypeAnnotations(annotationsPath)
	}

	h.d.progChan <- &progressInfo{dataset: h.source, done: true}
	atomic.AddUint64(&h.d.totalParsedEntry, total)
}

func (h *hpo) saveEntry(id string, datasetID string, attr *pbuf.HPOAttr) {
	// Note: HPOAttr.Diseases will be populated via xrefs from phenotype.hpoa
	// The diseases field can be used for embedded queryable attributes in future
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

	// Get dataset ID for HPO
	hpoDatasetID := config.Dataconf[h.source]["id"]

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

		// Create cross-reference: HPO term → human gene databases
		// addHumanGeneXrefsAll creates xrefs to HGNC, Entrez, and Ensembl
		h.d.addHumanGeneXrefsAll(geneSymbol, hpoID, hpoDatasetID)

		associationCount++
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

// parsePhenotypeAnnotations parses phenotype.hpoa file for disease-phenotype associations
// This is the official HPO annotation file containing ~280K disease-phenotype associations
// Format: 12 tab-separated columns (see https://obophenotype.github.io/human-phenotype-ontology/annotations/phenotype_hpoa/)
//
// Columns:
// 0: database_id    - OMIM:619340, ORPHA:558, or MONDO:0007947
// 1: disease_name   - Disease label
// 2: qualifier      - "NOT" or empty (skip NOT annotations)
// 3: hpo_id         - HP:0011097
// 4: reference      - PMID:12345 or OMIM:123456
// 5: evidence       - PCS (published clinical study), TAS (traceable author statement), IEA (inferred)
// 6: onset          - Age of onset HP term (optional)
// 7: frequency      - "1/2", "HP:0040283" (optional)
// 8: sex            - MALE, FEMALE (optional)
// 9: modifier       - Clinical modifiers (optional)
// 10: aspect        - P (phenotype), I (inheritance), C (clinical course), M (modifier), H (history)
// 11: biocuration   - Curator and date
func (h *hpo) parsePhenotypeAnnotations(path string) {
	var br *bufio.Reader

	// Get dataset ID for HPO
	hpoDatasetID := config.Dataconf[h.source]["id"]

	if config.Dataconf[h.source]["useLocalFile"] == "yes" {
		file, err := os.Open(filepath.FromSlash(path))
		h.check(err, "opening phenotype annotations file")
		br = bufio.NewReaderSize(file, fileBufSize)
		defer file.Close()
	} else {
		resp, err := http.Get(path)
		h.check(err, "downloading phenotype annotations file")
		br = bufio.NewReaderSize(resp.Body, fileBufSize)
		defer resp.Body.Close()
	}

	scanner := bufio.NewScanner(br)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var annotationCount int64
	var omimCount, orphaCount, mondoCount int64

	for scanner.Scan() {
		line := scanner.Text()

		// Skip comment lines (start with #) and header line
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		// Skip header line
		if strings.HasPrefix(line, "database_id") {
			continue
		}

		// Parse tab-delimited line
		fields := strings.Split(line, "\t")
		if len(fields) < 11 {
			continue
		}

		databaseID := fields[0]   // OMIM:619340, ORPHA:558, MONDO:xxx
		qualifier := fields[2]    // "NOT" or empty
		hpoID := fields[3]        // HP:0011097
		evidence := fields[5]     // PCS, TAS, IEA
		frequency := fields[7]    // "1/2" or "HP:0040283" (optional)
		aspect := fields[10]      // P, I, C, M, H

		// Skip negative associations (NOT qualifier)
		if qualifier == "NOT" {
			continue
		}

		// Only process phenotype associations (aspect P)
		// Skip: I (inheritance), C (clinical course), M (modifier), H (history)
		if aspect != "P" {
			continue
		}

		// Validate HPO ID
		if !strings.HasPrefix(hpoID, "HP:") {
			continue
		}

		// Build evidence string with frequency if available
		// Format: "PCS;freq=3/8" or just "PCS"
		evidenceStr := evidence
		if frequency != "" && !strings.HasPrefix(frequency, "HP:") {
			// Only include numeric frequencies (e.g., "3/8"), not HP terms
			evidenceStr = evidence + ";freq=" + frequency
		}

		// Determine target dataset and ID from database_id prefix
		var targetDataset string
		var targetID string

		switch {
		case strings.HasPrefix(databaseID, "OMIM:"):
			targetDataset = "mim"
			// MIM uses numeric IDs without prefix
			targetID = strings.TrimPrefix(databaseID, "OMIM:")
			omimCount++
		case strings.HasPrefix(databaseID, "ORPHA:"):
			targetDataset = "orphanet"
			// Orphanet uses numeric IDs only (e.g., "558" not "ORPHA:558")
			targetID = strings.TrimPrefix(databaseID, "ORPHA:")
			orphaCount++
		case strings.HasPrefix(databaseID, "MONDO:"):
			targetDataset = "mondo"
			targetID = databaseID
			mondoCount++
		case strings.HasPrefix(databaseID, "DECIPHER:"):
			// DECIPHER is not currently in biobtree, skip
			continue
		default:
			// Unknown database, skip
			continue
		}

		// Verify target dataset exists in config
		if _, ok := config.Dataconf[targetDataset]; !ok {
			continue
		}

		// Create bidirectional cross-reference with evidence
		// HPO term → Disease
		// Disease → HPO term (automatic reverse via addXrefWithEvidence)
		h.d.addXrefWithEvidence(hpoID, hpoDatasetID, targetID, targetDataset, false, evidenceStr)

		annotationCount++
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	// Log annotation statistics
	if annotationCount > 0 {
		_ = omimCount   // Used for potential debug logging
		_ = orphaCount  // Used for potential debug logging
		_ = mondoCount  // Used for potential debug logging
		// log.Printf("[HPO] Parsed %d disease-phenotype annotations (OMIM: %d, Orphanet: %d, MONDO: %d)",
		//     annotationCount, omimCount, orphaCount, mondoCount)
	}
}
