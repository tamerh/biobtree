package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// ctd handles parsing CTD (Comparative Toxicogenomics Database) data
// Source: https://ctdbase.org/
// Files processed:
// - CTD_chemicals.tsv.gz: Chemical vocabulary with MeSH IDs
// - CTD_chem_gene_ixns.tsv.gz: Chemical-gene interactions
// - CTD_chemicals_diseases.tsv.gz: Chemical-disease associations
type ctd struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (c *ctd) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// Chemical entry aggregating data from multiple CTD files
type ctdChemicalEntry struct {
	chemicalID   string
	chemicalName string
	casRN        string
	definition   string
	pubchemCID   string
	inchiKey     string
	synonyms     []string
	treeNumbers  []string

	// Gene interactions (from CTD_chem_gene_ixns.tsv)
	geneInteractions []*pbuf.CtdGeneInteraction

	// Disease associations (from CTD_chemicals_diseases.tsv)
	diseaseAssociations []*pbuf.CtdDiseaseAssociation

	// Inferred genes from disease associations
	inferredGenes map[string]bool
}

func (c *ctd) update() {
	defer c.d.wg.Done()

	log.Println("CTD: Starting comprehensive data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("CTD: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Phase 1: Load chemical vocabulary to build base entries
	chemicals := c.loadChemicals(testLimit)
	log.Printf("CTD: Phase 1 complete - loaded %d chemicals", len(chemicals))

	// Phase 2: Add gene interactions
	c.loadChemGeneInteractions(chemicals, testLimit)
	log.Printf("CTD: Phase 2 complete - added gene interactions")

	// Phase 3: Add disease associations
	c.loadChemDiseaseAssociations(chemicals, testLimit)
	log.Printf("CTD: Phase 3 complete - added disease associations")

	// Phase 4: Save all entries and create cross-references
	c.saveEntries(chemicals, idLogFile)

	log.Printf("CTD: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress tracker
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}

// loadChemicals reads CTD_chemicals.tsv.gz and creates base chemical entries
// CTD_chemicals.tsv columns:
// ChemicalName, ChemicalID, CasRN, PubChemCID, PubChemSID, DTXSID, InChIKey, Definition,
// ParentIDs, TreeNumbers, ParentTreeNumbers, MESHSynonyms, CTDCuratedSynonyms
func (c *ctd) loadChemicals(testLimit int) map[string]*ctdChemicalEntry {
	basePath := config.Dataconf[c.source]["path"]
	chemFile := config.Dataconf[c.source]["chemicalsFile"]
	filePath := basePath + chemFile

	log.Printf("CTD: Loading chemicals from %s", filePath)

	reader, cleanup := c.openGzipURL(filePath)
	defer cleanup()

	scanner := bufio.NewScanner(reader)
	// Increase buffer for long lines
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	chemicals := make(map[string]*ctdChemicalEntry)
	var lineNum int
	var entryCount int

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}

		chemName := fields[0]
		chemID := fields[1]

		// CTD uses MESH: prefix for chemical IDs
		// Normalize to just the ID (e.g., "MESH:D000082" -> "D000082")
		normalizedID := normalizeCtdID(chemID)
		if normalizedID == "" {
			continue
		}

		entry := &ctdChemicalEntry{
			chemicalID:    normalizedID,
			chemicalName:  chemName,
			inferredGenes: make(map[string]bool),
		}

		// CAS Registry Number
		if len(fields) > 2 {
			entry.casRN = fields[2]
		}

		// PubChem CID - normalize by removing CID: prefix
		if len(fields) > 3 && fields[3] != "" {
			cid := fields[3]
			// CTD stores PubChem CIDs sometimes as "CID:12345" or just "12345"
			cid = strings.TrimPrefix(cid, "CID:")
			entry.pubchemCID = cid
		}

		// InChI Key
		if len(fields) > 6 {
			entry.inchiKey = fields[6]
		}

		// Definition
		if len(fields) > 7 {
			entry.definition = fields[7]
		}

		// Tree Numbers
		if len(fields) > 9 && fields[9] != "" {
			entry.treeNumbers = splitAndClean(fields[9], "|")
		}

		// MeSH Synonyms
		if len(fields) > 11 && fields[11] != "" {
			entry.synonyms = splitAndClean(fields[11], "|")
		}

		// CTD Curated Synonyms
		if len(fields) > 12 && fields[12] != "" {
			ctdSynonyms := splitAndClean(fields[12], "|")
			entry.synonyms = append(entry.synonyms, ctdSynonyms...)
		}

		chemicals[normalizedID] = entry
		entryCount++

		if testLimit > 0 && entryCount >= testLimit {
			log.Printf("CTD: [TEST MODE] Reached chemical limit of %d", testLimit)
			break
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("CTD: Scanner error reading chemicals: %v", err)
	}

	return chemicals
}

// loadChemGeneInteractions reads CTD_chem_gene_ixns.tsv.gz and adds to chemical entries
// Columns: ChemicalName, ChemicalID, CasRN, GeneSymbol, GeneID, GeneForms,
// Organism, OrganismID, Interaction, InteractionActions, PubMedIDs
func (c *ctd) loadChemGeneInteractions(chemicals map[string]*ctdChemicalEntry, testLimit int) {
	basePath := config.Dataconf[c.source]["path"]
	ixnFile := config.Dataconf[c.source]["chemGeneIxnsFile"]
	filePath := basePath + ixnFile

	log.Printf("CTD: Loading chemical-gene interactions from %s", filePath)

	reader, cleanup := c.openGzipURL(filePath)
	defer cleanup()

	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var lineNum int
	var ixnCount int

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 11 {
			continue
		}

		chemID := normalizeCtdID(fields[1])
		if chemID == "" {
			continue
		}

		entry, exists := chemicals[chemID]
		if !exists {
			// Chemical not in our vocabulary, skip
			continue
		}

		// Parse organism ID
		var organismID int32
		if fields[7] != "" {
			if oid, err := strconv.ParseInt(fields[7], 10, 32); err == nil {
				organismID = int32(oid)
			}
		}

		// Parse interaction actions
		var actions []string
		if fields[9] != "" {
			actions = splitAndClean(fields[9], "|")
		}

		// Parse PubMed IDs
		var pubmedIDs []string
		if fields[10] != "" {
			pubmedIDs = splitAndClean(fields[10], "|")
		}

		ixn := &pbuf.CtdGeneInteraction{
			GeneSymbol:         fields[3],
			GeneId:             fields[4],
			Organism:           fields[6],
			OrganismId:         organismID,
			Interaction:        fields[8],
			InteractionActions: actions,
			GeneForms:          fields[5],
			PubmedIds:          pubmedIDs,
		}

		// Limit to top 50 interactions per chemical to avoid huge entries
		if len(entry.geneInteractions) < 50 {
			entry.geneInteractions = append(entry.geneInteractions, ixn)
		}

		ixnCount++

		// Progress logging
		if ixnCount%100000 == 0 {
			log.Printf("CTD: Processed %d gene interactions...", ixnCount)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("CTD: Scanner error reading gene interactions: %v", err)
	}

	log.Printf("CTD: Total gene interactions processed: %d", ixnCount)
}

// loadChemDiseaseAssociations reads CTD_chemicals_diseases.tsv.gz
// Columns: ChemicalName, ChemicalID, CasRN, DiseaseName, DiseaseID,
// DirectEvidence, InferenceGeneSymbol, InferenceScore, OmimIDs, PubMedIDs
func (c *ctd) loadChemDiseaseAssociations(chemicals map[string]*ctdChemicalEntry, testLimit int) {
	basePath := config.Dataconf[c.source]["path"]
	diseaseFile := config.Dataconf[c.source]["chemDiseasesFile"]
	filePath := basePath + diseaseFile

	log.Printf("CTD: Loading chemical-disease associations from %s", filePath)

	reader, cleanup := c.openGzipURL(filePath)
	defer cleanup()

	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var lineNum int
	var assocCount int

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			continue
		}

		chemID := normalizeCtdID(fields[1])
		if chemID == "" {
			continue
		}

		entry, exists := chemicals[chemID]
		if !exists {
			continue
		}

		// Parse inference score
		var inferenceScore float64
		if len(fields) > 7 && fields[7] != "" {
			if score, err := strconv.ParseFloat(fields[7], 64); err == nil {
				inferenceScore = score
			}
		}

		// Parse OMIM IDs
		var omimIDs []string
		if len(fields) > 8 && fields[8] != "" {
			omimIDs = splitAndClean(fields[8], "|")
		}

		// Parse PubMed IDs
		var pubmedIDs []string
		if len(fields) > 9 && fields[9] != "" {
			pubmedIDs = splitAndClean(fields[9], "|")
		}

		assoc := &pbuf.CtdDiseaseAssociation{
			DiseaseName:         fields[3],
			DiseaseId:           normalizeCtdID(fields[4]),
			DirectEvidence:      fields[5],
			InferenceGeneSymbol: safeField(fields, 6),
			InferenceScore:      inferenceScore,
			OmimIds:             omimIDs,
			PubmedIds:           pubmedIDs,
		}

		// Track inferred genes
		if assoc.InferenceGeneSymbol != "" {
			entry.inferredGenes[assoc.InferenceGeneSymbol] = true
		}

		// Limit to top 30 disease associations per chemical
		if len(entry.diseaseAssociations) < 30 {
			entry.diseaseAssociations = append(entry.diseaseAssociations, assoc)
		}

		assocCount++

		if assocCount%100000 == 0 {
			log.Printf("CTD: Processed %d disease associations...", assocCount)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("CTD: Scanner error reading disease associations: %v", err)
	}

	log.Printf("CTD: Total disease associations processed: %d", assocCount)
}

