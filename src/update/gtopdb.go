package update

import (
	"biobtree/pbuf"
	"encoding/csv"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pquerna/ffjson/ffjson"
)

// gtopdb handles parsing IUPHAR/BPS Guide to Pharmacology data
// Source: https://www.guidetopharmacology.org/
// Files processed:
// - targets_and_families.csv: Drug targets (GPCRs, ion channels, enzymes, etc.)
// - ligands.csv: Ligands (drugs, compounds, peptides)
// - interactions.csv: Ligand-target binding data with affinity values
// - ligand_id_mapping.csv: Cross-references to PubChem, ChEMBL, ChEBI
// - ligand_physchem_properties.csv: ADME-related physicochemical properties
// - peptides.csv: Peptide sequences
// - GtP_to_UniProt_mapping.csv: Target to UniProt mappings
// - GtP_to_HGNC_mapping.csv: Target to HGNC mappings
type gtopdb struct {
	source string
	d      *DataUpdate
}

// Helper for context-aware error checking
func (g *gtopdb) check(err error, operation string) {
	checkWithContext(err, g.source, operation)
}

// gtopdbTargetEntry holds parsed target data
type gtopdbTargetEntry struct {
	targetID   string
	name       string
	targetType string
	familyName string
	familyIDs  []int32
	subunitIDs []int32
	complexIDs []int32
	synonyms   []string
}

// gtopdbLigandEntry holds parsed ligand data
type gtopdbLigandEntry struct {
	ligandID       string
	name           string
	ligandType     string
	inn            string
	synonyms       []string
	approved       bool
	withdrawn      bool
	whoEssential   bool
	antibacterial  bool
	radioactive    bool
	labelled       bool
	smiles         string
	inchiKey       string
	// Physico-chemical properties
	molecularWeight   float64
	logP              float64
	hba               int32
	hbd               int32
	psa               float64
	rotatableBonds    int32
	lipinskiBroken    int32
	// Peptide sequences
	oneLetterSeq   string
	threeLetterSeq string
	// Cross-references
	pubchemCID  string
	chemblID    string
	chebiID     string
	drugbankID  string
	casNumber   string
}

// gtopdbInteractionEntry holds parsed interaction data
type gtopdbInteractionEntry struct {
	interactionID     string
	targetID          int32
	ligandID          int32
	targetName        string
	ligandName        string
	targetSpecies     string
	actionType        string
	action            string
	selectivity       string
	endogenous        bool
	primaryTarget     bool
	affinity          string
	affinityParameter string
	affinityUnits     string
	affinityValue     float64
	pubmedIDs         []string
}

func (g *gtopdb) update() {
	defer g.d.wg.Done()

	log.Println("GtoPdb: Starting data processing...")
	startTime := time.Now()

	// Test mode support
	testLimit := config.GetTestLimit(g.source)
	if config.IsTestMode() {
		log.Printf("GtoPdb: [TEST MODE] Processing up to %d entries per dataset", testLimit)
	}

	basePath := config.Dataconf[g.source]["path"]

	// Phase 1: Load cross-reference mappings
	log.Println("GtoPdb: Phase 1 - Loading cross-reference mappings...")
	uniprotMap := g.loadUniProtMappings(basePath)
	hgncMap := g.loadHGNCMappings(basePath)
	ligandXrefs := g.loadLigandIdMappings(basePath)
	physchemProps := g.loadPhyschemProperties(basePath)
	peptideSeqs := g.loadPeptideSequences(basePath)
	log.Printf("GtoPdb: Loaded %d UniProt, %d HGNC mappings, %d ligand xrefs, %d physchem, %d peptides",
		len(uniprotMap), len(hgncMap), len(ligandXrefs), len(physchemProps), len(peptideSeqs))

	// Phase 2: Process targets
	log.Println("GtoPdb: Phase 2 - Processing targets...")
	targetNames := g.processTargets(basePath, uniprotMap, hgncMap, testLimit)
	log.Printf("GtoPdb: Processed %d targets", len(targetNames))

	// Phase 3: Process ligands
	log.Println("GtoPdb: Phase 3 - Processing ligands...")
	ligandNames := g.processLigands(basePath, ligandXrefs, physchemProps, peptideSeqs, testLimit)
	log.Printf("GtoPdb: Processed %d ligands", len(ligandNames))

	// Phase 4: Process interactions
	log.Println("GtoPdb: Phase 4 - Processing interactions...")
	g.processInteractions(basePath, targetNames, ligandNames, testLimit)

	log.Printf("GtoPdb: Processing complete (%.2fs)", time.Since(startTime).Seconds())

	// Signal completion to progress tracker
	g.d.progChan <- &progressInfo{dataset: g.source, done: true}
}

