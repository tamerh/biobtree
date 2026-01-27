package update

import (
	"biobtree/pbuf"
	"bufio"
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

	// Signal completion to progress handler so status is updated from "processing" to "processed"
	i.d.progChan <- &progressInfo{dataset: i.source, done: true}
}

// parseAndSaveInteractions processes the IntAct PSI-MITAB 2.7 file
// New approach: Each interaction is stored as a separate entry keyed by interaction_id
// Cross-references link both proteins to the interaction entry
func (i *intact) parseAndSaveInteractions(testLimit int, idLogFile *os.File) {
	// Get full URL from config (includes ftp://host/path)
	fullURL := config.Dataconf[i.source]["path"]

	if config.IsTestMode() {
		log.Printf("IntAct: [TEST MODE] Downloading from %s (will stop after %d interactions)", fullURL, testLimit)
	} else {
		log.Printf("IntAct: Downloading from %s", fullURL)
	}

	// Open PSI-MITAB file from FTP (pass full FTP URL directly)
	br, gz, ftpFile, client, localFile, _, err := getDataReaderNew(i.source, "", "", fullURL)
	i.check(err, "opening IntAct PSI-MITAB file")
	defer closeReaders(gz, ftpFile, client, localFile)

	// Use bufio.Scanner for line-by-line reading
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
		name = strings.TrimSpace(name)
		// PSI-MITAB format often has # at start of first column header
		if idx == 0 && strings.HasPrefix(name, "#") {
			name = strings.TrimPrefix(name, "#")
		}
		colMap[name] = idx
	}

	log.Printf("IntAct: Found %d columns in header", len(colMap))

	// Source ID for cross-references
	sourceID := config.Dataconf[i.source]["id"]
	textLinkID := "0" // Text search link ID

	var processedCount int
	var previous int64
	var skippedLines int
	var totalRowsRead int
	var selfInteractions int
	uniqueProteins := make(map[string]bool)

	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		totalRowsRead++

		// Split by tab
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

		// Extract interaction ID (required)
		interactionID := i.extractInteractionID(getField(row, colMap, "Interaction identifier(s)"))
		if interactionID == "" {
			skippedLines++
			continue
		}

		// Get ID fields for both interactors
		idFieldA := getField(row, colMap, "ID(s) interactor A")
		altIdFieldA := getField(row, colMap, "Alt. ID(s) interactor A")
		idFieldB := getField(row, colMap, "ID(s) interactor B")
		altIdFieldB := getField(row, colMap, "Alt. ID(s) interactor B")

		// Extract UniProt IDs for both interactors
		proteinA := i.extractUniProtID(idFieldA)
		if proteinA == "" {
			proteinA = i.extractUniProtID(altIdFieldA)
		}

		proteinB := i.extractUniProtID(idFieldB)
		if proteinB == "" {
			proteinB = i.extractUniProtID(altIdFieldB)
		}

		// Extract ChEBI IDs (for small molecule interactions)
		chebiA := i.extractChEBIID(idFieldA)
		if chebiA == "" {
			chebiA = i.extractChEBIID(altIdFieldA)
		}
		chebiB := i.extractChEBIID(idFieldB)
		if chebiB == "" {
			chebiB = i.extractChEBIID(altIdFieldB)
		}

		// Extract RNAcentral IDs (for RNA interactions)
		rnaA := i.extractRNAcentralID(idFieldA)
		if rnaA == "" {
			rnaA = i.extractRNAcentralID(altIdFieldA)
		}
		rnaB := i.extractRNAcentralID(idFieldB)
		if rnaB == "" {
			rnaB = i.extractRNAcentralID(altIdFieldB)
		}

		// Extract interactor types
		typeA := i.extractInteractorType(getField(row, colMap, "Type(s) interactor A"))
		typeB := i.extractInteractorType(getField(row, colMap, "Type(s) interactor B"))

		// Determine if this is a valid interaction we can process:
		// - Protein-Protein: both have UniProt IDs
		// - Protein-ChEBI: one UniProt + one ChEBI
		// - Protein-RNA: one UniProt + one RNAcentral
		hasProteinA := proteinA != ""
		hasProteinB := proteinB != ""
		hasChEBIA := chebiA != ""
		hasChEBIB := chebiB != ""
		hasRNAA := rnaA != ""
		hasRNAB := rnaB != ""

		// Skip if we can't identify the interactors
		// Valid interactions: Protein-Protein, Protein-ChEBI, or Protein-RNA
		validInteraction := (hasProteinA && hasProteinB) ||
			(hasProteinA && hasChEBIB) || (hasChEBIA && hasProteinB) ||
			(hasProteinA && hasRNAB) || (hasRNAA && hasProteinB)

		if !validInteraction {
			skippedLines++
			continue
		}

		// In test mode, filter to only human interactions (taxid:9606)
		if config.IsTestMode() {
			taxidA := i.extractTaxid(getField(row, colMap, "Taxid interactor A"))
			taxidB := i.extractTaxid(getField(row, colMap, "Taxid interactor B"))
			// For non-protein interactors, taxid might be 0 or -1, so only check protein side
			if hasProteinA && hasProteinB {
				if taxidA != 9606 || taxidB != 9606 {
					continue
				}
			} else if hasProteinA && taxidA != 9606 {
				continue
			} else if hasProteinB && taxidB != 9606 {
				continue
			}
		}

		// Track unique proteins
		if hasProteinA {
			uniqueProteins[proteinA] = true
		}
		if hasProteinB {
			uniqueProteins[proteinB] = true
		}

		// Check for self-interaction (protein-protein only)
		if hasProteinA && hasProteinB && proteinA == proteinB {
			selfInteractions++
		}

		// Build and save interaction entry
		attr := i.buildInteractionAttrFull(row, colMap, interactionID, proteinA, proteinB,
			chebiA, chebiB, rnaA, rnaB, typeA, typeB)
		if attr == nil {
			skippedLines++
			continue
		}

		// Marshal and save interaction entry (keyed by interaction_id)
		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("IntAct: Error marshaling interaction %s: %v", interactionID, err)
			skippedLines++
			continue
		}
		i.d.addProp3(interactionID, sourceID, attrBytes)

		// Create cross-references based on interactor types
		// interaction (IntAct) → UniProt proteins
		if hasProteinA {
			i.d.addXref(interactionID, sourceID, proteinA, "uniprot", false)
		}
		if hasProteinB && proteinA != proteinB {
			i.d.addXref(interactionID, sourceID, proteinB, "uniprot", false)
		}

		// interaction (IntAct) → ChEBI compounds
		if hasChEBIA {
			i.d.addXref(interactionID, sourceID, chebiA, "chebi", false)
		}
		if hasChEBIB && chebiA != chebiB {
			i.d.addXref(interactionID, sourceID, chebiB, "chebi", false)
		}

		// interaction (IntAct) → RNAcentral
		if hasRNAA {
			i.d.addXref(interactionID, sourceID, rnaA, "rnacentral", false)
		}
		if hasRNAB && rnaA != rnaB {
			i.d.addXref(interactionID, sourceID, rnaB, "rnacentral", false)
		}

		// Text search: interaction_id → interaction entry
		i.d.addXref(interactionID, textLinkID, interactionID, i.source, true)

		// Cross-reference to PubMed
		if attr.PubmedId != "" && len(attr.PubmedId) > 0 && attr.PubmedId[0] >= '0' && attr.PubmedId[0] <= '9' {
			i.d.addXref(interactionID, sourceID, attr.PubmedId, "pubmed", false)
		}

		// Log ID for testing
		if idLogFile != nil {
			idLogFile.WriteString(interactionID + "\n")
		}

		processedCount++
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Printf("IntAct: Scanner error: %v", err)
	}

	log.Printf("IntAct: Total rows read: %d, Interactions saved: %d", totalRowsRead, processedCount)
	log.Printf("IntAct: Unique proteins: %d, Self-interactions: %d, Skipped: %d", len(uniqueProteins), selfInteractions, skippedLines)

	// Update entry statistics
	atomic.AddUint64(&i.d.totalParsedEntry, uint64(processedCount))
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

