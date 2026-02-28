package update

import (
	"biobtree/pbuf"
	"bufio"
	"compress/gzip"
	"fmt"
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
// - CTD_chem_gene_ixns.tsv.gz: Chemical-gene interactions (stored as ctd_gene_interaction)
// - CTD_chemicals_diseases.tsv.gz: Chemical-disease associations (stored as ctd_disease_association)
//
// Data structure:
// - Main CTD entries store chemical metadata and counts
// - Gene interactions are stored as separate ctd_gene_interaction entries
// - Disease associations are stored as separate ctd_disease_association entries
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

	// Counts only - actual data stored in separate datasets
	geneInteractionCount    int32
	diseaseAssociationCount int32

	// Inferred genes from disease associations (summary)
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

	// Phase 2: Process gene interactions (save as separate entries)
	c.loadChemGeneInteractions(chemicals, testLimit)
	log.Printf("CTD: Phase 2 complete - saved gene interactions as separate entries")

	// Phase 3: Process disease associations (save as separate entries)
	c.loadChemDiseaseAssociations(chemicals, testLimit)
	log.Printf("CTD: Phase 3 complete - saved disease associations as separate entries")

	// Phase 4: Save all chemical entries and create cross-references
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

// geneInteractionAggregated holds aggregated data for a unique chemical-gene-organism interaction
type geneInteractionAggregated struct {
	chemID       string
	chemName     string
	geneSymbol   string
	geneID       string
	organism     string
	organismID   int32
	geneForms    map[string]bool   // unique gene forms
	interactions []string          // all interaction descriptions
	actions      map[string]bool   // unique interaction actions
	pubmedIDs    map[string]bool   // unique PubMed IDs
}

// diseaseAssociationAggregated holds aggregated data for a unique chemical-disease association
type diseaseAssociationAggregated struct {
	chemID            string
	chemName          string
	diseaseName       string
	diseaseID         string
	directEvidence    map[string]bool   // unique direct evidence types
	inferenceGenes    map[string]bool   // unique inference gene symbols
	maxInferenceScore float64           // maximum inference score
	omimIDs           map[string]bool   // unique OMIM IDs
	pubmedIDs         map[string]bool   // unique PubMed IDs
}

// loadChemGeneInteractions reads CTD_chem_gene_ixns.tsv.gz and saves each interaction
// as a separate ctd_gene_interaction entry with cross-references
// Columns: ChemicalName, ChemicalID, CasRN, GeneSymbol, GeneID, GeneForms,
// Organism, OrganismID, Interaction, InteractionActions, PubMedIDs
// Note: CTD file has multiple rows per chemical-gene-organism with different interaction details
// We aggregate all rows into a single entry per unique interaction ID
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

	// Get dataset IDs
	geneInteractionSourceID := config.Dataconf["ctd_gene_interaction"]["id"]

	var lineNum int
	var rowCount int

	// First pass: aggregate all rows by unique interactionID
	aggregated := make(map[string]*geneInteractionAggregated)

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

		geneID := fields[4]
		if geneID == "" {
			continue
		}

		// Parse organism ID
		var organismID int32
		if fields[7] != "" {
			if oid, err := strconv.ParseInt(fields[7], 10, 32); err == nil {
				organismID = int32(oid)
			}
		}

		// Create unique interaction ID: {chemID}_{geneID}_{organismID}
		interactionID := fmt.Sprintf("%s_%s_%d", chemID, geneID, organismID)

		// Get or create aggregated entry
		agg, exists := aggregated[interactionID]
		if !exists {
			agg = &geneInteractionAggregated{
				chemID:       chemID,
				chemName:     entry.chemicalName,
				geneSymbol:   fields[3],
				geneID:       geneID,
				organism:     fields[6],
				organismID:   organismID,
				geneForms:    make(map[string]bool),
				interactions: []string{},
				actions:      make(map[string]bool),
				pubmedIDs:    make(map[string]bool),
			}
			aggregated[interactionID] = agg
		}

		// Add gene form
		if fields[5] != "" {
			agg.geneForms[fields[5]] = true
		}

		// Add interaction description (limit to prevent huge entries)
		if fields[8] != "" && len(agg.interactions) < 20 {
			agg.interactions = append(agg.interactions, fields[8])
		}

		// Add interaction actions
		if fields[9] != "" {
			for _, action := range splitAndClean(fields[9], "|") {
				agg.actions[action] = true
			}
		}

		// Add PubMed IDs
		if fields[10] != "" {
			for _, pmid := range splitAndClean(fields[10], "|") {
				agg.pubmedIDs[pmid] = true
			}
		}

		rowCount++
	}

	if err := scanner.Err(); err != nil {
		log.Printf("CTD: Scanner error reading gene interactions: %v", err)
	}

	log.Printf("CTD: Aggregated %d rows into %d unique gene interactions", rowCount, len(aggregated))

	// Second pass: create entries and xrefs from aggregated data
	var savedCount int
	for interactionID, agg := range aggregated {
		// Convert maps to slices
		var geneForms []string
		for form := range agg.geneForms {
			geneForms = append(geneForms, form)
		}
		var actions []string
		for action := range agg.actions {
			actions = append(actions, action)
		}
		var pubmedIDs []string
		for pmid := range agg.pubmedIDs {
			pubmedIDs = append(pubmedIDs, pmid)
		}

		// Join interaction descriptions
		interaction := strings.Join(agg.interactions, " | ")

		// Build interaction attribute (PubMed IDs stored as xrefs, not in attr)
		attr := &pbuf.CtdGeneInteractionAttr{
			InteractionId:      interactionID,
			ChemicalId:         agg.chemID,
			ChemicalName:       agg.chemName,
			GeneSymbol:         agg.geneSymbol,
			GeneId:             agg.geneID,
			Organism:           agg.organism,
			OrganismId:         agg.organismID,
			Interaction:        interaction,
			InteractionActions: actions,
			GeneForms:          strings.Join(geneForms, "|"),
			PubmedCount:        int32(len(pubmedIDs)),
		}

		// Marshal and save interaction entry
		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("CTD: Error marshaling gene interaction %s: %v", interactionID, err)
			continue
		}
		c.d.addProp3(interactionID, geneInteractionSourceID, attrBytes)

		// Cross-references with species priority sorting
		taxIDStr := strconv.Itoa(int(agg.organismID))
		sortLevels := []string{
			ComputeSortLevelValue(SortLevelSpeciesPriority, map[string]interface{}{"taxID": taxIDStr}),
		}

		// ctd_gene_interaction → ctd (chemical) with species sorting
		c.d.addXrefWithSortLevels(interactionID, geneInteractionSourceID, agg.chemID, "ctd", sortLevels)

		// ctd_gene_interaction → entrez (gene) with species sorting
		if _, exists := config.Dataconf["entrez"]; exists {
			c.d.addXrefWithSortLevels(interactionID, geneInteractionSourceID, agg.geneID, "entrez", sortLevels)
		}

		// ctd_gene_interaction → taxonomy
		if agg.organismID > 0 {
			if _, exists := config.Dataconf["taxonomy"]; exists {
				c.d.addXref(interactionID, geneInteractionSourceID, taxIDStr, "taxonomy", false)
			}
		}

		// ctd_gene_interaction → pubmed (supporting literature)
		if _, exists := config.Dataconf["pubmed"]; exists {
			for _, pmid := range pubmedIDs {
				c.d.addXref(interactionID, geneInteractionSourceID, pmid, "pubmed", false)
			}
		}

		// Increment count in chemical entry
		if entry, exists := chemicals[agg.chemID]; exists {
			entry.geneInteractionCount++
		}
		savedCount++

		// Progress logging
		if savedCount%100000 == 0 {
			log.Printf("CTD: Saved %d gene interactions...", savedCount)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("CTD: Scanner error reading gene interactions: %v", err)
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(savedCount))
	log.Printf("CTD: Total gene interactions processed: %d rows, saved: %d unique", rowCount, savedCount)
}