// downloadCSV downloads a CSV file from URL and returns a reader
func (g *gtopdb) downloadCSV(url string) (io.ReadCloser, error) {
	log.Printf("GtoPdb: Downloading %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, &httpError{statusCode: resp.StatusCode, url: url}
	}
	return resp.Body, nil
}

type httpError struct {
	statusCode int
	url        string
}

func (e *httpError) Error() string {
	return "HTTP " + strconv.Itoa(e.statusCode) + " for " + e.url
}

// loadUniProtMappings loads target ID to UniProt accession mappings
func (g *gtopdb) loadUniProtMappings(basePath string) map[string][]string {
	mappings := make(map[string][]string)

	url := basePath + config.Dataconf[g.source]["uniprotMappingFile"]
	reader, err := g.downloadCSV(url)
	if err != nil {
		log.Printf("GtoPdb: Warning - could not load UniProt mappings: %v", err)
		return mappings
	}
	defer reader.Close()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1 // Variable fields

	// Read header, skip comment line if present
	header, err := csvReader.Read()
	if err != nil {
		return mappings
	}
	if len(header) > 0 && strings.HasPrefix(header[0], "# ") {
		header, err = csvReader.Read()
		if err != nil {
			return mappings
		}
	}

	// Build column map
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(strings.ToLower(name))] = i
	}

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Column names: "UniProtKB ID", "GtoPdb IUPHAR ID"
		// Note: UniProtKB ID column contains accessions (like O75342) but occasionally
		// has entry names (like MICU1_RAT) which we skip
		uniprotID := safeFieldByColLower(record, colMap, "uniprotkb id")
		targetID := safeFieldByColLower(record, colMap, "gtopdb iuphar id")
		if targetID != "" && uniprotID != "" && isValidUniProtAccession(uniprotID) {
			mappings[targetID] = append(mappings[targetID], uniprotID)
		}
	}

	return mappings
}

// loadHGNCMappings loads target ID to HGNC ID mappings
func (g *gtopdb) loadHGNCMappings(basePath string) map[string][]string {
	mappings := make(map[string][]string)

	url := basePath + config.Dataconf[g.source]["hgncMappingFile"]
	reader, err := g.downloadCSV(url)
	if err != nil {
		log.Printf("GtoPdb: Warning - could not load HGNC mappings: %v", err)
		return mappings
	}
	defer reader.Close()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1

	// Read header, skip comment line if present
	header, err := csvReader.Read()
	if err != nil {
		return mappings
	}
	if len(header) > 0 && strings.HasPrefix(header[0], "# ") {
		header, err = csvReader.Read()
		if err != nil {
			return mappings
		}
	}

	// Build column map
	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(strings.ToLower(name))] = i
	}

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Column names: "HGNC ID", "IUPHAR ID"
		hgncID := safeFieldByColLower(record, colMap, "hgnc id")
		targetID := safeFieldByColLower(record, colMap, "iuphar id")
		if targetID != "" && hgncID != "" {
			// Ensure HGNC: prefix
			if !strings.HasPrefix(hgncID, "HGNC:") {
				hgncID = "HGNC:" + hgncID
			}
			mappings[targetID] = append(mappings[targetID], hgncID)
		}
	}

	return mappings
}

// loadLigandIdMappings loads ligand cross-references to external databases
func (g *gtopdb) loadLigandIdMappings(basePath string) map[string]*gtopdbLigandEntry {
	mappings := make(map[string]*gtopdbLigandEntry)

	url := basePath + config.Dataconf["gtopdb_ligand"]["idMappingFile"]
	reader, err := g.downloadCSV(url)
	if err != nil {
		log.Printf("GtoPdb: Warning - could not load ligand ID mappings: %v", err)
		return mappings
	}
	defer reader.Close()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1

	// Read header, skip comment line if present
	header, err := csvReader.Read()
	if err != nil {
		return mappings
	}
	if len(header) > 0 && strings.HasPrefix(header[0], "# ") {
		header, err = csvReader.Read()
		if err != nil {
			return mappings
		}
	}

	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(strings.ToLower(name))] = i
	}

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		ligandID := safeFieldByColLower(record, colMap, "ligand id")
		if ligandID == "" {
			continue
		}

		entry := &gtopdbLigandEntry{
			ligandID:   ligandID,
			pubchemCID: safeFieldByColLower(record, colMap, "pubchem cid"),
			chemblID:   safeFieldByColLower(record, colMap, "chembl id"),
			chebiID:    safeFieldByColLower(record, colMap, "chebi id"),
			drugbankID: safeFieldByColLower(record, colMap, "drugbank id"),
			casNumber:  safeFieldByColLower(record, colMap, "cas"),
		}

		mappings[ligandID] = entry
	}

	return mappings
}