// saveEntries saves all chemical entries and creates cross-references
func (c *ctd) saveEntries(chemicals map[string]*ctdChemicalEntry, idLogFile *os.File) {
	sourceID := config.Dataconf[c.source]["id"]
	var savedCount int

	for chemID, entry := range chemicals {
		// Build protobuf attribute
		attr := c.buildCtdAttr(entry)

		// Marshal attributes
		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("CTD: Error marshaling %s: %v", chemID, err)
			continue
		}

		// Save primary entry
		c.d.addProp3(chemID, sourceID, attrBytes)

		// Create cross-references
		c.createCrossReferences(chemID, entry, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(chemID + "\n")
		}

		savedCount++

		if savedCount%10000 == 0 {
			log.Printf("CTD: Saved %d entries...", savedCount)
		}
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(savedCount))
	log.Printf("CTD: Total entries saved: %d", savedCount)
}

// buildCtdAttr creates the protobuf attribute for a chemical entry
func (c *ctd) buildCtdAttr(entry *ctdChemicalEntry) *pbuf.CtdAttr {
	// Collect inferred gene symbols
	var inferredGenes []string
	for gene := range entry.inferredGenes {
		inferredGenes = append(inferredGenes, gene)
		if len(inferredGenes) >= 20 {
			break // Limit
		}
	}

	return &pbuf.CtdAttr{
		ChemicalId:              entry.chemicalID,
		ChemicalName:            entry.chemicalName,
		CasRn:                   entry.casRN,
		Definition:              entry.definition,
		Synonyms:                entry.synonyms,
		MeshTreeNumbers:         entry.treeNumbers,
		PubchemCid:              entry.pubchemCID,
		InchiKey:                entry.inchiKey,
		GeneInteractions:        entry.geneInteractions,
		GeneInteractionCount:    int32(len(entry.geneInteractions)),
		DiseaseAssociations:     entry.diseaseAssociations,
		DiseaseAssociationCount: int32(len(entry.diseaseAssociations)),
		InferredGenes:           inferredGenes,
	}
}