// extractChEBIID extracts ChEBI identifier from the ID field
// Format: chebi:"CHEBI:50210" → "CHEBI:50210"
func (i *intact) extractChEBIID(idField string) string {
	ids := strings.Split(idField, "|")
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "chebi:") {
			// Remove chebi: prefix and quotes
			chebiID := strings.TrimPrefix(id, "chebi:")
			chebiID = strings.Trim(chebiID, "\"")
			return chebiID
		}
	}
	return ""
}

// extractRNAcentralID extracts RNAcentral identifier from the ID field
// Format: rnacentral:URS00002AFD52_559292 → "URS00002AFD52" (strips taxid suffix)
func (i *intact) extractRNAcentralID(idField string) string {
	ids := strings.Split(idField, "|")
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "rnacentral:") {
			rnaID := strings.TrimPrefix(id, "rnacentral:")
			// Strip taxid suffix (everything after underscore)
			if idx := strings.Index(rnaID, "_"); idx > 0 {
				rnaID = rnaID[:idx]
			}
			return rnaID
		}
	}
	return ""
}

// extractInteractorType extracts the interactor type from PSI-MI term
// Format: psi-mi:"MI:0326"(protein) → "protein"
func (i *intact) extractInteractorType(typeField string) string {
	if typeField == "" || typeField == "-" {
		return ""
	}
	// Extract term name from last parentheses
	if idx := strings.LastIndex(typeField, "("); idx >= 0 {
		if endIdx := strings.LastIndex(typeField, ")"); endIdx > idx {
			return typeField[idx+1 : endIdx]
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
// Format: "intact:EBI-7121552|mint:MINT-16056" → "EBI-7121552"
func (i *intact) extractInteractionID(idField string) string {
	// Split by pipe
	ids := strings.Split(idField, "|")

	// Prefer intact: IDs, strip prefix
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if strings.HasPrefix(id, "intact:") {
			return strings.TrimPrefix(id, "intact:") // Return just "EBI-xxx"
		}
	}

	// Fall back to first ID, strip any prefix
	if len(ids) > 0 {
		id := strings.TrimSpace(ids[0])
		if colonIdx := strings.Index(id, ":"); colonIdx > 0 {
			return id[colonIdx+1:]
		}
		return id
	}

	return ""
}

// extractPSIMITerm extracts PSI-MI term (legacy - returns raw string)
// Format: "psi-mi:\"MI:0018\"(two hybrid)"
func (i *intact) extractPSIMITerm(field string) string {
	if field == "" || field == "-" {
		return ""
	}
	// Return as-is (already formatted)
	return field
}

// parsePsiMiTerm parses PSI-MI formatted string into structured PsiMiTerm
// Format: "psi-mi:\"MI:0018\"(two hybrid)" → {mi_id: "MI:0018", term_name: "two hybrid"}
func (i *intact) parsePsiMiTerm(field string) *pbuf.PsiMiTerm {
	if field == "" || field == "-" {
		return nil
	}

	term := &pbuf.PsiMiTerm{FullString: field}

	// Extract MI ID: look for "MI:XXXX" pattern
	miStart := strings.Index(field, "MI:")
	if miStart >= 0 {
		// Find end of MI ID (next quote or parenthesis)
		miEnd := miStart + 3
		for miEnd < len(field) && (field[miEnd] >= '0' && field[miEnd] <= '9') {
			miEnd++
		}
		term.MiId = field[miStart:miEnd]
	}

	// Extract term name: content in last parentheses
	lastOpen := strings.LastIndex(field, "(")
	if lastOpen >= 0 && lastOpen < len(field)-1 {
		lastClose := strings.LastIndex(field, ")")
		if lastClose > lastOpen {
			term.TermName = field[lastOpen+1 : lastClose]
		}
	}

	return term
}

// parseConfidenceScores parses confidence score components
// Format: "intact-miscore:0.56|author-score:0.8|method-score:0.6"
func (i *intact) parseConfidenceScores(field string) *pbuf.IntactConfidenceScores {
	if field == "" || field == "-" {
		return nil
	}

	scores := &pbuf.IntactConfidenceScores{RawString: field}

	// Split by pipe
	parts := strings.Split(field, "|")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		colonIdx := strings.Index(part, ":")
		if colonIdx <= 0 {
			continue
		}
		key := part[:colonIdx]
		valueStr := part[colonIdx+1:]
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		switch key {
		case "intact-miscore":
			scores.Miscore = value
		case "method-score":
			scores.MethodScore = value
		case "type-score":
			scores.TypeScore = value
		case "author-score":
			scores.AuthorScore = value
		}
	}

	return scores
}

// parseHostOrganism parses host organism field
// Format: "taxid:4932(Saccharomyces cerevisiae)" or "taxid:-1(in vitro)"
func (i *intact) parseHostOrganism(field string) (int32, string) {
	if field == "" || field == "-" {
		return 0, ""
	}

	// Take first entry if multiple (pipe-separated)
	parts := strings.Split(field, "|")
	if len(parts) == 0 {
		return 0, ""
	}
	field = strings.TrimSpace(parts[0])

	// Extract taxid
	var taxid int32
	if strings.HasPrefix(field, "taxid:") {
		taxidStr := strings.TrimPrefix(field, "taxid:")
		if idx := strings.Index(taxidStr, "("); idx > 0 {
			taxidStr = taxidStr[:idx]
		}
		taxidVal, err := strconv.ParseInt(taxidStr, 10, 32)
		if err == nil {
			taxid = int32(taxidVal)
		}
	}

	// Extract organism name from parentheses
	var orgName string
	if idx := strings.Index(field, "("); idx > 0 {
		if endIdx := strings.LastIndex(field, ")"); endIdx > idx {
			orgName = field[idx+1 : endIdx]
		}
	}

	return taxid, orgName
}

// parseFeatures parses binding site features
// Format: "binding-associated region:23-45|mutation:R175H(MI:0118)"
func (i *intact) parseFeatures(field string) []*pbuf.IntactFeature {
	if field == "" || field == "-" {
		return nil
	}

	var features []*pbuf.IntactFeature
	parts := strings.Split(field, "|")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "-" {
			continue
		}

		feature := &pbuf.IntactFeature{Description: part}

		// Try to extract feature type (before first colon)
		colonIdx := strings.Index(part, ":")
		if colonIdx > 0 {
			feature.FeatureType = part[:colonIdx]
			remaining := part[colonIdx+1:]

			// Try to extract range (digits-digits pattern)
			dashIdx := strings.Index(remaining, "-")
			if dashIdx > 0 {
				startStr := ""
				endStr := ""
				// Extract start number
				for j := dashIdx - 1; j >= 0 && remaining[j] >= '0' && remaining[j] <= '9'; j-- {
					startStr = string(remaining[j]) + startStr
				}
				// Extract end number
				for j := dashIdx + 1; j < len(remaining) && remaining[j] >= '0' && remaining[j] <= '9'; j++ {
					endStr = endStr + string(remaining[j])
				}
				if startStr != "" {
					start, _ := strconv.ParseInt(startStr, 10, 32)
					feature.RangeStart = int32(start)
				}
				if endStr != "" {
					end, _ := strconv.ParseInt(endStr, 10, 32)
					feature.RangeEnd = int32(end)
				}
			}
		}

		// Extract MI term if present
		if miStart := strings.Index(part, "MI:"); miStart >= 0 {
			miEnd := miStart + 3
			for miEnd < len(part) && (part[miEnd] >= '0' && part[miEnd] <= '9') {
				miEnd++
			}
			feature.MiType = part[miStart:miEnd]
		}

		features = append(features, feature)
	}

	return features
}