// loadPhyschemProperties loads physicochemical properties for ligands
func (g *gtopdb) loadPhyschemProperties(basePath string) map[string]*gtopdbLigandEntry {
	props := make(map[string]*gtopdbLigandEntry)

	url := basePath + config.Dataconf["gtopdb_ligand"]["physchemFile"]
	reader, err := g.downloadCSV(url)
	if err != nil {
		log.Printf("GtoPdb: Warning - could not load physchem properties: %v", err)
		return props
	}
	defer reader.Close()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1

	header, err := csvReader.Read()
	if err != nil {
		return props
	}
	// Skip comment lines (may use "# " or "## ")
	if len(header) > 0 && strings.HasPrefix(header[0], "#") {
		header, err = csvReader.Read()
		if err != nil {
			return props
		}
	}

	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(strings.ToLower(name))] = i
	}

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		ligandID := safeFieldByColLower(record, colMap, "ligand id")
		if ligandID == "" {
			continue
		}

		entry := &gtopdbLigandEntry{
			ligandID:        ligandID,
			molecularWeight: parseFloatFieldSafe(safeFieldByColLower(record, colMap, "mol weight")),
			logP:            parseFloatFieldSafe(safeFieldByColLower(record, colMap, "xlogp")),
			hba:             parseIntFieldSafe(safeFieldByColLower(record, colMap, "hbond acceptors")),
			hbd:             parseIntFieldSafe(safeFieldByColLower(record, colMap, "hbond donors")),
			psa:             parseFloatFieldSafe(safeFieldByColLower(record, colMap, "tpsa")),
			rotatableBonds:  parseIntFieldSafe(safeFieldByColLower(record, colMap, "rotatable bonds")),
			lipinskiBroken:  parseIntFieldSafe(safeFieldByColLower(record, colMap, "lipinskiro5")),
		}

		props[ligandID] = entry
	}

	return props
}

// loadPeptideSequences loads peptide sequences for peptide ligands
func (g *gtopdb) loadPeptideSequences(basePath string) map[string]*gtopdbLigandEntry {
	seqs := make(map[string]*gtopdbLigandEntry)

	url := basePath + config.Dataconf["gtopdb_ligand"]["peptidesFile"]
	reader, err := g.downloadCSV(url)
	if err != nil {
		log.Printf("GtoPdb: Warning - could not load peptide sequences: %v", err)
		return seqs
	}
	defer reader.Close()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1

	header, err := csvReader.Read()
	if err != nil {
		return seqs
	}
	if len(header) > 0 && strings.HasPrefix(header[0], "# ") {
		header, err = csvReader.Read()
		if err != nil {
			return seqs
		}
	}

	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(strings.ToLower(name))] = i
	}

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		ligandID := safeFieldByColLower(record, colMap, "ligand id")
		if ligandID == "" {
			continue
		}

		entry := &gtopdbLigandEntry{
			ligandID:       ligandID,
			oneLetterSeq:   safeFieldByColLower(record, colMap, "single letter amino acid sequence"),
			threeLetterSeq: safeFieldByColLower(record, colMap, "three letter amino acid sequence"),
		}

		seqs[ligandID] = entry
	}

	return seqs
}