// loadChemDiseaseAssociations reads CTD_chemicals_diseases.tsv.gz and saves each association
// as a separate ctd_disease_association entry with cross-references
// Columns: ChemicalName, ChemicalID, CasRN, DiseaseName, DiseaseID,
// DirectEvidence, InferenceGeneSymbol, InferenceScore, OmimIDs, PubMedIDs
// Note: CTD file has multiple rows per chemical-disease pair with different evidence/genes
// We aggregate all rows into a single entry per unique association ID
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

	// Get dataset IDs
	diseaseAssocSourceID := config.Dataconf["ctd_disease_association"]["id"]

	var lineNum int
	var rowCount int

	// First pass: aggregate all rows by unique associationID
	aggregated := make(map[string]*diseaseAssociationAggregated)

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

		diseaseID := normalizeCtdID(fields[4])
		if diseaseID == "" {
			continue
		}

		// Create unique association ID: {chemID}_{diseaseID}
		associationID := fmt.Sprintf("%s_%s", chemID, diseaseID)

		// Get or create aggregated entry
		agg, exists := aggregated[associationID]
		if !exists {
			agg = &diseaseAssociationAggregated{
				chemID:            chemID,
				chemName:          entry.chemicalName,
				diseaseName:       fields[3],
				diseaseID:         diseaseID,
				directEvidence:    make(map[string]bool),
				inferenceGenes:    make(map[string]bool),
				maxInferenceScore: 0,
				omimIDs:           make(map[string]bool),
				pubmedIDs:         make(map[string]bool),
			}
			aggregated[associationID] = agg
		}

		// Add direct evidence
		if fields[5] != "" {
			agg.directEvidence[fields[5]] = true
		}

		// Add inference gene symbol
		if len(fields) > 6 && fields[6] != "" {
			agg.inferenceGenes[fields[6]] = true
		}

		// Track max inference score
		if len(fields) > 7 && fields[7] != "" {
			if score, err := strconv.ParseFloat(fields[7], 64); err == nil {
				if score > agg.maxInferenceScore {
					agg.maxInferenceScore = score
				}
			}
		}

		// Add OMIM IDs
		if len(fields) > 8 && fields[8] != "" {
			for _, omimID := range splitAndClean(fields[8], "|") {
				agg.omimIDs[omimID] = true
			}
		}

		// Add PubMed IDs
		if len(fields) > 9 && fields[9] != "" {
			for _, pmid := range splitAndClean(fields[9], "|") {
				agg.pubmedIDs[pmid] = true
			}
		}

		rowCount++
	}

	if err := scanner.Err(); err != nil {
		log.Printf("CTD: Scanner error reading disease associations: %v", err)
	}

	log.Printf("CTD: Aggregated %d rows into %d unique disease associations", rowCount, len(aggregated))

	// Second pass: create entries and xrefs from aggregated data
	var savedCount int
	for associationID, agg := range aggregated {
		// Convert maps to slices
		var directEvidence []string
		for ev := range agg.directEvidence {
			directEvidence = append(directEvidence, ev)
		}
		var inferenceGenes []string
		for gene := range agg.inferenceGenes {
			inferenceGenes = append(inferenceGenes, gene)
		}
		var omimIDs []string
		for omim := range agg.omimIDs {
			omimIDs = append(omimIDs, omim)
		}
		var pubmedIDs []string
		for pmid := range agg.pubmedIDs {
			pubmedIDs = append(pubmedIDs, pmid)
		}

		// Limit inference genes to prevent huge entries
		if len(inferenceGenes) > 50 {
			inferenceGenes = inferenceGenes[:50]
		}

		// Build association attribute (PubMed/OMIM IDs stored as xrefs, not in attr)
		attr := &pbuf.CtdDiseaseAssociationAttr{
			AssociationId:        associationID,
			ChemicalId:           agg.chemID,
			ChemicalName:         agg.chemName,
			DiseaseName:          agg.diseaseName,
			DiseaseId:            agg.diseaseID,
			DirectEvidence:       directEvidence,
			InferenceGeneSymbols: inferenceGenes,
			InferenceScore:       agg.maxInferenceScore,
			PubmedCount:          int32(len(pubmedIDs)),
		}

		// Track inferred genes in chemical entry (summary)
		if chemEntry, exists := chemicals[agg.chemID]; exists {
			for gene := range agg.inferenceGenes {
				chemEntry.inferredGenes[gene] = true
			}
		}

		// Marshal and save association entry
		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("CTD: Error marshaling disease association %s: %v", associationID, err)
			continue
		}
		c.d.addProp3(associationID, diseaseAssocSourceID, attrBytes)

		// Sort by inference score (higher = better)
		// Convert to int (multiply by 1000 for precision, invert for descending)
		scoreInt := int(agg.maxInferenceScore * 1000)
		sortLevels := []string{
			ComputeSortLevelValue(SortLevelInteractionScore, map[string]interface{}{"score": scoreInt}),
		}

		// ctd_disease_association → ctd (chemical) with score sorting
		c.d.addXrefWithSortLevels(associationID, diseaseAssocSourceID, agg.chemID, "ctd", sortLevels)

		// ctd_disease_association → mesh (disease) with score sorting
		if _, exists := config.Dataconf["mesh"]; exists {
			c.d.addXrefWithSortLevels(associationID, diseaseAssocSourceID, agg.diseaseID, "mesh", sortLevels)
		}

		// ctd_disease_association → OMIM
		for _, omimID := range omimIDs {
			if omimID != "" {
				normalizedOMIM := strings.TrimPrefix(omimID, "OMIM:")
				if _, exists := config.Dataconf["mim"]; exists {
					c.d.addXref(associationID, diseaseAssocSourceID, normalizedOMIM, "mim", false)
				}
			}
		}

		// ctd_disease_association → pubmed (supporting literature)
		if _, exists := config.Dataconf["pubmed"]; exists {
			for _, pmid := range pubmedIDs {
				c.d.addXref(associationID, diseaseAssocSourceID, pmid, "pubmed", false)
			}
		}

		// Increment count in chemical entry
		if chemEntry, exists := chemicals[agg.chemID]; exists {
			chemEntry.diseaseAssociationCount++
		}
		savedCount++

		if savedCount%100000 == 0 {
			log.Printf("CTD: Saved %d disease associations...", savedCount)
		}
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(savedCount))
	log.Printf("CTD: Total disease associations processed: %d rows, saved: %d unique", rowCount, savedCount)
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

		// Create cross-references (only for chemical metadata, not interactions)
		c.createCrossReferences(chemID, entry, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(chemID + "\n")
		}

		savedCount++

		if savedCount%10000 == 0 {
			log.Printf("CTD: Saved %d chemical entries...", savedCount)
		}
	}

	atomic.AddUint64(&c.d.totalParsedEntry, uint64(savedCount))
	log.Printf("CTD: Total chemical entries saved: %d", savedCount)
}

