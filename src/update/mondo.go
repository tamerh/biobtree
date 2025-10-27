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
	frparentStr := m.source + "parent"
	frchildStr := m.source + "child"
	frparent := config.Dataconf[frparentStr]["id"]
	frchild := config.Dataconf[frchildStr]["id"]

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
	var parents []string
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
				m.saveParentChildRelations(currentID, fr, frparent, frchild, parents)
				total++
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
		} else if strings.HasPrefix(line, "is_a: MONDO:") {
			// Parse parent relationship: is_a: MONDO:0000001 ! disease or disorder
			parentID := extractParentID(line)
			if parentID != "" {
				parents = append(parents, parentID)
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
		m.saveParentChildRelations(currentID, fr, frparent, frchild, parents)
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

	// Deduplicate search terms to avoid duplicate text xrefs
	searchTerms := make(map[string]bool)

	// Add disease name to search terms
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
		m.d.addXref(term, textLinkID, id, m.source, true)
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

	// Map known databases to biobtree dataset names (not IDs!)
	// addXref expects dataset names like "efo", "orphanet", not IDs like "22", "55"
	var targetDatasetName string
	var targetID string

	if strings.HasPrefix(xrefID, "EFO:") {
		// EFO is dataset 22 in biobtree
		targetDatasetName = "efo"
		targetID = xrefID
	} else if strings.HasPrefix(xrefID, "Orphanet:") {
		// Orphanet is dataset 55 in biobtree (10,344 xrefs available)
		targetDatasetName = "orphanet"
		targetID = xrefID
	} else if strings.HasPrefix(xrefID, "HGNC:") {
		// HGNC is dataset 5 in biobtree (55 xrefs available)
		targetDatasetName = "hgnc"
		targetID = xrefID
	} else if strings.HasPrefix(xrefID, "PMID:") {
		// PMID via literature_mappings is dataset 12 (30 xrefs available)
		targetDatasetName = "literature_mappings"
		targetID = xrefID
	} else if strings.HasPrefix(xrefID, "OMIM:") {
		// OMIM is "mim" dataset 51 in biobtree (10,038 xrefs available)
		targetDatasetName = "mim"
		// Strip "OMIM:" prefix, keep only the numeric ID
		targetID = strings.TrimPrefix(xrefID, "OMIM:")
	} else if strings.HasPrefix(xrefID, "OMIMPS:") {
		// OMIM Phenotypic Series also maps to "mim" dataset 51 (601 xrefs available)
		targetDatasetName = "mim"
		// Strip "OMIMPS:" prefix, keep only the numeric ID
		targetID = strings.TrimPrefix(xrefID, "OMIMPS:")
	} else if strings.HasPrefix(xrefID, "DOID:") {
		// TODO: Disease Ontology - not currently in biobtree (11,866 xrefs available)
		// Would provide comprehensive disease classification
		return
	} else if strings.HasPrefix(xrefID, "MESH:") {
		// TODO: MeSH - not currently in biobtree (8,378 xrefs available)
		// Medical Subject Headings - would enable PubMed literature linking
		return
	} else if strings.HasPrefix(xrefID, "NCIT:") {
		// TODO: NCI Thesaurus - not currently in biobtree (7,550 xrefs available)
		// Cancer-focused terminology from National Cancer Institute
		return
	} else if strings.HasPrefix(xrefID, "UMLS:") {
		// TODO: UMLS - not currently in biobtree (21,381 xrefs available)
		// Unified Medical Language System - comprehensive medical terminology
		return
	} else if strings.HasPrefix(xrefID, "MEDGEN:") {
		// TODO: MEDGEN - not currently in biobtree (21,381 xrefs available)
		// NCBI's gene-disease relationships database
		return
	} else if strings.HasPrefix(xrefID, "GARD:") {
		// TODO: GARD - not currently in biobtree (10,730 xrefs available)
		// Genetic and Rare Diseases Information Center
		return
	} else if strings.HasPrefix(xrefID, "SCTID:") {
		// TODO: SNOMED CT - not currently in biobtree (9,278 xrefs available)
		// Clinical terminology standard
		return
	} else if strings.HasPrefix(xrefID, "ICD9:") {
		// TODO: ICD-9 - not currently in biobtree (5,732 xrefs available)
		// International Classification of Diseases version 9
		return
	} else if strings.HasPrefix(xrefID, "ICD10") {
		// TODO: ICD-10 variants - not currently in biobtree (2,918 xrefs available)
		// ICD10CM, ICD10WHO, ICD10EXP
		return
	} else if strings.HasPrefix(xrefID, "icd11.foundation:") {
		// TODO: ICD-11 - not currently in biobtree (4,170 xrefs available)
		// Latest version of International Classification of Diseases
		return
	} else if strings.HasPrefix(xrefID, "NANDO:") {
		// TODO: NANDO - not currently in biobtree (2,345 xrefs available)
		// Nanbyo Disease Ontology (Japanese rare diseases)
		return
	} else if strings.HasPrefix(xrefID, "MedDRA:") {
		// TODO: MedDRA - not currently in biobtree (1,488 xrefs available)
		// Medical Dictionary for Regulatory Activities
		return
	} else if strings.HasPrefix(xrefID, "NORD:") {
		// TODO: NORD - not currently in biobtree (908 xrefs available)
		// National Organization for Rare Disorders
		return
	} else if strings.HasPrefix(xrefID, "HP:") {
		// TODO: Human Phenotype Ontology - not currently in biobtree (579 xrefs available)
		// Phenotypic abnormalities in human disease
		return
	} else {
		// Unknown xref type, skip
		return
	}

	// Create bidirectional cross-reference if we found a mapping
	if targetDatasetName != "" && targetID != "" {
		// mondoID (e.g., MONDO:0005138) in mondo dataset -> targetID (e.g., EFO:0001071) in target dataset
		// addXref expects: (key, fromDatasetID, value, toDatasetName, isLink)
		m.d.addXref(mondoID, mondoDatasetID, targetID, targetDatasetName, false)
		// Reverse: targetID in target dataset -> mondoID in mondo dataset
		// Need to get the target dataset ID from config
		targetDatasetID := config.Dataconf[targetDatasetName]["id"]
		m.d.addXref(targetID, targetDatasetID, mondoID, m.source, false)
	}
}

// extractParentID extracts the parent MONDO ID from an is_a line
// Example: is_a: MONDO:0000001 ! disease or disorder
// Example: is_a: MONDO:0000001 {source="..."} ! disease or disorder
func extractParentID(line string) string {
	line = strings.TrimPrefix(line, "is_a: ")

	// Find the space, exclamation mark, or opening brace (whichever comes first)
	endIdx := len(line)

	spaceIdx := strings.Index(line, " ")
	braceIdx := strings.Index(line, "{")
	exclamIdx := strings.Index(line, "!")

	// Find the minimum valid index
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

	// Validate it's a MONDO ID
	if strings.HasPrefix(parentID, "MONDO:") {
		return parentID
	}

	return ""
}

// saveParentChildRelations creates parent/child cross-references for hierarchical relationships
func (m *mondo) saveParentChildRelations(childID string, mondoDatasetID string,
	parentDatasetID string, childDatasetID string, parents []string) {

	for _, parentID := range parents {
		if parentID == "" || parentID == childID {
			continue
		}

		// Create parent relationships
		// childID -> parent link
		m.d.addXref2(childID, mondoDatasetID, parentID, m.source+"parent")
		// parent term itself links back to parent dataset
		m.d.addXref2(parentID, parentDatasetID, parentID, m.source)

		// Create child relationships
		// parentID -> child link
		m.d.addXref2(parentID, mondoDatasetID, childID, m.source+"child")
		// child term itself links back to child dataset
		m.d.addXref2(childID, childDatasetID, childID, m.source)
	}
}