// processTargets processes target entries and creates cross-references
func (g *gtopdb) processTargets(basePath string, uniprotMap, hgncMap map[string][]string, testLimit int) map[string]string {
	targetNames := make(map[string]string)

	targetSource := "gtopdb"
	if _, exists := config.Dataconf[targetSource]; !exists {
		log.Printf("GtoPdb: Warning - %s dataset not configured", targetSource)
		return targetNames
	}
	sourceID := config.Dataconf[targetSource]["id"]

	url := basePath + config.Dataconf[g.source]["targetsFile"]
	reader, err := g.downloadCSV(url)
	if err != nil {
		log.Printf("GtoPdb: Error downloading targets: %v", err)
		return targetNames
	}
	defer reader.Close()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.LazyQuotes = true

	// Skip comment line if present (starts with "# ")
	header, err := csvReader.Read()
	if err != nil {
		log.Printf("GtoPdb: Error reading targets header: %v", err)
		return targetNames
	}
	// Check if first line is a comment
	if len(header) > 0 && strings.HasPrefix(header[0], "# ") {
		header, err = csvReader.Read()
		if err != nil {
			log.Printf("GtoPdb: Error reading targets header after comment: %v", err)
			return targetNames
		}
	}

	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(strings.ToLower(name))] = i
	}

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, targetSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var entryCount int

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		targetID := safeFieldByColLower(record, colMap, "target id")
		if targetID == "" {
			continue
		}

		// Parse synonyms (pipe-separated, e.g., "5-HT1A|serotonin receptor 1A")
		var synonyms []string
		synonymsStr := safeFieldByColLower(record, colMap, "synonyms")
		if synonymsStr != "" {
			for _, syn := range strings.Split(synonymsStr, "|") {
				syn = strings.TrimSpace(syn)
				if syn != "" {
					synonyms = append(synonyms, syn)
				}
			}
		}

		entry := &gtopdbTargetEntry{
			targetID:   targetID,
			name:       safeFieldByColLower(record, colMap, "target name"),
			targetType: safeFieldByColLower(record, colMap, "type"),
			familyName: safeFieldByColLower(record, colMap, "family name"),
			synonyms:   synonyms,
		}

		// Parse family IDs (comma-separated)
		familyIDsStr := safeFieldByColLower(record, colMap, "family id")
		if familyIDsStr != "" {
			for _, idStr := range strings.Split(familyIDsStr, ",") {
				if id := parseIntFieldSafe(strings.TrimSpace(idStr)); id > 0 {
					entry.familyIDs = append(entry.familyIDs, id)
				}
			}
		}

		// Store target name for interaction lookup
		targetNames[targetID] = entry.name

		// Build protobuf attribute
		attr := &pbuf.GtopdbAttr{
			Name:       entry.name,
			Type:       entry.targetType,
			FamilyName: entry.familyName,
			FamilyIds:  entry.familyIDs,
			SubunitIds: entry.subunitIDs,
			ComplexIds: entry.complexIDs,
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("GtoPdb: Error marshaling target %s: %v", targetID, err)
			continue
		}

		// Save entry
		g.d.addProp3(targetID, sourceID, attrBytes)

		// Create cross-references
		g.createTargetCrossRefs(targetID, entry, sourceID, uniprotMap, hgncMap)

		if idLogFile != nil {
			idLogFile.WriteString(targetID + "\n")
		}

		entryCount++

		if testLimit > 0 && entryCount >= testLimit {
			log.Printf("GtoPdb: [TEST MODE] Reached target limit of %d", testLimit)
			break
		}

		if entryCount%500 == 0 {
			log.Printf("GtoPdb: Processed %d targets...", entryCount)
		}
	}

	atomic.AddUint64(&g.d.totalParsedEntry, uint64(entryCount))
	log.Printf("GtoPdb: Total target entries saved: %d", entryCount)

	// Signal completion for target dataset
	g.d.progChan <- &progressInfo{dataset: targetSource, done: true}

	return targetNames
}

// createTargetCrossRefs creates cross-references for a target
func (g *gtopdb) createTargetCrossRefs(targetID string, entry *gtopdbTargetEntry, sourceID string, uniprotMap, hgncMap map[string][]string) {
	// Text search: target name (cleaned of HTML)
	if entry.name != "" {
		cleanName := cleanGtoPdbText(entry.name)
		if cleanName != "" {
			g.d.addXref(cleanName, textLinkID, targetID, "gtopdb", true)
		}
	}

	// Text search: synonyms (e.g., "5-HT1A", "serotonin receptor 1A")
	for _, synonym := range entry.synonyms {
		cleanSyn := cleanGtoPdbText(synonym)
		if cleanSyn != "" && cleanSyn != entry.name {
			// Index full synonym
			g.d.addXref(cleanSyn, textLinkID, targetID, "gtopdb", true)
			// Also index significant individual words from multi-word synonyms
			for _, word := range extractSignificantWords(cleanSyn) {
				g.d.addXref(word, textLinkID, targetID, "gtopdb", true)
			}
		}
	}

	// Cross-reference to UniProt
	if uniprots, exists := uniprotMap[targetID]; exists {
		if _, uniprotExists := config.Dataconf["uniprot"]; uniprotExists {
			for _, uniprotID := range uniprots {
				g.d.addXref(targetID, sourceID, uniprotID, "uniprot", false)
			}
		}
	}

	// Cross-reference to HGNC
	if hgncs, exists := hgncMap[targetID]; exists {
		if _, hgncExists := config.Dataconf["hgnc"]; hgncExists {
			for _, hgncID := range hgncs {
				g.d.addXref(targetID, sourceID, hgncID, "hgnc", false)
			}
		}
	}
}

