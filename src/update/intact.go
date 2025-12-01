package update

import (
	"biobtree/pbuf"
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

type intact struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (i *intact) check(err error, operation string) {
	checkWithContext(err, i.source, operation)
}

// Main update entry point
func (i *intact) update() {
	defer i.d.wg.Done()

	log.Println("IntAct: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(i.source)
	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, i.source+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
		log.Printf("IntAct: [TEST MODE] Processing up to %d interactions", testLimit)
	}

	// Process PSI-MITAB file
	i.parseAndSaveInteractions(testLimit, idLogFile)

	log.Printf("IntAct: Processing complete (%.2fs)", time.Since(startTime).Seconds())
}

// parseAndSaveInteractions processes the IntAct PSI-MITAB 2.7 file
func (i *intact) parseAndSaveInteractions(testLimit int, idLogFile *os.File) {
	// Download from EBI FTP
	ftpServer := config.Dataconf[i.source]["ftpUrl"]
	filePath := config.Dataconf[i.source]["path"]

	if config.IsTestMode() {
		log.Printf("IntAct: [TEST MODE] Downloading from ftp://%s%s (will stop after %d interactions)", ftpServer, filePath, testLimit)
	} else {
		log.Printf("IntAct: Downloading from ftp://%s%s", ftpServer, filePath)
	}

	// Open PSI-MITAB file from FTP
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(i.source, ftpServer, "", filePath)
	i.check(err, "opening IntAct PSI-MITAB file")
	defer closeReaders(gz, ftpFile, client, localFile)

	// Use bufio.Scanner for line-by-line reading (similar to GWAS)
	scanner := bufio.NewScanner(br)
	const maxCapacity = 1024 * 1024 // 1MB buffer for long lines
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	// Read and parse header
	if !scanner.Scan() {
		i.check(scanner.Err(), "reading IntAct header")
		return
	}
	headerLine := scanner.Text()
	header := strings.Split(headerLine, "\t")

	// Map column names to indices
	colMap := make(map[string]int)
	for idx, name := range header {
		colMap[strings.TrimSpace(name)] = idx
	}

	log.Printf("IntAct: Found %d columns in header", len(colMap))

	// Group interactions by protein UniProt ID
	// Key: UniProt ID, Value: list of interactions for that protein
	proteinInteractions := make(map[string][]*pbuf.IntactInteraction)

	var processedCount int
	var previous int64
	var skippedLines int
	var totalRowsRead int
	var uniqueProteins int

	// Source ID for cross-references
	sourceID := config.Dataconf[i.source]["id"]

	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		totalRowsRead++

		// Split by tab (manual parsing like GWAS and Rhea)
		row := strings.Split(line, "\t")

		// In test mode, check if we've reached the limit
		if testLimit > 0 && processedCount >= testLimit {
			log.Printf("IntAct: [TEST MODE] Reached limit of %d interactions, stopping", testLimit)
			break
		}

		// Progress reporting
		elapsed := int64(time.Since(i.d.start).Seconds())
		if elapsed > previous+i.d.progInterval {
			previous = elapsed
			i.d.progChan <- &progressInfo{dataset: i.source, currentKBPerSec: int64(processedCount / int(elapsed))}
		}

		// Extract UniProt IDs for both proteins
		// Try primary IDs first, then alternate IDs
		proteinA := i.extractUniProtID(getField(row, colMap, "ID(s) interactor A"))
		if proteinA == "" {
			proteinA = i.extractUniProtID(getField(row, colMap, "Alt. ID(s) interactor A"))
		}

		proteinB := i.extractUniProtID(getField(row, colMap, "ID(s) interactor B"))
		if proteinB == "" {
			proteinB = i.extractUniProtID(getField(row, colMap, "Alt. ID(s) interactor B"))
		}

		// Skip if either protein doesn't have a UniProt ID
		if proteinA == "" || proteinB == "" {
			skippedLines++
			continue
		}

		// In test mode, filter to only human-human interactions (taxid:9606)
		if config.IsTestMode() {
			taxidA := i.extractTaxid(getField(row, colMap, "Taxid interactor A"))
			taxidB := i.extractTaxid(getField(row, colMap, "Taxid interactor B"))
			if taxidA != 9606 || taxidB != 9606 {
				continue
			}
		}

		// Build interaction object
		interaction := i.buildInteraction(row, colMap, proteinA, proteinB)
		if interaction == nil {
			skippedLines++
			continue
		}

		// Store bidirectionally (A→B and B→A)
		if _, exists := proteinInteractions[proteinA]; !exists {
			uniqueProteins++
		}
		proteinInteractions[proteinA] = append(proteinInteractions[proteinA], interaction)

		// Create reverse interaction (B→A)
		reverseInteraction := i.buildInteraction(row, colMap, proteinB, proteinA)
		if reverseInteraction != nil {
			if _, exists := proteinInteractions[proteinB]; !exists {
				uniqueProteins++
			}
			proteinInteractions[proteinB] = append(proteinInteractions[proteinB], reverseInteraction)
		}

		processedCount++
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("IntAct: Scanner error: %v", err)
	}

	log.Printf("IntAct: Total rows read: %d, Interactions processed: %d", totalRowsRead, processedCount)
	log.Printf("IntAct: Unique proteins: %d, Skipped: %d", uniqueProteins, skippedLines)

	// Now save grouped protein interactions
	savedProteins := 0
	for uniprotID, interactions := range proteinInteractions {
		// Calculate summary statistics
		uniquePartners := make(map[string]bool)
		highConfCount := 0
		partnerOrgs := make(map[int32]bool)

		for _, interaction := range interactions {
			uniquePartners[interaction.PartnerUniprot] = true
			if interaction.ConfidenceScore > 0.6 {
				highConfCount++
			}
			partnerOrgs[interaction.TaxidB] = true
		}

		// Convert partner organisms map to slice
		partnerOrgsList := make([]int32, 0, len(partnerOrgs))
		for taxid := range partnerOrgs {
			partnerOrgsList = append(partnerOrgsList, taxid)
		}

		// Save protein
		i.saveProtein(uniprotID, interactions, uniquePartners, highConfCount, partnerOrgsList, sourceID)

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(uniprotID + "\n")
		}

		savedProteins++
	}

	log.Printf("IntAct: Saved %d unique proteins with interactions", savedProteins)

	// Update entry statistics
	atomic.AddUint64(&i.d.totalParsedEntry, uint64(savedProteins))
	i.d.addEntryStat(i.source, uint64(savedProteins))
}