// createCrossReferences builds all cross-references for a CTD chemical entry
func (c *ctd) createCrossReferences(chemID string, entry *ctdChemicalEntry, sourceID string) {
	// Text search: chemical name and synonyms
	c.d.addXref(entry.chemicalName, textLinkID, chemID, c.source, true)

	for _, syn := range entry.synonyms {
		if syn != "" && len(syn) > 2 && len(syn) < 200 {
			c.d.addXref(syn, textLinkID, chemID, c.source, true)
		}
	}

	// Cross-reference: CTD → MeSH (bidirectional via mesh bucket)
	if _, exists := config.Dataconf["mesh"]; exists {
		c.d.addXref(chemID, sourceID, chemID, "mesh", false)
	}

	// Cross-reference: CTD → PubChem
	if entry.pubchemCID != "" {
		if _, exists := config.Dataconf["pubchem"]; exists {
			c.d.addXref(chemID, sourceID, entry.pubchemCID, "pubchem", false)
		}
	}

	// Cross-references for gene interactions
	genesSeen := make(map[string]bool)
	for _, ixn := range entry.geneInteractions {
		// CTD → Entrez Gene (via gene ID)
		if ixn.GeneId != "" && !genesSeen[ixn.GeneId] {
			genesSeen[ixn.GeneId] = true
			if _, exists := config.Dataconf["entrez"]; exists {
				c.d.addXref(chemID, sourceID, ixn.GeneId, "entrez", false)
			}
		}

		// CTD → Taxonomy (via organism ID)
		if ixn.OrganismId > 0 {
			taxID := strconv.Itoa(int(ixn.OrganismId))
			if _, exists := config.Dataconf["taxonomy"]; exists {
				c.d.addXref(chemID, sourceID, taxID, "taxonomy", false)
			}
		}

		// CTD → PubMed references
		for _, pmid := range ixn.PubmedIds {
			if pmid != "" {
				if _, exists := config.Dataconf["literature_mappings"]; exists {
					c.d.addXref(chemID, sourceID, pmid, "literature_mappings", false)
				}
			}
		}
	}

	// Cross-references for disease associations
	diseasesSeen := make(map[string]bool)
	for _, assoc := range entry.diseaseAssociations {
		diseaseID := assoc.DiseaseId
		if diseaseID != "" && !diseasesSeen[diseaseID] {
			diseasesSeen[diseaseID] = true

			// CTD → MeSH disease
			if strings.HasPrefix(diseaseID, "MESH:") || strings.HasPrefix(diseaseID, "D") || strings.HasPrefix(diseaseID, "C") {
				normalizedDiseaseID := normalizeCtdID(diseaseID)
				if _, exists := config.Dataconf["mesh"]; exists {
					c.d.addXref(chemID, sourceID, normalizedDiseaseID, "mesh", false)
				}
			}
		}

		// CTD → OMIM
		for _, omimID := range assoc.OmimIds {
			if omimID != "" {
				// OMIM IDs in CTD may have "OMIM:" prefix
				normalizedOMIM := strings.TrimPrefix(omimID, "OMIM:")
				if _, exists := config.Dataconf["omim"]; exists {
					c.d.addXref(chemID, sourceID, normalizedOMIM, "omim", false)
				}
				// Also create MONDO xref via OMIM (MONDO has OMIM mappings)
				// Format: OMIM:123456 → MONDO uses this for disease linking
				if _, exists := config.Dataconf["mondo"]; exists {
					mondoOMIM := "OMIM:" + normalizedOMIM
					c.d.addXref(chemID, sourceID, mondoOMIM, "mondo", false)
				}
			}
		}

		// CTD → EFO (via MeSH disease ID, EFO has MeSH mappings)
		if diseaseID != "" {
			normalizedDiseaseID := normalizeCtdID(diseaseID)
			if normalizedDiseaseID != "" {
				if _, exists := config.Dataconf["efo"]; exists {
					// EFO uses MESH: prefix for MeSH mappings
					efoMeshID := "MESH:" + normalizedDiseaseID
					c.d.addXref(chemID, sourceID, efoMeshID, "efo", false)
				}
			}
		}
	}
}