// processLigands processes ligand entries and creates cross-references
func (g *gtopdb) processLigands(basePath string, xrefs map[string]*gtopdbLigandEntry, physchem map[string]*gtopdbLigandEntry, peptides map[string]*gtopdbLigandEntry, testLimit int) map[string]string {
	ligandNames := make(map[string]string)

	ligandSource := "gtopdb_ligand"
	if _, exists := config.Dataconf[ligandSource]; !exists {
		log.Printf("GtoPdb: Warning - %s dataset not configured", ligandSource)
		return ligandNames
	}
	sourceID := config.Dataconf[ligandSource]["id"]

	url := basePath + config.Dataconf[ligandSource]["ligandsFile"]
	reader, err := g.downloadCSV(url)
	if err != nil {
		log.Printf("GtoPdb: Error downloading ligands: %v", err)
		return ligandNames
	}
	defer reader.Close()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.LazyQuotes = true

	// Skip comment line if present (starts with "# ")
	header, err := csvReader.Read()
	if err != nil {
		log.Printf("GtoPdb: Error reading ligands header: %v", err)
		return ligandNames
	}
	if len(header) > 0 && strings.HasPrefix(header[0], "# ") {
		header, err = csvReader.Read()
		if err != nil {
			log.Printf("GtoPdb: Error reading ligands header after comment: %v", err)
			return ligandNames
		}
	}

	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(strings.ToLower(name))] = i
	}

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, ligandSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var entryCount int

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		ligandID := safeFieldByColLower(record, colMap, "ligand id")
		if ligandID == "" {
			continue
		}

		// Parse synonyms (pipe-separated, e.g., "5-HT|serotonin")
		var synonyms []string
		synonymsStr := safeFieldByColLower(record, colMap, "synonyms")
		if synonymsStr != "" {
			for _, syn := range strings.Split(synonymsStr, "|") {
				syn = strings.TrimSpace(syn)
				if syn != "" {
					synonyms = append(synonyms, syn)
				}
			}
		}

		entry := &gtopdbLigandEntry{
			ligandID:      ligandID,
			name:          safeFieldByColLower(record, colMap, "name"),
			ligandType:    safeFieldByColLower(record, colMap, "type"),
			inn:           safeFieldByColLower(record, colMap, "inn"),
			synonyms:      synonyms,
			approved:      strings.ToLower(safeFieldByColLower(record, colMap, "approved")) == "yes",
			withdrawn:     strings.ToLower(safeFieldByColLower(record, colMap, "withdrawn")) == "yes",
			whoEssential:  strings.ToLower(safeFieldByColLower(record, colMap, "who essential medicine")) == "yes",
			antibacterial: strings.ToLower(safeFieldByColLower(record, colMap, "antibacterial")) == "yes",
			radioactive:   strings.ToLower(safeFieldByColLower(record, colMap, "radioactive")) == "yes",
			labelled:      strings.ToLower(safeFieldByColLower(record, colMap, "labelled")) == "yes",
			smiles:        safeFieldByColLower(record, colMap, "smiles"),
			inchiKey:      safeFieldByColLower(record, colMap, "inchikey"),
		}

		// Merge cross-reference data
		if xref, exists := xrefs[ligandID]; exists {
			entry.pubchemCID = xref.pubchemCID
			entry.chemblID = xref.chemblID
			entry.chebiID = xref.chebiID
			entry.drugbankID = xref.drugbankID
			entry.casNumber = xref.casNumber
		}

		// Merge physicochemical properties
		if props, exists := physchem[ligandID]; exists {
			entry.molecularWeight = props.molecularWeight
			entry.logP = props.logP
			entry.hba = props.hba
			entry.hbd = props.hbd
			entry.psa = props.psa
			entry.rotatableBonds = props.rotatableBonds
			entry.lipinskiBroken = props.lipinskiBroken
		}

		// Merge peptide sequences
		if pep, exists := peptides[ligandID]; exists {
			entry.oneLetterSeq = pep.oneLetterSeq
			entry.threeLetterSeq = pep.threeLetterSeq
		}

		// Store ligand name for interaction lookup
		ligandNames[ligandID] = entry.name

		// Build protobuf attribute
		attr := &pbuf.GtopdbLigandAttr{
			Name:            entry.name,
			Type:            entry.ligandType,
			Inn:             entry.inn,
			Approved:        entry.approved,
			Withdrawn:       entry.withdrawn,
			WhoEssential:    entry.whoEssential,
			Antibacterial:   entry.antibacterial,
			Radioactive:     entry.radioactive,
			Labelled:        entry.labelled,
			Smiles:          entry.smiles,
			InchiKey:        entry.inchiKey,
			MolecularWeight: entry.molecularWeight,
			Logp:            entry.logP,
			Hba:             entry.hba,
			Hbd:             entry.hbd,
			Psa:             entry.psa,
			RotatableBonds:  entry.rotatableBonds,
			LipinskiRulesBroken: entry.lipinskiBroken,
			OneLetterSeq:    entry.oneLetterSeq,
			ThreeLetterSeq:  entry.threeLetterSeq,
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("GtoPdb: Error marshaling ligand %s: %v", ligandID, err)
			continue
		}

		// Save entry
		g.d.addProp3(ligandID, sourceID, attrBytes)

		// Create cross-references
		g.createLigandCrossRefs(ligandID, entry, sourceID)

		if idLogFile != nil {
			idLogFile.WriteString(ligandID + "\n")
		}

		entryCount++

		if testLimit > 0 && entryCount >= testLimit {
			log.Printf("GtoPdb: [TEST MODE] Reached ligand limit of %d", testLimit)
			break
		}

		if entryCount%1000 == 0 {
			log.Printf("GtoPdb: Processed %d ligands...", entryCount)
		}
	}

	atomic.AddUint64(&g.d.totalParsedEntry, uint64(entryCount))
	log.Printf("GtoPdb: Total ligand entries saved: %d", entryCount)

	// Signal completion for ligand dataset
	g.d.progChan <- &progressInfo{dataset: ligandSource, done: true}

	return ligandNames
}