// extractUniProtID extracts UniProt accession from the ID field
// Format: "uniprotkb:P49418" or "intact:EBI-xxx"
func (i *intact) extractUniProtID(idField string) string {
	// Split by pipe to handle multiple IDs
	ids := strings.Split(idField, "|")

	// Prefer uniprotkb: entries
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "uniprotkb:") {
			return strings.TrimPrefix(id, "uniprotkb:")
		}
	}

	return ""
}

// extractTaxid extracts NCBI taxonomy ID
// Format: "taxid:9606(human)|taxid:9606(Homo sapiens)"
func (i *intact) extractTaxid(taxidField string) int32 {
	// Split by pipe
	parts := strings.Split(taxidField, "|")
	if len(parts) == 0 {
		return 0
	}

	// Parse first taxid
	taxidStr := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(taxidStr, "taxid:") {
		return 0
	}

	// Extract number: "taxid:9606(human)" → "9606"
	taxidStr = strings.TrimPrefix(taxidStr, "taxid:")
	if idx := strings.Index(taxidStr, "("); idx > 0 {
		taxidStr = taxidStr[:idx]
	}

	taxid, err := strconv.ParseInt(taxidStr, 10, 32)
	if err != nil {
		return 0
	}

	return int32(taxid)
}

// extractGeneName extracts gene name from alias field
// Format: "psi-mi:amph_human(display_long)|uniprotkb:AMPH(gene name)|..."
func (i *intact) extractGeneName(aliasField string) string {
	// Split by pipe
	aliases := strings.Split(aliasField, "|")

	// Look for "uniprotkb:XXX(gene name)"
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if strings.HasPrefix(alias, "uniprotkb:") && strings.Contains(alias, "(gene name)") {
			// Extract gene name: "uniprotkb:AMPH(gene name)" → "AMPH"
			geneName := strings.TrimPrefix(alias, "uniprotkb:")
			if idx := strings.Index(geneName, "("); idx > 0 {
				return geneName[:idx]
			}
		}
	}

	return ""
}