// buildCtdAttr creates the protobuf attribute for a chemical entry
// Note: Gene interactions and disease associations are stored separately
func (c *ctd) buildCtdAttr(entry *ctdChemicalEntry) *pbuf.CtdAttr {
	// Collect inferred gene symbols (summary)
	var inferredGenes []string
	for gene := range entry.inferredGenes {
		inferredGenes = append(inferredGenes, gene)
		if len(inferredGenes) >= 50 {
			break // Limit for summary
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
		// Gene interactions stored separately in ctd_gene_interaction
		GeneInteractionCount:    entry.geneInteractionCount,
		// Disease associations stored separately in ctd_disease_association
		DiseaseAssociationCount: entry.diseaseAssociationCount,
		InferredGenes:           inferredGenes,
	}
}

// createCrossReferences builds cross-references for a CTD chemical entry
// Note: Interaction/association xrefs are created during their respective load phases
func (c *ctd) createCrossReferences(chemID string, entry *ctdChemicalEntry, sourceID string) {
	// Text search: chemical name and synonyms
	c.d.addXref(entry.chemicalName, textLinkID, chemID, c.source, true)

	for _, syn := range entry.synonyms {
		if syn != "" && len(syn) > 2 && len(syn) < 200 {
			c.d.addXref(syn, textLinkID, chemID, c.source, true)
		}
	}

	// Cross-reference: CTD → MeSH (chemical is a MeSH descriptor)
	if _, exists := config.Dataconf["mesh"]; exists {
		c.d.addXref(chemID, sourceID, chemID, "mesh", false)
	}

	// Cross-reference: CTD → PubChem
	if entry.pubchemCID != "" {
		if _, exists := config.Dataconf["pubchem"]; exists {
			c.d.addXref(chemID, sourceID, entry.pubchemCID, "pubchem", false)
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