// createLigandCrossRefs creates cross-references for a ligand
func (g *gtopdb) createLigandCrossRefs(ligandID string, entry *gtopdbLigandEntry, sourceID string) {
	// Text search: ligand name and INN (cleaned of HTML)
	if entry.name != "" {
		cleanName := cleanGtoPdbText(entry.name)
		if cleanName != "" {
			g.d.addXref(cleanName, textLinkID, ligandID, "gtopdb_ligand", true)
		}
	}
	if entry.inn != "" && entry.inn != entry.name {
		cleanINN := cleanGtoPdbText(entry.inn)
		if cleanINN != "" {
			g.d.addXref(cleanINN, textLinkID, ligandID, "gtopdb_ligand", true)
		}
	}

	// Text search: synonyms (e.g., "serotonin" for 5-hydroxytryptamine)
	for _, synonym := range entry.synonyms {
		cleanSyn := cleanGtoPdbText(synonym)
		if cleanSyn != "" && cleanSyn != entry.name && cleanSyn != entry.inn {
			// Index full synonym
			g.d.addXref(cleanSyn, textLinkID, ligandID, "gtopdb_ligand", true)
			// Also index significant individual words from multi-word synonyms
			for _, word := range extractSignificantWords(cleanSyn) {
				g.d.addXref(word, textLinkID, ligandID, "gtopdb_ligand", true)
			}
		}
	}

	// Cross-reference to PubChem
	if entry.pubchemCID != "" {
		if _, exists := config.Dataconf["pubchem"]; exists {
			g.d.addXref(ligandID, sourceID, entry.pubchemCID, "pubchem", false)
		}
	}

	// Cross-reference to ChEMBL
	if entry.chemblID != "" {
		if _, exists := config.Dataconf["chembl_molecule"]; exists {
			g.d.addXref(ligandID, sourceID, entry.chemblID, "chembl_molecule", false)
		}
	}

	// Cross-reference to ChEBI
	if entry.chebiID != "" {
		if _, exists := config.Dataconf["chebi"]; exists {
			// Ensure CHEBI: prefix
			chebiID := entry.chebiID
			if !strings.HasPrefix(chebiID, "CHEBI:") {
				chebiID = "CHEBI:" + chebiID
			}
			g.d.addXref(ligandID, sourceID, chebiID, "chebi", false)
		}
	}
}

