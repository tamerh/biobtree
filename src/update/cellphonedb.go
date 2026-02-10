package update

import (
	"biobtree/pbuf"
	"bufio"
	"encoding/csv"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// cellphonedb parses CellPhoneDB ligand-receptor interactions
// Source: https://www.cellphonedb.org/
// Format: Multiple CSV files (interaction_table.csv, multidata_table.csv, gene_table.csv, etc.)
type cellphonedb struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (c *cellphonedb) check(err error, operation string) {
	checkWithContext(err, c.source, operation)
}

// multidataEntry represents an entity (protein or complex) in CellPhoneDB
type multidataEntry struct {
	ID           string
	Name         string
	Receptor     bool
	Secreted     bool
	Transmembrane bool
	Integrin     bool
	IsComplex    bool
}

// geneEntry represents gene information linked to a protein
type geneEntry struct {
	Ensembl    string
	GeneName   string
	HGNCSymbol string
	ProteinID  string
}

func (c *cellphonedb) update() {
	defer c.d.wg.Done()

	log.Println("CellPhoneDB: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(c.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, c.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("CellPhoneDB: [TEST MODE] Processing up to %d entries", testLimit)
	}

	// Get path and source ID
	basePath := config.Dataconf[c.source]["path"]
	sourceID := config.Dataconf[c.source]["id"]

	// Load supporting data files
	multidata := c.loadMultidataTable(basePath)
	genes := c.loadGeneTable(basePath)
	complexComp := c.loadComplexComposition(basePath)

	log.Printf("CellPhoneDB: Loaded %d entities, %d genes, %d complex components", len(multidata), len(genes), len(complexComp))

	// Build protein_id -> gene map
	geneByProteinID := make(map[string]*geneEntry)
	for _, g := range genes {
		geneByProteinID[g.ProteinID] = g
	}

	// Process interactions
	interactionPath := filepath.Join(basePath, "interaction_table.csv")
	file, err := os.Open(interactionPath)
	c.check(err, "opening interaction_table.csv")
	defer file.Close()

	reader := csv.NewReader(bufio.NewReaderSize(file, fileBufSize))

	// Read header
	header, err := reader.Read()
	c.check(err, "reading interaction_table header")

	// Map column names to indices
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	var savedEntries int
	var previous int64

	// PMID extraction regex
	pmidRegex := regexp.MustCompile(`PMID:(\d+)`)

	for {
		row, err := reader.Read()
		if err != nil {
			break // EOF or error
		}

		// Progress tracking
		elapsed := int64(time.Since(c.d.start).Seconds())
		if elapsed > previous+c.d.progInterval {
			previous = elapsed
			if elapsed > 0 {
				c.d.progChan <- &progressInfo{dataset: c.source, currentKBPerSec: int64(savedEntries / int(elapsed))}
			}
		}

		// Extract fields
		interactionID := getField(row, colMap, "id_cp_interaction")
		if interactionID == "" {
			continue
		}

		multidata1ID := getField(row, colMap, "multidata_1_id")
		multidata2ID := getField(row, colMap, "multidata_2_id")
		source := getField(row, colMap, "source")
		annotationStrategy := getField(row, colMap, "annotation_strategy")
		isPPIStr := getField(row, colMap, "is_ppi")
		directionality := getField(row, colMap, "directionality")
		classification := getField(row, colMap, "classification")

		// Get entity details
		entityA := multidata[multidata1ID]
		entityB := multidata[multidata2ID]

		if entityA == nil || entityB == nil {
			continue
		}

		// Collect genes for each partner
		genesA := c.getGenesForEntity(entityA, geneByProteinID, complexComp, multidata)
		genesB := c.getGenesForEntity(entityB, geneByProteinID, complexComp, multidata)

		// Build attribute
		attr := &pbuf.CellphonedbAttr{
			PartnerA:       entityA.Name,
			PartnerB:       entityB.Name,
			Directionality: directionality,
			Classification: classification,
			Source:         source,
			IsPpi:          strings.ToLower(isPPIStr) == "true",
			ReceptorA:      entityA.Receptor,
			SecretedA:      entityA.Secreted,
			IsComplexA:     entityA.IsComplex,
			GenesA:         genesA,
			ReceptorB:      entityB.Receptor,
			SecretedB:      entityB.Secreted,
			IsComplexB:     entityB.IsComplex,
			GenesB:         genesB,
			IsIntegrin:     entityA.Integrin || entityB.Integrin,
		}

		// Marshal and save
		attrBytes, err := ffjson.Marshal(attr)
		c.check(err, "marshaling CellPhoneDB attributes")

		c.d.addProp3(interactionID, sourceID, attrBytes)

		// Text search indexing - make partner names searchable
		c.d.addXref(entityA.Name, textLinkID, interactionID, c.source, true)
		c.d.addXref(entityB.Name, textLinkID, interactionID, c.source, true)

		// Index gene symbols for search
		for _, gene := range genesA {
			c.d.addXref(gene, textLinkID, interactionID, c.source, true)
		}
		for _, gene := range genesB {
			c.d.addXref(gene, textLinkID, interactionID, c.source, true)
		}

		// Cross-reference to UniProt (for non-complex entities)
		if !entityA.IsComplex && isUniProtID(entityA.Name) {
			c.d.addXref(interactionID, sourceID, entityA.Name, "uniprot", false)
		}
		if !entityB.IsComplex && isUniProtID(entityB.Name) {
			c.d.addXref(interactionID, sourceID, entityB.Name, "uniprot", false)
		}

		// Cross-reference to Ensembl via gene IDs
		for _, gene := range genesA {
			if geneInfo := c.findGeneBySymbol(genes, gene); geneInfo != nil && geneInfo.Ensembl != "" {
				c.d.addXref(interactionID, sourceID, geneInfo.Ensembl, "ensembl", false)
			}
		}
		for _, gene := range genesB {
			if geneInfo := c.findGeneBySymbol(genes, gene); geneInfo != nil && geneInfo.Ensembl != "" {
				c.d.addXref(interactionID, sourceID, geneInfo.Ensembl, "ensembl", false)
			}
		}

		// Cross-reference to HGNC via gene symbols (human genes)
		for _, gene := range genesA {
			c.d.addHumanGeneXrefs(gene, interactionID, sourceID)
		}
		for _, gene := range genesB {
			c.d.addHumanGeneXrefs(gene, interactionID, sourceID)
		}

		// Extract and cross-reference PMIDs from source field
		if matches := pmidRegex.FindAllStringSubmatch(source, -1); matches != nil {
			for _, match := range matches {
				if len(match) > 1 {
					c.d.addXref(interactionID, sourceID, match[1], "pubmed", false)
				}
			}
		}

		// Index by annotation strategy if specified
		if annotationStrategy != "" {
			c.d.addXref(annotationStrategy, textLinkID, interactionID, c.source, true)
		}

		// Index by classification for search
		if classification != "" {
			c.d.addXref(classification, textLinkID, interactionID, c.source, true)
		}

		// Log ID for testing
		if idLogFile != nil {
			logProcessedID(idLogFile, interactionID)
		}

		savedEntries++

		// Test mode: check limit
		if testLimit > 0 && savedEntries >= testLimit {
			log.Printf("CellPhoneDB: [TEST MODE] Reached limit of %d entries, stopping", testLimit)
			break
		}
	}

	log.Printf("CellPhoneDB: Saved %d ligand-receptor interactions", savedEntries)
	log.Printf("CellPhoneDB: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Update entry statistics
	atomic.AddUint64(&c.d.totalParsedEntry, uint64(savedEntries))

	// Signal completion
	c.d.progChan <- &progressInfo{dataset: c.source, done: true}
}

// loadMultidataTable loads the multidata_table.csv into a map
func (c *cellphonedb) loadMultidataTable(basePath string) map[string]*multidataEntry {
	result := make(map[string]*multidataEntry)

	filePath := filepath.Join(basePath, "multidata_table.csv")
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("CellPhoneDB: Warning - could not open multidata_table.csv: %v", err)
		return result
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReaderSize(file, fileBufSize))

	// Read header
	header, err := reader.Read()
	if err != nil {
		return result
	}

	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	for {
		row, err := reader.Read()
		if err != nil {
			break
		}

		id := getField(row, colMap, "id_multidata")
		entry := &multidataEntry{
			ID:           id,
			Name:         getField(row, colMap, "name"),
			Receptor:     strings.ToLower(getField(row, colMap, "receptor")) == "true",
			Secreted:     strings.ToLower(getField(row, colMap, "secreted")) == "true",
			Transmembrane: strings.ToLower(getField(row, colMap, "transmembrane")) == "true",
			Integrin:     strings.ToLower(getField(row, colMap, "integrin")) == "true",
			IsComplex:    strings.ToLower(getField(row, colMap, "is_complex")) == "true",
		}
		result[id] = entry
	}

	return result
}

// loadGeneTable loads the gene_table.csv
func (c *cellphonedb) loadGeneTable(basePath string) []*geneEntry {
	var result []*geneEntry

	filePath := filepath.Join(basePath, "gene_table.csv")
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("CellPhoneDB: Warning - could not open gene_table.csv: %v", err)
		return result
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReaderSize(file, fileBufSize))

	// Read header
	header, err := reader.Read()
	if err != nil {
		return result
	}

	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	for {
		row, err := reader.Read()
		if err != nil {
			break
		}

		entry := &geneEntry{
			Ensembl:    getField(row, colMap, "ensembl"),
			GeneName:   getField(row, colMap, "gene_name"),
			HGNCSymbol: getField(row, colMap, "hgnc_symbol"),
			ProteinID:  getField(row, colMap, "protein_id"),
		}
		result = append(result, entry)
	}

	return result
}