// parseStoichiometry parses stoichiometry field
// Format: "1" or "-"
func (i *intact) parseStoichiometry(field string) int32 {
	if field == "" || field == "-" {
		return 0
	}
	val, err := strconv.ParseInt(strings.TrimSpace(field), 10, 32)
	if err != nil {
		return 0
	}
	return int32(val)
}

// parseParameters parses kinetic parameters
// Format: "kd:1.2e-9(M)|ic50:5.3e-8(M)"
func (i *intact) parseParameters(field string) []*pbuf.IntactParameter {
	if field == "" || field == "-" {
		return nil
	}

	var params []*pbuf.IntactParameter
	parts := strings.Split(field, "|")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "-" {
			continue
		}

		param := &pbuf.IntactParameter{RawString: part}

		// Extract type and value
		colonIdx := strings.Index(part, ":")
		if colonIdx > 0 {
			param.ParameterType = part[:colonIdx]
			remaining := part[colonIdx+1:]

			// Extract value (before parenthesis)
			parenIdx := strings.Index(remaining, "(")
			valueStr := remaining
			if parenIdx > 0 {
				valueStr = remaining[:parenIdx]
				// Extract unit from parentheses
				if endIdx := strings.Index(remaining, ")"); endIdx > parenIdx {
					param.Unit = remaining[parenIdx+1 : endIdx]
				}
			}

			// Parse value (handle scientific notation)
			if val, err := strconv.ParseFloat(valueStr, 64); err == nil {
				param.Value = val
			}
		}

		params = append(params, param)
	}

	return params
}