// processInteractions processes interaction entries and creates cross-references
func (g *gtopdb) processInteractions(basePath string, targetNames, ligandNames map[string]string, testLimit int) {
	interactionSource := "gtopdb_interaction"
	if _, exists := config.Dataconf[interactionSource]; !exists {
		log.Printf("GtoPdb: Warning - %s dataset not configured", interactionSource)
		return
	}
	sourceID := config.Dataconf[interactionSource]["id"]

	url := basePath + config.Dataconf[interactionSource]["interactionsFile"]
	reader, err := g.downloadCSV(url)
	if err != nil {
		log.Printf("GtoPdb: Error downloading interactions: %v", err)
		return
	}
	defer reader.Close()

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.LazyQuotes = true

	// Skip comment line if present (starts with "# ")
	header, err := csvReader.Read()
	if err != nil {
		log.Printf("GtoPdb: Error reading interactions header: %v", err)
		return
	}
	if len(header) > 0 && strings.HasPrefix(header[0], "# ") {
		header, err = csvReader.Read()
		if err != nil {
			log.Printf("GtoPdb: Error reading interactions header after comment: %v", err)
			return
		}
	}

	colMap := make(map[string]int)
	for i, name := range header {
		colMap[strings.TrimSpace(strings.ToLower(name))] = i
	}

	var idLogFile *os.File
	if config.IsTestMode() {
		idLogFile = openIDLogFile(config.TestRefDir, interactionSource+"_ids.txt")
		if idLogFile != nil {
			defer idLogFile.Close()
		}
	}

	var entryCount int

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		targetIDStr := safeFieldByColLower(record, colMap, "target id")
		ligandIDStr := safeFieldByColLower(record, colMap, "ligand id")

		if targetIDStr == "" || ligandIDStr == "" {
			continue
		}

		targetID := parseIntFieldSafe(targetIDStr)
		ligandID := parseIntFieldSafe(ligandIDStr)

		// Create composite interaction ID
		interactionID := targetIDStr + "_" + ligandIDStr

		// Get affinity value and parameter
		affinityStr := safeFieldByColLower(record, colMap, "affinity median")
		if affinityStr == "" {
			affinityStr = safeFieldByColLower(record, colMap, "affinity high")
		}
		if affinityStr == "" {
			affinityStr = safeFieldByColLower(record, colMap, "affinity low")
		}

		entry := &gtopdbInteractionEntry{
			interactionID:     interactionID,
			targetID:          targetID,
			ligandID:          ligandID,
			targetName:        targetNames[targetIDStr],
			ligandName:        ligandNames[ligandIDStr],
			targetSpecies:     safeFieldByColLower(record, colMap, "target species"),
			actionType:        safeFieldByColLower(record, colMap, "type"),
			action:            safeFieldByColLower(record, colMap, "action"),
			selectivity:       safeFieldByColLower(record, colMap, "selectivity"),
			endogenous:        strings.ToLower(safeFieldByColLower(record, colMap, "endogenous")) == "true",
			primaryTarget:     strings.ToLower(safeFieldByColLower(record, colMap, "primary target")) == "true",
			affinity:          affinityStr,
			affinityParameter: safeFieldByColLower(record, colMap, "affinity units"),
			affinityValue:     parseFloatFieldSafe(affinityStr),
		}

		// Parse PubMed IDs
		pubmedStr := safeFieldByColLower(record, colMap, "pubmed id")
		if pubmedStr != "" {
			for _, pmid := range strings.Split(pubmedStr, "|") {
				pmid = strings.TrimSpace(pmid)
				if pmid != "" && len(entry.pubmedIDs) < 10 {
					entry.pubmedIDs = append(entry.pubmedIDs, pmid)
				}
			}
		}

		// Build protobuf attribute
		attr := &pbuf.GtopdbInteractionAttr{
			TargetId:          entry.targetID,
			LigandId:          entry.ligandID,
			TargetName:        entry.targetName,
			LigandName:        entry.ligandName,
			TargetSpecies:     entry.targetSpecies,
			Type:              entry.actionType,
			Action:            entry.action,
			Selectivity:       entry.selectivity,
			Endogenous:        entry.endogenous,
			PrimaryTarget:     entry.primaryTarget,
			Affinity:          entry.affinity,
			AffinityParameter: entry.affinityParameter,
			AffinityUnits:     entry.affinityUnits,
			AffinityValue:     entry.affinityValue,
			PubmedIds:         entry.pubmedIDs,
		}

		attrBytes, err := ffjson.Marshal(attr)
		if err != nil {
			log.Printf("GtoPdb: Error marshaling interaction %s: %v", interactionID, err)
			continue
		}

		// Save entry
		g.d.addProp3(interactionID, sourceID, attrBytes)

		// Create cross-references
		g.createInteractionCrossRefs(interactionID, entry, sourceID, targetIDStr, ligandIDStr)

		if idLogFile != nil {
			idLogFile.WriteString(interactionID + "\n")
		}

		entryCount++

		if testLimit > 0 && entryCount >= testLimit {
			log.Printf("GtoPdb: [TEST MODE] Reached interaction limit of %d", testLimit)
			break
		}

		if entryCount%5000 == 0 {
			log.Printf("GtoPdb: Processed %d interactions...", entryCount)
		}
	}

	atomic.AddUint64(&g.d.totalParsedEntry, uint64(entryCount))
	log.Printf("GtoPdb: Total interaction entries saved: %d", entryCount)

	// Signal completion for interaction dataset
	g.d.progChan <- &progressInfo{dataset: interactionSource, done: true}
}