// extractInteractionID extracts IntAct interaction ID
// Format: "intact:EBI-7121552|mint:MINT-16056"
func (i *intact) extractInteractionID(idField string) string {
	// Split by pipe
	ids := strings.Split(idField, "|")

	// Prefer intact: IDs
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "intact:") {
			return id // Keep "intact:EBI-xxx" format
		}
	}

	// Fall back to first ID
	if len(ids) > 0 {
		return strings.TrimSpace(ids[0])
	}

	return ""
}

// extractPSIMITerm extracts PSI-MI term
// Format: "psi-mi:\"MI:0018\"(two hybrid)"
func (i *intact) extractPSIMITerm(field string) string {
	if field == "" || field == "-" {
		return ""
	}
	// Return as-is (already formatted)
	return field
}

// extractConfidenceScore extracts MIscore
// Format: "intact-miscore:0.67"
func (i *intact) extractConfidenceScore(confidenceField string) float64 {
	if confidenceField == "" || confidenceField == "-" {
		return 0.0
	}

	// Split by colon
	parts := strings.Split(confidenceField, ":")
	if len(parts) != 2 {
		return 0.0
	}

	score, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0.0
	}

	return score
}

// extractPubMedID extracts PubMed ID
// Format: "pubmed:10542231|mint:MINT-5211933"
func (i *intact) extractPubMedID(pubField string) string {
	// Split by pipe
	ids := strings.Split(pubField, "|")

	// Look for pubmed: entries
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "pubmed:") {
			return strings.TrimPrefix(id, "pubmed:")
		}
	}

	return ""
}

// extractSourceDatabase extracts source database
// Format: "psi-mi:\"MI:0471\"(MINT)"
func (i *intact) extractSourceDatabase(sourceField string) string {
	if sourceField == "" || sourceField == "-" {
		return ""
	}

	// Extract database name from PSI-MI term: "psi-mi:\"MI:0471\"(MINT)" → "MINT"
	if idx := strings.LastIndex(sourceField, "("); idx > 0 {
		if endIdx := strings.Index(sourceField[idx:], ")"); endIdx > 0 {
			return sourceField[idx+1 : idx+endIdx]
		}
	}

	return sourceField
}

// buildInteraction creates an interaction record from a row
func (i *intact) buildInteraction(row []string, colMap map[string]int, proteinA, proteinB string) *pbuf.IntactInteraction {
	// Extract interaction ID
	interactionID := i.extractInteractionID(getField(row, colMap, "Interaction identifier(s)"))

	// Extract partner gene name
	partnerGeneName := i.extractGeneName(getField(row, colMap, "Alias(es) interactor B"))

	// Extract experimental details
	detectionMethod := i.extractPSIMITerm(getField(row, colMap, "Interaction detection method(s)"))
	interactionType := i.extractPSIMITerm(getField(row, colMap, "Interaction type(s)"))

	// Extract confidence score
	confidenceScore := i.extractConfidenceScore(getField(row, colMap, "Confidence value(s)"))

	// Extract experimental roles (if available - columns 19, 20)
	expRoleA := ""
	expRoleB := ""
	if len(row) > 19 {
		expRoleA = i.extractPSIMITerm(getField(row, colMap, "Experimental role(s) interactor A"))
	}
	if len(row) > 20 {
		expRoleB = i.extractPSIMITerm(getField(row, colMap, "Experimental role(s) interactor B"))
	}

	// Extract taxonomy IDs
	taxidA := i.extractTaxid(getField(row, colMap, "Taxid interactor A"))
	taxidB := i.extractTaxid(getField(row, colMap, "Taxid interactor B"))

	// Extract organism names
	organismA := ""
	organismB := ""
	taxFieldA := getField(row, colMap, "Taxid interactor A")
	taxFieldB := getField(row, colMap, "Taxid interactor B")
	if idx := strings.Index(taxFieldA, "("); idx > 0 {
		if endIdx := strings.Index(taxFieldA[idx:], ")"); endIdx > 0 {
			organismA = taxFieldA[idx+1 : idx+endIdx]
		}
	}
	if idx := strings.Index(taxFieldB, "("); idx > 0 {
		if endIdx := strings.Index(taxFieldB[idx:], ")"); endIdx > 0 {
			organismB = taxFieldB[idx+1 : idx+endIdx]
		}
	}

	// Extract publication info
	pubmedID := i.extractPubMedID(getField(row, colMap, "Publication Identifier(s)"))
	firstAuthor := getField(row, colMap, "Publication 1st author(s)")

	// Extract source database
	sourceDB := i.extractSourceDatabase(getField(row, colMap, "Source database(s)"))

	// Extract negative flag (if available - column 36)
	isNegative := false
	if len(row) > 36 {
		negField := getField(row, colMap, "Negative")
		isNegative = (negField == "true")
	}

	// Extract dates (if available - columns 31, 32)
	creationDate := ""
	updateDate := ""
	if len(row) > 31 {
		creationDate = getField(row, colMap, "Creation date")
	}
	if len(row) > 32 {
		updateDate = getField(row, colMap, "Update date")
	}

	return &pbuf.IntactInteraction{
		InteractionId:      interactionID,
		PartnerUniprot:     proteinB,
		PartnerGeneName:    partnerGeneName,
		DetectionMethod:    detectionMethod,
		InteractionType:    interactionType,
		ConfidenceScore:    confidenceScore,
		ExperimentalRoleA:  expRoleA,
		ExperimentalRoleB:  expRoleB,
		TaxidA:             taxidA,
		TaxidB:             taxidB,
		OrganismA:          organismA,
		OrganismB:          organismB,
		PubmedId:           pubmedID,
		FirstAuthor:        firstAuthor,
		SourceDatabase:     sourceDB,
		IsNegative:         isNegative,
		CreationDate:       creationDate,
		UpdateDate:         updateDate,
	}
}