// parseInteractionXrefs parses interaction cross-references
// Format: "go:\"GO:0045947\"(negative regulation...)|reactome:R-HSA-123"
func (i *intact) parseInteractionXrefs(field string) ([]*pbuf.IntactXref, string) {
	if field == "" || field == "-" {
		return nil, ""
	}

	var xrefs []*pbuf.IntactXref
	var imexID string

	parts := strings.Split(field, "|")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "-" {
			continue
		}

		colonIdx := strings.Index(part, ":")
		if colonIdx <= 0 {
			continue
		}

		db := part[:colonIdx]
		identifier := part[colonIdx+1:]

		// Extract identifier (may be quoted)
		identifier = strings.Trim(identifier, "\"")
		// Remove parenthetical description if present
		if parenIdx := strings.Index(identifier, "("); parenIdx > 0 {
			identifier = strings.TrimSpace(identifier[:parenIdx])
		}
		identifier = strings.Trim(identifier, "\"")

		switch db {
		case "go":
			// Keep GO: prefix
			if !strings.HasPrefix(identifier, "GO:") {
				identifier = "GO:" + identifier
			}
			xrefs = append(xrefs, &pbuf.IntactXref{Database: "go", Identifier: identifier})
		case "reactome":
			xrefs = append(xrefs, &pbuf.IntactXref{Database: "reactome", Identifier: identifier})
		case "imex":
			imexID = identifier
		}
	}

	return xrefs, imexID
}