// createInteractionCrossRefs creates cross-references for an interaction
func (g *gtopdb) createInteractionCrossRefs(interactionID string, entry *gtopdbInteractionEntry, sourceID, targetIDStr, ligandIDStr string) {
	// Cross-reference to target (bidirectional)
	if _, exists := config.Dataconf["gtopdb"]; exists {
		targetSourceID := config.Dataconf["gtopdb"]["id"]
		g.d.addXref(interactionID, sourceID, targetIDStr, "gtopdb", false)
		g.d.addXref(targetIDStr, targetSourceID, interactionID, "gtopdb_interaction", false)
	}

	// Cross-reference to ligand (bidirectional)
	if _, exists := config.Dataconf["gtopdb_ligand"]; exists {
		ligandSourceID := config.Dataconf["gtopdb_ligand"]["id"]
		g.d.addXref(interactionID, sourceID, ligandIDStr, "gtopdb_ligand", false)
		g.d.addXref(ligandIDStr, ligandSourceID, interactionID, "gtopdb_interaction", false)
	}

	// Cross-reference to PubMed
	if _, exists := config.Dataconf["pubmed"]; exists {
		for _, pmid := range entry.pubmedIDs {
			g.d.addXref(interactionID, sourceID, pmid, "pubmed", false)
		}
	}
}

// Helper functions

// safeFieldByColLower returns field value by lowercase column name or empty string
func safeFieldByColLower(fields []string, colMap map[string]int, colName string) string {
	if idx, exists := colMap[colName]; exists && idx < len(fields) {
		return strings.TrimSpace(fields[idx])
	}
	return ""
}

// parseIntFieldSafe parses integer field with fallback to 0
func parseIntFieldSafe(s string) int32 {
	if s == "" {
		return 0
	}
	val, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(val)
}

// parseFloatFieldSafe parses float field with fallback to 0
func parseFloatFieldSafe(s string) float64 {
	if s == "" {
		return 0
	}
	// Handle range values like "7.1 - 7.5" by taking midpoint
	if strings.Contains(s, "-") && !strings.HasPrefix(s, "-") {
		parts := strings.Split(s, "-")
		if len(parts) == 2 {
			low, errLow := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			high, errHigh := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if errLow == nil && errHigh == nil {
				return (low + high) / 2
			}
		}
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return val
}

// isValidUniProtAccession checks if the string looks like a UniProt accession
// UniProt accessions are 6 or 10 alphanumeric characters (e.g., O75342, A0A1W2PQ64)
// Entry names (e.g., MICU1_RAT) contain underscores and should be skipped
func isValidUniProtAccession(s string) bool {
	if len(s) < 6 || len(s) > 10 {
		return false
	}
	// Entry names contain underscore, accessions don't
	if strings.Contains(s, "_") {
		return false
	}
	// Accessions are alphanumeric only
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// cleanGtoPdbText removes HTML tags and decodes HTML entities from GtoPdb text
// GtoPdb uses HTML for formatting (e.g., <sub>, <sup>) and entities (e.g., &alpha;)
func cleanGtoPdbText(s string) string {
	if s == "" {
		return ""
	}
	// Remove HTML tags like <sub>, </sub>, <sup>, </sup>, etc.
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	s = tagRegex.ReplaceAllString(s, "")
	// Decode HTML entities like &alpha;, &beta;, &amp;, etc.
	s = html.UnescapeString(s)
	// Clean up whitespace
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// gtopdbStopWords are common words in pharmacology that shouldn't be indexed alone
var gtopdbStopWords = map[string]bool{
	"receptor": true, "receptors": true,
	"channel": true, "channels": true,
	"protein": true, "proteins": true,
	"enzyme": true, "enzymes": true,
	"transporter": true, "transporters": true,
	"kinase": true, "kinases": true,
	"coupled": true, "binding": true,
	"subunit": true, "subunits": true,
	"type": true, "family": true,
	"alpha": true, "beta": true, "gamma": true, "delta": true,
	"human": true, "mouse": true, "rat": true,
	"the": true, "and": true, "for": true, "with": true,
}

// extractSignificantWords extracts meaningful words from a multi-word phrase
// Returns words that are significant enough to be indexed separately
func extractSignificantWords(phrase string) []string {
	words := strings.Fields(phrase)
	if len(words) <= 1 {
		return nil // Single word, already indexed as-is
	}

	var significant []string
	for _, word := range words {
		word = strings.ToLower(strings.Trim(word, "(),"))
		// Skip short words (like "1A", "2B") and stop words
		if len(word) < 4 {
			continue
		}
		if gtopdbStopWords[word] {
			continue
		}
		significant = append(significant, word)
	}
	return significant
}