// saveProtein saves a protein with all its interactions
func (i *intact) saveProtein(uniprotID string, interactions []*pbuf.IntactInteraction,
	uniquePartners map[string]bool, highConfCount int, partnerOrgs []int32, sourceID string) {

	attr := &pbuf.IntactAttr{
		ProteinId:           uniprotID,
		Interactions:        interactions,
		InteractionCount:    int32(len(interactions)),
		UniquePartners:      int32(len(uniquePartners)),
		HighConfidenceCount: int32(highConfCount),
		PartnerOrganisms:    partnerOrgs,
	}

	// Marshal attributes
	attrBytes, err := ffjson.Marshal(attr)
	i.check(err, fmt.Sprintf("marshaling IntAct attributes for %s", uniprotID))

	// Save entry
	i.d.addProp3(uniprotID, sourceID, attrBytes)

	// Create cross-references
	i.createCrossReferences(uniprotID, interactions, sourceID)
}

// createCrossReferences creates cross-references from proteins to partners and publications
func (i *intact) createCrossReferences(uniprotID string, interactions []*pbuf.IntactInteraction, sourceID string) {
	textLinkID := "0" // Text search link ID

	// Protein → Intact (text search for protein ID)
	i.d.addXref(uniprotID, textLinkID, uniprotID, i.source, true)

	// Track unique entries to avoid duplicate xrefs
	uniquePartners := make(map[string]bool)
	uniquePubMeds := make(map[string]bool)
	uniqueGenes := make(map[string]bool)

	for _, interaction := range interactions {
		// Protein → Partner protein
		if interaction.PartnerUniprot != "" && !uniquePartners[interaction.PartnerUniprot] {
			i.d.addXref(uniprotID, sourceID, interaction.PartnerUniprot, "uniprot", false)
			uniquePartners[interaction.PartnerUniprot] = true
		}

		// Protein → PubMed (skip non-numeric IDs like "UNASSIGNED1312")
		if interaction.PubmedId != "" && !uniquePubMeds[interaction.PubmedId] {
			// Validate PubMed ID is numeric
			if len(interaction.PubmedId) > 0 && interaction.PubmedId[0] >= '0' && interaction.PubmedId[0] <= '9' {
				i.d.addXref(uniprotID, sourceID, interaction.PubmedId, "pubmed", false)
				uniquePubMeds[interaction.PubmedId] = true
			} else {
				log.Printf("[%s] Skipping non-numeric PubMed ID: '%s' for protein: %s", i.source, interaction.PubmedId, uniprotID)
			}
		}

		// Gene name → Protein (text search)
		if interaction.PartnerGeneName != "" && len(interaction.PartnerGeneName) < 50 && !uniqueGenes[interaction.PartnerGeneName] {
			i.d.addXref(interaction.PartnerGeneName, textLinkID, uniprotID, i.source, true)
			uniqueGenes[interaction.PartnerGeneName] = true
		}
	}
}