// methodReliabilityScores maps PSI-MI method IDs to reliability scores (0-1)
var methodReliabilityScores = map[string]float64{
	"MI:0114": 1.0,  // X-ray crystallography
	"MI:0077": 0.95, // NMR
	"MI:0107": 0.9,  // Surface plasmon resonance
	"MI:0019": 0.8,  // Co-immunoprecipitation
	"MI:0004": 0.7,  // Affinity chromatography
	"MI:0096": 0.65, // Pull-down
	"MI:0018": 0.6,  // Two hybrid
	"MI:0071": 0.5,  // Molecular sieving
	"MI:0364": 0.4,  // Inferred by curator
}

// getMethodReliabilityScore returns reliability score for a detection method
func (i *intact) getMethodReliabilityScore(miID string) float64 {
	if miID == "" {
		return 0.0
	}
	if score, ok := methodReliabilityScores[miID]; ok {
		return score
	}
	return 0.5 // Default for unknown methods
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
// buildInteractionAttr creates an IntactAttr for a single interaction entry
// Entry key will be the interaction_id, and cross-refs link both proteins to this entry
func (i *intact) buildInteractionAttr(row []string, colMap map[string]int, interactionID, proteinA, proteinB string) *pbuf.IntactAttr {
	// Extract gene names for both proteins
	geneA := i.extractGeneName(getField(row, colMap, "Alias(es) interactor A"))
	geneB := i.extractGeneName(getField(row, colMap, "Alias(es) interactor B"))

	// Extract experimental details (legacy raw strings)
	detectionMethodRaw := getField(row, colMap, "Interaction detection method(s)")
	interactionTypeRaw := getField(row, colMap, "Interaction type(s)")
	detectionMethod := i.extractPSIMITerm(detectionMethodRaw)
	interactionType := i.extractPSIMITerm(interactionTypeRaw)

	// Parse structured PSI-MI terms (P0)
	detectionMethodParsed := i.parsePsiMiTerm(detectionMethodRaw)
	interactionTypeParsed := i.parsePsiMiTerm(interactionTypeRaw)

	// Parse biological roles (P0)
	biologicalRoleA := i.parsePsiMiTerm(getField(row, colMap, "Biological role(s) interactor A"))
	biologicalRoleB := i.parsePsiMiTerm(getField(row, colMap, "Biological role(s) interactor B"))

	// Extract confidence score (legacy)
	confidenceScore := i.extractConfidenceScore(getField(row, colMap, "Confidence value(s)"))

	// Parse confidence score components (P0)
	confidenceScores := i.parseConfidenceScores(getField(row, colMap, "Confidence value(s)"))

	// Extract experimental roles (if available)
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

	// Parse host organism (P2)
	hostTaxid, hostOrgName := i.parseHostOrganism(getField(row, colMap, "Host organism(s)"))

	// Extract publication info
	pubmedID := i.extractPubMedID(getField(row, colMap, "Publication Identifier(s)"))
	firstAuthor := getField(row, colMap, "Publication 1st author(s)")

	// Extract source database
	sourceDB := i.extractSourceDatabase(getField(row, colMap, "Source database(s)"))

	// Extract negative flag (if available)
	isNegative := false
	if len(row) > 36 {
		negField := getField(row, colMap, "Negative")
		isNegative = (negField == "true")
	}

	// Extract dates (if available)
	creationDate := ""
	updateDate := ""
	if len(row) > 31 {
		creationDate = getField(row, colMap, "Creation date")
	}
	if len(row) > 32 {
		updateDate = getField(row, colMap, "Update date")
	}

	// Parse binding site features (P1)
	featuresA := i.parseFeatures(getField(row, colMap, "Feature(s) interactor A"))
	featuresB := i.parseFeatures(getField(row, colMap, "Feature(s) interactor B"))

	// Parse stoichiometry (P2)
	stoichA := i.parseStoichiometry(getField(row, colMap, "Stoichiometry(s) interactor A"))
	stoichB := i.parseStoichiometry(getField(row, colMap, "Stoichiometry(s) interactor B"))

	// Parse kinetic parameters (P2)
	parameters := i.parseParameters(getField(row, colMap, "Interaction parameter(s)"))

	// Parse interaction cross-references (P1/P2)
	interactionXrefs, imexID := i.parseInteractionXrefs(getField(row, colMap, "Interaction Xref(s)"))

	// Calculate method reliability score (P1)
	var methodReliability float64
	if detectionMethodParsed != nil && detectionMethodParsed.MiId != "" {
		methodReliability = i.getMethodReliabilityScore(detectionMethodParsed.MiId)
	}

	return &pbuf.IntactAttr{
		InteractionId:          interactionID,
		ProteinA:               proteinA,
		ProteinAGene:           geneA,
		ProteinB:               proteinB,
		ProteinBGene:           geneB,
		DetectionMethod:        detectionMethod,
		InteractionType:        interactionType,
		ConfidenceScore:        confidenceScore,
		ExperimentalRoleA:      expRoleA,
		ExperimentalRoleB:      expRoleB,
		TaxidA:                 taxidA,
		TaxidB:                 taxidB,
		OrganismA:              organismA,
		OrganismB:              organismB,
		PubmedId:               pubmedID,
		FirstAuthor:            firstAuthor,
		SourceDatabase:         sourceDB,
		IsNegative:             isNegative,
		CreationDate:           creationDate,
		UpdateDate:             updateDate,
		DetectionMethodParsed:  detectionMethodParsed,
		InteractionTypeParsed:  interactionTypeParsed,
		BiologicalRoleA:        biologicalRoleA,
		BiologicalRoleB:        biologicalRoleB,
		ConfidenceScores:       confidenceScores,
		HostTaxid:              hostTaxid,
		HostOrganismName:       hostOrgName,
		FeaturesA:              featuresA,
		FeaturesB:              featuresB,
		StoichiometryA:         stoichA,
		StoichiometryB:         stoichB,
		Parameters:             parameters,
		InteractionXrefs:       interactionXrefs,
		ImexId:                 imexID,
		MethodReliabilityScore: methodReliability,
	}
}

// buildInteractionAttrFull creates an IntactAttr with support for non-protein interactors
// This extends buildInteractionAttr to handle protein-ChEBI and protein-RNA interactions
func (i *intact) buildInteractionAttrFull(row []string, colMap map[string]int,
	interactionID, proteinA, proteinB, chebiA, chebiB, rnaA, rnaB, typeA, typeB string) *pbuf.IntactAttr {

	// Use existing buildInteractionAttr for the base attributes
	attr := i.buildInteractionAttr(row, colMap, interactionID, proteinA, proteinB)
	if attr == nil {
		return nil
	}

	// Add the new fields for non-protein interactors
	attr.InteractorTypeA = typeA
	attr.InteractorTypeB = typeB
	attr.ChebiA = chebiA
	attr.ChebiB = chebiB
	attr.RnacentralA = rnaA
	attr.RnacentralB = rnaB

	return attr
}