// openGzipURL opens a gzipped file from HTTP URL
func (c *ctd) openGzipURL(url string) (*bufio.Reader, func()) {
	log.Printf("CTD: Downloading %s", url)

	resp, err := http.Get(url)
	c.check(err, "HTTP GET "+url)

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("CTD: HTTP error %d for %s", resp.StatusCode, url)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	c.check(err, "creating gzip reader for "+url)

	reader := bufio.NewReader(gzReader)

	cleanup := func() {
		gzReader.Close()
		resp.Body.Close()
	}

	return reader, cleanup
}

// normalizeCtdID removes MESH: prefix and validates ID format
func normalizeCtdID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}

	// Remove MESH: prefix
	id = strings.TrimPrefix(id, "MESH:")

	// Validate format: should start with letter followed by digits
	if len(id) < 2 {
		return ""
	}

	// CTD uses D (descriptor), C (supplementary), and OMIM formats
	firstChar := id[0]
	if firstChar != 'D' && firstChar != 'C' && (firstChar < '0' || firstChar > '9') {
		// Could be OMIM ID (starts with digit) or other format
		if firstChar >= '0' && firstChar <= '9' {
			return id // OMIM-like numeric ID
		}
		return "" // Invalid format
	}

	return id
}

// safeField returns field at index or empty string if out of bounds
func safeField(fields []string, idx int) string {
	if idx < len(fields) {
		return fields[idx]
	}
	return ""
}