// loadComplexComposition loads the complex_composition_table.csv
func (c *cellphonedb) loadComplexComposition(basePath string) map[string][]string {
	result := make(map[string][]string)

	filePath := filepath.Join(basePath, "complex_composition_table.csv")
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("CellPhoneDB: Warning - could not open complex_composition_table.csv: %v", err)
		return result
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReaderSize(file, fileBufSize))

	// Read header
	header, err := reader.Read()
	if err != nil {
		return result
	}

	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(name)] = i
	}

	for {
		row, err := reader.Read()
		if err != nil {
			break
		}

		complexID := getField(row, colMap, "complex_multidata_id")
		proteinID := getField(row, colMap, "protein_multidata_id")

		if complexID != "" && proteinID != "" {
			result[complexID] = append(result[complexID], proteinID)
		}
	}

	return result
}

// getGenesForEntity collects gene symbols for an entity (protein or complex)
func (c *cellphonedb) getGenesForEntity(entity *multidataEntry, geneByProteinID map[string]*geneEntry, complexComp map[string][]string, multidata map[string]*multidataEntry) []string {
	var genes []string
	seenGenes := make(map[string]bool)

	if entity.IsComplex {
		// For complexes, get genes from all subunits
		if subunits, ok := complexComp[entity.ID]; ok {
			for _, subunitID := range subunits {
				if geneInfo, ok := geneByProteinID[subunitID]; ok {
					if geneInfo.GeneName != "" && !seenGenes[geneInfo.GeneName] {
						genes = append(genes, geneInfo.GeneName)
						seenGenes[geneInfo.GeneName] = true
					}
				}
			}
		}
	} else {
		// For single proteins, look up gene by multidata ID (which maps to protein_id)
		if geneInfo, ok := geneByProteinID[entity.ID]; ok {
			if geneInfo.GeneName != "" {
				genes = append(genes, geneInfo.GeneName)
			}
		}
	}

	return genes
}

// findGeneBySymbol finds gene info by gene symbol
func (c *cellphonedb) findGeneBySymbol(genes []*geneEntry, symbol string) *geneEntry {
	for _, g := range genes {
		if g.GeneName == symbol || g.HGNCSymbol == symbol {
			return g
		}
	}
	return nil
}

// isUniProtID checks if a string looks like a UniProt accession
func isUniProtID(s string) bool {
	// UniProt accessions are typically 6 or 10 characters
	// Format: [A-Z][0-9A-Z]{5} or [A-Z][0-9A-Z]{9}
	if len(s) != 6 && len(s) != 10 {
		return false
	}
	if s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}
